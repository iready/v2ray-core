//go:build darwin

package helper

import (
	"strings"
	"testing"
)

func TestRouteArgsWithSrc(t *testing.T) {
	got := routeArgsWithSrc([]string{"-n", "change", "-host", "1.2.3.4", "10.0.0.1"}, "10.0.0.8")
	want := "-n change -ifa 10.0.0.8 -host 1.2.3.4 10.0.0.1"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %q", strings.Join(got, " "))
	}
	plain := routeArgsWithSrc([]string{"-n", "add", "-host", "1.2.3.4", "10.0.0.1"}, "")
	if strings.Join(plain, " ") != "-n add -host 1.2.3.4 10.0.0.1" {
		t.Fatalf("empty src got %q", strings.Join(plain, " "))
	}
}

func TestRouteNotInTable(t *testing.T) {
	if !routeNotInTable("change host 1.2.3.4: not in table") {
		t.Fatal("missing route")
	}
	if routeNotInTable("File exists") {
		t.Fatal("exists is not missing")
	}
}
