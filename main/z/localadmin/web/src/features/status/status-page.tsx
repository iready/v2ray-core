import { useCallback, useEffect, useRef, useState } from 'react'
import { Activity, Copy, Link2, RefreshCw, Server, Shield, Stethoscope } from 'lucide-react'
import { toast } from 'sonner'
import {
  fetchStatus,
  reconnect,
  enableTun,
  disableTun,
  installTunHelper,
  uninstallTunHelper,
  runDiagnose,
  installAutostart,
  uninstallAutostart,
  relaunchAutostart,
  apiErrorMessage,
  type DiagnoseReport,
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

function formatDiagnoseLog(report: DiagnoseReport): string {
  const lines: string[] = [
    `$ rocket diagnose`,
    `# ${new Date().toLocaleString()}`,
    '',
  ]
  if (report.verdict) {
    lines.push(`VERDICT: ${report.verdict}`)
    if (report.cause) lines.push(`CAUSE:   ${report.cause}`)
    if (report.fix) lines.push(`FIX:     ${report.fix}`)
    lines.push('')
  }
  for (const ch of report.checks) {
    const tag = ch.level === 'ok' ? 'PASS' : ch.level === 'warn' ? 'WARN' : 'FAIL'
    const mark = ch.level === 'ok' ? '✓' : ch.level === 'warn' ? '!' : '✗'
    lines.push(`[${mark}] ${tag.padEnd(4)} ${ch.title}${ch.elapsed ? ` (${ch.elapsed})` : ''}`)
    lines.push(`      ${ch.detail}`)
    if (ch.hint) {
      lines.push(`      hint: ${ch.hint}`)
    }
    lines.push('')
  }
  lines.push('─'.repeat(48))
  lines.push(report.summary)
  lines.push(report.ok ? 'RESULT: OK' : 'RESULT: FAILED')
  return lines.join('\n')
}

export default function StatusPage() {
  const [status, setStatus] = useState<StatusSnapshot | null>(null)
  const [loading, setLoading] = useState(false)
  const [diagLoading, setDiagLoading] = useState(false)
  const [diag, setDiag] = useState<DiagnoseReport | null>(null)
  const [diagLog, setDiagLog] = useState('')
  const diagLogRef = useRef<HTMLPreElement>(null)

  const refresh = useCallback(async () => {
    try {
      const data = await fetchStatus()
      setStatus(data)
    } catch (e) {
      console.error(e)
    }
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, 5000)
    return () => clearInterval(id)
  }, [refresh])

  useEffect(() => {
    if (diagLogRef.current) {
      diagLogRef.current.scrollTop = diagLogRef.current.scrollHeight
    }
  }, [diagLog, diagLoading])

  const handleReconnect = async () => {
    setLoading(true)
    try {
      await reconnect()
      toast.success('已触发重连')
      await refresh()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '重连失败'
      toast.error(msg)
    } finally {
      setLoading(false)
    }
  }

  const handleDiagnose = async () => {
    setDiagLoading(true)
    setDiag(null)
    setDiagLog(
      [
        '$ rocket diagnose',
        `# ${new Date().toLocaleString()}`,
        '',
        'running checks…',
        '  · 管理员权限 / 控制面 / 实例 / TUN',
        '  · 绑网卡 / DNS / 国内·外网 HTTPS（TUN 下含真拉取与 1.1.1.1 TLS）',
        '',
      ].join('\n'),
    )
    try {
      const report = await runDiagnose()
      setDiag(report)
      setDiagLog(formatDiagnoseLog(report))
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, '排查失败')
      setDiagLog((prev) => `${prev}\n✗ ERROR ${msg}\n`)
      toast.error(msg)
    } finally {
      setDiagLoading(false)
    }
  }

  const handleCopyDiagLog = async () => {
    if (!diagLog) return
    try {
      await navigator.clipboard.writeText(diagLog)
      toast.success('已复制排查日志')
    } catch {
      toast.error('复制失败')
    }
  }

  const [tunLoading, setTunLoading] = useState(false)
  const [autostartLoading, setAutostartLoading] = useState(false)

  const handleInstallAutostart = async () => {
    setAutostartLoading(true)
    try {
      const r = await installAutostart()
      toast.success(r.message || '已开启提权自启')
      await refresh()
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, '开启失败'))
    } finally {
      setAutostartLoading(false)
    }
  }

  const handleUninstallAutostart = async () => {
    setAutostartLoading(true)
    try {
      const r = await uninstallAutostart()
      toast.success(r.message || '已关闭提权自启')
      await refresh()
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, '关闭失败'))
    } finally {
      setAutostartLoading(false)
    }
  }

  const handleRelaunchAutostart = async () => {
    setAutostartLoading(true)
    try {
      const r = await relaunchAutostart()
      toast.success(r.message || '正在提权重启')
      if (!r.elevated) {
        // 进程即将退出，稍后刷新会连上新实例
        setTimeout(() => {
          window.location.reload()
        }, 1200)
      } else {
        await refresh()
      }
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, '提权重启失败'))
      setAutostartLoading(false)
    }
  }

  const handleTunToggle = async () => {
    setTunLoading(true)
    try {
      if (status?.tun_enabled) {
        await disableTun()
        toast.success('TUN 已关闭')
      } else {
        await enableTun()
        toast.success('TUN 已开启')
      }
      await refresh()
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, 'TUN 操作失败'))
    } finally {
      setTunLoading(false)
    }
  }

  const handleInstallHelper = async () => {
    setTunLoading(true)
    try {
      await installTunHelper()
      toast.success('Helper 安装完成')
      await refresh()
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, '安装失败'))
    } finally {
      setTunLoading(false)
    }
  }

  const handleUninstallHelper = async () => {
    setTunLoading(true)
    try {
      await uninstallTunHelper()
      toast.success('Helper 已拆卸')
      await refresh()
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, '拆卸失败'))
    } finally {
      setTunLoading(false)
    }
  }

  if (!status) {
    return <p className="text-muted-foreground text-sm">加载中...</p>
  }

  const servers = status.servers ?? []
  const feat = status.features ?? {}
  const isWindows = status.os === 'windows'
  const isDarwin = status.os === 'darwin'
  const showElevatedAuth = !!feat.elevated_auth
  const showHelper = !!feat.privileged_helper
  const showLoginAutostart = !!feat.login_autostart

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">运行状态</h1>
        <p className="text-muted-foreground mt-1 text-sm">查看 Agent 连接与 v2ray 实例运行情况</p>
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
              v2ray 实例
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
          <CardDescription>各 v2ray 实例运行状态与配置版本</CardDescription>
        </CardHeader>
        <CardContent>
          {servers.length === 0 ? (
            <p className="text-muted-foreground text-sm">暂无运行中的 v2ray 实例</p>
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
                    {inst.last_heartbeat && (
                      <p className="text-muted-foreground text-xs">启动于 {inst.last_heartbeat}</p>
                    )}
                    {inst.last_error && (
                      <p className="text-destructive text-xs break-all">{inst.last_error}</p>
                    )}
                  </div>
                  <span className="text-muted-foreground text-xs">{statusLabel[inst.status] ?? inst.status}</span>
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
          <DetailRow label="本地后台" value={status.admin_port ? `http://127.0.0.1:${status.admin_port}` : undefined} />
          <DetailRow label="客户端标记" value={status.client_build} />
          <DetailRow label="可执行文件" value={status.exe_path} />
          <DetailRow label="系统" value={status.os} />
          <DetailRow label="Agent PID" value={status.agent_pid || undefined} />
          <DetailRow label="Token 版本" value={status.token_version} />
          <DetailRow label="连接时间" value={status.connected_at} />
          <DetailRow label="启动时间" value={status.started_at} />
          <DetailRow label="配置路径" value={status.config_path} />
          {status.last_error && (
            <>
              <Separator />
              <p className="text-destructive text-sm break-all">最近错误: {status.last_error}</p>
            </>
          )}
        </CardContent>
      </Card>

      {showElevatedAuth && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Shield className="size-4" />
              权限与自启
            </CardTitle>
            <CardDescription>
              Windows 需管理员权限才能开 TUN；开启后登录自动提权，也可立即提权重启
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <StatusBadge ok={!!status.elevated} okLabel="已提权" failLabel="未提权" />
              <StatusBadge
                ok={!!status.autostart_installed}
                okLabel="提权自启已开"
                failLabel="提权自启未开"
              />
            </div>
            <DetailRow label="当前权限" value={status.elevated ? '管理员' : '普通用户'} />
            <DetailRow
              label="登录自启"
              value={status.autostart_installed ? '已注册（最高权限）' : '未注册'}
            />
            <div className="flex flex-wrap gap-2">
              {!status.autostart_installed ? (
                <Button onClick={handleInstallAutostart} disabled={autostartLoading}>
                  开启提权自启
                </Button>
              ) : (
                <Button variant="outline" onClick={handleUninstallAutostart} disabled={autostartLoading}>
                  关闭提权自启
                </Button>
              )}
              {!status.elevated && (
                <Button
                  variant="secondary"
                  onClick={handleRelaunchAutostart}
                  disabled={autostartLoading || !status.autostart_installed}
                >
                  立即提权重启
                </Button>
              )}
            </div>
            {!status.elevated && !status.autostart_installed && (
              <p className="text-muted-foreground text-xs">
                点「开启提权自启」会弹出 UAC 确认；完成后可再点「立即提权重启」
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {showLoginAutostart && !showElevatedAuth && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Shield className="size-4" />
              登录自启
            </CardTitle>
            <CardDescription>注册 LaunchAgent，登录后自动拉起 rocket</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <StatusBadge
              ok={!!status.autostart_installed}
              okLabel="已开启登录自启"
              failLabel="未开启登录自启"
            />
            <div className="flex flex-wrap gap-2">
              {!status.autostart_installed ? (
                <Button onClick={handleInstallAutostart} disabled={autostartLoading}>
                  开启登录自启
                </Button>
              ) : (
                <Button variant="outline" onClick={handleUninstallAutostart} disabled={autostartLoading}>
                  关闭登录自启
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>TUN 模式</CardTitle>
          <CardDescription>
            {isDarwin
              ? '公网流量先进 TUN，再按路由规则分流；需先安装 Privileged Helper'
              : isWindows
                ? '公网流量先进 TUN（wintun），再按路由规则分流；需管理员权限'
                : '公网流量先进 TUN，再按路由规则分流'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <DetailRow label="运行状态" value={status.tun_enabled ? '网卡已拉起' : '未运行'} />
          <DetailRow label="本地开关" value={status.tun_local_use ? '已开启' : '已关闭'} />
          <DetailRow label="TUN 拥有者" value={status.tun_owner_key || undefined} />
          <DetailRow
            label="TUN 生效"
            value={
              status.tun_local_use
                ? status.tun_active
                  ? '是'
                  : status.tun_degraded_reason
                    ? `否（${status.tun_degraded_reason}）`
                    : '否'
                : undefined
            }
          />
          <DetailRow label="服务端模板" value={status.tun_in_config ? '含 services.tun' : '未下发'} />
          {showHelper && (
            <DetailRow label="Helper" value={status.tun_helper_installed ? '已安装' : '未安装'} />
          )}
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
          <div className="flex flex-wrap gap-2">
            {showHelper && !status.tun_helper_installed && (
              <Button variant="outline" onClick={handleInstallHelper} disabled={tunLoading}>
                安装 Helper
              </Button>
            )}
            {showHelper && status.tun_helper_installed && (
              <Button variant="outline" onClick={handleUninstallHelper} disabled={tunLoading}>
                拆卸 Helper
              </Button>
            )}
            <Button
              onClick={handleTunToggle}
              disabled={!status.agent_ready || tunLoading}
            >
              {status.tun_enabled ? '关闭 TUN' : '开启 TUN'}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>一键排查</CardTitle>
              <CardDescription>权限、TUN、DNS、连通性等完整检测日志</CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={handleDiagnose} disabled={diagLoading} variant="secondary">
                <Stethoscope className={diagLoading ? 'animate-pulse' : ''} />
                {diagLoading ? '排查中…' : '开始排查'}
              </Button>
              <Button
                variant="outline"
                onClick={handleCopyDiagLog}
                disabled={!diagLog || diagLoading}
              >
                <Copy />
                复制日志
              </Button>
              <Button onClick={handleReconnect} disabled={!status.agent_ready || loading}>
                <RefreshCw className={loading ? 'animate-spin' : ''} />
                立即重连
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {diag && (
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={diag.ok ? 'success' : 'destructive'}>
                  {diag.ok ? '全部通过' : '存在失败项'}
                </Badge>
                {diag.cause && (
                  <Badge variant="outline">{diag.cause}</Badge>
                )}
                <span className="text-muted-foreground text-xs">{diag.summary}</span>
              </div>
              {diag.verdict && (
                <div className="rounded-lg border border-amber-900/50 bg-amber-950/40 px-3 py-2 text-sm text-amber-100">
                  <div className="font-medium">根因：{diag.verdict}</div>
                  {diag.fix && (
                    <div className="mt-1 text-xs text-amber-200/80">建议：{diag.fix}</div>
                  )}
                </div>
              )}
            </div>
          )}
          <pre
            ref={diagLogRef}
            className="max-h-[28rem] overflow-auto rounded-lg border border-zinc-800 bg-zinc-950 p-4 font-mono text-xs leading-5 break-all whitespace-pre-wrap text-zinc-100"
          >
            {diagLog || '# 点击「开始排查」后，完整检测过程会输出在这里'}
          </pre>
        </CardContent>
      </Card>
    </div>
  )
}
