// Package server 提供 user-service 的 gRPC 实现。
package server

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/user"
)

// userServer 实现 UserServiceServer 接口。
// 使用内存存储，演示 gRPC 服务端的基本用法。
type userServer struct {
	userpb.UnimplementedUserServiceServer
	mu    sync.Mutex
	users map[int64]*userpb.User
	seq   int64
}

// Register 注册 gRPC 服务到 gRPC Server。
func Register(srv *grpc.Server) {
	us := &userServer{
		users: make(map[int64]*userpb.User),
	}
	userpb.RegisterUserServiceServer(srv, us)
}

// CreateUser 创建用户。
func (s *userServer) CreateUser(ctx context.Context, req *userpb.CreateUserReq) (*userpb.CreateUserResp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	user := &userpb.User{
		Id:      s.seq,
		Name:    req.Name,
		Email:   req.Email,
		Created: time.Now().Unix(),
	}
	s.users[s.seq] = user

	return &userpb.CreateUserResp{Id: user.Id}, nil
}

// GetUser 获取用户详情。
func (s *userServer) GetUser(ctx context.Context, req *userpb.GetUserReq) (*userpb.GetUserResp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[req.Id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.Id)
	}
	return &userpb.GetUserResp{User: u}, nil
}
