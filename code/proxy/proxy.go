package main

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/gin-gonic/gin"
)

// Logger 日志
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		var logFilename string
		if runtime.GOOS == "windows" {
			logFilename = "v2.log"
		} else {
			logFilename = "v2.log"
		}
		f, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			panic(err)
		}
		st, _ := f.Stat()
		fmt.Println(st.Name())
		gin.DefaultWriter = io.MultiWriter(f, os.Stdout)
	}
}
