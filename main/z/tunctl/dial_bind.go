package tunctl

import (
	"strings"
	"sync"

	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

var (
	dialBindMu    sync.RWMutex
	dialBindIface string
)

func init() {
	ctl := func(network, address string, fd uintptr) error {
		dialBindMu.RLock()
		iface := dialBindIface
		dialBindMu.RUnlock()
		if iface == "" {
			return nil
		}
		return bindOutboundToInterface(network, address, fd, iface)
	}
	_ = internet.RegisterDialerController(ctl)
	// UDP 出站走 ListenPacket，须同步注册 listener controller。
	_ = internet.RegisterListenerController(ctl)
}

// EnableDialBind 对齐 sing-box route.auto_detect_interface：TUN 活跃时全局绑定出站物理网卡。
func EnableDialBind(iface string) {
	dialBindMu.Lock()
	dialBindIface = strings.TrimSpace(iface)
	dialBindMu.Unlock()
}

// DisableDialBind 关闭出站绑网卡（TUN 关闭或降级时调用）。
func DisableDialBind() {
	EnableDialBind("")
}
