//go:build darwin

package tunctl

import (
	"fmt"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/helper"
)

// RepairStaleSystemDNS 在 TUN 未运行时清除残留的系统 DNS（172.19.0.1）。
func RepairStaleSystemDNS() error {
	client := NewHelperClient()
	if client.Ping() == nil {
		up, _, err := client.Status()
		if err == nil && up {
			// rocket 崩溃后 Helper 仍可能持有 utun + 路由，但 53535 已死；须先拆再修 DNS。
			_ = client.Teardown()
		} else if err == nil {
			if _, err := client.RepairDNS(); err == nil {
				_, localErr := helper.RepairStaleTUNDNS(singtun.DefaultTunIPv4)
				return localErr
			}
		}
	}
	_, err := helper.RepairStaleTUNDNS(singtun.DefaultTunIPv4)
	return err
}

func (c *HelperClient) Status() (up bool, ifName string, err error) {
	line, err := c.roundTrip("STATUS")
	if err != nil {
		return false, "", err
	}
	if !strings.HasPrefix(line, "OK ") {
		return false, "", fmt.Errorf("helper: unexpected status %q", line)
	}
	for _, field := range strings.Fields(strings.TrimPrefix(line, "OK ")) {
		if strings.HasPrefix(field, "up=") {
			up = strings.TrimPrefix(field, "up=") == "true"
		}
		if strings.HasPrefix(field, "ifname=") {
			ifName = strings.TrimPrefix(field, "ifname=")
		}
	}
	return up, ifName, nil
}

func (c *HelperClient) RepairDNS() (repaired int, err error) {
	line, err := c.roundTrip("REPAIR_DNS")
	if err != nil {
		return 0, err
	}
	if !strings.HasPrefix(line, "OK ") {
		return 0, fmt.Errorf("helper: unexpected repair reply %q", line)
	}
	for _, field := range strings.Fields(strings.TrimPrefix(line, "OK ")) {
		if strings.HasPrefix(field, "repaired=") {
			var n int
			if _, err := fmt.Sscanf(field, "repaired=%d", &n); err == nil {
				repaired = n
			}
		}
	}
	return repaired, nil
}
