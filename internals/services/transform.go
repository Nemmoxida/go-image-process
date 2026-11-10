package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"math/rand"
	"net/http"
	"strings"

	"simple-image-processing-golang/pkg"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

const maxTransformResults = 100

type resizeType struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type valueRange struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

type randomCropType struct {
	Width  *valueRange `json:"width"`
	Height *valueRange `json:"height"`
}

type editReq struct {
	Action     string          `json:"action"`
	Range      *valueRange     `json:"range"`
	Resize     *resizeType     `json:"resize"`
	RandomCrop *randomCropType `json:"randomCrop"`
	Format     string          `json:"format"`
}

type flipAugmentation struct {
	Enabled     bool     `json:"enabled"`
	Probability *float64 `json:"probability"`
}

type transformReq struct {
	Count int               `json:"count"`
	Ext   string            `json:"ext"`
	Edits []editReq         `json:"edits"`
	Flip  *flipAugmentation `json:"flip"`
}

type newImageReq struct {
	Action   string          `json:"action"`
	Metadata json.RawMessage `json:"metadata"`
	UserId   string          `json:"userid"`
}

func Transform(c *gin.Context) {
	metadata := c.PostForm("metadata")
	fileHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "no image found", "message": err.Error()})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "could not open image", "message": err.Error()})
		return
	}
	defer file.Close()

	var request transformReq
	if err := json.Unmarshal([]byte(metadata), &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "invalid json format", "message": err.Error()})
		return
	}

	if request.Count < 1 || request.Count > maxTransformResults {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("count must be between 1 and %d", maxTransformResults)})
		return
	}
	if strings.TrimSpace(request.Ext) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "ext is required"})
		return
	}
	if len(request.Edits) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "at least one edit is required"})
		return
	}
	if request.Count < len(request.Edits) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "count must be at least the number of selected edits"})
		return
	}
	outputFormat, err := formatForExtension(request.Ext)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	for _, edit := range request.Edits {
		if err := validateEdit(edit); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
			return
		}
	}
	if err := validateFlip(request.Flip); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	img, err := imaging.Decode(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "could not decode image", "message": err.Error()})
		return
	}

	rng := rand.New(rand.NewSource(rand.Int63()))
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for i := 0; i < request.Count; i++ {
		edit := request.Edits[i%len(request.Edits)]
		result, format, err := applyEdit(img, edit, outputFormat, rng)
		if err != nil {
			_ = archive.Close()
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
			return
		}
		if request.Flip != nil && request.Flip.Enabled && rng.Float64() < *request.Flip.Probability {
			result = imaging.FlipV(result)
		}

		name := fmt.Sprintf("image_%03d.%s", i+1, extensionForFormat(format))
		writer, err := archive.Create(name)
		if err == nil {
			err = imaging.Encode(writer, result, format)
		}
		if err != nil {
			_ = archive.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding image"})
			return
		}
	}
	if err := archive.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error creating image archive"})
		return
	}

	c.Header("Content-Disposition", `attachment; filename="results.zip"`)
	c.Data(http.StatusOK, "application/zip", output.Bytes())
}

func validateEdit(edit editReq) error {
	switch edit.Action {
	case "resize":
		if edit.Resize == nil || edit.Resize.Width < 0 || edit.Resize.Height < 0 ||
			(edit.Resize.Width == 0 && edit.Resize.Height == 0) {
			return fmt.Errorf("resize requires non-negative width and height with at least one positive dimension")
		}
	case "randomCrop":
		if edit.RandomCrop == nil {
			return fmt.Errorf("randomCrop requires width and height fraction ranges")
		}
		if err := validateRange("randomCrop.width", edit.RandomCrop.Width, 0.01, 1); err != nil {
			return err
		}
		if err := validateRange("randomCrop.height", edit.RandomCrop.Height, 0.01, 1); err != nil {
			return err
		}
	case "rotate", "hue", "exposure", "blur", "gaussianNoise", "brightness", "contrast", "saturation", "randomScale":
		if err := validateRange(edit.Action, edit.Range, math.Inf(-1), math.Inf(1)); err != nil {
			return err
		}
		switch edit.Action {
		case "rotate":
			return validateRange(edit.Action, edit.Range, -360, 360)
		case "hue":
			return validateRange(edit.Action, edit.Range, -360, 360)
		case "exposure":
			return validateRange(edit.Action, edit.Range, -10, 10)
		case "blur":
			return validateRange(edit.Action, edit.Range, 0, 100)
		case "gaussianNoise":
			return validateRange(edit.Action, edit.Range, 0, 255)
		case "brightness", "contrast", "saturation":
			return validateRange(edit.Action, edit.Range, -100, 100)
		case "randomScale":
			return validateRange(edit.Action, edit.Range, 0.1, 4)
		}
	case "changeFormat":
		if _, err := formatForExtension(edit.Format); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported action %q", edit.Action)
	}
	return nil
}

func validateRange(name string, value *valueRange, minimum, maximum float64) error {
	if value == nil || value.Min == nil || value.Max == nil ||
		math.IsNaN(*value.Min) || math.IsNaN(*value.Max) ||
		math.IsInf(*value.Min, 0) || math.IsInf(*value.Max, 0) ||
		*value.Min > *value.Max {
		return fmt.Errorf("%s requires a valid range with min less than or equal to max", name)
	}
	if *value.Min < minimum || *value.Max > maximum {
		return fmt.Errorf("%s range must be between %g and %g", name, minimum, maximum)
	}
	return nil
}

func validateFlip(flip *flipAugmentation) error {
	if flip == nil || !flip.Enabled {
		return nil
	}
	if flip.Probability == nil || math.IsNaN(*flip.Probability) ||
		math.IsInf(*flip.Probability, 0) || *flip.Probability < 0 || *flip.Probability > 1 {
		return fmt.Errorf("flip probability must be between 0 and 1 when flip is enabled")
	}
	return nil
}

func applyEdit(img image.Image, edit editReq, defaultFormat imaging.Format, rng *rand.Rand) (image.Image, imaging.Format, error) {
	resultFormat := defaultFormat
	var result image.Image

	switch edit.Action {
	case "resize":
		resized, err := pkg.Resize(img, edit.Resize.Width, edit.Resize.Height)
		if err != nil {
			return nil, resultFormat, err
		}
		result = resized
	case "randomCrop":
		widthFraction := sampleRange(edit.RandomCrop.Width, rng)
		heightFraction := sampleRange(edit.RandomCrop.Height, rng)
		cropped, err := pkg.RandomCrop(img, widthFraction, heightFraction, rng)
		if err != nil {
			return nil, resultFormat, err
		}
		result = cropped
	case "rotate":
		result = pkg.Rotate(img, sampleRange(edit.Range, rng))
	case "hue":
		result = pkg.AdjustHue(img, sampleRange(edit.Range, rng))
	case "exposure":
		result = pkg.AdjustExposure(img, sampleRange(edit.Range, rng))
	case "blur":
		sigma := sampleRange(edit.Range, rng)
		if sigma == 0 {
			result = imaging.Clone(img)
		} else {
			result = imaging.Blur(img, sigma)
		}
	case "gaussianNoise":
		result = pkg.GaussianNoise(img, sampleRange(edit.Range, rng))
	case "brightness":
		result = imaging.AdjustBrightness(img, sampleRange(edit.Range, rng))
	case "contrast":
		result = imaging.AdjustContrast(img, sampleRange(edit.Range, rng))
	case "saturation":
		result = imaging.AdjustSaturation(img, sampleRange(edit.Range, rng))
	case "randomScale":
		scale := sampleRange(edit.Range, rng)
		bounds := img.Bounds()
		width := max(1, int(math.Round(float64(bounds.Dx())*scale)))
		height := max(1, int(math.Round(float64(bounds.Dy())*scale)))
		result = imaging.Resize(img, width, height, imaging.Lanczos)
	case "changeFormat":
		format, err := formatForExtension(edit.Format)
		if err != nil {
			return nil, resultFormat, err
		}
		resultFormat = format
		result = img
	}

	return result, resultFormat, nil
}

func sampleRange(value *valueRange, rng *rand.Rand) float64 {
	if *value.Min == *value.Max {
		return *value.Min
	}
	return *value.Min + rng.Float64()*(*value.Max-*value.Min)
}

func formatForExtension(ext string) (imaging.Format, error) {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "png":
		return imaging.PNG, nil
	case "jpeg", "jpg":
		return imaging.JPEG, nil
	default:
		return 0, fmt.Errorf("unsupported image format %q; use png or jpeg", ext)
	}
}

func extensionForFormat(format imaging.Format) string {
	if format == imaging.PNG {
		return "png"
	}
	return "jpg"
}
