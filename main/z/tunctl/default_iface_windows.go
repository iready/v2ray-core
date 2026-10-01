//go:build windows

package tunctl

import (
	"net"

	"golang.org/x/sys/windows"
)

// preferredAutoBindInterface 用 GetBestInterfaceEx 取默认出口，对齐 darwin route get default。
func preferredAutoBindInterface() (string, error) {
	var idx uint32
	sa := &windows.SockaddrInet4{Addr: [4]byte{8, 8, 8, 8}}
	if err := windows.GetBestInterfaceEx(sa, &idx); err != nil {
		return "", err
	}
	iface, err := net.InterfaceByIndex(int(idx))
	if err != nil {
		return "", err
	}
	if !ifaceIsBindCandidate(*iface) {
		return "", errNoDefaultRouteIface
	}
	return iface.Name, nil
}
