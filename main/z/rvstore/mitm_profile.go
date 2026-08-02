package rvstore

import "strings"

// MitmMapRemoteRule Map Remote：匹配 From，改写到 To。
// 空字段表示通配（From）或保留原值（To）。Path 以 * 结尾表示前缀匹配。
type MitmMapRemoteRule struct {
	ID           string `json:"id,omitempty"`
	Enabled      bool   `json:"enabled"`
	FromProto    string `json:"from_proto,omitempty"`
	FromHost     string `json:"from_host,omitempty"`
	FromPort     string `json:"from_port,omitempty"`
	FromPath     string `json:"from_path,omitempty"`
	FromQuery    string `json:"from_query,omitempty"`
	ToProto      string `json:"to_proto,omitempty"`
	ToHost       string `json:"to_host,omitempty"`
	ToPort       string `json:"to_port,omitempty"`
	ToPath       string `json:"to_path,omitempty"`
	ToQuery      string `json:"to_query,omitempty"`
	PreserveHost bool   `json:"preserve_host,omitempty"` // 保留原 Host 头
	Note         string `json:"note,omitempty"`
}

// MitmProfile 本地 HTTPS 抓包（显式代理 + CA），默认关闭。
type MitmProfile struct {
	Use         bool                `json:"use"`
	Addr        string              `json:"addr,omitempty"`
	WebAddr     string              `json:"web_addr,omitempty"`
	Upstream    string              `json:"upstream,omitempty"`
	SslInsecure bool                `json:"ssl_insecure,omitempty"`
	IgnoreHosts []string            `json:"ignore_hosts,omitempty"` // 手动不解密 host
	MediaBypass bool                `json:"media_bypass,omitempty"` // 一键绕行常见媒体 CDN（不解密、隧道直通）
	MapRemote   []MitmMapRemoteRule `json:"map_remote,omitempty"`
}

// MediaBypassHosts 开启「媒体绕行」时并入不解密列表（后缀匹配）。
func MediaBypassHosts() []string {
	return []string{
		"qpic.cn",
		"qlogo.cn",
		"gtimg.cn",
		"idqqimg.com",
		"v.qq.com",
		"video.qq.com",
		"tc.qq.com",
		"wxs.qq.com",
		"servicewechat.com",
	}
}

func DefaultMitmProfile() MitmProfile {
	return MitmProfile{
		Use:     false,
		Addr:    ":19080",
		WebAddr: "",
		IgnoreHosts: []string{
			"localhost",
			"127.0.0.1",
			"::1",
		},
		MediaBypass: false,
	}
}

func normalizeMitmProfile(p MitmProfile) MitmProfile {
	d := DefaultMitmProfile()
	if strings.TrimSpace(p.Addr) == "" {
		p.Addr = d.Addr
	}
	if p.IgnoreHosts == nil {
		p.IgnoreHosts = append([]string(nil), d.IgnoreHosts...)
	}
	if p.MapRemote == nil {
		p.MapRemote = []MitmMapRemoteRule{}
	}
	return p
}

func (p MitmProfile) Clone() MitmProfile {
	out := MitmProfile{
		Use:         p.Use,
		Addr:        p.Addr,
		WebAddr:     p.WebAddr,
		Upstream:    p.Upstream,
		SslInsecure: p.SslInsecure,
		MediaBypass: p.MediaBypass,
	}
	if p.IgnoreHosts != nil {
		out.IgnoreHosts = append([]string(nil), p.IgnoreHosts...)
	}
	if p.MapRemote != nil {
		out.MapRemote = append([]MitmMapRemoteRule(nil), p.MapRemote...)
	} else {
		out.MapRemote = []MitmMapRemoteRule{}
	}
	return out
}

// EffectiveIgnoreHosts 手动列表 +（可选）媒体绕行。
func (p MitmProfile) EffectiveIgnoreHosts() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(h string) {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	for _, h := range p.IgnoreHosts {
		add(h)
	}
	if p.MediaBypass {
		for _, h := range MediaBypassHosts() {
			add(h)
		}
	}
	return out
}

func (p MitmProfile) EffectiveAddr() string {
	if strings.TrimSpace(p.Addr) == "" {
		return DefaultMitmProfile().Addr
	}
	return strings.TrimSpace(p.Addr)
}

func (p MitmProfile) EffectiveWebAddr() string {
	return strings.TrimSpace(p.WebAddr)
}
