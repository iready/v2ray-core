import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  apiErrorMessage,
  fetchStatus,
  fetchTunProfile,
  saveTunProfile,
  type TunProfile,
} from '@/api/client'

const DEFAULT_TUN_IPV4 = '172.19.0.1'
const DEFAULT_TUN_PREFIX = 30

const defaultServicesExample = `{
  "name": "tun0",
  "mtu": 1500,
  "tag": "tun-in"
}`

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
  const out = { ...(services ?? {}) }
  out.ips = [{ ip: octets, prefix }]
  return out
}

export default function TunPage() {
  const [profile, setProfile] = useState<TunProfile | null>(null)
  const [servicesText, setServicesText] = useState('')
  const [tunIPv4, setTunIPv4] = useState(DEFAULT_TUN_IPV4)
  const [tunPrefix, setTunPrefix] = useState(String(DEFAULT_TUN_PREFIX))
  const [extraHostInput, setExtraHostInput] = useState('')
  const [rocketAddr, setRocketAddr] = useState<string>()
  const [osName, setOsName] = useState<string>()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [p, status] = await Promise.all([fetchTunProfile(), fetchStatus()])
      setProfile(p)
      setRocketAddr(status.addr)
      setOsName(status.os)
      setServicesText(
        p.services && Object.keys(p.services).length > 0
          ? JSON.stringify(p.services, null, 2)
          : '',
      )
      setTunIPv4(readTunIPv4(p.services))
      setTunPrefix(String(readTunPrefix(p.services)))
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

  const addExtraHost = () => {
    if (!profile) return
    const host = extraHostInput.trim()
    if (!host) return
    if (extraHosts.includes(host)) {
      setExtraHostInput('')
      return
    }
    setProfile({ ...profile, extra_bypass_hosts: [...extraHosts, host] })
    setExtraHostInput('')
  }

  const removeExtraHost = (host: string) => {
    if (!profile) return
    setProfile({
      ...profile,
      extra_bypass_hosts: extraHosts.filter((h) => h !== host),
    })
  }

  const handleSave = async () => {
    if (!profile) return
    let services: Record<string, unknown> | undefined
    const trimmed = servicesText.trim()
    if (trimmed) {
      try {
        services = JSON.parse(trimmed) as Record<string, unknown>
      } catch {
        toast.error('services JSON 格式错误')
        return
      }
    }
    const prefix = parseInt(tunPrefix, 10)
    if (Number.isNaN(prefix) || prefix < 1 || prefix > 32) {
      toast.error('子网前缀长度须在 1–32')
      return
    }
    if (!parseIPv4Octets(tunIPv4)) {
      toast.error('TUN IPv4 格式错误')
      return
    }
    services = withTunIPv4(services, tunIPv4, prefix)
    setSaving(true)
    try {
      const saved = await saveTunProfile({
        use: profile.use,
        tun_owner_key: profile.tun_owner_key || undefined,
        bind_interface: profile.bind_interface || undefined,
        bypass_rocket_server: profile.bypass_rocket_server,
        bypass_lan: profile.bypass_lan,
        bypass_loopback: profile.bypass_loopback,
        extra_bypass_hosts:
          profile.extra_bypass_hosts && profile.extra_bypass_hosts.length > 0
            ? profile.extra_bypass_hosts
            : undefined,
        services,
        server_has_template: profile.server_has_template,
      })
      setProfile(saved)
      setTunIPv4(readTunIPv4(saved.services))
      setTunPrefix(String(readTunPrefix(saved.services)))
      toast.success('已保存并应用 TUN 配置')
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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">TUN 配置</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          本地决定是否启用 TUN；IPv4 由本机决定（服务端不下发）。公网流量默认先进 TUN，再由 Rocket
          路由规则分流（直连或走代理）。
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>本地开关</CardTitle>
          <CardDescription>
            服务端模板：{profile.server_has_template ? '已下发 services.tun（不含 IP）' : '未下发（可仅使用本地覆盖）'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <label className="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              checked={profile.use}
              onChange={(e) => setProfile({ ...profile, use: e.target.checked })}
              className="size-4"
            />
            <span className="text-sm">启用 TUN（本地优先于服务端模板）</span>
          </label>
          {profile.use && (profile.tun_owner_candidates?.length ?? 0) > 0 && (
            <div className="space-y-2">
              <Label htmlFor="tun-owner-key">TUN 拥有者实例</Label>
              <select
                id="tun-owner-key"
                className="border-input bg-background w-full max-w-md rounded-md border px-3 py-2 text-sm"
                value={profile.tun_owner_key ?? ''}
                onChange={(e) =>
                  setProfile({ ...profile, tun_owner_key: e.target.value || undefined })
                }
              >
                <option value="">自动（第一个带 TUN 的 key）</option>
                {profile.tun_owner_candidates!.map((key) => (
                  <option key={key} value={key}>
                    {key}
                  </option>
                ))}
              </select>
              <p className="text-muted-foreground text-xs">
                全局仅一个 TUN 设备（
                {osName === 'darwin' ? 'utun' : osName === 'windows' ? 'wintun' : 'tun'}
                ）；请指定由哪个 server key 承载。
              </p>
            </div>
          )}
          {profile.use && (profile.bind_interface_candidates?.length ?? 0) > 0 && (
            <div className="space-y-2">
              <Label htmlFor="bind-interface">出站绑网卡</Label>
              <select
                id="bind-interface"
                className="border-input bg-background w-full max-w-md rounded-md border px-3 py-2 text-sm"
                value={profile.bind_interface ?? ''}
                onChange={(e) =>
                  setProfile({ ...profile, bind_interface: e.target.value || undefined })
                }
              >
                <option value="">自动（物理默认路由网卡）</option>
                {profile.bind_interface_candidates!.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
              {profile.bind_interface_effective ? (
                <p className="text-muted-foreground text-xs">
                  当前生效：{profile.bind_interface_effective}
                  {!profile.bind_interface ? '（自动）' : ''}
                </p>
              ) : (
                <p className="text-muted-foreground text-xs">
                  代理出站拨号将绑定物理网卡，避免经 TUN 回环。
                </p>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>本机 TUN IPv4</CardTitle>
          <CardDescription>
            默认 {DEFAULT_TUN_IPV4}/{DEFAULT_TUN_PREFIX}；须与 sing-tun 用户态栈绑定地址一致。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="tun-ipv4">IPv4 地址</Label>
            <Input
              id="tun-ipv4"
              value={tunIPv4}
              onChange={(e) => setTunIPv4(e.target.value)}
              placeholder={DEFAULT_TUN_IPV4}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="tun-prefix">前缀长度</Label>
            <Input
              id="tun-prefix"
              type="number"
              min={1}
              max={32}
              value={tunPrefix}
              onChange={(e) => setTunPrefix(e.target.value)}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>系统层绕行</CardTitle>
          <CardDescription>
            以下网段/地址在系统层不进 TUN（route_exclude）。其余公网流量仍会进入 TUN，再由路由规则在
            用户态判定直连或代理；另会自动绕行出站节点 IP 与 routing 中 tun-in 的 direct 规则目标。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <label className="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              checked={bypassRocket}
              onChange={(e) =>
                setProfile({ ...profile, bypass_rocket_server: e.target.checked })
              }
              className="size-4"
            />
            <span className="text-sm">系统层绕行 Rocket 服务器（Wire）</span>
          </label>
          <label className="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              checked={bypassLAN}
              onChange={(e) => setProfile({ ...profile, bypass_lan: e.target.checked })}
              className="size-4"
            />
            <span className="text-sm">系统层绕行局域网（10/8、172.16/12、192.168/16、169.254/16）</span>
          </label>
          <label className="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              checked={bypassLoopback}
              onChange={(e) =>
                setProfile({ ...profile, bypass_loopback: e.target.checked })
              }
              className="size-4"
            />
            <span className="text-sm">系统层绕行本机回环（127.0.0.0/8）</span>
          </label>

          <div className="space-y-1 text-sm">
            <span className="text-muted-foreground">当前 Rocket 地址</span>
            <p className="font-mono text-xs break-all">
              {rocketAddr ?? '—'}
              {rocketAddr ? (
                <span className="text-muted-foreground ml-2">
                  （主机 {rocketHostFromAddr(rocketAddr)}）
                </span>
              ) : null}
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="extra-bypass-host">额外系统层绕行</Label>
            <div className="flex gap-2">
              <Input
                id="extra-bypass-host"
                value={extraHostInput}
                onChange={(e) => setExtraHostInput(e.target.value)}
                placeholder="IP、CIDR 或域名，如 10.1.0.0/24"
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
              <p className="text-muted-foreground text-xs">无额外主机</p>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>高级</CardTitle>
          <CardDescription>
            services.tun 覆盖（mtu、tag 等）；有代理规则时 capture 路由由 Rocket 自动添加，一般无需手写
            routes。IP 请用上方面板配置。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <details className="group">
            <summary className="cursor-pointer text-sm font-medium select-none">
              展开 services.tun JSON
            </summary>
            <div className="mt-3 space-y-3">
              <textarea
                id="tun-services"
                className="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring min-h-[240px] w-full rounded-md border px-3 py-2 font-mono text-xs focus-visible:ring-2 focus-visible:outline-none"
                value={servicesText}
                onChange={(e) => setServicesText(e.target.value)}
                placeholder={defaultServicesExample}
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setServicesText(defaultServicesExample)}
              >
                填入示例
              </Button>
            </div>
          </details>
        </CardContent>
      </Card>

      <Button onClick={handleSave} disabled={saving}>
        {saving ? '保存中…' : '保存并应用'}
      </Button>
    </div>
  )
}
