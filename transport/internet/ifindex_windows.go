//go:build windows

package internet

import (
	"net"
	"sync"
)

var (
	ifIndexMu   sync.Mutex
	ifIndexName string
	ifIndexVal  int
)

// InterfaceIndex 返回网卡 index。net.InterfaceByName 在 Windows 上每次都调用 GetAdaptersAddresses，出站每条连接都会走到。
func InterfaceIndex(name string) (int, error) {
	ifIndexMu.Lock()
	defer ifIndexMu.Unlock()
	if name != "" && name == ifIndexName && ifIndexVal != 0 {
		return ifIndexVal, nil
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return 0, err
	}
	ifIndexName = name
	ifIndexVal = iface.Index
	return iface.Index, nil
}

// ForgetInterfaceIndex 在绑定网卡切换时丢掉缓存。
func ForgetInterfaceIndex() {
	ifIndexMu.Lock()
	ifIndexName = ""
	ifIndexVal = 0
	ifIndexMu.Unlock()
}
