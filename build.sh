#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RES_DIR="$SCRIPT_DIR/main/z/res"
WEB_DIR="$SCRIPT_DIR/main/z/localadmin/web"
CERT_DST="$SCRIPT_DIR/main/z/wire/certs"
BUILD_CONFIG="$SCRIPT_DIR/.build.local"
LEGACY_BUILD_CONFIG="$SCRIPT_DIR/.build-mtls.local"
BUILD_PREFS="$SCRIPT_DIR/.build.prefs.local"

REUSE_LAST=0
BUILD_CHOICES_REUSED=0
# windows | darwin | linux | all（默认只编 Windows）
TARGET="${TARGET:-windows}"

ask_yes_no() {
  local prompt="$1"
  local answer
  read -r -p "$prompt [y/N]: " answer
  case "$answer" in
    [yY] | [yY][eE][sS]) return 0 ;;
    *) return 1 ;;
  esac
}

ask_yes_no_default_yes() {
  local prompt="$1"
  local answer
  read -r -p "$prompt [Y/n]: " answer
  case "$answer" in
    [nN] | [nN][oO]) return 1 ;;
    *) return 0 ;;
  esac
}

yn_label() {
  [[ "$1" -eq 1 ]] && printf '是' || printf '否'
}

set_default_build_prefs() {
  CONFIGURE_DEFAULTS=1
  USE_SAVED_PATHS=1
  INJECT_MTLS=1
  DOWNLOAD_GEODATA=0
  BUILD_FRONTEND=1
  RUN_UPX=0
  MAKE_ZIP=0
  TARGET=windows
}

load_build_prefs() {
  set_default_build_prefs
  if [[ -f "$BUILD_PREFS" ]]; then
    # shellcheck disable=SC1090
    source "$BUILD_PREFS"
  fi
  TARGET="${TARGET:-windows}"
}

save_build_prefs() {
  cat >"$BUILD_PREFS" <<EOF
CONFIGURE_DEFAULTS=$CONFIGURE_DEFAULTS
USE_SAVED_PATHS=$USE_SAVED_PATHS
INJECT_MTLS=$INJECT_MTLS
DOWNLOAD_GEODATA=$DOWNLOAD_GEODATA
BUILD_FRONTEND=$BUILD_FRONTEND
RUN_UPX=$RUN_UPX
MAKE_ZIP=$MAKE_ZIP
TARGET=$TARGET
EOF
  chmod 600 "$BUILD_PREFS"
}

print_build_prefs_summary() {
  echo "构建选择（沿用上次）："
  echo "  目标平台: $TARGET"
  if [[ $CONFIGURE_DEFAULTS -eq 1 ]]; then
    if [[ $USE_SAVED_PATHS -eq 1 ]]; then
      echo "  配置默认值: 是（使用已保存路径）"
    else
      echo "  配置默认值: 是（重新输入路径）"
    fi
  else
    echo "  配置默认值: 否"
  fi
  echo "  注入 mTLS: $(yn_label "$INJECT_MTLS")"
  echo "  下载 geodata: $(yn_label "$DOWNLOAD_GEODATA")"
  echo "  构建前端: $(yn_label "$BUILD_FRONTEND")"
  echo "  UPX: $(yn_label "$RUN_UPX")"
  echo "  zip: $(yn_label "$MAKE_ZIP")"
}

print_usage() {
  cat <<'EOF'
用法: ./build.sh [选项]

  --os <target>         目标平台: windows（默认）| darwin | linux | all
  -r, --reuse-last      沿用上次/默认构建选择，全程无提问
  --install-startup     Windows：编完后覆盖 Startup\rocket.exe、拉起、等后台就绪
  -h, --help            显示此帮助

示例:
  ./build.sh -r                 # 仅 Windows，沿用上次选择
  ./build.sh --os windows -r    # 同上
  ./build.sh --os all -r        # 全平台
  ./build.sh --os windows -r --install-startup

无参数时：若存在 .build.prefs.local 则只问一次是否沿用上次全部选择。
EOF
}

normalize_target() {
  case "$1" in
    windows | win | exe) TARGET=windows ;;
    darwin | macos | mac) TARGET=darwin ;;
    linux) TARGET=linux ;;
    all) TARGET=all ;;
    *)
      echo "未知 --os 目标: $1（可用 windows|darwin|linux|all）"
      exit 1
      ;;
  esac
}

target_enabled() {
  [[ "$TARGET" == "all" || "$TARGET" == "$1" ]]
}

is_windows_host() {
  case "$(uname -s 2>/dev/null)" in
    MINGW* | MSYS* | CYGWIN*) return 0 ;;
  esac
  [[ "${OS:-}" == "Windows_NT" ]]
}

# Git Bash 里 go/yarn 跑完后 PATH 经常丢 System32，cmd.exe 会变 command not found。
windows_sys32() {
  local s
  s="$(cygpath -S 2>/dev/null || true)"
  if [[ -n "$s" ]]; then
    cygpath -u "$s"
    return
  fi
  echo "/c/Windows/System32"
}

windows_root() {
  local s
  s="$(cygpath -W 2>/dev/null || true)"
  if [[ -n "$s" ]]; then
    cygpath -u "$s"
    return
  fi
  echo "/c/Windows"
}

win32() {
  local exe="$1"
  shift
  local bin
  case "$exe" in
    explorer.exe) bin="$(windows_root)/explorer.exe" ;;
    *) bin="$(windows_sys32)/$exe" ;;
  esac
  MSYS_NO_PATHCONV=1 "$bin" "$@"
}

rocket_exe_running() {
  win32 tasklist.exe /FI "IMAGENAME eq rocket.exe" 2>/dev/null | grep -qi rocket.exe
}

# 本机 Windows 编 rocket.exe 时，正在跑的进程会锁文件导致 go build / upx 失败。
stop_running_rocket_exe() {
  local f="$SCRIPT_DIR/build/rocket.exe"
  echo "结束本机 rocket.exe ..."
  if rocket_exe_running; then
    win32 taskkill.exe /F /IM rocket.exe /T >/dev/null 2>&1 || true
  fi
  local i
  for i in $(seq 1 30); do
    rocket_exe_running || break
    sleep 1
  done
  if rocket_exe_running; then
    echo "rocket.exe 仍在运行，无法覆盖" >&2
    exit 1
  fi
  [[ -f "$f" ]] || return 0
  for i in $(seq 1 20); do
    if mv -f "$f" "$f.unlock" 2>/dev/null; then
      mv -f "$f.unlock" "$f"
      return 0
    fi
    echo "build/rocket.exe 仍被占用 (${i}s)"
    sleep 1
  done
  echo "build/rocket.exe 仍被占用，无法覆盖" >&2
  exit 1
}

install_windows_startup() {
  local src="$SCRIPT_DIR/build/rocket.exe"
  local dest="${APPDATA}/Microsoft/Windows/Start Menu/Programs/Startup/rocket.exe"
  local dest_win
  [[ -f "$src" ]] || { echo "缺少 $src" >&2; exit 1; }
  mkdir -p "$(dirname "$dest")"
  cp -f "$src" "$dest"
  dest_win="$(cygpath -w "$dest")"
  echo "已写入 $dest_win"
  win32 cmd.exe /c start "" "$dest_win"
  local i
  for i in $(seq 1 45); do
    if curl -fsS --max-time 2 http://127.0.0.1:19527/api/status >/dev/null 2>&1; then
      echo "rocket admin ok (${i}s)"
      win32 explorer.exe /select,"$dest_win"
      return 0
    fi
    sleep 1
  done
  echo "rocket admin 未就绪" >&2
  exit 1
}

# 覆盖本机 GOTOOLCHAIN=local / GOSUMDB=off，按 go.mod 的 toolchain 拉对应 Go。
ensure_go_toolchain() {
  local toolchain sumdb
  toolchain=$(awk '/^toolchain[[:space:]]/{print $2; exit}' "$SCRIPT_DIR/go.mod")
  if [[ -z "$toolchain" ]]; then
    return 0
  fi
  # 必须先读 GOSUMDB：一旦 export GOTOOLCHAIN，go env 会去拉 toolchain，GOSUMDB=off 会直接失败。
  sumdb=$(go env GOSUMDB 2>/dev/null || true)
  export GOTOOLCHAIN="$toolchain"
  if [[ "$sumdb" == "off" ]]; then
    export GOSUMDB=sum.golang.google.cn
  fi
  echo "Go toolchain: $GOTOOLCHAIN"
}

parse_args() {
  CLI_TARGET=""
  INSTALL_STARTUP=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --os)
        [[ $# -ge 2 ]] || { echo "--os 需要参数"; exit 1; }
        normalize_target "$2"
        CLI_TARGET=$TARGET
        shift 2
        ;;
      --os=*)
        normalize_target "${1#--os=}"
        CLI_TARGET=$TARGET
        shift
        ;;
      -r | --reuse-last) REUSE_LAST=1; shift ;;
      --install-startup) INSTALL_STARTUP=1; shift ;;
      -h | --help) print_usage; exit 0 ;;
      *)
        echo "未知参数: $1"
        print_usage
        exit 1
        ;;
    esac
  done
}

resolve_build_choices() {
  if [[ $REUSE_LAST -eq 1 ]]; then
    load_build_prefs
    BUILD_CHOICES_REUSED=1
    print_build_prefs_summary
    return
  fi

  if [[ -f "$BUILD_PREFS" ]] && ask_yes_no_default_yes "沿用上次全部构建选择"; then
    load_build_prefs
    BUILD_CHOICES_REUSED=1
    print_build_prefs_summary
    return
  fi

  BUILD_CHOICES_REUSED=0
  set_default_build_prefs

  if ask_yes_no_default_yes "是否配置/更新构建默认值（Rocket 地址、mTLS 证书）"; then
    CONFIGURE_DEFAULTS=1
  else
    CONFIGURE_DEFAULTS=0
  fi

  if ask_yes_no_default_yes "是否注入 mTLS 证书（打包进二进制）"; then
    INJECT_MTLS=1
  else
    INJECT_MTLS=0
  fi

  if ask_yes_no "是否下载并更新 geoip.dat / geosite.dat（xget → v2ray-rules-dat）？"; then
    DOWNLOAD_GEODATA=1
  else
    DOWNLOAD_GEODATA=0
  fi

  if ask_yes_no_default_yes "是否构建本地管理后台前端（yarn → main/z/localadmin/web/dist）"; then
    BUILD_FRONTEND=1
  else
    BUILD_FRONTEND=0
  fi

  if ask_yes_no "是否执行 upx 二次压缩？"; then
    RUN_UPX=1
  else
    RUN_UPX=0
  fi

  if ask_yes_no "是否打包 zip 压缩包？"; then
    MAKE_ZIP=1
  else
    MAKE_ZIP=0
  fi
}

expand_path() {
  local p="$1"
  p="${p/#\~/$HOME}"
  if [[ "$p" != /* ]]; then
    p="$SCRIPT_DIR/$p"
  fi
  printf '%s' "$p"
}

load_build_config() {
  CA_FILE=""
  P12_FILE=""
  P12_PASS="changeit"
  ROCKET_URL=""
  local cfg="$BUILD_CONFIG"
  if [[ ! -f "$cfg" && -f "$LEGACY_BUILD_CONFIG" ]]; then
    cfg="$LEGACY_BUILD_CONFIG"
  fi
  if [[ -f "$cfg" ]]; then
    # shellcheck disable=SC1090
    source "$cfg"
  fi
}

save_build_config() {
  cat >"$BUILD_CONFIG" <<EOF
CA_FILE='$CA_FILE'
P12_FILE='$P12_FILE'
P12_PASS='$P12_PASS'
ROCKET_URL='$ROCKET_URL'
EOF
  chmod 600 "$BUILD_CONFIG"
  echo "已保存构建配置到 $BUILD_CONFIG"
}

prompt_path() {
  local label="$1"
  local current="$2"
  local input path
  while true; do
    if [[ -n "$current" ]]; then
      read -r -p "$label [$current]: " input
      path="${input:-$current}"
    else
      read -r -p "$label: " path
    fi
    path="$(expand_path "$path")"
    if [[ -f "$path" ]]; then
      printf '%s' "$path"
      return 0
    fi
    echo "文件不存在: $path"
  done
}

configure_build_paths() {
  load_build_config

  if [[ $BUILD_CHOICES_REUSED -eq 1 ]]; then
    if [[ -n "$CA_FILE" && -n "$P12_FILE" ]]; then
      return 0
    fi
    echo "沿用选择需要证书路径，但 .build.local 不完整。"
    echo "请 ./build.sh 重新交互选择，或删除 .build.prefs.local 后重试。"
    exit 1
  fi

  local reconfigure=1
  if [[ -f "$BUILD_CONFIG" || -f "$LEGACY_BUILD_CONFIG" ]]; then
    if [[ -n "$CA_FILE" || -n "$P12_FILE" || -n "$ROCKET_URL" ]]; then
      echo "已保存的构建配置："
      [[ -n "$ROCKET_URL" ]] && echo "  Rocket 地址: $ROCKET_URL"
      [[ -n "$CA_FILE" ]] && echo "  根 CA:       $CA_FILE"
      [[ -n "$P12_FILE" ]] && echo "  P12:         $P12_FILE"
      [[ -n "$P12_PASS" ]] && echo "  P12 密码:    $P12_PASS"
      if ask_yes_no_default_yes "使用以上配置"; then
        USE_SAVED_PATHS=1
        reconfigure=0
      else
        USE_SAVED_PATHS=0
      fi
    fi
  fi
  if [[ "$reconfigure" -eq 1 ]]; then
    local input
    read -r -p "Rocket WebSocket 地址 [${ROCKET_URL:-wss://47.98.194.25/rpc}]: " input
    ROCKET_URL="${input:-${ROCKET_URL:-wss://47.98.194.25/rpc}}"
    CA_FILE="$(prompt_path "根 CA 文件路径（如 顶级证书.pem）" "$CA_FILE")"
    P12_FILE="$(prompt_path "客户端 P12 文件路径（含证书与私钥）" "$P12_FILE")"
    read -r -p "P12 密码 [${P12_PASS:-changeit}]: " input
    P12_PASS="${input:-${P12_PASS:-changeit}}"
    USE_SAVED_PATHS=0
    save_build_config
  fi
}

stage_mtls_certs() {
  if [[ -z "$CA_FILE" || -z "$P12_FILE" ]]; then
    echo "mTLS 证书路径未配置。"
    exit 1
  fi
  echo "注入 mTLS 证书到 $CERT_DST ..."
  cp "$CA_FILE" "$CERT_DST/root_ca.pem"
  cp "$P12_FILE" "$CERT_DST/client.p12"
  printf '%s' "${P12_PASS:-changeit}" >"$CERT_DST/client.p12.pass"
}

apply_build_ldflags() {
  BUILD_STAMP="$(date -u +%Y%m%d-%H%M%S)"
  LDFLAGS="-s -w -buildid="
  LDFLAGS+=" -X github.com/v2fly/v2ray-core/v5/main/z/localadmin.ClientBuild=${BUILD_STAMP}-cndns-tcplocal"
  if [[ -n "$ROCKET_URL" ]]; then
    LDFLAGS+=" -X github.com/v2fly/v2ray-core/v5/main/z/rvstore.buildDefaultRocketURL=${ROCKET_URL}"
    case "$ROCKET_URL" in
      wss://* | WSS://*) LDFLAGS+=" -X github.com/v2fly/v2ray-core/v5/main/z/rvstore.buildDefaultTLSMode=embed" ;;
    esac
    echo "默认 Rocket 地址: $ROCKET_URL"
  fi
  echo "客户端标记: ${BUILD_STAMP}-cndns-tcplocal"
}

BUILD_FLAGS="-trimpath -tags with_gvisor"

parse_args "$@"

# 创建构建目录
mkdir -p build

echo "开始构建 rocket..."

# ---------------------------------------------------------------------------
# 构建步骤选择（保存于 .build.prefs.local，不入库）
# ---------------------------------------------------------------------------
resolve_build_choices

# CLI --os 覆盖 prefs / 默认
if [[ -n "${CLI_TARGET:-}" ]]; then
  TARGET=$CLI_TARGET
fi
echo "目标平台: $TARGET"

# ---------------------------------------------------------------------------
# 构建默认值：Rocket 地址（ldflags）+ mTLS 证书路径（保存于 .build.local，不入库）
# ---------------------------------------------------------------------------
if [[ $CONFIGURE_DEFAULTS -eq 1 ]]; then
  configure_build_paths
else
  load_build_config
fi

if [[ $BUILD_CHOICES_REUSED -eq 0 ]]; then
  save_build_prefs
fi

apply_build_ldflags

# ---------------------------------------------------------------------------
# mTLS：构建前从本地路径注入 go:embed 材料
# ---------------------------------------------------------------------------
if [[ $INJECT_MTLS -eq 1 ]]; then
  stage_mtls_certs
else
  if [[ ! -f "$CERT_DST/root_ca.pem" || ! -f "$CERT_DST/client.p12" || ! -f "$CERT_DST/client.p12.pass" ]]; then
    echo "未注入 mTLS 且 $CERT_DST 缺少证书文件，go build 会失败。"
    echo "请重新运行并选择注入，或手动放置 root_ca.pem / client.p12 / client.p12.pass。"
    echo "也可 ./build.sh 重新交互选择，或删除 .build.prefs.local 后重试。"
    exit 1
  fi
  echo "使用 $CERT_DST 已有证书继续构建。"
fi

# ---------------------------------------------------------------------------
# Loyalsoldier/v2ray-rules-dat（https://github.com/Loyalsoldier/v2ray-rules-dat）
# - 每日构建的「加强版」V2Ray 路由数据，可替代官方 geoip.dat / geosite.dat。
# - geoip.dat：基于 Loyalsoldier/geoip（MaxMind 等），含 geoip:cn 及 telegram、
#   google、netflix 等扩展类别（见该仓库 README）。
# - geosite.dat：在 v2fly/domain-list-community 基础上合并大陆域名列表、GFWList、
#   广告列表等；并多出 geosite:china-list、apple-cn、google-cn、win-* 等类别。
# - 同一 GitHub Release 还附带：direct-list.txt、proxy-list.txt、reject-list.txt、
#   china-list.txt、apple-cn.txt、gfw.txt、win-*.txt 等纯文本列表，以及各文件的
#   *.sha256sum；本客户端仅在 main/z/res/geo_data.go 里 go:embed 上述两个 .dat，
#   故脚本只下载 geoip.dat 与 geosite.dat（经 xget 加速的 GitHub releases 路径）。
# ---------------------------------------------------------------------------
if [[ $DOWNLOAD_GEODATA -eq 1 ]]; then
  echo "更新 geoip.dat / geosite.dat ..."
  XGET_GH="https://xget.kjsup.cn/gh/Loyalsoldier/v2ray-rules-dat/releases/latest/download"
  curl -fL "${XGET_GH}/geoip.dat" -o "$RES_DIR/geoip.dat"
  curl -fL "${XGET_GH}/geosite.dat" -o "$RES_DIR/geosite.dat"
  curl -fL "${XGET_GH}/geoip.dat.sha256sum" -o "$RES_DIR/geoip.dat.sha256sum"
  curl -fL "${XGET_GH}/geosite.dat.sha256sum" -o "$RES_DIR/geosite.dat.sha256sum"
  (
    cd "$RES_DIR" || exit 1
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum -c geoip.dat.sha256sum && sha256sum -c geosite.dat.sha256sum
    elif command -v shasum >/dev/null 2>&1; then
      shasum -a 256 -c geoip.dat.sha256sum && shasum -a 256 -c geosite.dat.sha256sum
    else
      echo "未找到 sha256sum/shasum，跳过 SHA256 校验。"
    fi
  ) || {
    echo "geoip/geosite SHA256 校验失败，请检查网络或 xget 镜像。"
    exit 1
  }
  rm -f "$RES_DIR/geoip.dat.sha256sum" "$RES_DIR/geosite.dat.sha256sum"
else
  echo "跳过 geodata 下载。"
fi

# ---------------------------------------------------------------------------
# 本地管理后台（go:embed web/dist）
# ---------------------------------------------------------------------------
if [[ $BUILD_FRONTEND -eq 1 ]]; then
  if ! command -v yarn >/dev/null 2>&1; then
    echo "未找到 yarn，请先安装 Node.js / yarn。"
    exit 1
  fi
  echo "构建 localadmin 前端..."
  (
    cd "$WEB_DIR" || exit 1
    yarn install
    yarn build
  ) || {
    echo "前端构建失败。"
    exit 1
  }
else
  if [[ ! -f "$SCRIPT_DIR/main/z/localadmin/web/dist/index.html" ]]; then
    echo "未构建前端且缺少 web/dist/index.html，go build 会失败。"
    echo "请重新运行并选择构建前端，或在 $WEB_DIR 执行 yarn build。"
    echo "也可 ./build.sh 重新交互选择，或删除 .build.prefs.local 后重试。"
    exit 1
  fi
  echo "使用已有 web/dist 继续构建。"
fi

CLIENT_PKG=github.com/v2fly/v2ray-core/v5/main/z/client
HELPER_PKG=github.com/v2fly/v2ray-core/v5/main/z/cmd/v2ray-helper
BUILT_LINUX=0
BUILT_WINDOWS=0

ensure_go_toolchain

if target_enabled darwin; then
  echo "构建 macOS ARM64..."
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $BUILD_FLAGS -ldflags "$LDFLAGS" -o build/rocket "$CLIENT_PKG" || exit 1
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $BUILD_FLAGS -ldflags "$LDFLAGS" -o build/v2ray-helper "$HELPER_PKG" || exit 1
  # launchd plist 已 go:embed 进 rocket，安装 Helper 时写出，无需旁路拷贝 plist。

  echo "构建 macOS x86_64..."
  CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $BUILD_FLAGS -ldflags "$LDFLAGS" -o build/rocket-darwin-x86 "$CLIENT_PKG" || exit 1
  CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $BUILD_FLAGS -ldflags "$LDFLAGS" -o build/v2ray-helper-darwin-x86 "$HELPER_PKG" || exit 1
fi

if target_enabled linux; then
  echo "构建 Linux x86_64..."
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $BUILD_FLAGS -ldflags "$LDFLAGS" -o build/rocket-linux "$CLIENT_PKG" || exit 1
  BUILT_LINUX=1
fi

if target_enabled windows; then
  if is_windows_host; then
    stop_running_rocket_exe
  fi
  # -H windowsgui：无控制台黑框
  echo "构建 Windows x86_64..."
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $BUILD_FLAGS -ldflags "$LDFLAGS -H windowsgui" -o build/rocket.exe "$CLIENT_PKG" || exit 1
  BUILT_WINDOWS=1
fi

# 可选：进一步压缩（需确认）
if [[ $RUN_UPX -eq 1 ]]; then
  if command -v upx >/dev/null 2>&1; then
    echo "检测到 upx，开始二次压缩（跳过 macOS）..."
    [[ $BUILT_LINUX -eq 1 ]] && upx --best --lzma build/rocket-linux || true
    [[ $BUILT_WINDOWS -eq 1 ]] && upx --best --lzma build/rocket.exe || true
  else
    echo "未检测到 upx，跳过二次压缩。"
  fi
else
  echo "跳过 upx 二次压缩。"
fi

# 可选：生成 zip 压缩包（需确认）
if [[ $MAKE_ZIP -eq 1 ]]; then
  ZIP_NAME="rocket-build-$(date +%Y%m%d-%H%M%S).zip"
  echo "打包 zip: $ZIP_NAME"
  zip_args=()
  for f in \
    build/rocket \
    build/rocket-darwin-x86 \
    build/rocket-linux \
    build/rocket.exe \
    build/v2ray-helper \
    build/v2ray-helper-darwin-x86
  do
    [[ -f "$f" ]] && zip_args+=("$f")
  done
  if [[ ${#zip_args[@]} -eq 0 ]]; then
    echo "没有可打包的构建产物，跳过 zip。"
  else
    zip -9 -j "build/$ZIP_NAME" "${zip_args[@]}"
  fi
else
  echo "跳过 zip 打包。"
fi

echo "构建完成！目标=$TARGET"
echo "构建文件："
ls -la build/

if [[ ${INSTALL_STARTUP:-0} -eq 1 ]]; then
  if [[ ${BUILT_WINDOWS:-0} -ne 1 ]]; then
    echo "--install-startup 需要本次编出 Windows（--os windows|all）" >&2
    exit 1
  fi
  install_windows_startup
fi
