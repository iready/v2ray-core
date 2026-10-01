package tunctl

import "testing"

func TestHijackWanted(t *testing.T) {
	cases := []struct {
		in   hijackInputs
		want bool
	}{
		{hijackInputs{enabled: true}, true},
		{hijackInputs{}, false},
		{hijackInputs{enabled: true, wireDialing: true}, false},
		{hijackInputs{enabled: true, ifaceGone: true}, false},
		{hijackInputs{enabled: true, cnOutage: true}, false},
		{hijackInputs{enabled: true, wireDialing: true, cnOutage: true}, false},
	}
	for i, tc := range cases {
		if got := hijackWanted(tc.in); got != tc.want {
			t.Fatalf("#%d %+v: got %v want %v", i, tc.in, got, tc.want)
		}
	}
}

func TestHijackPausedReason(t *testing.T) {
	SetUserDisabled(false)
	t.Cleanup(func() { SetUserDisabled(false) })
	cases := []struct {
		in   hijackInputs
		want string
	}{
		{hijackInputs{enabled: true}, ""},
		{hijackInputs{enabled: true, wireDialing: true}, HijackPausedWireDial},
		{hijackInputs{enabled: true, ifaceGone: true}, HijackPausedIfaceGone},
		{hijackInputs{enabled: true, cnOutage: true}, HijackPausedCNOutage},
		{hijackInputs{}, HijackPausedEngineOff},
	}
	for i, tc := range cases {
		if got := hijackPausedReason(tc.in); got != tc.want {
			t.Fatalf("#%d %+v: got %q want %q", i, tc.in, got, tc.want)
		}
	}
	SetUserDisabled(true)
	if got := hijackPausedReason(hijackInputs{}); got != HijackPausedUserOff {
		t.Fatalf("opted out: got %q want %q", got, HijackPausedUserOff)
	}
}

func TestCNOutageFollowsGateway(t *testing.T) {
	if cnOutageActive(true, "192.168.1.1", "192.168.1.1") != true {
		t.Fatal("same gateway should keep the outage")
	}
	if cnOutageActive(true, "192.168.1.1", "192.168.8.1") {
		t.Fatal("new gateway invalidates the outage")
	}
	if cnOutageActive(true, "192.168.1.1 192.168.1.9", "192.168.1.1 192.168.1.20") {
		t.Fatal("same gateway, new local address invalidates the outage")
	}
	if cnOutageActive(true, "192.168.1.1", "") {
		t.Fatal("missing gateway is not this outage")
	}
	if cnOutageActive(false, "192.168.1.1", "192.168.1.1") {
		t.Fatal("successful judgement is not an outage")
	}
}
