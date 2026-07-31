//go:build windows

package tunctl

import (
	"net/netip"
	"sync"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

// Windows：路由与 TUN 适配器 DNS 由 sing-tun AutoRoute 独占（同 sing-box），不改物理网卡 DNS。

type Manager struct {
	mu         sync.Mutex
	active     bool
	ifName     string
	tunIPv4    string
	bypassPlan BypassPlan
	routePlan  RoutePlan
}

var defaultManager = &Manager{}

func Default() *Manager { return defaultManager }

type Status struct {
	Enabled         bool   `json:"enabled"`
	HelperInstalled bool   `json:"helper_installed"`
	IfName          string `json:"if_name,omitempty"`
	Error           string `json:"error,omitempty"`
}

func (m *Manager) HelperInstalled() bool { return IsElevated() }

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{
		Enabled:         m.active && !userOptedOut(),
		HelperInstalled: IsElevated(),
		IfName:          m.ifName,
	}
	if !st.HelperInstalled {
		st.Error = "not elevated: run as Administrator"
	}
	return st
}

func (m *Manager) SetBypassPlan(plan BypassPlan) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bypassPlan = plan
	ApplyBypassPlatform(plan)
}

func (m *Manager) SetRoutePlan(plan RoutePlan) {
	m.mu.Lock()
	m.routePlan = plan
	m.mu.Unlock()
}

func (m *Manager) SetBypassHosts(hosts []string) {
	m.SetBypassPlan(BypassPlan{Hosts: hosts, RouteExcludes: singtun.DefaultRouteExcludePrefixes()})
}

func (m *Manager) ApplyBypass() error { return nil }

func (m *Manager) EnableFromConfig(raw string) error {
	p := ParseTUNParams(raw)
	inet4, _ := p.FirstInet4()
	return m.Enable(p.Name, p.MTU, inet4)
}

func (m *Manager) Enable(name string, mtu uint32, tunIPv4 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return nil
	}
	_ = name
	_ = mtu
	if tunIPv4 == "" {
		tunIPv4 = singtun.DefaultTunIPv4
	}
	m.tunIPv4 = tunIPv4
	singtun.SetPlatform(singtun.Platform{
		AutoRoute:             true,
		ExternalConfiguration: false,
		RouteExcludeAddresses: append([]netip.Prefix(nil), m.bypassPlan.RouteExcludes...),
		BypassHosts:           append([]string(nil), m.bypassPlan.Hosts...),
	})
	m.active = true
	m.ifName = ""
	return nil
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	singtun.ResetPlatform()
	singtun.ClearBypassDecisionCache()
	m.active = false
	m.ifName = ""
	m.tunIPv4 = ""
	m.mu.Unlock()
	return nil
}

func (m *Manager) ApplyRoutes() error {
	m.mu.Lock()
	if !m.active {
		m.mu.Unlock()
		return nil
	}
	if m.ifName != "" {
		m.mu.Unlock()
		return nil
	}
	ip := m.tunIPv4
	if ip == "" {
		ip = singtun.DefaultTunIPv4
	}
	m.mu.Unlock()

	name, _, err := waitInterfaceByIPv4(ip, 20, 100*time.Millisecond)

	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		return err
	}
	if m.active {
		m.ifName = name
	}
	return nil
}

// ApplyDNS 空操作：TUN 适配器 DNS 由 sing-tun 设置，不改物理网卡。
func (m *Manager) ApplyDNS() error { return nil }

func (m *Manager) RefreshDNS() error { return nil }

func (m *Manager) SuspendRoutes() error { return nil }

func (m *Manager) ResumeRoutes() error { return nil }

func (m *Manager) PrepareConfigs(rawConfigs []string) error {
	if userOptedOut() {
		return nil
	}
	for _, raw := range rawConfigs {
		if HasTUNService(raw) {
			return m.EnableFromConfig(raw)
		}
	}
	return nil
}

func (m *Manager) Cleanup() error { return m.Disable() }
