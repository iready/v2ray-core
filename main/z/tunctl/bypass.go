package tunctl

import (
	"net/netip"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

// BypassPlan 系统路由绕行与用户态 handler 绕行配置。
type BypassPlan struct {
	Hosts           []string
	Networks        []string
	RouteExcludes   []netip.Prefix
	IncludeLoopback bool
	IncludeLAN      bool
}

func boolDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// BuildBypassPlan 按 TunProfile、Wire 地址与 v2fly 出站配置生成绕行计划。
func BuildBypassPlan(profile rvstore.TunProfile, wireURL string, configJSONs ...string) BypassPlan {
	includeRocket := boolDefault(profile.BypassRocketServer, true)
	includeLAN := boolDefault(profile.BypassLAN, true)
	includeLoopback := boolDefault(profile.BypassLoopback, true)

	var extraEntries []string
	if includeRocket {
		extraEntries = append(extraEntries, resolveWireHosts(wireURL)...)
	}
	extraEntries = append(extraEntries, profile.ExtraBypassHosts...)
	classified := classifyBypassEntries(extraEntries)

	var hosts []string
	hosts = append(hosts, classified.Hosts...)

	var networks []string
	if includeLAN {
		networks = append(networks, lanBypassNetworks()...)
	}
	networks = append(networks, classified.Networks...)
	// 127.0.0.0/8 不得写入 macOS 系统绕行（走默认网关会破坏 lo0，导致 127.0.0.1 不可达）。
	// 回环仅通过 RouteExcludes + sing-tun handler 排除。
	networks = uniqueNonEmptyStrings(networks)

	excludes := singtun.DefaultRouteExcludePrefixes()
	if !includeLoopback {
		excludes = removePrefix(excludes, "127.0.0.0/8")
	}
	if !includeLAN {
		for _, cidr := range lanBypassNetworks() {
			excludes = removePrefix(excludes, cidr)
		}
	}
	excludes = append(excludes, classified.Prefixes...)
	ntpClassified := classifyBypassEntries(rvstore.DefaultNTPBypassHosts)
	for _, p := range ntpClassified.Prefixes {
		if p.Addr().Is4() && !singtun.IsFakeDNSAddr(p.Addr()) {
			excludes = append(excludes, p)
		}
	}
	// 国内 DNS 上游始终绕行/排除（不依赖 config 是否已含 services.tun）。
	for _, cnHost := range profile.EffectiveCNResolverHosts() {
		hosts = append(hosts, cnHost)
		if p, err := netip.ParsePrefix(cnHost + "/32"); err == nil {
			excludes = append(excludes, p)
		}
	}
	for _, raw := range configJSONs {
		if !HasTUNService(raw) {
			continue
		}
		params := ParseTUNParams(raw)
		direct := deriveDirectBypassSet(raw, params.Tag)
		excludes = append(excludes, direct.Prefixes...)
		directClassified := classifyBypassEntries(direct.Hosts)
		hosts = append(hosts, directClassified.Hosts...)
		excludes = append(excludes, directClassified.Prefixes...)
	}
	outboundHosts := OutboundBypassHosts(configJSONs...)
	hosts = append(hosts, outboundHosts...)
	hosts = uniqueNonEmptyStrings(hosts)
	excludes = append(excludes, hostsToIPv4Prefixes(outboundHosts)...)
	excludes = dedupePrefixes(excludes)

	return BypassPlan{
		Hosts:           hosts,
		Networks:        networks,
		RouteExcludes:   excludes,
		IncludeLoopback: includeLoopback,
		IncludeLAN:      includeLAN,
	}
}

func lanBypassNetworks() []string {
	// 与 singtun.DefaultRouteExcludeCIDRs 对齐，但不写 127.0.0.0/8（会破坏 lo0）。
	var nets []string
	for _, cidr := range singtun.DefaultRouteExcludeCIDRs {
		if cidr == "127.0.0.0/8" {
			continue
		}
		nets = append(nets, cidr)
	}
	return nets
}

func resolveWireHosts(wireURL string) []string {
	return expandBypassHost(strings.TrimSpace(wire.HostFromURL(wireURL)))
}

func removePrefix(list []netip.Prefix, cidr string) []netip.Prefix {
	target, err := netip.ParsePrefix(cidr)
	if err != nil {
		return list
	}
	out := list[:0]
	for _, p := range list {
		if p != target {
			out = append(out, p)
		}
	}
	return out
}

func uniqueNonEmptyStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// ApplyBypassPlatform 写入 sing-tun 平台绕行（Linux / handler 层）。
func ApplyBypassPlatform(plan BypassPlan) {
	singtun.SetRouteExcludeAddresses(plan.RouteExcludes)
	singtun.SetBypassHosts(plan.Hosts)
}

// SetBypassPlan 配置当前 Manager 的绕行计划。
func SetBypassPlan(plan BypassPlan) {
	defaultManager.SetBypassPlan(plan)
}

// ApplyBypass 将绕行写入系统路由（macOS Helper）；其它平台可为空操作。
func ApplyBypass() error {
	return defaultManager.ApplyBypass()
}
