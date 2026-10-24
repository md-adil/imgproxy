package image

import (
	"strconv"
	"strings"
	"sync"

	"github.com/h2non/bimg"
)

type acceptItem struct {
	mimeType string
	q        float64
}

func parseAccept(accept string) []acceptItem {
	parts := strings.Split(accept, ",")
	items := make([]acceptItem, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments := strings.Split(part, ";")
		mime := strings.TrimSpace(segments[0])
		q := 1.0
		for _, seg := range segments[1:] {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "q=") {
				if v, err := strconv.ParseFloat(seg[2:], 64); err == nil {
					q = v
				}
			}
		}
		items = append(items, acceptItem{mimeType: mime, q: q})
	}
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].q > items[j-1].q; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	return items
}

var savable = sync.OnceValue(func() map[bimg.ImageType]bool {
	m := make(map[bimg.ImageType]bool)
	for _, t := range []bimg.ImageType{bimg.AVIF, bimg.WEBP} {
		m[t] = bimg.IsImageTypeSupportedByVips(t).Save
	}
	return m
})

func GetMediaType(accept string) bimg.ImageType {
	for _, item := range parseAccept(accept) {
		if item.q <= 0 {
			continue
		}
		var t bimg.ImageType
		switch item.mimeType {
		case "image/avif":
			t = bimg.AVIF
		case "image/webp":
			t = bimg.WEBP
		default:
			continue
		}
		if savable()[t] {
			return t
		}
	}
	return bimg.JPEG
}
