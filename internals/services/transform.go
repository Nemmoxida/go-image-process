package services

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"office-expense-management-backend/pkg"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
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
	URL     string       `json:"url" binding:"required,url"`
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
	var request newImageReq
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	// check if rezie width is valid (more than 0)
	// if request.Resize.Width <= 0 || request.Resize.Height <= 0 {
	// 	c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "resize width and height must be greater than zero"})
	// 	return
	// }

	parsedURL, err := url.Parse(request.Metadata.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "url must use http or https"})
		return
	}

	// get image from cloud
	response, err := http.Get(request.Metadata.URL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": "could not download image"})
		return
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": "image URL returned an unsuccessful response"})
		return
	}

	img, err := imaging.Decode(response.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not decode image"})
		return
	}

	var afterImage bytes.Buffer

	switch request.Action {
	case "resize":
		resized := pkg.Resize(img, request.Metadata.Resize.Width, request.Metadata.Resize.Height, request.Metadata.Ext)

		if err := imaging.Encode(&afterImage, resized, imaging.PNG); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "error encoding resized image"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "no valid action"})
		return
	}

	client, err := azblob.NewClientFromConnectionString(os.Getenv("CON_STRING"), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not connect to Azure Blob Storage"})
		return
	}

	blobName := RandStringBytes(15) + ".png"
	if _, err := client.UploadStream(context.Background(), os.Getenv("CON_NAME"), blobName, &afterImage, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not upload resized image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"image":  "https://imageproject123.blob.core.windows.net/" + os.Getenv("CON_NAME") + "/" + blobName,
	})
}
