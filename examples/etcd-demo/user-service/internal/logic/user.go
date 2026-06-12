package logic

import (
	"context"
	"fmt"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/user-service/internal/dto"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/user-service/internal/repository"
)

type UserLogic struct{ repo repository.UserRepository }

func NewUserLogic(repo repository.UserRepository) *UserLogic { return &UserLogic{repo: repo} }

func (l *UserLogic) CreateUser(ctx context.Context, req *dto.CreateUserReq) (*dto.CreateUserResp, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	id, err := l.repo.Create(ctx, req.Name, req.Email)
	if err != nil {
		return nil, err
	}
	return &dto.CreateUserResp{ID: id}, nil
}

func (l *UserLogic) GetUser(ctx context.Context, req *dto.GetUserReq) (*dto.GetUserResp, error) {
	u, err := l.repo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return &dto.GetUserResp{User: *u}, nil
}
