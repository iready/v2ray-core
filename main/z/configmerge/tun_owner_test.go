package configmerge

import (
	"encoding/json"
	"testing"
)

func TestApplyTunOwnerPolicyFakeDNSHasDedicatedProxyRule(t *testing.T) {
	raw := `{
		"services": {"tun": {"tag": "tun-in"}},
		"outbounds": [
			{"protocol": "vmess", "tag": "proxy"},
			{"protocol": "freedom", "tag": "direct"}
		],
		"routing": {
			"rules": [
				{"type": "field", "domain": ["geosite:gfw"], "outboundTag": "proxy"},
				{"type": "field", "inboundTag": ["tun-in"], "ip": ["geoip:!cn"], "domain": ["geosite:gfw"], "outboundTag": "proxy"},
				{"type": "field", "inboundTag": ["tun-in"], "outboundTag": "direct"}
			]
		}
	}`
	out, err := ApplyTunOwnerPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	routing, _ := doc["routing"].(map[string]interface{})
	rules, _ := routing["rules"].([]interface{})
	if !hasTunFakeDNSProxyRule(rules, "tun-in", "proxy") {
		t.Fatalf("missing dedicated FakeDNS proxy rule in %s", out)
	}
	for _, item := range rules {
		rule, _ := item.(map[string]interface{})
		if !ruleHasIP(rule, tunFakeDNSPool) {
			continue
		}
		if rule["domain"] != nil || rule["domains"] != nil {
			t.Fatalf("FakeDNS pool must not share a domain-constrained rule: %+v", rule)
		}
	}
}
