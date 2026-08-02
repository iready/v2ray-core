package rvstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"google.golang.org/protobuf/proto"
)

const (
	dirName       = ".rv"
	agentFileName = "agent.json"
	systemDir     = "/root/.rv"
)

// V2Cache v2ray 配置缓存（原 config.dat 内容）。
type V2Cache struct {
	Token      string `json:"token,omitempty"`
	ConfigByte []byte `json:"config_byte,omitempty"`
	Version    int32  `json:"version,omitempty"`
}

// Runtime agent 运行时状态。
type Runtime struct {
	PID       int    `json:"pid,omitempty"`
	AdminPort int    `json:"admin_port,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

// AgentFile ~/.rv/agent.json 完整结构。
type AgentFile struct {
	Connection Connection  `json:"connection"`
	Tun        TunProfile  `json:"tun,omitempty"`
	LogProfile LogProfile  `json:"log_profile,omitempty"`
	Mitm       MitmProfile `json:"mitm,omitempty"`
	Cache      V2Cache     `json:"cache,omitempty"`
	Runtime    Runtime     `json:"runtime,omitempty"`
}

// Store 读写 ~/.rv/agent.json。
type Store struct {
	mu sync.Mutex
}

func NewStore() *Store { return &Store{} }

func ConfigDir() string {
	if useSystemConfigDir() {
		return systemDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return systemDir
	}
	return filepath.Join(home, dirName)
}

func AgentPath() string {
	return filepath.Join(ConfigDir(), agentFileName)
}

func useSystemConfigDir() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("HOME") == "" {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	if _, err := os.Stat(home); os.IsNotExist(err) {
		return true
	}
	return false
}

func (s *Store) Load() (*AgentFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() (*AgentFile, error) {
	path := AgentPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AgentFile{Connection: DefaultConnection()}, nil
		}
		return nil, err
	}
	// 空文件视同不存在：旧 WriteFile 在 SIGKILL 下会留下 0 字节截断结果。
	if len(bytes.TrimSpace(data)) == 0 {
		return &AgentFile{Connection: DefaultConnection()}, nil
	}
	var file AgentFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse agent.json: %w", err)
	}
	file.Connection = normalizeConnection(file.Connection)
	file.Tun = normalizeTunProfile(file.Tun)
	file.LogProfile = normalizeLogProfile(file.LogProfile)
	file.Mitm = normalizeMitmProfile(file.Mitm)
	return &file, nil
}

func (s *Store) Save(file *AgentFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(file)
}

func (s *Store) saveLocked(file *AgentFile) error {
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(AgentPath(), data, 0600)
}

// writeFileAtomic 先写同目录临时文件再 rename，避免 O_TRUNC 后被强杀留下空 agent.json。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (s *Store) LoadConnection() (Connection, error) {
	file, err := s.Load()
	if err != nil {
		return Connection{}, err
	}
	return file.Connection, nil
}

func (s *Store) SaveConnection(conn Connection) error {
	if err := conn.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	file.Connection = conn
	return s.saveLocked(file)
}

func (s *Store) LoadTunProfile() (TunProfile, error) {
	file, err := s.Load()
	if err != nil {
		return TunProfile{}, err
	}
	return file.Tun, nil
}

func (s *Store) SaveTunProfile(profile TunProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	file.Tun = normalizeTunProfile(profile)
	return s.saveLocked(file)
}

func (s *Store) LoadLogProfile() (LogProfile, error) {
	file, err := s.Load()
	if err != nil {
		return LogProfile{}, err
	}
	return file.LogProfile, nil
}

func (s *Store) SaveLogProfile(profile LogProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	file.LogProfile = normalizeLogProfile(profile)
	return s.saveLocked(file)
}

func (s *Store) LoadMitmProfile() (MitmProfile, error) {
	file, err := s.Load()
	if err != nil {
		return MitmProfile{}, err
	}
	return file.Mitm, nil
}

func (s *Store) SaveMitmProfile(profile MitmProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	file.Mitm = normalizeMitmProfile(profile)
	return s.saveLocked(file)
}

func (s *Store) SaveRuntime(rt Runtime) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	file.Runtime = rt
	return s.saveLocked(file)
}

func (s *Store) HasConfigCache() bool {
	file, err := s.Load()
	if err != nil {
		return false
	}
	return len(file.Cache.ConfigByte) > 0
}

func (s *Store) LoadConfigCache() (*pb.SaveConf, error) {
	file, err := s.Load()
	if err != nil {
		return nil, err
	}
	if len(file.Cache.ConfigByte) == 0 {
		return nil, os.ErrNotExist
	}
	key := encryptionKey(file.Cache.Token)
	decrypted, err := decryptConfig(file.Cache.ConfigByte, key)
	if err != nil {
		return nil, fmt.Errorf("decrypt config: %w", err)
	}
	var config pb.GetConfigRes
	if err := proto.Unmarshal(decrypted, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &pb.SaveConf{
		Token:  file.Cache.Token,
		Config: &config,
	}, nil
}

func (s *Store) SaveConfigCache(config *pb.GetConfigRes, token string) error {
	configData, err := proto.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	encrypted, err := encryptConfig(configData, encryptionKey(token))
	if err != nil {
		return fmt.Errorf("encrypt config: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	file.Cache = V2Cache{
		Token:      token,
		ConfigByte: encrypted,
		Version:    config.Version,
	}
	return s.saveLocked(file)
}

func NewRuntimeRecord(pid, adminPort int) Runtime {
	return Runtime{
		PID:       pid,
		AdminPort: adminPort,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
}
