//go:build darwin

package tunctl

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"time"
)

func ensureDirectBypassRoute(network, address string) {
	if strings.TrimSpace(network) == "" {
		return
	}
	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return
	}
	if ip := net.ParseIP(host); ip != nil {
		ensureChinaBypassForIP(ip)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return
	}
	for _, ip := range ips {
		ensureChinaBypassForIP(ip)
	}
}

func ensureChinaBypassForIP(ip net.IP) {
	addr, ok := netipAddrFromIP(ip)
	if !ok || !isChinaIP(addr) {
		return
	}
	defaultManager.ensureBypassHost(addr.String())
}

func netipAddrFromIP(ip net.IP) (netip.Addr, bool) {
	if ip4 := ip.To4(); ip4 != nil {
		return netip.AddrFrom4([4]byte{ip4[0], ip4[1], ip4[2], ip4[3]}), true
	}
	if len(ip) == net.IPv6len {
		var a [16]byte
		copy(a[:], ip)
		return netip.AddrFrom16(a), true
	}
	return netip.Addr{}, false
}

func prepareChinaOutboundBypass(addr netip.Addr) {
	if !isChinaIP(addr) {
		return
	}
	defaultManager.ensureBypassHost(addr.String())
}
