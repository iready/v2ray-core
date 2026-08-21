import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import {
  apiErrorMessage,
  disableTun,
  enableTun,
  fetchStatus,
  fetchTunProfile,
  installTunHelper,
  saveTunProfile,
  uninstallTunHelper,
  type StatusSnapshot,
  type TunProfile,
} from '@/api/client'

const DEFAULT_TUN_IPV4 = '172.19.0.1'
const DEFAULT_TUN_PREFIX = 30

function boolDefault(v: boolean | undefined, def: boolean): boolean {
  return v === undefined ? def : v
}

function rocketHostFromAddr(addr: string | undefined): string {
  if (!addr) return '—'
  try {
    const u = new URL(addr)
    return u.hostname || addr
  } catch {
    return addr
  }
}

function tunDeviceName(osName: string | undefined): string {
  if (osName === 'darwin') return 'utun'
  if (osName === 'windows') return 'wintun'
  return 'tun'
}

function parseIPv4Octets(ip: string): number[] | null {
  const parts = ip.trim().split('.')
  if (parts.length !== 4) return null
  const octets = parts.map((p) => parseInt(p, 10))
  if (octets.some((n) => Number.isNaN(n) || n < 0 || n > 255)) return null
  return octets
}

function readTunIPv4(services: Record<string, unknown> | undefined): string {
  const ips = services?.ips
  if (!Array.isArray(ips) || ips.length === 0) return DEFAULT_TUN_IPV4
  const first = ips[0] as { ip?: number[] } | undefined
  if (!first?.ip || first.ip.length !== 4) return DEFAULT_TUN_IPV4
  return first.ip.join('.')
}

function readTunPrefix(services: Record<string, unknown> | undefined): number {
  const ips = services?.ips
  if (!Array.isArray(ips) || ips.length === 0) return DEFAULT_TUN_PREFIX
  const first = ips[0] as { prefix?: number } | undefined
  const p = first?.prefix
  return typeof p === 'number' && p > 0 && p <= 32 ? p : DEFAULT_TUN_PREFIX
}

function withTunIPv4(
  services: Record<string, unknown> | undefined,
  ipv4: string,
  prefix: number,
): Record<string, unknown> {
  const octets = parseIPv4Octets(ipv4)
  if (!octets) return services ?? {}
  return { ...(services ?? {}), ips: [{ ip: octets, prefix }] }
}

function applyProfileToForm(
  p: TunProfile,
  setTunIPv4: (v: string) => void,
  setTunPrefix: (v: string) => void,
): void {
  setTunIPv4(readTunIPv4(p.services))
  setTunPrefix(String(readTunPrefix(p.services)))
}

type CheckRowProps = {
  checked: boolean
  label: string
  hint?: string
  onChange: (checked: boolean) => void
}

function CheckRow({ checked, label, hint, onChange }: CheckRowProps) {
  return (
    <label className="flex cursor-pointer items-start gap-2">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="mt-0.5 size-4 shrink-0"
      />
      <span className="space-y-0.5">
        <span className="block text-sm">{label}</span>
        {hint ? <span className="text-muted-foreground block text-xs">{hint}</span> : null}
      </span>
    </label>
  )
}

export default function TunPage() {
  const [profile, setProfile] = useState<TunProfile | null>(null)
  const [runtime, setRuntime] = useState<StatusSnapshot | null>(null)
  const [tunIPv4, setTunIPv4] = useState(DEFAULT_TUN_IPV4)
  const [tunPrefix, setTunPrefix] = useState(String(DEFAULT_TUN_PREFIX))
  const [extraHostInput, setExtraHostInput] = useState('')
  const [cnDnsInput, setCnDnsInput] = useState('')
  const [rocketAddr, setRocketAddr] = useState<string>()
  const [osName, setOsName] = useState<string>()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [opsLoading, setOpsLoading] = useState(false)

  const patch = useCallback((partial: Partial<TunProfile>) => {
    setProfile((prev) => (prev ? { ...prev, ...partial } : prev))
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [p, status] = await Promise.all([fetchTunProfile(), fetchStatus()])
      setProfile(p)
      setRuntime(status)
      setRocketAddr(status.addr)
      setOsName(status.os)
      applyProfileToForm(p, setTunIPv4, setTunPrefix)
    } catch (e) {
      toast.error(apiErrorMessage(e, '加载 TUN 配置失败'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const extraHosts = profile?.extra_bypass_hosts ?? []
  const cnDnsList = profile?.cn_dns ?? []
  const showHelper = !!runtime?.features?.privileged_helper

  async function refreshRuntime(): Promise<void> {
    const status = await fetchStatus()
    setRuntime(status)
    setRocketAddr(status.addr)
    setOsName(status.os)
  }

  async function handleTunToggle(): Promise<void> {
    setOpsLoading(true)
    try {
      if (runtime?.tun_enabled) {
        await disableTun()
        toast.success('TUN 已关闭')
      } else {
        await enableTun()
        toast.success('TUN 已开启')
      }
      await refreshRuntime()
      const p = await fetchTunProfile()
      setProfile(p)
    } catch (e) {
      toast.error(apiErrorMessage(e, 'TUN 操作失败'))
    } finally {
      setOpsLoading(false)
    }
  }

  async function handleInstallHelper(): Promise<void> {
    setOpsLoading(true)
    try {
      await installTunHelper()
      toast.success('Helper 安装完成')
      await refreshRuntime()
    } catch (e) {
      toast.error(apiErrorMessage(e, '安装失败'))
    } finally {
      setOpsLoading(false)
    }
  }

  async function handleUninstallHelper(): Promise<void> {
    setOpsLoading(true)
    try {
      await uninstallTunHelper()
      toast.success('Helper 已拆卸')
      await refreshRuntime()
    } catch (e) {
      toast.error(apiErrorMessage(e, '拆卸失败'))
    } finally {
      setOpsLoading(false)
    }
  }

  function addCnDns(): void {
    if (!profile) return
    const addr = cnDnsInput.trim()
    if (!addr || cnDnsList.includes(addr)) {
      setCnDnsInput('')
      return
    }
    patch({ cn_dns: [...cnDnsList, addr] })
    setCnDnsInput('')
  }

  function removeCnDns(addr: string): void {
    patch({ cn_dns: cnDnsList.filter((item) => item !== addr) })
  }

  function fillCnDnsDefaults(): void {
    const defaults = profile?.cn_dns_default ?? []
    if (defaults.length === 0) return
    patch({ cn_dns: [...defaults] })
  }

  function addExtraHost(): void {
    if (!profile) return
    const host = extraHostInput.trim()
    if (!host || extraHosts.includes(host)) {
      setExtraHostInput('')
      return
    }
    patch({ extra_bypass_hosts: [...extraHosts, host] })
    setExtraHostInput('')
  }

  function removeExtraHost(host: string): void {
    patch({ extra_bypass_hosts: extraHosts.filter((h) => h !== host) })
  }

  async function handleSave(): Promise<void> {
    if (!profile) return
    const prefix = parseInt(tunPrefix, 10)
    if (Number.isNaN(prefix) || prefix < 1 || prefix > 32) {
      toast.error('子网前缀长度须在 1–32')
      return
    }
    if (!parseIPv4Octets(tunIPv4)) {
      toast.error('TUN IPv4 格式错误')
      return
    }
    const services = withTunIPv4(profile.services, tunIPv4, prefix)
    setSaving(true)
    try {
      const saved = await saveTunProfile({
        use: profile.use,
        tun_owner_key: profile.tun_owner_key || undefined,
        bind_interface: profile.bind_interface || undefined,
        cn_dns: cnDnsList.length > 0 ? cnDnsList : undefined,
        remote_dns: profile.remote_dns?.trim() || undefined,
        fakedns_domains:
          profile.fakedns_domains && profile.fakedns_domains.length > 0
            ? profile.fakedns_domains
            : undefined,
        bypass_rocket_server: profile.bypass_rocket_server,
        bypass_lan: profile.bypass_lan,
        bypass_loopback: profile.bypass_loopback,
        extra_bypass_hosts: extraHosts.length > 0 ? extraHosts : undefined,
        services,
        server_has_template: profile.server_has_template,
      })
      setProfile(saved)
      applyProfileToForm(saved, setTunIPv4, setTunPrefix)
      toast.success('已保存并应用')
    } catch (e) {
      toast.error(apiErrorMessage(e, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  if (loading || !profile) {
    return <p className="text-muted-foreground text-sm">加载中…</p>
  }

  const bypassRocket = boolDefault(profile.bypass_rocket_server, true)
  const bypassLAN = boolDefault(profile.bypass_lan, true)
  const bypassLoopback = boolDefault(profile.bypass_loopback, true)
  const ownerCandidates = profile.tun_owner_candidates ?? []
  const bindCandidates = profile.bind_interface_candidates ?? []

  return (
    <div className="mx-auto max-w-3xl space-y-6 pb-24">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">TUN 配置</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            运行开关、Helper 与分流偏好；公网先进 TUN，再由路由决定直连或代理。
          </p>
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" disabled={saving || loading || opsLoading} onClick={() => void load()}>
            刷新
          </Button>
          <Button type="button" disabled={saving} onClick={() => void handleSave()}>
            {saving ? '保存中…' : '保存并应用'}
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>运行控制</CardTitle>
          <CardDescription>
            {runtime?.tun_enabled ? '网卡已拉起' : '网卡未运行'}
            {runtime?.tun_degraded_reason ? ` · ${runtime.tun_degraded_reason}` : ''}
            {runtime?.tun_if_name ? ` · ${runtime.tun_if_name}` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {showHelper && !runtime?.tun_helper_installed ? (
            <Button type="button" variant="outline" disabled={opsLoading} onClick={() => void handleInstallHelper()}>
              安装 Helper
            </Button>
          ) : null}
          {showHelper && runtime?.tun_helper_installed ? (
            <Button type="button" variant="outline" disabled={opsLoading} onClick={() => void handleUninstallHelper()}>
              拆卸 Helper
            </Button>
          ) : null}
          <Button
            type="button"
            disabled={!runtime?.agent_ready || opsLoading}
            onClick={() => void handleTunToggle()}
          >
            {runtime?.tun_enabled ? '关闭 TUN' : '开启 TUN'}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>启用</CardTitle>
          <CardDescription>
            模板：{profile.server_has_template ? '服务端已下发 services.tun' : '未下发，可仅用本地覆盖'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <CheckRow
            checked={profile.use}
            label="启用 TUN"
            hint="本地优先于服务端模板；关闭后不再注入 TUN。"
            onChange={(use) => patch({ use })}
          />

          {profile.use ? (
            <>
              <Separator />
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="tun-ipv4">本机 TUN IPv4</Label>
                  <Input
                    id="tun-ipv4"
                    value={tunIPv4}
                    onChange={(e) => setTunIPv4(e.target.value)}
                    placeholder={DEFAULT_TUN_IPV4}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="tun-prefix">前缀</Label>
                  <Input
                    id="tun-prefix"
                    type="number"
                    min={1}
                    max={32}
                    value={tunPrefix}
                    onChange={(e) => setTunPrefix(e.target.value)}
                  />
                </div>
              </div>
              <p className="text-muted-foreground text-xs">
                默认 {DEFAULT_TUN_IPV4}/{DEFAULT_TUN_PREFIX}，须与用户态栈一致。
              </p>

              {ownerCandidates.length > 0 ? (
                <div className="space-y-2">
                  <Label htmlFor="tun-owner-key">承载实例</Label>
                  <Select
                    id="tun-owner-key"
                    value={profile.tun_owner_key ?? ''}
                    onChange={(e) => patch({ tun_owner_key: e.target.value || undefined })}
                  >
                    <option value="">自动（第一个可用 key）</option>
                    {ownerCandidates.map((key) => (
                      <option key={key} value={key}>
                        {key}
                      </option>
                    ))}
                  </Select>
                  <p className="text-muted-foreground text-xs">
                    全局仅一个 {tunDeviceName(osName)} 设备。
                  </p>
                </div>
              ) : null}

              {bindCandidates.length > 0 ? (
                <div className="space-y-2">
                  <Label htmlFor="bind-interface">出站绑网卡</Label>
                  <Select
                    id="bind-interface"
                    value={profile.bind_interface ?? ''}
                    onChange={(e) => patch({ bind_interface: e.target.value || undefined })}
                  >
                    <option value="">自动（默认路由网卡）</option>
                    {bindCandidates.map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                  </Select>
                  <p className="text-muted-foreground text-xs">
                    {profile.bind_interface_effective
                      ? `当前生效：${profile.bind_interface_effective}${!profile.bind_interface ? '（自动）' : ''}`
                      : '绑定物理网卡，避免经 TUN 回环。'}
                  </p>
                </div>
              ) : null}
            </>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>DNS</CardTitle>
          <CardDescription>空则用默认；国内可配多台，自上而下依次尝试。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="cn-dns">国内（geosite:cn）</Label>
            <div className="flex gap-2">
              <Input
                id="cn-dns"
                value={cnDnsInput}
                onChange={(e) => setCnDnsInput(e.target.value)}
                placeholder="223.5.5.5 或完整地址"
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    addCnDns()
                  }
                }}
              />
              <Button type="button" variant="outline" onClick={addCnDns}>
                添加
              </Button>
              <Button type="button" variant="outline" onClick={fillCnDnsDefaults}>
                填入默认
              </Button>
            </div>
            {cnDnsList.length > 0 ? (
              <ul className="flex flex-wrap gap-2 pt-1">
                {cnDnsList.map((addr, i) => (
                  <li
                    key={`${addr}-${i}`}
                    className="bg-muted flex items-center gap-1 rounded-md px-2 py-1 font-mono text-xs"
                  >
                    <span className="text-muted-foreground">{i + 1}.</span>
                    {addr}
                    <button
                      type="button"
                      className="text-muted-foreground hover:text-foreground ml-1"
                      aria-label={`移除 ${addr}`}
                      onClick={() => removeCnDns(addr)}
                    >
                      ×
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-muted-foreground text-xs">
                当前使用默认
                {(profile.cn_dns_default ?? []).length > 0
                  ? `：${(profile.cn_dns_default ?? []).join('、')}`
                  : ''}
              </p>
            )}
          </div>
          <div className="space-y-2 sm:max-w-sm">
            <Label htmlFor="remote-dns">境外（经代理）</Label>
            <Input
              id="remote-dns"
              value={profile.remote_dns ?? ''}
              onChange={(e) => patch({ remote_dns: e.target.value })}
              placeholder="8.8.8.8"
            />
            <p className="text-muted-foreground text-xs">
              默认 {profile.remote_dns_default ?? 'tcp://8.8.8.8:53'}
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>系统绕行</CardTitle>
          <CardDescription>
            不进 TUN 的地址；其余公网仍进 TUN，再由用户态路由分流。出站节点 IP 会自动绕行。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <CheckRow
            checked={bypassRocket}
            label="绕行 Rocket（Wire）"
            hint={
              rocketAddr
                ? `当前 ${rocketHostFromAddr(rocketAddr)}`
                : '未拿到 Rocket 地址'
            }
            onChange={(v) => patch({ bypass_rocket_server: v })}
          />
          <CheckRow
            checked={bypassLAN}
            label="绕行局域网"
            hint="10/8、172.16–31（除 TUN 段）、192.168/16、169.254/16"
            onChange={(v) => patch({ bypass_lan: v })}
          />
          <CheckRow
            checked={bypassLoopback}
            label="绕行本机回环"
            hint="127.0.0.0/8（用户态排除；勿写入破坏 lo0 的系统路由）"
            onChange={(v) => patch({ bypass_loopback: v })}
          />

          <Separator />

          <div className="space-y-2">
            <Label htmlFor="extra-bypass-host">额外绕行</Label>
            <div className="flex gap-2">
              <Input
                id="extra-bypass-host"
                value={extraHostInput}
                onChange={(e) => setExtraHostInput(e.target.value)}
                placeholder="IP / CIDR / 域名"
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    addExtraHost()
                  }
                }}
              />
              <Button type="button" variant="outline" onClick={addExtraHost}>
                添加
              </Button>
            </div>
            {extraHosts.length > 0 ? (
              <ul className="flex flex-wrap gap-2 pt-1">
                {extraHosts.map((host) => (
                  <li
                    key={host}
                    className="bg-muted flex items-center gap-1 rounded-md px-2 py-1 font-mono text-xs"
                  >
                    {host}
                    <button
                      type="button"
                      className="text-muted-foreground hover:text-foreground ml-1"
                      aria-label={`移除 ${host}`}
                      onClick={() => removeExtraHost(host)}
                    >
                      ×
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-muted-foreground text-xs">无额外项</p>
            )}
          </div>
        </CardContent>
      </Card>

      <div className="bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky bottom-0 z-10 -mx-1 border-t px-1 py-3 backdrop-blur">
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" disabled={saving} onClick={() => void load()}>
            放弃更改并刷新
          </Button>
          <Button type="button" disabled={saving} onClick={() => void handleSave()}>
            {saving ? '保存中…' : '保存并应用'}
          </Button>
        </div>
      </div>
    </div>
  )
}
