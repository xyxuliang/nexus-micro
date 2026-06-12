// Package handler 提供 HTTP REST → gRPC 的转换层。
// 网关对外暴露 REST API，内部通过 gRPC 连接池调用后端服务。
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool"
	homepb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/home"
	paymentpb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/payment"
	userpb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/user"
)

// HTTPHandler 提供 HTTP REST API。
// 将 JSON 请求转换为 gRPC 调用，结果以 JSON 返回。
type HTTPHandler struct {
	pools *pool.Services
}

// New 创建 HTTPHandler。
func New(pools *pool.Services) *HTTPHandler {
	return &HTTPHandler{pools: pools}
}

// ------- 用户服务 --------

// CreateUser POST /api/v1/users
func (h *HTTPHandler) CreateUser(c *gin.Context) {
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	conn, _, err := h.pools.User.Select()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": err.Error()})
		return
	}

	resp, err := userpb.NewUserServiceClient(conn).CreateUser(c.Request.Context(),
		&userpb.CreateUserReq{Name: req.Name, Email: req.Email})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"id": resp.Id}})
}

// GetUser GET /api/v1/users/:id
func (h *HTTPHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}

	conn, _, err := h.pools.User.Select()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": err.Error()})
		return
	}

	resp, err := userpb.NewUserServiceClient(conn).GetUser(c.Request.Context(),
		&userpb.GetUserReq{Id: id})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": resp})
}

// ------- 首页服务 --------

// GetHome GET /api/v1/home
func (h *HTTPHandler) GetHome(c *gin.Context) {
	conn, _, err := h.pools.Home.Select()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": err.Error()})
		return
	}

	resp, err := homepb.NewHomeServiceClient(conn).GetHome(c.Request.Context(), &homepb.GetHomeReq{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": resp})
}

// ------- 支付服务 --------

// CreatePayment POST /api/v1/payments
func (h *HTTPHandler) CreatePayment(c *gin.Context) {
	var req struct {
		OrderID int64   `json:"order_id"`
		Amount  float64 `json:"amount"`
		Method  string  `json:"method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	conn, _, err := h.pools.Payment.Select()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": err.Error()})
		return
	}

	resp, err := paymentpb.NewPaymentServiceClient(conn).CreatePayment(c.Request.Context(),
		&paymentpb.CreatePaymentReq{OrderId: req.OrderID, Amount: req.Amount, Method: req.Method})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"id": resp.Id, "status": resp.Status}})
}

// GetPayment GET /api/v1/payments/:id
func (h *HTTPHandler) GetPayment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}

	conn, _, err := h.pools.Payment.Select()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": err.Error()})
		return
	}

	resp, err := paymentpb.NewPaymentServiceClient(conn).GetPayment(c.Request.Context(),
		&paymentpb.GetPaymentReq{Id: id})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": resp})
}
