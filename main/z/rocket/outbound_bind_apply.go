package rocket

import (
	"fmt"

	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/main/z/configmerge"
)

func reloadServerConfig(server *ServerInstance, raw string) error {
	loadJSON, err := configmerge.StripRocketTunFieldsForLoad(raw)
	if err != nil {
		loadJSON = raw
	}
	cfg, err := core.LoadConfig(core.FormatJSON, []byte(loadJSON))
	if err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	server.RawJSON = raw
	server.Config = cfg
	return nil
}

func applyOutboundBindToServer(server *ServerInstance, iface string) error {
	if server == nil || iface == "" {
		return nil
	}
	bound, err := configmerge.ApplyOutboundBind(server.RawJSON, iface)
	if err != nil {
		return err
	}
	if bound == server.RawJSON {
		return nil
	}
	return reloadServerConfig(server, bound)
}
