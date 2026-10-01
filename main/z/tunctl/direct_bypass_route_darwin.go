//go:build darwin

package tunctl

import (
	"net"
	"net/netip"
	"strings"
)

func ensureDirectBypassRoute(network, address string) {
	if strings.TrimSpace(network) == "" {
		return
	}
	host := dialHost(address)
	if host == "" {
		return
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// 禁止在 dial 路径做系统 DNS：TUN 下系统 DNS=127.0.0.1→dokodemo→dns-out，
		// 出站 dial 再 LookupIP 会自引用死锁，DNS 全挂。
		return
	}
	ensureChinaBypassForIP(ip)
}

func ensureChinaBypassForIP(ip net.IP) {
	addr, ok := netipAddrFromIP(ip)
	if ok {
		prepareChinaOutboundBypass(addr)
	}
}

func prepareChinaOutboundBypass(addr netip.Addr) {
	if !isChinaIP(addr) {
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
