package localconfig

import (
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// Manager 本地 v2ray 配置缓存（~/.rv/agent.json cache 段）。
type Manager struct {
	inner *rvstore.Store
}

func NewManager() *Manager {
	return &Manager{inner: rvstore.NewStore()}
}

func (m *Manager) LoadConfig() (*pb.SaveConf, error) {
	return m.inner.LoadConfigCache()
}

func (m *Manager) SaveConfig(config *pb.GetConfigRes, token string) error {
	return m.inner.SaveConfigCache(config, token)
}

func (m *Manager) HasConfig() bool {
	return m.inner.HasConfigCache()
}
