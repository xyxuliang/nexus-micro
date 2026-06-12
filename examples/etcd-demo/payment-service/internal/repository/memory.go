package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/payment-service/internal/dto"
)

type MemoryPaymentRepo struct {
	mu       sync.RWMutex
	payments map[int64]*dto.Payment
	nextID   atomic.Int64
}

func NewMemoryPaymentRepo() *MemoryPaymentRepo {
	return &MemoryPaymentRepo{payments: make(map[int64]*dto.Payment)}
}

func (r *MemoryPaymentRepo) Create(ctx context.Context, orderId int64, amount float64, method string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextID.Add(1) - 1
	r.payments[id] = &dto.Payment{ID: id, OrderId: orderId, Amount: amount, Method: method, Status: "success", Created: time.Now().Unix()}
	return id, nil
}

func (r *MemoryPaymentRepo) FindByID(ctx context.Context, id int64) (*dto.Payment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.payments[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("payment %d not found", id)
}
