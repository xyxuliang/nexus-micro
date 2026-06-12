package svc

import (
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/config"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/logic"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/repository"
)

type ServiceContext struct {
	Config       *config.Config
	PaymentRepo  repository.PaymentRepository
	PaymentLogic *logic.PaymentLogic
}

func New(cfg *config.Config) *ServiceContext {
	repo := repository.NewMemoryPaymentRepo()
	return &ServiceContext{Config: cfg, PaymentRepo: repo, PaymentLogic: logic.NewPaymentLogic(repo)}
}
