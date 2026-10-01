package domainreq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

const fileName = "domain_route_requests.json"

type Status string

const (
	StatusPending   Status = "pending"
	StatusSubmitted Status = "submitted"
	StatusApproved  Status = "approved"
	StatusRejected  Status = "rejected"
)

type Item struct {
	ClientReqID  string   `json:"client_req_id"`
	RequestID    string   `json:"request_id,omitempty"`
	Domains      []string `json:"domains"`
	Remark       string   `json:"remark,omitempty"`
	Status       Status   `json:"status"`
	RejectReason string   `json:"reject_reason,omitempty"`
	Added        int      `json:"added,omitempty"`
	Skipped      int      `json:"skipped,omitempty"`
	RouteNames   []string `json:"route_names,omitempty"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	LastError    string   `json:"last_error,omitempty"`
}

type fileDoc struct {
	Items []Item `json:"items"`
}

// Submitter 向服务端提交申请。
type Submitter interface {
	SubmitDomainRouteRequest(ctx context.Context, domains []string, remark, clientReqID string) (requestID string, err error)
}

var (
	mu   sync.Mutex
	path = filepath.Join(rvstore.ConfigDir(), fileName)
)

func SetPath(p string) { path = p }

func List() ([]Item, error) {
	mu.Lock()
	defer mu.Unlock()
	doc, err := loadLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Item, len(doc.Items))
	copy(out, doc.Items)
	return out, nil
}

func Create(domains []string, remark string) (Item, error) {
	domains, err := NormalizeDomains(domains)
	if err != nil {
		return Item{}, err
	}
	if len(domains) == 0 {
		return Item{}, fmt.Errorf("domains 不能为空")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	item := Item{
		ClientReqID: uuid.NewString(),
		Domains:     domains,
		Remark:      strings.TrimSpace(remark),
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	mu.Lock()
	defer mu.Unlock()
	doc, err := loadLocked()
	if err != nil {
		return Item{}, err
	}
	doc.Items = append([]Item{item}, doc.Items...)
	if err := saveLocked(doc); err != nil {
		return Item{}, err
	}
	return item, nil
}

func ApplyStatusPush(requestID, clientReqID, status, rejectReason string, added, skipped int, routeNames []string) error {
	mu.Lock()
	defer mu.Unlock()
	doc, err := loadLocked()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated := false
	for i := range doc.Items {
		it := &doc.Items[i]
		if clientReqID != "" && it.ClientReqID == clientReqID {
			updated = true
		} else if requestID != "" && it.RequestID == requestID {
			updated = true
		} else {
			continue
		}
		if requestID != "" {
			it.RequestID = requestID
		}
		switch status {
		case "approved":
			it.Status = StatusApproved
		case "rejected":
			it.Status = StatusRejected
		default:
			it.Status = Status(status)
		}
		it.RejectReason = rejectReason
		it.Added = added
		it.Skipped = skipped
		it.RouteNames = routeNames
		it.UpdatedAt = now
		it.LastError = ""
		break
	}
	if !updated {
		return nil
	}
	return saveLocked(doc)
}

// FlushPending 提交本地 pending 到服务端。
func FlushPending(ctx context.Context, s Submitter) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("wire 未连接")
	}
	mu.Lock()
	doc, err := loadLocked()
	if err != nil {
		mu.Unlock()
		return 0, err
	}
	pending := make([]Item, 0)
	for _, it := range doc.Items {
		if it.Status == StatusPending {
			pending = append(pending, it)
		}
	}
	mu.Unlock()

	n := 0
	var lastErr error
	for _, it := range pending {
		reqID, err := s.SubmitDomainRouteRequest(ctx, it.Domains, it.Remark, it.ClientReqID)
		mu.Lock()
		doc2, loadErr := loadLocked()
		if loadErr != nil {
			mu.Unlock()
			return n, loadErr
		}
		for i := range doc2.Items {
			if doc2.Items[i].ClientReqID != it.ClientReqID {
				continue
			}
			doc2.Items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err != nil {
				doc2.Items[i].LastError = err.Error()
				lastErr = err
			} else {
				doc2.Items[i].Status = StatusSubmitted
				doc2.Items[i].RequestID = reqID
				doc2.Items[i].LastError = ""
				n++
			}
			break
		}
		_ = saveLocked(doc2)
		mu.Unlock()
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
	}
	return n, lastErr
}

func NormalizeDomains(raw []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		for _, p := range strings.FieldsFunc(r, func(c rune) bool {
			return c == ',' || c == ';' || c == '|' || c == '\n' || c == '\r' || c == '\t' || c == ' '
		}) {
			d := strings.ToLower(strings.Trim(strings.TrimSpace(p), "."))
			if d == "" {
				continue
			}
			if err := validateDomain(d); err != nil {
				return nil, err
			}
			if _, ok := seen[d]; ok {
				continue
			}
			seen[d] = struct{}{}
			out = append(out, d)
		}
	}
	return out, nil
}

func validateDomain(d string) error {
	prefixes := []string{"geosite:", "geoip:", "ext:", "domain:", "full:", "keyword:", "regexp:"}
	for _, p := range prefixes {
		if strings.HasPrefix(d, p) {
			return nil
		}
	}
	if strings.ContainsAny(d, " \t") || len(d) > 260 {
		return fmt.Errorf("非法域名/条目: %s", d)
	}
	ok := false
	for _, c := range d {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '*' || c == '/' || c == '_' {
			ok = true
			continue
		}
		return fmt.Errorf("非法域名/条目: %s", d)
	}
	if !ok {
		return fmt.Errorf("非法域名/条目: %s", d)
	}
	return nil
}

func loadLocked() (*fileDoc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &fileDoc{Items: []Item{}}, nil
		}
		return nil, err
	}
	if len(bytesTrimSpace(b)) == 0 {
		return &fileDoc{Items: []Item{}}, nil
	}
	var doc fileDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if doc.Items == nil {
		doc.Items = []Item{}
	}
	return &doc, nil
}

func saveLocked(doc *fileDoc) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
