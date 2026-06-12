package svc

import (
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/user-service/internal/config"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/user-service/internal/logic"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/user-service/internal/repository"
)

type ServiceContext struct {
	Config    *config.Config
	UserRepo  repository.UserRepository
	UserLogic *logic.UserLogic
}

func New(cfg *config.Config) *ServiceContext {
	repo := repository.NewMemoryUserRepo()
	return &ServiceContext{Config: cfg, UserRepo: repo, UserLogic: logic.NewUserLogic(repo)}
}
