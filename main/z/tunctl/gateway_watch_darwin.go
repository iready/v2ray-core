//go:build darwin

package tunctl

import (
	"log"
	"sync"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/dns"
	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

const gatewayReconcileDelay = time.Second

var (
	gwWatchMu    sync.Mutex
	gwWatchUnreg func()
	gwWatchTimer *time.Timer
	gwWatchGen   uint64
	gwApplied    string
)

func startGatewayWatch() {
	stopGatewayWatch()
	unreg, err := singtun.RegisterNetworkUpdate(scheduleGatewayReconcile)
	if err != nil {
		log.Printf("gateway watch: %v", err)
		notifyAnomalyOnce(anomalyGatewayWatch, "没有监听到路由变化，换网络后绕行可能仍走旧网关")
		return
	}
	clearAnomaly(anomalyGatewayWatch)
	gwWatchMu.Lock()
	gwWatchUnreg = unreg
	gwWatchMu.Unlock()
}

func stopGatewayWatch() {
	gwWatchMu.Lock()
	unreg := gwWatchUnreg
	gwWatchUnreg = nil
	gwWatchGen++
	if gwWatchTimer != nil {
		gwWatchTimer.Stop()
		gwWatchTimer = nil
	}
	gwApplied = ""
	gwWatchMu.Unlock()
	if unreg != nil {
		unreg()
	}
}

func scheduleGatewayReconcile() {
	gwWatchMu.Lock()
	defer gwWatchMu.Unlock()
	if gwWatchUnreg == nil {
		return
	}
	gen := gwWatchGen
	if gwWatchTimer == nil {
		gwWatchTimer = time.AfterFunc(gatewayReconcileDelay, func() { reconcileGateway(gen) })
		return
	}
	gwWatchTimer.Reset(gatewayReconcileDelay)
}

func gatewayWatchCurrent(gen uint64) bool {
	gwWatchMu.Lock()
	defer gwWatchMu.Unlock()
	return gen == gwWatchGen && gwWatchUnreg != nil
}

func reconcileGateway(gen uint64) {
	if !gatewayWatchCurrent(gen) {
		return
	}
	gw, src, err := darwinDefaultEndpoint()
	if !gatewayWatchCurrent(gen) {
		return
	}
	if err != nil || gw == "" {
		noteGatewayAbsent()
		_ = syncHijack()
		return
	}
	key := attachmentKey(gw, src)
	gwWatchMu.Lock()
	changed := key != gwApplied
	prev := gwApplied
	if changed {
		gwApplied = key
	}
	gwWatchMu.Unlock()
	if changed {
		if err := defaultManager.client.RefreshBypass(); err != nil {
			gwWatchMu.Lock()
			if gwApplied == key {
				gwApplied = prev
			}
			gwWatchMu.Unlock()
			log.Printf("refresh bypass gateway: %v", err)
			notifyAnomalyOnce(anomalyBypassRefresh, "绕行下一跳没有改到当前网关")
			return
		}
		clearAnomaly(anomalyBypassRefresh)
		if !gatewayWatchCurrent(gen) {
			return
		}
		dns.ResetLocalDoH()
		log.Printf("绕行改到 %s", key)
	}
	if !gatewayWatchCurrent(gen) {
		return
	}
	noteGateway(key)
	_ = syncHijack()
}
