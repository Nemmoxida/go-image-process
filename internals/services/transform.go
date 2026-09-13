package services

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
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

func Transform(c *gin.Context) {
	var request imageReq
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	if request.Resize.Width <= 0 || request.Resize.Height <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "resize width and height must be greater than zero"})
		return
	}

	parsedURL, err := url.Parse(request.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "url must use http or https"})
		return
	}

	// get image from cloud
	response, err := http.Get(request.URL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": "could not download image"})
		return
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": "image URL returned an unsuccessful response"})
		return
	}

	// if err := os.MkdirAll("images", 0755); err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not create images directory"})
	// 	return
	// }

	// baseName := filepath.Base(c.Param("id"))
	// if baseName == "." || baseName == "" || baseName == string(filepath.Separator) {
	// 	baseName = RandStringBytes(15)
	// }
	// baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	// sourcePath := filepath.Join("images", baseName+".png")
	// file, err := os.Create(sourcePath)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not save downloaded image"})
	// 	return
	// }

	// // save image to local
	// _, copyErr := io.Copy(file, response.Body)
	// closeErr := file.Close()
	// if copyErr != nil || closeErr != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not save downloaded image"})
	// 	return
	// }

	// open the image for processing
	// img, err := imaging.Open(sourcePath)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "downloaded file is not a supported image"})
	// 	return
	// }

	img, err := imaging.Decode(response.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not decode image"})
		return
	}

	resized := imaging.Resize(img, request.Resize.Width, request.Resize.Height, imaging.Lanczos)

	var resizedBuffer bytes.Buffer
	var imageFormat imaging.Format

	switch request.Ext {
	case "png":
		imageFormat = imaging.PNG
	case "jpeg", "jpg":
		imageFormat = imaging.JPEG
	}

	if err := imaging.Encode(&resizedBuffer, resized, imageFormat); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err})
	}

	// resizedPath := filepath.Join("images", baseName+"_resized.png")
	// if err := imaging.Save(resized, resizedPath); err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": fmt.Sprintf("could not save resized image: %v", err)})
	// 	return
	// }

	client, err := azblob.NewClientFromConnectionString(os.Getenv("CON_STRING"), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not connect to Azure Blob Storage"})
		return
	}

	// resizedFile, err := os.Open(resizedPath)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not open resized image"})
	// 	return
	// }
	// defer resizedFile.Close()

	blobName := RandStringBytes(15) + ".png"
	if _, err := client.UploadStream(context.Background(), os.Getenv("CON_NAME"), blobName, &resizedBuffer, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not upload resized image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"image":  "https://imageproject123.blob.core.windows.net/" + os.Getenv("CON_NAME") + "/" + blobName,
	})
}
