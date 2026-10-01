//go:build darwin

package tunctl

import "net"

// EnsureChinaBypassIP 为 geoip:cn 地址增量添加 OS bypass（DNS 解析后预置，避免首包进 TUN）。
func EnsureChinaBypassIP(ip net.IP) {
	ensureChinaBypassForIP(ip)
}
