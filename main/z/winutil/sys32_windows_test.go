package winutil

import (
	"strings"
	"testing"
)

func TestSystem32(t *testing.T) {
	if p := System32("route"); !strings.HasSuffix(strings.ToLower(p), `\system32\route.exe`) {
		t.Fatalf("route: %s", p)
	}
	if p := System32("powershell"); !strings.Contains(strings.ToLower(p), `\windowspowershell\v1.0\powershell.exe`) {
		t.Fatalf("powershell: %s", p)
	}
}
