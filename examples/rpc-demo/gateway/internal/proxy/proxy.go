// Package proxy 提供 gRPC 网关代理层。
// 网关对客户端暴露 gRPC 接口，内部通过连接池转发到后端服务。
package proxy

import (
	"context"

	"google.golang.org/grpc"

	homepb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/home"
	paymentpb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/payment"
	userpb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/user"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool"
)

// Register 将所有代理 gRPC 服务注册到 gRPC Server。
func Register(srv *grpc.Server, pools *pool.Services) {
	userpb.RegisterUserServiceServer(srv, &userProxy{pool: pools.User})
	homepb.RegisterHomeServiceServer(srv, &homeProxy{pool: pools.Home})
	paymentpb.RegisterPaymentServiceServer(srv, &paymentProxy{pool: pools.Payment})
}

// ---- user proxy ----

type userProxy struct {
	userpb.UnimplementedUserServiceServer
	pool *pool.GrpcPool
}

func (p *userProxy) CreateUser(ctx context.Context, req *userpb.CreateUserReq) (*userpb.CreateUserResp, error) {
	conn, _, err := p.pool.Select()
	if err != nil {
		return nil, err
	}
	return userpb.NewUserServiceClient(conn).CreateUser(ctx, req)
}

func (p *userProxy) GetUser(ctx context.Context, req *userpb.GetUserReq) (*userpb.GetUserResp, error) {
	conn, _, err := p.pool.Select()
	if err != nil {
		return nil, err
	}
	return userpb.NewUserServiceClient(conn).GetUser(ctx, req)
}

// ---- home proxy ----

type homeProxy struct {
	homepb.UnimplementedHomeServiceServer
	pool *pool.GrpcPool
}

func (p *homeProxy) GetHome(ctx context.Context, req *homepb.GetHomeReq) (*homepb.GetHomeResp, error) {
	conn, _, err := p.pool.Select()
	if err != nil {
		return nil, err
	}
	return homepb.NewHomeServiceClient(conn).GetHome(ctx, req)
}

// ---- payment proxy ----

type paymentProxy struct {
	paymentpb.UnimplementedPaymentServiceServer
	pool *pool.GrpcPool
}

func (p *paymentProxy) CreatePayment(ctx context.Context, req *paymentpb.CreatePaymentReq) (*paymentpb.CreatePaymentResp, error) {
	conn, _, err := p.pool.Select()
	if err != nil {
		return nil, err
	}
	return paymentpb.NewPaymentServiceClient(conn).CreatePayment(ctx, req)
}

func (p *paymentProxy) GetPayment(ctx context.Context, req *paymentpb.GetPaymentReq) (*paymentpb.GetPaymentResp, error) {
	conn, _, err := p.pool.Select()
	if err != nil {
		return nil, err
	}
	return paymentpb.NewPaymentServiceClient(conn).GetPayment(ctx, req)
}
