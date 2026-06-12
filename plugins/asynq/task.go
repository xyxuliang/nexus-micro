// Package asynq 提供 Asynq 异步任务队列实现。
// 实现 Nexus Micro 框架的 queue.Queue 接口，支持延迟任务、优先级和重试。
// 基于 Redis 的可靠任务队列，适合异步处理、定时任务和后台作业。
package asynq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/xyxuliang/nexus-micro/queue"
)

// Queue 是基于 Asynq 的任务队列实现。
// 实现 queue.Queue 接口，提供任务分发、调度和执行能力。
type Queue struct {
	client    *asynq.Client    // 任务生产者
	server    *asynq.Server    // 任务消费者
	mux       *asynq.ServeMux  // 任务路由
	handlers  map[string]queue.Handler
}

// Config 是 Asynq Queue 的配置。
type Config struct {
	RedisAddr     string        // Redis 地址（如 "localhost:6379"）
	RedisPassword string        // Redis 密码
	DB            int           // Redis 数据库编号
	Concurrency   int           // 并发 worker 数量
	Queues        map[string]int // 队列优先级（队列名 -> 优先级权重）
}

// NewQueue 创建一个新的 Asynq Queue。
func NewQueue(cfg *Config) (*Queue, error) {
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 10
	}
	if cfg.Queues == nil {
		cfg.Queues = map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		}
	}

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.DB,
	}

	client := asynq.NewClient(redisOpt)
	mux := asynq.NewServeMux()

	return &Queue{
		client:   client,
		mux:      mux,
		handlers: make(map[string]queue.Handler),
		server: asynq.NewServer(
			redisOpt,
			asynq.Config{
				Concurrency: cfg.Concurrency,
				Queues:      cfg.Queues,
			},
		),
	}, nil
}

// Dispatch 分发一个普通任务。
func (q *Queue) Dispatch(ctx context.Context, task *queue.Task) error {
	payload, err := json.Marshal(task.Payload)
	if err != nil {
		return fmt.Errorf("asynq: marshal task failed: %w", err)
	}

	opts := []asynq.Option{
		asynq.TaskID(task.ID),
		asynq.MaxRetry(task.MaxRetry),
		asynq.Queue(task.Queue),
	}

	asynqTask := asynq.NewTask(task.Type, payload)

	_, err = q.client.EnqueueContext(ctx, asynqTask, opts...)
	if err != nil {
		return fmt.Errorf("asynq: enqueue failed: %w", err)
	}

	return nil
}

// Delay 分发一个延迟任务。
func (q *Queue) Delay(ctx context.Context, task *queue.Task, delay time.Duration) error {
	payload, err := json.Marshal(task.Payload)
	if err != nil {
		return fmt.Errorf("asynq: marshal task failed: %w", err)
	}

	opts := []asynq.Option{
		asynq.TaskID(task.ID),
		asynq.MaxRetry(task.MaxRetry),
		asynq.Queue(task.Queue),
		asynq.ProcessIn(delay),
	}

	asynqTask := asynq.NewTask(task.Type, payload)

	_, err = q.client.EnqueueContext(ctx, asynqTask, opts...)
	if err != nil {
		return fmt.Errorf("asynq: delay enqueue failed: %w", err)
	}

	return nil
}

// Retry 重试一个失败的任务。
func (q *Queue) Retry(ctx context.Context, task *queue.Task) error {
	return q.Dispatch(ctx, task)
}

// Register 注册任务处理器。
func (q *Queue) Register(taskType string, handler queue.Handler) {
	q.handlers[taskType] = handler
	q.mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
		return handler(ctx, t.Payload())
	})
}

// Run 启动任务消费者。
func (q *Queue) Run() error {
	return q.server.Run(q.mux)
}

// Shutdown 优雅关闭任务队列。
func (q *Queue) Shutdown(ctx context.Context) error {
	q.client.Close()
	q.server.Shutdown()
	return nil
}

// =============================================================================
// 使用示例
// =============================================================================

// Example 展示如何使用 Asynq Queue。
func Example() {
	q, err := NewQueue(&Config{
		RedisAddr: "localhost:6379",
		Concurrency: 10,
	})
	if err != nil {
		panic(err)
	}
	defer q.Shutdown(context.Background())

	// 注册任务处理器
	q.Register("send_welcome_email", func(ctx context.Context, payload interface{}) error {
		fmt.Printf("sending welcome email: %v\n", payload)
		return nil
	})

	// 分发任务
	_ = q.Dispatch(context.Background(), &queue.Task{
		ID:       "task-001",
		Type:     "send_welcome_email",
		Payload:  map[string]interface{}{"user_id": "123", "email": "user@example.com"},
		MaxRetry: 3,
		Queue:    "default",
	})

	// 分发延迟任务
	_ = q.Delay(context.Background(), &queue.Task{
		ID:       "task-002",
		Type:     "send_welcome_email",
		Payload:  map[string]interface{}{"user_id": "456"},
		MaxRetry: 3,
	}, 5*time.Minute)

	// 启动消费
	_ = q.Run()
}