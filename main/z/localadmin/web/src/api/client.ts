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
  tun_bind_interface?: string
  tun_bind_auto?: boolean
  elevated?: boolean
  autostart_installed?: boolean
  client_build?: string
  exe_path?: string
  os?: string
  features?: PlatformFeatures
}

export interface PlatformFeatures {
  privileged_helper?: boolean
  elevated_auth?: boolean
  login_autostart?: boolean
}

const api = axios.create({ baseURL: '/api' })

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

export async function enableTun(): Promise<TunStatus> {
  const { data } = await api.post<TunStatus>('/tun/enable')
  return data
}

export async function disableTun(): Promise<TunStatus> {
  const { data } = await api.post<TunStatus>('/tun/disable')
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
  const { data } = await api.put<TunProfile>('/tun/profile', profile)
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
  const { data } = await api.post<DiagnoseReport>('/diagnose')
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
