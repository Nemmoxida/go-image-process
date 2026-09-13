package router

import (
	"office-expense-management-backend/internals/services"

	"github.com/gin-gonic/gin"
)

func Router() *gin.Engine {
	r := gin.Default()

	r.POST("/login", services.Login)
	r.POST("/signup", services.Signup)
	// image upload for edit
	r.POST("/images", services.AddImage)
	// send the image and image id to the client
	r.GET("/images/:id")
	// apply edit to the image
	r.POST("/images/transform", services.Transform)

	return r

}
