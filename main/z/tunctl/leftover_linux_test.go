//go:build linux

package tunctl

import "testing"

func TestIsNumberedTun(t *testing.T) {
	if !isNumberedTun("tun0") || !isNumberedTun("tun124") {
		t.Fatal("tunN")
	}
	if isNumberedTun("tun") || isNumberedTun("tunl0") || isNumberedTun("eth0") || isNumberedTun("tunnel") {
		t.Fatal("non tunN")
	}
}
