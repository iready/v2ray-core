package rocket

import (
	"log"
	"strings"

	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

// ListTunOwnerCandidates 返回下发配置中带 services.tun 模板的 key（保持下发顺序）。
func ListTunOwnerCandidates(items []*pb.ConfigItem) []string {
	var out []string
	for _, item := range items {
		if item == nil || item.Key == "" {
			continue
		}
		if tunctl.HasTUNService(item.Config) {
			out = append(out, item.Key)
		}
	}
	return out
}

// ResolveTunOwner 解析唯一 TUN 拥有者；profile.Use 为 false 时返回空 key。
// 远端无 services.tun 模板时，若本地 profile.Services 可注入 TUN，则所有实例 key 均为候选。
func ResolveTunOwner(profile rvstore.TunProfile, items []*pb.ConfigItem) (key string, candidates []string) {
	candidates = ListTunOwnerCandidates(items)
	if profile.Use && len(candidates) == 0 && localTunInjectable(profile) {
		candidates = listAllConfigKeys(items)
	}
	if !profile.Use || len(candidates) == 0 {
		return "", candidates
	}
	want := strings.TrimSpace(profile.TunOwnerKey)
	if want != "" {
		for _, c := range candidates {
			if c == want {
				return want, candidates
			}
		}
		log.Printf("TUN 拥有者 %q 无效或无可用的 services.tun，回退为 %q", want, candidates[0])
		return candidates[0], candidates
	}
	return candidates[0], candidates
}

func localTunInjectable(profile rvstore.TunProfile) bool {
	return profile.Services != nil && len(profile.Services) > 0
}

func listAllConfigKeys(items []*pb.ConfigItem) []string {
	var out []string
	for _, item := range items {
		if item != nil && item.Key != "" {
			out = append(out, item.Key)
		}
	}
	return out
}
