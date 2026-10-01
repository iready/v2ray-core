package singtun

import (
	"testing"

	stun "github.com/sagernet/sing-tun"
)

// 热更新回归：ip rule 优先级 / 路由表必须固定为 sing-tun 默认值，
// 否则 Linux 上规则落在优先级 0，Close 清不掉，下次 Start 报 "add rule 0/9: file exists"。
func TestBuildOptionsFixedIPRoute2Indexes(t *testing.T) {
	opts, _, _, _, _ := BuildOptions(ConfigInput{})
	if opts.IPRoute2RuleIndex != stun.DefaultIPRoute2RuleIndex {
		t.Fatalf("IPRoute2RuleIndex=%d want %d", opts.IPRoute2RuleIndex, stun.DefaultIPRoute2RuleIndex)
	}
	if opts.IPRoute2TableIndex != stun.DefaultIPRoute2TableIndex {
		t.Fatalf("IPRoute2TableIndex=%d want %d", opts.IPRoute2TableIndex, stun.DefaultIPRoute2TableIndex)
	}
	if opts.IPRoute2AutoRedirectFallbackRuleIndex != stun.DefaultIPRoute2AutoRedirectFallbackRuleIndex {
		t.Fatalf("IPRoute2AutoRedirectFallbackRuleIndex=%d", opts.IPRoute2AutoRedirectFallbackRuleIndex)
	}
}
