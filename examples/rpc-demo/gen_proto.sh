#!/bin/bash
# =============================================================================
# Nexus Micro RPC Demo — .proto 代码生成脚本
# 从 proto/*.proto 生成 Go 类型定义和 gRPC stub
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NEXUS_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[gen]${NC} $*"; }
ok()    { echo -e "${GREEN}[gen] ${NC} $*"; }
err()   { echo -e "${RED}[gen] ${NC} $*"; }

# protoc + 插件检查
require_protoc() {
    if ! command -v protoc &>/dev/null; then
        err "protoc not found. Install: brew install protobuf"
        exit 1
    fi
    if ! command -v protoc-gen-go &>/dev/null; then
        err "protoc-gen-go not found. Install: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
        exit 1
    fi
    if ! command -v protoc-gen-go-grpc &>/dev/null; then
        err "protoc-gen-go-grpc not found. Install: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
        exit 1
    fi
}

# 模块前缀（与 go_package 对应）
MODULE="github.com/xyxuliang/nexus-micro/examples/rpc-demo"

echo ""
echo "======================================"
echo -e "  ${GREEN}Nexus Micro RPC Code Generation${NC}"
echo "======================================"
echo ""

require_protoc

info "generating protobuf + gRPC stubs..."
cd "$SCRIPT_DIR"

protoc \
    --proto_path=proto \
    --go_out=. \
    --go_opt=module="$MODULE" \
    --go-grpc_out=. \
    --go-grpc_opt=module="$MODULE" \
    proto/user.proto proto/home.proto proto/payment.proto

ok "generation complete"
echo ""
info "generated files:"
find pkg/pb -name "*.go" -type f | sort | while read f; do
    echo "  pkg/pb/${f#pkg/pb/}"
done
