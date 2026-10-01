//go:build windows

package tunctl

func staleLoopbackDNSInterfaces() []string {
	var out []string
	for _, iface := range ListBindInterfaceCandidates() {
		servers, err := interfaceDNSServers(iface)
		if err != nil || len(servers) == 0 {
			continue
		}
		onlyLoopback := true
		for _, s := range servers {
			if s != "127.0.0.1" && s != "::1" {
				onlyLoopback = false
				break
			}
		}
		if onlyLoopback {
			out = append(out, iface)
		}
	}
	return out
}
