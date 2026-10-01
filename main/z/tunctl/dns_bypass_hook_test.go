package tunctl

import "testing"

func TestClassifyBypassDropsDomainName(t *testing.T) {
	got := classifyBypassEntries([]string{"apple.com", "1.2.3.4"})
	for _, host := range got.Hosts {
		if host == "apple.com" {
			t.Fatal("domain must not be a route host")
		}
	}
	found := false
	for _, host := range got.Hosts {
		if host == "1.2.3.4" {
			found = true
		}
	}
	if !found {
		t.Fatal("ipv4 missing")
	}
}

func TestMatchesBypassDomain(t *testing.T) {
	patterns := []string{"apple.com", "apple-cloudkit.com"}
	if !matchesBypassDomain("gsa.apple.com", patterns) {
		t.Fatal("subdomain")
	}
	if !matchesBypassDomain("apple.com", patterns) {
		t.Fatal("apex")
	}
	if matchesBypassDomain("notapple.com", patterns) {
		t.Fatal("suffix collision")
	}
}
