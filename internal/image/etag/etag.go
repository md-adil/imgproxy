package etag

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/h2non/bimg"
)

func hash(args ...any) (string, error) {
	hasher := sha256.New()
	for _, val := range args {
		var s string
		switch v := val.(type) {
		case []byte:
			if _, err := hasher.Write(v); err != nil {
				return "", err
			}
			continue
		case int:
			s = strconv.Itoa(v)
		case bool:
			s = strconv.FormatBool(v)
		case string:
			s = v
		default:
			return "", fmt.Errorf("unsupported type %v", v)
		}
		if _, err := io.WriteString(hasher, s); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func Generate(image []byte, url string, opt bimg.Options) string {
	val, err := hash(image, url, opt.Height, opt.Width, opt.Quality, int(opt.Type), opt.Force, opt.Enlarge, opt.Compression)
	if err != nil {
		log.Printf("etag generation error: %v", err)
		return ""
	}
	return val
}

func Matched(r *http.Request, etag string) bool {
	if etag == "" {
		return false
	}
	for _, v := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		v = strings.TrimSpace(v)
		if v == "*" || strings.Trim(strings.TrimPrefix(v, "W/"), `"`) == etag {
			return true
		}
	}
	return false
}
