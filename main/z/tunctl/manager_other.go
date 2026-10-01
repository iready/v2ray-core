//go:build !darwin && !windows && !linux

package tunctl

import "sync"

type Manager struct {
	mu sync.Mutex
}

var defaultManager = &Manager{}

func Default() *Manager { return defaultManager }

type Status struct {
	Enabled         bool   `json:"enabled"`
	HelperInstalled bool   `json:"helper_installed"`
	IfName          string `json:"if_name,omitempty"`
	Error           string `json:"error,omitempty"`
}

func (m *Manager) HelperInstalled() bool { return false }

func (m *Manager) Status() Status { return Status{} }

func (m *Manager) SetBypassPlan(plan BypassPlan) {
	ApplyBypassPlatform(plan)
}

func (m *Manager) SetRoutePlan(plan RoutePlan) {}

func (m *Manager) SetBypassHosts(hosts []string) {}

func (m *Manager) ApplyBypass() error { return nil }

func (m *Manager) EnableFromConfig(raw string) error { return nil }

func (m *Manager) Enable(name string, mtu uint32) error { return nil }

func (m *Manager) Disable() error { return nil }

func (m *Manager) ApplyRoutes() error { return nil }

func (m *Manager) ApplyDNS() error { return nil }

func (m *Manager) RefreshDNS() error { return nil }

func (m *Manager) SuspendRoutes() error { return nil }

func (m *Manager) ResumeRoutes() error { return nil }

func (m *Manager) PrepareConfigs(rawConfigs []string) error { return nil }

func (m *Manager) Cleanup() error { return nil }
