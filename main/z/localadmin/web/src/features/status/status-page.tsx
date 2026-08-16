import { useCallback, useEffect, useState } from 'react'
import { Activity, Link2, RotateCw, Server } from 'lucide-react'
import { toast } from 'sonner'
import {
  apiErrorMessage,
  fetchStatus,
  restartAgent,
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

function tunEffectValue(status: StatusSnapshot): string | undefined {
  if (!status.tun_local_use) return undefined
  if (status.tun_active) return '是'
  if (status.tun_degraded_reason) return `否（${status.tun_degraded_reason}）`
  return '否'
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

  const refresh = useCallback(async () => {
    try {
      setStatus(await fetchStatus())
    } catch (e) {
      console.error(e)
    }
  }, [])

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

  useEffect(() => {
    if (restarting) return
    void refresh()
    const id = setInterval(() => void refresh(), 5000)
    return () => clearInterval(id)
  }, [refresh, restarting])

  if (!status) {
    return <p className="text-muted-foreground text-sm">加载中...</p>
  }

  const servers = status.servers ?? []
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
        <Button onClick={() => void handleRestart()} disabled={restarting}>
          <RotateCw className={restarting ? 'animate-spin' : ''} />
          {restarting ? '重启中…' : '重启 Rocket'}
        </Button>
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
          <DetailRow label="TUN 生效" value={tunEffectValue(status)} />
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
