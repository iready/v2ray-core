import { useCallback, useEffect, useState } from 'react'
import { Activity, Link2, Power, RotateCw, Server } from 'lucide-react'
import { toast } from 'sonner'
import {
  apiErrorMessage,
  fetchStatus,
  probeOutboundReach,
  restartAgent,
  stopAgent,
  tunTrafficLabel,
  type OutboundReach,
  type OutboundSnapshot,
  type ProbeLeg,
  type ServerInstanceStatus,
  type StatusSnapshot,
} from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'

function StatusBadge({ ok, okLabel, failLabel }: { ok: boolean; okLabel: string; failLabel: string }) {
  return <Badge variant={ok ? 'success' : 'destructive'}>{ok ? okLabel : failLabel}</Badge>
}

function DetailRow({ label, value }: { label: string; value?: string | number }) {
  if (value === undefined || value === '') return null
  return (
    <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
      <span className="text-muted-foreground text-xs">{label}</span>
      <span className="text-sm break-all">{value}</span>
    </div>
  )
}

const statusLabel: Record<string, string> = {
  STARTING: '启动中',
  STARTED: '运行中',
  FAILED: '失败',
  READY: '已停止',
}

function instanceBadge(inst: ServerInstanceStatus) {
  if (inst.healthy) {
    return <Badge variant="success">健康</Badge>
  }
  switch (inst.status) {
    case 'FAILED':
      return <Badge variant="destructive">{statusLabel.FAILED}</Badge>
    case 'STARTING':
      return <Badge variant="warning">{statusLabel.STARTING}</Badge>
    case 'READY':
      return <Badge variant="muted">{statusLabel.READY}</Badge>
    default:
      return <Badge variant="outline">{statusLabel[inst.status] ?? inst.status}</Badge>
  }
}

function outboundEndpoint(ob: OutboundSnapshot): string | undefined {
  if (!ob.address) return undefined
  return ob.port ? `${ob.address}:${ob.port}` : ob.address
}

function outboundTransport(ob: OutboundSnapshot): string | undefined {
  const parts = [ob.network, ob.security].filter((s) => s && s !== 'none')
  return parts.length ? parts.join(' / ') : undefined
}

function reachKey(serverKey: string, tag: string): string {
  return `${serverKey}\0${tag}`
}

function protocolLabel(protocol: string): string {
  return (protocol || 'unknown').toUpperCase()
}

function groupOutbounds(list: OutboundSnapshot[]): { protocol: string; items: OutboundSnapshot[] }[] {
  const buckets = new Map<string, OutboundSnapshot[]>()
  for (const ob of list) {
    if (!ob.remote) continue
    const protocol = ob.protocol || 'unknown'
    const bucket = buckets.get(protocol)
    if (bucket) {
      bucket.push(ob)
    } else {
      buckets.set(protocol, [ob])
    }
  }
  return [...buckets.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([protocol, items]) => ({ protocol, items }))
}

function ProbeBadge({
  label,
  probing,
  leg,
}: {
  label: string
  probing: boolean
  leg?: ProbeLeg
}) {
  if (probing) {
    return <Badge variant="muted">{label} 探测中</Badge>
  }
  if (!leg) return null
  return (
    <Badge variant={leg.ok ? 'success' : 'destructive'}>
      {label} {leg.ok ? `${leg.latency_ms}ms` : '不通'}
    </Badge>
  )
}

function tunHijackValue(status: StatusSnapshot): string | undefined {
  if (!status.tun_local_use) return undefined
  if (status.tun_degraded_reason && !status.tun_enabled) {
    return `网卡未运行（${status.tun_degraded_reason}）`
  }
  return tunTrafficLabel(status)
}

async function waitUntilBack(): Promise<boolean> {
  for (let i = 0; i < 40; i++) {
    await new Promise((r) => setTimeout(r, 500))
    try {
      await fetchStatus()
      return true
    } catch {
      /* 进程退出期间接口会断 */
    }
  }
  return false
}

export default function StatusPage() {
  const [status, setStatus] = useState<StatusSnapshot | null>(null)
  const [restarting, setRestarting] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [reachByKey, setReachByKey] = useState<Record<string, OutboundReach>>({})
  const [probingKeys, setProbingKeys] = useState<Record<string, true>>({})

  const refresh = useCallback(async () => {
    try {
      setStatus(await fetchStatus())
    } catch (e) {
      console.error(e)
    }
  }, [])

  async function probeOne(ob: OutboundSnapshot): Promise<void> {
    if (!ob.remote) return
    const key = reachKey(ob.server_key, ob.tag)
    setProbingKeys((prev) => ({ ...prev, [key]: true }))
    try {
      const item = await probeOutboundReach(ob.server_key, ob.tag)
      setReachByKey((prev) => ({ ...prev, [key]: item }))
    } catch (e) {
      toast.error(apiErrorMessage(e, '出站探测失败'))
    } finally {
      setProbingKeys((prev) => {
        const next = { ...prev }
        delete next[key]
        return next
      })
    }
  }

  async function probeAll(list: OutboundSnapshot[]): Promise<void> {
    const remote = list.filter((o) => o.remote)
    for (let i = 0; i < remote.length; i += 4) {
      await Promise.all(remote.slice(i, i + 4).map((ob) => probeOne(ob)))
    }
  }

  async function handleRestart(): Promise<void> {
    setRestarting(true)
    try {
      await restartAgent()
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, '')
      if (msg && !/network|timeout|ECONN|Failed to fetch|Network Error/i.test(msg)) {
        toast.error(apiErrorMessage(e, '重启失败'))
        setRestarting(false)
        return
      }
    }
    toast.message('正在重启 Rocket…')
    const ok = await waitUntilBack()
    if (ok) {
      window.location.reload()
      return
    }
    toast.error('重启超时，请手动打开后台')
    setRestarting(false)
  }

  async function handleStop(): Promise<void> {
    if (!window.confirm('关闭并退出 Rocket？本机代理会断开。')) return
    setStopping(true)
    try {
      await stopAgent()
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, '')
      if (msg && !/network|timeout|ECONN|Failed to fetch|Network Error/i.test(msg)) {
        toast.error(apiErrorMessage(e, '停止失败'))
        setStopping(false)
        return
      }
    }
    toast.message('Rocket 已退出')
  }

  useEffect(() => {
    if (restarting || stopping) return
    void refresh()
    const id = setInterval(() => void refresh(), 5000)
    return () => clearInterval(id)
  }, [refresh, restarting, stopping])

  if (!status) {
    return <p className="text-muted-foreground text-sm">加载中...</p>
  }

  const servers = status.servers ?? []
  const outbounds = (status.outbounds ?? []).filter((o) => o.remote)
  const probingAny = outbounds.some((o) => probingKeys[reachKey(o.server_key, o.tag)])
  const feat = status.features ?? {}
  const showElevated = !!feat.elevated_auth
  const showHelper = !!feat.privileged_helper
  const showLoginAutostart = !!feat.login_autostart

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">运行状态</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            一个 Rocket 进程；下面多条是它拉起的核心实例。开关与安装请到「TUN」「连接配置」。
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="destructive"
            onClick={() => void handleStop()}
            disabled={restarting || stopping}
          >
            <Power />
            {stopping ? '正在退出…' : '停止'}
          </Button>
          <Button onClick={() => void handleRestart()} disabled={restarting || stopping}>
            <RotateCw className={restarting ? 'animate-spin' : ''} />
            {restarting ? '重启中…' : '重启 Rocket'}
          </Button>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-1.5">
              <Activity className="size-3.5" />
              配置就绪
            </CardDescription>
            <CardTitle className="text-lg">
              <StatusBadge ok={status.agent_ready} okLabel="已配置" failLabel="待配置" />
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-1.5">
              <Link2 className="size-3.5" />
              Rocket 连接
            </CardDescription>
            <CardTitle className="text-lg">
              <StatusBadge ok={status.wire_connected} okLabel="已连接" failLabel="未连接" />
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-1.5">
              <Server className="size-3.5" />
              核心实例
            </CardDescription>
            <CardTitle className="font-mono text-2xl">
              {status.healthy_count}/{status.server_count}
            </CardTitle>
          </CardHeader>
          <CardContent className="pt-0">
            <p className="text-muted-foreground text-xs">健康 / 总数</p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>实例健康</CardTitle>
          <CardDescription>各核心实例运行状态与配置版本</CardDescription>
        </CardHeader>
        <CardContent>
          {servers.length === 0 ? (
            <p className="text-muted-foreground text-sm">暂无运行中的核心实例</p>
          ) : (
            <div className="space-y-3">
              {servers.map((inst) => (
                <div
                  key={inst.key}
                  className="border-border bg-muted/20 flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-sm font-medium">{inst.key}</span>
                      {instanceBadge(inst)}
                      <Badge variant="outline">v{inst.version}</Badge>
                    </div>
                    {inst.last_heartbeat ? (
                      <p className="text-muted-foreground text-xs">启动于 {inst.last_heartbeat}</p>
                    ) : null}
                    {inst.last_error ? (
                      <p className="text-destructive text-xs break-all">{inst.last_error}</p>
                    ) : null}
                  </div>
                  <span className="text-muted-foreground text-xs">
                    {statusLabel[inst.status] ?? inst.status}
                  </span>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3 space-y-0">
          <div className="space-y-1.5">
            <CardTitle>出站</CardTitle>
          </div>
          <Button
            type="button"
            variant="outline"
            disabled={restarting || stopping || probingAny || outbounds.length === 0}
            onClick={() => void probeAll(outbounds)}
          >
            {probingAny ? '探测中…' : '测全部'}
          </Button>
        </CardHeader>
        <CardContent>
          {outbounds.length === 0 ? (
            <p className="text-muted-foreground text-sm">暂无远端出站（实例未拉起或配置未下发）</p>
          ) : (
            <div className="space-y-5">
              {groupOutbounds(outbounds).map((group) => (
                <section key={group.protocol} className="space-y-2">
                  <div className="flex items-center gap-2">
                    <h3 className="text-muted-foreground text-xs font-semibold tracking-wider uppercase">
                      {protocolLabel(group.protocol)}
                    </h3>
                    <span className="text-muted-foreground tabular-nums text-xs">{group.items.length}</span>
                    <div className="bg-border h-px min-w-4 flex-1" />
                  </div>
                  <div className="space-y-2">
                    {group.items.map((ob) => {
                      const key = reachKey(ob.server_key, ob.tag)
                      const reach = reachByKey[key]
                      const probing = !!probingKeys[key]
                      return (
                        <div
                          key={key}
                          className="border-border bg-muted/20 flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
                        >
                          <div className="min-w-0 space-y-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-mono text-sm font-medium">{ob.tag}</span>
                              {ob.server_key ? (
                                <span className="text-muted-foreground font-mono text-xs">{ob.server_key}</span>
                              ) : null}
                            </div>
                            <p className="text-muted-foreground text-xs break-all">
                              {[outboundEndpoint(ob), outboundTransport(ob)].filter(Boolean).join(' · ') ||
                                '无远端地址'}
                            </p>
                            {reach?.tcp && !reach.tcp.ok && reach.tcp.error ? (
                              <p className="text-destructive text-xs break-all">TCP：{reach.tcp.error}</p>
                            ) : null}
                            {reach?.protocol_cn && !reach.protocol_cn.ok && reach.protocol_cn.error ? (
                              <p className="text-destructive text-xs break-all">国内：{reach.protocol_cn.error}</p>
                            ) : null}
                            {reach?.protocol_foreign && !reach.protocol_foreign.ok && reach.protocol_foreign.error ? (
                              <p className="text-destructive text-xs break-all">境外：{reach.protocol_foreign.error}</p>
                            ) : null}
                          </div>
                          <div className="flex shrink-0 flex-wrap items-center gap-2">
                            <ProbeBadge label="TCP" probing={probing} leg={reach?.tcp} />
                            <ProbeBadge label="国内" probing={probing} leg={reach?.protocol_cn} />
                            <ProbeBadge label="境外" probing={probing} leg={reach?.protocol_foreign} />
                            <Button
                              type="button"
                              size="sm"
                              variant="outline"
                              disabled={probing || restarting || stopping}
                              onClick={() => void probeOne(ob)}
                            >
                              {probing ? '测…' : '测'}
                            </Button>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </section>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>详细信息</CardTitle>
          <CardDescription>本地 Agent 运行快照</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <DetailRow label="Rocket 地址" value={status.addr} />
          <DetailRow
            label="本地后台"
            value={status.admin_port ? `http://127.0.0.1:${status.admin_port}` : undefined}
          />
          <DetailRow label="客户端标记" value={status.client_build} />
          <DetailRow label="可执行文件" value={status.exe_path} />
          <DetailRow label="系统" value={status.os} />
          <DetailRow label="Agent PID" value={status.agent_pid || undefined} />
          <DetailRow label="Token 版本" value={status.token_version} />
          <DetailRow label="连接时间" value={status.connected_at} />
          <DetailRow label="启动时间" value={status.started_at} />
          <DetailRow label="配置路径" value={status.config_path} />
          {showElevated ? (
            <DetailRow label="权限" value={status.elevated ? '已提权' : '未提权'} />
          ) : null}
          {showElevated || showLoginAutostart ? (
            <DetailRow
              label="登录自启"
              value={status.autostart_installed ? '已注册' : '未注册'}
            />
          ) : null}
          {status.last_error ? (
            <>
              <Separator />
              <p className="text-destructive text-sm break-all">最近错误: {status.last_error}</p>
            </>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>TUN 快照</CardTitle>
          <CardDescription>运行态只读；开关与 Helper 在「TUN」页操作</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <DetailRow label="运行状态" value={status.tun_enabled ? '网卡已拉起' : '未运行'} />
          <DetailRow label="本地开关" value={status.tun_local_use ? '已开启' : '已关闭'} />
          <DetailRow label="TUN 拥有者" value={status.tun_owner_key || undefined} />
          <DetailRow label="流量劫持" value={tunHijackValue(status)} />
          <DetailRow label="服务端模板" value={status.tun_in_config ? '含 services.tun' : '未下发'} />
          {showHelper ? (
            <DetailRow label="Helper" value={status.tun_helper_installed ? '已安装' : '未安装'} />
          ) : null}
          <DetailRow label="接口" value={status.tun_if_name} />
          <DetailRow
            label="出站绑网卡"
            value={
              status.tun_bind_interface
                ? status.tun_bind_auto
                  ? `${status.tun_bind_interface}（自动）`
                  : status.tun_bind_interface
                : undefined
            }
          />
        </CardContent>
      </Card>
    </div>
  )
}
