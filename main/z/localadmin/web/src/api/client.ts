import axios from 'axios'

export interface AddressPreset {
  name: string
  url: string
}

export interface TLSConfig {
  mode: string
  ca_file?: string
  cert_file?: string
  key_file?: string
  p12_file?: string
  p12_pass?: string
  server_name?: string
  insecure?: boolean
}

export interface AgentConfig {
  presets: AddressPreset[]
  active: string
  token: string
  sign: string
  tls: TLSConfig
}

export interface ServerInstanceStatus {
  key: string
  status: string
  version: number
  healthy: boolean
  last_heartbeat?: string
  last_error?: string
}

export interface StatusSnapshot {
  wire_connected: boolean
  token_version: number
  server_keys: string[] | null
  server_count: number
  healthy_count: number
  servers: ServerInstanceStatus[] | null
  last_error: string
  connected_at?: string
  started_at?: string
  config_path: string
  agent_ready: boolean
  agent_pid: number
  admin_port: number
  addr?: string
  tun_enabled?: boolean
  tun_helper_installed?: boolean
  tun_if_name?: string
  tun_in_config?: boolean
  tun_local_use?: boolean
  tun_owner_key?: string
  tun_active?: boolean
  tun_degraded_reason?: string
  tun_hijack?: boolean
  tun_hijack_paused?: string
  tun_bind_interface?: string
  tun_bind_auto?: boolean
  elevated?: boolean
  autostart_installed?: boolean
  client_build?: string
  exe_path?: string
  os?: string
  features?: PlatformFeatures
  outbounds?: OutboundSnapshot[]
  goroutines?: number
  heap_mb?: number
  udp_flows?: number
  faults?: FaultItem[]
}

export interface FaultItem {
  at: number
  dest: string
  tag: string
  error: string
  n: number
}

export interface OutboundSnapshot {
  server_key: string
  tag: string
  protocol: string
  address?: string
  port?: number
  network?: string
  security?: string
  remote: boolean
}

export interface OutboundReach {
  server_key: string
  tag: string
  tcp: ProbeLeg
  protocol_cn: ProbeLeg
  protocol_foreign: ProbeLeg
}

export interface ProbeLeg {
  ok: boolean
  latency_ms?: number
  error?: string
}

export interface PlatformFeatures {
  privileged_helper?: boolean
  elevated_auth?: boolean
  login_autostart?: boolean
}

export function tunHijackPausedLabel(paused?: string): string {
  switch (paused) {
    case 'user_off':
      return '用户已关闭'
    case 'engine_off':
      return '网卡未运行'
    case 'iface_gone':
      return '默认网卡丢失，暂直出'
    case 'cn_outage':
      return '国内大面积不通，暂直出'
    default:
      return paused ? `暂直出（${paused}）` : '暂直出'
  }
}

export function tunTrafficLabel(status: StatusSnapshot): string {
  if (status.tun_hijack) return '流量经 TUN'
  return tunHijackPausedLabel(status.tun_hijack_paused)
}

export function tunRuntimeCaption(status?: StatusSnapshot | null): string {
  if (!status) return ''
  const engine = status.tun_enabled ? '网卡已拉起' : '网卡未运行'
  if (!status.tun_enabled) return engine
  if (status.tun_hijack) return `${engine} · 流量经 TUN`
  return `${engine} · ${tunHijackPausedLabel(status.tun_hijack_paused)}`
}

const api = axios.create({ baseURL: '/api', timeout: 8000 })
let csrfToken = ''
let csrfLoading: Promise<string> | null = null
api.interceptors.request.use(async config => {
  if (['post', 'put', 'patch', 'delete'].includes(String(config.method || 'get').toLowerCase())) {
    if (!csrfToken) {
      csrfLoading ||= axios.get<{csrf_token: string}>('/api/session').then(({data}) => {
        csrfToken = data.csrf_token
        return csrfToken
      }).finally(() => { csrfLoading = null })
      await csrfLoading
    }
    config.headers.set('X-Rocket-CSRF', csrfToken)
  }
  return config
})
export function apiErrorMessage(err: unknown, fallback: string): string {
  if (axios.isAxiosError(err)) {
    const body = err.response?.data
    if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string') {
      return body.error
    }
    if (err.message) return err.message
  }
  if (err instanceof Error) return err.message
  return fallback
}

export async function fetchStatus(): Promise<StatusSnapshot> {
  const { data } = await api.get<StatusSnapshot>('/status')
  return data
}

export async function probeOutboundReach(serverKey: string, tag: string): Promise<OutboundReach> {
  const { data } = await api.post<OutboundReach>(
    '/outbounds/reach',
    { server_key: serverKey, tag },
    { timeout: 15000 },
  )
  return data
}

export async function fetchConfig(): Promise<AgentConfig> {
  const { data } = await api.get<AgentConfig>('/config')
  return data
}

export async function saveConfig(cfg: AgentConfig): Promise<AgentConfig> {
  const { data } = await api.put<AgentConfig>('/config', cfg)
  return data
}

export async function reconnect(): Promise<void> {
  await api.post('/reconnect')
}

export async function restartAgent(): Promise<void> {
  await api.post('/restart')
}

export async function stopAgent(): Promise<void> {
  await api.post('/stop')
}

export interface TunStatus {
  enabled: boolean
  helper_installed: boolean
  if_name?: string
  error?: string
}

export async function fetchTunStatus(): Promise<TunStatus> {
  const { data } = await api.get<TunStatus>('/tun/status')
  return data
}

// 开关和保存都会重拉整个实例，本来就要几十秒，不套全局 8s。
const tunApplyTimeout = 0

export async function enableTun(): Promise<TunStatus> {
  const { data } = await api.post<TunStatus>('/tun/enable', null, { timeout: tunApplyTimeout })
  return data
}

export async function disableTun(): Promise<TunStatus> {
  const { data } = await api.post<TunStatus>('/tun/disable', null, { timeout: tunApplyTimeout })
  return data
}

export async function installTunHelper(): Promise<void> {
  await api.post('/tun/install-helper')
}

export async function uninstallTunHelper(): Promise<void> {
  await api.post('/tun/uninstall-helper')
}

export interface TunProfile {
  use: boolean
  tun_owner_key?: string
  tun_owner_candidates?: string[]
  bind_interface?: string
  bind_interface_candidates?: string[]
  bind_interface_effective?: string
  cn_dns?: string[]
  cn_dns_default?: string[]
  remote_dns?: string
  remote_dns_default?: string
  fakedns_domains?: string[]
  bypass_rocket_server?: boolean
  bypass_lan?: boolean
  bypass_loopback?: boolean
  extra_bypass_hosts?: string[]
  services?: Record<string, unknown>
  server_has_template: boolean
}

export async function fetchTunProfile(): Promise<TunProfile> {
  const { data } = await api.get<TunProfile>('/tun/profile')
  return data
}

export async function saveTunProfile(profile: TunProfile): Promise<TunProfile> {
  const { data } = await api.put<TunProfile>('/tun/profile', profile, { timeout: tunApplyTimeout })
  return data
}

export interface LogProfile {
  use: boolean
  loglevel?: string
  access?: string
  error?: string
  server_loglevel?: string
}

export async function fetchLogProfile(): Promise<LogProfile> {
  const { data } = await api.get<LogProfile>('/log/profile')
  return data
}

export async function saveLogProfile(profile: LogProfile): Promise<LogProfile> {
  const { data } = await api.put<LogProfile>('/log/profile', profile)
  return data
}

export interface MitmConnectEndpoint {
  host: string
  port: string
  proxy: string
  interface?: string
  local?: boolean
}

export interface MitmStatus {
  running: boolean
  addr?: string
  web_addr?: string
  upstream?: string
  ca_cert_path?: string
  error?: string
  ssl_insecure?: boolean
  flow_count?: number
  port?: string
  connect?: MitmConnectEndpoint[]
}

export interface MitmMapRemoteRule {
  id?: string
  enabled: boolean
  from_proto?: string
  from_host?: string
  from_port?: string
  from_path?: string
  from_query?: string
  to_proto?: string
  to_host?: string
  to_port?: string
  to_path?: string
  to_query?: string
  preserve_host?: boolean
  note?: string
}

export interface MitmHostCertRule {
  id?: string
  enabled: boolean
  host: string
  cert_pem: string
  key_pem?: string
  has_key?: boolean
  note?: string
}

export interface MitmProfile {
  use: boolean
  addr?: string
  web_addr?: string
  upstream?: string
  ssl_insecure?: boolean
  ignore_hosts?: string[]
  media_bypass?: boolean
  map_remote?: MitmMapRemoteRule[]
  host_certs?: MitmHostCertRule[]
  running?: boolean
  ca_cert_path?: string
  error?: string
  port?: string
  connect?: MitmConnectEndpoint[]
}

export async function fetchMitmProfile(): Promise<MitmProfile> {
  const { data } = await api.get<MitmProfile>('/mitm/profile')
  return data
}

export async function saveMitmProfile(profile: MitmProfile): Promise<MitmProfile> {
  const { data } = await api.put<MitmProfile>('/mitm/profile', profile)
  return data
}

export async function fetchMitmStatus(): Promise<MitmStatus> {
  const { data } = await api.get<MitmStatus>('/mitm/status')
  return data
}

export async function enableMitm(): Promise<MitmStatus> {
  const { data } = await api.post<MitmStatus>('/mitm/enable')
  return data
}

export async function disableMitm(): Promise<MitmStatus> {
  const { data } = await api.post<MitmStatus>('/mitm/disable')
  return data
}

export function mitmCADownloadURL(): string {
  // iOS 隔空投送需要 DER .cer；带 PKCS12 头的 PEM 会报「无效的描述文件」
  return '/api/mitm/ca.pem?format=cer'
}

export async function exportMitmCABase64(kind: 'cert' | 'bundle' = 'cert'): Promise<string> {
  const { data } = await api.get<{ content: string }>('/mitm/ca.base64', { params: { kind } })
  return data.content
}

export async function importMitmCA(content: string, password = ''): Promise<MitmProfile> {
  const { data } = await api.post<MitmProfile>('/mitm/ca/import', { content, password })
  return data
}

export async function resetMitmCA(): Promise<MitmProfile> {
  const { data } = await api.post<MitmProfile>('/mitm/ca/reset')
  return data
}

export async function parseMitmHostCert(
  content: string,
  password = '',
): Promise<{ cert_pem: string; key_pem: string }> {
  const { data } = await api.post<{ cert_pem: string; key_pem: string }>('/mitm/host-cert/parse', {
    content,
    password,
  })
  return data
}

export interface MitmFlowSummary {
  id: string
  method: string
  url: string
  host: string
  status_code?: number
  req_size: number
  resp_size: number
  duration_ms?: number
  started_at: string
  error?: string
}

export interface MitmFlowDetail extends MitmFlowSummary {
  req_headers?: Record<string, string[]>
  resp_headers?: Record<string, string[]>
  req_body?: string
  resp_body?: string
  req_body_truncated?: boolean
  resp_body_truncated?: boolean
}

export async function fetchMitmFlows(): Promise<MitmFlowSummary[]> {
  const { data } = await api.get<{ flows: MitmFlowSummary[] }>('/mitm/flows')
  return data.flows ?? []
}

export async function fetchMitmFlow(id: string): Promise<MitmFlowDetail> {
  const { data } = await api.get<MitmFlowDetail>(`/mitm/flows/${encodeURIComponent(id)}`)
  return data
}

export async function clearMitmFlows(): Promise<void> {
  await api.delete('/mitm/flows')
}

export interface TrafficItem {
  id: string
  at: number
  dest: string
  tag: string
  pending: boolean
  ok: boolean
  error?: string
  dur_ms: number
}

export interface TrafficView {
  active: boolean
  started_at?: number
  stopped_at?: number
  total: number
  items: TrafficItem[]
}

export async function clearFaults(): Promise<void> {
  await api.post('/faults/clear')
}

export async function fetchFaultCopy(i: number): Promise<string> {
  const { data } = await api.get<{ text: string }>('/faults/copy', { params: { i } })
  return data.text
}

export async function fetchTraffic(limit = 500): Promise<TrafficView> {
  const { data } = await api.get<TrafficView>('/traffic', { params: { limit } })
  return { ...data, items: data.items ?? [] }
}

export async function startTraffic(): Promise<TrafficView> {
  const { data } = await api.post<TrafficView>('/traffic/start')
  return { ...data, items: data.items ?? [] }
}

export async function stopTraffic(): Promise<TrafficView> {
  const { data } = await api.post<TrafficView>('/traffic/stop')
  return { ...data, items: data.items ?? [] }
}

export interface DiagnoseCheck {
  id: string
  title: string
  ok: boolean
  level: 'ok' | 'warn' | 'fail' | string
  detail: string
  hint?: string
  elapsed?: string
}

export interface DiagnoseReport {
  ok: boolean
  summary: string
  verdict?: string
  cause?: string
  fix?: string
  checks: DiagnoseCheck[]
}

export async function runDiagnose(): Promise<DiagnoseReport> {
  // 后端单项/整体已有超时；前端再兜一层，避免卡顿时页面一直转圈
  const { data } = await api.post<DiagnoseReport>('/diagnose', null, { timeout: 20000 })
  return data
}

export interface AutostartStatus {
  elevated: boolean
  installed: boolean
  message?: string
}

export async function fetchAutostart(): Promise<AutostartStatus> {
  const { data } = await api.get<AutostartStatus>('/autostart')
  return data
}

export async function installAutostart(): Promise<AutostartStatus> {
  const { data } = await api.post<AutostartStatus>('/autostart/install')
  return data
}

export async function uninstallAutostart(): Promise<AutostartStatus> {
  const { data } = await api.post<AutostartStatus>('/autostart/uninstall')
  return data
}

export async function relaunchAutostart(): Promise<AutostartStatus> {
  const { data } = await api.post<AutostartStatus>('/autostart/relaunch')
  return data
}

export interface DomainRouteRequestItem {
  client_req_id: string
  request_id?: string
  domains: string[]
  remark?: string
  status: string
  reject_reason?: string
  added?: number
  skipped?: number
  route_names?: string[]
  created_at: string
  updated_at: string
  last_error?: string
}

export async function fetchDomainRouteRequests(): Promise<DomainRouteRequestItem[]> {
  const { data } = await api.get<{ items: DomainRouteRequestItem[] }>('/domain-route-requests')
  return data.items ?? []
}

export async function submitDomainRouteRequest(text: string, remark = ''): Promise<DomainRouteRequestItem> {
  const { data } = await api.post<DomainRouteRequestItem>('/domain-route-requests', { text, remark })
  return data
}

export async function flushDomainRouteRequests(): Promise<{ flushed: number; items: DomainRouteRequestItem[]; error?: string }> {
  const { data } = await api.post<{ flushed: number; items: DomainRouteRequestItem[]; error?: string }>(
    '/domain-route-requests/flush',
  )
  return data
}

export interface EditableServerSummary {
  id: string
  key: string
  address: string
  version: number
  token_version: number
  client_version: number | null
  online: boolean
  healthy: boolean
  last_error: string
  synced: boolean
  shared_token_count: number
}

export interface EditableServerDocument {
  id: string
  key: string
  address: string
  version: number
  outbound_tags: string[]
  owner: { name: string; contact?: string; managed_by?: string }
  config: Record<string, unknown>
}

export async function fetchMyEditableServers(): Promise<{ owner: {name: string; contact?: string; managed_by?: string}; rows: EditableServerSummary[] }> {
  const {data} = await api.get('/my-servers')
  return data
}

export async function fetchMyEditableServer(id: string): Promise<EditableServerDocument> {
  const {data} = await api.get<EditableServerDocument>(`/my-servers/${encodeURIComponent(id)}`)
  return data
}

export async function saveMyEditableServer(id: string, version: number, config: Record<string, unknown>): Promise<{ server_id: string; version: number; token_version: number; online: boolean; online_client_version: number | null; sync_error?: string }> {
  const {data} = await api.put(`/my-servers/${encodeURIComponent(id)}`, {id, version, config}, {timeout: 45000})
  return data
}
