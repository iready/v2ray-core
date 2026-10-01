package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
)

const pingInterval = 60 * time.Second

var errClosed = errors.New("wire: closed")

type rpcReply struct {
	result json.RawMessage
	err    error
}

// Client Rocket v2fly_wire JSON-RPC 客户端（单读协程）。
type Client struct {
	token string
	sign  string

	mu   sync.Mutex
	conn *websocket.Conn

	writeMu sync.Mutex
	nextID  atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan rpcReply

	pumpCtx    context.Context
	pumpCancel context.CancelFunc
	pumpDone   chan error
	pumpOnce   sync.Once

	onConfigPush       func(*ConfigPayload)
	onExecutePush      func([]*pb.Execute)
	onDomainStatusPush func(DomainRouteStatusPush)
	onHTTPFetch        func(HTTPFetchPush)
}

// CapHTTPFetch 与 Rocket v2fly_wire.registry.CAP_HTTP_FETCH 成对。
const CapHTTPFetch = "http_fetch"

// ClientCapabilities 本客户端向 Rocket 声明的能力。只在这里列出。
func ClientCapabilities() []string {
	return []string{CapHTTPFetch}
}

// HTTPFetchPush Rocket 推来的一次页面抓取。
type HTTPFetchPush struct {
	ID      string            `json:"id"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Timeout float64           `json:"timeout"`
}

// HTTPFetchResult 抓取回执。
type HTTPFetchResult struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Body   string `json:"body,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Dial 建立 WebSocket。需随后调用 Start() 再发 RPC。
func Dial(wsURL, token, sign string, tls *TLSConfig) (*Client, error) {
	wsURL = normalizeWSURL(wsURL)
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}
	if !IsWSS(wsURL) {
		tls = nil
	}
	dialer := &websocket.Dialer{HandshakeTimeout: 45 * time.Second}
	if tls != nil {
		goTLS, err := tls.goTLS(u.Hostname())
		if err != nil {
			return nil, err
		}
		dialer.TLSClientConfig = goTLS
	}
	conn, _, err := dialer.Dial(u.String(), http.Header{})
	if err != nil {
		return nil, err
	}
	c := &Client{
		token:   token,
		sign:    sign,
		conn:    conn,
		pending: make(map[int64]chan rpcReply),
	}
	c.nextID.Store(1)
	return c, nil
}

// Start 启动唯一读协程，同一 Client 只执行一次。读协程生命周期与 Close 绑定，不受 hello 超时 ctx 影响。
func (c *Client) Start() {
	c.pumpOnce.Do(func() {
		c.pumpCtx, c.pumpCancel = context.WithCancel(context.Background())
		c.pumpDone = make(chan error, 1)
		go c.readPump()
	})
}

// Wait 阻塞直到 ctx 取消或读协程因断线退出。
func (c *Client) Wait(ctx context.Context) error {
	if c.pumpDone == nil {
		return fmt.Errorf("wire: Start not called")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c.pumpDone:
		if err == nil {
			return fmt.Errorf("wire: connection closed")
		}
		return err
	}
}

func (c *Client) Close() error {
	if c.pumpCancel != nil {
		c.pumpCancel()
	}
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	c.failAllPending(errClosed)
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (c *Client) Hello(ctx context.Context, machine string) (*ConfigPayload, error) {
	var res ConfigPayload
	err := c.call(ctx, "v2fly.agent.hello", map[string]any{
		"token": c.token, "sign": c.sign, "machine": machine,
		"capabilities": ClientCapabilities(),
	}, &res)
	if err != nil {
		return nil, err
	}
	if !res.Ok {
		return &res, fmt.Errorf("hello: %s", res.Msg)
	}
	return &res, nil
}

func (c *Client) Ping(ctx context.Context, version int32) error {
	var res struct {
		Ok bool `json:"ok"`
	}
	return c.call(ctx, "v2fly.agent.ping", map[string]any{
		"token": c.token, "version": version,
	}, &res)
}

func (c *Client) UploadExecuteResult(ctx context.Context, id, msg string, ok bool) error {
	var res struct {
		Ok bool `json:"ok"`
	}
	return c.call(ctx, "v2fly.execute.result", map[string]any{
		"id": id, "msg": msg, "ok": ok,
	}, &res)
}

func (c *Client) ReportStatus(ctx context.Context, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["token"] = c.token
	var res struct {
		Ok bool `json:"ok"`
	}
	return c.call(ctx, "v2fly.agent.report", payload, &res)
}

// DomainRouteRequestResult 服务端受理回执。
type EditableServerSummary struct {
	ID               string `json:"id"`
	Key              string `json:"key"`
	Address          string `json:"address"`
	Version          int64  `json:"version"`
	TokenVersion     int64  `json:"token_version"`
	ClientVersion    *int64 `json:"client_version"`
	Online           bool   `json:"online"`
	Healthy          bool   `json:"healthy"`
	LastError        string `json:"last_error"`
	Synced           bool   `json:"synced"`
	SharedTokenCount int    `json:"shared_token_count"`
}

type EditableServerOwner struct {
	Name      string `json:"name"`
	Contact   string `json:"contact"`
	ManagedBy string `json:"managed_by"`
}

type EditableServerList struct {
	Owner EditableServerOwner     `json:"owner"`
	Rows  []EditableServerSummary `json:"rows"`
}

type EditableServerDocument struct {
	ID           string              `json:"id"`
	Key          string              `json:"key"`
	Address      string              `json:"address"`
	Version      int64               `json:"version"`
	OutboundTags []string            `json:"outbound_tags"`
	Owner        EditableServerOwner `json:"owner"`
	Config       json.RawMessage     `json:"config"`
}

type EditableServerSaveResult struct {
	ServerID            string `json:"server_id"`
	Version             int64  `json:"version"`
	TokenVersion        int64  `json:"token_version"`
	Online              bool   `json:"online"`
	OnlineClientVersion *int64 `json:"online_client_version"`
	Pushed              int    `json:"pushed"`
	OfflineTokens       []struct {
		Name string `json:"name"`
	} `json:"offline_tokens"`
}

func (c *Client) ListEditableServers(ctx context.Context) (*EditableServerList, error) {
	var result EditableServerList
	if err := c.call(ctx, "v2fly.agent.server_edit.list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetEditableServer(ctx context.Context, id string) (*EditableServerDocument, error) {
	var result EditableServerDocument
	if err := c.call(ctx, "v2fly.agent.server_edit.get", map[string]any{"server_id": id}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SaveEditableServer(ctx context.Context, id string, version int64, config any) (*EditableServerSaveResult, error) {
	var result EditableServerSaveResult
	if err := c.call(ctx, "v2fly.agent.server_edit.save", map[string]any{"server_id": id, "version": version, "config": config}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DomainRouteRequestResult 服务端受理回执。
type DomainRouteRequestResult struct {
	Ok        bool   `json:"ok"`
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Msg       string `json:"msg"`
}

// DomainRouteStatusPush 审批结果推送。
type DomainRouteStatusPush struct {
	RequestID    string   `json:"request_id"`
	ClientReqID  string   `json:"client_req_id"`
	Status       string   `json:"status"`
	RejectReason string   `json:"reject_reason"`
	Added        int      `json:"added"`
	Skipped      int      `json:"skipped"`
	RouteNames   []string `json:"route_names"`
	Domains      []string `json:"domains"`
}

func (c *Client) SubmitDomainRouteRequest(ctx context.Context, domains []string, remark, clientReqID string) (*DomainRouteRequestResult, error) {
	var res DomainRouteRequestResult
	err := c.call(ctx, "v2fly.agent.domain_route_request", map[string]any{
		"token":         c.token,
		"domains":       domains,
		"remark":        remark,
		"client_req_id": clientReqID,
	}, &res)
	if err != nil {
		return nil, err
	}
	if !res.Ok {
		msg := res.Msg
		if msg == "" {
			msg = "domain_route_request failed"
		}
		return &res, fmt.Errorf("%s", msg)
	}
	return &res, nil
}

func (c *Client) SetPushHandlers(onConfig func(*ConfigPayload), onExecute func([]*pb.Execute), onDomainStatus func(DomainRouteStatusPush), onHTTPFetch func(HTTPFetchPush)) {
	c.onConfigPush = onConfig
	c.onExecutePush = onExecute
	c.onDomainStatusPush = onDomainStatus
	c.onHTTPFetch = onHTTPFetch
}

func (c *Client) UploadHTTPResult(ctx context.Context, result HTTPFetchResult) error {
	var res struct {
		Ok bool `json:"ok"`
	}
	return c.call(ctx, "v2fly.http.result", result, &res)
}

func (c *Client) StartPingLoop(ctx context.Context, version func() int32) {
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				_ = c.Ping(pctx, version())
				cancel()
			}
		}
	}()
}

func (c *Client) readPump() {
	var err error
	defer func() {
		c.failAllPending(errClosed)
		if c.pumpDone != nil {
			select {
			case c.pumpDone <- err:
			default:
			}
		}
	}()

	for {
		if c.pumpCtx != nil {
			select {
			case <-c.pumpCtx.Done():
				err = c.pumpCtx.Err()
				return
			default:
			}
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			err = errClosed
			return
		}

		_, data, readErr := conn.ReadMessage()
		if readErr != nil {
			err = readErr
			return
		}
		c.safeDispatch(data)
	}
}

// safeDispatch 推送处理（配置热更新、远程指令）panic 时丢掉这一帧，读循环继续。
func (c *Client) safeDispatch(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic wire 推送: %v\n%s", r, debug.Stack())
		}
	}()
	c.dispatchFrame(data)
}

func (c *Client) dispatchFrame(data []byte) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil {
		return
	}
	if rawMethod, ok := envelope["method"]; ok && len(rawMethod) > 0 && string(rawMethod) != "null" {
		var method string
		if json.Unmarshal(rawMethod, &method) == nil && method != "" {
			c.dispatchPush(data)
			return
		}
	}

	var rid int64
	if rawID, ok := envelope["id"]; ok && rawID != nil {
		_ = json.Unmarshal(rawID, &rid)
	}
	if rid == 0 {
		return
	}

	c.pendingMu.Lock()
	ch, ok := c.pending[rid]
	if ok {
		delete(c.pending, rid)
	}
	c.pendingMu.Unlock()
	if !ok {
		return
	}

	reply := rpcReply{}
	if errRaw, ok := envelope["error"]; ok && errRaw != nil {
		var errObj struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errRaw, &errObj)
		if errObj.Message != "" {
			reply.err = fmt.Errorf("wire: %s", errObj.Message)
		} else {
			reply.err = fmt.Errorf("wire: rpc error")
		}
	} else if resRaw, ok := envelope["result"]; ok {
		reply.result = resRaw
	} else {
		reply.err = fmt.Errorf("wire: missing result")
	}
	ch <- reply
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("wire: not connected")
	}

	reqID := c.nextID.Add(1)
	respCh := make(chan rpcReply, 1)
	c.pendingMu.Lock()
	c.pending[reqID] = respCh
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	writeErr := conn.WriteJSON(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      reqID,
	})
	c.writeMu.Unlock()
	if writeErr != nil {
		c.pendingMu.Lock()
		delete(c.pending, reqID)
		c.pendingMu.Unlock()
		return writeErr
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, reqID)
		c.pendingMu.Unlock()
		return ctx.Err()
	case reply := <-respCh:
		if reply.err != nil {
			return reply.err
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(reply.result, result)
	}
}

func (c *Client) failAllPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- rpcReply{err: err}:
		default:
		}
		delete(c.pending, id)
	}
}

func (c *Client) dispatchPush(data []byte) {
	var msg struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if json.Unmarshal(data, &msg) != nil {
		return
	}
	switch msg.Method {
	case "v2fly.config.push":
		if c.onConfigPush == nil {
			return
		}
		var payload ConfigPayload
		if json.Unmarshal(msg.Params, &payload) == nil {
			c.onConfigPush(&payload)
		}
	case "v2fly.execute.push":
		if c.onExecutePush == nil {
			return
		}
		var body struct {
			Executes []executeWire `json:"executes"`
		}
		if json.Unmarshal(msg.Params, &body) == nil {
			c.onExecutePush(executesToProto(body.Executes))
		}
	case "v2fly.domain_route_request.status":
		if c.onDomainStatusPush == nil {
			return
		}
		var body DomainRouteStatusPush
		if json.Unmarshal(msg.Params, &body) == nil {
			c.onDomainStatusPush(body)
		}
	case "v2fly.http.fetch":
		if c.onHTTPFetch == nil {
			return
		}
		var body HTTPFetchPush
		if json.Unmarshal(msg.Params, &body) != nil || body.ID == "" {
			return
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("panic http fetch: %v\n%s", r, debug.Stack())
				}
			}()
			c.onHTTPFetch(body)
		}()
	}
}
