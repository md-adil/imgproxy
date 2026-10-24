package image

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/h2non/bimg"

	"imgproxy/internal/cache"
	"imgproxy/internal/config"
	"imgproxy/internal/image/etag"
)

var (
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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Hostname() != via[0].URL.Hostname() {
				return fmt.Errorf("redirect to another host %q blocked", req.URL.Host)
			}
			return nil
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

func setCacheHeaders(header http.Header, tag string) {
	header.Set("Cache-Control", "public, max-age=2592000")
	if tag != "" {
		header.Set("ETag", `"`+tag+`"`)
	}
	header.Set("Vary", "Accept")
}

func writeNotModified(w http.ResponseWriter, tag string) {
	setCacheHeaders(w.Header(), tag)
	w.WriteHeader(http.StatusNotModified)
}

func writeImage(w http.ResponseWriter, r *http.Request, img []byte, mediaType bimg.ImageType, tag string) {
	if etag.Matched(r, tag) {
		writeNotModified(w, tag)
		return
	}
	header := w.Header()
	header.Set("Content-Type", fmt.Sprintf("image/%s", bimg.ImageTypeName(mediaType)))
	setCacheHeaders(header, tag)
	header.Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(img); err != nil {
		log.Printf("write error: %v\n", err)
	}
}

func intParam(query url.Values, name string, maxValue int) (int, bool) {
	s := query.Get(name)
	if s == "" {
		return 0, true
	}
	v, err := strconv.Atoi(s)
	return v, err == nil && v >= 0 && v <= maxValue
}

func handleImage(store *cache.Store, defaultSSL bool, w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	srcDomain := query.Get("d")

	const maxDim = 8000
	const maxQuality = 100
	width, ok := intParam(query, "w", maxDim)
	if !ok {
		http.Error(w, "Invalid width", http.StatusBadRequest)
		return
	}
	height, ok := intParam(query, "h", maxDim)
	if !ok {
		http.Error(w, "Invalid height", http.StatusBadRequest)
		return
	}
	quality, ok := intParam(query, "q", maxQuality)
	if !ok {
		http.Error(w, "Invalid quality", http.StatusBadRequest)
		return
	}

	ssl := defaultSSL
	switch query.Get("ssl") {
	case "":
	case "false":
		ssl = false
	case "true":
		ssl = true
	default:
		http.Error(w, "Invalid ssl", http.StatusBadRequest)
		return
	}
	scheme := "http"
	if ssl {
		scheme = "https"
	}

	srcURL := fmt.Sprintf("%s://%s/%s", scheme, srcDomain, r.URL.EscapedPath())

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

	var cacheKey string
	if store != nil {
		cacheKey = etag.Generate(nil, srcURL, options)
		if img, ok := store.Get(cacheKey); ok {
			writeImage(w, r, img, mediaType, etag.Generate(img, srcURL, options))
			return
		}
	}

	log.Println("Processing Image:", srcURL)

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

	var tag string
	if store == nil {
		tag = etag.Generate(origImg, srcURL, options)
		if etag.Matched(r, tag) {
			writeNotModified(w, tag)
			log.Println("Skipping already matched with ETag")
			return
		}
	}

	newImg, err := safeProcessImage(origImg, options)

	if err != nil {
		http.Error(w, "Failed to process image", http.StatusInternalServerError)
		log.Printf("Error processing %s, mediaType: %v, image: %v\n", srcURL, mediaType, err)
		return
	}

	if store != nil {
		store.Put(cacheKey, newImg)
		tag = etag.Generate(newImg, srcURL, options)
	}

	log.Println("Sending Type", bimg.ImageTypeName(mediaType))
	writeImage(w, r, newImg, mediaType, tag)
}

func Handler(store *cache.Store) func(w http.ResponseWriter, r *http.Request) {
	bimg.VipsCacheSetMax(100)
	bimg.VipsCacheSetMaxMem(config.Int("VIPS_CACHE_MAX_MEM_MB", 512))
	PrintSupportedFormats()
	WarmLibVips()
	defaultSSL := config.Bool("DEFAULT_SSL", false)
	log.Printf("DEFAULT_SSL=%t", defaultSSL)
	return func(w http.ResponseWriter, r *http.Request) {
		handleImage(store, defaultSSL, w, r)
	}
}
