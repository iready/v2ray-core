package tunctl

import (
	"net/netip"
	"sync"

	"github.com/v2fly/v2ray-core/v5/main/z/res"
	"go4.org/netipx"
)

var (
	cnIPMu     sync.RWMutex
	cnIPSet    *netipx.IPSet
	cnIPLoaded bool
)

func loadChinaIPSet() {
	cnIPMu.Lock()
	defer cnIPMu.Unlock()
	if cnIPLoaded {
		return
	}
	prefixes, err := res.GeoIPPrefixes("cn")
	if err == nil {
		var b netipx.IPSetBuilder
		for _, p := range prefixes {
			if p.IsValid() {
				b.AddPrefix(p.Masked())
			}
		}
		cnIPSet, _ = b.IPSet()
	}
	cnIPLoaded = true
}

func isChinaIP(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	loadChinaIPSet()
	cnIPMu.RLock()
	set := cnIPSet
	cnIPMu.RUnlock()
	return set != nil && set.Contains(addr)
}
