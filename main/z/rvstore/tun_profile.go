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
	if p.Services != nil && len(p.Services) == 0 {
		p.Services = nil
	}
	if len(p.ExtraBypassHosts) == 0 {
		p.ExtraBypassHosts = nil
	}
	return p
}

func (p TunProfile) Clone() TunProfile {
	out := TunProfile{Use: p.Use, TunOwnerKey: p.TunOwnerKey, BindInterface: p.BindInterface}
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
