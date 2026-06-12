package logic

import (
	"context"
	"fmt"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/dto"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/repository"
)

// PaymentLogic 支付业务逻辑。
type PaymentLogic struct {
	repo repository.PaymentRepository
}

// NewPaymentLogic 创建 PaymentLogic。
func NewPaymentLogic(repo repository.PaymentRepository) *PaymentLogic {
	return &PaymentLogic{repo: repo}
}

// CreatePayment 创建支付。
func (l *PaymentLogic) CreatePayment(ctx context.Context, req *dto.CreatePaymentReq) (*dto.CreatePaymentResp, error) {
	if req.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	id, err := l.repo.Create(ctx, req.OrderId, req.Amount, req.Method)
	if err != nil {
		return nil, err
	}
	return &dto.CreatePaymentResp{ID: id, Status: "success"}, nil
}

// GetPayment 获取支付详情。
func (l *PaymentLogic) GetPayment(ctx context.Context, req *dto.GetPaymentReq) (*dto.GetPaymentResp, error) {
	p, err := l.repo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return &dto.GetPaymentResp{Payment: *p}, nil
}
