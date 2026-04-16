package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// HeartbeatConfig configures the agent heartbeat.
type HeartbeatConfig struct {
	ServerURL    string
	AgentName    string
	BuildkitAddr string        // public host:port for buildkitd
	CacheRoot    string        // disk path to monitor
	Interval     time.Duration // default 10s
}

// Heartbeater sends periodic health reports to the control plane.
type Heartbeater struct {
	cfg    HeartbeatConfig
	client *http.Client
}

// NewHeartbeater creates a Heartbeater.
func NewHeartbeater(cfg HeartbeatConfig) *Heartbeater {
	if cfg.Interval == 0 {
		cfg.Interval = 10 * time.Second
	}
	return &Heartbeater{cfg: cfg, client: &http.Client{Timeout: 5 * time.Second}}
}

type heartbeatPayload struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Arch    string  `json:"arch"`
	Status  string  `json:"status"`
	CPUPct  float64 `json:"cpu_pct"`
	MemPct  float64 `json:"mem_pct"`
	DiskPct float64 `json:"disk_pct"`
}

// Run registers the builder once, then sends heartbeats until ctx is cancelled.
func (h *Heartbeater) Run(ctx context.Context) {
	// Initial registration
	if err := h.register(ctx); err != nil {
		log.Printf("initial registration failed: %v", err)
	}

	ticker := time.NewTicker(h.cfg.Interval)
	defer ticker.Stop()

	for {
		if err := h.send(ctx, "/api/builders/heartbeat"); err != nil {
			log.Printf("heartbeat error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Heartbeater) register(ctx context.Context) error {
	return h.send(ctx, "/api/builders/register")
}

func (h *Heartbeater) send(ctx context.Context, path string) error {
	payload := h.collect()
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.cfg.ServerURL+path,
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// collect gathers current metrics for the heartbeat payload.
func (h *Heartbeater) collect() heartbeatPayload {
	p := heartbeatPayload{
		Name:    h.cfg.AgentName,
		Address: h.cfg.BuildkitAddr,
		Arch:    runtime.GOARCH,
		Status:  "healthy",
	}
	if cpus, err := cpu.Percent(0, false); err == nil && len(cpus) > 0 {
		p.CPUPct = cpus[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		p.MemPct = vm.UsedPercent
	}
	if h.cfg.CacheRoot != "" {
		if d, err := disk.Usage(h.cfg.CacheRoot); err == nil {
			p.DiskPct = d.UsedPercent
			if d.UsedPercent > 90 {
				p.Status = "disk_pressure"
			}
		}
	}
	return p
}
