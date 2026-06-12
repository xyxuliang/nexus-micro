// Package server 提供 home-service 的 gRPC 实现。
package server

import (
	"context"

	"google.golang.org/grpc"

	homepb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/home"
)

// homeServer 实现 HomeServiceServer 接口。
type homeServer struct {
	homepb.UnimplementedHomeServiceServer
}

// Register 注册 gRPC 服务。
func Register(srv *grpc.Server) {
	homepb.RegisterHomeServiceServer(srv, &homeServer{})
}

// GetHome 获取首页数据。
func (s *homeServer) GetHome(ctx context.Context, req *homepb.GetHomeReq) (*homepb.GetHomeResp, error) {
	banners := []*homepb.Banner{
		{Id: 1, Title: "Nexus Micro 微服务框架", Image: "/img/banner1.png", Url: "/docs", Sort: 1},
		{Id: 2, Title: "gRPC 高性能通信", Image: "/img/banner2.png", Url: "/rpc", Sort: 2},
		{Id: 3, Title: "etcd 服务发现", Image: "/img/banner3.png", Url: "/discovery", Sort: 3},
	}

	return &homepb.GetHomeResp{
		Banners: banners,
		Message: "Welcome to Nexus Micro RPC!",
	}, nil
}
