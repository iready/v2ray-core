//go:build !darwin

package tunctl

import "net/netip"

func ensureDirectBypassRoute(network, address string) {}

func prepareChinaOutboundBypass(addr netip.Addr) {}
