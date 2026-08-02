package mitmctl

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/lqqyt2423/go-mitmproxy/proxy"
)

const (
	defaultFlowCap = 500
	maxBodyStore   = 256 * 1024
	maxBodyPreview = 8 * 1024
)

// FlowSummary 列表项。
type FlowSummary struct {
	ID         string `json:"id"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	Host       string `json:"host"`
	StatusCode int    `json:"status_code,omitempty"`
	ReqSize    int    `json:"req_size"`
	RespSize   int    `json:"resp_size"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	StartedAt  string `json:"started_at"`
	Error      string `json:"error,omitempty"`
}

// FlowDetail 详情（含头与 body 预览）。
type FlowDetail struct {
	FlowSummary
	ReqHeaders    map[string][]string `json:"req_headers,omitempty"`
	RespHeaders   map[string][]string `json:"resp_headers,omitempty"`
	ReqBody       string              `json:"req_body,omitempty"`
	RespBody      string              `json:"resp_body,omitempty"`
	ReqBodyTrunc  bool                `json:"req_body_truncated,omitempty"`
	RespBodyTrunc bool                `json:"resp_body_truncated,omitempty"`
}

type storedFlow struct {
	sum       FlowSummary
	reqHdr    map[string][]string
	respHdr   map[string][]string
	reqBody   []byte
	respBody  []byte
	reqTrunc  bool
	respTrunc bool
}

// FlowStore 环形缓冲，供 localadmin 内嵌流量页使用。
type FlowStore struct {
	mu   sync.RWMutex
	cap  int
	seq  []string
	byID map[string]*storedFlow
}

func NewFlowStore(cap int) *FlowStore {
	if cap <= 0 {
		cap = defaultFlowCap
	}
	return &FlowStore{
		cap:  cap,
		seq:  make([]string, 0, cap),
		byID: make(map[string]*storedFlow, cap),
	}
}

func (s *FlowStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.seq)
}

func (s *FlowStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq = s.seq[:0]
	s.byID = make(map[string]*storedFlow, s.cap)
}

func (s *FlowStore) List() []FlowSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FlowSummary, 0, len(s.seq))
	for i := len(s.seq) - 1; i >= 0; i-- {
		if f := s.byID[s.seq[i]]; f != nil {
			out = append(out, f.sum)
		}
	}
	return out
}

func (s *FlowStore) Get(id string) (FlowDetail, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.byID[id]
	if !ok || f == nil {
		return FlowDetail{}, false
	}
	d := FlowDetail{
		FlowSummary:   f.sum,
		ReqHeaders:    f.reqHdr,
		RespHeaders:   f.respHdr,
		ReqBody:       previewBody(f.reqBody),
		RespBody:      previewBody(f.respBody),
		ReqBodyTrunc:  f.reqTrunc || len(f.reqBody) > maxBodyPreview,
		RespBodyTrunc: f.respTrunc || len(f.respBody) > maxBodyPreview,
	}
	return d, true
}

func (s *FlowStore) upsert(f *storedFlow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := f.sum.ID
	if _, exists := s.byID[id]; !exists {
		if len(s.seq) >= s.cap {
			old := s.seq[0]
			s.seq = s.seq[1:]
			delete(s.byID, old)
		}
		s.seq = append(s.seq, id)
	}
	s.byID[id] = f
}

func previewBody(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) > maxBodyPreview {
		b = b[:maxBodyPreview]
	}
	if utf8.Valid(b) {
		return string(b)
	}
	const hexN = 512
	n := len(b)
	if n > hexN {
		n = hexN
	}
	var sb strings.Builder
	sb.WriteString("[binary hex] ")
	for i := 0; i < n; i++ {
		sb.WriteByte("0123456789abcdef"[b[i]>>4])
		sb.WriteByte("0123456789abcdef"[b[i]&0xf])
		if i+1 < n {
			sb.WriteByte(' ')
		}
	}
	return sb.String()
}

func clipBody(b []byte) (out []byte, trunc bool) {
	if len(b) <= maxBodyStore {
		return append([]byte(nil), b...), false
	}
	return append([]byte(nil), b[:maxBodyStore]...), true
}

// storeBodyForView 按 Content-Encoding 解压后再存，方便 UI 直接看 JSON/文本。
func storeBodyForView(hdr map[string][]string, raw []byte) (out []byte, trunc bool) {
	decoded := decodeContentEncoding(headerGet(hdr, "Content-Encoding"), raw)
	return clipBody(decoded)
}

func headerGet(hdr map[string][]string, key string) string {
	if hdr == nil {
		return ""
	}
	if vals := hdr[key]; len(vals) > 0 {
		return vals[0]
	}
	for k, vals := range hdr {
		if strings.EqualFold(k, key) && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func decodeContentEncoding(encHeader string, raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	encodings := splitEncodings(encHeader)
	if len(encodings) == 0 {
		if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
			encodings = []string{"gzip"}
		} else {
			return raw
		}
	}
	cur := raw
	for i := len(encodings) - 1; i >= 0; i-- {
		next, err := decodeOneEncoding(encodings[i], cur)
		if err != nil || next == nil {
			return cur
		}
		cur = next
	}
	return cur
}

func splitEncodings(h string) []string {
	h = strings.TrimSpace(h)
	if h == "" || strings.EqualFold(h, "identity") {
		return nil
	}
	parts := strings.Split(h, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || p == "identity" {
			continue
		}
		if i := strings.IndexByte(p, ';'); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}
		out = append(out, p)
	}
	return out
}

func decodeOneEncoding(enc string, raw []byte) ([]byte, error) {
	switch enc {
	case "gzip", "x-gzip":
		r, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(io.LimitReader(r, maxBodyStore+1))
	case "deflate":
		if zr, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
			defer zr.Close()
			if out, err := io.ReadAll(io.LimitReader(zr, maxBodyStore+1)); err == nil {
				return out, nil
			}
		}
		r := flate.NewReader(bytes.NewReader(raw))
		defer r.Close()
		return io.ReadAll(io.LimitReader(r, maxBodyStore+1))
	case "br":
		return io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(raw)), maxBodyStore+1))
	default:
		return raw, nil
	}
}

func cloneHeader(h map[string][]string) map[string][]string {
	if h == nil {
		return nil
	}
	out := make(map[string][]string, len(h))
	for k, vals := range h {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

// RecorderAddon 把 flow 写入 FlowStore。
type RecorderAddon struct {
	proxy.BaseAddon
	store *FlowStore
}

func NewRecorderAddon(store *FlowStore) *RecorderAddon {
	return &RecorderAddon{store: store}
}

func (a *RecorderAddon) Request(f *proxy.Flow) {
	if a.store == nil || f == nil || f.Request == nil {
		return
	}
	hdr := cloneHeader(f.Request.Header)
	reqBody, trunc := storeBodyForView(hdr, f.Request.Body)
	host := ""
	urlStr := ""
	if f.Request.URL != nil {
		urlStr = f.Request.URL.String()
		host = f.Request.URL.Host
	}
	if host == "" {
		host = f.Request.Header.Get("Host")
	}
	a.store.upsert(&storedFlow{
		sum: FlowSummary{
			ID:        f.Id.String(),
			Method:    f.Request.Method,
			URL:       urlStr,
			Host:      host,
			ReqSize:   len(f.Request.Body),
			StartedAt: f.StartTime.UTC().Format(time.RFC3339Nano),
		},
		reqHdr:   hdr,
		reqBody:  reqBody,
		reqTrunc: trunc,
	})
}

func (a *RecorderAddon) Response(f *proxy.Flow) {
	if a.store == nil || f == nil {
		return
	}
	id := f.Id.String()
	a.store.mu.Lock()
	cur := a.store.byID[id]
	a.store.mu.Unlock()
	if cur == nil {
		a.Request(f)
		a.store.mu.Lock()
		cur = a.store.byID[id]
		a.store.mu.Unlock()
		if cur == nil {
			return
		}
	}
	cp := *cur
	cp.sum = cur.sum
	if f.Response != nil {
		cp.sum.StatusCode = f.Response.StatusCode
		cp.sum.RespSize = len(f.Response.Body)
		cp.respHdr = cloneHeader(f.Response.Header)
		body, trunc := storeBodyForView(cp.respHdr, f.Response.Body)
		cp.respBody = body
		cp.respTrunc = trunc
	}
	if !f.StartTime.IsZero() {
		cp.sum.DurationMs = time.Since(f.StartTime).Milliseconds()
	}
	a.store.upsert(&cp)
}
