package singtun

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
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
	if destAddr.IsValid() && h.shouldBypass(destAddr) {
		if network == N.NetworkICMP {
			return ping.ConnectDestination(context.Background(), nopLogger{}, nil, destAddr, routeContext, timeout)
		}
		return nil, stun.ErrBypass
	}
	if h.shouldBypassPreMatch(network, source, destination) {
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
	// TUN 全量劫持下 udpnat LRU=1024；非 DNS UDP（QUIC/WebRTC/STUN…）会打满并持续淘汰重建出站套接字。
	// DNS 走 53；其余 UDP 在建 NAT 前丢弃（浏览主要靠 TCP）。
	if destination.Port != 53 {
		return true
	}
	return false
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
		_ = h.handleTCP(ctx, conn, source, destination)
	}()
}

func (h *Handler) handleTCP(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr) error {
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

	link, err := h.dispatcher.Dispatch(inboundCtx, dest)
	if err != nil {
		return err
	}

	responseDone := func() error {
		defer timer.SetTimeout(sessionPolicy.Timeouts.UplinkOnly)
		if err := buf.Copy(link.Reader, buf.NewWriter(conn), buf.UpdateActivity(timer)); err != nil {
			return err
		}
		return nil
	}
	requestDone := func() error {
		defer timer.SetTimeout(sessionPolicy.Timeouts.DownlinkOnly)
		if err := buf.Copy(buf.NewReader(conn), link.Writer, buf.UpdateActivity(timer)); err != nil {
			return err
		}
		return nil
	}
	requestDoneAndCloseWriter := task.OnSuccess(requestDone, task.Close(link.Writer))
	if err := task.Run(ctx, requestDoneAndCloseWriter, responseDone); err != nil {
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
		_ = h.handleUDP(ctx, conn, source, destination)
	}()
}

func (h *Handler) handleUDP(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr) error {
	defer conn.Close()
	// 取消上下文并关闭 SplitDispatcher，否则出站 ListenPacket 套接字会永久泄漏。
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	inboundCtx := session.ContextWithInbound(ctx, &session.Inbound{Tag: h.tag})
	inboundCtx = h.withSniffing(inboundCtx)

	// TUN 路径空闲回收要短：NAT 仅在有新包时 PurgeExpired，无包时靠读超时退出并 Close 出站。
	const tunUDPIdle = 30 * time.Second
	udpServer := udp.NewSplitDispatcherWithIdle(h.dispatcher, func(ctx context.Context, packet *udp_proto.Packet) {
		buffer := singbuf.NewPacket()
		defer buffer.Release()
		_, _ = buffer.Write(packet.Payload.Bytes())
		_ = conn.WritePacket(buffer, source)
	}, tunUDPIdle)
	defer common.Close(udpServer)

	_ = conn.SetReadDeadline(time.Now().Add(tunUDPIdle))
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
		_ = conn.SetReadDeadline(time.Now().Add(tunUDPIdle))
		payload := append([]byte(nil), buffer.Bytes()...)
		buffer.Release()
		dest := socksaddrToUDPDestination(remote)
		// TUN UDP 量极大，跳过 per-packet access 日志（仍走 routing / DNS hijack）。
		pktCtx := h.withDNSHijack(inboundCtx, dest)
		udpServer.Dispatch(pktCtx, dest, buf.FromBytes(payload))
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
