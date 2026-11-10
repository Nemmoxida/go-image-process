package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

func TestTransformBatchDistributesEditsAndReturnsZip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}

	body, contentType := transformMultipart(t, source.Bytes(),
		`{"count":3,"ext":"png","edits":[{"action":"resize","resize":{"width":1,"height":1}},{"action":"randomCrop","randomCrop":{"width":{"min":0.5,"max":0.5},"height":{"min":0.5,"max":0.5}}}]}`)
	request := httptest.NewRequest(http.MethodPost, "/images/transform", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	Transform(context)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("expected ZIP response, got %q", response.Header().Get("Content-Type"))
	}

	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 3 {
		t.Fatalf("expected 3 results, got %d", len(archive.File))
	}

	dimensions := map[int]int{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		result, _, err := image.Decode(reader)
		closeErr := reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		dimensions[result.Bounds().Dx()]++
	}
	if dimensions[1] != 2 || dimensions[2] != 1 {
		t.Fatalf("expected round-robin distribution of 2 resize and 1 random crop results, got dimensions %v", dimensions)
	}
}

func TestTransformProcessesMultipleInputImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var firstImage, secondImage bytes.Buffer
	if err := png.Encode(&firstImage, image.NewNRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&secondImage, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}

	body, contentType := transformMultipartImages(t, [][]byte{firstImage.Bytes(), secondImage.Bytes()},
		`{"count":2,"ext":"png","edits":[{"action":"resize","resize":{"width":1,"height":1}}]}`)
	request := httptest.NewRequest(http.MethodPost, "/images/transform", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	Transform(context)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 4 {
		t.Fatalf("expected 4 archive entries, got %d", len(archive.File))
	}
	if archive.File[0].Name != "image_01/result_001.png" ||
		archive.File[1].Name != "image_01/result_002.png" ||
		archive.File[2].Name != "image_02/result_001.png" ||
		archive.File[3].Name != "image_02/result_002.png" {
		t.Fatalf("unexpected archive entry names: %q, %q, %q, %q",
			archive.File[0].Name, archive.File[1].Name, archive.File[2].Name, archive.File[3].Name)
	}
}

func TestTransformRejectsMoreThanFiftyInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	inputs := make([][]byte, maxTransformInputImages+1)
	for i := range inputs {
		inputs[i] = source.Bytes()
	}
	body, contentType := transformMultipartImages(t, inputs,
		`{"count":1,"ext":"png","edits":[{"action":"rotate","range":{"min":0,"max":0}}]}`)
	request := httptest.NewRequest(http.MethodPost, "/images/transform", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	Transform(context)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestTransformRejectsOversizedInputImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body, contentType := transformMultipartImages(t, [][]byte{bytes.Repeat([]byte{0}, maxImageFileSize+1)},
		`{"count":1,"ext":"png","edits":[{"action":"rotate","range":{"min":0,"max":0}}]}`)
	request := httptest.NewRequest(http.MethodPost, "/images/transform", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	Transform(context)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d: %s", response.Code, response.Body.String())
	}
}

func TestTransformRejectsImageDimensionsOver500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, maxImageDimension+1, 1))); err != nil {
		t.Fatal(err)
	}
	body, contentType := transformMultipartImages(t, [][]byte{source.Bytes()},
		`{"count":1,"ext":"png","edits":[{"action":"rotate","range":{"min":0,"max":0}}]}`)
	request := httptest.NewRequest(http.MethodPost, "/images/transform", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	Transform(context)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d: %s", response.Code, response.Body.String())
	}
}

func TestTransformAppliesEnabledVerticalFlip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	source := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	source.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	var encodedSource bytes.Buffer
	if err := png.Encode(&encodedSource, source); err != nil {
		t.Fatal(err)
	}

	body, contentType := transformMultipart(t, encodedSource.Bytes(),
		`{"count":1,"ext":"png","edits":[{"action":"resize","resize":{"width":1,"height":2}}],"flip":{"enabled":true,"probability":1}}`)
	request := httptest.NewRequest(http.MethodPost, "/images/transform", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	Transform(context)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("expected ZIP response, got %q", response.Header().Get("Content-Type"))
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	file, err := archive.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	result, err := png.Decode(file)
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	topRed, _, topBlue, _ := result.At(0, 0).RGBA()
	bottomRed, _, bottomBlue, _ := result.At(0, 1).RGBA()
	if topBlue <= topRed || bottomRed <= bottomBlue {
		t.Fatalf("expected vertical flip to swap rows, got top %#v and bottom %#v", result.At(0, 0), result.At(0, 1))
	}
}

func TestValidateEditRequiresBothRangeBounds(t *testing.T) {
	if err := validateEdit(editReq{Action: "rotate", Range: &valueRange{}}); err == nil {
		t.Fatal("expected an error when the range is missing bounds")
	}
}

func TestValidateFlipRequiresProbabilityWhenEnabled(t *testing.T) {
	if err := validateFlip(&flipAugmentation{Enabled: true}); err == nil {
		t.Fatal("expected an error when enabled flip is missing probability")
	}
	probability := 0.5
	if err := validateFlip(&flipAugmentation{Enabled: true, Probability: &probability}); err != nil {
		t.Fatalf("expected a valid flip probability, got %v", err)
	}
}

func TestApplyAllSupportedEditActions(t *testing.T) {
	minusOne, zero, one := -1.0, 0.0, 1.0
	source := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	randomRange := &valueRange{Min: &one, Max: &one}
	edits := []editReq{
		{Action: "resize", Resize: &resizeType{Width: 2, Height: 2}},
		{Action: "randomCrop", RandomCrop: &randomCropType{
			Width:  &valueRange{Min: &one, Max: &one},
			Height: &valueRange{Min: &one, Max: &one},
		}},
		{Action: "randomScale", Range: randomRange},
		{Action: "rotate", Range: &valueRange{Min: &minusOne, Max: &one}},
		{Action: "hue", Range: &valueRange{Min: &minusOne, Max: &one}},
		{Action: "exposure", Range: &valueRange{Min: &minusOne, Max: &one}},
		{Action: "blur", Range: &valueRange{Min: &zero, Max: &one}},
		{Action: "gaussianNoise", Range: &valueRange{Min: &zero, Max: &one}},
		{Action: "brightness", Range: &valueRange{Min: &minusOne, Max: &one}},
		{Action: "contrast", Range: &valueRange{Min: &minusOne, Max: &one}},
		{Action: "saturation", Range: &valueRange{Min: &minusOne, Max: &one}},
		{Action: "changeFormat", Format: "jpeg"},
	}

	rng := rand.New(rand.NewSource(1))
	for _, edit := range edits {
		if err := validateEdit(edit); err != nil {
			t.Errorf("%s should validate: %v", edit.Action, err)
			continue
		}
		result, _, err := applyEdit(source, edit, imaging.PNG, rng)
		if err != nil {
			t.Errorf("%s should apply: %v", edit.Action, err)
			continue
		}
		if result == nil {
			t.Errorf("%s returned a nil image", edit.Action)
		}
	}
}

func TestLegacyFilterAndFixedCropActionsAreUnsupported(t *testing.T) {
	for _, action := range []string{"filters", "crop"} {
		if err := validateEdit(editReq{Action: action}); err == nil {
			t.Errorf("expected legacy action %q to be unsupported", action)
		}
	}
}

func transformMultipart(t *testing.T, imageBytes []byte, metadata string) (*bytes.Buffer, string) {
	return transformMultipartImages(t, [][]byte{imageBytes}, metadata)
}

func transformMultipartImages(t *testing.T, images [][]byte, metadata string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("metadata", metadata); err != nil {
		t.Fatal(err)
	}
	for i, imageBytes := range images {
		file, err := writer.CreateFormFile("image", fmt.Sprintf("input_%02d.png", i+1))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(file, bytes.NewReader(imageBytes)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}
