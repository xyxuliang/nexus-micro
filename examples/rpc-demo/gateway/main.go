// apigw-rpc — Nexus Micro RPC 网关示例（HTTP REST + gRPC 双协议）。
//
// HTTP: 对外提供 REST API (:8889)，curl 即可调用。
// gRPC: 对外提供 gRPC 接口 (:8890)，grpcurl 调用。
//
// 内部通过 etcd 发现后端 gRPC 服务，连接池 + 轮询负载均衡。
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/config"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/proxy"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/router"
)

func main() {
	// ---- 1. 加载配置 ----
	cfg, err := config.Load("etc/gateway.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// ---- 2. 连接 etcd ----
	reg, err := registry.NewEtcdRegistry(cfg.Etcd.Endpoints, 5*time.Second)
	if err != nil {
		log.Fatalf("connect etcd: %v", err)
	}
	defer reg.Close()
	log.Println("[gateway] connected to etcd")

	// ---- 3. 创建 gRPC 连接池 ----
	pools := &pool.Services{
		User:    pool.New("user-service", reg),
		Home:    pool.New("home-service", reg),
		Payment: pool.New("payment-service", reg),
	}

	ctx := context.Background()
	for name, p := range map[string]*pool.GrpcPool{
		"user-service":    pools.User,
		"home-service":    pools.Home,
		"payment-service": pools.Payment,
	} {
		if err := p.Start(ctx); err != nil {
			log.Printf("[gateway] pool %s: %v", name, err)
		}
	}
	defer func() {
		pools.User.Close()
		pools.Home.Close()
		pools.Payment.Close()
	}()

	var wg sync.WaitGroup

	// ---- 4. 启动 HTTP REST 服务 (:8889) ----
	httpAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	router.Register(r, pools)

	httpSrv := &http.Server{
		Addr:         httpAddr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("[gateway] HTTP REST listening on %s", httpAddr)
		log.Println("[gateway] routes:")
		for _, ri := range r.Routes() {
			log.Printf("  %-6s %s", ri.Method, ri.Path)
		}
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP serve: %v", err)
		}
	}()

	// ---- 5. 启动 gRPC 服务 (:8890) ----
	grpcAddr := fmt.Sprintf(":%d", cfg.Server.GrpcPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen gRPC: %v", err)
	}

	srv := grpc.NewServer()
	reflection.Register(srv)
	proxy.Register(srv, pools)

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("[gateway] gRPC listening on %s", grpcAddr)
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("gRPC serve: %v", err)
		}
	}()

	// ---- 6. 优雅关闭 ----
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("[gateway] shutting down...")
		srv.GracefulStop()
		httpSrv.Shutdown(context.Background())
	}()

	log.Println("[gateway] ready: HTTP :8889 | gRPC :8890")
	log.Println("")

	wg.Wait()
}
