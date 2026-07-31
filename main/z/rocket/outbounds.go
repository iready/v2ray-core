package rocket

import (
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"strings"
)

var localOutboundProtocols = map[string]struct{}{
	"freedom":   {},
	"blackhole": {},
	"dns":       {},
	"loopback":  {},
}

// OutboundSnapshot 运行配置里的出站（不含账号）。Remote 表示有可探测的远端地址。
type OutboundSnapshot struct {
	ServerKey string `json:"server_key"`
	Tag       string `json:"tag"`
	Protocol  string `json:"protocol"`
	Address   string `json:"address,omitempty"`
	Port      int    `json:"port,omitempty"`
	Network   string `json:"network,omitempty"`
	Security  string `json:"security,omitempty"`
	Remote    bool   `json:"remote"`
}

type outboundJSON struct {
	Tag      string          `json:"tag"`
	Protocol string          `json:"protocol"`
	Settings json.RawMessage `json:"settings"`
	Stream   *streamJSON     `json:"streamSettings"`
}

type streamJSON struct {
	Network  string `json:"network"`
	Security string `json:"security"`
}

type outboundEndpoint struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// ListOutbounds 从各实例 RawJSON 抽出出站列表。
func ListOutbounds(rs *RS) []OutboundSnapshot {
	if rs == nil || len(rs.Servers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(rs.Servers))
	for key, srv := range rs.Servers {
		if srv != nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var out []OutboundSnapshot
	for _, key := range keys {
		out = append(out, ParseOutbounds(key, rs.Servers[key].RawJSON)...)
	}
	return out
}

// FindOutbound 按实例 key 与 tag 命中一条出站。
func FindOutbound(rs *RS, serverKey, tag string) (OutboundSnapshot, bool) {
	serverKey = strings.TrimSpace(serverKey)
	tag = strings.TrimSpace(tag)
	if serverKey == "" || tag == "" {
		return OutboundSnapshot{}, false
	}
	for _, ob := range ListOutbounds(rs) {
		if ob.ServerKey == serverKey && ob.Tag == tag {
			return ob, true
		}
	}
	return OutboundSnapshot{}, false
}

// RemoteOnly 状态页只展示远端出站；freedom/blackhole 等本地不进列表。
func RemoteOnly(list []OutboundSnapshot) []OutboundSnapshot {
	out := make([]OutboundSnapshot, 0, len(list))
	for _, ob := range list {
		if ob.Remote {
			out = append(out, ob)
		}
	}
	return out
}

// ParseOutbounds 解析单个实例 JSON 的 outbounds。
func ParseOutbounds(serverKey, raw string) []OutboundSnapshot {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var doc struct {
		Outbounds []outboundJSON `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}
	out := make([]OutboundSnapshot, 0, len(doc.Outbounds))
	for i, ob := range doc.Outbounds {
		item := snapshotFromOutbound(serverKey, i, ob)
		if item.Protocol == "" && item.Tag == "" && item.Address == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func snapshotFromOutbound(serverKey string, idx int, ob outboundJSON) OutboundSnapshot {
	item := OutboundSnapshot{
		ServerKey: serverKey,
		Tag:       strings.TrimSpace(ob.Tag),
		Protocol:  strings.ToLower(strings.TrimSpace(ob.Protocol)),
	}
	if item.Tag == "" {
		item.Tag = "(" + strconv.Itoa(idx) + ")"
	}
	if ob.Stream != nil {
		item.Network = strings.TrimSpace(ob.Stream.Network)
		item.Security = strings.TrimSpace(ob.Stream.Security)
	}
	addr, port := endpointFromSettings(ob.Settings)
	item.Address = addr
	item.Port = port
	item.Remote = remoteOutbound(item.Protocol, addr, port)
	return item
}

func remoteOutbound(protocol, addr string, port int) bool {
	if _, local := localOutboundProtocols[protocol]; local {
		return false
	}
	if addr == "" || port <= 0 || strings.EqualFold(addr, "localhost") {
		return false
	}
	ip := net.ParseIP(addr)
	if ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return false
	}
	return true
}

func endpointFromSettings(raw json.RawMessage) (string, int) {
	if len(raw) == 0 {
		return "", 0
	}
	var settings struct {
		Address   string             `json:"address"`
		Port      int                `json:"port"`
		Redirect  string             `json:"redirect"`
		Vnext     []outboundEndpoint `json:"vnext"`
		Servers   []outboundEndpoint `json:"servers"`
		Peers     []outboundEndpoint `json:"peers"`
		Receivers []outboundEndpoint `json:"receivers"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return "", 0
	}
	if ep := firstEndpoint(settings.Vnext, settings.Servers, settings.Peers, settings.Receivers); ep.Address != "" || ep.Port > 0 {
		return strings.TrimSpace(ep.Address), ep.Port
	}
	if addr := strings.TrimSpace(settings.Address); addr != "" {
		return addr, settings.Port
	}
	return splitHostPort(settings.Redirect)
}

func firstEndpoint(groups ...[]outboundEndpoint) outboundEndpoint {
	for _, group := range groups {
		for _, ep := range group {
			if strings.TrimSpace(ep.Address) != "" || ep.Port > 0 {
				ep.Address = strings.TrimSpace(ep.Address)
				return ep
			}
		}
	}
	return outboundEndpoint{}
}

func splitHostPort(redirect string) (string, int) {
	redirect = strings.TrimSpace(redirect)
	if redirect == "" {
		return "", 0
	}
	host, portStr, err := net.SplitHostPort(redirect)
	if err != nil {
		return redirect, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}
