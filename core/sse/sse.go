// Package sse 提供 Server-Sent Events 支持。
// 实现了完整的 SSE 协议规范，支持自动重连、心跳保活、事件 ID 追踪、
// 多事件类型、Channel 便捷 API 和与框架中间件链的无缝集成。
//
// SSE 协议规范：https://html.spec.whatwg.org/multipage/server-sent-events.html
//
// 使用方式（基础）：
//
//	sseHandler := sse.NewHandler(func(w *sse.Writer, r *http.Request) {
//	    w.SendEvent(sse.Event{Data: "hello"})
//	})
//	http.Handle("/events", sseHandler)
//
// 使用方式（Channel + 心跳，推荐）：
//
//	sseHandler := sse.NewChannelHandler(func(ch chan<- sse.Event, r *http.Request) {
//	    ticker := time.NewTicker(1 * time.Second)
//	    for range ticker.C {
//	        ch <- sse.Event{Data: fmt.Sprintf("time: %v", time.Now())}
//	    }
//	})
//	http.Handle("/events", sseHandler)
package sse

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// Writer — SSE 响应写入器
// =============================================================================

// Writer 是 SSE 协议的写入器，封装了 http.ResponseWriter。
// 自动设置 Content-Type 为 text/event-stream，并管理 Flusher 和连接状态。
type Writer struct {
	mu       sync.Mutex
	w        http.ResponseWriter
	flusher  http.Flusher
	closed   bool
	eventID  int64       // 自动递增的事件 ID
	lastID   string      // 最后一次发送的事件 ID（用于 Last-Event-ID 重连）
	notify   chan struct{} // 通知连接关闭
}

// NewWriter 创建一个 SSE Writer。
// 必须在调用任何 Write 方法之前调用 WriteHeader 设置 SSE 响应头。
func NewWriter(w http.ResponseWriter) (*Writer, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("sse: http.ResponseWriter does not implement http.Flusher")
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲
	w.WriteHeader(http.StatusOK)

	flusher.Flush()

	return &Writer{
		w:       w,
		flusher: flusher,
		notify:  make(chan struct{}),
	}, nil
}

// Event 是 SSE 事件的完整结构体。
// 每个字段对应 SSE 协议中的一个字段行。
type Event struct {
	ID    string      // 事件 ID（用于 Last-Event-ID 自动重连）
	Type  string      // 事件类型（默认 "message"）
	Data  interface{} // 事件数据（支持 string、[]byte、fmt.Stringer）
	Retry time.Duration // 重连间隔（毫秒，客户端据此自动重连）
}

// SendEvent 发送一个 SSE 事件。
// 自动递增 Event ID，如果用户未设置。
func (sw *Writer) SendEvent(evt Event) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.closed {
		return fmt.Errorf("sse: writer is closed")
	}

	sw.eventID++
	sw.lastID = strconv.FormatInt(sw.eventID, 10)

	// 事件 ID（用于自动重连）
	if evt.ID != "" {
		sw.lastID = evt.ID
		fmt.Fprintf(sw.w, "id: %s\n", evt.ID)
	} else {
		fmt.Fprintf(sw.w, "id: %s\n", sw.lastID)
	}

	// 事件类型
	if evt.Type != "" {
		fmt.Fprintf(sw.w, "event: %s\n", evt.Type)
	}

	// 重连间隔
	if evt.Retry > 0 {
		fmt.Fprintf(sw.w, "retry: %d\n", evt.Retry.Milliseconds())
	}

	// 事件数据（支持多行，每行以 "data: " 开头）
	data := formatData(evt.Data)
	fmt.Fprintf(sw.w, "data: %s\n\n", data)

	sw.flusher.Flush()
	return nil
}

// SendComment 发送 SSE 注释（用于心跳保活）。
// 注释行以 ":" 开头，客户端会忽略但连接保持活跃。
func (sw *Writer) SendComment(comment string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.closed {
		return fmt.Errorf("sse: writer is closed")
	}

	// 注释行：以冒号开头，客户端忽略
	if comment == "" {
		fmt.Fprintf(sw.w, ":\n")
	} else {
		fmt.Fprintf(sw.w, ": %s\n", comment)
	}

	sw.flusher.Flush()
	return nil
}

// SendPing 发送心跳保活（空注释）。
func (sw *Writer) SendPing() error {
	return sw.SendComment("ping")
}

// Close 关闭 SSE 连接。
func (sw *Writer) Close() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if !sw.closed {
		sw.closed = true
		close(sw.notify)
	}
}

// IsClosed 检查连接是否已关闭。
func (sw *Writer) IsClosed() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.closed
}

// Done 返回一个 channel，连接关闭时关闭。
// 可用于 select 监听连接状态。
func (sw *Writer) Done() <-chan struct{} {
	return sw.notify
}

// LastEventID 返回最后一次发送的事件 ID。
func (sw *Writer) LastEventID() string {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.lastID
}

// =============================================================================
// Handler — SSE HTTP 处理器
// =============================================================================

// HandlerFunc 是 SSE 处理函数类型。
// 接收 Writer 和 *http.Request，在函数内发送事件。
// Writer 会在连接断开时自动关闭。
type HandlerFunc func(w *Writer, r *http.Request)

// NewHandler 创建一个 SSE HTTP Handler。
// handler 在独立的 goroutine 中运行，连接断开时自动退出。
//
// 使用方式：
//
//	http.Handle("/events", sse.NewHandler(func(w *sse.Writer, r *http.Request) {
//	    for i := 0; i < 10; i++ {
//	        w.SendEvent(sse.Event{Data: fmt.Sprintf("msg #%d", i)})
//	        time.Sleep(1 * time.Second)
//	    }
//	}))
func NewHandler(handler HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw, err := NewWriter(w)
		if err != nil {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		// 监听客户端断开
		ctx := r.Context()
		go func() {
			<-ctx.Done()
			sw.Close()
		}()

		handler(sw, r)
		sw.Close()
	})
}

// =============================================================================
// ChannelHandler — 基于 Channel 的 SSE 处理器（推荐）
// =============================================================================

// ChannelHandlerFunc 是面向 Channel 的 SSE 处理函数类型。
// 用户通过 channel 发送事件，框架负责写入 HTTP 响应。
// 这种方式比直接使用 Writer 更安全，避免了并发写入问题。
type ChannelHandlerFunc func(ch chan<- Event, r *http.Request)

// ChannelConfig 是 ChannelHandler 的配置。
type ChannelConfig struct {
	HeartbeatInterval time.Duration // 心跳间隔（0 表示不发送心跳，默认 30s）
	BufferSize        int           // 事件 Channel 缓冲区大小（默认 64）
}

// NewChannelHandler 创建一个基于 Channel 的 SSE 处理器。
// 这是推荐的使用方式，用户通过 channel 发送事件，框架负责写入和心跳。
//
// 使用方式：
//
//	http.Handle("/events", sse.NewChannelHandler(func(ch chan<- sse.Event, r *http.Request) {
//	    ticker := time.NewTicker(1 * time.Second)
//	    defer ticker.Stop()
//	    for {
//	        select {
//	        case <-ticker.C:
//	            ch <- sse.Event{Data: fmt.Sprintf("now: %v", time.Now())}
//	        case <-r.Context().Done():
//	            return
//	        }
//	    }
//	}))
func NewChannelHandler(handler ChannelHandlerFunc, cfg ...*ChannelConfig) http.Handler {
	config := &ChannelConfig{
		HeartbeatInterval: 30 * time.Second,
		BufferSize:        64,
	}
	if len(cfg) > 0 && cfg[0] != nil {
		if cfg[0].HeartbeatInterval > 0 {
			config.HeartbeatInterval = cfg[0].HeartbeatInterval
		}
		if cfg[0].BufferSize > 0 {
			config.BufferSize = cfg[0].BufferSize
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw, err := NewWriter(w)
		if err != nil {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		ch := make(chan Event, config.BufferSize)
		ctx := r.Context()

		// 心跳 ticker
		var heartbeatTicker *time.Ticker
		var heartbeatCh <-chan time.Time
		if config.HeartbeatInterval > 0 {
			heartbeatTicker = time.NewTicker(config.HeartbeatInterval)
			defer heartbeatTicker.Stop()
			heartbeatCh = heartbeatTicker.C
		}

		// 启动用户 handler（在独立 goroutine 中发送事件）
		go func() {
			defer close(ch)
			handler(ch, r)
		}()

		// 事件循环：从 channel 读取事件并写入 HTTP 响应
		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					// channel 关闭，用户 handler 结束
					return
				}
				if err := sw.SendEvent(evt); err != nil {
					return
				}
			case <-heartbeatCh:
				// 发送心跳保持连接
				if err := sw.SendPing(); err != nil {
					return
				}
			case <-ctx.Done():
				// 客户端断开
				sw.Close()
				return
			case <-sw.Done():
				return
			}
		}
	})
}

// =============================================================================
// 高阶 API 工厂函数
// =============================================================================

// NewIntervalHandler 创建一个定时发送事件的 SSE 处理器。
// 按固定间隔调用 factory 生成事件并发送。
//
// 使用方式：
//
//	http.Handle("/events", sse.NewIntervalHandler(2*time.Second, func() sse.Event {
//	    return sse.Event{Data: fmt.Sprintf("time: %v", time.Now())}
//	}))
func NewIntervalHandler(interval time.Duration, factory func() Event) http.Handler {
	return NewChannelHandler(func(ch chan<- Event, r *http.Request) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ch <- factory()
			case <-r.Context().Done():
				return
			}
		}
	}, &ChannelConfig{HeartbeatInterval: interval * 3})
}

// NewStreamHandler 创建一个流式 SSE 处理器。
// 从 channel 读取事件并发送，适合已有事件源（如消息队列）的场景。
//
// 使用方式：
//
//	eventCh := make(chan sse.Event, 100)
//	http.Handle("/events", sse.NewStreamHandler(eventCh))
func NewStreamHandler(eventCh <-chan Event, cfg ...*ChannelConfig) http.Handler {
	return NewChannelHandler(func(ch chan<- Event, r *http.Request) {
		for {
			select {
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				select {
				case ch <- evt:
				case <-r.Context().Done():
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	}, cfg...)
}

// =============================================================================
// 事件构建器（Builder 模式）
// =============================================================================

// EventBuilder 是 SSE 事件的构建器，提供链式 API。
//
// 使用方式：
//
//	evt := sse.NewEvent().
//	    WithID("123").
//	    WithType("update").
//	    WithData(map[string]interface{}{"status": "ok"}).
//	    WithRetry(5 * time.Second)
type EventBuilder struct {
	event Event
}

// NewEvent 创建一个新的事件构建器。
func NewEvent() *EventBuilder {
	return &EventBuilder{}
}

// WithID 设置事件 ID。
func (b *EventBuilder) WithID(id string) *EventBuilder {
	b.event.ID = id
	return b
}

// WithType 设置事件类型。
func (b *EventBuilder) WithType(t string) *EventBuilder {
	b.event.Type = t
	return b
}

// WithData 设置事件数据。
func (b *EventBuilder) WithData(data interface{}) *EventBuilder {
	b.event.Data = data
	return b
}

// WithRetry 设置重连间隔。
func (b *EventBuilder) WithRetry(d time.Duration) *EventBuilder {
	b.event.Retry = d
	return b
}

// Build 构建事件。
func (b *EventBuilder) Build() Event {
	return b.event
}

// =============================================================================
// 辅助函数
// =============================================================================

// formatData 格式化事件数据。
// 支持多行字符串（每行以 "data: " 开头）和 JSON 序列化。
func formatData(data interface{}) string {
	if data == nil {
		return ""
	}

	switch v := data.(type) {
	case string:
		// 多行字符串：每行加 "data: " 前缀
		lines := strings.Split(v, "\n")
		if len(lines) == 1 {
			return v
		}
		// 多行数据需要逐行发送
		result := ""
		for i, line := range lines {
			if i > 0 {
				result += "\n"
			}
			result += "data: " + line
		}
		return result
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ParseLastEventID 从 HTTP 请求中提取 Last-Event-ID。
// 用于客户端重连时恢复事件流。
func ParseLastEventID(r *http.Request) string {
	// 优先从 HTTP Header 获取
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		return id
	}
	// 其次从查询参数获取
	if id := r.URL.Query().Get("last_event_id"); id != "" {
		return id
	}
	return ""
}

// =============================================================================
// 响应包装 — 跳过 JSON 序列化
// =============================================================================

// IsSSE 检查 HTTP 请求是否为 SSE 请求。
// 判断依据：Accept 头包含 text/event-stream。
func IsSSE(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/event-stream")
}

// =============================================================================
// 使用示例
// =============================================================================

// Example 展示 SSE 的完整使用方式。
func Example() {
	// 方式 1：基础 Handler
	http.Handle("/events/basic", NewHandler(func(w *Writer, r *http.Request) {
		for i := 0; i < 10; i++ {
			w.SendEvent(Event{
				Type: "update",
				Data: fmt.Sprintf("message #%d", i),
			})
			time.Sleep(1 * time.Second)
		}
	}))

	// 方式 2：Channel Handler（推荐）
	http.Handle("/events/channel", NewChannelHandler(func(ch chan<- Event, r *http.Request) {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ch <- NewEvent().
					WithType("heartbeat").
					WithData(map[string]interface{}{
						"time": time.Now().Unix(),
					}).
					Build()
			case <-r.Context().Done():
				return
			}
		}
	}))

	// 方式 3：定时推送
	http.Handle("/events/interval", NewIntervalHandler(3*time.Second, func() Event {
		return Event{
			Type: "update",
			Data: fmt.Sprintf("server time: %v", time.Now().Format(time.RFC3339)),
		}
	}))

	// 方式 4：从外部 channel 流式推送
	eventCh := make(chan Event, 100)
	http.Handle("/events/stream", NewStreamHandler(eventCh))

	_ = eventCh
}