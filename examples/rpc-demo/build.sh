#!/bin/bash
# =============================================================================
# Nexus Micro RPC Demo — 构建脚本
# 编译 user-service、home-service、payment-service（gRPC）和 gateway
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NEXUS_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[build]${NC} $*"; }
ok()    { echo -e "${GREEN}[build] ${NC} $*"; }
err()   { echo -e "${RED}[build] ${NC} $*"; }

mkdir -p "$BIN_DIR"

build_svc() {
    local name="$1"; local pkg="$2"
    info "building $name..."
    if go build -o "$BIN_DIR/$name" "$pkg"; then
        ok "$name → bin/$name"
    else
        err "$name build failed"
        exit 1
    fi
}

cd "$NEXUS_DIR"

build_svc "user-service-rpc"    "./examples/rpc-demo/user-service/"
build_svc "home-service-rpc"    "./examples/rpc-demo/home-service/"
build_svc "payment-service-rpc" "./examples/rpc-demo/payment-service/"
build_svc "gateway-rpc"         "./examples/rpc-demo/gateway/"

echo ""
info "build complete:"
ls -lh "$BIN_DIR/"
