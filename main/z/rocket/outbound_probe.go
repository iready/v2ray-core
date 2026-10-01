package rocket

import (
	"context"
	"crypto/tls"
	"fmt"

	core "github.com/v2fly/v2ray-core/v5"
	v2net "github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/session"
	"github.com/v2fly/v2ray-core/v5/features/outbound"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
)

// 协议探测两腿：国内直出、境外出口。不猜节点在美国还是国内。
// SkipDNSResolve：域名原样交给对端解析，避免本机 TUN FakeDNS 改成 198.18。
var outboundProbeCN = outboundProbeTarget{
	Dest: v2net.TCPDestination(v2net.ParseAddress("www.aliyun.com"), 443),
	SNI:  "www.aliyun.com",
}
var outboundProbeForeign = outboundProbeTarget{
	Dest: v2net.TCPDestination(v2net.ParseAddress("www.google.com"), 443),
	SNI:  "www.google.com",
}

type outboundProbeTarget struct {
	Dest v2net.Destination
	SNI  string
}

// ProbeOutboundCN 经 tag 握国内 HTTPS。
func ProbeOutboundCN(ctx context.Context, rs *RS, serverKey, tag string) error {
	return probeOutbound(ctx, rs, serverKey, tag, outboundProbeCN)
}

// ProbeOutboundForeign 经 tag 握境外 HTTPS。
func ProbeOutboundForeign(ctx context.Context, rs *RS, serverKey, tag string) error {
	return probeOutbound(ctx, rs, serverKey, tag, outboundProbeForeign)
}

func probeOutbound(ctx context.Context, rs *RS, serverKey, tag string, target outboundProbeTarget) error {
	inst, err := runningInstance(rs, serverKey)
	if err != nil {
		return err
	}
	ohm, ok := inst.GetFeature(outbound.ManagerType()).(outbound.Manager)
	if !ok || ohm == nil {
		return fmt.Errorf("实例未运行")
	}
	if ohm.GetHandler(tag) == nil {
		return fmt.Errorf("出站不存在")
	}

	content := new(session.Content)
	content.SkipDNSResolve = true
	ctx = session.ContextWithContent(ctx, content)
	ctx = session.SetForcedOutboundTagToContext(ctx, tag)

	conn, err := core.Dial(ctx, inst, target.Dest)
	if err != nil {
		return err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         target.SNI,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	return tlsConn.HandshakeContext(ctx)
}

func runningInstance(rs *RS, serverKey string) (*core.Instance, error) {
	if rs == nil {
		return nil, fmt.Errorf("实例未运行")
	}
	srv := rs.Servers[serverKey]
	if srv == nil || srv.Status != pb.ServerStatus_STARTED || srv.V2flyServer == nil {
		return nil, fmt.Errorf("实例未运行")
	}
	return srv.V2flyServer, nil
}
