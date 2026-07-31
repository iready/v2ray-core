#!/bin/bash
# Cursor stop hook：快速本机构建 → Helper → rocket → 网络自检。
# 实时日志：仓库根 hook-deploy.log ；另开终端 tail -f 或 watch-hook-deploy.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LOG="$ROOT/hook-deploy.log"
WATCH_SCRIPT="$ROOT/.cursor/hooks/watch-hook-deploy.sh"
ROCKET="$ROOT/build/rocket"
HELPER_BIN="$ROOT/build/v2ray-helper"
HELPER_PLIST="$ROOT/main/z/helper/com.v2ray.helper.plist"
PRIV_SCRIPT="$ROOT/.cursor/hooks/helper-privileged.sh"
FAST_BUILD="$ROOT/.cursor/hooks/hook-build-fast.sh"
SPEC_DOC="$ROOT/docs/superpowers/specs/2026-06-26-sing-box-tun-prematch-design.md"
LOG_ROCKET="$ROOT/build/log.log"
STAMP_OK="$ROOT/build/.hook-deploy.ok.last"
BIN_STAMP="$ROOT/build/.hook-deploy.bin.sha"
ADMIN_PORT="${ROCKET_ADMIN_PORT:-19527}"
MIN_INTERVAL="${ROCKET_HOOK_MIN_INTERVAL:-120}"
export ROCKET_HELPER_NONINTERACTIVE=1
export ROCKET_HELPER_PRIV_SCRIPT="$PRIV_SCRIPT"

mkdir -p "$ROOT/build"
touch "$LOG"
ln -sf "$LOG" "$ROOT/build/hook-deploy.log"
ln -sf "$LOG" /tmp/rocket-hook-deploy.log 2>/dev/null || true
# 禁止每次 truncate（并发 hook / tail -f 时会把已写内容清掉）

# macOS 无 GNU stdbuf；逐行写文件 + stdout，tail -f 可实时看到
log() {
  printf '%s\n' "$*"
  printf '%s\n' "$*" >>"$LOG"
}

log_stream() {
  while IFS= read -r line || [[ -n "$line" ]]; do
    printf '%s\n' "$line"
    printf '%s\n' "$line" >>"$LOG"
  done
}

log_session_banner() {
  log ""
  log "========== rocket-deploy $(date '+%Y-%m-%d %H:%M:%S') pid=$$ =========="
  log "实时查看（须在终端 tail，编辑器标签不会自动刷新）:"
  log "  tail -f $LOG"
  log "  tail -f /tmp/rocket-hook-deploy.log"
  log "  bash $WATCH_SCRIPT"
  log "运行时: tail -f $LOG_ROCKET"
}

agent_guidance() {
  local status="$1"
  cat <<EOF

--- ROCKET_AGENT_GUIDANCE ---
部署日志(实时): tail -f $LOG
运行时日志: $LOG_ROCKET
设计参照: $SPEC_DOC

sing-box 对齐要点:
  1. OS: auto_route 全量进 TUN + route_exclude 仅 infra
  2. 用户态: PrepareConnection PreMatch
  3. 出站环路: tunctl.EnableDialBind
  4. bypass 合并全部实例出站 IP
  5. DNS: 系统 DNS→TUN 网关；geosite:cn 真实解析；其余 FakeDNS

EOF
  if [[ "$status" == fail ]]; then
    echo "deploy 失败，查 $LOG_ROCKET 中 mg_out/TUN/dial bind。"
    if [[ -f "$LOG_ROCKET" ]]; then
      echo ""
      echo "--- $LOG_ROCKET 尾部 (50 行) ---"
      tail -n 50 "$LOG_ROCKET" 2>/dev/null || true
      echo "--- end ---"
    fi
  elif [[ "$status" == ok ]]; then
    echo "deploy 通过。"
  else
    echo "已跳过（成功 deploy 后 ${MIN_INTERVAL}s 内防抖；失败可立即重试）。"
  fi
  echo "--- end ROCKET_AGENT_GUIDANCE ---"
}

finish() {
  local status="$1"
  local summary="$2"
  log ""
  log "========== 结束 status=$status =========="
  log "$summary"
  case "$status" in
    ok) date +%s >"$STAMP_OK" ;;
    fail) rm -f "$STAMP_OK" ;;
  esac
  echo ""
  echo "ROCKET_DEPLOY_STATUS=$status"
  echo "ROCKET_DEPLOY_SUMMARY=$summary"
  echo "ROCKET_DEPLOY_LOG=$LOG"
  echo "ROCKET_HOOK_TAIL_CMD=tail -f $LOG"
  echo "ROCKET_RUNTIME_LOG=$LOG_ROCKET"
  agent_guidance "$status" | log_stream
}

should_skip_debounce() {
  [[ -n "${ROCKET_HOOK_FORCE:-}" ]] && return 1
  [[ ! -f "$STAMP_OK" ]] && return 1
  local last now
  last="$(cat "$STAMP_OK" 2>/dev/null || echo 0)"
  now="$(date +%s)"
  (( now - last < MIN_INTERVAL ))
}

bin_fingerprint() {
  shasum -a 256 "$ROCKET" "$HELPER_BIN" 2>/dev/null | shasum -a 256 | awk '{print $1}'
}

rocket_healthy() {
  pgrep -x rocket >/dev/null 2>&1 || return 1
  curl -fsS --max-time 2 "http://127.0.0.1:${ADMIN_PORT}/api/status" >/dev/null 2>&1
}

emergency_network_recovery() {
  log "!!! EMERGENCY: 停 rocket + 拆 TUN 路由（保留 Helper）..."
  pkill -TERM -x rocket 2>/dev/null || true
  sleep 1
  pkill -KILL -x rocket 2>/dev/null || true
  if [[ "$(uname -s)" == "Darwin" ]] && [[ -S /var/run/com.v2ray.helper.sock ]]; then
    helper_cmd TEARDOWN >/dev/null 2>&1 || true
  fi
}

stop_rocket() {
  if pgrep -x rocket >/dev/null 2>&1; then
    log "  停止 rocket..."
    pkill -TERM -x rocket 2>/dev/null || true
    sleep 1
    pkill -KILL -x rocket 2>/dev/null || true
  fi
  if [[ "$(uname -s)" == "Darwin" ]] && [[ -S /var/run/com.v2ray.helper.sock ]]; then
    helper_cmd TEARDOWN >/dev/null 2>&1 || true
    sleep 1
  fi
}

wait_rocket_ready() {
  local i
  for i in $(seq 1 12); do
    if pgrep -x rocket >/dev/null 2>&1 && \
      curl -fsS --max-time 2 "http://127.0.0.1:${ADMIN_PORT}/api/status" >/dev/null 2>&1; then
      log "  localadmin 就绪 (${i}/12)"
      return 0
    fi
    sleep 1
  done
  return 1
}

fetch_admin_status() {
  curl -fsS --max-time 3 "http://127.0.0.1:${ADMIN_PORT}/api/status" 2>/dev/null
}

wait_servers_started() {
  local i json
  for i in $(seq 1 45); do
    json="$(fetch_admin_status)" || { sleep 2; continue; }
    if python3 -c 'import sys,json; d=json.load(sys.stdin); sys.exit(0 if (d.get("server_count") or 0)>0 else 1)' <<<"$json" 2>/dev/null; then
      log "  v2fly 实例已启动 (${i}/45)"
      return 0
    fi
    sleep 2
  done
  json="$(fetch_admin_status)" || json='{}'
  log "  [FAIL] v2fly 未启动: $(python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("last_error") or d)' <<<"$json" 2>/dev/null)"
  return 1
}

wait_tun_active() {
  local i json
  for i in $(seq 1 30); do
    json="$(fetch_admin_status)" || { sleep 1; continue; }
    if python3 -c 'import sys,json; d=json.load(sys.stdin); sys.exit(0 if d.get("tun_active") else 1)' <<<"$json" 2>/dev/null; then
      local ifname degraded
      ifname="$(python3 -c 'import sys,json; print(json.load(sys.stdin).get("tun_if_name") or "")' <<<"$json")"
      degraded="$(python3 -c 'import sys,json; print(json.load(sys.stdin).get("tun_degraded_reason") or "")' <<<"$json")"
      log "  TUN 已激活 (${i}/20) if=${ifname:-?} bind=$(python3 -c 'import sys,json; print(json.load(sys.stdin).get("tun_bind_interface") or "")' <<<"$json")"
      [[ -z "$degraded" ]] || log "  注意: tun_degraded=$degraded"
      return 0
    fi
    sleep 1
  done
  json="$(fetch_admin_status)" || json='{}'
  log "  [FAIL] TUN 未激活: $(python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("tun_degraded_reason") or d)' <<<"$json" 2>/dev/null)"
  return 1
}

test_http() {
  local name="$1" url="$2" proxy="${3:-}"
  local code curl_args=(-4 -fsS -o /dev/null -w '%{http_code}' --max-time 60 --connect-timeout 20)
  [[ -n "$proxy" ]] && curl_args+=(-x "$proxy")
  code="$(curl "${curl_args[@]}" "$url" 2>/dev/null)" || code="000"
  case "$code" in
    200 | 204 | 301 | 302)
      log "  [PASS] $name HTTP $code"
      return 0
      ;;
    *)
      log "  [FAIL] $name HTTP $code ($url${proxy:+, proxy=$proxy})"
      return 1
      ;;
  esac
}

test_dns_a() {
  local name="$1" host="$2" bad_prefix="${3:-}"
  local ip
  ip="$(python3 -c "import socket; print(socket.gethostbyname('${host}'))" 2>/dev/null)"
  if [[ -z "$ip" ]]; then
    log "  [FAIL] $name DNS empty ($host)"
    return 1
  fi
  if [[ -n "$bad_prefix" && "$ip" == ${bad_prefix}* ]]; then
    log "  [FAIL] $name DNS polluted $ip ($host)"
    return 1
  fi
  log "  [PASS] $name DNS $ip"
  return 0
}

test_tcp_port() {
  local name="$1" host="$2" port="$3"
  if nc -z -G 10 -w 5 "$host" "$port" 2>/dev/null; then
    log "  [PASS] $name ${host}:${port}"
    return 0
  fi
  log "  [FAIL] $name ${host}:${port} unreachable"
  return 1
}

wait_tun_dns_ready() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    return 0
  fi
  local i svc dns ip
  for i in $(seq 1 45); do
    dns=""
    while IFS= read -r svc; do
      [[ -z "$svc" ]] && continue
      dns="$(networksetup -getdnsservers "$svc" 2>/dev/null | head -1 || true)"
      case "$dns" in
        127.0.0.1 | 172.19.0.1) break ;;
        *) dns="" ;;
      esac
    done < <(networksetup -listallnetworkservices 2>/dev/null | sed '1d;/^\*/d')
    ip="$(python3 -c "import socket; print(socket.gethostbyname('gitee.com'))" 2>/dev/null || true)"
    if [[ -n "$dns" && -n "$ip" && "$ip" != 198.18.* ]]; then
      log "  系统 DNS 就绪 (${i}/45) via ${svc:-?} gitee.com=$ip"
      return 0
    fi
    sleep 1
  done
  log "  [FAIL] 系统 DNS/解析未就绪 (需 127.0.0.1 + gitee 真实 IP)"
  return 1
}

ensure_dns_forward() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    return 0
  fi
  local i ip gip
  for i in $(seq 1 20); do
    gip="$(python3 -c "import socket; print(socket.gethostbyname('www.google.com'))" 2>/dev/null || true)"
    ip="$(python3 -c "import socket; print(socket.gethostbyname('gitee.com'))" 2>/dev/null || true)"
    if [[ -n "$gip" && -n "$ip" ]]; then
      log "  DNS 转发探活 (${i}/20) gitee.com=$ip www.google.com=$gip"
      return 0
    fi
    if [[ "$i" == "1" || $((i % 5)) -eq 0 ]]; then
      helper_cmd "SET_DNS 127.0.0.1" >/dev/null 2>&1 || true
    fi
    sleep 1
  done
  log "  [FAIL] Helper :53 DNS 转发未响应 (gitee=${ip:-empty} google=${gip:-empty})"
  return 1
}

# 1090=HTTP 入站，1091=SOCKS；勿把 SOCKS 握手发到 HTTP 端口
detect_socks_port() {
  local p="${ROCKET_SOCKS_PORT:-}"
  [[ -n "$p" ]] && { echo "$p"; return; }
  if grep -q 'listening TCP on 127.0.0.1:1091' "$LOG_ROCKET" 2>/dev/null; then
    echo 1091
    return
  fi
  echo 1091
}

run_network_tests() {
  local ok=0 socks_port proxy
  log "  等待 v2fly 实例..."
  wait_servers_started || return 1
  log "  校验 TUN..."
  wait_tun_active || return 1
  log "  等待系统 DNS..."
  wait_tun_dns_ready || return 1
  log "  [PASS] 系统 DNS 已指向 TUN 转发"
  ensure_dns_forward || return 1
  sleep 5
  socks_port="$(detect_socks_port)"
  proxy="socks5h://127.0.0.1:${socks_port}"
  log "  网络连通测试（TUN 直连 + SOCKS 对照 + DNS）"
  test_dns_a "国内-gitee.com" "gitee.com" || ok=1
  sleep 2
  test_tcp_port "国内-gitee-SSH" "gitee.com" 22 || ok=1
  sleep 2
  test_http "国内-百度(直连)" "https://www.baidu.com" || ok=1
  sleep 2
  test_http "国内-淘宝(直连)" "https://www.taobao.com" || ok=1
  sleep 2
  test_http "国内-京东(直连)" "https://www.jd.com" || ok=1
  sleep 2
  test_http "国外-Google(经TUN)" "https://www.google.com/generate_204" || ok=1
  sleep 2
  test_http "国外-Google(经SOCKS)" "https://www.google.com/generate_204" "$proxy" || ok=1
  [[ $ok -eq 0 ]]
}

helper_cmd() {
  local cmd="$1"
  python3 - "$cmd" <<'PY'
import os, socket, sys
cmd = sys.argv[1]
if not cmd.endswith("\n"):
    cmd += "\n"
path = "/var/run/com.v2ray.helper.sock"
if not os.path.exists(path):
    sys.exit(1)
sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
local = f"/tmp/v2ray-hook-{os.getpid()}.sock"
try:
    os.unlink(local)
except FileNotFoundError:
    pass
try:
    sock.bind(local)
    sock.settimeout(10)
    sock.sendto(cmd.encode(), path)
    data, _ = sock.recvfrom(4096)
    sys.stdout.write(data.decode())
except Exception:
    sys.exit(1)
finally:
    sock.close()
    try:
        os.unlink(local)
    except FileNotFoundError:
        pass
PY
}

helper_ping_ok() {
  [[ "$(uname -s)" == "Darwin" ]] || return 1
  [[ -S /var/run/com.v2ray.helper.sock ]] || return 1
  local reply
  reply="$(helper_cmd PING 2>/dev/null || true)"
  [[ "$reply" == OK* ]]
}

helper_binary_unchanged() {
  [[ -f "$HELPER_BIN" && -f /Library/PrivilegedHelperTools/com.v2ray.helper ]] || return 1
  cmp -s "$HELPER_BIN" /Library/PrivilegedHelperTools/com.v2ray.helper
}

ensure_helper() {
  if [[ -z "${ROCKET_HOOK_FORCE:-}" ]] && helper_ping_ok && helper_binary_unchanged; then
    log "  Helper 已就绪，跳过安装"
    return 0
  fi
  [[ -x "$PRIV_SCRIPT" ]] || return 1
  log "  sudo -n 安装 Helper..."
  sudo -n "$PRIV_SCRIPT" install "$HELPER_BIN" "$HELPER_PLIST" "$(id -u)"
}

# --- main ---
log_session_banner

if should_skip_debounce; then
  finish skip "距上次成功 deploy 不足 ${MIN_INTERVAL}s"
  exit 0
fi

cd "$ROOT"

log "[1/5] 快速本机构建..."
log "  $(date '+%H:%M:%S') 开始..."
set +e
bash "$FAST_BUILD" 2>&1 | log_stream
build_rc=${PIPESTATUS[0]}
set -e
if [[ $build_rc -ne 0 ]]; then
  finish fail "快速构建失败"
  exit 1
fi

for f in "$ROCKET" "$HELPER_BIN" "$HELPER_PLIST"; do
  [[ -e "$f" ]] || { finish fail "缺少 $f"; exit 1; }
done

fp="$(bin_fingerprint)"
if [[ -f "$BIN_STAMP" && "$(cat "$BIN_STAMP")" == "$fp" ]] && rocket_healthy; then
  json="$(fetch_admin_status)" || json='{}'
  if python3 -c 'import sys,json; d=json.load(sys.stdin); sys.exit(0 if d.get("tun_active") else 1)' <<<"$json" 2>/dev/null; then
    log "[skip] 二进制未变且 rocket+TUN 已运行"
    finish ok "无变更，rocket+TUN 运行中"
    exit 0
  fi
fi

log "[2/5] 停止旧 rocket..."
stop_rocket

if [[ "$(uname -s)" == "Darwin" ]]; then
  log "[3/5] Helper..."
  if ! ensure_helper; then
    emergency_network_recovery
    finish fail "Helper 安装失败"
    exit 1
  fi
else
  log "[3/5] 非 macOS，跳过 Helper"
fi

log "[4/5] 启动 rocket..."
nohup "$ROCKET" >>"$LOG_ROCKET" 2>&1 &
log "  pid=$!"

if ! wait_rocket_ready; then
  emergency_network_recovery
  finish fail "rocket 12s 内未就绪"
  exit 1
fi

log "[5/5] TUN 网络测试..."
if ! run_network_tests; then
  emergency_network_recovery
  finish fail "网络测试失败"
  exit 1
fi

printf '%s' "$fp" >"$BIN_STAMP"
finish ok "构建+部署+网络测试通过"
exit 0
