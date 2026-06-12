// Package proxy 提供网关的长连接池代理处理器。
package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/core/client"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/gateway/internal/config"
)

// Handler 长连接池代理处理器，为每条网关路由创建对应的 gin.HandlerFunc。
type Handler struct {
	clients map[string]*client.PooledClient
}

// New 创建代理处理器。
func New(clients map[string]*client.PooledClient) *Handler {
	return &Handler{clients: clients}
}

// Handle 为指定路由创建 gin.HandlerFunc。
// 转发流程：读取请求体 → 长连接池 CallRaw（负载均衡+熔断+重试）→ 透传响应。
func (h *Handler) Handle(route config.RouteConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		cli, ok := h.clients[route.Upstream]
		if !ok {
			c.JSON(http.StatusBadGateway, gin.H{
				"code":    502,
				"message": fmt.Sprintf("upstream %s not available", route.Upstream),
			})
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "read body failed"})
			return
		}
		defer c.Request.Body.Close()

		resp, err := cli.CallRaw(c.Request.Context(), route.Method, c.Request.URL.Path, body)
		if err != nil {
			log.Printf("[proxy] error %s %s → %v", route.Method, c.Request.URL.Path, err)
			c.JSON(http.StatusBadGateway, gin.H{
				"code":    502,
				"message": fmt.Sprintf("%v", err),
			})
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			for _, vv := range v {
				c.Header(k, vv)
			}
		}
		c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
	}
}
