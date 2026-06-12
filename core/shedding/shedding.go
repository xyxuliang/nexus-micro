// Package shedding 提供 CPU 和内存双层过载保护。
// 当 CPU 或内存使用率超过阈值，直接拒绝新请求，避免服务被拖垮。
// 参考 go-zero 的过载保护设计。
package shedding

import (
	"runtime"
	"sync"
	"time"
)

// Shedder 过载保护器接口。
type Shedder interface {
	// Allow 判断当前请求是否允许通过。
	// true = 允许，false = 拒绝。
	Allow() bool
}

// Config 过载保护配置。
type Config struct {
	CPUThreshold float64 // CPU 使用率阈值（0-1，默认 0.9 = 90%）
	MemThreshold float64 // 内存使用率阈值（0-1，默认 0.85 = 85%）
	Window       int64    // 滑动窗口大小（秒，默认 5s）
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		CPUThreshold: 0.9,
		MemThreshold: 0.85,
		Window:       5,
	}
}

// Dummy 无过载保护（总是允许）。
type Dummy struct{}

func (d Dummy) Allow() bool { return true }

// DoubleShedder CPU + 内存双层过载保护。
type DoubleShedder struct {
	cfg     *Config
	mu      sync.Mutex
	lastCPU float64
	lastMem float64
}

// New 创建双层过载保护器。
func New(cfg *Config) *DoubleShedder {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &DoubleShedder{
		cfg: cfg,
	}
}

// Allow 判断当前请求是否允许通过。
// true = 允许，false = 拒绝。
func (ds *DoubleShedder) Allow() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	cpuUsage := getCPUUsage()
	memUsage := getMemoryUsage()

	ds.lastCPU = cpuUsage
	ds.lastMem = memUsage

	if cpuUsage >= ds.cfg.CPUThreshold {
		return false
	}
	if memUsage >= ds.cfg.MemThreshold {
		return false
	}
	return true
}

// LastCPU 返回最近一次测量的 CPU 使用率。
func (ds *DoubleShedder) LastCPU() float64 {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.lastCPU
}

// LastMem 返回最近一次测量的内存使用率。
func (ds *DoubleShedder) LastMem() float64 {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.lastMem
}

// getCPUUsage 获取当前 CPU 使用率（估算）。
// 简化实现：基于 GOMAXPROCS 和 Goroutine 数量粗略估算。
// 生产环境可以使用第三方库（如 github.com/shirou/gopsutil/v3/cpu）获取精确值。
func getCPUUsage() float64 {
	// 简化实现：基于 goroutine 数量粗略估算
	// 每个 goroutine 占约 1/GOMAXPROCS 使用率
	gomaxprocs := runtime.GOMAXPROCS(0)
	goroutines := runtime.NumGoroutine()
	usage := float64(goroutines) / float64(gomaxprocs*4) // 4 goroutines per CPU on average
	if usage > 1.0 {
		usage = 1.0
	}
	return usage
}

// getMemoryUsage 获取当前内存使用率。
// 返回 0-1 之间的值（已使用 / 可用）。
func getMemoryUsage() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// 估算：Alloc / Sys
	return float64(m.Alloc) / float64(m.Sys)
}

// IsOverloaded 返回是否过载。
func (ds *DoubleShedder) IsOverloaded() bool {
	return !ds.Allow()
}