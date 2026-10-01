package localadmin

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

type outboundReachReq struct {
	ServerKey string `json:"server_key"`
	Tag       string `json:"tag"`
}

// ProbeLeg 一次探测的结果。TCP 通和协议通是两件事。
type ProbeLeg struct {
	OK        bool   `json:"ok"`
	LatencyMs int    `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// OutboundReach 指定 outbound 的 TCP 与协议探测；不是实例健康。
// 协议分国内/境外两腿，不猜节点地理、不合成一个「协议通」。
type OutboundReach struct {
	ServerKey       string   `json:"server_key"`
	Tag             string   `json:"tag"`
	TCP             ProbeLeg `json:"tcp"`
	ProtocolCN      ProbeLeg `json:"protocol_cn"`
	ProtocolForeign ProbeLeg `json:"protocol_foreign"`
}

func (h *Handler) PostOutboundReach(c *gin.Context) {
	var req outboundReachReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ServerKey = strings.TrimSpace(req.ServerKey)
	req.Tag = strings.TrimSpace(req.Tag)
	if req.ServerKey == "" || req.Tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要 server_key 和 tag"})
		return
	}
	rs := h.hooks.GetRS()
	ob, errMsg := resolveReachOutbound(rs, req.ServerKey, req.Tag)
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, probeOneOutbound(ctx, rs, ob))
}

func resolveReachOutbound(rs *rocket.RS, serverKey, tag string) (rocket.OutboundSnapshot, string) {
	ob, ok := rocket.FindOutbound(rs, serverKey, tag)
	if !ok {
		return rocket.OutboundSnapshot{}, "出站不存在"
	}
	if !ob.Remote {
		return rocket.OutboundSnapshot{}, "本地出站不测"
	}
	return ob, ""
}

func probeOneOutbound(ctx context.Context, rs *rocket.RS, ob rocket.OutboundSnapshot) OutboundReach {
	var tcp, cn, foreign ProbeLeg
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		tcp = probeTCP(ctx, ob)
	}()
	go func() {
		defer wg.Done()
		cn = probeProtocol(ctx, rs, ob.ServerKey, ob.Tag, rocket.ProbeOutboundCN)
	}()
	go func() {
		defer wg.Done()
		foreign = probeProtocol(ctx, rs, ob.ServerKey, ob.Tag, rocket.ProbeOutboundForeign)
	}()
	wg.Wait()
	return OutboundReach{ServerKey: ob.ServerKey, Tag: ob.Tag, TCP: tcp, ProtocolCN: cn, ProtocolForeign: foreign}
}

func probeTCP(ctx context.Context, ob rocket.OutboundSnapshot) ProbeLeg {
	if ob.Address == "" || ob.Port <= 0 {
		return ProbeLeg{Error: "无远端地址"}
	}
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	conn, err := tunctl.DialPhysicalTCP(dialCtx, net.JoinHostPort(ob.Address, strconv.Itoa(ob.Port)))
	if conn != nil {
		_ = conn.Close()
	}
	return latencyLeg(start, err)
}

func probeProtocol(ctx context.Context, rs *rocket.RS, serverKey, tag string, probe func(context.Context, *rocket.RS, string, string) error) ProbeLeg {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	return latencyLeg(start, probe(dialCtx, rs, serverKey, tag))
}

func latencyLeg(start time.Time, err error) ProbeLeg {
	ms := int(time.Since(start).Milliseconds())
	if ms < 1 {
		ms = 1
	}
	if err != nil {
		return ProbeLeg{LatencyMs: ms, Error: err.Error()}
	}
	return ProbeLeg{OK: true, LatencyMs: ms}
}
