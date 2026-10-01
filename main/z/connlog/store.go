package connlog

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	capN       = 5000
	errLimit   = 240
	defaultLim = 500
	faultCap   = 40
)

// Item 一条经过核心出站的连接。不含请求体。
type Item struct {
	ID      string `json:"id"`
	At      int64  `json:"at"`
	Dest    string `json:"dest"`
	Tag     string `json:"tag"`
	Pending bool   `json:"pending"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	DurMs   int64  `json:"dur_ms"`
}

// View 录制快照。Items 为最新 limit 条（新在前），Total 为会话内保留条数。
type View struct {
	Active    bool   `json:"active"`
	StartedAt int64  `json:"started_at,omitempty"`
	StoppedAt int64  `json:"stopped_at,omitempty"`
	Total     int    `json:"total"`
	Items     []Item `json:"items"`
}

type store struct {
	active atomic.Bool
	mu     sync.Mutex
	seq    []string
	head   int
	n      int
	byID   map[string]*Item
	next   uint64
	start  int64
	stop   int64
	recent []Fault
}

var snapshot atomic.Value // *contextSnapshot, published outside outbound processing

type contextSnapshot struct {
	tun, routes string
	at          time.Time
}

// SetSnapshot 在配置/状态变化及低频采样时生成快照；出站错误只引用已发布的数据。
func SetSnapshot(fn func() (string, string)) {
	if fn == nil {
		return
	}
	tun, routes := fn()
	snapshot.Store(&contextSnapshot{tun: tun, routes: routes, at: time.Now()})
}

// takeSnapshot only loads immutable strings; no I/O or configuration traversal here.
func takeSnapshot() (string, string) {
	s, _ := snapshot.Load().(*contextSnapshot)
	if s == nil {
		return "", ""
	}
	return "快照生成: " + s.at.Format(time.RFC3339Nano) + "\n" + s.tun, s.routes
}

var defaultStore = &store{
	seq:  make([]string, capN),
	byID: map[string]*Item{},
}

// Active 录制开关。出站热路径只做这一次原子读。
func Active() bool { return defaultStore.active.Load() }

// Begin 录制中登记一条进行中的连接。未录制时返回空 id。
func Begin(tag, dest string) string {
	if !defaultStore.active.Load() {
		return ""
	}
	return defaultStore.begin(tag, dest)
}

// End 连接结束。id 为空或会话已换则忽略。上下文取消是会话正常拆除，不算失败。
func End(id string, err error) {
	if id == "" {
		return
	}
	reason := ""
	if err != nil && !errors.Is(err, context.Canceled) {
		reason = err.Error()
	}
	defaultStore.finish(id, reason)
}

// Fail 选路失败、没有出站可派发时记一条已结束的失败。
func Fail(tag, dest, reason string) {
	Note(tag, dest, errors.New(reason))
	if !defaultStore.active.Load() {
		return
	}
	id := defaultStore.begin(tag, dest)
	defaultStore.finish(id, reason)
}

// Fault 未开录制也保留的失败。相同目的地+错误合并计数。
type Fault struct {
	At     int64  `json:"at"`
	Dest   string `json:"dest"`
	Tag    string `json:"tag"`
	Error  string `json:"error"`
	N      int    `json:"n"`
	tun    string
	routes string
}

// Note 记录一条异常。成功和上下文取消不记。热路径只在出错时调用。
func Note(tag, dest string, err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	reason := err.Error()
	if reason == "" || reason == "context canceled" {
		return
	}
	if len(reason) > errLimit {
		reason = reason[:errLimit]
	}
	defaultStore.note(tag, dest, reason)
}

// Faults 最近的异常，新的在前。
func Faults() []Fault {
	return defaultStore.faults()
}

// ClearFaults 清掉状态页上的临时异常，不影响「流量」录制。
func ClearFaults() {
	defaultStore.mu.Lock()
	defaultStore.recent = nil
	defaultStore.mu.Unlock()
}

func Start() {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	defaultStore.seq = make([]string, capN)
	defaultStore.head = 0
	defaultStore.n = 0
	defaultStore.byID = make(map[string]*Item, 256)
	defaultStore.start = time.Now().UnixMilli()
	defaultStore.stop = 0
	defaultStore.active.Store(true)
}

func Stop() {
	defaultStore.active.Store(false)
	defaultStore.mu.Lock()
	defaultStore.stop = time.Now().UnixMilli()
	defaultStore.mu.Unlock()
}

func Snapshot(limit int) View {
	if limit <= 0 {
		limit = defaultLim
	}
	if limit > capN {
		limit = capN
	}
	return defaultStore.snapshot(limit)
}

func (s *store) begin(tag, dest string) string {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active.Load() {
		return ""
	}
	s.next++
	id := formatID(s.next)
	it := &Item{
		ID:      id,
		At:      now.UnixMilli(),
		Dest:    dest,
		Tag:     tag,
		Pending: true,
	}
	if s.n >= capN {
		old := s.seq[s.head]
		s.head = (s.head + 1) % capN
		s.n--
		delete(s.byID, old)
	}
	s.seq[(s.head+s.n)%capN] = id
	s.n++
	s.byID[id] = it
	return id
}

func (s *store) finish(id, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := s.byID[id]
	if it == nil || !it.Pending {
		return
	}
	it.Pending = false
	it.DurMs = time.Now().UnixMilli() - it.At
	if reason == "" {
		it.OK = true
		return
	}
	it.OK = false
	if len(reason) > errLimit {
		reason = reason[:errLimit]
	}
	it.Error = reason
}

func (s *store) note(tag, dest, reason string) {
	now := time.Now().UnixMilli()
	s.mu.Lock()
	for i, f := range s.recent {
		if f.Dest == dest && f.Tag == tag && f.Error == reason {
			f.At = now
			f.N++
			copy(s.recent[1:i+1], s.recent[:i])
			s.recent[0] = f
			s.mu.Unlock()
			return
		}
	}
	s.mu.Unlock()

	tun, routes := takeSnapshot()

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, f := range s.recent {
		if f.Dest == dest && f.Tag == tag && f.Error == reason {
			f.At = now
			f.N++
			if f.tun == "" {
				f.tun, f.routes = tun, routes
			}
			copy(s.recent[1:i+1], s.recent[:i])
			s.recent[0] = f
			return
		}
	}
	s.recent = append([]Fault{{
		At: now, Dest: dest, Tag: tag, Error: reason, N: 1,
		tun: tun, routes: routes,
	}}, s.recent...)
	if len(s.recent) > faultCap {
		s.recent = s.recent[:faultCap]
	}
}

// CopyText 第 i 条异常（0 为最新）的可粘贴文本。含记下时的 TUN 配置和路由。
func CopyText(i int) (string, bool) {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	if i < 0 || i >= len(defaultStore.recent) {
		return "", false
	}
	f := defaultStore.recent[i]
	tun, routes := f.tun, f.routes
	if tun == "" {
		tun = "（记下时没有 TUN 配置）"
	}
	if routes == "" {
		routes = "（记下时没有路由）"
	}
	return "目的地: " + f.Dest +
		"\n出站: " + f.Tag +
		"\n次数: " + strconv.Itoa(f.N) +
		"\n时间: " + time.UnixMilli(f.At).Format(time.RFC3339) +
		"\n错误: " + f.Error +
		"\n\nTUN配置:\n" + tun +
		"\n\n路由:\n" + routes + "\n", true
}

func (s *store) faults() []Fault {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Fault, len(s.recent))
	copy(out, s.recent)
	return out
}

func (s *store) snapshot(limit int) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit > s.n {
		limit = s.n
	}
	out := View{
		Active:    s.active.Load(),
		StartedAt: s.start,
		StoppedAt: s.stop,
		Total:     s.n,
		Items:     make([]Item, 0, limit),
	}
	for i := s.n - 1; i >= s.n-limit; i-- {
		if it := s.byID[s.seq[(s.head+i)%capN]]; it != nil {
			out.Items = append(out.Items, *it)
		}
	}
	return out
}

func formatID(n uint64) string {
	const digits = "0123456789abcdef"
	var b [16]byte
	for i := 15; i >= 0; i-- {
		b[i] = digits[n&0xf]
		n >>= 4
		if n == 0 {
			return string(b[i:])
		}
	}
	return string(b[:])
}
