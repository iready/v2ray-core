package singtun

import (
	"context"
	"errors"
	"io"
	stdlog "log"
	"net"
	"net/netip"
	"os"
	"runtime/debug"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/proxyman"
	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/common/log"
	v2net "github.com/v2fly/v2ray-core/v5/common/net"
	udp_proto "github.com/v2fly/v2ray-core/v5/common/protocol/udp"
	"github.com/v2fly/v2ray-core/v5/common/session"
	"github.com/v2fly/v2ray-core/v5/common/signal"
	"github.com/v2fly/v2ray-core/v5/common/task"
	"github.com/v2fly/v2ray-core/v5/features/outbound"
	"github.com/v2fly/v2ray-core/v5/features/policy"
	"github.com/v2fly/v2ray-core/v5/features/routing"
	"github.com/v2fly/v2ray-core/v5/main/z/applog"
	"github.com/v2fly/v2ray-core/v5/transport/internet/udp"

	stun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing-tun/ping"
	singbuf "github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type Handler struct {
	ctx           context.Context
	dispatcher    routing.Dispatcher
	router        routing.Router
	outbound      outbound.Manager
	policyManager policy.Manager
	tag           string
	userLevel     uint32
	sniffing      *proxyman.SniffingConfig
	excludeSet    *prefixSet
	hostIndex     *hostBypassIndex
	tunAddrSet    map[netip.Addr]struct{}
}

func NewHandler(ctx context.Context, dispatcher routing.Dispatcher, router routing.Router, outboundManager outbound.Manager, policyManager policy.Manager, input ConfigInput, exclude []netip.Prefix, bypassHosts []string, tunAddrs []netip.Addr) *Handler {
	tunSet := make(map[netip.Addr]struct{}, len(tunAddrs))
	for _, a := range tunAddrs {
		tunSet[a] = struct{}{}
	}
	return &Handler{
		ctx:           ctx,
		dispatcher:    dispatcher,
		router:        router,
		outbound:      outboundManager,
		policyManager: policyManager,
		tag:           input.Tag,
		userLevel:     input.UserLevel,
		sniffing:      input.SniffingSettings,
		excludeSet:    newPrefixSet(exclude),
		hostIndex:     buildHostBypassIndex(bypassHosts),
		tunAddrSet:    tunSet,
	}
}

func (h *Handler) PrepareConnection(network string, source M.Socksaddr, destination M.Socksaddr, routeContext stun.DirectRouteContext, timeout time.Duration) (stun.DirectRouteDestination, error) {
	destAddr := destination.Addr
	// 网页刷新会打大量 QUIC(UDP/443) 与局域网发现包；进 NAT/出站后再 blackhole 仍会占满 CPU/套接字。
	if network == N.NetworkUDP && shouldDropUDP(destination) {
		return nil, stun.ErrDrop
	}
	// FakeDNS 池必须进 userspace（嗅探改写域名）；gvisor 对 ErrBypass 会 RST → connection refused。
	if IsFakeDNSAddr(destAddr) {
		return nil, nil
	}
	if (destAddr.IsValid() && h.shouldBypass(destAddr)) || h.shouldBypassPreMatch(network, source, destination) {
		if network == N.NetworkICMP {
			return ping.ConnectDestination(context.Background(), nopLogger{}, nil, destAddr, routeContext, timeout)
		}
		return nil, stun.ErrBypass
	}
	return nil, nil
}

func shouldDropUDP(destination M.Socksaddr) bool {
	addr := destination.Addr
	if addr.IsValid() && (addr.IsMulticast() || addr.IsLinkLocalMulticast() || addr.IsLinkLocalUnicast() || addr.IsUnspecified()) {
		return true
	}
	return false
}

func udpIdle(destination M.Socksaddr) time.Duration {
	if destination.Port == 53 || destination.Port == 123 {
		return udpOneShot
	}
	return udpTimeout
}

func (h *Handler) shouldBypass(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if h.isTunInterfaceAddr(addr) {
		return false
	}
	if bypass, ok := globalBypassCache.lookup(addr); ok {
		return bypass
	}
	bypass := h.excludeSet.Contains(addr) || h.hostIndex.contains(addr)
	globalBypassCache.store(addr, bypass)
	return bypass
}

func (h *Handler) isTunInterfaceAddr(addr netip.Addr) bool {
	_, ok := h.tunAddrSet[addr]
	return ok
}

func (h *Handler) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	go func() {
		defer func() {
			if onClose != nil {
				onClose(nil)
			}
		}()
		defer recoverConn("tcp", destination, conn)
		// gvisor 握手是懒的：出站拨号结束前不会 SYN-ACK。代理一慢，对端 connect 就停在 SYN，直到调用方超时。
		handshakeAt := time.Now()
		if err := acceptTUNHandshake(conn); err != nil {
			applog.LimitedPrintf("tun_handshake_error:"+destination.String(), "tun tcp %s 握手失败 %s: %v", destination, time.Since(handshakeAt).Round(time.Millisecond), err)
			_ = conn.Close()
			return
		}
		if d := time.Since(handshakeAt); d >= 200*time.Millisecond {
			applog.LimitedPrintf("tun_handshake_slow:"+destination.String(), "tun tcp %s 握手 %s", destination, d.Round(time.Millisecond))
		}
		_ = h.handleTCP(ctx, conn, source, destination)
	}()
}

func acceptTUNHandshake(conn net.Conn) error {
	hs, ok := conn.(interface{ HandshakeSuccess() error })
	if !ok {
		return nil
	}
	return hs.HandshakeSuccess()
}

func (h *Handler) handleTCP(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr) error {
	LiveTCP.Add(1)
	defer LiveTCP.Add(-1)
	defer conn.Close()

	prepareOutboundBypass(destination.Addr)

	inboundCtx := session.ContextWithInbound(ctx, &session.Inbound{Tag: h.tag})
	sessionPolicy := h.policyManager.ForLevel(h.userLevel)

	dest := socksaddrToTCPDestination(destination)
	src := socksaddrToTCPDestination(source)
	inboundCtx = log.ContextWithAccessMessage(inboundCtx, &log.AccessMessage{
		From:   src,
		To:     dest,
		Status: log.AccessAccepted,
	})
	inboundCtx = h.withSniffing(inboundCtx)
	inboundCtx = h.withDNSHijack(inboundCtx, dest)

	inboundCtx, cancel := context.WithCancel(inboundCtx)
	defer cancel()
	timer := signal.CancelAfterInactivity(inboundCtx, cancel, sessionPolicy.Timeouts.ConnectionIdle)

	dialAt := time.Now()
	link, err := h.dispatcher.Dispatch(inboundCtx, dest)
	if d := time.Since(dialAt); err != nil || d >= time.Second {
		applog.LimitedPrintf("tun_dispatch:"+dest.String(), "tun tcp %s 出站 %s: %v", dest, d.Round(time.Millisecond), err)
	}
	if err != nil {
		return err
	}

	responseDone := func() error {
		defer timer.SetTimeout(sessionPolicy.Timeouts.UplinkOnly)
		if err := buf.Copy(link.Reader, buf.NewWriter(conn), buf.UpdateActivity(timer), buf.CountAtomic(&DownloadBytes)); err != nil {
			return err
		}
		return nil
	}
	requestDone := func() error {
		defer timer.SetTimeout(sessionPolicy.Timeouts.DownlinkOnly)
		if err := buf.Copy(buf.NewReader(conn), link.Writer, buf.UpdateActivity(timer), buf.CountAtomic(&UploadBytes)); err != nil {
			return err
		}
		return nil
	}
	requestDoneAndCloseWriter := task.OnSuccess(requestDone, task.Close(link.Writer))
	if err := task.Run(inboundCtx, requestDoneAndCloseWriter, responseDone); err != nil {
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
		return err
	}
	return nil
}

func (h *Handler) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	go func() {
		defer func() {
			if onClose != nil {
				onClose(nil)
			}
		}()
		defer recoverConn("udp", destination, conn)
		_ = h.handleUDP(ctx, conn, source, destination)
	}()
}

// recoverConn 单条连接 panic 只关这条；进程退出会留下指向本机的系统 DNS，整机断网。
func recoverConn(network string, destination M.Socksaddr, conn io.Closer) {
	if r := recover(); r != nil {
		stdlog.Printf("panic tun %s %s: %v\n%s", network, destination, r, debug.Stack())
		_ = conn.Close()
	}
}

func (h *Handler) handleUDP(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr) error {
	liveUDP.Add(1)
	defer liveUDP.Add(-1)
	defer conn.Close()
	idle := udpIdle(destination)
	if idle == udpOneShot {
		if s, ok := conn.(interface{ SetTimeout(time.Duration) bool }); ok {
			s.SetTimeout(idle)
		}
	}
	// 取消上下文并关闭 SplitDispatcher，否则出站 ListenPacket 套接字会永久泄漏。
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	inboundCtx := session.ContextWithInbound(ctx, &session.Inbound{Tag: h.tag})
	inboundCtx = h.withSniffing(inboundCtx)

	// DNS/NTP 短回收；其余 UDP 跟 udpTimeout，避免长连接静默几秒就被拆掉重连。
	udpServer := udp.NewSplitDispatcherWithIdle(h.dispatcher, func(ctx context.Context, packet *udp_proto.Packet) {
		defer packet.Payload.Release()
		buffer := singbuf.NewPacket()
		defer buffer.Release()
		_, _ = buffer.Write(packet.Payload.Bytes())
		if err := conn.WritePacket(buffer, source); err == nil {
			DownloadBytes.Add(uint64(packet.Payload.Len()))
		}
	}, idle)
	defer common.Close(udpServer)

	_ = conn.SetReadDeadline(time.Now().Add(idle))
	for {
		buffer := singbuf.NewPacket()
		remote, err := conn.ReadPacket(buffer)
		if err != nil {
			buffer.Release()
			if errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, os.ErrDeadlineExceeded) {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(idle))
		UploadBytes.Add(uint64(buffer.Len()))
		payload := buf.NewWithSize(int32(buffer.Len()))
		_, _ = payload.Write(buffer.Bytes())
		buffer.Release()
		dest := socksaddrToUDPDestination(remote)
		// TUN UDP 量极大，跳过 per-packet access 日志（仍走 routing / DNS hijack）。
		pktCtx := h.withDNSHijack(inboundCtx, dest)
		udpServer.Dispatch(pktCtx, dest, payload)
	}
}

func (h *Handler) withSniffing(ctx context.Context) context.Context {
	content := new(session.Content)
	if h.sniffing != nil {
		content.SniffingRequest.Enabled = h.sniffing.Enabled
		content.SniffingRequest.OverrideDestinationForProtocol = h.sniffing.DestinationOverride
		content.SniffingRequest.MetadataOnly = h.sniffing.MetadataOnly
	}
	return session.ContextWithContent(ctx, content)
}

func (h *Handler) withDNSHijack(ctx context.Context, dest v2net.Destination) context.Context {
	if dest.Port != 53 {
		return ctx
	}
	if dest.Network != v2net.Network_UDP && dest.Network != v2net.Network_TCP {
		return ctx
	}
	// 国内 DNS 上游：不可强制 dns-out，否则 TCP 探测/直连查询被吞成 connection refused。
	if ip := dest.Address.IP(); len(ip) == 4 || len(ip) == 16 {
		if ip4 := ip.To4(); ip4 != nil && ip4[0] == 223 && ip4[1] == 5 && ip4[2] == 5 && ip4[3] == 5 {
			return ctx
		}
	}
	return session.SetForcedOutboundTagToContext(ctx, TunDNSOutboundTag)
}

func socksaddrToTCPDestination(addr M.Socksaddr) v2net.Destination {
	return v2net.TCPDestination(v2net.IPAddress(addr.Addr.AsSlice()), v2net.Port(addr.Port))
}

func socksaddrToUDPDestination(addr M.Socksaddr) v2net.Destination {
	return v2net.UDPDestination(v2net.IPAddress(addr.Addr.AsSlice()), v2net.Port(addr.Port))
}
