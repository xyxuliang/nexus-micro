// Package server 提供 payment-service 的 gRPC 实现。
package server

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentpb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/payment"
	"google.golang.org/grpc"
)

// paymentServer 实现 PaymentServiceServer 接口。
type paymentServer struct {
	paymentpb.UnimplementedPaymentServiceServer
	mu       sync.Mutex
	payments map[int64]*paymentpb.Payment
	seq      int64
}

// Register 注册 gRPC 服务。
func Register(srv *grpc.Server) {
	ps := &paymentServer{
		payments: make(map[int64]*paymentpb.Payment),
	}
	paymentpb.RegisterPaymentServiceServer(srv, ps)
}

// CreatePayment 创建支付。
func (s *paymentServer) CreatePayment(ctx context.Context, req *paymentpb.CreatePaymentReq) (*paymentpb.CreatePaymentResp, error) {
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	p := &paymentpb.Payment{
		Id:      s.seq,
		OrderId: req.OrderId,
		Amount:  req.Amount,
		Method:  req.Method,
		Status:  "success",
		Created: time.Now().Unix(),
	}
	s.payments[s.seq] = p

	return &paymentpb.CreatePaymentResp{Id: p.Id, Status: p.Status}, nil
}

// GetPayment 获取支付详情。
func (s *paymentServer) GetPayment(ctx context.Context, req *paymentpb.GetPaymentReq) (*paymentpb.GetPaymentResp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.payments[req.Id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "payment %d not found", req.Id)
	}
	return &paymentpb.GetPaymentResp{Payment: p}, nil
}
