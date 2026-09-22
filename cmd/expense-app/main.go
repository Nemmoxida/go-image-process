package main

import (
	"simple-image-processing-golang/internals/router"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	r := router.Router()

	r.Run(":3001")
}
