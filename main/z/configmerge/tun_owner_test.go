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

func TestCNQUICGoesDirectBeforeBlackhole(t *testing.T) {
	raw := `{
		"services": {"tun": {"tag": "tun-in"}},
		"outbounds": [
			{"protocol": "vmess", "tag": "proxy"},
			{"protocol": "freedom", "tag": "direct"}
		],
		"routing": {"rules": [
			{"type": "field", "domain": ["geosite:gfw"], "outboundTag": "proxy"}
		]}
	}`
	once, err := ApplyTunOwnerPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := ApplyTunOwnerPolicy(once)
	if err != nil {
		t.Fatal(err)
	}
	directAt, blockAt := quicRuleIndexes(t, twice)
	if directAt < 0 || blockAt < 0 || directAt >= blockAt {
		t.Fatalf("cn quic direct=%d blackhole=%d, want direct first", directAt, blockAt)
	}
	if a, b := quicRuleIndexes(t, once); a != directAt || b != blockAt {
		t.Fatalf("rule moved on second apply: once %d,%d twice %d,%d", a, b, directAt, blockAt)
	}
}

func quicRuleIndexes(t *testing.T, raw string) (directAt, blockAt int) {
	t.Helper()
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	routing, _ := doc["routing"].(map[string]interface{})
	rules, _ := routing["rules"].([]interface{})
	directAt, blockAt = -1, -1
	for i, item := range rules {
		rule, _ := item.(map[string]interface{})
		if !isTunQUICRule(rule, "tun-in") {
			continue
		}
		switch asString(rule["outboundTag"]) {
		case "direct":
			if !ruleHasIP(rule, "geoip:cn") {
				t.Fatalf("direct quic rule missing geoip:cn: %+v", rule)
			}
			directAt = i
		case "blackhole":
			blockAt = i
		}
	}
	return directAt, blockAt
}
