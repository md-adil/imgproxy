package image

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/h2non/bimg"

	"imgproxy/internal/cache"
	"imgproxy/internal/config"
	"imgproxy/internal/image/etag"
)

var (
	// HTTP client with keepalive + timeouts
	httpClient = &http.Client{
		Timeout: 12 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          200,
			MaxIdleConnsPerHost:   50,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	bufPool = sync.Pool{
		New: func() any { return new(bytes.Buffer) },
	}
)

func safeProcessImage(origImg []byte, opts bimg.Options) (newImg []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in bimg processing: %v", r)
			err = fmt.Errorf("internal processing error")
		}
	}()
	newImg, err = bimg.NewImage(origImg).Process(opts)
	return
}

func writeImage(w http.ResponseWriter, img []byte, mediaType bimg.ImageType, tag string) {
	mimeType := fmt.Sprintf("image/%s", bimg.ImageTypeName(mediaType))
	header := w.Header()
	header.Set("Content-Type", mimeType)

	header.Set("Cache-Control", "public, max-age=2592000")

	if tag != "" {
		header.Set("ETag", tag)
	}

	header.Set("Vary", "Accept")

	header.Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(img); err != nil {
		log.Printf("write error: %v\n", err)
	}
}

func handleImage(store *cache.Store, w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	srcDomain := query.Get("d")

	width, _ := strconv.Atoi(query.Get("w"))
	height, _ := strconv.Atoi(query.Get("h"))
	quality, _ := strconv.Atoi(query.Get("q"))

	// Clamp dimensions and quality to sane limits
	const maxDim = 8000
	const maxQuality = 100
	if width < 0 || width > maxDim {
		http.Error(w, "Invalid width", http.StatusBadRequest)
		return
	}
	if height < 0 || height > maxDim {
		http.Error(w, "Invalid height", http.StatusBadRequest)
		return
	}
	if quality < 0 || quality > maxQuality {
		http.Error(w, "Invalid quality", http.StatusBadRequest)
		return
	}

	// Source scheme: http by default, ssl=true switches to https
	scheme := "http"
	switch query.Get("ssl") {
	case "", "false":
	case "true":
		scheme = "https"
	default:
		http.Error(w, "Invalid ssl", http.StatusBadRequest)
		return
	}

	// Route prefix is stripped by the mux (see IMAGE_PATH_PREFIX)
	srcURL := fmt.Sprintf("%s://%s/%s", scheme, srcDomain, r.URL.Path)

	mediaType := GetMediaType(r.Header.Get("Accept"))

	options := bimg.Options{
		Force:         false,
		Enlarge:       false,
		Compression:   1,
		StripMetadata: true,
		Type:          mediaType,
	}

	if quality > 0 {
		options.Quality = quality
	}

	if width > 0 {
		options.Width = width
	}

	if height > 0 {
		options.Height = height
	}

	// Cache key depends only on source URL and options, so hits skip fetching the source.
	// With caching enabled it also serves as the ETag, keeping hits and misses consistent.
	var cacheKey string
	if store != nil {
		cacheKey = etag.Generate(nil, srcURL, options)
		if etag.Matched(r, cacheKey) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if img, ok := store.Get(cacheKey); ok {
			writeImage(w, img, mediaType, cacheKey)
			return
		}
	}

	log.Println("Processing Image:", srcURL)

	// Use request context so upstream fetch is cancelled if client disconnects
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, srcURL, nil)
	if err != nil {
		http.Error(w, "Failed to build request", http.StatusInternalServerError)
		return
	}
	resp, err := httpClient.Do(req)

	if err != nil {
		http.Error(w, "Failed to fetch source image", http.StatusBadGateway)
		log.Printf("Error fetching image from %s: %v\n", srcURL, err)
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		http.Error(w, "Failed to fetch source image", http.StatusBadGateway)
		return
	}

	buf := bufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufPool.Put(buf)
	}()
	const maxAcceptSize = 25 << 20
	limitReq := io.LimitReader(resp.Body, maxAcceptSize+1)
	n, err := io.Copy(buf, limitReq)

	if err != nil {
		http.Error(w, "Error reading image", http.StatusInternalServerError)
		log.Printf("read error %v\n", err)
		return
	}

	if n > maxAcceptSize {
		http.Error(w, "Source image too large", http.StatusRequestEntityTooLarge)
		return
	}

	origImg := buf.Bytes()

	tag := cacheKey
	if tag == "" {
		tag = etag.Generate(origImg, srcURL, options)
	}
	if etag.Matched(r, tag) {
		w.WriteHeader(http.StatusNotModified)
		log.Println("Skipping already matched with ETag")
		return
	}

	newImg, err := safeProcessImage(origImg, options)

	if err != nil {
		http.Error(w, "Failed to process image", http.StatusInternalServerError)
		log.Printf("Error processing %s, mediaType: %v, image: %v\n", srcURL, mediaType, err)
		return
	}

	if cacheKey != "" {
		store.Put(cacheKey, newImg)
	}

	log.Println("Sending Type", bimg.ImageTypeName(mediaType))
	writeImage(w, newImg, mediaType, tag)
}

// Handler returns the image handler. A nil store disables disk caching.
func Handler(store *cache.Store) func(w http.ResponseWriter, r *http.Request) {
	bimg.VipsCacheSetMax(100)
	bimg.VipsCacheSetMaxMem(config.Int("VIPS_CACHE_MAX_MEM_MB", 512)) // MB
	PrintSupportedFormats()
	WarmLibVips()
	return func(w http.ResponseWriter, r *http.Request) {
		handleImage(store, w, r)
	}
}
