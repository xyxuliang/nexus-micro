// Package event 提供领域事件系统。
// 支持发布/订阅模式，默认底层使用 NATS，也可替换为其他实现。
// 业务代码只依赖 event 包的接口，不直接依赖 NATS，方便测试和替换。
package event

import (
	"context"
	"encoding/json"
	"sync"
)

// Event 是领域事件接口。
// 所有领域事件都必须实现此接口。
type Event interface {
	// Topic 返回事件主题，遵循 {domain}.{verb} 格式，如 user.created。
	Topic() string
}

// Handler 是事件处理器函数类型。
type Handler func(ctx context.Context, evt Event) error

// Bus 是事件总线接口。
type Bus interface {
	// Publish 发布一个事件。
	Publish(ctx context.Context, evt Event) error

	// Subscribe 订阅一个主题。
	Subscribe(topic string, handler Handler) error

	// Unsubscribe 取消订阅一个主题。
	Unsubscribe(topic string) error
}

// MemoryBus 是内存事件总线（用于开发测试和单体部署）。
// 所有事件都在内存中处理，不依赖外部消息中间件。
type MemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewMemoryBus 创建内存事件总线。
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		handlers: make(map[string][]Handler),
	}
}

// Publish 发布事件到内存总线。
func (b *MemoryBus) Publish(ctx context.Context, evt Event) error {
	b.mu.RLock()
	handlers, ok := b.handlers[evt.Topic()]
	b.mu.RUnlock()

	if !ok {
		return nil // 没有订阅者，静默成功
	}

	// 并发调用所有处理器
	var wg sync.WaitGroup
	errChan := make(chan error, len(handlers))

	for _, h := range handlers {
		wg.Add(1)
		go func(handler Handler) {
			defer wg.Done()
			if err := handler(ctx, evt); err != nil {
				errChan <- err
			}
		}(h)
	}

	wg.Wait()
	close(errChan)

	// 返回第一个错误
	for err := range errChan {
		return err
	}

	return nil
}

// Subscribe 订阅一个主题。
func (b *MemoryBus) Subscribe(topic string, handler Handler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[topic] = append(b.handlers[topic], handler)
	return nil
}

// Unsubscribe 取消订阅。
func (b *MemoryBus) Unsubscribe(topic string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.handlers, topic)
	return nil
}

// Publish 便捷函数：从 Bus 发布事件。
func Publish(ctx context.Context, bus Bus, evt Event) error {
	return bus.Publish(ctx, evt)
}

// Subscribe 便捷函数：在 Bus 上订阅主题。
func Subscribe(ctx context.Context, bus Bus, topic string, handler Handler) error {
	return bus.Subscribe(topic, handler)
}

// JSONMarshal 将事件序列化为 JSON。
func JSONMarshal(evt Event) ([]byte, error) {
	return json.Marshal(evt)
}

// JSONUnmarshal 将 JSON 反序列化为事件。
func JSONUnmarshal(data []byte, evt Event) error {
	return json.Unmarshal(data, evt)
}