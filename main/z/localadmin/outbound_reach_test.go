package localadmin

import (
	"context"
	"testing"

	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
)

func TestResolveReachOutboundLocal(t *testing.T) {
	rs := &rocket.RS{Servers: map[string]*rocket.ServerInstance{
		"alpha": {Key: "alpha", RawJSON: `{"outbounds":[{"tag":"direct","protocol":"freedom"},{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"1.2.3.4","port":443}]}}]}`},
	}}
	if _, msg := resolveReachOutbound(rs, "alpha", "direct"); msg != "本地出站不测" {
		t.Fatalf("direct: %q", msg)
	}
	if _, msg := resolveReachOutbound(rs, "alpha", "missing"); msg != "出站不存在" {
		t.Fatalf("missing: %q", msg)
	}
	ob, msg := resolveReachOutbound(rs, "alpha", "proxy")
	if msg != "" || !ob.Remote {
		t.Fatalf("proxy: msg=%q %+v", msg, ob)
	}
}

func TestProbeOneOutboundInstanceDown(t *testing.T) {
	item := probeOneOutbound(context.Background(), nil, rocket.OutboundSnapshot{
		ServerKey: "cs-kj-client",
		Tag:       "kj_out_fadan",
		Remote:    true,
	})
	if item.ProtocolCN.OK || item.ProtocolForeign.OK {
		t.Fatal("protocol expected fail")
	}
	if item.ProtocolCN.Error != "实例未运行" {
		t.Fatalf("protocol_cn error=%q", item.ProtocolCN.Error)
	}
	if item.ProtocolForeign.Error != "实例未运行" {
		t.Fatalf("protocol_foreign error=%q", item.ProtocolForeign.Error)
	}
	if item.TCP.OK {
		t.Fatal("tcp expected fail")
	}
	if item.TCP.Error != "无远端地址" {
		t.Fatalf("tcp error=%q", item.TCP.Error)
	}
	if item.ServerKey != "cs-kj-client" || item.Tag != "kj_out_fadan" {
		t.Fatalf("%+v", item)
	}
}
