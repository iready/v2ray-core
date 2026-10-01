package rvstore

import "encoding/json"

// LogProfile 本地 v2ray log 覆盖偏好（优先于服务端下发的 log 段）。
type LogProfile struct {
	Use      bool    `json:"use"`
	Loglevel *string `json:"loglevel,omitempty"`
	Access   *string `json:"access,omitempty"`
	Error    *string `json:"error,omitempty"`
}

func DefaultLogProfile() LogProfile {
	return LogProfile{Use: false}
}

func normalizeLogProfile(p LogProfile) LogProfile {
	return p
}

func (p LogProfile) Clone() LogProfile {
	out := LogProfile{Use: p.Use}
	if p.Loglevel != nil {
		v := *p.Loglevel
		out.Loglevel = &v
	}
	if p.Access != nil {
		v := *p.Access
		out.Access = &v
	}
	if p.Error != nil {
		v := *p.Error
		out.Error = &v
	}
	return out
}

func (p LogProfile) OverrideMap() map[string]interface{} {
	out := map[string]interface{}{}
	if p.Loglevel != nil {
		out["loglevel"] = *p.Loglevel
	}
	if p.Access != nil {
		out["access"] = *p.Access
	}
	if p.Error != nil {
		out["error"] = *p.Error
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseLoglevelFromV2JSON 从 v2fly JSON 读取 log.loglevel。
func ParseLoglevelFromV2JSON(raw string) string {
	if raw == "" {
		return ""
	}
	var doc struct {
		Log map[string]interface{} `json:"log"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil || doc.Log == nil {
		return ""
	}
	if v, ok := doc.Log["loglevel"].(string); ok {
		return v
	}
	return ""
}
