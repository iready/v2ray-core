//go:build windows

package tunctl

import (
	"fmt"
	"net"
	"strings"
	"time"
)

func findInterfaceByIPv4(ipStr string) (name string, index int, err error) {
	want := net.ParseIP(strings.TrimSpace(ipStr))
	if want == nil {
		return "", 0, fmt.Errorf("invalid tun ip %q", ipStr)
	}
	want = want.To4()
	if want == nil {
		return "", 0, fmt.Errorf("tun ip %q is not ipv4", ipStr)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", 0, err
	}
	for _, iface := range ifaces {
		addrs, aerr := iface.Addrs()
		if aerr != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			if ipnet.IP.Equal(want) {
				return iface.Name, iface.Index, nil
			}
		}
	}
	return "", 0, fmt.Errorf("interface with %s not found", ipStr)
}

func waitInterfaceByIPv4(ipStr string, attempts int, gap time.Duration) (string, int, error) {
	var last error
	for i := 0; i < attempts; i++ {
		name, idx, err := findInterfaceByIPv4(ipStr)
		if err == nil {
			return name, idx, nil
		}
		last = err
		time.Sleep(gap)
	}
	return "", 0, last
}
