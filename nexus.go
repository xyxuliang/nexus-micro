// Package nexus 是 Nexus Micro 框架的统一入口。
// 通过此包开发者可以用最少的代码创建和启动一个完整的微服务。
//
// 典型用法：
//
//	import "github.com/nexus-micro/nexus-micro"
//
//	func main() {
//	    srv := nexus.NewServer(
//	        nexus.WithConfig("etc/config.yaml"),
//	        nexus.WithHTTP(":8080"),
//	        nexus.WithGRPC(":9090"),
//	    )
//	    srv.Start()
//	}
package nexus

import (
	"github.com/nexus-micro/nexus-micro/core"
	"github.com/nexus-micro/nexus-micro/core/server"
)

// NewServer 创建一个新的服务实例（便捷方法）。
// 等价于 server.New(options...)。
func NewServer(options ...server.Option) *server.Server {
	return server.New(options...)
}

// 导出核心类型和工具函数
type (
	Server     = server.Server
	Option     = server.Option
	Handler    = core.Handler
	Middleware = core.Middleware
	Chain      = core.MiddlewareChain
)

// 导出配置选项
var (
	WithConfig     = server.WithConfig
	WithHTTP       = server.WithHTTP
	WithGRPC       = server.WithGRPC
	WithMiddleware = server.WithMiddleware
	WithRateLimit  = server.WithRateLimit
	WithTimeout    = server.WithTimeout
	WithName       = server.WithName
	WithVersion    = server.WithVersion
)