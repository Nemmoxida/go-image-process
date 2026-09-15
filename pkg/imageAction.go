package pkg

import (
	"errors"
	"image"

	"github.com/disintegration/imaging"
)

func Resize(img image.Image, width int, height int, ext string) (*image.NRGBA, error) {

	if width <= 0 && height <= 0 {
		return nil, errors.New("Width or Height cannot bet 0 or less")
	}

	resized := imaging.Resize(img, width, height, imaging.Lanczos)

	// var resizedBuffer bytes.Buffer
	// var imageFormat imaging.Format

	// if err := imaging.Encode(&resizedBuffer, resized, imageFormat); err != nil {
	// 	return resizedBuffer, err
	// }

	return resized, nil
}
