package main

import (
	"context"
	"log"
	"os"
	goruntime "runtime"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/applog"
	"github.com/v2fly/v2ray-core/v5/transport/internet/udp"
)

// One aggregate per minute. No file/configuration access on forwarding goroutines.
func (rt *runtime) logPerformance(ctx context.Context) {
	defer applog.Recover("performance sampler")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var previous goruntime.MemStats
	goruntime.ReadMemStats(&previous)
	at := time.Now()
	cpu, cpuOK := applog.ProcessCPU()
	up, down := singtun.UploadBytes.Load(), singtun.DownloadBytes.Load()
	created, closed, expired := udp.Created.Load(), udp.Closed.Load(), udp.TimedOut.Load()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			var current goruntime.MemStats
			goruntime.ReadMemStats(&current)
			nextCPU, ok := applog.ProcessCPU()
			u, d := singtun.UploadBytes.Load(), singtun.DownloadBytes.Load()
			c, f, e := udp.Created.Load(), udp.Closed.Load(), udp.TimedOut.Load()
			cpuPercent := -1.0
			if ok && cpuOK {
				cpuPercent = float64(nextCPU-cpu) / float64(now.Sub(at)) * 100
			}
			log.Printf("perf pid=%d window=%s cpu_one_core_pct=%.2f goroutines=%d heap_mb=%d alloc_bytes=%d gc=%d gc_pause_ms=%.3f tcp=%d udp=%d up_bytes=%d down_bytes=%d udp_created=%d udp_closed=%d udp_timeout=%d log_suppressed_total=%d", os.Getpid(), now.Sub(at).Round(time.Millisecond), cpuPercent, goruntime.NumGoroutine(), current.HeapAlloc>>20, current.TotalAlloc-previous.TotalAlloc, current.NumGC-previous.NumGC, float64(current.PauseTotalNs-previous.PauseTotalNs)/1e6, singtun.LiveTCP.Load(), singtun.LiveUDP(), u-up, d-down, c-created, f-closed, e-expired, applog.Suppressed.Load())
			at, cpu, cpuOK, previous = now, nextCPU, ok, current
			up, down, created, closed, expired = u, d, c, f, e
			rt.refreshSnapshot()
		}
	}
}
