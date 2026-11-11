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
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
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
	maxAnnotationFileSize       = 400 * 1024
	maxImageDimension           = 500
	maxGeneratedDimension       = maxImageDimension * 4
	maxArchiveSize              = 250 * 1024 * 1024
	maxMultipartOverhead        = 1024 * 1024
	maxMultipartRequestSize     = maxTransformInputImages*(maxImageFileSize+maxAnnotationFileSize) + maxMultipartOverhead
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

type yoloBox struct {
	Class  int
	X      float64
	Y      float64
	Width  float64
	Height float64
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
	annotationHeaders := c.Request.MultipartForm.File["annotation"]
	if len(annotationHeaders) > 0 && len(annotationHeaders) != len(fileHeaders) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "provide exactly one matching annotation for each image"})
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

	annotations := make([][]yoloBox, len(fileHeaders))
	if len(annotationHeaders) > 0 {
		for _, header := range annotationHeaders {
			if header.Size > maxAnnotationFileSize {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": "annotation exceeds the 400 KiB file size limit"})
				return
			}
		}
		var err error
		annotations, err = readMatchingAnnotations(fileHeaders, annotationHeaders)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
			return
		}
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
			var result image.Image
			var format imaging.Format
			var resultAnnotations []yoloBox
			if len(annotationHeaders) > 0 {
				result, format, resultAnnotations, err = applyEditWithAnnotations(img, edit, outputFormat, rng, annotations[imageIndex])
			} else {
				result, format, err = applyEdit(img, edit, outputFormat, rng)
			}
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
				resultAnnotations = flipYoloBoxesVertically(resultAnnotations)
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
			if len(annotationHeaders) > 0 {
				annotationWriter, err := archive.Create(fmt.Sprintf("image_%02d/result_%03d.txt", imageIndex+1, resultIndex+1))
				if err != nil {
					_ = archive.Close()
					if errors.Is(err, errArchiveTooLarge) {
						c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": errArchiveTooLarge.Error()})
						return
					}
					c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error creating annotation archive entry"})
					return
				}
				if err := writeYOLOAnnotations(annotationWriter, resultAnnotations); err != nil {
					_ = archive.Close()
					if errors.Is(err, errArchiveTooLarge) {
						c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "message": errArchiveTooLarge.Error()})
						return
					}
					c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error writing annotation"})
					return
				}
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

func readMatchingAnnotations(imageHeaders, annotationHeaders []*multipart.FileHeader) ([][]yoloBox, error) {
	imagesByName := make(map[string]int, len(imageHeaders))
	for index, header := range imageHeaders {
		name := filepath.Base(header.Filename)
		extension := strings.ToLower(filepath.Ext(name))
		if extension != ".jpg" && extension != ".jpeg" && extension != ".png" {
			return nil, fmt.Errorf("image %q must use a .jpg, .jpeg, or .png extension when annotations are provided", name)
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if stem == "" {
			return nil, fmt.Errorf("image %q must have a filename", name)
		}
		if _, exists := imagesByName[stem]; exists {
			return nil, fmt.Errorf("image filename %q is duplicated", name)
		}
		imagesByName[stem] = index
	}

	annotations := make([][]yoloBox, len(imageHeaders))
	seen := make(map[int]struct{}, len(annotationHeaders))
	for _, header := range annotationHeaders {
		name := filepath.Base(header.Filename)
		if strings.ToLower(filepath.Ext(name)) != ".txt" {
			return nil, fmt.Errorf("annotation %q must be a .txt file", name)
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		imageIndex, exists := imagesByName[stem]
		if !exists {
			return nil, fmt.Errorf("annotation %q does not match an uploaded image filename", name)
		}
		if _, exists := seen[imageIndex]; exists {
			return nil, fmt.Errorf("more than one annotation matches image %q", name)
		}
		seen[imageIndex] = struct{}{}
		boxes, err := readYOLOAnnotation(header)
		if err != nil {
			return nil, fmt.Errorf("invalid annotation %q: %w", name, err)
		}
		annotations[imageIndex] = boxes
	}
	if len(seen) != len(imageHeaders) {
		return nil, fmt.Errorf("each uploaded image must have one annotation with the same filename")
	}
	return annotations, nil
}

func readYOLOAnnotation(header *multipart.FileHeader) ([]yoloBox, error) {
	if header.Size > maxAnnotationFileSize {
		return nil, fmt.Errorf("annotation exceeds the 400 KiB file size limit")
	}
	file, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("could not open annotation: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxAnnotationFileSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("could not read annotation: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("could not close annotation: %w", closeErr)
	}
	if len(data) > maxAnnotationFileSize {
		return nil, fmt.Errorf("annotation exceeds the 400 KiB file size limit")
	}

	boxes := make([]yoloBox, 0)
	for lineNumber, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 5 {
			return nil, fmt.Errorf("line %d must contain a class id and four box values", lineNumber+1)
		}
		classID, err := strconv.Atoi(fields[0])
		if err != nil || classID < 0 {
			return nil, fmt.Errorf("line %d has an invalid class id", lineNumber+1)
		}
		values := [4]float64{}
		for i, field := range fields[1:] {
			value, err := strconv.ParseFloat(field, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				return nil, fmt.Errorf("line %d has an invalid normalized box value", lineNumber+1)
			}
			values[i] = value
		}
		if values[2] == 0 || values[3] == 0 ||
			values[0]-values[2]/2 < 0 || values[0]+values[2]/2 > 1 ||
			values[1]-values[3]/2 < 0 || values[1]+values[3]/2 > 1 {
			return nil, fmt.Errorf("line %d describes a box outside the image", lineNumber+1)
		}
		boxes = append(boxes, yoloBox{Class: classID, X: values[0], Y: values[1], Width: values[2], Height: values[3]})
	}
	return boxes, nil
}

func applyEditWithAnnotations(img image.Image, edit editReq, defaultFormat imaging.Format, rng *rand.Rand, boxes []yoloBox) (image.Image, imaging.Format, []yoloBox, error) {
	switch edit.Action {
	case "randomCrop":
		bounds := img.Bounds()
		widthFraction := sampleRange(edit.RandomCrop.Width, rng)
		heightFraction := sampleRange(edit.RandomCrop.Height, rng)
		result, crop, err := pkg.RandomCropWithBounds(img, widthFraction, heightFraction, rng)
		if err != nil {
			return nil, defaultFormat, nil, err
		}
		return result, defaultFormat, cropYOLOBoxes(boxes, bounds.Dx(), bounds.Dy(),
			crop.Min.X-bounds.Min.X, crop.Min.Y-bounds.Min.Y, crop.Dx(), crop.Dy()), nil
	case "rotate":
		angle := sampleRange(edit.Range, rng)
		result := pkg.Rotate(img, angle)
		bounds := img.Bounds()
		resultBounds := result.Bounds()
		return result, defaultFormat, rotateYOLOBoxes(boxes, bounds.Dx(), bounds.Dy(), resultBounds.Dx(), resultBounds.Dy(), angle), nil
	default:
		result, format, err := applyEdit(img, edit, defaultFormat, rng)
		return result, format, boxes, err
	}
}

func cropYOLOBoxes(boxes []yoloBox, sourceWidth, sourceHeight, cropX, cropY, cropWidth, cropHeight int) []yoloBox {
	transformed := make([]yoloBox, 0, len(boxes))
	for _, box := range boxes {
		left := box.X*float64(sourceWidth) - box.Width*float64(sourceWidth)/2 - float64(cropX)
		right := box.X*float64(sourceWidth) + box.Width*float64(sourceWidth)/2 - float64(cropX)
		top := box.Y*float64(sourceHeight) - box.Height*float64(sourceHeight)/2 - float64(cropY)
		bottom := box.Y*float64(sourceHeight) + box.Height*float64(sourceHeight)/2 - float64(cropY)
		left, right = max(0, left), min(float64(cropWidth), right)
		top, bottom = max(0, top), min(float64(cropHeight), bottom)
		if right <= left || bottom <= top {
			continue
		}
		transformed = append(transformed, yoloBox{
			Class:  box.Class,
			X:      (left + right) / (2 * float64(cropWidth)),
			Y:      (top + bottom) / (2 * float64(cropHeight)),
			Width:  (right - left) / float64(cropWidth),
			Height: (bottom - top) / float64(cropHeight),
		})
	}
	return transformed
}

func rotateYOLOBoxes(boxes []yoloBox, sourceWidth, sourceHeight, outputWidth, outputHeight int, angle float64) []yoloBox {
	radians := math.Pi * math.Mod(angle, 360) / 180
	sin, cos := math.Sincos(radians)
	transformed := make([]yoloBox, 0, len(boxes))
	for _, box := range boxes {
		centerX := box.X * float64(sourceWidth)
		centerY := box.Y * float64(sourceHeight)
		halfWidth := box.Width * float64(sourceWidth) / 2
		halfHeight := box.Height * float64(sourceHeight) / 2
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for _, corner := range [][2]float64{
			{centerX - halfWidth, centerY - halfHeight},
			{centerX + halfWidth, centerY - halfHeight},
			{centerX - halfWidth, centerY + halfHeight},
			{centerX + halfWidth, centerY + halfHeight},
		} {
			x := corner[0] - float64(sourceWidth)/2
			y := corner[1] - float64(sourceHeight)/2
			rotatedX := x*cos + y*sin + float64(outputWidth)/2
			rotatedY := -x*sin + y*cos + float64(outputHeight)/2
			minX, maxX = min(minX, rotatedX), max(maxX, rotatedX)
			minY, maxY = min(minY, rotatedY), max(maxY, rotatedY)
		}
		minX, maxX = max(0, minX), min(float64(outputWidth), maxX)
		minY, maxY = max(0, minY), min(float64(outputHeight), maxY)
		if maxX <= minX || maxY <= minY {
			continue
		}
		transformed = append(transformed, yoloBox{
			Class:  box.Class,
			X:      (minX + maxX) / (2 * float64(outputWidth)),
			Y:      (minY + maxY) / (2 * float64(outputHeight)),
			Width:  (maxX - minX) / float64(outputWidth),
			Height: (maxY - minY) / float64(outputHeight),
		})
	}
	return transformed
}

func flipYoloBoxesVertically(boxes []yoloBox) []yoloBox {
	transformed := make([]yoloBox, len(boxes))
	for i, box := range boxes {
		box.Y = 1 - box.Y
		transformed[i] = box
	}
	return transformed
}

func writeYOLOAnnotations(writer io.Writer, boxes []yoloBox) error {
	for _, box := range boxes {
		if _, err := fmt.Fprintf(writer, "%d %s %s %s %s\n", box.Class,
			strconv.FormatFloat(box.X, 'f', -1, 64),
			strconv.FormatFloat(box.Y, 'f', -1, 64),
			strconv.FormatFloat(box.Width, 'f', -1, 64),
			strconv.FormatFloat(box.Height, 'f', -1, 64)); err != nil {
			return err
		}
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
