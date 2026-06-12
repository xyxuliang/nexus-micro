package repository

import "context"
import "github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/dto"

type HomeRepository interface {
	GetHome(ctx context.Context) (*dto.HomeResp, error)
}
