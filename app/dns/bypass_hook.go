package dns

import "net"

// ChinaBypassIPHook 在 A 记录写入本地缓存后调用（由 rocket/tunctl 注入，供国内直连预置 OS bypass）。
var ChinaBypassIPHook func(ips []net.IP)
