package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/svc"
)

type HomeHandler struct{}

func NewHomeHandler() *HomeHandler { return &HomeHandler{} }

func (h *HomeHandler) GetHome(ctx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := ctx.HomeLogic.GetHome(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"code": 500, "message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"code": 0, "message": "ok", "data": resp})
	}
}
