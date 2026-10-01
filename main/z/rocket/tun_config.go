package rocket

import (
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

// ConfigHasTUN reports whether any running or cached v2fly JSON enables services.tun.
func ConfigHasTUN(rs *RS, cached *pb.GetConfigRes) bool {
	if rs != nil {
		for _, srv := range rs.Servers {
			if srv != nil && srv.RawJSON != "" && tunctl.HasTUNService(srv.RawJSON) {
				return true
			}
		}
	}
	if cached != nil {
		for _, item := range cached.Config {
			if item != nil && item.Config != "" && tunctl.HasTUNService(item.Config) {
				return true
			}
		}
	}
	return false
}
