package core

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// collectCounter counts the ticker's collects by the warning each one logs on a
// test firewall, whose nftables connection is nil. Atomic: the ticker logs from
// its own goroutine.
type collectCounter struct{ n *atomic.Int32 }

func (h collectCounter) Enabled(context.Context, slog.Level) bool { return true }
func (h collectCounter) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, "could not read the port counters") {
		h.n.Add(1)
	}
	return nil
}
func (h collectCounter) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h collectCounter) WithGroup(string) slog.Handler      { return h }

func setUsageInterval(cfg *Config, seconds int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Usage.Interval = &seconds
}

// SIGHUP reloads [usage] (Config.Reload, system-settings.md). The ticker used to
// read the interval once: a reload changed nothing, and an interval of 0 at
// start ended the goroutine, so switching the ticker back on took a restart.
func TestUsageTicker_FollowsAReloadedInterval(t *testing.T) {
	var n atomic.Int32
	prev := slog.Default()
	slog.SetDefault(slog.New(collectCounter{&n}))
	defer slog.SetDefault(prev)

	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	setUsageInterval(cfg, 0)

	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		fw.collectUsagePeriodically(quit)
		close(done)
	}()
	defer func() {
		close(quit)
		<-done
	}()

	select {
	case <-done:
		t.Fatal("with usage.interval = 0 the ticker returned; a later SIGHUP can never switch it on")
	case <-time.After(1500 * time.Millisecond):
	}
	if got := n.Load(); got != 0 {
		t.Fatalf("interval 0 collected %d times", got)
	}

	setUsageInterval(cfg, 1)
	deadline := time.Now().Add(4 * time.Second)
	for n.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n.Load() == 0 {
		t.Fatal("interval reloaded 0 → 1s: no collect within 4s")
	}

	setUsageInterval(cfg, 0)
	time.Sleep(1200 * time.Millisecond) // a collect already in flight may land
	before := n.Load()
	time.Sleep(2500 * time.Millisecond)
	if got := n.Load(); got != before {
		t.Errorf("interval reloaded 1s → 0: %d more collects, want none", got-before)
	}
}

// The other half of the same defect: a running ticker kept its start-up
// interval, so shortening it from the 300 s default waited out the old one.
func TestUsageTicker_AShorterReloadedIntervalTakesEffectAtOnce(t *testing.T) {
	var n atomic.Int32
	prev := slog.Default()
	slog.SetDefault(slog.New(collectCounter{&n}))
	defer slog.SetDefault(prev)

	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	setUsageInterval(cfg, 300)

	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		fw.collectUsagePeriodically(quit)
		close(done)
	}()
	defer func() {
		close(quit)
		<-done
	}()

	time.Sleep(200 * time.Millisecond) // the ticker has read its start-up interval
	setUsageInterval(cfg, 1)
	deadline := time.Now().Add(4 * time.Second)
	for n.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n.Load() == 0 {
		t.Fatal("interval reloaded 300s → 1s: no collect within 4s")
	}
}
