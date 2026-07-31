package wire

import (
	"net/url"
	"strings"
)

// IsWSS 仅 wss:// 使用 mTLS 客户端证书。
func IsWSS(wsURL string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(wsURL)), "wss://")
}

// NormalizeWSURL 补全 ws:// 前缀；已是 ws(s):// 则原样返回。
func NormalizeWSURL(raw string) string {
	return normalizeWSURL(raw)
}

func normalizeWSURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
		return raw
	}
	return "ws://" + raw
}

// HostFromURL 解析 Rocket Wire 地址中的主机名/IP（用于 TUN 绕行路由）。
func HostFromURL(raw string) string {
	u, err := url.Parse(normalizeWSURL(raw))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// PortFromURL 解析显式端口；未写则空串（由调用方按协议默认）。
func PortFromURL(raw string) string {
	u, err := url.Parse(normalizeWSURL(raw))
	if err != nil {
		return ""
	}
	return u.Port()
}
