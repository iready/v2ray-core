package dns

import "net"

// ChinaBypassIPHook 在 A 记录写入本地缓存后调用（由 rocket/tunctl 注入，供国内直连预置 OS bypass）。
var ChinaBypassIPHook func(ips []net.IP)

// ExtraBypassAHook 额外绕行域名拿到真实 A 记录时调用，用来给这些 IP 写系统绕行。
var ExtraBypassAHook func(domain string, ips []net.IP)
