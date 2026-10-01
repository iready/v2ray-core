package tunctl

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"
)

var skipOutboundProtocols = map[string]struct{}{
	"freedom":   {},
	"blackhole": {},
	"dns":       {},
	"loopback":  {},
}

// OutboundBypassHosts 从 v2fly JSON 提取出站代理服务器地址，供 TUN 系统路由绕行。
func OutboundBypassHosts(configJSONs ...string) []string {
	var hosts []string
	for _, raw := range configJSONs {
		hosts = append(hosts, extractOutboundHostsFromJSON(raw)...)
	}
	return uniqueNonEmptyStrings(hosts)
}

func extractOutboundHostsFromJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}
	obRaw, ok := doc["outbounds"]
	if !ok {
		return nil
	}
	var outbounds []json.RawMessage
	if err := json.Unmarshal(obRaw, &outbounds); err != nil {
		return nil
	}
	var hosts []string
	for _, item := range outbounds {
		hosts = append(hosts, extractOutboundHosts(item)...)
	}
	return expandBypassHosts(hosts)
}

type outboundDoc struct {
	Protocol string          `json:"protocol"`
	Settings json.RawMessage `json:"settings"`
}

func extractOutboundHosts(obRaw json.RawMessage) []string {
	var ob outboundDoc
	if err := json.Unmarshal(obRaw, &ob); err != nil {
		return nil
	}
	protocol := strings.ToLower(strings.TrimSpace(ob.Protocol))
	if _, skip := skipOutboundProtocols[protocol]; skip {
		return nil
	}
	if len(ob.Settings) == 0 {
		return nil
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(ob.Settings, &settings); err != nil {
		return nil
	}
	return collectOutboundAddressFields(settings)
}

func collectOutboundAddressFields(settings map[string]json.RawMessage) []string {
	var hosts []string
	if addr, ok := settings["address"]; ok {
		if s := jsonStringField(addr); s != "" {
			hosts = append(hosts, s)
		}
	}
	for _, key := range []string{"servers", "vnext", "peers", "receivers"} {
		if raw, ok := settings[key]; ok {
			hosts = append(hosts, addressesFromJSONArray(raw)...)
		}
	}
	return hosts
}

func addressesFromJSONArray(raw json.RawMessage) []string {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	var hosts []string
	for _, item := range items {
		if addr, ok := item["address"]; ok {
			if s := jsonStringField(addr); s != "" {
				hosts = append(hosts, s)
			}
		}
		if endpoint, ok := item["endpoint"]; ok {
			if s := jsonStringField(endpoint); s != "" {
				hosts = append(hosts, s)
			}
		}
	}
	return hosts
}

func jsonStringField(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func expandBypassHosts(hosts []string) []string {
	var out []string
	for _, h := range hosts {
		out = append(out, expandBypassHost(h)...)
	}
	return uniqueNonEmptyStrings(out)
}

func expandBypassHost(host string) []string {
	host = strings.TrimSpace(host)
	if host == "" || isSkippableBypassHost(host) {
		return nil
	}
	result := []string{host}
	if net.ParseIP(host) != nil {
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return result
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			result = append(result, v4.String())
		}
	}
	return uniqueNonEmptyStrings(result)
}

func isSkippableBypassHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
