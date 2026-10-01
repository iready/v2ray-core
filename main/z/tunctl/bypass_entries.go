package tunctl

import (
	"net"
	"net/netip"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

type classifiedBypass struct {
	Hosts    []string
	Networks []string
	Prefixes []netip.Prefix
}

func classifyBypassEntries(entries []string) classifiedBypass {
	var out classifiedBypass
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if prefix, ok := parseBypassPrefix(entry); ok {
			if prefix.Bits() == 32 && prefix.Addr().Is4() {
				out.Hosts = append(out.Hosts, prefix.Addr().String())
			} else {
				out.Networks = append(out.Networks, prefix.String())
			}
			out.Prefixes = append(out.Prefixes, prefix)
			continue
		}
		if prefix, ok := ipv4Prefix(net.ParseIP(entry)); ok {
			out.Hosts = append(out.Hosts, prefix.Addr().String())
			out.Prefixes = append(out.Prefixes, prefix)
			continue
		}
		// 域名不能进 route -n。只收下此刻解析到的真实 IPv4；之后的地址由 DNS 真解析再写。
		for _, resolved := range expandBypassHost(entry) {
			ip := net.ParseIP(resolved)
			prefix, ok := ipv4Prefix(ip)
			if !ok || singtun.IsFakeDNSAddr(prefix.Addr()) {
				continue
			}
			out.Hosts = append(out.Hosts, prefix.Addr().String())
			out.Prefixes = append(out.Prefixes, prefix)
		}
	}
	out.Hosts = uniqueNonEmptyStrings(out.Hosts)
	out.Networks = uniqueNonEmptyStrings(out.Networks)
	out.Prefixes = dedupePrefixes(out.Prefixes)
	return out
}

func hostsToIPv4Prefixes(hosts []string) []netip.Prefix {
	var out []netip.Prefix
	for _, h := range hosts {
		if prefix, ok := ipv4Prefix(net.ParseIP(strings.TrimSpace(h))); ok {
			out = append(out, prefix)
		}
	}
	return dedupePrefixes(out)
}

func parseBypassPrefix(entry string) (netip.Prefix, bool) {
	if !strings.Contains(entry, "/") {
		return netip.Prefix{}, false
	}
	prefix, err := netip.ParsePrefix(entry)
	if err != nil || !prefix.IsValid() {
		return netip.Prefix{}, false
	}
	return prefix.Masked(), true
}

func parseIPTokenPrefix(token string) (netip.Prefix, bool) {
	if prefix, ok := parseBypassPrefix(token); ok {
		return prefix, true
	}
	if prefix, ok := ipv4Prefix(net.ParseIP(token)); ok {
		return prefix, true
	}
	ip := net.ParseIP(token)
	if ip == nil {
		return netip.Prefix{}, false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok || addr.Is4() {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr, 128).Masked(), true
}

func ipv4Prefix(ip net.IP) (netip.Prefix, bool) {
	if ip == nil {
		return netip.Prefix{}, false
	}
	v4 := ip.To4()
	if v4 == nil {
		return netip.Prefix{}, false
	}
	addr := netip.AddrFrom4([4]byte{v4[0], v4[1], v4[2], v4[3]})
	return netip.PrefixFrom(addr, 32).Masked(), true
}
