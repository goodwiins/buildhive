package agent_test

import (
	"testing"

	"github.com/buildhive/buildhive/internal/agent"
)

func TestDefaultBuildkitdConfig(t *testing.T) {
	cfg := agent.DefaultBuildkitdConfig("/data/buildkit")
	if cfg.Root != "/data/buildkit" {
		t.Errorf("Root: got %q, want /data/buildkit", cfg.Root)
	}
	if cfg.Addr == "" {
		t.Error("Addr should not be empty")
	}
}

func TestNewManager(t *testing.T) {
	cfg := agent.DefaultBuildkitdConfig("/tmp/buildkit")
	m := agent.NewManager(cfg)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.Addr() == "" {
		t.Error("Addr() should return non-empty")
	}
}
