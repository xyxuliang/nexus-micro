// Package event 提供领域事件系统。
// 支持发布/订阅模式，默认 MemoryBus 实现，可通过插件扩展为 NATS/Kafka 等。
package event

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xyxuliang/nexus-micro/internal/util"
)

// Event 领域事件接口。
type Event interface {
	// EventType 返回事件类型（用于路由分发）。
	EventType() string

	// EventID 返回事件唯一 ID。
	EventID() string

	// OccurredAt 返回事件发生时间。
	OccurredAt() time.Time

	// Payload 返回事件数据。
	Payload() interface{}
}

// Envelope 事件信封，用于跨服务传输。
// 包含事件元数据和序列化后的数据。
type Envelope struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Data       []byte    `json:"data"`
	OccurredAt time.Time `json:"occurred_at"`
	Source     string    `json:"source"`
	TraceID    string    `json:"trace_id,omitempty"`
}

// Handler 事件处理器函数。
type Handler func(ctx context.Context, evt Event) error

// Subscriber 事件订阅者，管理订阅和取消。
type Subscriber interface {
	// ID 返回订阅者 ID。
	ID() string

	// Unsubscribe 取消订阅。
	Unsubscribe() error
}

// Bus 事件总线接口。
// 支持发布/订阅、主题路由和插件扩展。
type Bus interface {
	// Publish 发布事件到所有订阅者。
	Publish(ctx context.Context, evt Event) error

	// Subscribe 订阅指定事件类型。
	Subscribe(eventType string, handler Handler) (Subscriber, error)

	// Unsubscribe 取消订阅指定主题的所有处理器。
	Unsubscribe(eventType string) error
}

// BaseEvent 基础事件实现，提供默认的 ID 和时间戳。
type BaseEvent struct {
	id         string
	occurredAt time.Time
}

// NewBaseEvent 创建一个基础事件。
func NewBaseEvent() BaseEvent {
	return BaseEvent{
		id:         util.GenerateID(),
		occurredAt: time.Now(),
	}
}

// EventID 返回事件唯一 ID。
func (e BaseEvent) EventID() string { return e.id }

// OccurredAt 返回事件发生时间。
func (e BaseEvent) OccurredAt() time.Time { return e.occurredAt }

// MemoryBus 基于内存的事件总线实现。
// 线程安全，适用于单服务内的事件通信。
type MemoryBus struct {
	mu          sync.RWMutex
	subscribers map[string][]struct {
		handler Handler
		id      string
	}
}

// NewMemoryBus 创建一个内存事件总线。
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		subscribers: make(map[string][]struct {
			handler Handler
			id      string
		}),
	}
}

// Publish 发布事件到该类型的所有订阅者。并发执行所有 handler。
func (b *MemoryBus) Publish(ctx context.Context, evt Event) error {
	b.mu.RLock()
	subs := b.subscribers[evt.EventType()]
	handlers := make([]Handler, len(subs))
	for i, s := range subs {
		handlers[i] = s.handler
	}
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(handlers))

	for _, h := range handlers {
		wg.Add(1)
		go func(handler Handler) {
			defer wg.Done()
			if err := handler(ctx, evt); err != nil {
				errCh <- err
			}
		}(h)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("event: %d handler(s) failed: %v", len(errs), errs[0])
	}
	return nil
}

// memorySubscriber 实现 Subscriber 接口。
type memorySubscriber struct {
	bus       *MemoryBus
	eventType string
	id        string
}

func (s *memorySubscriber) ID() string { return s.id }

func (s *memorySubscriber) Unsubscribe() error {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()

	subs := s.bus.subscribers[s.eventType]
	for i, sub := range subs {
		if sub.id == s.id {
			s.bus.subscribers[s.eventType] = append(subs[:i], subs[i+1:]...)
			return nil
		}
	}
	return nil
}

// Subscribe 订阅指定事件类型，返回 Subscriber 可用于取消订阅。
func (b *MemoryBus) Subscribe(eventType string, handler Handler) (Subscriber, error) {
	sub := &memorySubscriber{
		bus:       b,
		eventType: eventType,
		id:        util.GenerateID(),
	}

	b.mu.Lock()
	b.subscribers[eventType] = append(b.subscribers[eventType], struct {
		handler Handler
		id      string
	}{handler, sub.id})
	b.mu.Unlock()

	return sub, nil
}

// Unsubscribe 取消订阅指定主题的所有处理器。
func (b *MemoryBus) Unsubscribe(eventType string) error {
	b.mu.Lock()
	delete(b.subscribers, eventType)
	b.mu.Unlock()
	return nil
}

// 编译期接口断言
var _ Bus = (*MemoryBus)(nil)
var _ Subscriber = (*memorySubscriber)(nil)