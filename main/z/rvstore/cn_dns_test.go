package rvstore

import (
	"runtime"
	"strings"
	"testing"
)

func TestDefaultCNResolverPlatform(t *testing.T) {
	got := DefaultCNResolver()
	switch runtime.GOOS {
	case "darwin":
		if !strings.HasPrefix(got, "https+local://") {
			t.Fatalf("darwin want https+local, got %s", got)
		}
	default:
		if !strings.HasPrefix(got, "tcp+local://") {
			t.Fatalf("%s want tcp+local, got %s", runtime.GOOS, got)
		}
	}
	bare := ResolveCNResolver("223.6.6.6")
	if runtime.GOOS == "darwin" {
		if bare != "https+local://223.6.6.6/dns-query" {
			t.Fatalf("got %s", bare)
		}
	} else if bare != "tcp+local://223.6.6.6:53" {
		t.Fatalf("got %s", bare)
	}
}
