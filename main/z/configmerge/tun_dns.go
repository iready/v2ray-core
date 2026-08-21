package configmerge

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

const (
	tunDNSOutboundTag = "dns-out"
	tunDNSInboundTag  = "tun-dns-in"
	tunCNDNSTag       = "tun-dns-cn"
	tunRemoteDNSTag   = "tun-dns-remote"
)

// ApplyTunOwnerDNSPolicy 为 TUN 拥有者注入 FakeDNS、国内真实解析与 UDP/53→dns 出站路由（对齐 sing-box TUN DNS）。
// profile 中 cn_dns / remote_dns / fakedns_domains 可空（用默认）。
func ApplyTunOwnerDNSPolicy(raw string, profile rvstore.TunProfile) (string, error) {
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
	ensureDNSOutbound(doc)
	ensureTunOwnerDNS(doc, profile)
	ensureTunDNSInbound(doc)
	ensureTunDNSForwardRule(doc)
	ensureTunDNSRule(doc, tunTag)
	ensureTunNTPDirectRule(doc, tunTag)
	ensureRoutingDomainStrategy(doc)
	_ = tunIPv4FromDoc(tun)
	return marshalDoc(doc)
}

func ensureTunOwnerDNS(doc map[string]interface{}, profile rvstore.TunProfile) {
	dns, _ := doc["dns"].(map[string]interface{})
	if dns == nil {
		dns = map[string]interface{}{}
		doc["dns"] = dns
	}
	ensureFakeDNSPool(dns)
	cnResolvers := profile.EffectiveCNResolvers()
	remoteResolver := profile.EffectiveRemoteResolver()
	fakeDomains := profile.EffectiveFakeDNSDomains()
	fakeDomainList := make([]interface{}, 0, len(fakeDomains))
	for _, d := range fakeDomains {
		fakeDomainList = append(fakeDomainList, d)
	}
	// 国内+NTP → cnResolvers（真直连，顺序即优先级）；FakeDNS → fakeDomains；其余境外 → remoteResolver 经代理。
	remote := map[string]interface{}{
		"address": remoteResolver,
		"tag":     tunRemoteDNSTag,
	}
	if proxyTag := preferTUNProxyOutboundTag(doc); proxyTag != "" {
		ensureDNSRemoteInboundRoute(doc, tunRemoteDNSTag, proxyTag)
	}
	cnDomains := []interface{}{"geosite:cn"}
	for _, h := range rvstore.DefaultNTPBypassHosts {
		cnDomains = append(cnDomains, "full:"+h, "domain:"+h)
	}
	servers := make([]interface{}, 0, len(cnResolvers)+2)
	for i, addr := range cnResolvers {
		item := map[string]interface{}{
			"address":      addr,
			"domains":      cnDomains,
			"skipFallback": true,
		}
		if i == 0 {
			item["tag"] = tunCNDNSTag
		} else {
			item["tag"] = fmt.Sprintf("%s-%d", tunCNDNSTag, i)
		}
		servers = append(servers, item)
	}
	servers = append(servers,
		map[string]interface{}{
			"address":      "fakedns",
			"domains":      fakeDomainList,
			"skipFallback": true,
		},
		remote,
	)
	dns["servers"] = servers
	if asString(dns["queryStrategy"]) == "" {
		dns["queryStrategy"] = "UseIPv4"
	}
}

func preferTUNProxyOutboundTag(doc map[string]interface{}) string {
	// 优先用 geosite:gfw / google 规则上的出站，避免 firstProxy 误选到 reverse 等无关 outbound。
	routing, _ := doc["routing"].(map[string]interface{})
	rules, _ := routing["rules"].([]interface{})
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		tag := strings.TrimSpace(asString(rule["outboundTag"]))
		if tag == "" || tag == "direct" || tag == "freedom" || tag == "blackhole" || tag == "dns-out" {
			continue
		}
		if ruleHasDomainMarker(rule, "geosite:gfw") || ruleHasDomainMarker(rule, "geosite:google") || ruleHasDomainMarker(rule, "geosite:youtube") {
			return tag
		}
	}
	return firstProxyOutboundTag(doc)
}

func ruleHasDomainMarker(rule map[string]interface{}, want string) bool {
	for _, key := range []string{"domain", "domains"} {
		raw, ok := rule[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			if v == want {
				return true
			}
		case []interface{}:
			for _, x := range v {
				if asString(x) == want {
					return true
				}
			}
		}
	}
	return false
}

func firstProxyOutboundTag(doc map[string]interface{}) string {
	obRaw, ok := doc["outbounds"].([]interface{})
	if !ok {
		return ""
	}
	for _, item := range obRaw {
		ob, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		protocol := strings.ToLower(strings.TrimSpace(asString(ob["protocol"])))
		switch protocol {
		case "", "freedom", "direct", "blackhole", "block", "dns", "tun":
			continue
		}
		if tag := strings.TrimSpace(asString(ob["tag"])); tag != "" {
			return tag
		}
	}
	return ""
}

func ensureDNSRemoteInboundRoute(doc map[string]interface{}, inTag, outTag string) {
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if asString(rule["outboundTag"]) != outTag {
			continue
		}
		if ruleMatchesInbound(rule, inTag) {
			return
		}
	}
	rules = append([]interface{}{map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{inTag},
		"outboundTag": outTag,
	}}, rules...)
	routing["rules"] = rules
}

func ensureFakeDNSPool(dns map[string]interface{}) {
	if hasFakeDNSPool(dns) {
		return
	}
	dns["fakedns"] = []interface{}{
		map[string]interface{}{
			"ipPool":   singtun.DefaultFakeDNSPool,
			"poolSize": singtun.DefaultFakeDNSLRU,
		},
	}
}

func hasFakeDNSPool(dns map[string]interface{}) bool {
	raw, ok := dns["fakedns"]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return true
	default:
		return false
	}
}

func ensureDNSOutbound(doc map[string]interface{}) {
	if hasOutboundTag(doc, tunDNSOutboundTag) {
		return
	}
	obRaw, _ := doc["outbounds"].([]interface{})
	obRaw = append(obRaw, map[string]interface{}{
		"protocol": "dns",
		"tag":      tunDNSOutboundTag,
		"settings": map[string]interface{}{},
	})
	doc["outbounds"] = obRaw
}

func ensureTunDNSInbound(doc map[string]interface{}) {
	port := singtun.TunDNSForwardPort
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		// Windows/Linux：系统 DNS=127.0.0.1，本机 :53 dokodemo 进 dns-out。
		// Darwin 用 Helper :53→53535，dokodemo 仍在 53535。
		port = 53
	}
	inRaw, _ := doc["inbounds"].([]interface{})
	for i, item := range inRaw {
		in, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if asString(in["tag"]) != tunDNSInboundTag {
			continue
		}
		in["listen"] = "127.0.0.1"
		in["port"] = port
		inRaw[i] = in
		doc["inbounds"] = inRaw
		return
	}
	inRaw = append(inRaw, map[string]interface{}{
		"listen":   "127.0.0.1",
		"port":     port,
		"protocol": "dokodemo-door",
		"tag":      tunDNSInboundTag,
		"settings": map[string]interface{}{
			"network": "tcp,udp",
			"address": "8.8.8.8",
			"port":    53,
		},
	})
	doc["inbounds"] = inRaw
}

func ensureTunDNSForwardRule(doc map[string]interface{}) {
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	if hasInboundDNSForwardRule(rules) {
		return
	}
	rules = append([]interface{}{map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{tunDNSInboundTag},
		"outboundTag": tunDNSOutboundTag,
	}}, rules...)
	routing["rules"] = rules
}

func hasInboundDNSForwardRule(rules []interface{}) bool {
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if asString(rule["outboundTag"]) != tunDNSOutboundTag {
			continue
		}
		raw, ok := rule["inboundTag"]
		if !ok {
			continue
		}
		switch tags := raw.(type) {
		case string:
			if tags == tunDNSInboundTag {
				return true
			}
		case []interface{}:
			for _, t := range tags {
				if asString(t) == tunDNSInboundTag {
					return true
				}
			}
		}
	}
	return false
}

func ensureTunNTPDirectRule(doc map[string]interface{}, tunTag string) {
	directTag := firstDirectOutboundTag(doc)
	if directTag == "" {
		directTag = "direct"
	}
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	if hasTunNTPDirectRule(rules, tunTag, directTag) {
		return
	}
	rules = append([]interface{}{map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{tunTag},
		"network":     "udp",
		"port":        "123",
		"outboundTag": directTag,
	}}, rules...)
	routing["rules"] = rules
}

func hasTunNTPDirectRule(rules []interface{}, tunTag, directTag string) bool {
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !ruleMatchesInbound(rule, tunTag) {
			continue
		}
		if asString(rule["port"]) != "123" {
			continue
		}
		if asString(rule["outboundTag"]) == directTag {
			return true
		}
	}
	return false
}

func ensureTunDNSRule(doc map[string]interface{}, tunTag string) {
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	if hasTunDNSRule(rules, tunTag) {
		return
	}
	rules = append([]interface{}{map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{tunTag},
		"port":        "53",
		"outboundTag": tunDNSOutboundTag,
	}}, rules...)
	routing["rules"] = rules
}

func hasTunDNSRule(rules []interface{}, tunTag string) bool {
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !ruleMatchesInbound(rule, tunTag) {
			continue
		}
		if asString(rule["port"]) != "53" {
			continue
		}
		if asString(rule["outboundTag"]) == tunDNSOutboundTag {
			return true
		}
	}
	return false
}

func ensureRoutingDomainStrategy(doc map[string]interface{}) {
	routing, _ := doc["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		doc["routing"] = routing
	}
	ds := strings.TrimSpace(asString(routing["domainStrategy"]))
	if ds == "" || strings.EqualFold(ds, "AsIs") {
		routing["domainStrategy"] = "IPIfNonMatch"
	}
}

func tunIPv4FromDoc(tun map[string]interface{}) string {
	ips, _ := tun["ips"].([]interface{})
	for _, item := range ips {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		ipRaw, ok := entry["ip"].([]interface{})
		if !ok || len(ipRaw) < 4 {
			continue
		}
		octets := make([]int, 4)
		for i := 0; i < 4; i++ {
			switch v := ipRaw[i].(type) {
			case float64:
				octets[i] = int(v)
			default:
				return singtun.DefaultTunIPv4
			}
		}
		return fmt.Sprintf("%d.%d.%d.%d", octets[0], octets[1], octets[2], octets[3])
	}
	return singtun.DefaultTunIPv4
}
