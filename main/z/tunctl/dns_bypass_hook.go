package tunctl

import (
	"net"

	coredns "github.com/v2fly/v2ray-core/v5/app/dns"
)

// ConfigureChinaDNSBypassHook DNS 解析到国内 A 记录时预置 OS bypass。
func ConfigureChinaDNSBypassHook(enabled bool) {
	if !enabled {
		coredns.ChinaBypassIPHook = nil
		return
	}
	coredns.ChinaBypassIPHook = func(ips []net.IP) {
		for _, ip := range ips {
			EnsureChinaBypassIP(ip)
		}
	}
}
