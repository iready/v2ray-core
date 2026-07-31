//go:build windows

package tunctl

import (
	"net"
	"strconv"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

const fakeDNSRouteDest = "198.18.0.0"
const fakeDNSRouteMask = "255.254.0.0"

// ensureWindowsFakeDNSRoute FakeDNS 段必须进 TUN：与 Linux 相同，proxy_list/geoip 常不含 198.18。
func ensureWindowsFakeDNSRoute(ifIndex int, tunIPv4 string) error {
	if ifIndex <= 0 {
		return nil
	}
	gw := tunPeerIPv4(tunIPv4)
	if gw == "" {
		return nil
	}
	out, err := runHidden("route", "add", fakeDNSRouteDest, "mask", fakeDNSRouteMask, gw, "metric", "1", "if", strconv.Itoa(ifIndex))
	if err == nil {
		return nil
	}
	msg := string(out)
	if strings.Contains(msg, "already") || strings.Contains(msg, "已存在") {
		return nil
	}
	return err
}

func clearWindowsFakeDNSRoute() {
	_, _ = runHidden("route", "delete", fakeDNSRouteDest, "mask", fakeDNSRouteMask)
}

func clearWindowsTunDefaultRoute(tunIPv4 string) {
	gw := tunPeerIPv4(tunIPv4)
	if gw == "" {
		return
	}
	_, _ = runHidden("route", "delete", "0.0.0.0", "mask", "0.0.0.0", gw)
}

func addWindowsTunDefaultRoute(ifIndex int, tunIPv4 string) error {
	if ifIndex <= 0 {
		return nil
	}
	gw := tunPeerIPv4(tunIPv4)
	if gw == "" {
		return nil
	}
	out, err := runHidden("route", "add", "0.0.0.0", "mask", "0.0.0.0", gw, "metric", "1", "if", strconv.Itoa(ifIndex))
	if err == nil {
		return nil
	}
	msg := string(out)
	if strings.Contains(msg, "already") || strings.Contains(msg, "已存在") {
		return nil
	}
	return err
}

func tunPeerIPv4(tunIPv4 string) string {
	ip := net.ParseIP(strings.TrimSpace(tunIPv4)).To4()
	if ip == nil {
		ip = net.ParseIP(singtun.DefaultTunIPv4).To4()
	}
	if ip == nil {
		return ""
	}
	peer := append(net.IP(nil), ip...)
	if peer[3]%2 == 1 {
		peer[3]++
	} else if peer[3] > 0 {
		peer[3]--
	}
	return peer.String()
}
