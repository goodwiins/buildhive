package proxy_test

import (
	"context"
	"testing"

	"github.com/buildhive/buildhive/internal/proxy"
)

func TestNewProxy(t *testing.T) {
	called := false
	director := proxy.BuildkitDirector(func(ctx context.Context) (string, error) {
		called = true
		return "localhost:1234", nil
	})
	p := proxy.New(director)
	if p == nil {
		t.Fatal("New() returned nil")
	}
	h := p.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}
	// Director is not called until a stream arrives
	if called {
		t.Error("director should not be called during construction")
	}
}
