package agent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buildhive/buildhive/internal/agent"
)

func TestHeartbeatSendsCorrectPayload(t *testing.T) {
	var called int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("invalid JSON payload: %v", err)
		}
		if payload["name"] != "test-builder" {
			t.Errorf("name: got %v, want test-builder", payload["name"])
		}
		if payload["address"] != "192.168.1.10:1234" {
			t.Errorf("address: got %v, want 192.168.1.10:1234", payload["address"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hb := agent.NewHeartbeater(agent.HeartbeatConfig{
		ServerURL:    srv.URL,
		AgentName:    "test-builder",
		BuildkitAddr: "192.168.1.10:1234",
		Interval:     50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	hb.Run(ctx)

	if atomic.LoadInt32(&called) < 2 {
		t.Errorf("expected at least 2 HTTP calls (register + heartbeat), got %d", called)
	}
}

func TestHeartbeatSurvivesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	hb := agent.NewHeartbeater(agent.HeartbeatConfig{
		ServerURL:    srv.URL,
		AgentName:    "test-builder",
		BuildkitAddr: "127.0.0.1:1234",
		Interval:     50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Should not panic even though server returns 500
	hb.Run(ctx)
}
