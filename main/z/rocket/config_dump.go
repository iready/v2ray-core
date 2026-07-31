package rocket

import (
	"encoding/json"
	"fmt"
	"strings"

	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// ServerConfigItem 单个实例的配置视图（原始 / 本地合并后）。
type ServerConfigItem struct {
	Key        string `json:"key"`
	Version    int32  `json:"version"`
	IsTunOwner bool   `json:"is_tun_owner"`
	Raw        string `json:"raw,omitempty"`
	Merged     string `json:"merged,omitempty"`
}

// ServerConfigDump 服务端下发配置的诊断快照。
type ServerConfigDump struct {
	Version    int32              `json:"version"`
	TunUse     bool               `json:"tun_use"`
	TunOwner   string             `json:"tun_owner,omitempty"`
	Candidates []string           `json:"tun_owner_candidates,omitempty"`
	Items      []ServerConfigItem `json:"items"`
}

// BuildServerConfigDump 从 GetConfigRes 构建可打印的配置视图。
// keyFilter 为空表示全部实例；mergedOnly 为 true 时仅输出 TUN 拥有者的 merged。
func BuildServerConfigDump(cfg *pb.GetConfigRes, tun rvstore.TunProfile, log rvstore.LogProfile, includeRaw, includeMerged bool, keyFilter string, mergedOnly bool) (*ServerConfigDump, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if cfg.R != nil && cfg.R.Code != pb.Code_OK {
		return nil, fmt.Errorf("server error: %s", cfg.R.Msg)
	}
	if len(cfg.Config) == 0 {
		return nil, fmt.Errorf("no config items")
	}

	rs := &RS{
		TunProfileFunc: func() rvstore.TunProfile { return tun },
		LogProfileFunc: func() rvstore.LogProfile { return log },
	}
	ownerKey, candidates := ResolveTunOwner(tun, cfg.Config)

	out := &ServerConfigDump{
		Version:    cfg.Version,
		TunUse:     tun.Use,
		TunOwner:   ownerKey,
		Candidates: candidates,
	}

	keyFilter = strings.TrimSpace(keyFilter)
	for _, item := range cfg.Config {
		if item == nil || item.Key == "" {
			continue
		}
		if mergedOnly && item.Key != ownerKey {
			continue
		}
		if keyFilter != "" && item.Key != keyFilter {
			continue
		}
		isOwner := ownerKey != "" && item.Key == ownerKey
		entry := ServerConfigItem{
			Key:        item.Key,
			Version:    item.Version,
			IsTunOwner: isOwner,
		}
		if includeRaw {
			entry.Raw = prettyJSONOrRaw(item.Config)
		}
		if includeMerged {
			entry.Merged = prettyJSONOrRaw(rs.mergeServerJSONForItem(item.Config, isOwner))
		}
		out.Items = append(out.Items, entry)
	}
	if len(out.Items) == 0 {
		if keyFilter != "" {
			return nil, fmt.Errorf("instance %q not found", keyFilter)
		}
		return nil, fmt.Errorf("no matching config items")
	}
	return out, nil
}

func prettyJSONOrRaw(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	var doc interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw
	}
	pretty, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return raw
	}
	return string(pretty)
}

// FormatServerConfigDump 将 dump 格式化为 stdout 文本。
func FormatServerConfigDump(dump *ServerConfigDump, mergedOnly bool) string {
	if dump == nil {
		return ""
	}
	if mergedOnly && len(dump.Items) == 1 && dump.Items[0].Merged != "" {
		return dump.Items[0].Merged + "\n"
	}
	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return fmt.Sprintf("%#v\n", dump)
	}
	return string(data) + "\n"
}
