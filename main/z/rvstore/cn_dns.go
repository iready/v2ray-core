package rvstore

import (
	"net"
	"net/url"
	"runtime"
	"strings"
)

const (
	defaultCNHost = "223.5.5.5"
	// Darwin：部分网络 TCP/53→阿里 DNS 被重置，用 DoH local。
	defaultCNResolverDarwin = "https+local://223.5.5.5/dns-query"
	// Linux/Windows：沿用此前可用的 tcp+local。
	defaultCNResolverOther = "tcp+local://223.5.5.5:53"
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

// DefaultCNResolver 国内 geosite:cn 默认上游（按平台）。
func DefaultCNResolver() string {
	if runtime.GOOS == "darwin" {
		return defaultCNResolverDarwin
	}
	return defaultCNResolverOther
}

// ResolveCNResolver 将用户填写规范化为 v2ray DNS address。
// 空 → 平台默认；裸 IP/主机 → 按平台补协议；已含 :// → 原样。
func ResolveCNResolver(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return DefaultCNResolver()
	}
	if strings.Contains(s, "://") {
		return s
	}
	if runtime.GOOS == "darwin" {
		return "https+local://" + s + "/dns-query"
	}
	return "tcp+local://" + s + ":53"
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

// CNResolverHost 取出绕行用的主机/IP；解析失败时回退默认上游主机。
func CNResolverHost(raw string) string {
	return resolverHost(ResolveCNResolver(raw), defaultCNHost)
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

// EffectiveCNResolver 本机 TUN 偏好的国内 DNS 地址。
func (p TunProfile) EffectiveCNResolver() string {
	return ResolveCNResolver(p.CnDNS)
}

// EffectiveCNResolverHost 国内 DNS 绕行主机。
func (p TunProfile) EffectiveCNResolverHost() string {
	return CNResolverHost(p.CnDNS)
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
