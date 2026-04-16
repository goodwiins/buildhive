package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/buildhive/buildhive/internal/agent"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cacheRoot := envOr("CACHE_ROOT", "/var/lib/buildkit")
	serverURL := mustEnv("BUILDHIVE_SERVER_URL")
	agentName := envOr("AGENT_NAME", hostname())
	buildkitAddr := mustEnv("BUILDKIT_PUBLIC_ADDR")

	// Start buildkitd supervisor
	mgr := agent.NewManager(agent.DefaultBuildkitdConfig(cacheRoot))
	go mgr.Run(ctx)

	// Give buildkitd a moment to start
	time.Sleep(2 * time.Second)

	hb := agent.NewHeartbeater(agent.HeartbeatConfig{
		ServerURL:    serverURL,
		AgentName:    agentName,
		BuildkitAddr: buildkitAddr,
		CacheRoot:    cacheRoot,
		Interval:     10 * time.Second,
	})

	log.Printf("buildhive-agent running (name=%s, buildkit=%s, cache=%s)", agentName, buildkitAddr, cacheRoot)
	go hb.Run(ctx)

	<-ctx.Done()
	log.Println("shutting down")
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

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
