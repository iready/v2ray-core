package rvstore

import (
	"runtime"
	"strings"
	"testing"
)

func TestDefaultCNResolversPlatform(t *testing.T) {
	got := DefaultCNResolvers()
	if len(got) != 2 {
		t.Fatalf("want 2 defaults, got %v", got)
	}
	prefix := "tcp+local://"
	if runtime.GOOS == "darwin" {
		prefix = "https+local://"
	}
	for i, addr := range got {
		if !strings.HasPrefix(addr, prefix) {
			t.Fatalf("[%d] %s want prefix %s, got %s", i, runtime.GOOS, prefix, addr)
		}
	}
	if !strings.Contains(got[0], "223.5.5.5") {
		t.Fatalf("first want 223.5.5.5, got %s", got[0])
	}
	if !strings.Contains(got[1], "223.6.6.6") {
		t.Fatalf("second want 223.6.6.6, got %s", got[1])
	}
}

func TestResolveCNResolverBareIP(t *testing.T) {
	bare := ResolveCNResolver("223.6.6.6")
	if runtime.GOOS == "darwin" {
		if bare != "https+local://223.6.6.6/dns-query" {
			t.Fatalf("got %s", bare)
		}
		return
	}
	if bare != "tcp+local://223.6.6.6:53" {
		t.Fatalf("got %s", bare)
	}
}

func TestResolveCNResolversEmptyUsesDefaults(t *testing.T) {
	got := ResolveCNResolvers(nil)
	want := DefaultCNResolvers()
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got %s want %s", i, got[i], want[i])
		}
	}
	got = ResolveCNResolvers([]string{"", "  "})
	if got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("blank entries should fall back to defaults, got %v", got)
	}
}

func TestResolveCNResolversKeepsOrderAndDedupes(t *testing.T) {
	got := ResolveCNResolvers([]string{"114.114.114.114", "223.5.5.5", "114.114.114.114"})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(got[0], "114.114.114.114") {
		t.Fatalf("first want 114.114.114.114, got %s", got[0])
	}
	if !strings.Contains(got[1], "223.5.5.5") {
		t.Fatalf("second want 223.5.5.5, got %s", got[1])
	}
}

func TestCNResolverHosts(t *testing.T) {
	hosts := CNResolverHosts(nil)
	if len(hosts) != 2 || hosts[0] != "223.5.5.5" || hosts[1] != "223.6.6.6" {
		t.Fatalf("default hosts got %v", hosts)
	}
	hosts = CNResolverHosts([]string{"1.1.1.1", "tcp+local://1.1.1.1:53"})
	if len(hosts) != 1 || hosts[0] != "1.1.1.1" {
		t.Fatalf("dedupe hosts got %v", hosts)
	}
}
