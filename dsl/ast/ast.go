// Package ast 定义了 Nexus Micro .api DSL 的抽象语法树（AST）。
// AST 是 DSL 解析器（Lexer/Parser）的输出，代码生成器的输入。
// .api 文件通过解析器转换为 AST，再由代码生成器生成 Handler、Client SDK、OpenAPI 文档。
package ast

// File 表示一个完整的 .api 文件。
type File struct {
	Syntax   string    // DSL 语法版本（如 "v1"）
	Info     *Info     // 文件元信息（标题、描述、版本、作者）
	Types    []*Type   // 类型定义列表
	Services []*Service // 服务定义列表
}

// Info 是 .api 文件的元信息。
type Info struct {
	Title   string // 服务标题
	Desc    string // 服务描述
	Version string // 版本号
	Author  string // 作者
}

// Type 表示一个类型定义（如 User、CreateUserReq）。
// 对应 .api 文件中 type(...) 块内的定义。
type Type struct {
	Name   string     // 类型名称
	Fields []*Field   // 字段列表
	Doc    string     // 文档注释
}

// Field 表示类型的一个字段。
type Field struct {
	Name     string // 字段名（如 "Id", "Name"）
	Type     string // 字段类型（如 "string", "int32", "[]User"）
	Tag      string // JSON 标签（如 `json:"id"`）
	Validate string // 校验规则（如 "required,email"）
	Path     string // 路径参数绑定（如 path:"id"）
	Form     string // 表单参数绑定（如 form:"page,default=1"）
	Default  string // 默认值
	Optional bool   // 是否可选
}

// Service 表示一个服务定义。
// 对应 .api 文件中 @server(...) 块和 service XxxService {...} 块。
type Service struct {
	Name       string    // 服务名（如 "UserService"）
	Prefix     string    // HTTP 路由前缀（如 "/api/v1"）
	Group      string    // 服务分组名（如 "user"）
	Middleware []string  // 服务级中间件
	Handlers   []*Handler // 接口列表
	Doc        string    // 文档注释
}

// Handler 表示一个接口定义。
// 对应 .api 文件中 @handler Xxx 和 @doc "..." 注解下的路由定义。
type Handler struct {
	Name       string // Handler 名称（如 "CreateUser"）
	Doc        string // 接口文档
	Method     string // HTTP 方法（GET/POST/PUT/DELETE）
	Path       string // HTTP 路由（如 "/users", "/users/:id"）
	HasGRPC    bool   // 是否同时生成 gRPC 接口
	IsSSE      bool   // 是否为 Server-Sent Events 端点（流式推送）
	Request    *Type  // 请求类型
	Response   *Type  // 响应类型
	Middleware []string // 接口级中间件
}

// Config 是 @server 注解的配置。
type Config struct {
	Prefix     string   // HTTP 路由前缀
	Service    string   // 服务名
	Middleware []string // 全局中间件
}