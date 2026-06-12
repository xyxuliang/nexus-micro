// Package log 提供结构化日志能力。
// 零外部依赖，支持 JSON 和 Text 两种格式，支持分级日志（Debug/Info/Warn/Error）。
// 与 tracer 和 context 包深度集成，自动从 context 提取 request_id 和 trace_id。
package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/xyxuliang/nexus-micro/core/contextkeys"
	"github.com/xyxuliang/nexus-micro/core/tracer"
)

// =============================================================================
// 日志级别
// =============================================================================

// Level 表示日志级别。级别越高，越重要。
type Level int

const (
	LevelDebug Level = iota // 调试信息（开发环境）
	LevelInfo                // 常规信息（默认级别）
	LevelWarn                // 警告信息
	LevelError               // 错误信息
)

// String 返回级别的字符串表示。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel 从字符串解析日志级别。
func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info", "":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// =============================================================================
// 日志格式
// =============================================================================

// Format 表示日志输出格式。
type Format int

const (
	FormatJSON Format = iota // JSON 格式（生产环境推荐）
	FormatText               // 文本格式（开发环境推荐）
)

// =============================================================================
// Logger — 核心日志器
// =============================================================================

// Logger 是结构化日志器。
// 线程安全，支持从 context 自动提取 request_id、trace_id 等元信息。
type Logger struct {
	mu      sync.Mutex
	level   Level         // 最低输出级别
	format  Format        // 输出格式
	writer  io.Writer     // 输出目标
	fields  []Field       // 全局字段（如 service_name、version）
	enabled bool          // 是否启用
}

// Field 是结构化日志字段。
type Field struct {
	Key   string
	Value interface{}
}

// Config 是 Logger 的配置。
type Config struct {
	Level       Level   // 日志级别（默认 LevelInfo）
	Format      Format  // 输出格式（默认 FormatJSON）
	ServiceName string  // 服务名称
	Version     string  // 服务版本
	Writer      io.Writer // 输出目标（默认 os.Stdout）
}

// =============================================================================
// 全局 Logger
// =============================================================================

var globalLogger *Logger

// SetGlobalLogger 设置全局 Logger。
func SetGlobalLogger(l *Logger) {
	globalLogger = l
}

// GlobalLogger 返回全局 Logger。
func GlobalLogger() *Logger {
	if globalLogger == nil {
		globalLogger = NewLogger(&Config{
			Level:  LevelInfo,
			Format: FormatJSON,
		})
	}
	return globalLogger
}

// =============================================================================
// NewLogger — 构造函数
// =============================================================================

// NewLogger 创建一个新的 Logger 实例。
func NewLogger(cfg *Config) *Logger {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}

	l := &Logger{
		level:   cfg.Level,
		format:  cfg.Format,
		writer:  cfg.Writer,
		enabled: true,
	}

	// 全局字段
	if cfg.ServiceName != "" {
		l.fields = append(l.fields, Field{Key: "service", Value: cfg.ServiceName})
	}
	if cfg.Version != "" {
		l.fields = append(l.fields, Field{Key: "version", Value: cfg.Version})
	}

	return l
}

// =============================================================================
// 日志方法
// =============================================================================

// Debug 输出调试日志。
func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, fields...)
}

// Info 输出信息日志。
func (l *Logger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, fields...)
}

// Warn 输出警告日志。
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, fields...)
}

// Error 输出错误日志。
func (l *Logger) Error(msg string, fields ...Field) {
	l.log(LevelError, msg, fields...)
}

// =============================================================================
// 带 Context 的日志方法（自动提取 request_id、trace_id）
// =============================================================================

// DebugContext 从 context 提取元信息后输出调试日志。
func (l *Logger) DebugContext(ctx context.Context, msg string, fields ...Field) {
	l.logWithContext(ctx, LevelDebug, msg, fields...)
}

// InfoContext 从 context 提取元信息后输出信息日志。
func (l *Logger) InfoContext(ctx context.Context, msg string, fields ...Field) {
	l.logWithContext(ctx, LevelInfo, msg, fields...)
}

// WarnContext 从 context 提取元信息后输出警告日志。
func (l *Logger) WarnContext(ctx context.Context, msg string, fields ...Field) {
	l.logWithContext(ctx, LevelWarn, msg, fields...)
}

// ErrorContext 从 context 提取元信息后输出错误日志。
func (l *Logger) ErrorContext(ctx context.Context, msg string, fields ...Field) {
	l.logWithContext(ctx, LevelError, msg, fields...)
}

// =============================================================================
// 内部实现
// =============================================================================

// log 输出日志（内部方法）。
func (l *Logger) log(level Level, msg string, fields ...Field) {
	if !l.enabled || level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	allFields := append([]Field{}, l.fields...)
	allFields = append(allFields, Field{Key: "level", Value: level.String()})
	allFields = append(allFields, Field{Key: "msg", Value: msg})
	allFields = append(allFields, Field{Key: "time", Value: time.Now().Format(time.RFC3339Nano)})
	allFields = append(allFields, fields...)

	if l.format == FormatJSON {
		l.writeJSON(allFields)
	} else {
		l.writeText(level, msg, allFields)
	}
}

// logWithContext 从 context 提取元信息后输出日志。
func (l *Logger) logWithContext(ctx context.Context, level Level, msg string, fields ...Field) {
	if !l.enabled || level < l.level {
		return
	}

	// 从 context 提取元信息
	ctxFields := []Field{
		{Key: "request_id", Value: contextkeys.GetRequestID(ctx)},
		{Key: "trace_id", Value: contextkeys.GetTraceID(ctx)},
	}

	// 从 tracer context 提取 span 信息
	if sc := tracer.SpanContextFromContext(ctx); sc != nil {
		ctxFields = append(ctxFields, Field{Key: "span_id", Value: sc.SpanID})
	}

	allFields := append(ctxFields, fields...)
	l.log(level, msg, allFields...)
}

// writeJSON 以 JSON 格式输出日志。
func (l *Logger) writeJSON(fields []Field) {
	line := "{"
	for i, f := range fields {
		if i > 0 {
			line += ", "
		}
		line += fmt.Sprintf("\"%s\":%s", f.Key, toJSONValue(f.Value))
	}
	line += "}\n"
	fmt.Fprint(l.writer, line)
}

// writeText 以文本格式输出日志。
func (l *Logger) writeText(level Level, msg string, fields []Field) {
	now := time.Now().Format("15:04:05.000")
	prefix := fmt.Sprintf("%s [%s] %s", now, level.String(), msg)
	parts := prefix
	for _, f := range fields {
		if f.Key == "level" || f.Key == "msg" || f.Key == "time" {
			continue
		}
		parts += fmt.Sprintf(" %s=%v", f.Key, f.Value)
	}
	parts += "\n"
	fmt.Fprint(l.writer, parts)
}

// =============================================================================
// 便捷函数（使用全局 Logger）
// =============================================================================

// Debug 输出调试日志（全局 Logger）。
func Debug(msg string, fields ...Field) {
	GlobalLogger().log(LevelDebug, msg, fields...)
}

// Info 输出信息日志（全局 Logger）。
func Info(msg string, fields ...Field) {
	GlobalLogger().log(LevelInfo, msg, fields...)
}

// Warn 输出警告日志（全局 Logger）。
func Warn(msg string, fields ...Field) {
	GlobalLogger().log(LevelWarn, msg, fields...)
}

// Error 输出错误日志（全局 Logger）。
func Error(msg string, fields ...Field) {
	GlobalLogger().log(LevelError, msg, fields...)
}

// DebugContext 从 context 提取元信息后输出调试日志（全局 Logger）。
func DebugContext(ctx context.Context, msg string, fields ...Field) {
	GlobalLogger().logWithContext(ctx, LevelDebug, msg, fields...)
}

// InfoContext 从 context 提取元信息后输出信息日志（全局 Logger）。
func InfoContext(ctx context.Context, msg string, fields ...Field) {
	GlobalLogger().logWithContext(ctx, LevelInfo, msg, fields...)
}

// WarnContext 从 context 提取元信息后输出警告日志（全局 Logger）。
func WarnContext(ctx context.Context, msg string, fields ...Field) {
	GlobalLogger().logWithContext(ctx, LevelWarn, msg, fields...)
}

// ErrorContext 从 context 提取元信息后输出错误日志（全局 Logger）。
func ErrorContext(ctx context.Context, msg string, fields ...Field) {
	GlobalLogger().logWithContext(ctx, LevelError, msg, fields...)
}

// =============================================================================
// 辅助函数
// =============================================================================

// toJSONValue 将 Go 值转换为 JSON 字符串表示。
func toJSONValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("\"%s\"", val)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("\"%v\"", val)
	}
}

// =============================================================================
// 全局便捷函数
// =============================================================================

// 以下全局函数提供与标准库 log 类似的便捷使用方式。

// String 创建一个字符串字段。
func String(k, v string) Field { return Field{Key: k, Value: v} }

// Int 创建一个整数字段。
func Int(k string, v int) Field { return Field{Key: k, Value: v} }

// Int64 创建一个 int64 字段。
func Int64(k string, v int64) Field { return Field{Key: k, Value: v} }

// Float64 创建一个 float64 字段。
func Float64(k string, v float64) Field { return Field{Key: k, Value: v} }

// Bool 创建一个布尔字段。
func Bool(k string, v bool) Field { return Field{Key: k, Value: v} }

// Err 创建一个错误字段。
func Err(err error) Field { return Field{Key: "error", Value: err.Error()} }

// Duration 创建一个时长字段。
func Duration(k string, v time.Duration) Field { return Field{Key: k, Value: v.String()} }

// Any 创建一个任意类型字段。
func Any(k string, v interface{}) Field { return Field{Key: k, Value: v} }

// =============================================================================
// 使用示例
// =============================================================================

// Example 展示如何使用 Logger。
func Example() {
	// 1. 创建 Logger
	logger := NewLogger(&Config{
		Level:       LevelInfo,
		Format:      FormatJSON,
		ServiceName: "user-service",
		Version:     "1.0.0",
	})

	// 2. 记录日志
	logger.Info("server started",
		String("port", "8080"),
		Int("workers", 4),
	)

	logger.ErrorContext(context.Background(), "failed to connect to database",
		String("host", "localhost:5432"),
		Err(fmt.Errorf("connection refused")),
	)

	// 3. 使用全局 Logger
	SetGlobalLogger(logger)
	Info("request processed",
		String("method", "GET"),
		Int("status", 200),
		Duration("duration", 100*time.Millisecond),
	)
}