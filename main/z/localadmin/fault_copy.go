package localadmin

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/connlog"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

// CaptureContext 在生命周期变化/低频采样时提取 TUN 和 routing，禁止在出站热路径调用。
func CaptureContext(store *Store, rs *rocket.RS) (string, string) {
	tunObj := map[string]any{}
	if store != nil {
		if p, err := store.LoadTunProfile(); err == nil {
			tunObj["profile"] = p
		}
	}
	st := tunctl.Default().Status()
	tunObj["runtime"] = map[string]any{
		"enabled": st.Enabled,
		"if_name": st.IfName,
		"error":   st.Error,
		"hijack":  tunctl.HijackSnapshotNow(),
	}
	tunBytes, _ := json.MarshalIndent(tunObj, "", "  ")

	routes := map[string]json.RawMessage{}
	if rs != nil {
		servers, version := rs.ServerSnapshotVersion()
		tunObj["config_version"] = version
		tunBytes, _ = json.MarshalIndent(tunObj, "", "  ")
		keys := make([]string, 0, len(servers))
		for key := range servers {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			srv := servers[key]
			if srv == nil || srv.RawJSON == "" {
				continue
			}
			var doc map[string]json.RawMessage
			if json.Unmarshal([]byte(srv.RawJSON), &doc) != nil {
				continue
			}
			if len(doc["routing"]) == 0 {
				continue
			}
			routes[key] = doc["routing"]
		}
	}
	routeBytes, _ := json.MarshalIndent(routes, "", "  ")
	return string(tunBytes), string(routeBytes)
}

func (h *Handler) GetFaultCopy(c *gin.Context) {
	i, err := strconv.Atoi(c.DefaultQuery("i", "0"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "i"})
		return
	}
	text, ok := connlog.CopyText(i)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "没有这条异常"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"text": text})
}

func (h *Handler) PostFaultClear(c *gin.Context) {
	connlog.ClearFaults()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
