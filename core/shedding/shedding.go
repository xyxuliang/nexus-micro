// Package shedding 提供自适应过载保护（降级/削峰）。
// 基于 CPU 使用率和内存使用率双重指标，当系统负载过高时自动拒绝请求，
// 防止服务雪崩，保障核心服务的可用性。
//
// 使用 /proc/stat 读取真实 CPU 使用率，基于滑动窗口平滑计算。
package shedding

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Shedder 过载保护接口。
type Shedder interface {
	// Allow 判断是否允许请求通过。
	Allow() bool
}

// Config 过载保护配置。
type Config struct {
	CPUThreshold float64       // CPU 使用率阈值（0.0-1.0），默认 0.9
	MemThreshold float64       // 内存使用率阈值（0.0-1.0），默认 0.85
	Window       time.Duration // 滑动窗口大小，默认 5 秒
}

// AdaptiveShedder 自适应过载保护实现。
// 基于 /proc/stat 读取 CPU 使用率，基于 runtime.MemStats 读取内存使用率。
type AdaptiveShedder struct {
	mu           sync.RWMutex
	cpuThreshold float64
	memThreshold float64
	window       time.Duration

	// CPU 统计
	prevIdle  float64
	prevTotal float64
	lastCPU   float64
	lastCheck time.Time

	// 滑动窗口 CPU 采样
	cpuSamples []float64
	sampleIdx  int
}

// New 创建一个过载保护器。
// 如果 cfg 为 nil，使用默认配置。
func New(cfg *Config) *AdaptiveShedder {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.CPUThreshold <= 0 {
		cfg.CPUThreshold = 0.9
	}
	if cfg.MemThreshold <= 0 {
		cfg.MemThreshold = 0.85
	}
	if cfg.Window <= 0 {
		cfg.Window = 5 * time.Second
	}

	s := &AdaptiveShedder{
		cpuThreshold: cfg.CPUThreshold,
		memThreshold: cfg.MemThreshold,
		window:       cfg.Window,
		cpuSamples:   make([]float64, 10),
	}

	// 初始化 CPU 基准
	s.updateCPU()
	return s
}

// Allow 判断是否允许请求通过。
// 返回 false 表示系统过载，应拒绝请求。
func (s *AdaptiveShedder) Allow() bool {
	s.mu.RLock()
	cpu := s.smoothedCPU()
	mem := s.getMemoryUsage()
	s.mu.RUnlock()

	// 定期更新 CPU 采样
	if time.Since(s.lastCheck) > s.window/10 {
		s.mu.Lock()
		s.updateCPU()
		s.mu.Unlock()
	}

	return cpu < s.cpuThreshold && mem < s.memThreshold
}

// smoothedCPU 返回滑动窗口平滑后的 CPU 使用率。
func (s *AdaptiveShedder) smoothedCPU() float64 {
	if len(s.cpuSamples) == 0 {
		return 0
	}
	var sum float64
	count := 0
	for _, v := range s.cpuSamples {
		if v > 0 {
			sum += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// updateCPU 从 /proc/stat 读取 CPU 使用率并更新滑动窗口。
// CPU 使用率 = 1 - (idle2 - idle1) / (total2 - total1)
func (s *AdaptiveShedder) updateCPU() {
	now := time.Now()

	idle, total := readCPUStat()
	if s.prevTotal > 0 && total > s.prevTotal {
		deltaIdle := idle - s.prevIdle
		deltaTotal := total - s.prevTotal
		if deltaTotal > 0 {
			cpu := 1.0 - deltaIdle/deltaTotal
			if cpu < 0 {
				cpu = 0
			}
			if cpu > 1.0 {
				cpu = 1.0
			}
			// 写入滑动窗口
			s.cpuSamples[s.sampleIdx] = cpu
			s.sampleIdx = (s.sampleIdx + 1) % len(s.cpuSamples)
			s.lastCPU = cpu
		}
	}

	s.prevIdle = idle
	s.prevTotal = total
	s.lastCheck = now
}

// readCPUStat 读取 /proc/stat 的第一行 CPU 统计信息。
// 返回空闲时间和总时间（以 jiffies 为单位）。
func readCPUStat() (idle, total float64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}

	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return 0, 0
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0
	}

	// /proc/stat 格式: cpu user nice system idle iowait irq softirq steal ...
	var cpuFields [8]float64
	for i := 1; i < len(fields) && i-1 < 8; i++ {
		cpuFields[i-1], _ = strconv.ParseFloat(fields[i], 64)
	}

	// idle = idle + iowait
	idle = cpuFields[3] + cpuFields[4]
	// total = user + nice + system + idle + iowait + irq + softirq + steal
	for _, v := range cpuFields {
		total += v
	}

	return idle, total
}

// getMemoryUsage 获取内存使用率。
func (s *AdaptiveShedder) getMemoryUsage() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.HeapSys > 0 {
		return float64(m.HeapAlloc) / float64(m.HeapSys)
	}
	return 0
}

// 编译期接口断言
var _ Shedder = (*AdaptiveShedder)(nil)