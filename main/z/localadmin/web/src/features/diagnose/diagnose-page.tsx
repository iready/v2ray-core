import { useEffect, useRef, useState } from 'react'
import { Copy, RefreshCw, Stethoscope } from 'lucide-react'
import { toast } from 'sonner'
import {
  apiErrorMessage,
  fetchStatus,
  reconnect,
  runDiagnose,
  type DiagnoseReport,
} from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

function formatDiagnoseLog(report: DiagnoseReport): string {
  const lines: string[] = [`$ rocket diagnose`, `# ${new Date().toLocaleString()}`, '']
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

export default function DiagnosePage() {
  const [agentReady, setAgentReady] = useState(false)
  const [diagLoading, setDiagLoading] = useState(false)
  const [reconnectLoading, setReconnectLoading] = useState(false)
  const [diag, setDiag] = useState<DiagnoseReport | null>(null)
  const [diagLog, setDiagLog] = useState('')
  const diagLogRef = useRef<HTMLPreElement>(null)

  useEffect(() => {
    fetchStatus()
      .then((s) => setAgentReady(s.agent_ready))
      .catch(() => setAgentReady(false))
  }, [])

  useEffect(() => {
    if (diagLogRef.current) {
      diagLogRef.current.scrollTop = diagLogRef.current.scrollHeight
    }
  }, [diagLog, diagLoading])

  async function handleDiagnose(): Promise<void> {
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

  async function handleCopyDiagLog(): Promise<void> {
    if (!diagLog) return
    try {
      await navigator.clipboard.writeText(diagLog)
      toast.success('已复制排查日志')
    } catch {
      toast.error('复制失败')
    }
  }

  async function handleReconnect(): Promise<void> {
    setReconnectLoading(true)
    try {
      await reconnect()
      toast.success('已触发重连')
      const s = await fetchStatus()
      setAgentReady(s.agent_ready)
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, '重连失败'))
    } finally {
      setReconnectLoading(false)
    }
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">排查</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            权限、TUN、DNS、连通性一键检测；完整过程输出在下方日志。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => void handleDiagnose()} disabled={diagLoading} variant="secondary">
            <Stethoscope className={diagLoading ? 'animate-pulse' : ''} />
            {diagLoading ? '排查中…' : '开始排查'}
          </Button>
          <Button
            variant="outline"
            onClick={() => void handleCopyDiagLog()}
            disabled={!diagLog || diagLoading}
          >
            <Copy />
            复制日志
          </Button>
          <Button
            onClick={() => void handleReconnect()}
            disabled={!agentReady || reconnectLoading}
          >
            <RefreshCw className={reconnectLoading ? 'animate-spin' : ''} />
            立即重连
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>检测结果</CardTitle>
          <CardDescription>与 CLI `rocket diagnose` 同一套检查</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {diag ? (
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={diag.ok ? 'success' : 'destructive'}>
                  {diag.ok ? '全部通过' : '存在失败项'}
                </Badge>
                {diag.cause ? <Badge variant="outline">{diag.cause}</Badge> : null}
                <span className="text-muted-foreground text-xs">{diag.summary}</span>
              </div>
              {diag.verdict ? (
                <div className="rounded-lg border border-amber-900/50 bg-amber-950/40 px-3 py-2 text-sm text-amber-100">
                  <div className="font-medium">根因：{diag.verdict}</div>
                  {diag.fix ? (
                    <div className="mt-1 text-xs text-amber-200/80">建议：{diag.fix}</div>
                  ) : null}
                </div>
              ) : null}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">尚未运行排查</p>
          )}
          <pre
            ref={diagLogRef}
            className="max-h-[36rem] overflow-auto rounded-lg border border-zinc-800 bg-zinc-950 p-4 font-mono text-xs leading-5 break-all whitespace-pre-wrap text-zinc-100"
          >
            {diagLog || '# 点击「开始排查」后，完整检测过程会输出在这里'}
          </pre>
        </CardContent>
      </Card>
    </div>
  )
}
