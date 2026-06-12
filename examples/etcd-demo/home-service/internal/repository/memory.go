package repository

import (
	"context"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/dto"
)

type MemoryHomeRepo struct{}

func NewMemoryHomeRepo() *MemoryHomeRepo { return &MemoryHomeRepo{} }

func (r *MemoryHomeRepo) GetHome(ctx context.Context) (*dto.HomeResp, error) {
	return &dto.HomeResp{
		Message: "Welcome to Nexus Micro!",
		Banners: []dto.Banner{
			{ID: 1, Title: "Fast", Image: "/img/fast.png", Url: "/fast", Sort: 1},
			{ID: 2, Title: "Simple", Image: "/img/simple.png", Url: "/simple", Sort: 2},
			{ID: 3, Title: "Reliable", Image: "/img/reliable.png", Url: "/reliable", Sort: 3},
		},
	}, nil
}
