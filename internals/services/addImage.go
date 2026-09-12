package services

import (
	"context"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/gin-gonic/gin"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.N(len(letterBytes))]
	}
	return string(b)
}

func AddImage(c *gin.Context) {
	image, err := c.FormFile("image")

	if err != nil || image.Size == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "no image detected",
		})
		return
	}

	ext := filepath.Ext(image.Filename)

	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "File type not supported",
		})
		return
	}

	token := RandStringBytes(15) + ext

	client, err := azblob.NewClientFromConnectionString(os.Getenv("CON_STRING"), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "internalError",
			"message": "Error connectiong to cloud storage",
		})
		return
	}

	file, err := image.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "internalError",
			"message": "Error opening image",
		})
		return
	}
	defer file.Close()

	_, err = client.UploadStream(context.Background(), os.Getenv("CON_NAME"), token, file, nil)
	if err != nil {
		log.Printf("Gagal mengunggah file ke Blob Storage: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "internalError",
			"message": "Error uploading image",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "File has been saved", "metadata": gin.H{
		"url":  "https://imageproject123.blob.core.windows.net/images/" + token,
		"file": token,
	}})
}
