package svc

import (
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/config"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/logic"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/repository"
)

type ServiceContext struct {
	Config    *config.Config
	HomeRepo  repository.HomeRepository
	HomeLogic *logic.HomeLogic
}

func New(cfg *config.Config) *ServiceContext {
	repo := repository.NewMemoryHomeRepo()
	return &ServiceContext{Config: cfg, HomeRepo: repo, HomeLogic: logic.NewHomeLogic(repo)}
}
