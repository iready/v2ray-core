package tunctl

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

var errNoDefaultRouteIface = errors.New("no default route interface")

var (
	bindIfaceMu     sync.RWMutex
	bindIfaceSnap   string
	bindIfaceManual string
	bindIfaceAuto   bool
)

// SnapshotBindInterface 在 TUN 改写路由前记录出站绑定网卡；manual 非空时强制使用该名称（须存在于本机候选）。
func SnapshotBindInterface(manual string) (string, error) {
	manual = strings.TrimSpace(manual)
	bindIfaceMu.Lock()
	defer bindIfaceMu.Unlock()
	if manual != "" {
		if !BindInterfaceKnown(manual) {
			return "", fmt.Errorf("bind interface %q not found (candidates: %v)", manual, ListBindInterfaceCandidates())
		}
		bindIfaceManual = manual
		bindIfaceSnap = manual
		bindIfaceAuto = false
		return manual, nil
	}
	bindIfaceManual = ""
	bindIfaceAuto = true
	if name, err := preferredAutoBindInterface(); err == nil && name != "" {
		bindIfaceSnap = name
		return name, nil
	}
	if err := singtun.EnsureStandaloneMonitor(); err != nil {
		return "", fmt.Errorf("start interface monitor: %w", err)
	}
	name := singtun.CurrentInterfaceName()
	if name == "" {
		return "", fmt.Errorf("no default physical interface detected")
	}
	bindIfaceSnap = name
	return name, nil
}

// CurrentBindInterface 返回当前生效的出站绑定网卡名。
func CurrentBindInterface() string {
	bindIfaceMu.RLock()
	defer bindIfaceMu.RUnlock()
	if bindIfaceManual != "" {
		return bindIfaceManual
	}
	if bindIfaceSnap != "" {
		return bindIfaceSnap
	}
	return singtun.CurrentInterfaceName()
}

// BindInterfaceAuto 是否使用自动探测（非用户指定）。
func BindInterfaceAuto() bool {
	bindIfaceMu.RLock()
	defer bindIfaceMu.RUnlock()
	return bindIfaceAuto && bindIfaceManual == ""
}

// ResetBindInterface 清除出站绑网卡快照。
func ResetBindInterface() {
	bindIfaceMu.Lock()
	defer bindIfaceMu.Unlock()
	bindIfaceSnap = ""
	bindIfaceManual = ""
	bindIfaceAuto = false
	DisableDialBind()
}

// ListBindInterfaceCandidates 列出可用于 bindToDevice 的网卡名。
func ListBindInterfaceCandidates() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if !ifaceIsBindCandidate(iface) {
			continue
		}
		out = append(out, iface.Name)
	}
	return out
}

func ifaceIsBindCandidate(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 {
		return false
	}
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	name := strings.ToLower(iface.Name)
	if strings.HasPrefix(name, "tun") || strings.Contains(name, "wintun") {
		return false
	}
	for _, prefix := range []string{"awdl", "llw", "bridge", "gif", "stf", "ap", "anpi"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLoopback() {
			continue
		}
		return true
	}
	return false
}
