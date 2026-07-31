//go:build darwin

package helper

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

type Server struct {
	mu             sync.Mutex
	tunFD          int
	ifName         string
	routes         []string
	bypassHosts    []string
	bypassNetworks []string
	dnsSnapshots   []dnsSnapshot
	dnsForwardIP   string
}

func NewServer() *Server { return &Server{} }

func (s *Server) ListenAndServe(path string) error {
	if err := EnsureAllowedUIDFile(); err != nil {
		log.Printf("allowed uid: %v (reinstall helper if TUN cannot start)", err)
	}
	if n, err := RepairStaleTUNDNS(singtun.DefaultTunIPv4); err != nil {
		log.Printf("repair stale TUN DNS: %v", err)
	} else if n > 0 {
		log.Printf("repaired stale TUN DNS on %d interface(s)", n)
	}
	_ = os.Remove(path)
	addr, err := net.ResolveUnixAddr("unixgram", path)
	if err != nil {
		return err
	}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	const staffGID = 20
	_ = os.Chown(path, 0, staffGID)
	_ = os.Chmod(path, 0o660)

	buf := make([]byte, 512)
	oob := make([]byte, 1024)
	for {
		n, oobn, _, peer, err := conn.ReadMsgUnix(buf, oob)
		if err != nil {
			return err
		}
		if err := authorizePeer(conn, oob[:oobn]); err != nil {
			_ = s.reply(conn, peer, []byte("ERR unauthorized\n"), nil)
			continue
		}
		line := strings.TrimSpace(string(buf[:n]))
		s.dispatch(conn, peer, line)
	}
}

func (s *Server) reply(conn *net.UnixConn, peer *net.UnixAddr, msg, rights []byte) error {
	_, _, err := conn.WriteMsgUnix(msg, rights, peer)
	return err
}

func (s *Server) dispatch(conn *net.UnixConn, peer *net.UnixAddr, line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		_ = s.reply(conn, peer, []byte("ERR empty\n"), nil)
		return
	}
	switch parts[0] {
	case "PING":
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "STATUS":
		s.mu.Lock()
		up := s.tunFD > 0
		name := s.ifName
		routes := len(s.routes)
		s.mu.Unlock()
		_ = s.reply(conn, peer, []byte(fmt.Sprintf("OK up=%t ifname=%s routes=%d\n", up, name, routes)), nil)
	case "OPEN_TUN":
		name, mtu := "utun", uint32(1500)
		inet4, prefix := singtun.DefaultTunIPv4, singtun.DefaultTunPrefix
		if len(parts) > 1 {
			name = parts[1]
		}
		if len(parts) > 2 {
			var m uint32
			if _, err := fmt.Sscanf(parts[2], "%d", &m); err == nil {
				mtu = m
			}
		}
		if len(parts) > 3 {
			inet4 = parts[3]
		}
		if len(parts) > 4 {
			var p int
			if _, err := fmt.Sscanf(parts[4], "%d", &p); err == nil {
				prefix = p
			}
		}
		if err := s.openTUN(name, mtu, inet4, prefix); err != nil {
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		s.mu.Lock()
		fd := s.tunFD
		ifName := s.ifName
		s.mu.Unlock()
		fdSock := fmt.Sprintf("/var/run/com.v2ray.helper.fd.%d.sock", time.Now().UnixNano())
		ln, err := listenFDPass(fdSock)
		if err != nil {
			s.teardown()
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		errCh := make(chan error, 1)
		go func() {
			errCh <- serveFDOnce(ln, fdSock, fd)
		}()
		msg := []byte(fmt.Sprintf("OK ifname=%s fd_sock=%s\n", ifName, fdSock))
		if err := s.reply(conn, peer, msg, nil); err != nil {
			s.teardown()
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		go func() {
			if err := <-errCh; err != nil {
				s.teardown()
			}
		}()
	case "SETUP_BYPASS":
		hosts, networks := parseSetupBypassArgs(parts[1:])
		s.mu.Lock()
		oldHosts := append([]string(nil), s.bypassHosts...)
		oldNetworks := append([]string(nil), s.bypassNetworks...)
		s.mu.Unlock()
		removeDarwinBypassHosts(oldHosts)
		removeDarwinBypassNetworks(oldNetworks)
		// helper 重启后内存为空，但系统里可能残留旧绕行；添加前先删目标条目。
		removeDarwinBypassHosts(hosts)
		removeDarwinBypassNetworks(networks)
		if err := applyDarwinBypassHosts(hosts); err != nil {
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		if err := applyDarwinBypassNetworks(networks); err != nil {
			removeDarwinBypassHosts(hosts)
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		s.mu.Lock()
		s.bypassHosts = append([]string(nil), hosts...)
		s.bypassNetworks = append([]string(nil), networks...)
		s.mu.Unlock()
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "ADD_BYPASS_HOST":
		host := ""
		if len(parts) > 1 {
			host = strings.TrimSpace(parts[1])
		}
		if host == "" {
			_ = s.reply(conn, peer, []byte("ERR empty host\n"), nil)
			return
		}
		s.mu.Lock()
		for _, h := range s.bypassHosts {
			if h == host {
				s.mu.Unlock()
				_ = s.reply(conn, peer, []byte("OK\n"), nil)
				return
			}
		}
		s.mu.Unlock()
		if err := applyDarwinBypassHosts([]string{host}); err != nil {
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		s.mu.Lock()
		s.bypassHosts = append(s.bypassHosts, host)
		s.mu.Unlock()
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "SETUP_ROUTES":
		ifName := s.ifName
		if len(parts) > 1 {
			ifName = parts[1]
		}
		routes := parts[2:]
		if len(routes) == 0 {
			_ = s.reply(conn, peer, []byte("OK\n"), nil)
			return
		}
		if err := applyDarwinRoutes(ifName, routes); err != nil {
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		s.mu.Lock()
		s.routes = append([]string(nil), routes...)
		s.mu.Unlock()
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "SUSPEND_ROUTES":
		s.mu.Lock()
		ifName := s.ifName
		routes := append([]string(nil), s.routes...)
		s.mu.Unlock()
		if ifName != "" && len(routes) > 0 {
			if err := removeDarwinRoutes(ifName, routes); err != nil {
				_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
				return
			}
		}
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "RESUME_ROUTES":
		s.mu.Lock()
		ifName := s.ifName
		routes := append([]string(nil), s.routes...)
		s.mu.Unlock()
		if ifName != "" && len(routes) > 0 {
			if err := applyDarwinRoutes(ifName, routes); err != nil {
				_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
				return
			}
		}
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "SET_DNS":
		addr := singtun.DarwinDNSForwardIP
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			addr = parts[1]
		}
		s.mu.Lock()
		if len(s.dnsSnapshots) > 0 {
			_ = restoreDarwinSystemDNS(s.dnsSnapshots)
			s.dnsSnapshots = nil
		}
		s.mu.Unlock()
		snapshots, err := applyDarwinSystemDNS(addr)
		if err != nil {
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		s.mu.Lock()
		ifName := s.ifName
		s.dnsSnapshots = snapshots
		s.dnsForwardIP = addr
		s.mu.Unlock()
		if err := startDarwinDNSForward(ifName); err != nil {
			s.mu.Lock()
			_ = restoreDarwinSystemDNS(snapshots)
			s.dnsSnapshots = nil
			s.dnsForwardIP = ""
			s.mu.Unlock()
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "RESTORE_DNS":
		s.mu.Lock()
		snapshots := append([]dnsSnapshot(nil), s.dnsSnapshots...)
		s.dnsSnapshots = nil
		s.dnsForwardIP = ""
		s.mu.Unlock()
		stopDarwinDNSForward()
		if len(snapshots) > 0 {
			if err := restoreDarwinSystemDNS(snapshots); err != nil {
				_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
				return
			}
		} else {
			if _, err := RepairStaleTUNDNS(singtun.DefaultTunIPv4); err != nil {
				_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
				return
			}
		}
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	case "REPAIR_DNS":
		addr := singtun.DefaultTunIPv4
		if len(parts) > 1 {
			addr = parts[1]
		}
		stopDarwinDNSForward()
		n, err := RepairStaleTUNDNS(addr)
		if err != nil {
			_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR %v\n", err)), nil)
			return
		}
		s.mu.Lock()
		s.dnsSnapshots = nil
		s.dnsForwardIP = ""
		s.mu.Unlock()
		_ = s.reply(conn, peer, []byte(fmt.Sprintf("OK repaired=%d\n", n)), nil)
	case "TEARDOWN":
		s.teardown()
		_ = s.reply(conn, peer, []byte("OK\n"), nil)
	default:
		_ = s.reply(conn, peer, []byte(fmt.Sprintf("ERR unknown command %s\n", parts[0])), nil)
	}
}

func (s *Server) openTUN(name string, mtu uint32, inet4 string, prefix int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tunFD > 0 {
		return nil
	}
	fd, ifName, err := openUTun(name)
	if err != nil {
		return err
	}
	if mtu > 0 {
		_ = setDarwinMTU(ifName, int(mtu))
	}
	if err := assignDarwinInterfaceIP(ifName, inet4, prefix); err != nil {
		_ = closeFD(fd)
		return err
	}
	s.tunFD = fd
	s.ifName = ifName
	return nil
}

func parseSetupBypassArgs(args []string) (hosts, networks []string) {
	for i, arg := range args {
		if arg == "--" {
			networks = append([]string(nil), args[i+1:]...)
			return hosts, networks
		}
		if strings.Contains(arg, "/") {
			networks = append(networks, arg)
			continue
		}
		hosts = append(hosts, arg)
	}
	return hosts, networks
}

func parseSetupRoutesArgs(args []string) (bypass []string, routeStart int) {
	for routeStart < len(args) && !strings.Contains(args[routeStart], "/") {
		bypass = append(bypass, args[routeStart])
		routeStart++
	}
	return bypass, routeStart
}

func (s *Server) teardown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	stopDarwinDNSForward()
	s.dnsForwardIP = ""
	if len(s.dnsSnapshots) > 0 {
		_ = restoreDarwinSystemDNS(s.dnsSnapshots)
		s.dnsSnapshots = nil
	} else {
		_, _ = RepairStaleTUNDNS(singtun.DefaultTunIPv4)
	}
	if len(s.bypassHosts) > 0 {
		removeDarwinBypassHosts(s.bypassHosts)
	}
	if len(s.bypassNetworks) > 0 {
		removeDarwinBypassNetworks(s.bypassNetworks)
	}
	if s.ifName != "" && len(s.routes) > 0 {
		_ = removeDarwinRoutes(s.ifName, s.routes)
	}
	if s.tunFD > 0 {
		_ = closeFD(s.tunFD)
		s.tunFD = 0
	}
	s.ifName = ""
	s.routes = nil
	s.bypassHosts = nil
	s.bypassNetworks = nil
}
