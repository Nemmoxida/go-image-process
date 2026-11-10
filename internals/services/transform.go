package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"

	"simple-image-processing-golang/pkg"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

const (
	maxTransformResultsPerImage = 50
	maxTransformInputImages     = 50
	maxTransformTotalResults    = 2500
	maxImageFileSize            = 400 * 1024
	maxImageDimension           = 500
	maxGeneratedDimension       = maxImageDimension * 4
	maxArchiveSize              = 250 * 1024 * 1024
	maxMultipartOverhead        = 1024 * 1024
	maxMultipartRequestSize     = maxTransformInputImages*maxImageFileSize + maxMultipartOverhead
)

var errArchiveTooLarge = errors.New("generated archive exceeds the 250 MiB limit")

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, errArchiveTooLarge
	}
	return b.buffer.Write(p)
}

func (b *boundedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMultipartRequestSize)
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": "multipart request exceeds the maximum request size"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "invalid multipart request"})
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}

	metadata := c.PostForm("metadata")
	fileHeaders := c.Request.MultipartForm.File["image"]
	if len(fileHeaders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "no image found", "message": "provide at least one image field"})
		return
	}
	if len(fileHeaders) > maxTransformInputImages {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("no more than %d input images are allowed", maxTransformInputImages)})
		return
	}

	var request transformReq
	if err := json.Unmarshal([]byte(metadata), &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "invalid json format", "message": err.Error()})
		return
	}

	if request.Count < 1 || request.Count > maxTransformResultsPerImage {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("count must be between 1 and %d per input image", maxTransformResultsPerImage)})
		return
	}
	totalResults := len(fileHeaders) * request.Count
	if totalResults > maxTransformTotalResults {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("total results cannot exceed %d", maxTransformTotalResults)})
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

	rng := rand.New(rand.NewSource(rand.Int63()))
	output := boundedBuffer{limit: maxArchiveSize}
	archive := zip.NewWriter(&output)
	for imageIndex, fileHeader := range fileHeaders {
		if fileHeader.Size > maxImageFileSize {
			_ = archive.Close()
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": fmt.Sprintf("input image %d exceeds the 400 KiB file size limit", imageIndex+1)})
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			_ = archive.Close()
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("could not open input image %d", imageIndex+1)})
			return
		}
		config, _, err := image.DecodeConfig(file)
		if err == nil {
			_, err = file.Seek(0, io.SeekStart)
		}
		if err != nil {
			_ = file.Close()
			_ = archive.Close()
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("could not read input image %d", imageIndex+1)})
			return
		}
		if config.Width < 1 || config.Height < 1 || config.Width > maxImageDimension || config.Height > maxImageDimension {
			_ = file.Close()
			_ = archive.Close()
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": fmt.Sprintf("input image %d dimensions must not exceed 500x500 pixels", imageIndex+1)})
			return
		}
		img, err := imaging.Decode(file)
		closeErr := file.Close()
		if err != nil {
			_ = archive.Close()
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("could not decode input image %d", imageIndex+1)})
			return
		}
		if closeErr != nil {
			_ = archive.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": fmt.Sprintf("could not close input image %d", imageIndex+1)})
			return
		}

		for resultIndex := 0; resultIndex < request.Count; resultIndex++ {
			edit := request.Edits[resultIndex%len(request.Edits)]
			result, format, err := applyEdit(img, edit, outputFormat, rng)
			if err != nil {
				_ = archive.Close()
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
				return
			}
			bounds := result.Bounds()
			if bounds.Dx() > maxGeneratedDimension || bounds.Dy() > maxGeneratedDimension {
				_ = archive.Close()
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": fmt.Sprintf("transformed image dimensions cannot exceed %dx%d pixels", maxGeneratedDimension, maxGeneratedDimension)})
				return
			}
			if request.Flip != nil && request.Flip.Enabled && rng.Float64() < *request.Flip.Probability {
				result = imaging.FlipV(result)
			}

			name := fmt.Sprintf("image_%02d/result_%03d.%s", imageIndex+1, resultIndex+1, extensionForFormat(format))
			writer, err := archive.Create(name)
			if err == nil {
				err = imaging.Encode(writer, result, format)
			}
			if err != nil {
				_ = archive.Close()
				if errors.Is(err, errArchiveTooLarge) {
					c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": errArchiveTooLarge.Error()})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding image"})
				return
			}
		}
	}
	if err := archive.Close(); err != nil {
		if errors.Is(err, errArchiveTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": errArchiveTooLarge.Error()})
			return
		}
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
		if edit.Resize.Width > maxGeneratedDimension || edit.Resize.Height > maxGeneratedDimension {
			return fmt.Errorf("resize dimensions cannot exceed %dx%d pixels", maxGeneratedDimension, maxGeneratedDimension)
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
