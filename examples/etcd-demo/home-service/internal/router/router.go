package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/handler"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/svc"
)

func Register(r *gin.Engine, ctx *svc.ServiceContext) {
	h := handler.NewHomeHandler()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "ok",
			"data": gin.H{
				"service": ctx.Config.Server.Name,
				"status":  "healthy",
				"time":    time.Now().Format(time.RFC3339),
			},
		})
	})

	api := r.Group("/api/v1")
	{
		api.GET("/home", h.GetHome(ctx))
	}
}
