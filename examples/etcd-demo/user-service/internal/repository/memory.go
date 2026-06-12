package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/user-service/internal/dto"
)

type MemoryUserRepo struct {
	mu     sync.RWMutex
	users  map[int64]*dto.User
	nextID atomic.Int64
}

func NewMemoryUserRepo() *MemoryUserRepo {
	r := &MemoryUserRepo{users: make(map[int64]*dto.User)}
	r.nextID.Store(3)
	now := time.Now().Unix()
	r.users[1] = &dto.User{ID: 1, Name: "Alice", Email: "alice@example.com", Created: now}
	r.users[2] = &dto.User{ID: 2, Name: "Bob", Email: "bob@example.com", Created: now}
	return r
}

func (r *MemoryUserRepo) Create(ctx context.Context, name, email string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextID.Add(1) - 1
	r.users[id] = &dto.User{ID: id, Name: name, Email: email, Created: time.Now().Unix()}
	return id, nil
}

func (r *MemoryUserRepo) FindByID(ctx context.Context, id int64) (*dto.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("user %d not found", id)
}
