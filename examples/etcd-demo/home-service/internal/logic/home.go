package logic

import (
	"context"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/dto"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/repository"
)

// HomeLogic 首页业务逻辑。
type HomeLogic struct {
	repo repository.HomeRepository
}

// NewHomeLogic 创建 HomeLogic。
func NewHomeLogic(repo repository.HomeRepository) *HomeLogic {
	return &HomeLogic{repo: repo}
}

// GetHome 获取首页数据。
func (l *HomeLogic) GetHome(ctx context.Context) (*dto.HomeResp, error) {
	return l.repo.GetHome(ctx)
}
