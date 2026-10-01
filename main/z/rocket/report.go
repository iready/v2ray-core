package rocket

import (
	"context"
	"log"
	"time"
)

const maxReportSummary = 500
const maxReportError = 1000

func truncateReport(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

func serverInstanceHealthy(srv *ServerInstance) bool {
	return srv != nil && srv.V2flyServer != nil
}

func buildServerStatusList(servers map[string]*ServerInstance) []map[string]any {
	if len(servers) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(servers))
	for key, srv := range servers {
		if srv == nil {
			continue
		}
		ok := serverInstanceHealthy(srv)
		item := map[string]any{
			"key": key,
			"ok":  ok,
		}
		if !ok && srv.LastError != "" {
			item["error"] = truncateReport(srv.LastError, maxReportError)
		}
		out = append(out, item)
	}
	return out
}

// ReportAgentStatus 向 Rocket 上报 agent 运行状态（异步，不阻塞调用方）。
func (rs *RS) ReportAgentStatus(healthy bool, summary string, servers map[string]*ServerInstance) {
	if rs == nil || rs.wireClient == nil || rs.Token == "" {
		return
	}
	payload := map[string]any{
		"token":   rs.Token,
		"version": rs.TokenVersion,
		"healthy": healthy,
		"summary": truncateReport(summary, maxReportSummary),
	}
	if list := buildServerStatusList(servers); len(list) > 0 {
		payload["servers"] = list
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := rs.wireClient.ReportStatus(ctx, payload); err != nil {
			log.Printf("状态上报失败: %v", err)
		}
	}()
}

func (rs *RS) reportAfterStart(attempted map[string]*ServerInstance, startedCount, total int) {
	if startedCount == 0 {
		rs.ReportAgentStatus(false, "failed to start any server instances", attempted)
		return
	}
	if startedCount < total {
		summary := truncateReport(
			"failed to start "+itoa(total-startedCount)+"/"+itoa(total)+" server instances",
			maxReportSummary,
		)
		rs.ReportAgentStatus(false, summary, attempted)
		return
	}
	rs.ReportAgentStatus(true, "", rs.Servers)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
