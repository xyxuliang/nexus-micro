package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		b := make([]byte, 8)
		rand.Read(b)
		id := hex.EncodeToString(b)
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		start := time.Now()
		c.Next()
		c.Header("X-Response-Time", strconv.FormatInt(time.Since(start).Milliseconds(), 10)+"ms")
	}
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Request-ID")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func Log() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		c.Next()
		rid, _ := c.Get("request_id")
		log.Printf("[svc] %s %s %d %s rid=%v", method, path, c.Writer.Status(), time.Since(start), rid)
	}
}
