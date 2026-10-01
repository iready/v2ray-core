package rocket

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	core "github.com/v2fly/v2ray-core/v5"
	v2net "github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/session"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

// ponytail: 3MB 是为了塞进 aiohttp 默认 4MB 的 websocket 帧。要更大先改服务端 max_msg_size。
const fetchMaxBody = 3 * 1024 * 1024

var fetchSlots = make(chan struct{}, 4)

// DoHTTPFetch 经本机正在跑的 v2fly 实例按路由出站，取回一段文本。
func DoHTTPFetch(ctx context.Context, rs *RS, req wire.HTTPFetchPush) wire.HTTPFetchResult {
	if strings.TrimSpace(req.ID) == "" {
		return wire.HTTPFetchResult{OK: false, Error: "id 不能为空"}
	}
	select {
	case fetchSlots <- struct{}{}:
		defer func() { <-fetchSlots }()
	case <-ctx.Done():
		return fetchFail(req.ID, "已取消")
	}

	target, err := parseFetchURL(req.URL)
	if err != nil {
		return fetchFail(req.ID, err.Error())
	}
	inst, err := fetchInstance(rs)
	if err != nil {
		return fetchFail(req.ID, err.Error())
	}

	wait := time.Duration(req.Timeout * float64(time.Second))
	if wait <= 0 || wait > 60*time.Second {
		wait = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialInstance(ctx, inst, network, addr)
		},
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true}, // ponytail: 书源和片源证书经常无效，这里只把页面当数据
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: wait,
		DisableKeepAlives:     true,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   wait,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向过多")
			}
			if next.URL.Scheme != "http" && next.URL.Scheme != "https" {
				return fmt.Errorf("只允许 http(s)")
			}
			return nil
		},
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fetchFail(req.ID, err.Error())
	}
	httpReq.Header = cleanFetchHeaders(req.Headers)
	resp, err := client.Do(httpReq)
	if err != nil {
		return fetchFail(req.ID, err.Error())
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBody+1))
	if err != nil {
		return fetchFail(req.ID, err.Error())
	}
	if len(buf) > fetchMaxBody {
		return fetchFail(req.ID, "页面超过 3MB")
	}
	body := ""
	if resp.StatusCode < 400 {
		body = strings.ToValidUTF8(string(buf), "")
	}
	return wire.HTTPFetchResult{ID: req.ID, OK: true, Status: resp.StatusCode, Body: body}
}

func fetchFail(id, msg string) wire.HTTPFetchResult {
	return wire.HTTPFetchResult{ID: id, OK: false, Error: msg}
}

func parseFetchURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("只允许 http(s)")
	}
	return u, nil
}

func fetchInstance(rs *RS) (*core.Instance, error) {
	if rs == nil {
		return nil, fmt.Errorf("实例未运行")
	}
	snap := rs.ServerSnapshot()
	if srv := snap[rs.Status().TunOwnerKey]; instanceUp(srv) {
		return srv.V2flyServer, nil
	}
	keys := make([]string, 0, len(snap))
	for key := range snap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if instanceUp(snap[key]) {
			return snap[key].V2flyServer, nil
		}
	}
	return nil, fmt.Errorf("没有正在运行的实例")
}

func instanceUp(srv *ServerInstance) bool {
	return srv != nil && srv.Status == pb.ServerStatus_STARTED && srv.V2flyServer != nil
}

func dialInstance(ctx context.Context, inst *core.Instance, network, addr string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("只支持 tcp")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("端口无效")
	}
	dest := v2net.TCPDestination(v2net.ParseAddress(host), v2net.Port(port))
	content := new(session.Content)
	content.SkipDNSResolve = true
	ctx = session.ContextWithContent(ctx, content)
	return core.Dial(ctx, inst, dest)
}

func cleanFetchHeaders(in map[string]string) http.Header {
	out := make(http.Header)
	n := 0
	for key, value := range in {
		if n >= 16 {
			break
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || len(key) > 64 || len(value) > 4096 {
			continue
		}
		if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			continue
		}
		switch strings.ToLower(key) {
		case "host", "content-length", "transfer-encoding", "connection":
			continue
		}
		out.Set(key, value)
		n++
	}
	if out.Get("User-Agent") == "" {
		out.Set("User-Agent", "Mozilla/5.0")
	}
	return out
}
