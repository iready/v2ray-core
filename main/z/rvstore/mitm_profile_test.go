package rvstore

import (
	"slices"
	"testing"
)

func TestEffectiveIgnoreHostsMediaBypass(t *testing.T) {
	p := MitmProfile{
		IgnoreHosts: []string{"localhost"},
		MediaBypass: true,
	}
	got := p.EffectiveIgnoreHosts()
	if !slices.Contains(got, "localhost") || !slices.Contains(got, "qpic.cn") {
		t.Fatalf("got %#v", got)
	}
	p.MediaBypass = false
	got = p.EffectiveIgnoreHosts()
	if slices.Contains(got, "qpic.cn") {
		t.Fatalf("should not include media when off: %#v", got)
	}
}
