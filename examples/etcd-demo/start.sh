#!/bin/bash
# =============================================================================
# Nexus Micro etcd Demo — 一键启动脚本
# =============================================================================
#
# 功能说明:
#   按依赖顺序依次启动整个 etcd-demo 微服务集群：
#     etcd -> user-service(2实例) -> home-service -> payment-service -> gateway
#
#   启动后可通过 Ctrl+C 一键停止所有服务（自动清理 PID 文件和残留进程）。
#
# 使用方法:
#   ./start.sh               # 前台启动，Ctrl+C 停止
#
# 集群拓扑:
#   ┌─────────┐
#   │  etcd   │  :2379  <-- 注册中心 (服务发现)
#   └────┬────┘
#        │
#   ┌────┴──────────────────────────────────┐
#   │             服务注册                    │
#   ├──────────┬──────────┬────────┬───────┤
#   │  user-1  │  user-2  │  home  │payment│
#   │  :8081   │  :8082   │ :8083  │ :8084 │
#   └────┬─────┴────┬─────┴───┬────┴───┬───┘
#        └──────────┴─────────┴────────┘
#                          │
#                    ┌─────┴──────┐
#                    │  gateway   │  :8888  <-- 统一入口
#                    └────────────┘
#
# 端口规划:
#   2379 — etcd 客户端端口
#   2380 — etcd 对等端口
#   8081 — user-service 实例 1
#   8082 — user-service 实例 2
#   8083 — home-service
#   8084 — payment-service
#   8888 — API 网关
#
# 前置条件:
#   - etcd v3.x 二进制在 PATH 中，或 Docker 可用
#   - Go 已安装 (自动调用 build.sh 编译)
#   - 端口 2379,2380,8081-8084,8888 未被占用（脚本会自动清理）
#
# 日志与 PID 文件:
#   logs/etcd.log              — etcd 运行日志
#   logs/user-svc-8081.log     — user-service 实例1 日志
#   logs/user-svc-8082.log     — user-service 实例2 日志
#   logs/home-svc.log          — home-service 日志
#   logs/payment-svc.log       — payment-service 日志
#   logs/gateway.log           — gateway 日志
#   logs/*.pid                 — 各进程 PID 文件
#   logs/etcd-data/            — etcd 数据目录
# =============================================================================

# -e: 任何命令失败立即退出
# -u: 使用未定义变量时退出
# -o pipefail: 管道中任何命令失败都视为失败
set -euo pipefail

# ---------------------------------------------------------------------------
# 路径定义
# ---------------------------------------------------------------------------
# SCRIPT_DIR: 脚本所在目录 (examples/etcd-demo/)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# BIN_DIR: 编译后的二进制文件目录 (由 build.sh 产生)
BIN_DIR="$SCRIPT_DIR/bin"

# LOG_DIR: 运行日志和 PID 文件目录
LOG_DIR="$SCRIPT_DIR/logs"

# ---------------------------------------------------------------------------
# ANSI 颜色码 — 终端彩色输出
# ---------------------------------------------------------------------------
RED='\033[0;31m'     # 错误 / 失败
GREEN='\033[0;32m'   # 成功 / 就绪
YELLOW='\033[0;33m'  # 警告
BLUE='\033[0;34m'    # 进度 / 信息
NC='\033[0m'         # 重置颜色 (No Color)

# ---------------------------------------------------------------------------
# 日志函数 — 统一的彩色日志输出
# ---------------------------------------------------------------------------
info()  { echo -e "${BLUE}[start]${NC} $*"; }    # info
ok()    { echo -e "${GREEN}[start] OK${NC} $*"; } # ok
warn()  { echo -e "${YELLOW}[start] WARN${NC} $*"; } # warn
err()   { echo -e "${RED}[start] ERR${NC} $*"; }   # err

# ---------------------------------------------------------------------------
# 确保日志目录存在
# ---------------------------------------------------------------------------
mkdir -p "$LOG_DIR"

# =============================================================================
# PID 文件路径 — 用于进程管理和优雅关闭
# 每个启动的进程都将其 PID 写入对应的 PID 文件。
# cleanup() 函数通过读取这些文件来终止所有进程。
# =============================================================================
PID_ETCD="$LOG_DIR/etcd.pid"              # etcd 注册中心
PID_USVC1="$LOG_DIR/user-svc-8081.pid"    # user-service 实例 1 (:8081)
PID_USVC2="$LOG_DIR/user-svc-8082.pid"    # user-service 实例 2 (:8082)
PID_HSVC="$LOG_DIR/home-svc.pid"          # home-service (:8083)
PID_PSVC="$LOG_DIR/payment-svc.pid"       # payment-service (:8084)
PID_GW="$LOG_DIR/gateway.pid"             # API 网关 (:8888)

# =============================================================================
# cleanup — 优雅关闭所有服务
# =============================================================================
# 触发时机:
#   - 用户按下 Ctrl+C (SIGINT)
#   - 脚本正常退出 (EXIT)
#   - 收到 SIGTERM 信号
#
# 关闭顺序: gateway -> payment -> home -> user-2 -> user-1 -> etcd
# 从上游到下游依次关闭，避免请求丢失。
# =============================================================================
cleanup() {
    info "shutting down all services..."
    for pid_file in "$PID_GW" "$PID_PSVC" "$PID_HSVC" "$PID_USVC2" "$PID_USVC1" "$PID_ETCD"; do
        if [ -f "$pid_file" ]; then
            pid=$(cat "$pid_file")
            # kill -0 检查进程是否存在（不发送信号）
            if kill -0 "$pid" 2>/dev/null; then
                kill "$pid" 2>/dev/null || true
                info "stopped pid=$pid ($(basename "$pid_file"))"
            fi
            rm -f "$pid_file"
        fi
    done
    ok "all services stopped"
}

# 注册信号处理器，确保 Ctrl+C 或脚本退出时自动清理
trap cleanup EXIT INT TERM

# =============================================================================
# clear_ports — 清理已占用的端口
# =============================================================================
# 启动前检测并释放关键端口，避免 "address already in use" 错误。
# 使用 lsof -ti 查找占用端口的进程并强制终止。
# 覆盖端口: 8081, 8082, 8083, 8084, 8888
# =============================================================================
clear_ports() {
    for port in 8081 8082 8083 8084 8888; do
        if lsof -ti ":$port" &>/dev/null; then
            warn "port $port is in use, killing old process..."
            lsof -ti ":$port" | xargs kill -9 2>/dev/null || true
            sleep 0.5
        fi
    done
}

# =============================================================================
# start_etcd — 启动 etcd 注册中心
# =============================================================================
# 两种启动方式:
#   1. 本地二进制 (etcd 在 PATH 中) — 直接后台启动
#   2. Docker 容器 (本地无 etcd 但 Docker 可用) — 拉取镜像启动
#
# 启动后会轮询 http://localhost:2379/health 等待 etcd 就绪，最多等 30 秒。
# =============================================================================
start_etcd() {
    # 方式一: 本地 etcd 二进制
    if command -v etcd &>/dev/null; then
        info "starting etcd (local binary)..."
        etcd \
            --name=etcd-demo \
            --data-dir="$LOG_DIR/etcd-data" \
            --listen-client-urls=http://0.0.0.0:2379 \
            --advertise-client-urls=http://localhost:2379 \
            --listen-peer-urls=http://0.0.0.0:2380 \
            --initial-advertise-peer-urls=http://localhost:2380 \
            --initial-cluster=etcd-demo=http://localhost:2380 \
            > "$LOG_DIR/etcd.log" 2>&1 &
        echo $! > "$PID_ETCD"
        sleep 2
        ok "etcd started (pid=$(cat "$PID_ETCD"))"

    # 方式二: Docker 容器
    elif docker info &>/dev/null 2>&1; then
        info "starting etcd (docker)..."
        docker rm -f etcd-demo 2>/dev/null || true
        docker run -d --name etcd-demo \
            -p 2379:2379 -p 2380:2380 \
            quay.io/coreos/etcd:v3.5.18 \
            etcd \
            --name=etcd-demo \
            --data-dir=/etcd-data \
            --listen-client-urls=http://0.0.0.0:2379 \
            --advertise-client-urls=http://localhost:2379 \
            --listen-peer-urls=http://0.0.0.0:2380 \
            --initial-advertise-peer-urls=http://localhost:2380 \
            --initial-cluster=etcd-demo=http://localhost:2380 \
            > "$LOG_DIR/etcd.log" 2>&1
        ok "etcd started (docker: etcd-demo)"
    else
        err "etcd not found. Install etcd or Docker first."
        exit 1
    fi

    # 等待 etcd 就绪
    info "waiting for etcd to be ready..."
    for i in $(seq 1 30); do
        if curl -s http://localhost:2379/health >/dev/null 2>&1; then
            ok "etcd is ready"
            return 0
        fi
        sleep 1
    done
    err "etcd did not become ready in 30s"
    exit 1
}

# =============================================================================
# build_if_needed — 按需编译
# =============================================================================
# 检查 bin/ 下的所有必需二进制文件是否存在且可执行。
# 只要有一个缺失，就自动调用 build.sh 编译全部服务。
# =============================================================================
build_if_needed() {
    if [ ! -x "$BIN_DIR/user-service" ] || [ ! -x "$BIN_DIR/gateway" ] || \
       [ ! -x "$BIN_DIR/home-service" ] || [ ! -x "$BIN_DIR/payment-service" ]; then
        info "binaries not found, building..."
        bash "$SCRIPT_DIR/build.sh"
    fi
}

# =============================================================================
# start_user_services — 启动 user-service (多实例)
# =============================================================================
# 启动 2 个 user-service 实例以演示负载均衡:
#   - 实例 1: 端口 8081
#   - 实例 2: 端口 8082
#
# 两个实例共用同一配置 (etc/user.yaml)，通过 PORT 环境变量覆盖端口。
# 每个实例以唯一 ID 注册到 etcd，gateway 通过 etcd 发现并进行轮询负载均衡。
# =============================================================================
start_user_services() {
    build_if_needed

    info "starting user-service (port=8081)..."
    cd "$SCRIPT_DIR/user-service"
    PORT=8081 "$BIN_DIR/user-service" > "$LOG_DIR/user-svc-8081.log" 2>&1 &
    echo $! > "$PID_USVC1"
    ok "user-service-1 started (pid=$(cat "$PID_USVC1"), port=8081)"

    info "starting user-service (port=8082)..."
    PORT=8082 "$BIN_DIR/user-service" > "$LOG_DIR/user-svc-8082.log" 2>&1 &
    echo $! > "$PID_USVC2"
    ok "user-service-2 started (pid=$(cat "$PID_USVC2"), port=8082)"

    wait_for_health 8081 8082
}

# =============================================================================
# start_home_service — 启动 home-service (首页服务)
# =============================================================================
# 端口: 8083
# 功能: 提供首页数据（Banner列表等）
# API:  GET /api/v1/home
# =============================================================================
start_home_service() {
    info "starting home-service (port=8083)..."
    cd "$SCRIPT_DIR/home-service"
    PORT=8083 "$BIN_DIR/home-service" > "$LOG_DIR/home-svc.log" 2>&1 &
    echo $! > "$PID_HSVC"
    ok "home-service started (pid=$(cat "$PID_HSVC"), port=8083)"

    wait_for_health 8083
}

# =============================================================================
# start_payment_service — 启动 payment-service (支付服务)
# =============================================================================
# 端口: 8084
# 功能: 提供支付创建和查询
# API:  POST /api/v1/payments     # 创建支付
#       GET  /api/v1/payments/:id # 查询支付
# =============================================================================
start_payment_service() {
    info "starting payment-service (port=8084)..."
    cd "$SCRIPT_DIR/payment-service"
    PORT=8084 "$BIN_DIR/payment-service" > "$LOG_DIR/payment-svc.log" 2>&1 &
    echo $! > "$PID_PSVC"
    ok "payment-service started (pid=$(cat "$PID_PSVC"), port=8084)"

    wait_for_health 8084
}

# =============================================================================
# wait_for_health — 等待服务健康检查通过
# =============================================================================
# 参数: 可变数量的端口号，如 wait_for_health 8081 8082 8083
#
# 机制: 轮询每个端口的 /health 端点，每个端口最多等 10 秒。
#       如果 10 秒内未通过检查，脚本继续执行（非致命，允许服务延迟就绪）。
# =============================================================================
wait_for_health() {
    sleep 1
    for port in "$@"; do
        for i in $(seq 1 10); do
            if curl -s "http://localhost:$port/health" >/dev/null 2>&1; then
                ok "service :$port is ready"
                break
            fi
            sleep 1
        done
    done
}

# =============================================================================
# start_gateway — 启动 API 网关
# =============================================================================
# 端口: 8888
# 功能: 统一入口，通过 etcd 发现上游服务，进行请求路由和负载均衡。
# 启动后等待 /health 端点就绪（最多 10 秒）。
# =============================================================================
start_gateway() {
    info "starting gateway (port=8888)..."
    cd "$SCRIPT_DIR/gateway"
    "$BIN_DIR/gateway" > "$LOG_DIR/gateway.log" 2>&1 &
    echo $! > "$PID_GW"
    ok "gateway started (pid=$(cat "$PID_GW"), port=8888)"

    for i in $(seq 1 10); do
        if curl -s http://localhost:8888/health >/dev/null 2>&1; then
            ok "gateway is ready"
            break
        fi
        sleep 1
    done
}

# =============================================================================
# print_status — 打印集群状态和测试命令
# =============================================================================
print_status() {
    echo ""
    echo "======================================"
    echo -e "  ${GREEN}Nexus Micro etcd Demo${NC}"
    echo "======================================"
    echo ""
    echo "  etcd:             http://localhost:2379"
    echo "  user-service:     http://localhost:8081"
    echo "  user-service:     http://localhost:8082"
    echo "  home-service:     http://localhost:8083"
    echo "  payment-service:  http://localhost:8084"
    echo "  gateway:          http://localhost:8888"
    echo ""
    echo "  binaries:         $BIN_DIR/"
    echo "  logs:             $LOG_DIR/"
    echo "======================================"
    echo ""
    echo "Test commands:"
    echo ""
    echo "  # 健康检查"
    echo "  curl http://localhost:8888/health"
    echo ""
    echo "  # 用户服务"
    echo "  curl -X POST http://localhost:8888/api/v1/users \\"
    echo "    -H \"Content-Type: application/json\" \\"
    echo "    -d '{\"name\":\"test\",\"email\":\"t@t.com\"}'"
    echo "  curl http://localhost:8888/api/v1/users/1"
    echo ""
    echo "  # 首页服务"
    echo "  curl http://localhost:8888/api/v1/home"
    echo ""
    echo "  # 支付服务"
    echo "  curl -X POST http://localhost:8888/api/v1/payments \\"
    echo "    -H \"Content-Type: application/json\" \\"
    echo "    -d '{\"order_id\":\"o1\",\"amount\":99.9,\"method\":\"card\"}'"
    echo "  curl http://localhost:8888/api/v1/payments/1"
    echo ""
    echo "Press Ctrl+C to stop all services."
}

# =============================================================================
# Main — 主流程
# =============================================================================
# 启动顺序严格按依赖关系:
#   1. 清理端口                   — 避免端口冲突
#   2. 启动 etcd                  — 注册中心必须先就绪
#   3. 启动 user-service (2实例)  — 业务服务
#   4. 启动 home-service          — 业务服务
#   5. 启动 payment-service       — 业务服务
#   6. 启动 gateway               — 网关，依赖 etcd 和服务注册
#   7. 打印状态信息               — 展示测试命令
#   8. wait 保持前台运行          — 等待 Ctrl+C
# =============================================================================
clear_ports          # 清理已占用端口
start_etcd           # 启动注册中心
start_user_services  # 启动用户服务 (2 实例)
start_home_service   # 启动首页服务
start_payment_service # 启动支付服务
start_gateway        # 启动 API 网关
print_status         # 打印集群信息和测试命令

# keep alive — 等待 Ctrl+C 信号，触发 cleanup() 清理所有服务
wait
