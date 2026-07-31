//go:build linux || windows || darwin

package singtun

import (
	"fmt"
	"sync"

	stun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/x/list"
)

var (
	imMu     sync.Mutex
	imRefs   int
	imNet    stun.NetworkUpdateMonitor
	imIface  stun.DefaultInterfaceMonitor
	imFinder *control.DefaultInterfaceFinder
)

func needsInterfaceMonitor(opts stun.Options) bool {
	if opts.EXP_ExternalConfiguration || opts.FileDescriptor > 0 {
		return false
	}
	return true
}

func attachInterfaceMonitors(opts *stun.Options) error {
	if opts == nil || !needsInterfaceMonitor(*opts) {
		return nil
	}
	imMu.Lock()
	defer imMu.Unlock()
	if imRefs == 0 {
		log := nopLogger{}
		netMon, err := stun.NewNetworkUpdateMonitor(log)
		if err != nil {
			return err
		}
		if err := netMon.Start(); err != nil {
			_ = netMon.Close()
			return err
		}
		finder := control.NewDefaultInterfaceFinder()
		ifaceMon, err := stun.NewDefaultInterfaceMonitor(netMon, log, stun.DefaultInterfaceMonitorOptions{
			InterfaceFinder: finder,
		})
		if err != nil {
			_ = netMon.Close()
			return err
		}
		if err := ifaceMon.Start(); err != nil {
			_ = ifaceMon.Close()
			_ = netMon.Close()
			return err
		}
		imNet = netMon
		imIface = ifaceMon
		imFinder = finder
	}
	imRefs++
	opts.InterfaceFinder = imFinder
	opts.InterfaceMonitor = imIface
	return nil
}

// EnsureStandaloneMonitor 启动网卡监视器（不依赖 TUN engine），供出站 bind 探测使用。
func EnsureStandaloneMonitor() error {
	opts := stun.Options{}
	return attachInterfaceMonitors(&opts)
}

// CurrentInterfaceFinder 返回已启动的网卡查找器；监视器未启动时返回 nil。
func CurrentInterfaceFinder() control.InterfaceFinder {
	imMu.Lock()
	defer imMu.Unlock()
	return imFinder
}

// CurrentInterfaceName 返回当前默认物理网卡名；监视器未启动时返回空。
func CurrentInterfaceName() string {
	imMu.Lock()
	defer imMu.Unlock()
	if imIface == nil {
		return ""
	}
	def := imIface.DefaultInterface()
	if def == nil {
		return ""
	}
	return def.Name
}

// RegisterDefaultInterfaceUpdate 在默认物理网卡变更时回调网卡名（空串表示暂时无默认口）。
// 须先 EnsureStandaloneMonitor / TUN attach；返回的 unregister 可重复调用。
func RegisterDefaultInterfaceUpdate(cb func(name string)) (unregister func(), err error) {
	if cb == nil {
		return func() {}, nil
	}
	imMu.Lock()
	defer imMu.Unlock()
	if imIface == nil {
		return nil, fmt.Errorf("interface monitor not started")
	}
	var elem *list.Element[stun.DefaultInterfaceUpdateCallback]
	elem = imIface.RegisterCallback(func(def *control.Interface, _ int) {
		name := ""
		if def != nil {
			name = def.Name
		}
		cb(name)
	})
	var once sync.Once
	return func() {
		once.Do(func() {
			imMu.Lock()
			defer imMu.Unlock()
			if imIface != nil && elem != nil {
				imIface.UnregisterCallback(elem)
			}
			elem = nil
		})
	}, nil
}

func detachInterfaceMonitors() {
	imMu.Lock()
	defer imMu.Unlock()
	if imRefs <= 0 {
		return
	}
	imRefs--
	if imRefs > 0 {
		return
	}
	if imIface != nil {
		_ = imIface.Close()
		imIface = nil
	}
	if imNet != nil {
		_ = imNet.Close()
		imNet = nil
	}
	imFinder = nil
}
