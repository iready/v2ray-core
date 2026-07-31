package localadmin

import (
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

type (
	AddressPreset = rvstore.AddressPreset
	TLSConfig     = rvstore.TLSConfig
	AgentConfig   = rvstore.Connection
	TunProfile    = rvstore.TunProfile
	LogProfile    = rvstore.LogProfile
)

// Store 读写 ~/.rv/agent.json connection/runtime 段。
type Store struct {
	inner *rvstore.Store
}

func NewStore() *Store {
	return &Store{inner: rvstore.NewStore()}
}

func AgentConfigDir() string  { return rvstore.ConfigDir() }
func AgentConfigPath() string { return rvstore.AgentPath() }

func DefaultConfig() AgentConfig { return rvstore.DefaultConnection() }

func (s *Store) Load() (AgentConfig, error) {
	return s.inner.LoadConnection()
}

func (s *Store) Save(cfg AgentConfig) error {
	return s.inner.SaveConnection(cfg)
}

func (s *Store) LoadTunProfile() (TunProfile, error) {
	return s.inner.LoadTunProfile()
}

func (s *Store) SaveTunProfile(profile TunProfile) error {
	return s.inner.SaveTunProfile(profile)
}

func (s *Store) LoadLogProfile() (LogProfile, error) {
	return s.inner.LoadLogProfile()
}

func (s *Store) SaveLogProfile(profile LogProfile) error {
	return s.inner.SaveLogProfile(profile)
}

func (s *Store) LoadRuntime() rvstore.Runtime {
	file, err := s.inner.Load()
	if err != nil {
		return rvstore.Runtime{}
	}
	return file.Runtime
}

func (s *Store) SaveRuntime(rt rvstore.Runtime) error {
	return s.inner.SaveRuntime(rt)
}

func (s *Store) RVStore() *rvstore.Store {
	return s.inner
}
