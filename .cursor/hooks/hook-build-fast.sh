#!/bin/bash
# hook 专用：仅构建本机 macOS arm64 的 rocket + v2ray-helper（无变更时跳过 go build）。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BUILD_DIR="$ROOT/build"
CERT_DST="$ROOT/main/z/wire/certs"
BUILD_CONFIG="$ROOT/.build.local"
BUILD_FLAGS="-trimpath -tags with_gvisor"
LDFLAGS="-s -w -buildid="
ROCKET_OUT="$BUILD_DIR/rocket"
HELPER_OUT="$BUILD_DIR/v2ray-helper"

cd "$ROOT"
mkdir -p "$BUILD_DIR"

sources_newer_than() {
  local target="$1"
  [[ ! -f "$target" ]] && return 0
  find "$ROOT/main/z" "$ROOT/app/tun" -name '*.go' -newer "$target" -print -quit 2>/dev/null | grep -q .
}

if [[ -f "$BUILD_CONFIG" ]]; then
  # shellcheck disable=SC1090
  source "$BUILD_CONFIG"
  if [[ -n "${ROCKET_URL:-}" ]]; then
    LDFLAGS+=" -X github.com/v2fly/v2ray-core/v5/main/z/rvstore.buildDefaultRocketURL=${ROCKET_URL}"
    case "$ROCKET_URL" in
      wss://* | WSS://*) LDFLAGS+=" -X github.com/v2fly/v2ray-core/v5/main/z/rvstore.buildDefaultTLSMode=embed" ;;
    esac
  fi
fi

if [[ -f "$CERT_DST/root_ca.pem" && -f "$CERT_DST/client.p12" && -f "$CERT_DST/client.p12.pass" ]]; then
  echo "  使用已有 mTLS 证书（跳过 Downloads 拷贝）"
elif [[ -n "${CA_FILE:-}" && -n "${P12_FILE:-}" && -f "$CA_FILE" && -f "$P12_FILE" ]]; then
  cp "$CA_FILE" "$CERT_DST/root_ca.pem"
  cp "$P12_FILE" "$CERT_DST/client.p12"
  printf '%s' "${P12_PASS:-changeit}" >"$CERT_DST/client.p12.pass"
else
  echo "ERROR: 缺少 mTLS 材料，请配置 .build.local 或放入 main/z/wire/certs"
  exit 1
fi

need_rocket=0
need_helper=0
if [[ "${ROCKET_HOOK_FORCE_BUILD:-}" == 1 ]] || sources_newer_than "$ROCKET_OUT"; then
  need_rocket=1
fi
if [[ -f "$ROCKET_OUT" ]] && ! go version -m "$ROCKET_OUT" 2>/dev/null | rg -q 'tags=with_gvisor'; then
  echo "  rocket 缺少 with_gvisor 标签，强制重编..."
  need_rocket=1
fi
if [[ "${ROCKET_HOOK_FORCE_BUILD:-}" == 1 ]] || sources_newer_than "$HELPER_OUT"; then
  need_helper=1
fi

if [[ $need_rocket -eq 0 && $need_helper -eq 0 ]]; then
  echo "  go 源码未变，跳过 go build (沿用 ${ROCKET_OUT})"
else
  build_rocket() {
    echo "  go build rocket (darwin/arm64)..."
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $BUILD_FLAGS -ldflags "$LDFLAGS" \
      -o "$ROCKET_OUT" github.com/v2fly/v2ray-core/v5/main/z/client
  }
  build_helper() {
    echo "  go build v2ray-helper (darwin/arm64)..."
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $BUILD_FLAGS -ldflags "$LDFLAGS" \
      -o "$HELPER_OUT" github.com/v2fly/v2ray-core/v5/main/z/cmd/v2ray-helper
  }
  if [[ $need_rocket -eq 1 && $need_helper -eq 1 ]]; then
    build_rocket &
    pid_r=$!
    build_helper &
    pid_h=$!
    wait "$pid_r"
    wait "$pid_h"
  elif [[ $need_rocket -eq 1 ]]; then
    build_rocket
  else
    build_helper
  fi
fi

echo "  构建步骤完成 $(date '+%H:%M:%S')"
