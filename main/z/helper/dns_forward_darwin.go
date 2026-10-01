//go:build darwin

package helper

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"golang.org/x/sys/unix"
)

type dnsForwarder struct {
	mu      sync.Mutex
	tunConn *net.UDPConn
	done    chan struct{}
}

func (f *dnsForwarder) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopLocked()
}

func (f *dnsForwarder) start(ifName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopLocked()
	conn, err := listenTUNDNS(singtun.DarwinDNSForwardIP, ifName)
	if err != nil {
		return fmt.Errorf("listen dns %s:53: %w", singtun.DarwinDNSForwardIP, err)
	}
	target, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", singtun.TunDNSForwardPort))
	if err != nil {
		_ = conn.Close()
		return err
	}
	done := make(chan struct{})
	f.tunConn = conn
	f.done = done
	go runDNSForward(conn, target, done)
	return nil
}

func listenTUNDNS(listenIP, ifName string) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:53", listenIP))
	if err != nil {
		return nil, err
	}
	if listenIP == singtun.DarwinDNSForwardIP {
		lc := net.ListenConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				var sysErr error
				err := c.Control(func(fd uintptr) {
					_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
					sysErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
				})
				if err != nil {
					return err
				}
				return sysErr
			},
		}
		pc, err := lc.ListenPacket(context.Background(), "udp4", addr.String())
		if err != nil {
			return nil, err
		}
		conn, ok := pc.(*net.UDPConn)
		if !ok {
			_ = pc.Close()
			return nil, fmt.Errorf("listen dns: not UDPConn")
		}
		return conn, nil
	}
	return listenUDPOnInterface("udp4", addr.String(), ifName)
}

func listenUDPOnInterface(network, address, ifName string) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP(network, addr)
	if err != nil {
		return nil, err
	}
	if ifName != "" {
		if err := bindUDPToInterface(conn, ifName); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

func bindUDPToInterface(conn *net.UDPConn, ifName string) error {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return err
	}
	rc, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var sysErr error
	err = rc.Control(func(fd uintptr) {
		sysErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, iface.Index)
	})
	if err != nil {
		return err
	}
	return sysErr
}

func (f *dnsForwarder) stopLocked() {
	if f.done != nil {
		close(f.done)
		f.done = nil
	}
	if f.tunConn != nil {
		_ = f.tunConn.Close()
		f.tunConn = nil
	}
}

func runDNSForward(tunConn *net.UDPConn, target *net.UDPAddr, done <-chan struct{}) {
	buf := make([]byte, 4096)
	for {
		if isDone(done) {
			return
		}
		_ = tunConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, client, err := tunConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if isDone(done) || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("dns forward read: %v", err)
			continue
		}
		query := append([]byte(nil), buf[:n]...)
		go func(client *net.UDPAddr, query []byte) {
			resp, err := exchangeDNS(target, query)
			if err != nil || len(resp) == 0 {
				if err != nil {
					log.Printf("dns forward exchange: %v", err)
				}
				resp = dnsServFailResponse(query)
			}
			if isDone(done) {
				return
			}
			_, _ = tunConn.WriteToUDP(resp, client)
		}(client, query)
	}
}

func isDone(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func dnsServFailResponse(query []byte) []byte {
	if len(query) < 2 {
		return nil
	}
	resp := append([]byte(nil), query...)
	resp[2] |= 0x80 // QR=1
	resp[3] |= 0x02 // RCODE=2 SERVFAIL
	return resp
}

func exchangeDNS(target *net.UDPAddr, query []byte) ([]byte, error) {
	conn, err := net.DialUDP("udp", nil, target)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}
	resp := make([]byte, 4096)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), resp[:n]...), nil
}

func startDarwinDNSForward(ifName string) error {
	return defaultDNSForwarder.start(ifName)
}

func stopDarwinDNSForward() {
	defaultDNSForwarder.stop()
}

var defaultDNSForwarder dnsForwarder
