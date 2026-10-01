package tunctl

import (
	"encoding/json"
	"strings"
)

type routingRule struct {
	InboundTag  json.RawMessage `json:"inboundTag"`
	OutboundTag string          `json:"outboundTag"`
	Domain      json.RawMessage `json:"domain"`
	Domains     json.RawMessage `json:"domains"`
	IP          json.RawMessage `json:"ip"`
}

func ruleMatchesTunInbound(raw json.RawMessage, tunTag string) bool {
	if len(raw) == 0 {
		return true
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return strings.TrimSpace(one) == "" || one == tunTag
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		if len(many) == 0 {
			return true
		}
		for _, tag := range many {
			if tag == tunTag {
				return true
			}
		}
		return false
	}
	return true
}

func collectIPTokens(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	return nil
}

func isDirectOutbound(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	return tag == "direct" || tag == "freedom"
}

func isBlockOutbound(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	return tag == "block" || tag == "blocked" || tag == "blackhole"
}
