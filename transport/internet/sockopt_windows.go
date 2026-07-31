//go:build windows

package internet

import (
	"encoding/binary"
	"net"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	TCP_FASTOPEN    = 15 // nolint: revive,stylecheck
	IP_UNICAST_IF   = 31 // nolint: revive,stylecheck
	IPV6_UNICAST_IF = 31 // nolint: revive,stylecheck
)

func setTFO(fd syscall.Handle, settings SocketConfig_TCPFastOpenState) error {
	switch settings {
	case SocketConfig_Enable:
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, TCP_FASTOPEN, 1); err != nil {
			return err
		}
	case SocketConfig_Disable:
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, TCP_FASTOPEN, 0); err != nil {
			return err
		}
	}
	return nil
}

// setWindowsUnicastIF Windows IPv4 IP_UNICAST_IF 要 network-order 接口 index；IPv6 用主机序。
func setWindowsUnicastIF(fd uintptr, network string, ifIndex int) error {
	handle := windows.Handle(fd)
	if strings.HasSuffix(network, "6") {
		return windows.SetsockoptInt(handle, windows.IPPROTO_IPV6, IPV6_UNICAST_IF, ifIndex)
	}
	var be [4]byte
	binary.BigEndian.PutUint32(be[:], uint32(ifIndex))
	idx := int(*(*uint32)(unsafe.Pointer(&be[0])))
	if err := windows.SetsockoptInt(handle, windows.IPPROTO_IP, IP_UNICAST_IF, idx); err != nil {
		return err
	}
	// 双栈 fd：顺带设 IPv6；失败可忽略（网卡可能未开 v6）。
	_ = windows.SetsockoptInt(handle, windows.IPPROTO_IPV6, IPV6_UNICAST_IF, ifIndex)
	return nil
}

func applyOutboundSocketOptions(network string, address string, fd uintptr, config *SocketConfig) error {
	if isTCPSocket(network) {
		if err := setTFO(syscall.Handle(fd), config.Tfo); err != nil {
			return err
		}
		if config.TcpKeepAliveIdle > 0 {
			if err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1); err != nil {
				return newError("failed to set SO_KEEPALIVE", err)
			}
		}
	}

	if config.BindToDevice != "" {
		iface, err := net.InterfaceByName(config.BindToDevice)
		if err != nil {
			return newError("failed to get interface ", config.BindToDevice).Base(err)
		}
		if err := setWindowsUnicastIF(fd, network, iface.Index); err != nil {
			return newError("failed to set IP_UNICAST_IF").Base(err)
		}
	}

	if config.TxBufSize != 0 {
		if err := windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_SNDBUF, int(config.TxBufSize)); err != nil {
			return newError("failed to set SO_SNDBUF").Base(err)
		}
	}

	if config.RxBufSize != 0 {
		if err := windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_RCVBUF, int(config.TxBufSize)); err != nil {
			return newError("failed to set SO_RCVBUF").Base(err)
		}
	}

	return nil
}

func applyInboundSocketOptions(network string, fd uintptr, config *SocketConfig) error {
	if isTCPSocket(network) {
		if err := setTFO(syscall.Handle(fd), config.Tfo); err != nil {
			return err
		}
		if config.TcpKeepAliveIdle > 0 {
			if err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1); err != nil {
				return newError("failed to set SO_KEEPALIVE", err)
			}
		}
	}

	// UDP dial 走 ListenPacket：须在 inbound sockopt 路径也绑物理网卡，否则 DNS/UDP 出站无 bind。
	if config.BindToDevice != "" {
		iface, err := net.InterfaceByName(config.BindToDevice)
		if err != nil {
			return newError("failed to get interface ", config.BindToDevice).Base(err)
		}
		if err := setWindowsUnicastIF(fd, network, iface.Index); err != nil {
			return newError("failed to set IP_UNICAST_IF").Base(err)
		}
	}

	if config.TxBufSize != 0 {
		if err := windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_SNDBUF, int(config.TxBufSize)); err != nil {
			return newError("failed to set SO_SNDBUF").Base(err)
		}
	}

	if config.RxBufSize != 0 {
		if err := windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_RCVBUF, int(config.TxBufSize)); err != nil {
			return newError("failed to set SO_RCVBUF").Base(err)
		}
	}

	return nil
}

func bindAddr(fd uintptr, ip []byte, port uint32) error {
	return nil
}

func setReuseAddr(fd uintptr) error {
	return nil
}

func setReusePort(fd uintptr) error {
	return nil
}
