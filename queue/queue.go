// Package queue 提供异步任务队列统一接口。
// 默认实现基于 Asynq（基于 Redis），支持延迟任务、多优先级、自动重试。
// 框架只定义接口，不强制绑定 Asynq，可替换为其他实现。
package queue

import (
	"context"
	"time"
)

// Task 表示一个异步任务。
type Task struct {
	ID        string                 // 任务 ID
	Type      string                 // 任务类型
	Queue     string                 // 目标队列名（如 "critical", "default", "low"）
	Payload   interface{}            // 任务数据（任意类型，会被 JSON 序列化）
	MaxRetry  int                    // 最大重试次数
	Priority  int                    // 优先级（越大优先级越高）
	CreatedAt time.Time              // 创建时间
}

// Handler 任务处理器函数。
// payload 是反序列化后的任务数据。
type Handler func(ctx context.Context, payload interface{}) error

// Queue 任务队列接口。
type Queue interface {
	// Dispatch 分发一个立即执行的任务。
	Dispatch(ctx context.Context, task *Task) error

	// Delay 分发一个延迟执行的任务。
	Delay(ctx context.Context, task *Task, delay time.Duration) error

	// Retry 重试一个失败的任务。
	Retry(ctx context.Context, task *Task) error

	// Register 注册任务处理器。
	Register(taskType string, handler Handler)

	// Run 启动 worker 开始处理任务（阻塞）。
	Run() error

	// Shutdown 优雅关闭队列。
	Shutdown(ctx context.Context) error
}

// NewTask 创建一个新任务。
func NewTask(taskType string, payload interface{}) *Task {
	return &Task{
		Type:      taskType,
		Payload:   payload,
		Queue:     "default",
		MaxRetry:  3,
		Priority:  5,
		CreatedAt: time.Now(),
	}
}

// WithMaxRetry 设置最大重试次数。
func (t *Task) WithMaxRetry(n int) *Task {
	t.MaxRetry = n
	return t
}

// WithPriority 设置优先级。
func (t *Task) WithPriority(p int) *Task {
	t.Priority = p
	return t
}

// WithQueue 设置目标队列名。
func (t *Task) WithQueue(q string) *Task {
	t.Queue = q
	return t
}

// WithID 设置任务 ID。
func (t *Task) WithID(id string) *Task {
	t.ID = id
	return t
}

// 编译期接口断言
var _ Queue = (*noopQueue)(nil)

type noopQueue struct{}

func (q *noopQueue) Dispatch(ctx context.Context, task *Task) error            { return nil }
func (q *noopQueue) Delay(ctx context.Context, task *Task, delay time.Duration) error { return nil }
func (q *noopQueue) Retry(ctx context.Context, task *Task) error               { return nil }
func (q *noopQueue) Register(taskType string, handler Handler)                 {}
func (q *noopQueue) Run() error                                                { return nil }
func (q *noopQueue) Shutdown(ctx context.Context) error                        { return nil }