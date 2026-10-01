//go:build linux

package tunctl

import (
	"net"
	"os"
	"os/exec"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

const (
	resolvPath   = "/etc/resolv.conf"
	resolvBackup = "/etc/resolv.conf.rocket-bak"
	fakeDNSRoute = "198.18.0.0/15"
)

// applyLinuxTunDNS 把进程解析指到 127.0.0.1（:53 dokodemo→dns-out→FakeDNS）。
// 保留 127.0.0.11 作次选，避免 Docker 内 rocket_service 等容器名解析挂掉。
func applyLinuxTunDNS() error {
	if _, err := os.Stat(resolvBackup); err != nil {
		if raw, readErr := os.ReadFile(resolvPath); readErr == nil {
			_ = os.WriteFile(resolvBackup, raw, 0o644)
		}
	}
	body := "# managed by rocket TUN\nnameserver 127.0.0.1\nnameserver 127.0.0.11\n"
	return os.WriteFile(resolvPath, []byte(body), 0o644)
}

func restoreLinuxTunDNS() {
	raw, err := os.ReadFile(resolvBackup)
	if err != nil {
		return
	}
	_ = os.WriteFile(resolvPath, raw, 0o644)
	_ = os.Remove(resolvBackup)
}

// ensureLinuxFakeDNSRoute FakeDNS 段必须进 TUN：服务端 proxy_list 常不含 198.18，会落默认路由 eth0。
func ensureLinuxFakeDNSRoute() error {
	dev := linuxTunDevice()
	if dev == "" {
		return nil
	}
	gw := linuxTunPeerGateway()
	args := []string{"route", "replace", fakeDNSRoute}
	if gw != "" {
		args = append(args, "via", gw)
	}
	args = append(args, "dev", dev)
	return exec.Command("ip", args...).Run()
}

func linuxTunDevice() string {
	want := net.ParseIP(singtun.DefaultTunIPv4)
	if want == nil {
		return ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			if ipNet.IP.Equal(want) {
				return iface.Name
			}
		}
	}
	return ""
}

func linuxTunPeerGateway() string {
	ip := net.ParseIP(singtun.DefaultTunIPv4).To4()
	if ip == nil {
		return ""
	}
	peer := append(net.IP(nil), ip...)
	// /30 对端：.1↔.2（不能 xor 1，否则 .1 会变成 .0 网络地址）
	if peer[3]%2 == 1 {
		peer[3]++
	} else if peer[3] > 0 {
		peer[3]--
	}
	return peer.String()
}

func clearLinuxFakeDNSRoute() {
	dev := linuxTunDevice()
	args := []string{"route", "del", fakeDNSRoute}
	if dev != "" {
		args = append(args, "dev", dev)
	}
	_ = exec.Command("ip", args...).Run()
}
