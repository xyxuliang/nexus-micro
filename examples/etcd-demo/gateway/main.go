// apigw — Nexus Micro API 网关示例
//
// 通过 etcd 发现上游服务，HTTP 长连接池 + 负载均衡 + 熔断 + 重试。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/core/balancer"
	"github.com/xyxuliang/nexus-micro/core/client"
	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/gateway/internal/config"
	mw "github.com/xyxuliang/nexus-micro/examples/etcd-demo/gateway/internal/middleware"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/gateway/internal/proxy"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/gateway/internal/router"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("etc/gateway.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 2. etcd 服务发现
	reg, err := registry.NewEtcdRegistry(cfg.Etcd.Endpoints, 5*time.Second)
	if err != nil {
		log.Fatalf("connect etcd: %v", err)
	}
	defer reg.Close()
	log.Println("[gateway] connected to etcd")

	// 3. 为每个上游服务创建长连接池（去重）
	pooledClients := make(map[string]*client.PooledClient)
	seen := make(map[string]bool)
	for _, rt := range cfg.Routes {
		if seen[rt.Upstream] {
			continue
		}
		seen[rt.Upstream] = true

		cli := client.NewPooled(rt.Upstream,
			client.PooledWithRegistry(reg),
			client.PooledWithBalancer(balancer.New(&balancer.Config{Strategy: balancer.RoundRobin})),
			client.PooledWithTimeout(cfg.Proxy.Timeout),
			client.PooledWithRetries(cfg.Proxy.Retries, 100*time.Millisecond),
		)
		if err := cli.Start(context.Background()); err != nil {
			log.Printf("[gateway] warn: %s: %v", rt.Upstream, err)
			continue
		}
		pooledClients[rt.Upstream] = cli
		log.Printf("[gateway] pooled client ready: %s", rt.Upstream)
	}
	defer func() {
		for _, cli := range pooledClients {
			cli.Close()
		}
	}()

	// 4. 启动 Gin 服务
	r := gin.New()
	r.Use(mw.RequestID(), mw.CORS(), mw.Timeout(cfg.Proxy.Timeout), gin.Recovery())

	ph := proxy.New(pooledClients)
	router.Register(r, cfg, ph)

	// 5. 优雅关闭
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("[gateway] shutting down...")
		os.Exit(0)
	}()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("[gateway] listening on %s", addr)

	srv := &http.Server{
		Addr: addr, Handler: r,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[gateway] server error: %v", err)
		}
	}()

	wg.Wait()
	srv.Shutdown(context.Background())
}
