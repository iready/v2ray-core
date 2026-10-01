---
name: mitm-capture
description: >-
  Rocket 客户端（v2ray 仓库）HTTPS 抓包 / localadmin「抓包台」产品与实现约定。
  用户提到抓包、MITM、Map Remote、媒体绕行、CA 证书、流量列表、go-mitmproxy、
  域名证书、host_certs、微信图片转不出来、localadmin mitm 时使用。
---

# 抓包台约定（换机必读）

代码根：`main/z/mitmctl`、`main/z/rvstore`（`MitmProfile`）、`main/z/localadmin`（API + `web`）。
生效：改代码后 **本机 darwin 打包 & 重启**（quick_cmd / LaunchAgent `rocket.rocket`），勿用未编进二进制的临时路径冒充上线。

## 上游引擎（GitHub）

- 库：[`github.com/lqqyt2423/go-mitmproxy`](https://github.com/lqqyt2423/go-mitmproxy)（Go 实现的 mitmproxy 风格 HTTPS 中间人代理）
- 当前锁定：`go.mod` → **`v1.9.2`**
- 本仓库用法：进程内嵌 `proxy.NewProxy` + 自研 Addon（录制 / Map Remote），**不**默认启动其自带 Web UI
- 关键点约束（改行为前先对照该库源码）：
  - 根 CA 加载为 **RSA**（EC 会起不来）
  - `SetShouldInterceptRule`：false 时 CONNECT **直通不解密**（媒体绕行 / ignore_hosts）
  - Addon `Requestheaders` 里改 `URL.Host`/`Scheme` 后，引擎会改走 **独立上游连接**（Map Remote 依赖此点）
  - CA 目录：`CaRootPath` → 本机 `~/.rv/mitm/`（`mitmproxy-ca.pem` 等）
- 查模块缓存源码：`$(go env GOPATH)/pkg/mod/github.com/lqqyt2423/go-mitmproxy@v1.9.2/`

## 产品形态

- **显式 HTTP 代理**（默认 `:19080`），不是 TUN 透明抓包。
- 主区永远是 **工作台**（HOSTS / STREAM / INSPECT）；Map Remote、域名证书、连接/CA 都走顶栏 **弹框**，不占主区页签。
- 体验按「工具台」：**不要**加键盘快捷键（Esc 关弹框除外）；**不要**在文案里提第三方软件名或「v2ray」品牌话术。
- 弹框壳共用 `MitmDialog`；不要再复制一套遮罩/页脚。

## CA（装设备 / 导入）

- 引擎根 CA **只认 RSA**；EC P12 必须拒绝并提示重置。
- 「下载 CA」默认发 **干净 DER `.cer`**（iOS 隔空投送）；禁止带 PKCS12 bag 头的脏 PEM。
- 导入支持 PEM bundle 或 SSL 页 **P12 Base64 + 密码**；导入前校验：`CA:TRUE`、有效期合理、主题非空。
- 提供「重置为内置 CA」；坏证会导致 `running:false`、列表空、设备代理后「网络不稳定」。
- 手机信任：装 `.cer` 后还需系统「证书信任设置」打开完全信任。

## 流量

- 录制前按 `Content-Encoding` **自动解压** gzip/deflate/br（漏标头但带 gzip 魔数也要解）。
- 三栏：点过的域名（localStorage 留存，可 ★ 置顶）｜实时流｜详情。
- 详情：JSON 美化、curl/url/body 复制按钮即可；选中后实时流可继续刷。
- 过滤：query + 芯片（−OPTIONS / −静态 / API / ≥400）；偏好可持久。

## Map Remote

- 把打向 A 的请求改打到 B（From/To：proto/host/port/path/query，Preserve Host）。
- Path 以 `*` 结尾=前缀；空 Path=整个 Host。
- 规则进 `MitmProfile.map_remote`，保存并 Apply 后生效；命中写 `X-Rocket-Map-From`。
- UI：顶栏弹框（与域名证书 / 连接·CA 同形态）。

## 域名证书（host_certs）

- 对指定 SNI 使用用户提供的 **叶子证+私钥** 作为 MITM 服务端证；未命中仍走全局 SelfSign CA。
- 字段：`host`（精确 / `*.x.com` / 配 `example.com` 时子域也命中）、`cert_pem`、`key_pem`、`enabled`。
- 叶子可用 **RSA/EC**（与根 CA 必须 RSA 不同）；解析 API：`POST /mitm/host-cert/parse`（P12 Base64+密码或 PEM）。
- UI：顶栏弹框；录入模式「证书|私钥」左右分栏，或 Bundle/P12。
- **API 读出不回传 `key_pem`**（只给 `has_key`）；保存时空钥按 id/host 合并旧钥，避免前端重存时抹钥。
- 实现：`NewCaFunc` → `hostCertCA` 包装 `SelfSignCA`，命中则 `GetCert` 直接返回指定证。
- App 证书锁定时，必须正好是被锁定的那张证；否则仍握手失败。

## 泄漏与启停

- 依赖 `third_party/go-mitmproxy`（replace）：`Shutdown`/`Close` 必须停掉 attacker Serve，并 `CloseIdleConnections`；若开了遗留 `web_addr`，Manager 须 `WebAddon.Close`。
- FlowStore 定长环形缓冲，禁止 `seq[1:]` 扩容写法。
- 巡检：`bash main/z/mitmctl/scripts/check-leaks.sh`；`go test ./main/z/mitmctl/ -run Leak`。

## 媒体绕行

- **开关** `media_bypass`，不要默认往 `ignore_hosts` 塞一长串。
- 开启后把内置媒体 CDN **后缀**并入不解密列表（CONNECT 隧道、不 MITM）。
- 判断方式是**域名后缀表**（如 `qpic.cn`、`qlogo.cn`、`tc.qq.com`…），不是 Content-Type；HTTPS CONNECT 阶段看不到路径。
- 微信会话里图片/视频转不出：先开「媒体绕行」，再重开微信。

## 配置落盘

- `~/.rv/agent.json` → `mitm`；CA 文件在 `~/.rv/mitm/`。
- 换机：代码+skill 随仓库走；**本机 CA / agent.json / localStorage 域名留存不随仓库**，新机要重装 CA、按需再开媒体绕行与 Map 规则。

## 改代码时注意

- go-mitmproxy 改 Host/Scheme 后会走独立上游连接；Map Remote 挂在 `Requestheaders`。
- `IgnoreHosts` / 媒体绕行用后缀匹配（已有 `matchHost`）。
- 前端改完 `npm run build` 编进 `localadmin/web/dist`，再打包客户端。
