package agent

import (
	"context"
	"log"
	"os/exec"
	"time"
)

// BuildkitdConfig holds configuration for the managed buildkitd process.
type BuildkitdConfig struct {
	Root string // persistent cache directory
	Addr string // gRPC listen address (e.g. tcp://0.0.0.0:1234)
}

// DefaultBuildkitdConfig returns a sensible default config.
func DefaultBuildkitdConfig(cacheRoot string) BuildkitdConfig {
	return BuildkitdConfig{
		Root: cacheRoot,
		Addr: "tcp://0.0.0.0:1234",
	}
}

// Manager supervises the buildkitd process with automatic restarts.
type Manager struct {
	cfg BuildkitdConfig
}

// NewManager creates a buildkitd manager.
func NewManager(cfg BuildkitdConfig) *Manager {
	return &Manager{cfg: cfg}
}

// Addr returns the gRPC address buildkitd listens on.
func (m *Manager) Addr() string {
	return m.cfg.Addr
}

// Run starts buildkitd and restarts it if it exits unexpectedly.
// Blocks until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	const maxBackoff = 30 * time.Second
	backoff := time.Second

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		log.Printf("starting buildkitd (root=%s addr=%s)", m.cfg.Root, m.cfg.Addr)
		cmd := exec.CommandContext(ctx,
			"buildkitd",
			"--root", m.cfg.Root,
			"--addr", m.cfg.Addr,
			"--oci-worker-no-process-sandbox",
		)
		cmd.Stdout = log.Writer()
		cmd.Stderr = log.Writer()

		err := cmd.Run()
		if ctx.Err() != nil {
			return // graceful shutdown
		}

		if err != nil {
			log.Printf("buildkitd exited: %v — restarting in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		} else {
			backoff = time.Second
		}
	}
}
