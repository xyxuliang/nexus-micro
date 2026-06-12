// Nexus Micro — Next Generation Golang Microservice Framework
// CLI 工具入口，提供项目创建、代码生成、开发运行等命令。
//
// 使用方式：
//
//	nx new <service-name>         创建新服务
//	nx gen api <file.api>         从 .api 生成完整服务
//	nx gen proto <file.proto>     从 .proto 生成完整服务
//	nx gen client <service-name>  生成客户端 SDK
//	nx gen doc                    生成 OpenAPI 文档
//	nx module <name>              创建业务模块
//	nx slice <module> <name>      创建 Vertical Slice 切片
//	nx query <module> <name>      创建 Query 切片
//	nx run                        启动开发服务器
//	nx build                      构建生产二进制
//	nx test                       运行测试
//	nx lint                       代码检查
package main

import (
	"fmt"
	"os"

	"github.com/xyxuliang/nexus-micro/generator"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "new":
		cmdNew(os.Args[2:])
	case "gen":
		cmdGen(os.Args[2:])
	case "module":
		cmdModule(os.Args[2:])
	case "slice":
		cmdSlice(os.Args[2:])
	case "query":
		cmdQuery(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "build":
		cmdBuild(os.Args[2:])
	case "test":
		cmdTest(os.Args[2:])
	case "lint":
		cmdLint(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "nx: unknown command %q\n", command)
		fmt.Fprintf(os.Stderr, "Run 'nx help' for usage.\n")
		os.Exit(1)
	}
}

// printUsage 打印 CLI 使用帮助。
func printUsage() {
	fmt.Println(`Nexus Micro CLI — Next Generation Golang Microservice Framework

Usage:
  nx <command> [arguments]

Commands:
  new <service-name>         创建新服务
  gen api <file.api>         从 .api 生成完整服务
  gen proto <file.proto>     从 .proto 生成完整服务
  gen client <service-name>  生成客户端 SDK
  gen doc                    生成 OpenAPI 文档
  module <name>              创建业务模块 (Vertical Slice)
  slice <module> <name>      创建 Command 切片
  query <module> <name>      创建 Query 切片
  run                        启动开发服务器（热重载）
  build                      构建生产二进制
  test                       运行测试
  lint                       代码检查

Examples:
  nx new mysvc                    # 创建 mysvc 服务
  nx gen api user.api             # 从 user.api 生成代码
  nx module user                  # 创建 user 模块
  nx slice user register          # 创建 register 切片
  nx query user profile           # 创建 profile 查询
  nx run                          # 启动开发服务器`)
}

// cmdNew 创建新服务。
func cmdNew(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx new: missing service name")
		fmt.Fprintln(os.Stderr, "Usage: nx new <service-name>")
		os.Exit(1)
	}

	serviceName := args[0]
	dirs := []string{
		serviceName,
		serviceName + "/etc",
		serviceName + "/internal/config",
		serviceName + "/internal/handler",
		serviceName + "/internal/logic",
		serviceName + "/internal/svc",
		serviceName + "/internal/middleware",
		serviceName + "/internal/server",
		serviceName + "/client",
		serviceName + "/docs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "nx new: failed to create %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	fmt.Printf("✓ Service %s created\n", serviceName)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Write your .api DSL in %s/%s.api\n", serviceName, serviceName)
	fmt.Printf("  2. Run: nx gen api %s/%s.api\n", serviceName, serviceName)
	fmt.Printf("  3. Run: nx run\n")
}

// cmdGen 代码生成。
func cmdGen(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "nx gen: missing subcommand")
		fmt.Fprintln(os.Stderr, "Usage: nx gen [api|proto|client|doc] ...")
		os.Exit(1)
	}

	subcommand := args[0]
	switch subcommand {
	case "api":
		cmdGenAPI(args[1:])
	case "proto":
		cmdGenProto(args[1:])
	case "client":
		cmdGenClient(args[1:])
	case "doc":
		cmdGenDoc(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "nx gen: unknown subcommand %q\n", subcommand)
		os.Exit(1)
	}
}

// cmdGenAPI 从 .api 文件生成代码。
func cmdGenAPI(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx gen api: missing .api file")
		fmt.Fprintln(os.Stderr, "Usage: nx gen api <file.api>")
		os.Exit(1)
	}

	apiFile := args[0]
	fmt.Printf("Generating code from %s...\n", apiFile)

	gen := generator.New("service", "github.com/xyxuliang/nexus-micro")
	if err := gen.GenerateFromFile(apiFile); err != nil {
		fmt.Fprintf(os.Stderr, "nx gen api: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Code generation completed")
}

// cmdGenProto 从 .proto 文件生成代码。
func cmdGenProto(args []string) {
	fmt.Println("nx gen proto: generating from .proto file...")
	fmt.Println("(protoc + protoc-gen-go integration coming soon)")
}

// cmdGenClient 生成客户端 SDK。
func cmdGenClient(args []string) {
	fmt.Println("nx gen client: generating client SDK...")
	fmt.Println("(client SDK generation coming soon)")
}

// cmdGenDoc 生成 OpenAPI 文档。
func cmdGenDoc(args []string) {
	fmt.Println("nx gen doc: generating OpenAPI documentation...")
	fmt.Println("(OpenAPI doc generation coming soon)")
}

// cmdModule 创建业务模块。
func cmdModule(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx module: missing module name")
		fmt.Fprintln(os.Stderr, "Usage: nx module <name>")
		os.Exit(1)
	}

	moduleName := args[0]
	dirs := []string{
		"app/" + moduleName + "/domain",
		"app/" + moduleName + "/internal",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "nx module: failed to create %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	fmt.Printf("✓ Module %s created\n", moduleName)
	fmt.Printf("  app/%s/domain/    — entity, repository, event, errors\n", moduleName)
	fmt.Printf("  app/%s/internal/  — repository implementation\n", moduleName)
}

// cmdSlice 创建 Vertical Slice 切片。
func cmdSlice(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "nx slice: missing module or slice name")
		fmt.Fprintln(os.Stderr, "Usage: nx slice <module> <name>")
		os.Exit(1)
	}

	moduleName := args[0]
	sliceName := args[1]
	sliceDir := "app/" + moduleName + "/" + sliceName

	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "nx slice: failed to create %s: %v\n", sliceDir, err)
		os.Exit(1)
	}

	// 创建切片文件
	files := []string{"command.go", "handler.go", "validator.go", "dto.go", "mapper.go", "event.go"}
	for _, f := range files {
		path := sliceDir + "/" + f
		os.WriteFile(path, []byte("// "+sliceName+" "+f+"\n"), 0644)
	}

	fmt.Printf("✓ Slice %s/%s created\n", moduleName, sliceName)
	fmt.Printf("  app/%s/%s/ — command, handler, validator, dto, mapper, event\n", moduleName, sliceName)
}

// cmdQuery 创建 Query 切片。
func cmdQuery(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "nx query: missing module or query name")
		fmt.Fprintln(os.Stderr, "Usage: nx query <module> <name>")
		os.Exit(1)
	}

	moduleName := args[0]
	queryName := args[1]
	queryDir := "app/" + moduleName + "/" + queryName

	if err := os.MkdirAll(queryDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "nx query: failed to create %s: %v\n", queryDir, err)
		os.Exit(1)
	}

	files := []string{"query.go", "handler.go", "dto.go"}
	for _, f := range files {
		path := queryDir + "/" + f
		os.WriteFile(path, []byte("// "+queryName+" "+f+"\n"), 0644)
	}

	fmt.Printf("✓ Query %s/%s created\n", moduleName, queryName)
}

// cmdRun 启动开发服务器。
func cmdRun(args []string) {
	fmt.Println("Starting development server...")
	fmt.Println("(hot reload support coming soon)")
	fmt.Println("")
	fmt.Println("Run the following to start manually:")
	fmt.Println("  go run main.go")
}

// cmdBuild 构建生产二进制。
func cmdBuild(args []string) {
	fmt.Println("Building production binary...")
	fmt.Println("(build integration coming soon)")
	fmt.Println("")
	fmt.Println("Run the following to build manually:")
	fmt.Println("  go build -o bin/service main.go")
}

// cmdTest 运行测试。
func cmdTest(args []string) {
	fmt.Println("Running tests...")
	fmt.Println("(test integration coming soon)")
	fmt.Println("")
	fmt.Println("Run the following to test manually:")
	fmt.Println("  go test ./...")
}

// cmdLint 代码检查。
func cmdLint(args []string) {
	fmt.Println("Running linter...")
	fmt.Println("(lint integration coming soon)")
	fmt.Println("")
	fmt.Println("Run the following to lint manually:")
	fmt.Println("  go vet ./...")
}
