// Package router 负责注册网关的 HTTP 路由。
package router

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/gateway/internal/config"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/gateway/internal/proxy"
)

// Register 根据配置注册所有网关路由。
func Register(r *gin.Engine, cfg *config.Config, ph *proxy.Handler) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"code":    0,
			"message": "gateway ok",
			"data": gin.H{
				"service": cfg.Server.Name,
				"routes":  len(cfg.Routes),
				"uptime":  time.Now().Unix(),
			},
		})
	})

	// 代理路由
	log.Println("[gateway] routes:")
	for _, rt := range cfg.Routes {
		pattern := rt.Prefix + rt.Path
		handler := ph.Handle(rt)
		switch strings.ToUpper(rt.Method) {
		case "GET":
			r.GET(pattern, handler)
		case "POST":
			r.POST(pattern, handler)
		case "PUT":
			r.PUT(pattern, handler)
		case "DELETE":
			r.DELETE(pattern, handler)
		default:
			r.Any(pattern, handler)
		}
		log.Printf("  %-6s %s → %s", rt.Method, pattern, rt.Upstream)
	}
}
