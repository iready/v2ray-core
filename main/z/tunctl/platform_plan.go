package tunctl

import (
	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// PlatformPlan 汇总 bypass 与 OS 层 TUN 路由计划。
type PlatformPlan struct {
	Bypass BypassPlan
	Route  RoutePlan
}

// BuildPlatformPlan 按 TunProfile、Wire 与 v2fly 配置生成完整 TUN 平台计划。
func BuildPlatformPlan(profile rvstore.TunProfile, wireURL string, configJSONs ...string) PlatformPlan {
	if len(configJSONs) == 0 {
		return PlatformPlan{Bypass: BuildBypassPlan(profile, wireURL)}
	}
	return BuildPlatformPlanForOwner(profile, wireURL, configJSONs[0], configJSONs...)
}

// BuildPlatformPlanForOwner 路由计划仅取自 ownerRaw；绕行合并全部实例出站地址。
func BuildPlatformPlanForOwner(profile rvstore.TunProfile, wireURL string, ownerRaw string, allRaws ...string) PlatformPlan {
	bypassJSONs := allRaws
	if len(bypassJSONs) == 0 && ownerRaw != "" {
		bypassJSONs = []string{ownerRaw}
	}
	bypass := BuildBypassPlan(profile, wireURL, bypassJSONs...)
	route := RoutePlan{Mode: RouteModeProxyList}
	if ownerRaw != "" && HasTUNService(ownerRaw) {
		route = BuildRoutePlan(ownerRaw)
	}
	return PlatformPlan{Bypass: bypass, Route: route}
}

// SetPlatformPlan 配置 bypass 与系统路由计划。
func SetPlatformPlan(plan PlatformPlan) {
	defaultManager.SetBypassPlan(plan.Bypass)
	defaultManager.SetRoutePlan(plan.Route)
}

// ConfigureSingtunProxyList 按拥有者配置启用 TUN PreMatch（proxy_list）。
func ConfigureSingtunProxyList(ownerRaw string) {
	enabled := false
	if ownerRaw != "" && HasTUNService(ownerRaw) {
		params := ParseTUNParams(ownerRaw)
		enabled = !deriveProxyRuleSet(ownerRaw, params.Tag).empty()
	}
	singtun.SetProxyListMode(enabled)
	singtun.ClearBypassDecisionCache()
	if enabled {
		singtun.SetOutboundBypassPrepare(prepareChinaOutboundBypass)
		ConfigureChinaDNSBypassHook(true)
	} else {
		singtun.SetOutboundBypassPrepare(nil)
		ConfigureChinaDNSBypassHook(false)
	}
}
