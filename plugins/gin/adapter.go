// Package gin 提供 Gin 框架适配器。
// 将 Gin 的 HandlerFunc 和中间件无缝集成到 Nexus Micro 框架中。
// 如果你熟悉 Gin 的 API 风格，可以继续使用 Gin 的 Handler 编写业务逻辑。
package gin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/core"
	"github.com/xyxuliang/nexus-micro/core/middleware"
)

// Adapter 将 Gin 集成到 Nexus Micro 的 HTTP 路由中。
// 提供 Gin 的 Context 和 HandlerFunc 风格，同时保留框架的治理能力。
type Adapter struct {
	engine *gin.Engine
	chain  *core.MiddlewareChain
}

// NewAdapter 创建一个新的 Gin 适配器。
// mode: 运行模式（gin.DebugMode / gin.ReleaseMode / gin.TestMode）
func NewAdapter(mode string) *Adapter {
	gin.SetMode(mode)
	return &Adapter{
		engine: gin.New(),
		chain:  middleware.DefaultChain(),
	}
}

// Engine 返回底层的 Gin Engine 实例。
// 可以直接使用 Gin 的 API 注册路由和中间件。
func (a *Adapter) Engine() *gin.Engine {
	return a.engine
}

// HTTPHandler 返回兼容 http.Handler 的处理器。
// 可以直接传递给 http.Server 或 Nexus Micro Server。
func (a *Adapter) HTTPHandler() http.Handler {
	return a.engine
}

// Use 添加 Gin 中间件。
func (a *Adapter) Use(middleware ...gin.HandlerFunc) {
	a.engine.Use(middleware...)
}

// Handle 注册路由。
func (a *Adapter) Handle(method, path string, handlers ...gin.HandlerFunc) {
	a.engine.Handle(method, path, handlers...)
}

// GET 注册 GET 路由。
func (a *Adapter) GET(path string, handlers ...gin.HandlerFunc) {
	a.engine.GET(path, handlers...)
}

// POST 注册 POST 路由。
func (a *Adapter) POST(path string, handlers ...gin.HandlerFunc) {
	a.engine.POST(path, handlers...)
}

// PUT 注册 PUT 路由。
func (a *Adapter) PUT(path string, handlers ...gin.HandlerFunc) {
	a.engine.PUT(path, handlers...)
}

// DELETE 注册 DELETE 路由。
func (a *Adapter) DELETE(path string, handlers ...gin.HandlerFunc) {
	a.engine.DELETE(path, handlers...)
}

// Group 创建路由组。
func (a *Adapter) Group(path string, handlers ...gin.HandlerFunc) *gin.RouterGroup {
	return a.engine.Group(path, handlers...)
}

// =============================================================================
// 中间件转换
// =============================================================================

// ToGinMiddleware 将 core.Middleware 转换为 Gin HandlerFunc。
// 这样框架的中间件（RequestID、Tracing、Logger 等）可以在 Gin 中使用。
func ToGinMiddleware(mw core.Middleware) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// 包装为 core.Handler
		handler := mw(func(ctx context.Context, req interface{}) (interface{}, error) {
			c.Next()
			return nil, nil
		})

		handler(ctx, nil)
	}
}

// =============================================================================
// 使用示例
// =============================================================================

// Example 展示如何将 Gin 适配器集成到 Nexus Micro 中。
//
// 使用方式：
//
//	adapter := gin.NewAdapter(gin.ReleaseMode)
//	adapter.GET("/api/v1/users/:id", func(c *gin.Context) {
//	    c.JSON(200, gin.H{"id": c.Param("id"), "name": "test"})
//	})
//
//	// 使用 Nexus Micro Server
//	srv := server.New(server.WithName("user-service"))
//	srv.RegisterService(adapter.HTTPHandler())
//	srv.Start()
func Example() {
	_ = NewAdapter(gin.DebugMode)
}