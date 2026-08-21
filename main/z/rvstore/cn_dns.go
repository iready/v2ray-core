package rvstore

import (
	"net"
	"net/url"
	"runtime"
	"strings"
)

const (
	defaultCNHost    = "223.5.5.5"
	defaultCNHostAlt = "223.6.6.6"
)

// DefaultRemoteResolver 境外 DNS 默认上游（TCP，可经代理出站）。
const DefaultRemoteResolver = "tcp://8.8.8.8:53"

// DefaultFakeDNSDomains FakeDNS 命中的域名集。
var DefaultFakeDNSDomains = []string{
	"geosite:gfw",
	"geosite:google",
	"geosite:youtube",
	"geosite:geolocation-!cn",
}

// DefaultNTPBypassHosts 系统校时域名：国内 DNS 用 full+domain 抢在 FakeDNS geosite 之前真解析。
var DefaultNTPBypassHosts = []string{
	"time.windows.com",
	"time.apple.com",
	"time.google.com",
	"time.nist.gov",
	"pool.ntp.org",
	"ntp.aliyun.com",
	"ntp.tencent.com",
}

// DefaultCNResolvers 国内 geosite:cn 默认上游列表（按平台，顺序即优先级）。
func DefaultCNResolvers() []string {
	return []string{
		ResolveCNResolver(defaultCNHost),
		ResolveCNResolver(defaultCNHostAlt),
	}
}

// ResolveCNResolver 将单条用户填写规范化为 v2ray DNS address。
// 裸 IP/主机 → 按平台补协议；已含 :// → 原样；空 → 空。
func ResolveCNResolver(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		return s
	}
	if runtime.GOOS == "darwin" {
		return "https+local://" + s + "/dns-query"
	}
	return "tcp+local://" + s + ":53"
}

// ResolveCNResolvers 规范化国内 DNS 列表；空则用平台默认。
func ResolveCNResolvers(raws []string) []string {
	seen := make(map[string]struct{}, len(raws))
	out := make([]string, 0, len(raws))
	for _, r := range raws {
		addr := ResolveCNResolver(r)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	if len(out) == 0 {
		return DefaultCNResolvers()
	}
	return out
}

// ResolveRemoteResolver 境外 DNS：空 → 默认；裸 IP → tcp://IP:53；已含 :// → 原样。
func ResolveRemoteResolver(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return DefaultRemoteResolver
	}
	if strings.Contains(s, "://") {
		return s
	}
	if !strings.Contains(s, ":") {
		return "tcp://" + s + ":53"
	}
	return "tcp://" + s
}

// ResolveFakeDNSDomains 空则默认；否则用用户列表（去空白）。
func ResolveFakeDNSDomains(domains []string) []string {
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d != "" {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), DefaultFakeDNSDomains...)
	}
	return out
}

// CNResolverHosts 取出国内 DNS 绕行用的主机/IP（去重，保序）。
func CNResolverHosts(raws []string) []string {
	seen := make(map[string]struct{})
	var hosts []string
	for _, addr := range ResolveCNResolvers(raws) {
		h := resolverHost(addr, defaultCNHost)
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		hosts = append(hosts, h)
	}
	return hosts
}

// RemoteResolverHost 境外 DNS 绕行主机（若为 IP；域名则原样返回供 hosts 列表）。
func RemoteResolverHost(raw string) string {
	return resolverHost(ResolveRemoteResolver(raw), "8.8.8.8")
}

func resolverHost(addr, fallback string) string {
	if u, err := url.Parse(addr); err == nil {
		if h := u.Hostname(); h != "" {
			return h
		}
	}
	if host, _, err := net.SplitHostPort(addr); err == nil && host != "" {
		return host
	}
	return fallback
}

// EffectiveCNResolvers 本机 TUN 偏好的国内 DNS 地址列表。
func (p TunProfile) EffectiveCNResolvers() []string {
	return ResolveCNResolvers(p.CnDNS)
}

// EffectiveCNResolverHosts 国内 DNS 绕行主机列表。
func (p TunProfile) EffectiveCNResolverHosts() []string {
	return CNResolverHosts(p.CnDNS)
}

// EffectiveRemoteResolver 境外 DNS 地址。
func (p TunProfile) EffectiveRemoteResolver() string {
	return ResolveRemoteResolver(p.RemoteDNS)
}

// EffectiveRemoteResolverHost 境外 DNS 主机。
func (p TunProfile) EffectiveRemoteResolverHost() string {
	return RemoteResolverHost(p.RemoteDNS)
}

// EffectiveFakeDNSDomains FakeDNS 域名集。
func (p TunProfile) EffectiveFakeDNSDomains() []string {
	return ResolveFakeDNSDomains(p.FakeDNSDomains)
}
