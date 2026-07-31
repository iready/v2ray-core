//go:build windows

package tunctl

import (
	"encoding/binary"
	"net"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ipUnicastIf   = 31
	ipv6UnicastIf = 31
)

func bindOutboundToInterface(network, address string, fd uintptr, device string) error {
	_ = address
	iface, err := net.InterfaceByName(device)
	if err != nil {
		return err
	}
	handle := windows.Handle(fd)
	if strings.HasSuffix(network, "6") {
		return windows.SetsockoptInt(handle, windows.IPPROTO_IPV6, ipv6UnicastIf, iface.Index)
	}
	// Windows IPv4 IP_UNICAST_IF：network-order 接口 index（与 sockopt_windows 一致）。
	var be [4]byte
	binary.BigEndian.PutUint32(be[:], uint32(iface.Index))
	idx := int(*(*uint32)(unsafe.Pointer(&be[0])))
	return windows.SetsockoptInt(handle, windows.IPPROTO_IP, ipUnicastIf, idx)
}
