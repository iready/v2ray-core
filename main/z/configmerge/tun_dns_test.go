package configmerge

import (
	"encoding/json"
	"testing"

	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

func TestApplyTunOwnerDNSPolicyCatchAllIsDockerWhenPresent(t *testing.T) {
	orig := lookupDockerInternalDNS
	lookupDockerInternalDNS = func() string { return dockerDNSAddr }
	t.Cleanup(func() { lookupDockerInternalDNS = orig })

	out, err := ApplyTunOwnerDNSPolicy(tunDNSPolicySample, rvstore.TunProfile{})
	if err != nil {
		t.Fatal(err)
	}
	last := lastDNSServer(t, out)
	if asString(last["tag"]) != tunDockerDNSTag {
		t.Fatalf("catch-all tag=%q want %s", last["tag"], tunDockerDNSTag)
	}
	if asString(last["address"]) != dockerDNSAddr {
		t.Fatalf("catch-all address=%q", last["address"])
	}
	if last["domains"] != nil {
		t.Fatalf("Docker catch-all must not filter by hostname shape: %v", last["domains"])
	}
}

func TestExtraBypassDomainResolvesBeforeFakeDNS(t *testing.T) {
	orig := lookupDockerInternalDNS
	lookupDockerInternalDNS = func() string { return "" }
	t.Cleanup(func() { lookupDockerInternalDNS = orig })

	out, err := ApplyTunOwnerDNSPolicy(tunDNSPolicySample, rvstore.TunProfile{
		ExtraBypassHosts: []string{"apple.com", "1.2.3.4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	dns, _ := doc["dns"].(map[string]interface{})
	servers, _ := dns["servers"].([]interface{})
	cn, _ := servers[0].(map[string]interface{})
	found := false
	for _, item := range cn["domains"].([]interface{}) {
		if asString(item) == "domain:apple.com" {
			found = true
		}
		if asString(item) == "domain:1.2.3.4" {
			t.Fatal("ip must not be a dns domain")
		}
	}
	if !found {
		t.Fatal("cn dns missing domain:apple.com")
	}
	routing, _ := doc["routing"].(map[string]interface{})
	rules, _ := routing["rules"].([]interface{})
	first, _ := rules[0].(map[string]interface{})
	if asString(first["outboundTag"]) != "direct" {
		t.Fatalf("first rule outbound=%v", first["outboundTag"])
	}
}

func TestApplyTunOwnerDNSPolicyCatchAllIsRemoteWithoutDocker(t *testing.T) {
	orig := lookupDockerInternalDNS
	lookupDockerInternalDNS = func() string { return "" }
	t.Cleanup(func() { lookupDockerInternalDNS = orig })

	out, err := ApplyTunOwnerDNSPolicy(tunDNSPolicySample, rvstore.TunProfile{})
	if err != nil {
		t.Fatal(err)
	}
	last := lastDNSServer(t, out)
	if asString(last["tag"]) != tunRemoteDNSTag {
		t.Fatalf("catch-all tag=%q want %s", last["tag"], tunRemoteDNSTag)
	}
}

const tunDNSPolicySample = `{
	"services": {"tun": {"tag": "tun-in"}},
	"outbounds": [
		{"protocol": "trojan", "tag": "mg_out"},
		{"protocol": "freedom", "tag": "direct"}
	],
	"routing": {
		"rules": [
			{"type": "field", "domain": ["geosite:gfw"], "outboundTag": "mg_out"}
		]
	}
}`

func lastDNSServer(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	dns, _ := doc["dns"].(map[string]interface{})
	servers, _ := dns["servers"].([]interface{})
	if len(servers) == 0 {
		t.Fatal("no dns.servers")
	}
	last, _ := servers[len(servers)-1].(map[string]interface{})
	if last == nil {
		t.Fatal("last dns server is not an object")
	}
	return last
}
