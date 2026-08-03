#!/usr/bin/env bash
# MITM / 本地客户端泄漏巡检（供 /loop 调用）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$ROOT"
echo "root=$ROOT"

echo "=== mitm leak check $(date '+%F %T') ==="

echo "-- go test leak + ring --"
go test ./main/z/mitmctl/ -count=1 -timeout 90s -run 'Leak|FlowStoreRing' 2>&1

echo "-- replace go-mitmproxy --"
if ! grep -q 'replace github.com/lqqyt2423/go-mitmproxy => ./third_party/go-mitmproxy' go.mod; then
  echo "FAIL: missing go.mod replace for patched go-mitmproxy"
  exit 1
fi
if ! grep -q 'closeAttacker' third_party/go-mitmproxy/proxy/proxy.go; then
  echo "FAIL: patched closeAttacker missing"
  exit 1
fi
if ! grep -q 'once.Do' third_party/go-mitmproxy/proxy/attacker.go; then
  echo "FAIL: attackerListener.Close channel close missing"
  exit 1
fi
echo "OK patched mitmproxy"

echo "-- API must not echo key_pem in response helpers --"
if ! grep -q 'redactHostCertKeys' main/z/localadmin/mitm_api.go; then
  echo "FAIL: redactHostCertKeys missing"
  exit 1
fi
echo "OK key redact"

echo "-- WebAddon Close + Manager webCloser --"
if ! grep -q 'func (web \*WebAddon) Close' third_party/go-mitmproxy/web/web.go; then
  echo "FAIL: WebAddon.Close missing"
  exit 1
fi
if ! grep -q 'webCloser' main/z/mitmctl/manager.go; then
  echo "FAIL: Manager webCloser missing"
  exit 1
fi
echo "OK web closer"

echo "-- FlowStore ring (no seq[1:]) --"
if grep -n 'seq = s.seq\[1:\]' main/z/mitmctl/flow_store.go; then
  echo "FAIL: slice-drop ring still present"
  exit 1
fi
echo "OK flow ring"

echo "-- secrets in tracked mitm sources --"
if git grep -nE 'BEGIN (RSA |EC )?PRIVATE KEY' -- 'main/z/mitmctl' 'main/z/localadmin' 'main/z/rvstore' 2>/dev/null | grep -v '_test.go' | grep -v 'placeholder\|示例\|----'; then
  echo "WARN: possible private key material in non-test sources (review above)"
else
  echo "OK no private key blobs in non-test sources"
fi

echo "-- rocket process (optional) --"
if pgrep -x rocket >/dev/null 2>&1; then
  pid="$(pgrep -x rocket | head -1)"
  echo "rocket pid=$pid"
  if command -v ps >/dev/null; then
    ps -o pid,rss,vsz,comm -p "$pid" 2>/dev/null || true
  fi
  # macOS: sample open files count
  if command -v lsof >/dev/null; then
    n="$(lsof -nP -p "$pid" 2>/dev/null | wc -l | tr -d ' ')"
    echo "lsof_lines=$n"
  fi
else
  echo "rocket not running (skip process metrics)"
fi

echo "=== leak check done ==="
