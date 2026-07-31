package tunctl

import (
	"encoding/json"
	"log"
	"net/netip"
	"strings"

	"github.com/v2fly/v2ray-core/v5/main/z/res"
)

// DirectBypassSet 从 routing.rules 中 direct/freedom 规则推导的 TUN 绕行集。
type DirectBypassSet struct {
	Prefixes []netip.Prefix
	Hosts    []string
}

func deriveDirectBypassSet(raw, tunTag string) DirectBypassSet {
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
		return DirectBypassSet{}
	}

	seenPrefix := make(map[netip.Prefix]struct{})
	seenHost := make(map[string]struct{})
	var out DirectBypassSet

	for _, rule := range doc.Routing.Rules {
		if !ruleMatchesTunInbound(rule.InboundTag, tunTag) {
			continue
		}
		if !isDirectOutbound(rule.OutboundTag) {
			continue
		}
		for _, token := range collectIPTokens(rule.IP) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			lower := strings.ToLower(token)
			if strings.HasPrefix(lower, "geoip:") {
				country := strings.TrimSpace(token[len("geoip:"):])
				prefixes, err := res.GeoIPPrefixes(country)
				if err != nil {
					log.Printf("tunctl: direct bypass geoip:%s: %v", country, err)
					continue
				}
				for _, p := range prefixes {
					appendDirectPrefix(&out, seenPrefix, p)
				}
				continue
			}
			if p, ok := parseIPTokenPrefix(token); ok {
				appendDirectPrefix(&out, seenPrefix, p)
			}
		}
		var domainHosts []string
		for _, field := range []json.RawMessage{rule.Domain, rule.Domains} {
			for _, token := range collectIPTokens(field) {
				if host := extractExplicitHostname(token); host != "" {
					domainHosts = append(domainHosts, host)
				}
			}
		}
		for _, host := range uniqueNonEmptyStrings(domainHosts) {
			if _, ok := seenHost[host]; ok {
				continue
			}
			seenHost[host] = struct{}{}
			out.Hosts = append(out.Hosts, host)
		}
	}
	return out
}

func appendDirectPrefix(out *DirectBypassSet, seen map[netip.Prefix]struct{}, p netip.Prefix) {
	if !p.IsValid() {
		return
	}
	p = p.Masked()
	if _, ok := seen[p]; ok {
		return
	}
	seen[p] = struct{}{}
	out.Prefixes = append(out.Prefixes, p)
}

func extractExplicitHostname(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "geosite:") || strings.HasPrefix(lower, "geoip:") {
		return ""
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
		return ""
	}
	return host
}
