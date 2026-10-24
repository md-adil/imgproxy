package image

import (
	"encoding/base64"
	"log"

	"github.com/h2non/bimg"
)

var formats = []bimg.ImageType{
	bimg.JPEG,
	bimg.PNG,
	bimg.WEBP,
	bimg.AVIF,
	bimg.TIFF,
}

func PrintSupportedFormats() {
	for _, format := range formats {
		if supported := bimg.IsImageTypeSupportedByVips(format); supported.Load && supported.Save {
			log.Printf("Supported format: %s\n", bimg.ImageTypeName(format))
		} else {
			log.Printf("Format not supported: %s\n", bimg.ImageTypeName(format))
		}
	}
}

func WarmLibVips() {
	const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR4nGNgYAAAAAMAASsJTYQAAAAASUVORK5CYII="
	img, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		log.Println("Failed to decode warm-up PNG:", err)
		return
	}

	for _, format := range formats {
		opts := bimg.Options{
			Type:    format,
			Quality: 90,
			Width:   1,
			Height:  1,
		}
		_, err := bimg.NewImage(img).Process(opts)
		if err != nil {
			log.Printf("warming format %v failed: %v", format, err)
		} else {
			log.Printf("warmed format: %v", format)
		}
	}
}
