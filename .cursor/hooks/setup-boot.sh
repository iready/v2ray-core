#!/bin/bash
# 一次性配置：Helper 开机自启 + rocket 登录自启 + 健康检查
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "==> 构建 rocket + v2ray-helper"
bash .cursor/hooks/hook-build-fast.sh

echo "==> 安装 Privileged Helper（需 sudo）"
sudo .cursor/hooks/helper-privileged.sh install \
  "$ROOT/build/v2ray-helper" \
  "$ROOT/main/z/helper/com.v2ray.helper.plist" \
  "$(id -u)"

echo "==> 注册 rocket 登录自启"
"$ROOT/build/rocket" -install-autostart

echo "==> 等待就绪"
for _ in $(seq 1 20); do
  if curl -fsS --max-time 2 "http://127.0.0.1:19527/api/status" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "==> 状态"
launchctl print "system/com.v2ray.helper" 2>/dev/null | grep 'state =' || true
launchctl print "gui/$(id -u)/rocket.rocket" 2>/dev/null | grep -E 'state =|runs =' || true
test -f /var/db/com.v2ray.helper.uid && echo "helper uid: ok" || echo "helper uid: MISSING"
curl -fsS "http://127.0.0.1:19527/api/status" 2>/dev/null | python3 -c "
import sys, json
d = json.load(sys.stdin)
print('wire:', d.get('wire_connected'))
print('tun_active:', d.get('tun_active'))
print('tun_helper:', d.get('tun_helper_installed'))
print('healthy:', d.get('healthy_count'), '/', d.get('server_count'))
" || echo "rocket API 未响应（注销再登录后会自动拉起）"

echo "==> 完成"
