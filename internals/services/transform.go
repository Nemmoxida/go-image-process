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
	Rotate  *int         `json:"rotate"`
	Format  *string      `json:"format"`
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
		resized, err := pkg.Resize(img, payload.Metadata.Resize.Width, payload.Metadata.Resize.Height, payload.Metadata.Ext)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err})
			return
		}

		if err := imaging.Encode(&afterImage, resized, imageFormat); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding resized image"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "no valid action"})
		return
	}

	// client, err := azblob.NewClientFromConnectionString(os.Getenv("CON_STRING"), nil)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not connect to Azure Blob Storage"})
	// 	return
	// }

	// blobName := RandStringBytes(15) + ".png"
	// if _, err := client.UploadStream(context.Background(), os.Getenv("CON_NAME"), blobName, &afterImage, nil); err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not upload resized image"})
	// 	return
	// }

	// c.JSON(http.StatusOK, gin.H{
	// 	"status": "success",
	// 	"image":  "https://imageproject123.blob.core.windows.net/" + os.Getenv("CON_NAME") + "/" + blobName,
	// })

	c.DataFromReader(http.StatusOK, int64(afterImage.Len()), http.DetectContentType(afterImage.Bytes()), &afterImage, nil)
}
