package rvstore

import (
	"encoding/json"
	"strings"
)

// TunProfile 本地 TUN 偏好：是否启用、绕行策略及可选覆盖 services.tun。
type TunProfile struct {
	Use                bool                   `json:"use"`
	TunOwnerKey        string                 `json:"tun_owner_key,omitempty"`
	BindInterface      string                 `json:"bind_interface,omitempty"`
	// CnDNS 国内真实解析上游；可填 IP（如 223.5.5.5）或完整地址（如 https+local://…）。空则用默认。
	CnDNS string `json:"cn_dns,omitempty"`
	// RemoteDNS 境外 DNS 上游；可填 IP（如 8.8.8.8）或完整地址（如 tcp://…）。空则用默认。
	RemoteDNS string `json:"remote_dns,omitempty"`
	// FakeDNSDomains FakeDNS 命中域名集；空则用默认。仅 agent.json/API，UI 不展示。
	FakeDNSDomains     []string               `json:"fakedns_domains,omitempty"`
	BypassRocketServer *bool                  `json:"bypass_rocket_server,omitempty"`
	BypassLAN          *bool                  `json:"bypass_lan,omitempty"`
	BypassLoopback     *bool                  `json:"bypass_loopback,omitempty"`
	ExtraBypassHosts   []string               `json:"extra_bypass_hosts,omitempty"`
	Services           map[string]interface{} `json:"services,omitempty"`
}

func DefaultTunProfile() TunProfile {
	return TunProfile{Use: false}
}

func normalizeTunProfile(p TunProfile) TunProfile {
	p.TunOwnerKey = strings.TrimSpace(p.TunOwnerKey)
	p.BindInterface = strings.TrimSpace(p.BindInterface)
	p.CnDNS = strings.TrimSpace(p.CnDNS)
	p.RemoteDNS = strings.TrimSpace(p.RemoteDNS)
	if len(p.FakeDNSDomains) > 0 {
		cleaned := make([]string, 0, len(p.FakeDNSDomains))
		for _, d := range p.FakeDNSDomains {
			d = strings.TrimSpace(d)
			if d != "" {
				cleaned = append(cleaned, d)
			}
		}
		if len(cleaned) == 0 {
			p.FakeDNSDomains = nil
		} else {
			p.FakeDNSDomains = cleaned
		}
	} else {
		p.FakeDNSDomains = nil
	}
	if p.Services != nil && len(p.Services) == 0 {
		p.Services = nil
	}
	if len(p.ExtraBypassHosts) == 0 {
		p.ExtraBypassHosts = nil
	}
	return p
}

func (p TunProfile) Clone() TunProfile {
	out := TunProfile{
		Use:           p.Use,
		TunOwnerKey:   p.TunOwnerKey,
		BindInterface: p.BindInterface,
		CnDNS:         p.CnDNS,
		RemoteDNS:     p.RemoteDNS,
	}
	if len(p.FakeDNSDomains) > 0 {
		out.FakeDNSDomains = append([]string(nil), p.FakeDNSDomains...)
	}
	if p.BypassRocketServer != nil {
		v := *p.BypassRocketServer
		out.BypassRocketServer = &v
	}
	if p.BypassLAN != nil {
		v := *p.BypassLAN
		out.BypassLAN = &v
	}
	if p.BypassLoopback != nil {
		v := *p.BypassLoopback
		out.BypassLoopback = &v
	}
	if len(p.ExtraBypassHosts) > 0 {
		out.ExtraBypassHosts = append([]string(nil), p.ExtraBypassHosts...)
	}
	if p.Services == nil {
		return out
	}
	raw, err := json.Marshal(p.Services)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(raw, &out.Services)
	return out
}
