package tunctl

import (
	"encoding/json"
	"strings"
)

// ProxyRuleSet 从 routing.rules 推导的 tun-in 代理匹配范围。
type ProxyRuleSet struct {
	Domains    []string
	Geosites   []string
	HasGeosite bool
	HasGeoIP   bool
}

func deriveProxyRuleSet(raw, tunTag string) ProxyRuleSet {
	tunTag = strings.TrimSpace(tunTag)
	if tunTag == "" {
		tunTag = "tun-in"
	}
	var doc struct {
		Routing struct {
			Rules []routingRule `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return ProxyRuleSet{}
	}

	seen := make(map[string]struct{})
	var out ProxyRuleSet
	for _, rule := range doc.Routing.Rules {
		if !ruleMatchesTunInbound(rule.InboundTag, tunTag) {
			continue
		}
		if !isProxyOutbound(rule.OutboundTag) {
			continue
		}
		for _, token := range collectIPTokens(rule.Domain) {
			out.addDomainToken(token, seen)
		}
		for _, token := range collectIPTokens(rule.Domains) {
			out.addDomainToken(token, seen)
		}
		for _, token := range collectIPTokens(rule.IP) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(token), "geoip:") {
				out.HasGeoIP = true
			}
		}
	}
	return out
}

func (p *ProxyRuleSet) addDomainToken(token string, seen map[string]struct{}) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "geosite:") {
		p.HasGeosite = true
		if name := strings.TrimSpace(token[len("geosite:"):]); name != "" {
			p.Geosites = append(p.Geosites, name)
		}
	}
	if _, ok := seen[lower]; ok {
		return
	}
	seen[lower] = struct{}{}
	p.Domains = append(p.Domains, token)
}

func (p ProxyRuleSet) empty() bool {
	return len(p.Domains) == 0 && len(p.Geosites) == 0 && !p.HasGeoIP
}

// explicitHostnames 返回可做 DNS 解析的显式主机名（不含 geosite/geoip token）。
func (p ProxyRuleSet) explicitHostnames() []string {
	var out []string
	for _, token := range p.Domains {
		lower := strings.ToLower(strings.TrimSpace(token))
		if strings.HasPrefix(lower, "geosite:") || strings.HasPrefix(lower, "geoip:") {
			continue
		}
		host := token
		if idx := strings.Index(token, ":"); idx > 0 {
			prefix := strings.ToLower(token[:idx])
			switch prefix {
			case "domain", "full", "regexp":
				host = strings.TrimSpace(token[idx+1:])
			}
		}
		if host == "" || strings.Contains(host, "*") {
			continue
		}
		out = append(out, host)
	}
	return uniqueNonEmptyStrings(out)
}

func isProxyOutbound(tag string) bool {
	tag = strings.TrimSpace(tag)
	return tag != "" && !isDirectOutbound(tag) && !isBlockOutbound(tag)
}
