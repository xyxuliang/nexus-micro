package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/config"
	mw "github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/middleware"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/router"
	"github.com/xyxuliang/nexus-micro/examples/etcd-demo/home-service/internal/svc"
)

func main() {
	cfg, err := config.Load("etc/home.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if ps := os.Getenv("PORT"); ps != "" {
		if p, err := strconv.Atoi(ps); err == nil {
			cfg.Server.Port = p
		}
	}

	ctx := svc.New(cfg)

	inst := &registry.ServiceInstance{
		ID:   fmt.Sprintf("%s-%d", cfg.Server.Name, time.Now().UnixNano()),
		Name: cfg.Server.Name, Version: "1.0.0",
		Metadata:  map[string]string{"weight": "100"},
		Endpoints: []string{fmt.Sprintf("localhost:%d", cfg.Server.Port)},
	}
	reg, err := registry.NewEtcdRegistry(cfg.Etcd.Endpoints, 5*time.Second)
	if err != nil {
		log.Fatalf("connect etcd: %v", err)
	}
	defer reg.Close()
	if err := reg.Register(context.Background(), inst); err != nil {
		log.Fatalf("register: %v", err)
	}
	log.Printf("[%s] registered to etcd (%s)", cfg.Server.Name, inst.Endpoints[0])

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		reg.Deregister(context.Background(), inst)
		os.Exit(0)
	}()

	r := gin.New()
	r.Use(mw.RequestID(), mw.CORS(), mw.Log(), gin.Recovery())
	router.Register(r, ctx)

	log.Printf("[%s] listening on :%d", cfg.Server.Name, cfg.Server.Port)
	for _, ri := range r.Routes() {
		log.Printf("  %-6s %s", ri.Method, ri.Path)
	}

	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Server.Port), Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	wg.Wait()
	srv.Shutdown(context.Background())
}
