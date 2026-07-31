package configmerge

import (
	"encoding/json"
	"strings"
)

var skipBindProtocols = map[string]struct{}{
	"blackhole": {},
	"dns":       {},
	"loopback":  {},
}

// ApplyOutboundBind 为出站（含 freedom/direct 与代理）注入 streamSettings.sockopt.bindToDevice，避免 TUN 二次捕获环路。
func ApplyOutboundBind(raw string, iface string) (string, error) {
	iface = strings.TrimSpace(iface)
	if raw == "" || iface == "" {
		return raw, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}
	obRaw, ok := doc["outbounds"]
	if !ok {
		return raw, nil
	}
	outbounds, ok := obRaw.([]interface{})
	if !ok {
		return raw, nil
	}
	changed := false
	for i, item := range outbounds {
		ob, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !applyBindToOutbound(ob, iface) {
			continue
		}
		outbounds[i] = ob
		changed = true
	}
	if !changed {
		return raw, nil
	}
	doc["outbounds"] = outbounds
	return marshalDoc(doc)
}

func applyBindToOutbound(ob map[string]interface{}, iface string) bool {
	protocol, _ := ob["protocol"].(string)
	if _, skip := skipBindProtocols[strings.ToLower(strings.TrimSpace(protocol))]; skip {
		return false
	}
	stream, _ := ob["streamSettings"].(map[string]interface{})
	if stream == nil {
		stream = map[string]interface{}{}
		ob["streamSettings"] = stream
	}
	sockopt, _ := stream["sockopt"].(map[string]interface{})
	if sockopt == nil {
		sockopt = map[string]interface{}{}
		stream["sockopt"] = sockopt
	}
	if existing, _ := sockopt["bindToDevice"].(string); strings.TrimSpace(existing) != "" {
		return false
	}
	if existing, _ := sockopt["bind_to_device"].(string); strings.TrimSpace(existing) != "" {
		return false
	}
	sockopt["bindToDevice"] = iface
	return true
}

// RemoveOutboundBind 移除 Rocket 注入的 bindToDevice（TUN 降级时避免残留绑网卡）。
func RemoveOutboundBind(raw string) (string, error) {
	if raw == "" {
		return raw, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}
	obRaw, ok := doc["outbounds"]
	if !ok {
		return raw, nil
	}
	outbounds, ok := obRaw.([]interface{})
	if !ok {
		return raw, nil
	}
	changed := false
	for i, item := range outbounds {
		ob, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !removeBindFromOutbound(ob) {
			continue
		}
		outbounds[i] = ob
		changed = true
	}
	if !changed {
		return raw, nil
	}
	doc["outbounds"] = outbounds
	return marshalDoc(doc)
}

func removeBindFromOutbound(ob map[string]interface{}) bool {
	stream, _ := ob["streamSettings"].(map[string]interface{})
	if stream == nil {
		return false
	}
	sockopt, _ := stream["sockopt"].(map[string]interface{})
	if sockopt == nil {
		return false
	}
	_, hasCamel := sockopt["bindToDevice"]
	_, hasSnake := sockopt["bind_to_device"]
	if !hasCamel && !hasSnake {
		return false
	}
	delete(sockopt, "bindToDevice")
	delete(sockopt, "bind_to_device")
	if len(sockopt) == 0 {
		delete(stream, "sockopt")
	}
	return true
}
