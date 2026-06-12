package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/payment-service/internal/server"
)

type config struct {
	Server struct {
		Name     string `yaml:"name"`
		GrpcPort int    `yaml:"grpc_port"`
	} `yaml:"server"`
	Etcd struct {
		Endpoints []string `yaml:"endpoints"`
		TTL       int      `yaml:"ttl"`
	} `yaml:"etcd"`
}

func main() {
	cfg := &config{}
	cfg.Server.Name = "payment-service"
	cfg.Server.GrpcPort = 50053
	cfg.Etcd.Endpoints = []string{"localhost:2379"}
	cfg.Etcd.TTL = 30

	if ps := os.Getenv("GRPC_PORT"); ps != "" {
		if p, err := strconv.Atoi(ps); err == nil {
			cfg.Server.GrpcPort = p
		}
	}

	inst := &registry.ServiceInstance{
		ID:      fmt.Sprintf("%s-%d", cfg.Server.Name, time.Now().UnixNano()),
		Name:    cfg.Server.Name,
		Version: "1.0.0",
		Metadata: map[string]string{
			"weight":   "100",
			"protocol": "grpc",
		},
		Endpoints: []string{fmt.Sprintf("localhost:%d", cfg.Server.GrpcPort)},
	}

	reg, err := registry.NewEtcdRegistry(cfg.Etcd.Endpoints, 5*time.Second)
	if err != nil {
		log.Fatalf("[%s] connect etcd: %v", cfg.Server.Name, err)
	}
	defer reg.Close()

	if err := reg.Register(context.Background(), inst); err != nil {
		log.Fatalf("[%s] register: %v", cfg.Server.Name, err)
	}
	log.Printf("[%s] registered to etcd (grpc %s)", cfg.Server.Name, inst.Endpoints[0])

	addr := fmt.Sprintf(":%d", cfg.Server.GrpcPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[%s] listen tcp: %v", cfg.Server.Name, err)
	}

	srv := grpc.NewServer()
	server.Register(srv)
	reflection.Register(srv)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Printf("[%s] shutting down...", cfg.Server.Name)
		reg.Deregister(context.Background(), inst)
		srv.GracefulStop()
	}()

	log.Printf("[%s] gRPC listening on %s", cfg.Server.Name, addr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("[%s] gRPC serve: %v", cfg.Server.Name, err)
	}
	wg.Wait()
}
