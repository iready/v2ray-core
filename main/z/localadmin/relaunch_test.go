package localadmin

import "testing"

func TestStripRelaunchWait(t *testing.T) {
	got := stripRelaunchWait([]string{"-admin-port", "19527", "-relaunch-wait", "123", "-addr", "x"})
	want := []string{"-admin-port", "19527", "-addr", "x"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	got = stripRelaunchWait([]string{"-relaunch-wait=9", "-admin", "on"})
	if len(got) != 2 || got[0] != "-admin" || got[1] != "on" {
		t.Fatalf("equals form: %v", got)
	}
}
