//go:build darwin

package tunctl

import (
	"net"
	"os/exec"
	"strings"
)

// darwinDefaultRouteInterface 读取系统默认路由出口网卡（如 en0），比 sing-tun 监视器更可靠。
func darwinDefaultRouteInterface() (string, error) {
	out, err := exec.Command("/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
			if name != "" {
				return name, nil
			}
		}
	}
	return "", errNoDefaultRouteIface
}

func preferredAutoBindInterface() (string, error) {
	if name, err := darwinDefaultRouteInterface(); err == nil {
		iface, err := net.InterfaceByName(name)
		if err == nil && ifaceIsBindCandidate(*iface) {
			return name, nil
		}
	}
	return "", errNoDefaultRouteIface
}
