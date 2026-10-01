// Package applog 默认把进程日志落盘：标准 log 与 Go 运行时崩溃输出都写到
// <ConfigDir>/logs/rocket-日期.log。Windows 用 -H windowsgui 编译后没有控制台，不落盘就什么都看不到。
package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

const (
	keepDays = 7
	maxBytes = 64 << 20
	dayFmt   = "2006-01-02"
)

type dailyFile struct {
	mu   sync.Mutex
	dir  string
	day  string
	f    *os.File
	size int64
}

var Suppressed atomic.Uint64
var repeats = struct {
	sync.Mutex
	entries map[string]repeatEntry
}{entries: make(map[string]repeatEntry)}

type repeatEntry struct {
	at      time.Time
	skipped uint64
}

// Start 打开当天日志并接管标准 log；返回日志目录。
func Start() (string, error) {
	dir := filepath.Join(rvstore.ConfigDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	w := &dailyFile{dir: dir}
	if err := w.open(time.Now().Format(dayFmt)); err != nil {
		return "", err
	}
	log.SetOutput(io.MultiWriter(w, os.Stderr))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix(fmt.Sprintf("pid=%d ", os.Getpid()))
	return dir, nil
}

// Recover 放在协程 defer 里：记下 panic 与堆栈，不让一条连接拖垮整个进程。
func Recover(where string) {
	if r := recover(); r != nil {
		log.Printf("panic %s: %v\n%s", where, r, debug.Stack())
	}
}

// LimitedPrintf is opt-in for noisy connection events; file output never classifies text.
func LimitedPrintf(key, format string, args ...any) {
	repeats.Lock()
	item, exists := repeats.entries[key]
	now := time.Now()
	if exists && now.Sub(item.at) < 30*time.Second {
		item.skipped++
		repeats.entries[key] = item
		repeats.Unlock()
		Suppressed.Add(1)
		return
	}
	if len(repeats.entries) >= 256 {
		clear(repeats.entries)
	}
	repeats.entries[key] = repeatEntry{at: now}
	repeats.Unlock()
	if item.skipped > 0 {
		format += " suppressed_duplicates=%d"
		args = append(args, item.skipped)
	}
	log.Printf(format, args...)
}

func (d *dailyFile) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if today := time.Now().Format(dayFmt); today != d.day {
		if err := d.open(today); err != nil {
			return 0, err
		}
	}
	if d.size >= maxBytes {
		// Release the runtime duplicate before rotating on Windows. If rename fails
		// (e.g. a second process has the log open), reopen the same file and keep logging.
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		_ = d.f.Close()
		path := filepath.Join(d.dir, "rocket-"+d.day+".log")
		_ = os.Remove(path + ".2")
		_ = os.Rename(path+".1", path+".2")
		_ = os.Rename(path, path+".1")
		if err := d.open(d.day); err != nil {
			return 0, err
		}
	}
	n, err := d.f.Write(p)
	d.size += int64(n)
	return n, err
}

func (d *dailyFile) open(day string) error {
	f, err := os.OpenFile(filepath.Join(d.dir, "rocket-"+day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	// 运行时致命错误（未兜住的 panic、并发写 map）绕过 log，只能靠这里写进文件。
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	if d.f != nil {
		_ = d.f.Close()
	}
	d.f, d.day, d.size = f, day, 0
	if st, err := f.Stat(); err == nil {
		d.size = st.Size()
	}
	purge(d.dir, day)
	return nil
}

func purge(dir, today string) {
	t, err := time.Parse(dayFmt, today)
	if err != nil {
		return
	}
	cut := t.AddDate(0, 0, -keepDays).Format(dayFmt)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "rocket-") || (!strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.1") && !strings.HasSuffix(name, ".log.2")) {
			continue
		}
		if day := strings.SplitN(strings.TrimPrefix(name, "rocket-"), ".log", 2)[0]; day < cut {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}
