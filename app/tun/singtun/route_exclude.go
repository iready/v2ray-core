package singtun

import "net/netip"

// DefaultRouteExcludeCIDRs 默认不从 TUN 接管的网段（回环、私网、链路本地）。
var DefaultRouteExcludeCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
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
