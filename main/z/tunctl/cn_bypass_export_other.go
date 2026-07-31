//go:build !darwin

package tunctl

import "net"

func EnsureChinaBypassIP(ip net.IP) {}
