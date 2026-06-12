package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/dto"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/svc"
)

type PaymentHandler struct{}

func NewPaymentHandler() *PaymentHandler { return &PaymentHandler{} }

func (h *PaymentHandler) CreatePayment(ctx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.CreatePaymentReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"code": 400, "message": "invalid request"})
			return
		}
		resp, err := ctx.PaymentLogic.CreatePayment(c.Request.Context(), &req)
		if err != nil {
			c.JSON(500, gin.H{"code": 500, "message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"code": 0, "message": "ok", "data": resp})
	}
}

func (h *PaymentHandler) GetPayment(ctx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ID int64 `uri:"id"`
		}
		if err := c.ShouldBindUri(&req); err != nil {
			c.JSON(400, gin.H{"code": 400, "message": "invalid id"})
			return
		}
		resp, err := ctx.PaymentLogic.GetPayment(c.Request.Context(), &dto.GetPaymentReq{ID: req.ID})
		if err != nil {
			c.JSON(404, gin.H{"code": 404, "message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"code": 0, "message": "ok", "data": resp})
	}
}
