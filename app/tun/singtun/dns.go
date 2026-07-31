package singtun

import (
	"net/netip"
)

const (
	// DefaultFakeDNSPool 对齐 sing-box fake-ip 默认 IPv4 池。
	DefaultFakeDNSPool = "198.18.0.0/16"
	DefaultFakeDNSLRU  = 65535
	// TunDNSForwardPort Helper 将系统 DNS 转发到此 dokodemo 端口。
	TunDNSForwardPort = 53535
	// DarwinDNSForwardIP macOS 上 utun 网关 IP 无法 UDP 监听，系统 DNS 指向 loopback。
	DarwinDNSForwardIP = "127.0.0.1"
	// TunDNSOutboundTag v2ray DNS 出站，TUN UDP/53 hijack 目标。
	TunDNSOutboundTag = "dns-out"
)

// TunInterfaceAddrs 返回 TUN 接口本机地址（DNS 劫持目标），不得走 route_exclude bypass。
func TunInterfaceAddrs(ipv4Prefixes, ipv6Prefixes []netip.Prefix) []netip.Addr {
	var addrs []netip.Addr
	for _, p := range ipv4Prefixes {
		if p.IsValid() {
			addrs = append(addrs, p.Addr())
		}
	}
	for _, p := range ipv6Prefixes {
		if p.IsValid() {
			addrs = append(addrs, p.Addr())
		}
	}
	return addrs
}

// TunDNSAddr 返回 TUN 接口地址（Linux AutoRoute 时作 DNSServers；Darwin 由 helper 接管）。
func TunDNSAddr(ipv4Prefixes []netip.Prefix) netip.Addr {
	if len(ipv4Prefixes) == 0 {
		a, _ := netip.ParseAddr(DefaultTunIPv4)
		return a
	}
	return ipv4Prefixes[0].Addr()
}
