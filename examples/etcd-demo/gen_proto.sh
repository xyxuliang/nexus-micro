#!/bin/bash
# =============================================================================
# Nexus Micro etcd Demo — .api DSL 代码生成脚本
# =============================================================================
#
# 功能说明:
#   从 .api DSL 文件自动生成服务完整代码：
#     - DTO（数据传输对象，Go 结构体，JSON tag）
#     - Handler（HTTP + gRPC 处理函数框架）
#     - Router（Gin 路由注册，含健康检查端点）
#     - Logic（业务逻辑骨架）
#     - Config（服务配置结构体 + 默认值）
#     - ServiceContext（依赖注入容器）
#     - Middleware（中间件链：CORS/RequestID/Log/Recovery/Timeout）
#     - Client（客户端 SDK）
#     - OpenAPI v3 文档
#     - Protobuf 定义文件
#     - Main 入口
#     - 默认 YAML 配置文件
#
# 使用方法:
#   ./gen_proto.sh           # 从 .api 文件生成所有服务代码
#
# 输入文件:
#   user-service/user.api        -> 生成 service/user/
#   home-service/home.api        -> 生成 service/home/
#   payment-service/payment.api  -> 生成 service/payment/
#
# 输出目录结构 (示例: service/user/):
#   service/user/
#     ├── main.go                  # 服务入口，含 etcd 注册 + 优雅关闭
#     ├── etc/user.yaml            # 默认 YAML 配置文件
#     ├── client/                  # 客户端 SDK
#     ├── docs/                    # OpenAPI v3 文档
#     ├── user.proto               # Protobuf 定义（gRPC）
#     └── internal/
#         ├── config/config.go     # 配置结构体
#         ├── dto/                 # 数据传输对象 (每个 type 一个文件)
#         ├── handler/             # Handler 层 (每个 handler 一个文件)
#         ├── logic/               # 业务逻辑层 (每个 handler 一个文件)
#         ├── middleware/          # 中间件链
#         ├── router/              # 路由注册
#         └── svc/                 # 依赖注入容器
#
# .api DSL 语法参考:
#   info ( title: "My Service" desc: "..." version: "1.0.0" )
#   type ( MyType { Id int64 Name string } )
#   @server( prefix: "/api/v1" service: myservice )
#   service MyService {
#       @handler MyHandler
#       @doc "处理描述"
#       post /path/to (RequestType) returns (ResponseType)
#   }
#
# 前置条件:
#   - Go 1.25+ 已安装
#   - .api 文件已编写完毕
#   - 项目 go.mod 已配置 (module github.com/xyxuliang/nexus-micro)
#
# 与其他脚本的关系:
#   - gen_proto.sh -> 生成代码（创建服务骨架）
#   - build.sh     -> 编译服务（生成二进制文件）
#   - start.sh     -> 启动集群（运行 etcd + 全部服务）
# =============================================================================

set -euo pipefail

# ---- 路径定义 ----
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NEXUS_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
NX_BIN="$NEXUS_DIR/bin/nx"

# ---- ANSI 颜色码 ----
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m'

# ---- 日志函数 ----
info()  { echo -e "${BLUE}[gen]${NC} $*"; }
ok()    { echo -e "${GREEN}[gen] ✓${NC} $*"; }
warn()  { echo -e "${YELLOW}[gen] ⚠${NC} $*"; }
err()   { echo -e "${RED}[gen] ✗${NC} $*"; }

# =============================================================================
# build_nx — 构建 nx CLI 工具
# 检查 bin/nx 是否存在，不存在则从 cmd/nx/ 编译
# =============================================================================
build_nx() {
    if [ -x "$NX_BIN" ]; then
        info "nx binary found, skipping build"
        return 0
    fi

    info "building nx CLI..."
    cd "$NEXUS_DIR"
    mkdir -p "$NEXUS_DIR/bin"
    if go build -o "$NX_BIN" ./cmd/nx/; then
        ok "nx CLI -> bin/nx"
    else
        err "nx CLI build failed"
        exit 1
    fi
}

# =============================================================================
# gen_service — 从 .api 文件生成单个服务代码
# 参数: $1=.api文件路径, $2=服务名称
# =============================================================================
gen_service() {
    local api_file="$1"
    local svc_name="$2"

    info "generating $svc_name from $(basename "$api_file")..."
    cd "$SCRIPT_DIR"
    if "$NX_BIN" gen api "$api_file"; then
        ok "$svc_name code generated"
    else
        err "$svc_name code generation failed"
        exit 1
    fi
}

# =============================================================================
# format_code — 格式化生成的 Go 代码
# 使用 gofmt -w 原地格式化所有 .go 文件
# =============================================================================
format_code() {
    local dir="$1"
    info "formatting $dir..."
    if command -v gofmt >/dev/null 2>&1; then
        find "$dir" -name "*.go" -exec gofmt -w {} \;
        local count
        count=$(find "$dir" -name "*.go" | wc -l | tr -d ' ')
        ok "formatted $count Go files"
    else
        warn "gofmt not found, skipping format"
    fi
}

# =============================================================================
# Main
# =============================================================================
echo ""
echo "======================================"
echo -e "  ${GREEN}Nexus Micro Code Generation${NC}"
echo "======================================"
echo ""

# 1. 构建 nx CLI
build_nx

# 2. 生成各服务代码
gen_service "$SCRIPT_DIR/user-service/user.api"      "user-service"
gen_service "$SCRIPT_DIR/home-service/home.api"       "home-service"
gen_service "$SCRIPT_DIR/payment-service/payment.api" "payment-service"

# 3. 格式化
format_code "$SCRIPT_DIR/service"

# 4. 提示
echo ""
info "generation complete. Generated code is in $SCRIPT_DIR/service/"
echo ""
echo "Integration guide:"
echo "  cp -r $SCRIPT_DIR/service/user/*    $SCRIPT_DIR/user-service/"
echo "  cp -r $SCRIPT_DIR/service/home/*    $SCRIPT_DIR/home-service/"
echo "  cp -r $SCRIPT_DIR/service/payment/* $SCRIPT_DIR/payment-service/"
