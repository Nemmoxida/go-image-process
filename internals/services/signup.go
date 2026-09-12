package services

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func Signup(c *gin.Context) {
	var req UserReq

	c.ShouldBindJSON(&req)

	// pool, err := database.Connect()
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"errorInternalDatabase": err})
	// 	return
	// }
	// defer pool.Close()

	hashsedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"errorProcessingRequest": err})
	}

	c.JSON(http.StatusOK, gin.H{"hash": hashsedPassword})

	// your query to the database
	// row := pool.QueryRow(context.Background(), "INSERT INTO ** VALUES ($1, $2)", req.Username, hashsedPassword)

}
