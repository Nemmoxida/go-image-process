package middleware

import (
	"net/http"
	"simple-image-processing-golang/pkg"

	"github.com/gin-gonic/gin"
)

func AuthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("authorization")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Unautorized": "No Token Found"})
			return
		}

		claims, err := pkg.VerifyToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return

		}
		c.Set("claims", claims)
		c.Next()

	}

}
