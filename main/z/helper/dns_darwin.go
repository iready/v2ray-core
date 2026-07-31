//go:build darwin

package helper

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

type dnsSnapshot struct {
	service string
	servers []string
	dhcp    bool
}

func listDarwinNetworkServices() ([]string, error) {
	out, err := exec.Command("/usr/sbin/networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("list network services: %w", err)
	}
	var services []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") {
			continue
		}
		if strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func getDarwinDNSServers(service string) (servers []string, dhcp bool, err error) {
	out, err := exec.Command("/usr/sbin/networksetup", "-getdnsservers", service).Output()
	if err != nil {
		return nil, false, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, true, nil
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "there aren't any dns servers") {
		return nil, true, nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			servers = append(servers, line)
		}
	}
	return servers, false, nil
}

func setDarwinDNSServers(service string, servers ...string) error {
	args := []string{"-setdnsservers", service}
	if len(servers) == 0 {
		args = append(args, "Empty")
	} else {
		args = append(args, servers...)
	}
	out, err := exec.Command("/usr/sbin/networksetup", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("setdnsservers %s: %w (%s)", service, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func applyDarwinSystemDNS(addr string) ([]dnsSnapshot, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("empty dns address")
	}
	services, err := listDarwinNetworkServices()
	if err != nil {
		return nil, err
	}
	var snapshots []dnsSnapshot
	for _, service := range services {
		prev, dhcp, err := getDarwinDNSServers(service)
		if err != nil {
			continue
		}
		snapshots = append(snapshots, dnsSnapshot{
			service: service,
			servers: append([]string(nil), prev...),
			dhcp:    dhcp,
		})
		if err := setDarwinDNSServers(service, addr); err != nil {
			_ = restoreDarwinSystemDNS(snapshots[:len(snapshots)-1])
			return nil, err
		}
	}
	return snapshots, nil
}

func restoreDarwinSystemDNS(snapshots []dnsSnapshot) error {
	var first error
	for i := len(snapshots) - 1; i >= 0; i-- {
		snap := snapshots[i]
		var err error
		if snap.dhcp || len(snap.servers) == 0 {
			err = setDarwinDNSServers(snap.service)
		} else {
			err = setDarwinDNSServers(snap.service, snap.servers...)
		}
		if err != nil && first == nil {
			first = err
		}
	}
	return first
}

func isStaleTUNDNS(servers []string, tunIP string) bool {
	if len(servers) == 0 {
		return false
	}
	allowed := map[string]struct{}{
		strings.TrimSpace(tunIP):   {},
		singtun.DarwinDNSForwardIP: {},
		singtun.DefaultTunIPv4:     {},
	}
	for _, server := range servers {
		if _, ok := allowed[strings.TrimSpace(server)]; !ok {
			return false
		}
	}
	return true
}

func flushDarwinDNSCache() {
	_ = exec.Command("/usr/bin/dscacheutil", "-flushcache").Run()
	_ = exec.Command("/usr/bin/killall", "-HUP", "mDNSResponder").Run()
}

// RepairStaleTUNDNS 将仍指向 Rocket DNS 转发地址的系统 DNS 恢复为 DHCP。
func RepairStaleTUNDNS(tunIP string) (repaired int, err error) {
	tunIP = strings.TrimSpace(tunIP)
	if tunIP == "" {
		return 0, fmt.Errorf("empty tun ip")
	}
	services, err := listDarwinNetworkServices()
	if err != nil {
		return 0, err
	}
	var first error
	for _, service := range services {
		servers, _, err := getDarwinDNSServers(service)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if !isStaleTUNDNS(servers, tunIP) {
			continue
		}
		if err := setDarwinDNSServers(service); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		repaired++
	}
	if repaired > 0 {
		flushDarwinDNSCache()
	}
	return repaired, first
}
