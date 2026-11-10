package pkg

import (
	"errors"
	"image"
	"image/color"
	"math"
	"math/rand"

	"github.com/disintegration/imaging"
)

func Resize(img image.Image, width int, height int) (*image.NRGBA, error) {

	if width <= 0 && height <= 0 {
		return nil, errors.New("Width or Height cannot bet 0 or less")
	}

	resized := imaging.Resize(img, width, height, imaging.Lanczos)

	return resized, nil
}

func RandomCrop(img image.Image, widthFraction, heightFraction float64, rng *rand.Rand) (*image.NRGBA, error) {
	if widthFraction <= 0 || widthFraction > 1 || heightFraction <= 0 || heightFraction > 1 {
		return nil, errors.New("crop width and height fractions must be greater than 0 and at most 1")
	}
	if rng == nil {
		return nil, errors.New("random source cannot be nil")
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, errors.New("image dimensions must be greater than 0")
	}
	width := int(math.Round(float64(bounds.Dx()) * widthFraction))
	height := int(math.Round(float64(bounds.Dy()) * heightFraction))
	width = max(1, min(width, bounds.Dx()))
	height = max(1, min(height, bounds.Dy()))
	x := bounds.Min.X + rng.Intn(bounds.Dx()-width+1)
	y := bounds.Min.Y + rng.Intn(bounds.Dy()-height+1)

	return imaging.Crop(img, image.Rect(x, y, x+width, y+height)), nil
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

func AdjustHue(img image.Image, degrees float64) *image.NRGBA {
	shift := degrees / 360
	return imaging.AdjustFunc(img, func(c color.NRGBA) color.NRGBA {
		r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
		max := math.Max(r, math.Max(g, b))
		min := math.Min(r, math.Min(g, b))
		delta := max - min

		hue := 0.0
		saturation := 0.0
		if max > 0 {
			saturation = delta / max
		}
		if delta > 0 {
			switch max {
			case r:
				hue = math.Mod((g-b)/delta, 6)
			case g:
				hue = (b-r)/delta + 2
			default:
				hue = (r-g)/delta + 4
			}
			hue /= 6
			if hue < 0 {
				hue++
			}
		}

		hue = math.Mod(hue+shift, 1)
		if hue < 0 {
			hue++
		}

		h := hue * 6
		chroma := max * saturation
		x := chroma * (1 - math.Abs(math.Mod(h, 2)-1))
		var rr, gg, bb float64
		switch {
		case h < 1:
			rr, gg = chroma, x
		case h < 2:
			rr, gg = x, chroma
		case h < 3:
			gg, bb = chroma, x
		case h < 4:
			gg, bb = x, chroma
		case h < 5:
			rr, bb = x, chroma
		default:
			rr, bb = chroma, x
		}
		m := max - chroma
		return color.NRGBA{
			R: uint8(math.Round((rr + m) * 255)),
			G: uint8(math.Round((gg + m) * 255)),
			B: uint8(math.Round((bb + m) * 255)),
			A: c.A,
		}
	})
}

func AdjustExposure(img image.Image, stops float64) *image.NRGBA {
	factor := math.Pow(2, stops)
	return imaging.AdjustFunc(img, func(c color.NRGBA) color.NRGBA {
		return color.NRGBA{
			R: clampColor(float64(c.R) * factor),
			G: clampColor(float64(c.G) * factor),
			B: clampColor(float64(c.B) * factor),
			A: c.A,
		}
	})
}

func GaussianNoise(img image.Image, sigma float64) *image.NRGBA {
	if sigma <= 0 {
		return imaging.Clone(img)
	}

	return imaging.AdjustFunc(img, func(c color.NRGBA) color.NRGBA {
		return color.NRGBA{
			R: clampColor(float64(c.R) + rand.NormFloat64()*sigma),
			G: clampColor(float64(c.G) + rand.NormFloat64()*sigma),
			B: clampColor(float64(c.B) + rand.NormFloat64()*sigma),
			A: c.A,
		}
	})
}

func clampColor(value float64) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(math.Round(value))
}
