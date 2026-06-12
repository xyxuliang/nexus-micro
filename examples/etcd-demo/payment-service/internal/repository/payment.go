package repository

import "context"
import "github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/dto"

type PaymentRepository interface {
	Create(ctx context.Context, orderId int64, amount float64, method string) (int64, error)
	FindByID(ctx context.Context, id int64) (*dto.Payment, error)
}
