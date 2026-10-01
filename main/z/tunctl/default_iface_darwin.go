//go:build darwin

package tunctl

import (
	"net"
	"os/exec"
	"strings"
)

// darwinDefaultRouteInterface 读取系统默认路由出口网卡（如 en0），比 sing-tun 监视器更可靠。
func darwinDefaultRouteInterface() (string, error) {
	return darwinRouteField("interface:")
}

func darwinDefaultGateway() (string, error) {
	return darwinRouteField("gateway:")
}

func darwinRouteField(key string) (string, error) {
	out, err := exec.Command("/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) {
			val := strings.TrimSpace(strings.TrimPrefix(line, key))
			if val != "" {
				return val, nil
			}
		}
	}
	return "", errNoDefaultRouteIface
}

// darwinDefaultEndpoint 是这次直出贴着的网关和本机 IPv4。只比网关会漏掉同一路由器下 DHCP 换地址。
func darwinDefaultEndpoint() (gateway, src string, err error) {
	gateway, err = darwinDefaultGateway()
	if err != nil || gateway == "" {
		return "", "", err
	}
	name, nerr := darwinDefaultRouteInterface()
	if nerr != nil {
		return gateway, "", nil
	}
	return gateway, interfaceIPv4(name), nil
}

func interfaceIPv4(name string) string {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP == nil {
			continue
		}
		ip4 := n.IP.To4()
		if ip4 == nil || ip4.IsLinkLocalUnicast() {
			continue
		}
		return ip4.String()
	}
	return ""
}

func attachmentKey(gateway, src string) string {
	if src == "" {
		return gateway
	}
	return gateway + " " + src
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
