package services

import (
	"context"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/gin-gonic/gin"
)

func ApplyTransform(c *gin.Context) {
	imgHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "could not detect image", "message": err})
	}

	img, _ := imgHeader.Open()
	defer img.Close()

	client, err := azblob.NewClientFromConnectionString(os.Getenv("CON_STRING"), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not connect to Azure Blob Storage"})
		return
	}

	blobName := RandStringBytes(15) + ".png"
	if _, err := client.UploadStream(context.Background(), os.Getenv("CON_NAME"), blobName, img, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "could not upload resized image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"image":  "https://imageproject123.blob.core.windows.net/" + os.Getenv("CON_NAME") + "/" + blobName,
	})
}
