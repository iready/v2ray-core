package rocket

import "testing"

func TestParseOutbounds(t *testing.T) {
	raw := `{
		"outbounds": [
			{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"1.2.3.4","port":443}]},"streamSettings":{"network":"tcp","security":"tls"}},
			{"tag":"direct","protocol":"freedom","settings":{"redirect":"127.0.0.1:9"}},
			{"protocol":"blackhole","tag":"block"},
			{"tag":"ss","protocol":"shadowsocks","settings":{"servers":[{"address":"node.example.com","port":8388}]}}
		]
	}`
	got := ParseOutbounds("alpha", raw)
	if len(got) != 4 {
		t.Fatalf("len=%d want 4: %+v", len(got), got)
	}
	if got[0].Tag != "proxy" || got[0].Protocol != "vless" || got[0].Address != "1.2.3.4" || got[0].Port != 443 || !got[0].Remote {
		t.Fatalf("proxy: %+v", got[0])
	}
	if got[0].Network != "tcp" || got[0].Security != "tls" || got[0].ServerKey != "alpha" {
		t.Fatalf("proxy stream: %+v", got[0])
	}
	if got[1].Remote || got[1].Address != "127.0.0.1" || got[1].Port != 9 {
		t.Fatalf("freedom: %+v", got[1])
	}
	if got[2].Remote {
		t.Fatalf("blackhole should not be remote: %+v", got[2])
	}
	if got[3].Address != "node.example.com" || got[3].Port != 8388 || !got[3].Remote {
		t.Fatalf("ss: %+v", got[3])
	}
	remote := RemoteOnly(got)
	if len(remote) != 2 || remote[0].Tag != "proxy" || remote[1].Tag != "ss" {
		t.Fatalf("remote: %+v", remote)
	}
}

func TestFindOutbound(t *testing.T) {
	rs := &RS{Servers: map[string]*ServerInstance{
		"alpha": {Key: "alpha", RawJSON: `{
			"outbounds": [
				{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"1.2.3.4","port":443}]}},
				{"tag":"direct","protocol":"freedom"}
			]
		}`},
	}}
	ob, ok := FindOutbound(rs, "alpha", "proxy")
	if !ok || !ob.Remote || ob.Protocol != "vless" {
		t.Fatalf("proxy: ok=%v %+v", ok, ob)
	}
	ob, ok = FindOutbound(rs, "alpha", "direct")
	if !ok || ob.Remote {
		t.Fatalf("direct: ok=%v %+v", ok, ob)
	}
	if _, ok := FindOutbound(rs, "alpha", "missing"); ok {
		t.Fatal("missing should not hit")
	}
}
