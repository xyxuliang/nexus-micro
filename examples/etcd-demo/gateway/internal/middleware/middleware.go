// Package middleware 提供 gateway 的 Gin 中间件。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestID 为每个请求注入唯一 request_id。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := generateID()
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		start := time.Now()
		c.Next()
		c.Header("X-Response-Time", strconv.FormatInt(time.Since(start).Milliseconds(), 10)+"ms")
	}
}

// CORS 处理跨域请求。
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

// Timeout 设置请求超时。
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		done := make(chan struct{}, 1)
		go func() {
			c.Next()
			done <- struct{}{}
		}()
		select {
		case <-done:
		case <-time.After(d):
			c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
				"code":    504,
				"message": "request timeout",
			})
		}
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
