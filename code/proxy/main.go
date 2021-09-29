package main

import (
	"fmt"
	"io"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	setupRouter()
	select {}
}
func setupRouter() *gin.Engine {
	r := gin.Default()
	r.Use(Logger())

	f, err := os.Create("v2.log")
	if err != nil {
		panic(err)
	}

	gin.DefaultWriter = io.MultiWriter(f, os.Stdout)
	// r := gin.Default()
	r.GET("/", func(ctx *gin.Context) {
		fmt.Println("===")
	})
	r.Run(":12000")
	return r
}
