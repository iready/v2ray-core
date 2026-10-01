//go:build linux

package tunctl

import (
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

func rocketTunNetwork() netip.Prefix {
	p, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", singtun.DefaultTunIPv4, singtun.DefaultTunPrefix))
	if err != nil {
		return netip.Prefix{}
	}
	return p.Masked()
}

func isNumberedTun(name string) bool {
	if !strings.HasPrefix(name, "tun") {
		return false
	}
	rest := name[3:]
	if rest == "" {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func ifaceIsRocketTun(iface net.Interface, tunNet netip.Prefix) bool {
	if !isNumberedTun(iface.Name) {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	if len(addrs) == 0 {
		return true
	}
	if !tunNet.IsValid() {
		return false
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP == nil {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipNet.IP)
		if !ok {
			continue
		}
		if tunNet.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

// purgeStaleLinuxTun 清掉 sing-tun 启动失败留下的 tunN / ip rule。
// tunIf.Start 失败时以前不 Close 设备，热更新空实例会每分钟再 New 一张。
func purgeStaleLinuxTun() {
	tunNet := rocketTunNetwork()
	if tunNet.IsValid() {
		for i := 0; i < 8; i++ {
			if err := exec.Command("ip", "rule", "del", "to", tunNet.String()).Run(); err != nil {
				break
			}
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		if !ifaceIsRocketTun(iface, tunNet) {
			continue
		}
		_ = exec.Command("ip", "link", "del", iface.Name).Run()
	}
}
