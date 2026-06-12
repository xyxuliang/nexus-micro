// Package nats 提供 NATS 消息代理实现。
// 实现 Nexus Micro 框架的 event.Bus 接口，支持发布/订阅模式。
// 基于 NATS 的 At-Least-Once 语义，适合高吞吐、低延迟的消息场景。
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/xyxuliang/nexus-micro/event"
)

// Broker 是基于 NATS 的事件总线实现。
// 实现 event.Bus 接口，支持发布/订阅和请求/响应模式。
type Broker struct {
	mu          sync.RWMutex
	conn        *nats.Conn          // NATS 连接
	js          nats.JetStreamContext // JetStream 上下文
	subscribers map[string][]event.Subscriber // topic -> subscribers
}

// Config 是 NATS Broker 的配置。
type Config struct {
	URL      string        // NATS 服务器地址（如 "nats://localhost:4222"）
	Name     string        // 客户端名称
	Timeout  time.Duration // 连接超时
	Stream   string        // JetStream Stream 名称
	Subjects []string      // JetStream 订阅主题
}

// NewBroker 创建一个新的 NATS Broker。
func NewBroker(cfg *Config) (*Broker, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Stream == "" {
		cfg.Stream = "nexus-events"
	}

	opts := []nats.Option{
		nats.Name(cfg.Name),
		nats.Timeout(cfg.Timeout),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // 无限重连
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats: connect failed: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: jetstream init failed: %w", err)
	}

	// 创建/更新 Stream
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     cfg.Stream,
		Subjects: cfg.Subjects,
		Storage:  nats.FileStorage,
	})
	if err != nil {
		// Stream 可能已存在，忽略错误
	}

	return &Broker{
		conn:        nc,
		js:          js,
		subscribers: make(map[string][]event.Subscriber),
	}, nil
}

// Publish 发布事件到指定主题。
func (b *Broker) Publish(ctx context.Context, topic string, evt event.Event) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("nats: marshal event failed: %w", err)
	}

	// 发布到 JetStream，确保持久化
	_, err = b.js.Publish(topic, data)
	if err != nil {
		return fmt.Errorf("nats: publish failed: %w", err)
	}

	return nil
}

// Subscribe 订阅指定主题的事件。
func (b *Broker) Subscribe(ctx context.Context, topic string, subscriber event.Subscriber) error {
	b.mu.Lock()
	b.subscribers[topic] = append(b.subscribers[topic], subscriber)
	b.mu.Unlock()

	// 创建 JetStream 订阅
	sub, err := b.js.Subscribe(topic, func(msg *nats.Msg) {
		// 反序列化事件
		var env event.Envelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			msg.Nak()
			return
		}

		// 执行订阅者处理
		for _, s := range b.subscribers[topic] {
			if err := s(ctx, &env); err != nil {
				msg.Nak()
				return
			}
		}
		msg.Ack()
	}, nats.ManualAck())

	if err != nil {
		return fmt.Errorf("nats: subscribe failed: %w", err)
	}

	_ = sub // 保持订阅活跃
	return nil
}

// Close 关闭 NATS 连接。
func (b *Broker) Close() {
	b.conn.Close()
}

// =============================================================================
// 使用示例
// =============================================================================

// Example 展示如何使用 NATS Broker。
func Example() {
	broker, err := NewBroker(&Config{
		URL:      "nats://localhost:4222",
		Name:     "nexus-service",
		Subjects: []string{"user.created", "order.paid"},
	})
	if err != nil {
		panic(err)
	}
	defer broker.Close()

	// 发布事件
	_ = broker.Publish(context.Background(), "user.created", &event.Envelope{
		ID:   "evt-001",
		Type: "user.created",
		Data: map[string]interface{}{"user_id": "123"},
	})

	// 订阅事件
	_ = broker.Subscribe(context.Background(), "user.created", func(ctx context.Context, evt event.Event) error {
		fmt.Printf("received event: %s\n", evt.EventType())
		return nil
	})
}