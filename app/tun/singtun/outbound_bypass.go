package singtun

import "net/netip"

var outboundBypassPrepare func(addr netip.Addr)

// SetOutboundBypassPrepare 在 TUN TCP 分发前调用（由 tunctl 注入国内 IP bypass）。
func SetOutboundBypassPrepare(fn func(addr netip.Addr)) {
	outboundBypassPrepare = fn
}

func prepareOutboundBypass(addr netip.Addr) {
	if outboundBypassPrepare != nil && addr.IsValid() {
		outboundBypassPrepare(addr)
	}
}
