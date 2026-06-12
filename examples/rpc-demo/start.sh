#!/bin/bash
# =============================================================================
# Nexus Micro RPC Demo — 一键启动脚本
# 启动 etcd → user/home/payment (gRPC) → HTTP+RPC 双协议网关
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
LOG_DIR="$SCRIPT_DIR/logs"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; BLUE='\033[0;34m'; NC='\033[0m'

info()  { echo -e "${BLUE}[start]${NC} $*"; }
ok()    { echo -e "${GREEN}[start] ${NC} $*"; }
warn()  { echo -e "${YELLOW}[start] ${NC} $*"; }
err()   { echo -e "${RED}[start] ${NC} $*"; }

mkdir -p "$LOG_DIR"

PID_ETCD="$LOG_DIR/etcd.pid"
PID_USVC1="$LOG_DIR/user-svc-50051.pid"
PID_USVC2="$LOG_DIR/user-svc-50054.pid"
PID_HSVC="$LOG_DIR/home-svc.pid"
PID_PSVC="$LOG_DIR/payment-svc.pid"
PID_GW="$LOG_DIR/gateway.pid"

cleanup() {
    info "shutting down..."
    for pf in "$PID_GW" "$PID_PSVC" "$PID_HSVC" "$PID_USVC2" "$PID_USVC1" "$PID_ETCD"; do
        if [ -f "$pf" ]; then
            pid=$(cat "$pf"); kill -0 "$pid" 2>/dev/null && kill "$pid" 2>/dev/null || true
            rm -f "$pf"
        fi
    done
    ok "all services stopped"
}
trap cleanup EXIT INT TERM

# 清理端口：gRPC 50051/50052/50053/50054，HTTP 8889，gRPC 8890
clear_ports() {
    for port in 50051 50052 50053 50054 8889 8890; do
        if lsof -ti ":$port" &>/dev/null; then
            warn "port $port in use, killing..."
            lsof -ti ":$port" | xargs kill -9 2>/dev/null || true
            sleep 0.5
        fi
    done
}

start_etcd() {
    if command -v etcd &>/dev/null; then
        info "starting etcd..."
        etcd --name=etcd-demo --data-dir="$LOG_DIR/etcd-data" \
            --listen-client-urls=http://0.0.0.0:2379 --advertise-client-urls=http://localhost:2379 \
            --listen-peer-urls=http://0.0.0.0:2380 --initial-advertise-peer-urls=http://localhost:2380 \
            --initial-cluster=etcd-demo=http://localhost:2380 > "$LOG_DIR/etcd.log" 2>&1 &
        echo $! > "$PID_ETCD"; sleep 2; ok "etcd started"
    elif docker info &>/dev/null 2>&1; then
        info "starting etcd (docker)..."
        docker rm -f etcd-demo 2>/dev/null || true
        docker run -d --name etcd-demo -p 2379:2379 -p 2380:2380 \
            quay.io/coreos/etcd:v3.5.18 etcd --name=etcd-demo --data-dir=/etcd-data \
            --listen-client-urls=http://0.0.0.0:2379 --advertise-client-urls=http://localhost:2379 \
            --listen-peer-urls=http://0.0.0.0:2380 --initial-advertise-peer-urls=http://localhost:2380 \
            --initial-cluster=etcd-demo=http://localhost:2380 > "$LOG_DIR/etcd.log" 2>&1
        ok "etcd started (docker)"
    else
        err "etcd not found"; exit 1
    fi
    info "waiting for etcd..."
    for i in $(seq 1 30); do
        curl -s http://localhost:2379/health >/dev/null 2>&1 && ok "etcd ready" && return 0
        sleep 1
    done
    err "etcd timeout"; exit 1
}

build_if_needed() {
    if [ ! -x "$BIN_DIR/user-service-rpc" ] || [ ! -x "$BIN_DIR/gateway-rpc" ] ||
       [ ! -x "$BIN_DIR/home-service-rpc" ] || [ ! -x "$BIN_DIR/payment-service-rpc" ]; then
        info "building..."; bash "$SCRIPT_DIR/build.sh"
    fi
}

start_user_services() {
    build_if_needed
    info "starting user-service (grpc :50051)..."
    GRPC_PORT=50051 "$BIN_DIR/user-service-rpc" > "$LOG_DIR/user-svc-50051.log" 2>&1 &
    echo $! > "$PID_USVC1"; ok "user-service-1 (grpc :50051)"
    info "starting user-service (grpc :50054)..."
    GRPC_PORT=50054 "$BIN_DIR/user-service-rpc" > "$LOG_DIR/user-svc-50054.log" 2>&1 &
    echo $! > "$PID_USVC2"; ok "user-service-2 (grpc :50054)"
}

start_home_service() {
    info "starting home-service (grpc :50052)..."
    GRPC_PORT=50052 "$BIN_DIR/home-service-rpc" > "$LOG_DIR/home-svc.log" 2>&1 &
    echo $! > "$PID_HSVC"; ok "home-service (grpc :50052)"
}

start_payment_service() {
    info "starting payment-service (grpc :50053)..."
    GRPC_PORT=50053 "$BIN_DIR/payment-service-rpc" > "$LOG_DIR/payment-svc.log" 2>&1 &
    echo $! > "$PID_PSVC"; ok "payment-service (grpc :50053)"
}

start_gateway() {
    info "starting gateway (HTTP :8889 + gRPC :8890)..."
    cd "$SCRIPT_DIR/gateway"
    "$BIN_DIR/gateway-rpc" > "$LOG_DIR/gateway.log" 2>&1 &
    echo $! > "$PID_GW"
    sleep 3
    # 等待 HTTP 就绪
    ok "gateway (HTTP :8889, gRPC :8890)"
}

print_status() {
    echo ""
    echo "======================================"
    echo -e "  ${GREEN}Nexus Micro RPC Demo${NC}"
    echo "======================================"
    echo ""
    echo "  etcd:             http://localhost:2379"
    echo "  user-service-1:   grpc localhost:50051"
    echo "  user-service-2:   grpc localhost:50054"
    echo "  home-service:     grpc localhost:50052"
    echo "  payment-service:  grpc localhost:50053"
    echo "  gateway HTTP:     http://localhost:8889"
    echo "  gateway gRPC:     grpc localhost:8890"
    echo ""
    echo "======================================"
    echo "  HTTP REST 测试 (curl)"
    echo "======================================"
    echo ""
    echo "  # 健康检查"
    echo "  curl http://localhost:8889/health"
    echo ""
    echo "  # 创建用户"
    echo "  curl -X POST http://localhost:8889/api/v1/users \\"
    echo "    -H \"Content-Type: application/json\" \\"
    echo "    -d '{\"name\":\"test\",\"email\":\"t@t.com\"}'"
    echo ""
    echo "  # 查询用户"
    echo "  curl http://localhost:8889/api/v1/users/1"
    echo ""
    echo "  # 首页"
    echo "  curl http://localhost:8889/api/v1/home"
    echo ""
    echo "  # 创建支付"
    echo "  curl -X POST http://localhost:8889/api/v1/payments \\"
    echo "    -H \"Content-Type: application/json\" \\"
    echo "    -d '{\"order_id\":1,\"amount\":99.9,\"method\":\"card\"}'"
    echo ""
    echo "  # 查询支付"
    echo "  curl http://localhost:8889/api/v1/payments/1"
    echo ""
    echo "======================================"
    echo "  gRPC 测试 (grpcurl)"
    echo "======================================"
    echo ""
    echo "  # 列出所有服务"
    echo "  grpcurl -plaintext localhost:8890 list"
    echo ""
    echo "  # 获取首页"
    echo "  grpcurl -plaintext localhost:8890 home.HomeService/GetHome"
    echo ""
    echo "  # 创建用户"
    echo "  grpcurl -plaintext -d '{\"name\":\"test\",\"email\":\"t@t.com\"}' \\"
    echo "    localhost:8890 user.UserService/CreateUser"
    echo ""
    echo "Press Ctrl+C to stop all services."
}

# ========== Main ==========
clear_ports
start_etcd
start_user_services
start_home_service
start_payment_service
start_gateway
print_status

wait
