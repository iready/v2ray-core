package tunctl

import (
	"sync"

	"github.com/v2fly/v2ray-core/v5/main/z/notify"
)

const (
	anomalyBypassRefresh = "bypass_refresh"
	anomalyGatewayWatch  = "gateway_watch"
)

var (
	anomalyMu     sync.Mutex
	anomalyShown  = map[string]struct{}{}
	anomalyNotify = notify.Info
)

// 人对侧只认劫持状态机里的网络异常，外加 Darwin 绕行没改成功。同一种只弹一次，恢复后才能再弹。
func reportHijackAnomaly(in hijackInputs) {
	reason := hijackPausedReason(in)
	switch reason {
	case HijackPausedIfaceGone, HijackPausedCNOutage:
		if reason != HijackPausedIfaceGone {
			clearAnomaly(HijackPausedIfaceGone)
		}
		if reason != HijackPausedCNOutage {
			clearAnomaly(HijackPausedCNOutage)
		}
		notifyAnomalyOnce(reason, HijackPausedZH(reason))
	default:
		clearAnomaly(HijackPausedIfaceGone, HijackPausedCNOutage)
	}
}

func notifyAnomalyOnce(key, body string) {
	anomalyMu.Lock()
	if _, ok := anomalyShown[key]; ok {
		anomalyMu.Unlock()
		return
	}
	anomalyShown[key] = struct{}{}
	anomalyMu.Unlock()
	go anomalyNotify("Rocket", body)
}

func clearAnomaly(keys ...string) {
	anomalyMu.Lock()
	for _, key := range keys {
		delete(anomalyShown, key)
	}
	anomalyMu.Unlock()
}
