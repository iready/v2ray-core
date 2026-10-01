//go:build !darwin

package tunctl

import "net"

func EnsureChinaBypassIP(ip net.IP) {}

func EnsureExtraBypassIP(ip net.IP) {}
