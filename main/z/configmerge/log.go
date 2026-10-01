package configmerge

import (
	"encoding/json"

	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// ApplyLogProfile 按本地日志偏好覆盖服务端下发的 log 段。
func ApplyLogProfile(raw string, profile rvstore.LogProfile) (string, error) {
	if !profile.Use || raw == "" {
		return raw, nil
	}
	override := profile.OverrideMap()
	if override == nil {
		return raw, nil
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}

	serverLog, _ := doc["log"].(map[string]interface{})
	doc["log"] = mergeMaps(serverLog, override)
	return marshalDoc(doc)
}
