package pkg

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
)

func TestAdjustHueRotatesRedToGreen(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})

	result := AdjustHue(source, 120).NRGBAAt(0, 0)
	if result.G < 250 || result.R > 5 || result.B > 5 {
		t.Fatalf("expected red to become green, got %#v", result)
	}
}

func TestAdjustExposurePreservesAlpha(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 100, G: 50, B: 25, A: 128})

	result := AdjustExposure(source, 1).NRGBAAt(0, 0)
	if result.R != 200 || result.G != 100 || result.B != 50 || result.A != 128 {
		t.Fatalf("unexpected exposure result: %#v", result)
	}
}

func TestGaussianNoisePreservesAlphaAndBounds(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 128, G: 128, B: 128, A: 128})

	result := GaussianNoise(source, 20).NRGBAAt(0, 0)
	if result.A != 128 {
		t.Fatalf("expected alpha to be preserved, got %d", result.A)
	}
}

func TestRandomCropUsesRequestedFractions(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	result, err := RandomCrop(source, 0.5, 0.5, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Bounds().Dx() != 4 || result.Bounds().Dy() != 3 {
		t.Fatalf("expected 4x3 crop, got %dx%d", result.Bounds().Dx(), result.Bounds().Dy())
	}
}
