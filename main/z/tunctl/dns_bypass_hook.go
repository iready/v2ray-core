package tunctl

import (
	"net"
	"strings"

	coredns "github.com/v2fly/v2ray-core/v5/app/dns"
)

// ConfigureChinaDNSBypassHook DNS 解析到国内 A 记录时预置 OS bypass。
func ConfigureChinaDNSBypassHook(enabled bool) {
	if !enabled {
		coredns.ChinaBypassIPHook = nil
		return
	}
	coredns.ChinaBypassIPHook = func(ips []net.IP) {
		for _, ip := range ips {
			EnsureChinaBypassIP(ip)
		}
	}
}

// ConfigureExtraBypassDomains 额外绕行域名解析出的 IP 都写系统绕行，不限国内地址。
func ConfigureExtraBypassDomains(domains []string) {
	patterns := append([]string(nil), domains...)
	if len(patterns) == 0 {
		coredns.ExtraBypassAHook = nil
		return
	}
	coredns.ExtraBypassAHook = func(domain string, ips []net.IP) {
		if !matchesBypassDomain(domain, patterns) {
			return
		}
		for _, ip := range ips {
			EnsureExtraBypassIP(ip)
		}
	}
}

func matchesBypassDomain(host string, patterns []string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(pattern)), ".")
		if pattern == "" {
			continue
		}
		if host == pattern || strings.HasSuffix(host, "."+pattern) {
			return true
		}
	}
	return false
}
