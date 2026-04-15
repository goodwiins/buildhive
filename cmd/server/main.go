package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/buildhive/buildhive/internal/api"
	"github.com/buildhive/buildhive/internal/auth"
	"github.com/buildhive/buildhive/internal/proxy"
	"github.com/buildhive/buildhive/internal/store"
	"google.golang.org/grpc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := mustEnv("DATABASE_URL")
	adminToken := mustEnv("BUILDHIVE_ADMIN_TOKEN")
	port := envOr("PORT", "8080")
	grpcPort := envOr("GRPC_PORT", "8765")

	s, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer s.Close()

	adminHash := auth.HashToken(adminToken)

	httpSrv := api.New(api.Config{AdminTokenHash: adminHash}, s)

	// HTTP server
	httpAddr := fmt.Sprintf(":%s", port)
	go func() {
		log.Printf("HTTP listening on %s", httpAddr)
		if err := http.ListenAndServe(httpAddr, httpSrv); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// gRPC proxy
	grpcAddr := fmt.Sprintf(":%s", grpcPort)
	p := proxy.New(proxy.BuildkitDirector(func(ctx context.Context) (string, error) {
		builders, err := s.GetHealthyBuilders(ctx)
		if err != nil || len(builders) == 0 {
			return "", fmt.Errorf("no healthy builders")
		}
		return builders[0].Address, nil
	}))
	grpcSrv := grpc.NewServer(
		grpc.UnknownServiceHandler(p.Handler()),
	)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen grpc: %v", err)
	}
	go func() {
		log.Printf("gRPC proxy listening on %s", grpcAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("grpc: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	grpcSrv.GracefulStop()
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
