package configmerge

import (
	"encoding/json"
	"strings"
)

// ApplyTunOwnerPolicy 为 TUN 拥有者补齐 sniff、IP 兜底路由与 blackhole 出站。
func ApplyTunOwnerPolicy(raw string) (string, error) {
	if raw == "" {
		return raw, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}
	services, _ := doc["services"].(map[string]interface{})
	tun, _ := services["tun"].(map[string]interface{})
	if tun == nil {
		return raw, nil
	}
	tunTag := tunInboundTag(tun)
	ensureDefaultTunSniffing(tun)
	ensureTunBlockQUICRule(doc, tunTag)
	directTag := ensureDirectFreedomOutbound(doc)
	ensureTunNonCNIPProxyRule(doc, tunTag)
	ensureTunFinalDirectRule(doc, tunTag, directTag)
	ensureBlackholeOutbound(doc)
	return marshalDoc(doc)
}

func tunInboundTag(tun map[string]interface{}) string {
	if tag, ok := tun["tag"].(string); ok && strings.TrimSpace(tag) != "" {
		return strings.TrimSpace(tag)
	}
	return "tun-in"
}

// ensureDirectFreedomOutbound 若无 freedom/direct 出站则注入 tag=direct，供 proxy_list 兜底直连。
func ensureFakeDNSOverride(v interface{}) []interface{} {
	out := []interface{}{}
	has := false
	switch t := v.(type) {
	case []interface{}:
		for _, x := range t {
			s := asString(x)
			if s == "" {
				continue
			}
			if strings.EqualFold(s, "fakedns") {
				has = true
			}
			out = append(out, s)
		}
	case []string:
		for _, s := range t {
			if strings.EqualFold(s, "fakedns") {
				has = true
			}
			out = append(out, s)
		}
	}
	if !has {
		out = append([]interface{}{"fakedns"}, out...)
	}
	return out
}

func ensureDirectFreedomOutbound(doc map[string]interface{}) string {
	if tag := firstDirectOutboundTag(doc); tag != "" && hasOutboundTag(doc, tag) {
		return tag
	}
	const tag = "direct"
	if !hasOutboundTag(doc, tag) {
		obRaw, _ := doc["outbounds"].([]interface{})
		obRaw = append(obRaw, map[string]interface{}{
			"protocol": "freedom",
			"tag":      tag,
			"settings": map[string]interface{}{},
		})
		doc["outbounds"] = obRaw
	}
	return tag
}

func hasOutboundTag(doc map[string]interface{}, tag string) bool {
	obRaw, ok := doc["outbounds"].([]interface{})
	if !ok {
		return false
	}
	for _, item := range obRaw {
		ob, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if asString(ob["tag"]) == tag {
			return true
		}
	}
	return false
}

func ensureTunFinalDirectRule(doc map[string]interface{}, tunTag, directTag string) {
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	if hasTunCatchAllDirect(rules, tunTag, directTag) {
		return
	}
	rules = append(rules, map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{tunTag},
		"outboundTag": directTag,
	})
	routing["rules"] = rules
}

// ensureTunNonCNIPProxyRule 真 IP 访问时域名/嗅探常未命中，geoip:!cn 走代理，避免 Google 等直连空响应。
// FakeDNS 198.18/15 必须显式进代理：geoip 可能不收录该段，嗅探未改写时会落到 catch-all direct。
func ensureTunNonCNIPProxyRule(doc map[string]interface{}, tunTag string) {
	proxyTag := preferTUNProxyOutboundTag(doc)
	if proxyTag == "" {
		return
	}
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	const fakeDNSPool = "198.18.0.0/15"
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !ruleMatchesInbound(rule, tunTag) {
			continue
		}
		if asString(rule["outboundTag"]) != proxyTag {
			continue
		}
		if !ruleHasIP(rule, "geoip:!cn") {
			continue
		}
		if !ruleHasIP(rule, fakeDNSPool) {
			rule["ip"] = appendIPList(rule["ip"], fakeDNSPool)
		}
		return
	}
	insert := map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{tunTag},
		"ip":          []interface{}{"geoip:!cn", fakeDNSPool},
		"outboundTag": proxyTag,
	}
	if i := indexTunCatchAll(rules, tunTag); i >= 0 {
		rules = append(rules[:i], append([]interface{}{insert}, rules[i:]...)...)
	} else {
		rules = append(rules, insert)
	}
	routing["rules"] = rules
}

func appendIPList(raw interface{}, add string) []interface{} {
	var out []interface{}
	switch v := raw.(type) {
	case string:
		if v != "" {
			out = append(out, v)
		}
	case []interface{}:
		out = append(out, v...)
	}
	out = append(out, add)
	return out
}

func indexTunCatchAll(rules []interface{}, tunTag string) int {
	for i, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !ruleMatchesInbound(rule, tunTag) {
			continue
		}
		if rule["domain"] != nil || rule["domains"] != nil || rule["ip"] != nil {
			continue
		}
		if asString(rule["port"]) != "" || asString(rule["network"]) != "" {
			continue
		}
		return i
	}
	return -1
}

func ruleHasIP(rule map[string]interface{}, want string) bool {
	raw, ok := rule["ip"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case string:
		return v == want
	case []interface{}:
		for _, x := range v {
			if asString(x) == want {
				return true
			}
		}
	}
	return false
}

func hasTunCatchAllDirect(rules []interface{}, tunTag, directTag string) bool {
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !ruleMatchesInbound(rule, tunTag) {
			continue
		}
		if asString(rule["outboundTag"]) != directTag {
			continue
		}
		if rule["domain"] != nil || rule["domains"] != nil || rule["ip"] != nil {
			continue
		}
		if asString(rule["port"]) != "" || asString(rule["network"]) != "" {
			continue
		}
		return true
	}
	return false
}

func ensureDefaultTunSniffing(tun map[string]interface{}) {
	for _, key := range []string{"sniffing_settings", "sniffingSettings"} {
		if sniff, ok := tun[key].(map[string]interface{}); ok && sniff != nil {
			// 强制覆盖目标，避免仅嗅探不改写导致仍按 IP 直连
			sniff["enabled"] = true
			sniff["metadataOnly"] = true
			if sniff["destination_override"] == nil && sniff["destinationOverride"] == nil {
				sniff["destination_override"] = []interface{}{"fakedns", "http", "tls", "quic"}
			} else {
				// 确保含 fakedns
				sniff["destination_override"] = ensureFakeDNSOverride(sniff["destination_override"])
				if sniff["destinationOverride"] != nil {
					sniff["destinationOverride"] = ensureFakeDNSOverride(sniff["destinationOverride"])
				}
			}
			normalizeTunSniffingSettings(tun)
			return
		}
	}
	tun["sniffing_settings"] = map[string]interface{}{
		"enabled":              true,
		"metadataOnly":         true,
		"destination_override": []interface{}{"fakedns", "http", "tls", "quic"},
	}
}

func firstDirectOutboundTag(doc map[string]interface{}) string {
	obRaw, ok := doc["outbounds"].([]interface{})
	if !ok {
		return "direct"
	}
	for _, item := range obRaw {
		ob, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		protocol := strings.ToLower(strings.TrimSpace(asString(ob["protocol"])))
		if protocol != "freedom" && protocol != "direct" {
			continue
		}
		if tag := strings.TrimSpace(asString(ob["tag"])); tag != "" {
			return tag
		}
	}
	return "direct"
}

// ensureTunBlockQUICRule 丢弃 TUN 上 UDP/443（QUIC），迫使浏览器回退 TCP；当前 UDP 转发未完备。
func ensureTunBlockQUICRule(doc map[string]interface{}, tunTag string) {
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	if hasTunQUICBlockRule(rules, tunTag) {
		return
	}
	rules = append([]interface{}{map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{tunTag},
		"network":     "udp",
		"port":        "443",
		"outboundTag": "blackhole",
	}}, rules...)
	routing["rules"] = rules
	ensureBlackholeOutbound(doc)
}

func hasTunQUICBlockRule(rules []interface{}, tunTag string) bool {
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !ruleMatchesInbound(rule, tunTag) {
			continue
		}
		if !strings.EqualFold(asString(rule["network"]), "udp") {
			continue
		}
		if asString(rule["port"]) != "443" {
			continue
		}
		if asString(rule["outboundTag"]) == "blackhole" {
			return true
		}
	}
	return false
}

func ruleMatchesInbound(rule map[string]interface{}, tunTag string) bool {
	raw, ok := rule["inboundTag"]
	if !ok {
		return true
	}
	switch tags := raw.(type) {
	case string:
		return tags == "" || tags == tunTag
	case []interface{}:
		if len(tags) == 0 {
			return true
		}
		for _, t := range tags {
			if asString(t) == tunTag {
				return true
			}
		}
	}
	return false
}

func ensureBlackholeOutbound(doc map[string]interface{}) {
	if !routingReferencesTag(doc, "blackhole") {
		return
	}
	obRaw, _ := doc["outbounds"].([]interface{})
	for _, item := range obRaw {
		ob, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if asString(ob["tag"]) == "blackhole" {
			return
		}
	}
	obRaw = append(obRaw, map[string]interface{}{
		"protocol": "blackhole",
		"tag":      "blackhole",
		"settings": map[string]interface{}{},
	})
	doc["outbounds"] = obRaw
}

func routingReferencesTag(doc map[string]interface{}, tag string) bool {
	routing, _ := doc["routing"].(map[string]interface{})
	rules, _ := routing["rules"].([]interface{})
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if asString(rule["outboundTag"]) == tag {
			return true
		}
	}
	return false
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}
