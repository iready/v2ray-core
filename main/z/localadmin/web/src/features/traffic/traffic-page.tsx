import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import {
  apiErrorMessage,
  fetchTraffic,
  startTraffic,
  stopTraffic,
  type TrafficItem,
  type TrafficView,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type Outcome = 'all' | 'ok' | 'fail' | 'pending'

const EMPTY: TrafficView = { active: false, total: 0, items: [] }

type Group = {
  key: string
  at: number
  dest: string
  tag: string
  ok: number
  fail: number
  pending: number
  errors: string[]
}

function outcomeOf(it: TrafficItem): Exclude<Outcome, 'all'> {
  if (it.pending) return 'pending'
  return it.ok ? 'ok' : 'fail'
}

function groupItems(items: TrafficItem[]): Group[] {
  const map = new Map<string, Group>()
  const order: string[] = []
  for (const it of items) {
    const key = `${it.tag}\n${it.dest}`
    let g = map.get(key)
    if (!g) {
      g = { key, at: it.at, dest: it.dest, tag: it.tag, ok: 0, fail: 0, pending: 0, errors: [] }
      map.set(key, g)
      order.push(key)
    }
    if (it.at > g.at) g.at = it.at
    g[outcomeOf(it)]++
    if (it.error && !g.errors.includes(it.error)) g.errors.push(it.error)
  }
  return order.map((key) => map.get(key)!)
}

function signature(v: TrafficView): string {
  let s = `${v.active}|${v.total}|`
  for (const it of v.items) s += `${it.id}|${it.pending}|${it.ok}|${it.dur_ms}|${it.error ?? ''};`
  return s
}

export default function TrafficPage() {
  const [view, setView] = useState<TrafficView>(EMPTY)
  const [query, setQuery] = useState('')
  const [outcome, setOutcome] = useState<Outcome>('all')
  const [busy, setBusy] = useState(false)
  const sigRef = useRef('')

  const load = useCallback(async () => {
    try {
      const next = await fetchTraffic()
      const sig = signature(next)
      if (sig === sigRef.current) return
      sigRef.current = sig
      setView(next)
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    void load()
    const t = window.setInterval(() => void load(), 1000)
    return () => window.clearInterval(t)
  }, [load])

  const groups = useMemo(() => groupItems(view.items), [view.items])

  const counts = useMemo(() => {
    const c = { all: groups.length, ok: 0, fail: 0, pending: 0 }
    for (const g of groups) {
      if (g.ok > 0) c.ok++
      if (g.fail > 0) c.fail++
      if (g.pending > 0) c.pending++
    }
    return c
  }, [groups])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    return groups.filter((g) => {
      if (outcome === 'ok' && g.ok === 0) return false
      if (outcome === 'fail' && g.fail === 0) return false
      if (outcome === 'pending' && g.pending === 0) return false
      if (!q) return true
      return (
        g.dest.toLowerCase().includes(q) ||
        g.tag.toLowerCase().includes(q) ||
        g.errors.some((e) => e.toLowerCase().includes(q))
      )
    })
  }, [groups, query, outcome])

  const toggle = async () => {
    if (!view.active && view.total > 0 && !window.confirm(`开始新录制会清空上一段的 ${view.total} 条记录，继续？`)) {
      return
    }
    setBusy(true)
    try {
      const next = view.active ? await stopTraffic() : await startTraffic()
      sigRef.current = signature(next)
      setView(next)
      toast.success(next.active ? '开始录制' : '已停止录制')
    } catch (e) {
      toast.error(apiErrorMessage(e, '录制操作失败'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">流量</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          相同目标、相同出站合并成一行，数字是次数。失败单独标出。
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <Button
          size="sm"
          variant={view.active ? 'destructive' : 'default'}
          disabled={busy}
          onClick={() => void toggle()}
        >
          {view.active ? `■ 停止（${view.total}）` : '● 录制'}
        </Button>
        {(
          [
            ['all', '全部'],
            ['ok', '成功'],
            ['fail', '失败'],
            ['pending', '进行中'],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            type="button"
            onClick={() => setOutcome(id)}
            className={cn(
              'rounded-md border px-2 py-1 text-xs',
              outcome === id ? 'bg-accent text-accent-foreground' : 'text-muted-foreground',
            )}
          >
            {label} {counts[id]}
          </button>
        ))}
        <div className="bg-background border-border flex min-w-[12rem] flex-1 items-center rounded-md border px-2">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索目标 / 出站 / 错误"
            className="h-8 w-full bg-transparent text-xs outline-none"
          />
        </div>
        <span className="text-muted-foreground font-mono text-[11px]">
          {rows.length}/{groups.length}
        </span>
      </div>
      <div className="border-border min-h-0 overflow-auto rounded-md border" style={{ maxHeight: 'calc(100svh - 12rem)' }}>
        {rows.length === 0 ? (
          <p className="text-muted-foreground p-4 text-sm">
            {view.items.length === 0 ? '未录制。点「录制」后，经过核心的连接会出现在这里。' : '当前过滤无结果'}
          </p>
        ) : (
          <table className="w-full table-fixed text-left text-xs">
            <thead className="bg-muted/40 text-muted-foreground sticky top-0">
              <tr>
                <th className="w-20 px-2 py-1 font-medium">时间</th>
                <th className="w-36 px-2 py-1 font-medium">数量</th>
                <th className="w-28 px-2 py-1 font-medium">出站</th>
                <th className="px-2 py-1 font-medium">目标</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((g) => (
                <tr key={g.key} className="border-border/60 border-b">
                  <td className="text-muted-foreground px-2 py-1 font-mono">{fmtTime(g.at)}</td>
                  <td className="px-2 py-1 font-mono tabular-nums">
                    {g.ok + g.pending > 0 ? g.ok + g.pending : null}
                    {g.fail > 0 ? <span className="text-destructive ml-2">失败 {g.fail}</span> : null}
                  </td>
                  <td className="truncate px-2 py-1 font-mono">{g.tag || '—'}</td>
                  <td className="truncate px-2 py-1 font-mono">
                    {g.dest || '—'}
                    {g.errors.map((e) => (
                      <div key={e} className="text-destructive truncate font-sans">
                        {e}
                      </div>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

function fmtTime(ms: number) {
  if (!ms) return ''
  const d = new Date(ms)
  return d.toLocaleTimeString()
}

