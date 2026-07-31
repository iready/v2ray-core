//go:build darwin

package helper

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

const (
	utunControlName = "com.apple.net.utun_control"
	sysprotoControl = 2
)

type sockaddrCtl struct {
	sc_len      uint8
	sc_family   uint8
	ss_sysaddr  uint16
	sc_id       uint32
	sc_unit     uint32
	sc_reserved [5]uint32
}

func openUTun(name string) (int, string, error) {
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, sysprotoControl)
	if err != nil {
		return 0, "", fmt.Errorf("socket: %w", err)
	}

	var ctlInfo struct {
		ctl_id   uint32
		ctl_name [96]byte
	}
	copy(ctlInfo.ctl_name[:], utunControlName)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(0xc0644e03), uintptr(unsafe.Pointer(&ctlInfo))); errno != 0 {
		_ = unix.Close(fd)
		return 0, "", fmt.Errorf("ctl info: %w", errno)
	}

	unit := uint32(0)
	if name != "" {
		var n int
		if _, err := fmt.Sscanf(name, "utun%d", &n); err == nil && n > 0 {
			unit = uint32(n)
		}
	}

	addr := sockaddrCtl{
		sc_len:     uint8(unsafe.Sizeof(sockaddrCtl{})),
		sc_family:  unix.AF_SYSTEM,
		ss_sysaddr: sysprotoControl,
		sc_id:      ctlInfo.ctl_id,
		sc_unit:    unit,
	}
	if _, _, errno := unix.Syscall(unix.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&addr)), uintptr(addr.sc_len)); errno != 0 {
		_ = unix.Close(fd)
		return 0, "", fmt.Errorf("connect: %w", errno)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return 0, "", fmt.Errorf("nonblock: %w", err)
	}
	ifName, err := utunName(fd)
	if err != nil {
		_ = unix.Close(fd)
		return 0, "", fmt.Errorf("ifname: %w", err)
	}
	return fd, ifName, nil
}

func utunName(fd int) (string, error) {
	if name, err := unix.GetsockoptString(fd, sysprotoControl, 2); err == nil && name != "" {
		return name, nil
	}
	var name [unix.IFNAMSIZ]byte
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(0x40207462), uintptr(unsafe.Pointer(&name[0])))
	if errno != 0 {
		return "", errno
	}
	n := 0
	for n < len(name) && name[n] != 0 {
		n++
	}
	return string(name[:n]), nil
}

func setDarwinMTU(ifName string, mtu int) error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var ifr struct {
		name [unix.IFNAMSIZ]byte
		mtu  int32
		pad  [20]byte
	}
	copy(ifr.name[:], ifName)
	ifr.mtu = int32(mtu)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.SIOCSIFMTU), uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		return errno
	}
	return nil
}

func assignDarwinInterfaceIP(ifName, ip string, prefix int) error {
	if ip == "" {
		ip = singtun.DefaultTunIPv4
	}
	if prefix <= 0 {
		prefix = singtun.DefaultTunPrefix
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("invalid tun ipv4: %s", ip)
	}
	pfx := netip.PrefixFrom(addr, prefix)
	if !pfx.IsValid() {
		return fmt.Errorf("invalid tun prefix: %s/%d", ip, prefix)
	}
	peer := addr.Next()
	if !pfx.Contains(peer) {
		peer = addr.Prev()
	}
	if !pfx.Contains(peer) {
		return fmt.Errorf("tun prefix %s/%d too small for sing-tun", ip, prefix)
	}
	cmd := exec.Command("/sbin/ifconfig", ifName, "inet", addr.String(), peer.String(), "up")
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		return fmt.Errorf("ifconfig: %w (%s)", runErr, strings.TrimSpace(string(out)))
	}
	return nil
}

func darwinDefaultGateway() (string, error) {
	out, err := exec.Command("/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			gw := strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
			if gw != "" {
				return gw, nil
			}
		}
	}
	return "", fmt.Errorf("default gateway not found")
}

func applyDarwinBypassHosts(hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	gateway, err := darwinDefaultGateway()
	if err != nil {
		return err
	}
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		// 必须用默认网关，不能用 -interface en0；否则 macOS 会给公网 IP 写 permanent ARP 到本机 MAC。
		cmd := exec.Command("/sbin/route", darwinBypassHostRouteArgs(host, gateway)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(string(out), "File exists") {
				continue
			}
			return fmt.Errorf("bypass route %s: %w (%s)", host, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func applyDarwinBypassNetworks(cidrs []string) error {
	if len(cidrs) == 0 {
		return nil
	}
	gateway, err := darwinDefaultGateway()
	if err != nil {
		return err
	}
	for _, cidr := range cidrs {
		ip, prefix, ok := strings.Cut(cidr, "/")
		if !ok {
			continue
		}
		mask := cidrMask(prefix)
		if mask == "" {
			continue
		}
		cmd := exec.Command("/sbin/route", darwinBypassNetworkRouteArgs(ip, mask, gateway)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(string(out), "File exists") {
				continue
			}
			return fmt.Errorf("bypass net %s: %w (%s)", cidr, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func removeDarwinBypassHosts(hosts []string) {
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		cmd := exec.Command("/sbin/route", "-n", "delete", "-host", host)
		_ = cmd.Run()
		clearDarwinHostARP(host)
	}
}

func darwinBypassHostRouteArgs(host, gateway string) []string {
	return []string{"-n", "add", "-host", host, gateway}
}

func darwinBypassNetworkRouteArgs(ip, mask, gateway string) []string {
	return []string{"-n", "add", "-net", ip, "-netmask", mask, gateway}
}

func clearDarwinHostARP(host string) {
	cmd := exec.Command("/usr/sbin/arp", "-d", host)
	_ = cmd.Run()
}

func removeDarwinBypassNetworks(cidrs []string) {
	for _, cidr := range cidrs {
		ip, prefix, ok := strings.Cut(cidr, "/")
		if !ok {
			continue
		}
		mask := cidrMask(prefix)
		if mask == "" {
			continue
		}
		cmd := exec.Command("/sbin/route", "-n", "delete", "-net", ip, "-netmask", mask)
		_ = cmd.Run()
	}
}

func applyDarwinRoutes(ifName string, routes []string) error {
	for _, cidr := range routes {
		ip, prefix, ok := strings.Cut(cidr, "/")
		if !ok {
			continue
		}
		mask := cidrMask(prefix)
		if mask == "" {
			continue
		}
		cmd := exec.Command("/sbin/route", "-n", "add", "-net", ip, "-netmask", mask, "-interface", ifName)
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(string(out), "File exists") {
				continue
			}
			return fmt.Errorf("route add %s: %w (%s)", cidr, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func removeDarwinRoutes(ifName string, routes []string) error {
	for _, cidr := range routes {
		ip, prefix, ok := strings.Cut(cidr, "/")
		if !ok {
			continue
		}
		mask := cidrMask(prefix)
		if mask == "" {
			continue
		}
		cmd := exec.Command("/sbin/route", "-n", "delete", "-net", ip, "-netmask", mask, "-interface", ifName)
		_ = cmd.Run()
	}
	_ = ifName
	return nil
}

func cidrMask(prefix string) string {
	switch prefix {
	case "8":
		return "255.0.0.0"
	case "12":
		return "255.240.0.0"
	case "16":
		return "255.255.0.0"
	case "7":
		return "254.0.0.0"
	case "6":
		return "252.0.0.0"
	case "5":
		return "248.0.0.0"
	case "4":
		return "240.0.0.0"
	case "3":
		return "224.0.0.0"
	case "2":
		return "192.0.0.0"
	case "1":
		return "128.0.0.0"
	default:
		return ""
	}
}

func closeFD(fd int) error { return unix.Close(fd) }

func unixRights(fd int) []byte {
	return unix.UnixRights(fd)
}
