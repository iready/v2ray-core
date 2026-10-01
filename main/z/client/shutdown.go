package main

import (
	"log"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/mitmctl"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

func (rt *runtime) requestShutdown(reason string) {
	rt.mu.Lock()
	if rt.exiting {
		rt.mu.Unlock()
		return
	}
	rt.exiting = true
	rt.mu.Unlock()
	rt.exitRequested <- reason
}

// Only main returns after this bounded cleanup; cancellation does not exit main early.
func (rt *runtime) shutdown(reason string) {
	at := time.Now()
	log.Printf("phase=shutdown reason=%q result=begin", reason)
	rt.cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Remove system DNS redirection before waiting for any instance/lifecycle lock.
		log.Printf("phase=shutdown_dns result=begin")
		if err := tunctl.Default().SuspendRoutes(); err != nil {
			log.Printf("phase=shutdown_dns err=%v", err)
		}
		if rs := rt.getRS(); rs != nil {
			rs.Shutdown()
		}
		if err := mitmctl.Default().Stop(); err != nil {
			log.Printf("phase=shutdown_mitm err=%v", err)
		}
	}()
	select {
	case <-done:
		log.Printf("phase=shutdown result=complete elapsed=%s", time.Since(at))
	case <-time.After(15 * time.Second):
		log.Printf("phase=shutdown result=timeout elapsed=%s; process exit releases adapter handles", time.Since(at))
	}
}
