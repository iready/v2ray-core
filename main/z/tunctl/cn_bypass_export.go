//go:build darwin

package tunctl

import "net"

// EnsureChinaBypassIP 为 geoip:cn 地址增量添加 OS bypass（DNS 解析后预置，避免首包进 TUN）。
func EnsureChinaBypassIP(ip net.IP) {
	ensureChinaBypassForIP(ip)
}

// EnsureExtraBypassIP 额外绕行域名的真实地址，不论是否国内都写系统绕行。
func EnsureExtraBypassIP(ip net.IP) {
	if ip == nil {
		return
	}
	v4 := ip.To4()
	if v4 == nil {
		return
	}
	defaultManager.ensureBypassHost(v4.String())
}
