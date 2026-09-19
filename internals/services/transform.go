package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"office-expense-management-backend/pkg"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

type cropType struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	X      int `json:"x"`
	Y      int `json:"y"`
}

type filtersType struct {
	Inverted  *bool `json:"inverted"`
	Grayscale *bool `json:"grayscale"`
	Flip      *bool `json:"flip"`
	Blur      *int  `json:"blur"`
}

type resizeType struct {
	Width  int `json:"width" binding:"required"`
	Height int `json:"height" binding:"required"`
}

type imageReq struct {
	// URL     string       `json:"url" binding:"required,url"`
	Ext     string       `json:"ext" binding:"required"`
	Resize  *resizeType  `json:"resize" binding:"required"`
	Crop    *cropType    `json:"crop"`
	Rotate  *float64     `json:"rotate"`
	Format  string       `json:"format"`
	Filters *filtersType `json:"filters"`
}

type newImageReq struct {
	Action   string   `json:"action" binding:"required"`
	Metadata imageReq `json:"metadata" binding:"required"`
}

// add cache for image that hasn't been finalize (the edit)
// user can do more than one edit (resize, cop, etc) but one at a time
// add multiform respond (probably)

func Transform(c *gin.Context) {

	metadata := c.PostForm("metadata")
	fileHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "no image found", "message": err})
	}
	file, _ := fileHeader.Open()
	defer file.Close()

	var payload newImageReq

	if err := json.Unmarshal([]byte(metadata), &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "invalid json format", "message": err})
	}

	var afterImage bytes.Buffer
	var imageFormat imaging.Format

	img, err := imaging.Decode(file)

	switch payload.Metadata.Ext {
	case "png":
		imageFormat = imaging.PNG
	case "jpeg", "jpg":
		imageFormat = imaging.JPEG
	}

	switch payload.Action {
	case "resize":

		if payload.Metadata.Resize == nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "no action data detected"})
			return
		}

		resized, err := pkg.Resize(img, payload.Metadata.Resize.Width, payload.Metadata.Resize.Height)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err})
			return
		}

		if err := imaging.Encode(&afterImage, resized, imageFormat); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding resized image"})
			return
		}

	case "crop":

		if payload.Metadata.Crop == nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "no action data detected"})
			return
		}

		crop, err := pkg.CropAt(img, payload.Metadata.Crop.X, payload.Metadata.Crop.Y, payload.Metadata.Crop.Width, payload.Metadata.Crop.Height)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err})
			return
		}

		if err := imaging.Encode(&afterImage, crop, imageFormat); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding resized image"})
			return
		}

	case "rotate":
		if payload.Metadata.Rotate == nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "no action data detected"})
			return
		}

		rotate := pkg.Rotate(img, *payload.Metadata.Rotate)

		if err := imaging.Encode(&afterImage, rotate, imageFormat); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding resized image"})
			return
		}

	case "changeFormat":

		var imageFormatTo imaging.Format

		switch payload.Metadata.Format {
		case "png":
			imageFormatTo = imaging.PNG
		case "jpeg", "jpg":
			imageFormatTo = imaging.JPEG
		}

		if err := imaging.Encode(&afterImage, img, imageFormatTo); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding image"})
			return
		}

	case "filters":
		if payload.Metadata.Filters == nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "no action data detected"})
			return
		}

		filters := payload.Metadata.Filters
		inverted := filters.Inverted != nil && *filters.Inverted
		grayscale := filters.Grayscale != nil && *filters.Grayscale
		flip := filters.Flip != nil && *filters.Flip
		blur := 0
		if filters.Blur != nil {
			blur = *filters.Blur
		}

		filtered, err := pkg.Filters(img, inverted, grayscale, flip, blur)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
			return
		}

		if err := imaging.Encode(&afterImage, filtered, imageFormat); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding filtered image"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "no valid action"})
		return
	}

	c.DataFromReader(http.StatusOK, int64(afterImage.Len()), http.DetectContentType(afterImage.Bytes()), &afterImage, nil)
}
