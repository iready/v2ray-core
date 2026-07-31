package localadmin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// GetServerConfig 返回服务端下发的 v2fly 配置（优先运行时 RS，否则读本地缓存）。
// Query:
//   - key: 实例名
//   - merged=1: 仅 TUN 拥有者，响应体为合并后的单份 JSON
//   - view=merged: 同 merged=1
//   - raw=0: 省略原始下发
//   - merged=0&view=all: 同时包含 raw 与 merged（默认）
func (h *Handler) GetServerConfig(c *gin.Context) {
	includeRaw := c.Query("raw") != "0"
	includeMerged := true
	mergedOnly := false
	switch strings.ToLower(strings.TrimSpace(c.Query("view"))) {
	case "merged":
		includeMerged = true
		mergedOnly = true
	case "raw":
		includeMerged = false
	}
	if v := strings.ToLower(strings.TrimSpace(c.Query("merged"))); v == "1" || v == "true" {
		includeMerged = true
		mergedOnly = true
	} else if v == "0" || v == "false" {
		includeMerged = false
		mergedOnly = false
	}
	keyFilter := strings.TrimSpace(c.Query("key"))

	cfg, tun, logProfile, source, err := h.loadServerConfigSource()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !includeRaw && !includeMerged {
		includeRaw = true
	}

	dump, err := rocket.BuildServerConfigDump(cfg, tun, logProfile, includeRaw, includeMerged, keyFilter, mergedOnly)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if mergedOnly && len(dump.Items) == 1 && dump.Items[0].Merged != "" {
		c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(dump.Items[0].Merged))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"source": source,
		"dump":   dump,
	})
}

func (h *Handler) loadServerConfigSource() (*pb.GetConfigRes, rvstore.TunProfile, rvstore.LogProfile, string, error) {
	file, err := h.store.RVStore().Load()
	if err != nil {
		return nil, rvstore.TunProfile{}, rvstore.LogProfile{}, "", err
	}
	tun := file.Tun
	logProfile := file.LogProfile

	if rs := h.hooks.GetRS(); rs != nil && len(rs.Servers) > 0 {
		cfg := rebuildConfigFromRS(rs)
		if cfg != nil {
			return cfg, tun, logProfile, "runtime", nil
		}
	}
	cached, err := h.store.RVStore().LoadConfigCache()
	if err != nil {
		return nil, tun, logProfile, "", fmt.Errorf("no cached server config: %w", err)
	}
	if cached == nil || cached.Config == nil {
		return nil, tun, logProfile, "", fmt.Errorf("no cached server config")
	}
	return cached.Config, tun, logProfile, "cache", nil
}

func rebuildConfigFromRS(rs *rocket.RS) *pb.GetConfigRes {
	if rs == nil || len(rs.Servers) == 0 {
		return nil
	}
	items := make([]*pb.ConfigItem, 0, len(rs.Servers))
	for key, srv := range rs.Servers {
		if srv == nil || srv.RawJSON == "" {
			continue
		}
		items = append(items, &pb.ConfigItem{
			Key:     key,
			Config:  srv.RawJSON,
			Version: srv.Version,
		})
	}
	if len(items) == 0 {
		return nil
	}
	return &pb.GetConfigRes{
		Version: rs.TokenVersion,
		Config:  items,
	}
}
