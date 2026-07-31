//go:build darwin

package tunctl

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/helper"
)

type HelperClient struct {
	socket string
}

func NewHelperClient() *HelperClient {
	return &HelperClient{socket: helper.SocketPath}
}

type helperConn struct {
	conn   *net.UnixConn
	server *net.UnixAddr
	path   string
}

var helperSeq uint64

// macOS 上 DialUnix(unixgram) 收不到 Helper 回复，须本地 bind 后 WriteMsgUnix。
func (c *HelperClient) bindLocal() (*helperConn, error) {
	seq := atomic.AddUint64(&helperSeq, 1)
	path := fmt.Sprintf("/tmp/v2ray-helper-%d-%d.sock", os.Getpid(), seq)
	_ = os.Remove(path)
	laddr, err := net.ResolveUnixAddr("unixgram", path)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUnixgram("unixgram", laddr)
	if err != nil {
		return nil, err
	}
	server := &net.UnixAddr{Name: c.socket, Net: "unixgram"}
	return &helperConn{conn: conn, server: server, path: path}, nil
}

func (h *helperConn) Close() error {
	_ = os.Remove(h.path)
	return h.conn.Close()
}

func (h *helperConn) write(cmd string) error {
	deadline := time.Now().Add(10 * time.Second)
	_ = h.conn.SetDeadline(deadline)
	_, _, err := h.conn.WriteMsgUnix([]byte(cmd), nil, h.server)
	return err
}

func (c *HelperClient) Ping() error {
	_, err := c.roundTrip("PING")
	return err
}

func (c *HelperClient) OpenTUN(name string, mtu uint32, inet4 string, prefix int) (fd int, ifName string, err error) {
	h, err := c.bindLocal()
	if err != nil {
		return 0, "", err
	}
	defer h.Close()
	cmd := fmt.Sprintf("OPEN_TUN %s %d %s %d\n", name, mtu, inet4, prefix)
	if err := h.write(cmd); err != nil {
		return 0, "", err
	}
	line, _, err := readHelperMsg(h.conn)
	if err != nil {
		return 0, "", err
	}
	if strings.HasPrefix(line, "ERR ") {
		return 0, "", fmt.Errorf("%s", strings.TrimPrefix(line, "ERR "))
	}
	ifName, fdSock, err := parseOpenTUNReply(line)
	if err != nil {
		return 0, "", err
	}
	fd, err = recvTUNFD(fdSock)
	if err != nil {
		return 0, "", err
	}
	return fd, ifName, nil
}

func (c *HelperClient) SetupBypass(hosts []string, networks []string) error {
	parts := []string{"SETUP_BYPASS"}
	parts = append(parts, hosts...)
	if len(networks) > 0 {
		parts = append(parts, "--")
		parts = append(parts, networks...)
	}
	_, err := c.roundTrip(strings.Join(parts, " "))
	return err
}

func (c *HelperClient) AddBypassHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	_, err := c.roundTrip("ADD_BYPASS_HOST " + host)
	return err
}

func (c *HelperClient) SetupRoutes(ifName string, routes []string) error {
	parts := []string{"SETUP_ROUTES", ifName}
	parts = append(parts, routes...)
	_, err := c.roundTrip(strings.Join(parts, " "))
	return err
}

func (c *HelperClient) SuspendRoutes() error {
	_, err := c.roundTrip("SUSPEND_ROUTES")
	return err
}

func (c *HelperClient) ResumeRoutes() error {
	_, err := c.roundTrip("RESUME_ROUTES")
	return err
}

func (c *HelperClient) SetupDNS(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = singtun.DefaultTunIPv4
	}
	_, err := c.roundTrip("SET_DNS " + addr)
	return err
}

func (c *HelperClient) RestoreDNS() error {
	_, err := c.roundTrip("RESTORE_DNS")
	return err
}

func (c *HelperClient) Teardown() error {
	_, err := c.roundTrip("TEARDOWN")
	return err
}

func (c *HelperClient) roundTrip(cmd string) (string, error) {
	h, err := c.bindLocal()
	if err != nil {
		return "", err
	}
	defer h.Close()
	if !strings.HasSuffix(cmd, "\n") {
		cmd += "\n"
	}
	if err := h.write(cmd); err != nil {
		return "", err
	}
	line, _, err := readHelperMsg(h.conn)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(line, "ERR ") {
		return "", fmt.Errorf("%s", strings.TrimPrefix(line, "ERR "))
	}
	return line, nil
}

func parseOpenTUNReply(line string) (ifName, fdSock string, err error) {
	if !strings.HasPrefix(line, "OK ") {
		return "", "", fmt.Errorf("helper: unexpected %q", line)
	}
	for _, f := range strings.Fields(strings.TrimPrefix(line, "OK ")) {
		if strings.HasPrefix(f, "ifname=") {
			ifName = strings.TrimPrefix(f, "ifname=")
		}
		if strings.HasPrefix(f, "fd_sock=") {
			fdSock = strings.TrimPrefix(f, "fd_sock=")
		}
	}
	if ifName == "" || fdSock == "" {
		return "", "", fmt.Errorf("helper: incomplete open reply %q", line)
	}
	return ifName, fdSock, nil
}

func recvTUNFD(sockPath string) (int, error) {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: sockPath, Net: "unix"})
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	_, fd, err := readHelperMsg(conn)
	if err != nil {
		return 0, err
	}
	if fd <= 0 {
		return 0, fmt.Errorf("helper: no fd on stream socket")
	}
	return fd, nil
}

func readHelperMsg(conn *net.UnixConn) (line string, fd int, err error) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	buf := make([]byte, 256)
	oob := make([]byte, unix.CmsgSpace(4))
	n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return "", 0, err
	}
	line = strings.TrimSpace(string(buf[:n]))
	if oobn > 0 {
		msgs, e := unix.ParseSocketControlMessage(oob[:oobn])
		if e != nil {
			return line, 0, e
		}
		if len(msgs) > 0 {
			fds, e := unix.ParseUnixRights(&msgs[0])
			if e != nil {
				return line, 0, e
			}
			if len(fds) > 0 {
				fd = fds[0]
			}
		}
	}
	return line, fd, nil
}

type Manager struct {
	mu             sync.Mutex
	client         *HelperClient
	active         bool
	routesApplied  bool
	bypassApplied  bool
	dnsApplied     bool
	ifName         string
	tunFD          int
	tunDNSAddr     string
	bypassHosts    []string
	bypassNetworks []string
	bypassPlan     BypassPlan
	routePlan      RoutePlan
	systemRoutes   []string
}

var defaultManager = &Manager{client: NewHelperClient()}

func Default() *Manager { return defaultManager }

func (m *Manager) HelperInstalled() bool {
	return m.client.Ping() == nil
}

type Status struct {
	Enabled         bool   `json:"enabled"`
	HelperInstalled bool   `json:"helper_installed"`
	IfName          string `json:"if_name,omitempty"`
	Error           string `json:"error,omitempty"`
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{
		Enabled:         m.active && !userOptedOut(),
		HelperInstalled: m.client.Ping() == nil,
		IfName:          m.ifName,
	}
	return st
}

func (m *Manager) SetBypassPlan(plan BypassPlan) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bypassPlan = plan
	m.bypassHosts = append([]string(nil), plan.Hosts...)
	m.bypassNetworks = append([]string(nil), plan.Networks...)
	ApplyBypassPlatform(plan)
}

func (m *Manager) SetRoutePlan(plan RoutePlan) {
	m.mu.Lock()
	m.routePlan = plan
	m.systemRoutes = append([]string(nil), plan.SystemRoutes...)
	m.mu.Unlock()
}

func (m *Manager) replaceSystemRoutes(routes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		m.systemRoutes = append([]string(nil), routes...)
		return nil
	}
	if routeStringsEqual(m.systemRoutes, routes) {
		return nil
	}
	if m.routesApplied && len(m.systemRoutes) > 0 {
		if err := m.client.SuspendRoutes(); err != nil {
			return err
		}
		m.routesApplied = false
	}
	m.systemRoutes = append([]string(nil), routes...)
	if len(m.systemRoutes) == 0 {
		return nil
	}
	if err := m.client.SetupRoutes(m.ifName, m.systemRoutes); err != nil {
		return err
	}
	m.routesApplied = true
	return nil
}

func (m *Manager) SetBypassHosts(hosts []string) {
	m.SetBypassPlan(BypassPlan{Hosts: hosts, RouteExcludes: singtun.DefaultRouteExcludePrefixes()})
}

func (m *Manager) ApplyBypass() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyBypassLocked()
}

func (m *Manager) ensureBypassHost(host string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return
	}
	for _, h := range m.bypassHosts {
		if h == host {
			return
		}
	}
	m.bypassHosts = append(m.bypassHosts, host)
	m.bypassPlan.Hosts = append(m.bypassPlan.Hosts, host)
	if !m.active || m.ifName == "" {
		return
	}
	if m.bypassApplied {
		_ = m.client.AddBypassHost(host)
	}
}

func (m *Manager) applyBypassLocked() error {
	if !m.active || m.bypassApplied || m.ifName == "" {
		return nil
	}
	if len(m.bypassHosts) == 0 && len(m.bypassNetworks) == 0 {
		m.bypassApplied = true
		return nil
	}
	if err := m.client.SetupBypass(m.bypassHosts, m.bypassNetworks); err != nil {
		return err
	}
	m.bypassApplied = true
	singtun.SetPlatform(singtun.Platform{
		FileDescriptor:        m.tunFD,
		InterfaceName:         m.ifName,
		ExternalConfiguration: true,
		BypassHosts:           append([]string(nil), m.bypassHosts...),
		RouteExcludeAddresses: append([]netip.Prefix(nil), m.bypassPlan.RouteExcludes...),
	})
	return nil
}

func (m *Manager) EnableFromConfig(raw string) error {
	p := ParseTUNParams(raw)
	name := p.Name
	if name == "" || name == "tun0" {
		name = "utun"
	}
	inet4, prefix := p.FirstInet4()
	return m.Enable(name, p.MTU, inet4, prefix)
}

func (m *Manager) Enable(name string, mtu uint32, inet4 string, prefix int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return nil
	}
	fd, ifName, err := m.client.OpenTUN(name, mtu, inet4, prefix)
	if err != nil {
		return err
	}
	m.tunFD = fd
	m.ifName = ifName
	m.tunDNSAddr = inet4
	if m.tunDNSAddr == "" {
		m.tunDNSAddr = singtun.DefaultTunIPv4
	}
	m.active = true
	m.routesApplied = false
	m.bypassApplied = false
	m.dnsApplied = false
	return m.applyBypassLocked()
}

func (m *Manager) ApplyDNS() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		return nil
	}
	if err := m.client.SetupDNS(singtun.DarwinDNSForwardIP); err != nil {
		return err
	}
	m.dnsApplied = true
	return nil
}

// RefreshDNS 重新应用系统 DNS 并重启 Helper :53 转发（转发协程异常退出后可恢复）。
func (m *Manager) RefreshDNS() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		return nil
	}
	return m.client.SetupDNS(singtun.DarwinDNSForwardIP)
}

func (m *Manager) ApplyRoutes() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.routesApplied || m.ifName == "" {
		return nil
	}
	if len(m.systemRoutes) == 0 {
		m.routesApplied = true
		return nil
	}
	if err := m.client.SetupRoutes(m.ifName, m.systemRoutes); err != nil {
		return err
	}
	m.routesApplied = true
	return nil
}

func (m *Manager) SuspendRoutes() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || !m.routesApplied {
		return nil
	}
	if err := m.client.SuspendRoutes(); err != nil {
		return err
	}
	m.routesApplied = false
	return nil
}

func (m *Manager) ResumeRoutes() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.routesApplied || m.ifName == "" {
		return nil
	}
	if err := m.client.ResumeRoutes(); err != nil {
		return err
	}
	m.routesApplied = true
	return nil
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	_ = m.client.Teardown()
	if m.tunFD > 0 {
		_ = unix.Close(m.tunFD)
	}
	singtun.ResetPlatform()
	singtun.ClearBypassDecisionCache()
	m.tunFD = 0
	m.ifName = ""
	m.active = false
	m.routesApplied = false
	m.bypassApplied = false
	m.dnsApplied = false
	m.bypassHosts = nil
	m.bypassNetworks = nil
	m.systemRoutes = nil
	m.tunDNSAddr = ""
	m.mu.Unlock()
	_ = RepairStaleSystemDNS()
	return nil
}

func (m *Manager) PrepareConfigs(rawConfigs []string) error {
	if userOptedOut() {
		return nil
	}
	for _, raw := range rawConfigs {
		if HasTUNService(raw) {
			return m.EnableFromConfig(raw)
		}
	}
	return nil
}

func (m *Manager) Cleanup() error {
	if err := m.Disable(); err != nil {
		return err
	}
	return RepairStaleSystemDNS()
}
