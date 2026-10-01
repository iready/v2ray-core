package tunctl

import "strings"

// StaleLoopbackDNSInterfaces 返回 DNS 仅指向 127.0.0.1/::1 的物理网卡（异常退出残留）。
func StaleLoopbackDNSInterfaces() []string {
	return staleLoopbackDNSInterfaces()
}

// BindInterfaceKnown 判断名称是否在当前可绑定物理网卡列表中。
func BindInterfaceKnown(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, c := range ListBindInterfaceCandidates() {
		if c == name {
			return true
		}
	}
	return false
}
