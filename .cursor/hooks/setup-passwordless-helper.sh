#!/bin/bash
# 一次性配置：允许当前用户免密执行 helper-privileged.sh（hook / rocket 不再弹管理员密码框）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PRIV="$SCRIPT_DIR/helper-privileged.sh"
USER_NAME="$(whoami)"
DEST="/etc/sudoers.d/v2ray-rocket"

if [[ ! -f "$PRIV" ]]; then
  echo "缺少 $PRIV" >&2
  exit 1
fi
chmod +x "$PRIV"

LINE="$USER_NAME ALL=(ALL) NOPASSWD: $PRIV"
echo "将写入 $DEST ："
echo "  $LINE"
echo ""
read -r -p "继续？[y/N] " ans
case "$ans" in
  [yY] | [yY][eE][sS]) ;;
  *) echo "已取消"; exit 0 ;;
esac

TMP="$(mktemp)"
printf '%s\n' "$LINE" >"$TMP"
visudo -cf "$TMP"
install -m 440 "$TMP" "$DEST"
rm -f "$TMP"
echo "完成。可验证：sudo -n $PRIV install ...（或跑 hook）"
