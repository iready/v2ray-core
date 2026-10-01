package tunctl

import "net/netip"

// RoutePlan OS 层 TUN 路由（由 Rocket config + routing.rules 推导）。
type RoutePlan struct {
	Mode         string
	SystemRoutes []string
}

func BuildRoutePlan(raw string) RoutePlan {
	params := ParseTUNParams(raw)
	plan := RoutePlan{Mode: RouteModeProxyList}
	configRoutes := tunCIDRsToStrings(params.Routes)
	proxy := deriveProxyRuleSet(raw, params.Tag)
	plan = buildProxyListRoutePlan(plan, configRoutes, proxy)
	return plan
}

// HasValidProxyListRoutes TUN 已启用且代理名单非空（routing 已注入 tun-in 规则）。
func HasValidProxyListRoutes(raw string) bool {
	if !HasTUNService(raw) {
		return true
	}
	params := ParseTUNParams(raw)
	return !deriveProxyRuleSet(raw, params.Tag).empty()
}

func buildProxyListRoutePlan(plan RoutePlan, configRoutes []string, proxy ProxyRuleSet) RoutePlan {
	if proxy.empty() {
		plan.SystemRoutes = configRoutes
		return plan
	}
	plan.SystemRoutes = mergeRouteStrings(configRoutes, defaultCaptureRoutes())
	return plan
}

func mergeRouteStrings(a, b []string) []string {
	return uniqueNonEmptyStrings(append(append([]string(nil), a...), b...))
}

func defaultCaptureRoutes() []string {
	return []string{
		"0.0.0.0/1",
		"128.0.0.0/1",
	}
}

func tunCIDRsToStrings(routes []tunCIDR) []string {
	out := make([]string, 0, len(routes))
	for _, r := range routes {
		if len(r.IP) != 4 {
			continue
		}
		pr := r.Prefix
		if pr <= 0 || pr > 32 {
			continue
		}
		out = append(out, netip.PrefixFrom(
			netip.AddrFrom4([4]byte{byte(r.IP[0]), byte(r.IP[1]), byte(r.IP[2]), byte(r.IP[3])}),
			pr,
		).Masked().String())
	}
	return uniqueNonEmptyStrings(out)
}

func dedupePrefixes(in []netip.Prefix) []netip.Prefix {
	seen := make(map[netip.Prefix]struct{}, len(in))
	out := make([]netip.Prefix, 0, len(in))
	for _, p := range in {
		if !p.IsValid() {
			continue
		}
		p = p.Masked()
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func routeStringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			return false
		}
	}
	return true
}
