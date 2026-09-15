package pkg

import (
	"image"

	"github.com/disintegration/imaging"
)

func Resize(img image.Image, width int, height int, ext string) *image.NRGBA {
	resized := imaging.Resize(img, width, height, imaging.Lanczos)

	// var resizedBuffer bytes.Buffer
	// var imageFormat imaging.Format

	// if err := imaging.Encode(&resizedBuffer, resized, imageFormat); err != nil {
	// 	return resizedBuffer, err
	// }

	return resized
}
