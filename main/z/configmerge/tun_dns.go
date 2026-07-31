package configmerge

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

const (
	tunDNSOutboundTag = "dns-out"
	tunDNSInboundTag  = "tun-dns-in"
	// 国内 DNS 必须 tcp+local：走 DialSystem+绑网卡，不经 dispatcher，避免被 TUN/dns-out 劫持。
	tunCNResolver = "tcp+local://223.5.5.5:53"
	tunCNDNSTag   = "tun-dns-cn"
	// UDP/53 经代理常不通；TCP DNS 可走代理出站。
	tunRemoteResolver = "tcp://8.8.8.8:53"
)

// ApplyTunOwnerDNSPolicy 为 TUN 拥有者注入 FakeDNS、国内真实解析与 UDP/53→dns 出站路由（对齐 sing-box TUN DNS）。
func ApplyTunOwnerDNSPolicy(raw string) (string, error) {
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
	ensureTunOwnerDNS(doc)
	ensureTunDNSInbound(doc)
	ensureTunDNSForwardRule(doc)
	ensureTunDNSRule(doc, tunTag)
	ensureRoutingDomainStrategy(doc)
	_ = tunIPv4FromDoc(tun)
	return marshalDoc(doc)
}

func ensureTunOwnerDNS(doc map[string]interface{}) {
	dns, _ := doc["dns"].(map[string]interface{})
	if dns == nil {
		dns = map[string]interface{}{}
		doc["dns"] = dns
	}
	ensureFakeDNSPool(dns)
	// 国内 → tcp+local://223.5.5.5（真直连，不经代理/TUN）；
	// GFW/Google → FakeDNS；其余境外 → tcp://8.8.8.8 经代理。
	const remoteTag = "tun-dns-remote"
	remote := map[string]interface{}{
		"address": tunRemoteResolver,
		"tag":     remoteTag,
	}
	if proxyTag := preferTUNProxyOutboundTag(doc); proxyTag != "" {
		ensureDNSRemoteInboundRoute(doc, remoteTag, proxyTag)
	}
	dns["servers"] = []interface{}{
		map[string]interface{}{
			"address":      tunCNResolver,
			"domains":      []interface{}{"geosite:cn"},
			"skipFallback": true,
			"tag":          tunCNDNSTag,
		},
		map[string]interface{}{
			"address":      "fakedns",
			"domains":      []interface{}{"geosite:gfw", "geosite:google", "geosite:youtube", "geosite:geolocation-!cn"},
			"skipFallback": true,
		},
		remote,
	}
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

func ensureTunDNSServers(dns map[string]interface{}) {
	servers, _ := dns["servers"].([]interface{})
	if !hasCNResolver(servers) {
		servers = append([]interface{}{map[string]interface{}{
			"address":      tunCNResolver,
			"domains":      []interface{}{"geosite:cn"},
			"skipFallback": true,
		}}, servers...)
	}
	if !hasFakeDNSServer(servers) {
		servers = append(servers, "fakedns")
	}
	dns["servers"] = servers
}

func hasCNResolver(servers []interface{}) bool {
	for _, item := range servers {
		switch s := item.(type) {
		case string:
			if s == tunCNResolver {
				return true
			}
		case map[string]interface{}:
			if asString(s["address"]) != tunCNResolver {
				continue
			}
			if serverHasDomain(s, "geosite:cn") {
				return true
			}
		}
	}
	return false
}

func hasFakeDNSServer(servers []interface{}) bool {
	for _, item := range servers {
		switch s := item.(type) {
		case string:
			if strings.EqualFold(s, "fakedns") {
				return true
			}
		case map[string]interface{}:
			if strings.EqualFold(asString(s["address"]), "fakedns") {
				return true
			}
		}
	}
	return false
}

func serverHasDomain(server map[string]interface{}, want string) bool {
	raw, ok := server["domains"]
	if !ok {
		return false
	}
	switch domains := raw.(type) {
	case string:
		return domains == want
	case []interface{}:
		for _, d := range domains {
			if asString(d) == want {
				return true
			}
		}
	}
	return false
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
	inRaw, _ := doc["inbounds"].([]interface{})
	for _, item := range inRaw {
		in, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if asString(in["tag"]) == tunDNSInboundTag {
			return
		}
	}
	port := singtun.TunDNSForwardPort
	if runtime.GOOS == "windows" {
		// Windows 系统 DNS 固定 :53；与 DNSServers=127.0.0.1 对齐。
		port = 53
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
