//go:build !(linux || windows || darwin)

package singtun

import stun "github.com/sagernet/sing-tun"

func attachInterfaceMonitors(*stun.Options) error { return nil }

func EnsureStandaloneMonitor() error { return nil }

func CurrentInterfaceName() string { return "" }

func RegisterDefaultInterfaceUpdate(func(name string)) (unregister func(), err error) {
	return func() {}, nil
}

func detachInterfaceMonitors() {}
