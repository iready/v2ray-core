//go:build linux

package tunctl

import (
	"net/netip"
	"sync"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

// Linux 无 Privileged Helper：由 sing-tun 进程内创建 TUN 并配置路由（需 CAP_NET_ADMIN/root）。
type Manager struct {
	mu         sync.Mutex
	active     bool
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

func (m *Manager) HelperInstalled() bool { return true }

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{
		Enabled:         m.active && !userOptedOut(),
		HelperInstalled: true,
	}
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
	return m.Enable(p.Name, p.MTU)
}

func (m *Manager) applyPlatformLocked() {
	singtun.SetPlatform(singtun.Platform{
		AutoRoute:             true,
		ExternalConfiguration: false,
		RouteExcludeAddresses: append([]netip.Prefix(nil), m.bypassPlan.RouteExcludes...),
		BypassHosts:           append([]string(nil), m.bypassPlan.Hosts...),
	})
}

func (m *Manager) Enable(name string, mtu uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return nil
	}
	_ = name
	_ = mtu
	m.applyPlatformLocked()
	m.active = true
	return nil
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clearLinuxFakeDNSRoute()
	restoreLinuxTunDNS()
	singtun.ResetPlatform()
	singtun.ClearBypassDecisionCache()
	m.active = false
	return nil
}

func (m *Manager) ApplyRoutes() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		return nil
	}
	return ensureLinuxFakeDNSRoute()
}

func (m *Manager) ApplyDNS() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		return nil
	}
	if err := applyLinuxTunDNS(); err != nil {
		return err
	}
	return ensureLinuxFakeDNSRoute()
}

func (m *Manager) RefreshDNS() error { return m.ApplyDNS() }

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
