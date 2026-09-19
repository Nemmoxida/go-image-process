package pkg

import (
	"errors"
	"image"
	"image/color"
	"math"

	"github.com/disintegration/imaging"
)

func Resize(img image.Image, width int, height int) (*image.NRGBA, error) {

	if width <= 0 && height <= 0 {
		return nil, errors.New("Width or Height cannot bet 0 or less")
	}

	resized := imaging.Resize(img, width, height, imaging.Lanczos)

	return resized, nil
}

func CropAt(img image.Image, x, y, width, height int) (*image.NRGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("width and height must be greater than 0")
	}

	bounds := img.Bounds()

	if width > bounds.Dx() || height > bounds.Dy() {
		return nil, errors.New("width size and height size cannot be larger than the image")
	}

	// Keep the crop rectangle inside the image.
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	if x+width > bounds.Max.X {
		x = bounds.Max.X - width
	}
	if y+height > bounds.Max.Y {
		y = bounds.Max.Y - height
	}

	rect := image.Rect(x, y, x+width, y+height)

	return imaging.Crop(img, rect), nil
}

func Rotate(img image.Image, b float64) *image.NRGBA {

	var angle float64

	if b > 360.0 {
		angle = math.Mod(b, 360.0)
	} else {
		angle = b
	}

	return imaging.Rotate(img, angle, color.White)
}

func Filters(img image.Image, inverted, grayscale, flip bool, blur int) (*image.NRGBA, error) {
	if blur < 0 {
		return nil, errors.New("blur cannot be less than 0")
	}

	filtered := img

	if inverted {
		filtered = imaging.Invert(filtered)
	}
	if grayscale {
		filtered = imaging.Grayscale(filtered)
	}
	if flip {
		filtered = imaging.FlipH(filtered)
	}
	if blur > 0 {
		filtered = imaging.Blur(filtered, float64(blur))
	}

	return imaging.Clone(filtered), nil
}
