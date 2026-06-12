// Package router 注册网关 HTTP REST 路由。
// 对外暴露 REST API，内部通过 gRPC 连接池调用后端服务。
package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/handler"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool"
)

// Register 注册所有 HTTP 路由。
func Register(r *gin.Engine, pools *pool.Services) {
	h := handler.New(pools)

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "ok",
			"data": gin.H{
				"service": "apigw-rpc",
				"status":  "healthy",
				"time":    time.Now().Format(time.RFC3339),
			},
		})
	})

	api := r.Group("/api/v1")
	{
		// 用户服务
		api.POST("/users", h.CreateUser)
		api.GET("/users/:id", h.GetUser)

		// 首页服务
		api.GET("/home", h.GetHome)

		// 支付服务
		api.POST("/payments", h.CreatePayment)
		api.GET("/payments/:id", h.GetPayment)
	}
}
