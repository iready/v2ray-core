#!/bin/bash
# 供 sudo NOPASSWD 调用的 Helper 装卸（无 AppleScript 密码弹框）。
# 一次性配置：.cursor/hooks/setup-passwordless-helper.sh
set -euo pipefail

LABEL="com.v2ray.helper"
PLIST_DST="/Library/LaunchDaemons/com.v2ray.helper.plist"
BIN_DST="/Library/PrivilegedHelperTools/com.v2ray.helper"

usage() {
  echo "usage: sudo $0 install <helper_bin> <plist_src> [uid]" >&2
  echo "       sudo $0 uninstall" >&2
  exit 2
}

install_helper() {
  local helper_bin="$1"
  local plist_src="$2"
  local real_uid="${3:-${SUDO_UID:-$(id -u)}}"
  [[ -f "$helper_bin" && -f "$plist_src" ]] || {
    echo "missing helper or plist" >&2
    exit 1
  }
  mkdir -p /Library/PrivilegedHelperTools /Library/LaunchDaemons /var/run
  cp "$helper_bin" "$BIN_DST" && chmod 755 "$BIN_DST"
  cp "$plist_src" "$PLIST_DST"
  echo "$real_uid" >/var/db/com.v2ray.helper.uid
  chmod 644 /var/db/com.v2ray.helper.uid
  cp /var/db/com.v2ray.helper.uid /var/run/com.v2ray.helper.uid
  chmod 644 /var/run/com.v2ray.helper.uid
  launchctl bootout "system/$LABEL" 2>/dev/null || true
  launchctl bootout system "$PLIST_DST" 2>/dev/null || true
  launchctl unload "$PLIST_DST" 2>/dev/null || true
  launchctl enable "system/$LABEL" 2>/dev/null || true
  launchctl bootstrap system "$PLIST_DST" 2>/dev/null || launchctl load "$PLIST_DST"
  launchctl kickstart "system/$LABEL" 2>/dev/null || true
}

uninstall_helper() {
  launchctl bootout "system/$LABEL" 2>/dev/null || true
  launchctl bootout system "$PLIST_DST" 2>/dev/null || true
  launchctl unload "$PLIST_DST" 2>/dev/null || true
  launchctl remove "$LABEL" 2>/dev/null || true
  launchctl disable "system/$LABEL" 2>/dev/null || true
  local pid
  pid="$(pgrep -x com.v2ray.helper 2>/dev/null || true)"
  if [[ -n "$pid" ]]; then
    kill -TERM "$pid" 2>/dev/null || true
    sleep 1
    kill -KILL "$pid" 2>/dev/null || true
  fi
  rm -f "$PLIST_DST" "$BIN_DST"
  rm -f /var/run/com.v2ray.helper.sock /var/run/com.v2ray.helper.uid /var/db/com.v2ray.helper.uid
  rm -f /var/log/com.v2ray.helper.log /var/log/com.v2ray.helper.err.log
  rm -f /var/run/com.v2ray.helper.fd.*.sock 2>/dev/null || true
}

ACTION="${1:-}"
case "$ACTION" in
  install)
    [[ $# -ge 3 ]] || usage
    install_helper "$2" "$3" "${4:-}"
    ;;
  uninstall)
    uninstall_helper
    ;;
  *)
    usage
    ;;
esac
