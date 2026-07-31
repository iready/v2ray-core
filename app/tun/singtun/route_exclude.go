package singtun

import (
	"fmt"
	"net/netip"
)

// DefaultRouteExcludeCIDRs 默认不从 TUN 接管的网段（回环、私网、链路本地）。
// 172.16/12 拆成 /16 并排除 172.19，避免与 DefaultTunIPv4（172.19.0.1/30）重叠。
var DefaultRouteExcludeCIDRs = buildDefaultRouteExcludeCIDRs()

func buildDefaultRouteExcludeCIDRs() []string {
	out := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"192.168.0.0/16",
		"169.254.0.0/16",
	}
	for i := 16; i <= 31; i++ {
		if i == 19 {
			continue // TUN 默认段
		}
		out = append(out, fmt.Sprintf("172.%d.0.0/16", i))
	}
	return out
}

// DefaultRouteExcludePrefixes 解析后的前缀列表，供 sing-tun 与 handler 使用。
func DefaultRouteExcludePrefixes() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(DefaultRouteExcludeCIDRs))
	for _, cidr := range DefaultRouteExcludeCIDRs {
		p, err := netip.ParsePrefix(cidr)
		if err == nil && p.IsValid() {
			out = append(out, p)
		}
	}
	return out
}

// MergeRouteExcludes 合并平台注入与默认排除，去重。
func MergeRouteExcludes(platform []netip.Prefix) []netip.Prefix {
	seen := make(map[netip.Prefix]struct{})
	var out []netip.Prefix
	appendUnique := func(list []netip.Prefix) {
		for _, p := range list {
			if !p.IsValid() {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	appendUnique(DefaultRouteExcludePrefixes())
	appendUnique(platform)
	return out
}
