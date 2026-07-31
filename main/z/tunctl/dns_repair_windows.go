//go:build windows

package tunctl

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

const createNoWindow = 0x08000000

func runHidden(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd.CombinedOutput()
}

// RepairStaleSystemDNS 仅清理历史版本把物理网卡 DNS 钉成 127.0.0.1 的残留；新主路径不再改物理网卡。
func RepairStaleSystemDNS() error {
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
			_ = restoreInterfaceDNSDHCP(iface)
		}
	}
	_, _ = runHidden("ipconfig", "/flushdns")
	return nil
}

func restoreInterfaceDNSDHCP(iface string) error {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return nil
	}
	out, err := runHidden("netsh", "interface", "ip", "set", "dns", "name="+iface, "dhcp")
	if err != nil {
		return fmt.Errorf("restore dns dhcp %s: %w (%s)", iface, err, strings.TrimSpace(string(out)))
	}
	_, _ = runHidden("ipconfig", "/flushdns")
	return nil
}

func interfaceDNSServers(iface string) ([]string, error) {
	out, err := runHidden("netsh", "interface", "ip", "show", "dns", "name="+iface)
	if err != nil {
		return nil, err
	}
	var servers []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if i := strings.LastIndex(line, ":"); i >= 0 {
			cand := strings.TrimSpace(line[i+1:])
			if looksIPv4(cand) {
				servers = append(servers, cand)
			}
		}
		fields := strings.Fields(line)
		if len(fields) == 1 && looksIPv4(fields[0]) {
			servers = append(servers, fields[0])
		}
	}
	return uniqueStrings(servers), nil
}

func looksIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
