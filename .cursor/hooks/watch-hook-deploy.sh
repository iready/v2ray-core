#!/bin/bash
# 在 Cursor「集成终端」运行本脚本，实时看 deploy 日志。
# 注意：在编辑器里打开 hook-deploy.log 标签不会自动刷新，必须用 tail -f。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LOG="$ROOT/hook-deploy.log"
RUNTIME="$ROOT/build/log.log"

mkdir -p "$ROOT/build"
touch "$LOG"
ln -sf "$LOG" "$ROOT/build/hook-deploy.log"
ln -sf "$LOG" /tmp/rocket-hook-deploy.log 2>/dev/null || true

echo "=========================================="
echo "  部署日志: $LOG"
echo "  快捷路径: /tmp/rocket-hook-deploy.log"
echo "  运行时:   $RUNTIME"
echo "=========================================="
echo "在编辑器打开上述文件不会实时更新，请保持本终端运行。"
echo "按 Ctrl+C 停止跟踪。"
echo "------------------------------------------"
exec tail -n 30 -F "$LOG"
