package tunctl

import "testing"

func TestIsEphemeralListenAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"0.0.0.0:0", true},
		{"[::]:0", true},
		{"127.0.0.1:53535", false},
		{"127.0.0.1:1091", false},
		{"0.0.0.0:1080", false},
		{"bad", false},
	}
	for _, tc := range cases {
		if got := isEphemeralListenAddr(tc.addr); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.addr, got, tc.want)
		}
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:53535", true},
		{"127.0.0.1:0", true},
		{"localhost:53", true},
		{"0.0.0.0:0", false},
		{"8.8.8.8:53", false},
		{"[::1]:53", true},
	}
	for _, tc := range cases {
		if got := isLoopbackAddr(tc.addr); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.addr, got, tc.want)
		}
	}
}

func TestDialBindCacheFollowsUpdate(t *testing.T) {
	t.Cleanup(DisableDialBind)
	DisableDialBind()

	bindIfaceMu.Lock()
	bindIfaceManual = ""
	bindIfaceAuto = true
	bindIfaceSnap = "en0"
	bindIfaceMu.Unlock()

	EnableDialBind("en0")
	if got := getDialBindIface(); got != "en0" {
		t.Fatalf("seed: got %q", got)
	}
	onDialBindIfaceUpdate("en1")
	if got := getDialBindIface(); got != "en1" {
		t.Fatalf("after update: got %q", got)
	}
	if got := CurrentBindInterface(); got != "en1" {
		t.Fatalf("CurrentBindInterface: got %q", got)
	}
	onDialBindIfaceUpdate("")
	if got := getDialBindIface(); got != "en1" {
		t.Fatalf("empty update should keep last: got %q", got)
	}
}

func TestLanBypassSkipsTunSubnet(t *testing.T) {
	for _, cidr := range lanBypassNetworks() {
		if cidr == "172.16.0.0/12" || cidr == "172.19.0.0/16" {
			t.Fatalf("lan bypass must not cover TUN subnet, got %s", cidr)
		}
	}
}
