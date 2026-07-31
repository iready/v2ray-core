package configmerge

import (
	"encoding/json"
	"net/netip"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// rocketOnlyTunFields 由 main/z/tunctl 消费，不能传给 core.LoadConfig。
var rocketOnlyTunFields = []string{"route_mode", "routeMode"}

// StripRocketTunFieldsForLoad 在 v2ray 解析前移除 services.tun 中的 Rocket 扩展字段。
func StripRocketTunFieldsForLoad(raw string) (string, error) {
	if raw == "" {
		return raw, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}
	services, _ := doc["services"].(map[string]interface{})
	if services == nil {
		return raw, nil
	}
	tun, _ := services["tun"].(map[string]interface{})
	if tun == nil {
		return raw, nil
	}
	changed := false
	for _, key := range rocketOnlyTunFields {
		if _, ok := tun[key]; ok {
			delete(tun, key)
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	return marshalDoc(doc)
}

// RemoveTUNService 从 v2fly JSON 移除 services.tun（多实例时仅保留一个 TUN 拥有者）。
func RemoveTUNService(raw string) (string, error) {
	if raw == "" {
		return raw, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}
	if !removeTunFromDoc(doc) {
		return raw, nil
	}
	return marshalDoc(doc)
}

func removeTunFromDoc(doc map[string]interface{}) bool {
	services, _ := doc["services"].(map[string]interface{})
	if services == nil {
		return false
	}
	if _, ok := services["tun"]; !ok {
		return false
	}
	delete(services, "tun")
	if len(services) == 0 {
		delete(doc, "services")
	} else {
		doc["services"] = services
	}
	return true
}

// ApplyTunProfileNonOwner 非 TUN 拥有者：启用 TUN 时也不注入 services.tun。
func ApplyTunProfileNonOwner(raw string, profile rvstore.TunProfile) (string, error) {
	if !profile.Use {
		return ApplyTunProfile(raw, profile)
	}
	return RemoveTUNService(raw)
}

// ApplyTunProfile 按本地 TUN 偏好合并服务端下发的 v2fly JSON。
func ApplyTunProfile(raw string, profile rvstore.TunProfile) (string, error) {
	if raw == "" {
		return raw, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}

	services, _ := doc["services"].(map[string]interface{})
	if services == nil {
		services = map[string]interface{}{}
	}

	serverTun, _ := services["tun"].(map[string]interface{})

	if !profile.Use {
		if removeTunFromDoc(doc) {
			return marshalDoc(doc)
		}
		return raw, nil
	}

	var effective map[string]interface{}
	if profile.Services != nil {
		effective = mergeMaps(serverTun, profile.Services)
	} else if serverTun != nil {
		effective = serverTun
	}
	if effective != nil {
		normalizeTunSniffingSettings(effective)
		injectClientTunIPs(effective, profile)
		services["tun"] = effective
		doc["services"] = services
	} else {
		delete(services, "tun")
		if len(services) == 0 {
			delete(doc, "services")
		} else {
			doc["services"] = services
		}
	}

	return marshalDoc(doc)
}

func marshalDoc(doc map[string]interface{}) (string, error) {
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// injectClientTunIPs 服务端不下发 ips；由客户端默认或 TunProfile.Services 覆盖注入。
func injectClientTunIPs(tun map[string]interface{}, profile rvstore.TunProfile) {
	if hasTunIPs(tun) {
		return
	}
	ipv4, prefix := singtun.DefaultTunIPv4, singtun.DefaultTunPrefix
	if profile.Services != nil {
		if local, ok := profile.Services["ips"].([]interface{}); ok && len(local) > 0 {
			if ip, pr, ok := parseTunIPsEntry(local[0]); ok {
				tun["ips"] = []map[string]interface{}{{"ip": ip, "prefix": pr}}
				return
			}
		}
	}
	octets, ok := ipv4Octets(ipv4)
	if !ok {
		return
	}
	tun["ips"] = []map[string]interface{}{{"ip": octets, "prefix": prefix}}
}

func hasTunIPs(tun map[string]interface{}) bool {
	ips, ok := tun["ips"].([]interface{})
	return ok && len(ips) > 0
}

func parseTunIPsEntry(v interface{}) (ip []int, prefix int, ok bool) {
	item, ok := v.(map[string]interface{})
	if !ok {
		return nil, 0, false
	}
	raw, ok := item["ip"].([]interface{})
	if !ok || len(raw) != 4 {
		return nil, 0, false
	}
	ip = make([]int, 4)
	for i := range ip {
		switch n := raw[i].(type) {
		case float64:
			ip[i] = int(n)
		case int:
			ip[i] = n
		default:
			return nil, 0, false
		}
		if ip[i] < 0 || ip[i] > 255 {
			return nil, 0, false
		}
	}
	prefix = singtun.DefaultTunPrefix
	if p, ok := item["prefix"].(float64); ok {
		prefix = int(p)
	} else if p, ok := item["prefix"].(int); ok {
		prefix = p
	}
	if prefix <= 0 || prefix > 32 {
		return nil, 0, false
	}
	return ip, prefix, true
}

func ipv4Octets(s string) ([]int, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil || !addr.Is4() {
		return nil, false
	}
	b := addr.As4()
	return []int{int(b[0]), int(b[1]), int(b[2]), int(b[3])}, true
}

// normalizeTunSniffingSettings 将 inbound 风格的 destOverride 转为 protobuf 的 destinationOverride。
func normalizeTunSniffingSettings(tun map[string]interface{}) {
	for _, key := range []string{"sniffingSettings", "sniffing_settings"} {
		sniff, ok := tun[key].(map[string]interface{})
		if !ok {
			continue
		}
		if _, has := sniff["destinationOverride"]; has {
			continue
		}
		if v, ok := sniff["destOverride"]; ok {
			sniff["destinationOverride"] = v
			delete(sniff, "destOverride")
		}
	}
}

func mergeMaps(base, override map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range base {
		out[k] = v
	}
	if override == nil {
		return out
	}
	for k, v := range override {
		if v == nil {
			delete(out, k)
			continue
		}
		om, okO := v.(map[string]interface{})
		bm, okB := out[k].(map[string]interface{})
		if okO && okB {
			out[k] = mergeMaps(bm, om)
			continue
		}
		out[k] = v
	}
	return out
}
