package repository

import "context"
import "github.com/xyxuliang/nexus-micro/examples/etcd-demo/user-service/internal/dto"

type UserRepository interface {
	Create(ctx context.Context, name, email string) (int64, error)
	FindByID(ctx context.Context, id int64) (*dto.User, error)
}
