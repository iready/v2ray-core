import { useCallback, useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import {
  apiErrorMessage,
  clearMitmFlows,
  disableMitm,
  enableMitm,
  exportMitmCABase64,
  fetchMitmFlow,
  fetchMitmFlows,
  fetchMitmProfile,
  importMitmCA,
  mitmCADownloadURL,
  resetMitmCA,
  saveMitmProfile,
  type MitmConnectEndpoint,
  type MitmFlowDetail,
  type MitmFlowSummary,
  type MitmMapRemoteRule,
  type MitmProfile,
} from '@/api/client'

type Tab = 'flows' | 'map' | 'settings'
type DetailPane = 'resp' | 'req' | 'headers'
type NoiseMode = 'all' | 'api' | 'errors'

const VISITED_KEY = 'rocket-mitm-visited-hosts'
const PINNED_KEY = 'rocket-mitm-pinned-hosts'
const PREFS_KEY = 'rocket-mitm-prefs'
const MAX_VISITED = 100

type VisitedHost = { host: string; hits: number; lastAt: number }
type MitmPrefs = { hideOptions: boolean; hideStatic: boolean; noise: NoiseMode }

const DEFAULT_PREFS: MitmPrefs = { hideOptions: true, hideStatic: true, noise: 'all' }

const STATIC_RE =
  /\.(js|css|map|png|jpe?g|gif|webp|svg|ico|woff2?|ttf|eot|mp4|webm|m3u8|mp3)(\?|$)/i

export default function MitmPage() {
  const [tab, setTab] = useState<Tab>('flows')
  const [profile, setProfile] = useState<MitmProfile | null>(null)
  const [addr, setAddr] = useState(':19080')
  const [upstream, setUpstream] = useState('')
  const [ignoreHosts, setIgnoreHosts] = useState('localhost, 127.0.0.1, ::1')
  const [mediaBypass, setMediaBypass] = useState(false)
  const [sslInsecure, setSslInsecure] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [toggling, setToggling] = useState(false)
  const [flows, setFlows] = useState<MitmFlowSummary[]>([])
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [detail, setDetail] = useState<MitmFlowDetail | null>(null)
  const [live, setLive] = useState(true)
  const [visitedHosts, setVisitedHosts] = useState<VisitedHost[]>(() => loadVisited())
  const [pinnedHosts, setPinnedHosts] = useState<string[]>(() => loadPinned())
  const [hostFilter, setHostFilter] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [prefs, setPrefs] = useState<MitmPrefs>(() => loadPrefs())
  const [detailPane, setDetailPane] = useState<DetailPane>('resp')
  const [caBase64, setCaBase64] = useState('')
  const [caImportText, setCaImportText] = useState('')
  const [caImportPassword, setCaImportPassword] = useState('')
  const [caBusy, setCaBusy] = useState(false)
  const [mapRules, setMapRules] = useState<MitmMapRemoteRule[]>([])

  const hostCounts = useMemo(() => {
    const m = new Map<string, number>()
    for (const f of flows) m.set(f.host, (m.get(f.host) || 0) + 1)
    return m
  }, [flows])

  const filteredFlows = useMemo(() => {
    const q = query.trim().toLowerCase()
    return flows.filter((f) => {
      if (hostFilter && f.host !== hostFilter) return false
      if (prefs.hideOptions && f.method === 'OPTIONS') return false
      if (prefs.hideStatic && isStaticAsset(f)) return false
      if (prefs.noise === 'errors' && !(f.status_code && f.status_code >= 400)) return false
      if (prefs.noise === 'api' && !looksLikeAPI(f)) return false
      if (!q) return true
      return (
        f.host.toLowerCase().includes(q) ||
        f.url.toLowerCase().includes(q) ||
        f.method.toLowerCase().includes(q) ||
        String(f.status_code || '').includes(q)
      )
    })
  }, [flows, hostFilter, query, prefs])

  const hostRail = useMemo(() => {
    const pinned = pinnedHosts.map((host) => ({
      host,
      pinned: true,
      hits: visitedHosts.find((v) => v.host === host)?.hits || 0,
      live: hostCounts.get(host) || 0,
      lastAt: visitedHosts.find((v) => v.host === host)?.lastAt || 0,
    }))
    const pinnedSet = new Set(pinnedHosts)
    const rest = visitedHosts
      .filter((v) => !pinnedSet.has(v.host))
      .map((v) => ({
        host: v.host,
        pinned: false,
        hits: v.hits,
        live: hostCounts.get(v.host) || 0,
        lastAt: v.lastAt,
      }))
    return [...pinned, ...rest]
  }, [pinnedHosts, visitedHosts, hostCounts])

  const applyLocal = (p: MitmProfile) => {
    setProfile(p)
    setAddr(p.addr || ':19080')
    setUpstream(p.upstream || '')
    setIgnoreHosts(stripMediaHosts((p.ignore_hosts || []).join(', ')))
    setMediaBypass(!!p.media_bypass)
    setSslInsecure(!!p.ssl_insecure)
    setMapRules(Array.isArray(p.map_remote) ? p.map_remote : [])
  }

  const loadProfile = useCallback(async () => {
    setLoading(true)
    try {
      applyLocal(await fetchMitmProfile())
    } catch (e) {
      toast.error(apiErrorMessage(e, '加载抓包配置失败'))
    } finally {
      setLoading(false)
    }
  }, [])

  const loadFlows = useCallback(async () => {
    try {
      setFlows(await fetchMitmFlows())
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    void loadProfile()
  }, [loadProfile])

  useEffect(() => {
    if (tab !== 'flows') return
    void loadFlows()
    if (!live) return
    const t = window.setInterval(() => void loadFlows(), 1000)
    return () => window.clearInterval(t)
  }, [tab, live, loadFlows])

  useEffect(() => {
    if (!selectedID) {
      setDetail(null)
      return
    }
    let cancelled = false
    void (async () => {
      try {
        const d = await fetchMitmFlow(selectedID)
        if (!cancelled) setDetail(d)
      } catch (e) {
        if (!cancelled) toast.error(apiErrorMessage(e, '加载详情失败'))
      }
    })()
    return () => {
      cancelled = true
    }
  }, [selectedID])

  useEffect(() => {
    savePrefs(prefs)
  }, [prefs])

  const primaryConnect = useMemo(() => {
    const list = profile?.connect || []
    return list.find((c) => !c.local) || list[0]
  }, [profile?.connect])

  const buildPayload = (): MitmProfile => ({
    use: !!profile?.use,
    addr: addr.trim() || ':19080',
    web_addr: '',
    upstream: upstream.trim(),
    ssl_insecure: sslInsecure,
    ignore_hosts: ignoreHosts
      .split(/[,，\n]/)
      .map((s) => s.trim())
      .filter(Boolean),
    media_bypass: mediaBypass,
    map_remote: mapRules,
  })

  const rememberHost = (host: string) => {
    const h = host.trim()
    if (!h) return
    setVisitedHosts((prev) => {
      const next = touchVisited(prev, h)
      saveVisited(next)
      return next
    })
  }

  const selectFlow = useCallback((f: MitmFlowSummary) => {
    setSelectedID(f.id)
    rememberHost(f.host)
  }, [])

  const copyText = async (text: string, okMsg: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(okMsg)
    } catch {
      toast.error('复制失败')
    }
  }

  const handleSave = async () => {
    if (!profile) return
    setSaving(true)
    try {
      applyLocal(await saveMitmProfile(buildPayload()))
      toast.success('已保存并应用')
    } catch (e) {
      toast.error(apiErrorMessage(e, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleToggle = async () => {
    if (!profile) return
    setToggling(true)
    try {
      if (profile.running || profile.use) {
        await disableMitm()
        toast.success('已关闭抓包')
      } else {
        await saveMitmProfile({ ...buildPayload(), use: true })
        await enableMitm()
        toast.success('已启用抓包')
      }
      await loadProfile()
      await loadFlows()
    } catch (e) {
      toast.error(apiErrorMessage(e, '切换失败'))
    } finally {
      setToggling(false)
    }
  }

  const toggleMediaBypass = async () => {
    const next = !mediaBypass
    setMediaBypass(next)
    try {
      applyLocal(
        await saveMitmProfile({
          ...buildPayload(),
          media_bypass: next,
          ignore_hosts: stripMediaHosts(ignoreHosts)
            .split(/[,，\n]/)
            .map((s) => s.trim())
            .filter(Boolean),
        }),
      )
      toast.success(next ? '媒体绕行已开（图片/视频 CDN 不解密）' : '媒体绕行已关')
    } catch (e) {
      setMediaBypass(!next)
      toast.error(apiErrorMessage(e, '切换失败'))
    }
  }

  const handleClear = async () => {
    try {
      await clearMitmFlows()
      setSelectedID(null)
      setDetail(null)
      await loadFlows()
    } catch (e) {
      toast.error(apiErrorMessage(e, '清空失败'))
    }
  }

  const togglePin = (host: string) => {
    setPinnedHosts((prev) => {
      const next = prev.includes(host) ? prev.filter((h) => h !== host) : [host, ...prev]
      savePinned(next)
      return next
    })
  }

  const clearVisitedHosts = () => {
    setVisitedHosts([])
    saveVisited([])
    setHostFilter(null)
  }

  const handleExportCA = async (kind: 'cert' | 'bundle') => {
    setCaBusy(true)
    try {
      const content = await exportMitmCABase64(kind)
      setCaBase64(content)
      toast.success(kind === 'bundle' ? '已导出 CA bundle（含私钥）' : '已导出 CA 证书 Base64')
    } catch (e) {
      toast.error(apiErrorMessage(e, '导出失败'))
    } finally {
      setCaBusy(false)
    }
  }

  const handleImportCA = async () => {
    if (!caImportText.trim()) {
      toast.error('请粘贴 Base64 或 PEM')
      return
    }
    const looksPEM = caImportText.includes('-----BEGIN')
    if (!looksPEM && !caImportPassword.trim()) {
      toast.error('P12 Base64 需要填写导出时的密码')
      return
    }
    setCaBusy(true)
    try {
      applyLocal(await importMitmCA(caImportText, caImportPassword))
      setCaImportText('')
      setCaImportPassword('')
      toast.success('CA 已导入')
    } catch (e) {
      toast.error(apiErrorMessage(e, '导入失败'))
    } finally {
      setCaBusy(false)
    }
  }

  const handleResetCA = async () => {
    if (!window.confirm('将删除当前 CA 并重新生成 RSA 内置 CA，需重新给设备安装证书。继续？')) {
      return
    }
    setCaBusy(true)
    try {
      applyLocal(await resetMitmCA())
      toast.success('已重置为内置 RSA CA')
    } catch (e) {
      toast.error(apiErrorMessage(e, '重置失败'))
    } finally {
      setCaBusy(false)
    }
  }

  if (loading || !profile) {
    return <p className="text-muted-foreground text-sm">加载中…</p>
  }

  const running = !!profile.running

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight">抓包台</h1>
          <span
            className={cn(
              'inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[11px]',
              running ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-muted text-muted-foreground',
            )}
          >
            <span className={cn('size-1.5 rounded-full', running ? 'bg-emerald-500 animate-pulse' : 'bg-muted-foreground')} />
            {running ? 'LISTEN' : 'OFF'}
            {profile.error ? ` · ERR` : ''}
          </span>
          {profile.error ? <span className="text-destructive max-w-md truncate text-xs">{profile.error}</span> : null}
        </div>
        <div className="flex gap-1 rounded-lg border p-0.5">
          <TabBtn active={tab === 'flows'} onClick={() => setTab('flows')}>
            工作台
          </TabBtn>
          <TabBtn active={tab === 'map'} onClick={() => setTab('map')}>
            Map Remote
            {mapRules.filter((r) => r.enabled).length > 0 ? (
              <span className="text-muted-foreground ml-1 font-mono text-[10px]">
                {mapRules.filter((r) => r.enabled).length}
              </span>
            ) : null}
          </TabBtn>
          <TabBtn active={tab === 'settings'} onClick={() => setTab('settings')}>
            连接 / CA
          </TabBtn>
        </div>
      </div>

      <div className="border-border bg-muted/20 flex flex-wrap items-center gap-2 rounded-md border px-2.5 py-1.5 font-mono text-xs">
        <span className="text-muted-foreground">proxy</span>
        {primaryConnect ? (
          <>
            <button
              type="button"
              className="hover:bg-muted rounded px-1.5 py-0.5"
              onClick={() => void copyText(primaryConnect.proxy, '已复制代理')}
              title="点击复制"
            >
              {primaryConnect.proxy}
            </button>
            {primaryConnect.interface ? (
              <span className="text-muted-foreground">{primaryConnect.interface}</span>
            ) : null}
          </>
        ) : (
          <span className="text-muted-foreground">无地址</span>
        )}
        <span className="text-muted-foreground ml-auto hidden sm:inline">点地址可复制</span>
      </div>

      {tab === 'flows' ? (
        <div className="flex min-h-0 flex-col gap-2" style={{ height: 'calc(100svh - 9.5rem)' }}>
          <div className="flex flex-wrap items-center gap-1.5">
            <Button onClick={() => void handleToggle()} disabled={toggling} size="sm">
              {running || profile.use ? '停' : '开'}
            </Button>
            <a
              className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
              href={mitmCADownloadURL()}
              download="rocket-mitm-ca.cer"
            >
              CA.cer
            </a>
            <Button variant="outline" size="sm" onClick={() => void handleClear()}>
              清空流
            </Button>
            <Chip active={live} onClick={() => setLive((v) => !v)}>
              {live ? '● LIVE' : '○ PAUSE'}
            </Chip>
            <Chip active={mediaBypass} onClick={() => void toggleMediaBypass()} title="图片/视频 CDN 隧道直通，不解密">
              媒体绕行
            </Chip>
            <div className="bg-background border-border flex min-w-[12rem] flex-1 items-center gap-1 rounded-md border px-2">
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="筛选 host / path / method / status"
                className="h-8 w-full bg-transparent text-xs outline-none"
              />
              {query ? (
                <button type="button" className="text-muted-foreground text-[11px]" onClick={() => setQuery('')}>
                  ×
                </button>
              ) : null}
            </div>
            <Chip active={prefs.hideOptions} onClick={() => setPrefs((p) => ({ ...p, hideOptions: !p.hideOptions }))}>
              −OPTIONS
            </Chip>
            <Chip active={prefs.hideStatic} onClick={() => setPrefs((p) => ({ ...p, hideStatic: !p.hideStatic }))}>
              −静态
            </Chip>
            <Chip active={prefs.noise === 'api'} onClick={() => setPrefs((p) => ({ ...p, noise: p.noise === 'api' ? 'all' : 'api' }))}>
              API
            </Chip>
            <Chip
              active={prefs.noise === 'errors'}
              onClick={() => setPrefs((p) => ({ ...p, noise: p.noise === 'errors' ? 'all' : 'errors' }))}
            >
              ≥400
            </Chip>
            <span className="text-muted-foreground ml-auto font-mono text-[11px]">
              {filteredFlows.length}/{flows.length}
            </span>
          </div>

          <div className="grid min-h-0 flex-1 gap-2 lg:grid-cols-[minmax(12rem,0.75fr)_minmax(0,1.25fr)_minmax(0,1.15fr)]">
            {/* Host rail */}
            <div className="border-border flex min-h-0 flex-col overflow-hidden rounded-md border">
              <div className="bg-muted/40 text-muted-foreground flex items-center justify-between border-b px-2 py-1 font-mono text-[11px]">
                <span>HOSTS</span>
                {visitedHosts.length > 0 ? (
                  <button type="button" className="hover:text-foreground" onClick={clearVisitedHosts}>
                    clr
                  </button>
                ) : null}
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto">
                <HostRow
                  label="*"
                  sub={`${flows.length} flows`}
                  active={hostFilter == null}
                  onClick={() => setHostFilter(null)}
                />
                {hostRail.length === 0 ? (
                  <p className="text-muted-foreground p-3 text-[11px] leading-relaxed">
                    点过的域名会钉在这里（localStorage）。可 ★ 置顶重点 API 域。
                  </p>
                ) : (
                  hostRail.map((h) => (
                    <HostRow
                      key={h.host}
                      label={h.host}
                      sub={`×${h.hits} · live ${h.live}`}
                      active={hostFilter === h.host}
                      pinned={h.pinned}
                      onClick={() => setHostFilter(h.host === hostFilter ? null : h.host)}
                      onPin={() => togglePin(h.host)}
                    />
                  ))
                )}
              </div>
            </div>

            {/* Live stream */}
            <div className="border-border flex min-h-0 flex-col overflow-hidden rounded-md border">
              <div className="bg-muted/40 text-muted-foreground flex items-center justify-between border-b px-2 py-1 font-mono text-[11px]">
                <span>STREAM {live ? '↻' : '∥'}</span>
                {hostFilter ? <span className="truncate pl-2">{hostFilter}</span> : null}
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto">
                {filteredFlows.length === 0 ? (
                  <p className="text-muted-foreground p-4 text-sm">
                    {flows.length === 0 ? '无流量 — 设 HTTP 代理并装 CA.cer' : '当前过滤无结果'}
                  </p>
                ) : (
                  <table className="w-full table-fixed text-left text-xs">
                    <tbody>
                      {filteredFlows.map((f) => (
                        <tr
                          key={f.id}
                          data-flow-id={f.id}
                          className={cn(
                            'hover:bg-muted/50 cursor-pointer border-b border-border/60',
                            selectedID === f.id && 'bg-accent/60',
                          )}
                          onClick={() => selectFlow(f)}
                        >
                          <td className={cn('w-11 px-1.5 py-1 font-mono tabular-nums', statusClass(f.status_code))}>
                            {f.status_code || '…'}
                          </td>
                          <td className="text-muted-foreground w-12 px-0.5 py-1 font-mono">{f.method.slice(0, 4)}</td>
                          <td className="max-w-0 truncate px-1.5 py-1" title={f.url}>
                            <span className="font-medium">{shortHost(f.host)}</span>
                            <span className="text-muted-foreground"> {pathOf(f.url)}</span>
                          </td>
                          <td className="text-muted-foreground w-12 px-1 py-1 text-right font-mono tabular-nums">
                            {fmtSize(f.resp_size)}
                          </td>
                          <td className="text-muted-foreground w-12 px-1.5 py-1 text-right font-mono tabular-nums">
                            {f.duration_ms != null ? `${f.duration_ms}` : ''}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </div>

            {/* Inspector */}
            <div className="border-border flex min-h-0 flex-col overflow-hidden rounded-md border">
              <div className="bg-muted/40 flex flex-wrap items-center gap-1 border-b px-2 py-1">
                <span className="text-muted-foreground mr-1 font-mono text-[11px]">INSPECT</span>
                {(['resp', 'req', 'headers'] as DetailPane[]).map((p) => (
                  <Chip key={p} active={detailPane === p} onClick={() => setDetailPane(p)}>
                    {p}
                  </Chip>
                ))}
                {detail ? (
                  <div className="ml-auto flex flex-wrap gap-1">
                    <MiniBtn onClick={() => void copyText(toCurl(detail), 'curl')}>curl</MiniBtn>
                    <MiniBtn onClick={() => void copyText(detail.url, 'url')}>url</MiniBtn>
                    {detail.resp_body ? (
                      <MiniBtn onClick={() => void copyText(prettyMaybe(detail.resp_body!), 'resp')}>body</MiniBtn>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto p-2">
                {!detail ? (
                  <p className="text-muted-foreground p-2 text-xs leading-relaxed">
                    选中 STREAM 一行查看详情。响应 gzip 已自动解压。
                  </p>
                ) : (
                  <div className="space-y-2 text-xs">
                    <div className="border-border rounded border px-2 py-1.5">
                      <div className="flex flex-wrap items-baseline gap-2 font-mono">
                        <span className={statusClass(detail.status_code)}>{detail.status_code || '—'}</span>
                        <span className="font-semibold">{detail.method}</span>
                        <span className="text-muted-foreground">{fmtSize(detail.resp_size)}</span>
                        <span className="text-muted-foreground">{detail.duration_ms}ms</span>
                      </div>
                      <div className="text-muted-foreground mt-1 break-all font-mono text-[11px]">{detail.url}</div>
                      {headerFirst(detail.req_headers, 'X-Rocket-Map-From') ? (
                        <div className="mt-1 break-all font-mono text-[11px] text-amber-700 dark:text-amber-400">
                          mapped from {headerFirst(detail.req_headers, 'X-Rocket-Map-From')}
                        </div>
                      ) : null}
                    </div>
                    {detailPane === 'resp' ? (
                      <CodeBlock
                        label={`response${detail.resp_body_truncated ? ' truncated' : ''}`}
                        text={prettyMaybe(detail.resp_body || '（空）')}
                      />
                    ) : null}
                    {detailPane === 'req' ? (
                      <CodeBlock
                        label={`request${detail.req_body_truncated ? ' truncated' : ''}`}
                        text={prettyMaybe(detail.req_body || '（空）')}
                      />
                    ) : null}
                    {detailPane === 'headers' ? (
                      <div className="space-y-2">
                        <CodeBlock label="req headers" text={formatHeaders(detail.req_headers)} />
                        <CodeBlock label="resp headers" text={formatHeaders(detail.resp_headers)} />
                      </div>
                    ) : null}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      ) : tab === 'map' ? (
        <MapRemotePanel
          rules={mapRules}
          setRules={setMapRules}
          saving={saving}
          onSave={() => void handleSave()}
          seedHost={hostFilter || detail?.host || ''}
        />
      ) : (
        <SettingsPanel
          profile={profile}
          addr={addr}
          setAddr={setAddr}
          upstream={upstream}
          setUpstream={setUpstream}
          ignoreHosts={ignoreHosts}
          setIgnoreHosts={setIgnoreHosts}
          mediaBypass={mediaBypass}
          onToggleMediaBypass={() => void toggleMediaBypass()}
          sslInsecure={sslInsecure}
          setSslInsecure={setSslInsecure}
          setProfile={setProfile}
          saving={saving}
          caBusy={caBusy}
          caBase64={caBase64}
          caImportText={caImportText}
          setCaImportText={setCaImportText}
          caImportPassword={caImportPassword}
          setCaImportPassword={setCaImportPassword}
          onSave={() => void handleSave()}
          onExportCA={(k) => void handleExportCA(k)}
          onImportCA={() => void handleImportCA()}
          onResetCA={() => void handleResetCA()}
          onCopy={(t, m) => void copyText(t, m)}
        />
      )}
    </div>
  )
}

function MapRemotePanel({
  rules,
  setRules,
  saving,
  onSave,
  seedHost,
}: {
  rules: MitmMapRemoteRule[]
  setRules: (r: MitmMapRemoteRule[]) => void
  saving: boolean
  onSave: () => void
  seedHost: string
}) {
  const [draft, setDraft] = useState<MitmMapRemoteRule>(() => emptyMapRule(seedHost))

  const update = (id: string | undefined, patch: Partial<MitmMapRemoteRule>) => {
    setRules(rules.map((r) => (r.id === id ? { ...r, ...patch } : r)))
  }

  const remove = (id: string | undefined) => {
    setRules(rules.filter((r) => r.id !== id))
  }

  const add = () => {
    if (!draft.from_host?.trim()) {
      toast.error('From Host 必填')
      return
    }
    if (!draft.to_host?.trim() && !draft.to_proto?.trim() && !draft.to_path && !draft.to_port) {
      toast.error('To 至少填一项（通常是 Host）')
      return
    }
    setRules([{ ...draft, id: draft.id || newMapID(), enabled: draft.enabled !== false }, ...rules])
    setDraft(emptyMapRule(seedHost))
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-sm font-medium">Map Remote</h2>
        <p className="text-muted-foreground mt-1 text-xs leading-relaxed">
          把原本打向 A 的请求改打到 B。Path 以 <code>*</code> 结尾匹配子路径；留空 Path 表示整个 Host。
          保存后会重启抓包引擎生效。请求头会带 <code>X-Rocket-Map-From</code>。
        </p>
      </div>

      <div className="border-border grid gap-3 rounded-lg border p-3 lg:grid-cols-2">
        <fieldset className="space-y-2">
          <legend className="text-muted-foreground mb-1 font-mono text-[11px] tracking-wider uppercase">Map From</legend>
          <MapFields
            proto={draft.from_proto || ''}
            host={draft.from_host || ''}
            port={draft.from_port || ''}
            path={draft.from_path || ''}
            query={draft.from_query || ''}
            onChange={(p) =>
              setDraft((d) => ({
                ...d,
                from_proto: p.proto,
                from_host: p.host,
                from_port: p.port,
                from_path: p.path,
                from_query: p.query,
              }))
            }
          />
        </fieldset>
        <fieldset className="space-y-2">
          <legend className="text-muted-foreground mb-1 font-mono text-[11px] tracking-wider uppercase">Map To</legend>
          <MapFields
            proto={draft.to_proto || ''}
            host={draft.to_host || ''}
            port={draft.to_port || ''}
            path={draft.to_path || ''}
            query={draft.to_query || ''}
            onChange={(p) =>
              setDraft((d) => ({
                ...d,
                to_proto: p.proto,
                to_host: p.host,
                to_port: p.port,
                to_path: p.path,
                to_query: p.query,
              }))
            }
          />
        </fieldset>
        <label className="flex items-center gap-2 text-xs lg:col-span-2">
          <input
            type="checkbox"
            checked={!!draft.preserve_host}
            onChange={(e) => setDraft((d) => ({ ...d, preserve_host: e.target.checked }))}
            className="size-3.5"
          />
          Preserve Host header（保留原 Host 头）
        </label>
        <div className="flex flex-wrap gap-2 lg:col-span-2">
          <Button size="sm" onClick={add}>
            加入规则
          </Button>
          <Button size="sm" variant="outline" disabled={saving} onClick={onSave}>
            {saving ? '保存中…' : '保存并应用'}
          </Button>
        </div>
      </div>

      <div className="border-border overflow-hidden rounded-lg border">
        <div className="bg-muted/40 text-muted-foreground border-b px-2 py-1 font-mono text-[11px]">
          RULES · {rules.length}
        </div>
        {rules.length === 0 ? (
          <p className="text-muted-foreground p-3 text-xs">暂无规则。例：From host=api.prod.com → To host=127.0.0.1 port=8080</p>
        ) : (
          <ul className="divide-border divide-y">
            {rules.map((r) => (
              <li key={r.id || `${r.from_host}-${r.to_host}`} className="flex flex-wrap items-start gap-2 px-2 py-2 text-xs">
                <label className="mt-0.5 flex items-center gap-1">
                  <input
                    type="checkbox"
                    checked={r.enabled}
                    onChange={(e) => update(r.id, { enabled: e.target.checked })}
                    className="size-3.5"
                  />
                </label>
                <div className="min-w-0 flex-1 font-mono">
                  <div className="truncate">
                    <span className="text-muted-foreground">from </span>
                    {fmtMapSide(r, 'from')}
                  </div>
                  <div className="truncate">
                    <span className="text-muted-foreground">to </span>
                    {fmtMapSide(r, 'to')}
                    {r.preserve_host ? <span className="text-muted-foreground"> · keep Host</span> : null}
                  </div>
                </div>
                <Button size="sm" variant="outline" onClick={() => remove(r.id)}>
                  删
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
      {rules.length > 0 ? (
        <Button size="sm" disabled={saving} onClick={onSave}>
          {saving ? '保存中…' : '保存并应用'}
        </Button>
      ) : null}
    </div>
  )
}

function MapFields({
  proto,
  host,
  port,
  path,
  query,
  onChange,
}: {
  proto: string
  host: string
  port: string
  path: string
  query: string
  onChange: (v: { proto: string; host: string; port: string; path: string; query: string }) => void
}) {
  const set = (k: 'proto' | 'host' | 'port' | 'path' | 'query', v: string) =>
    onChange({ proto, host, port, path, query, [k]: v })
  return (
    <div className="grid grid-cols-[4.5rem_1fr] items-center gap-x-2 gap-y-1.5">
      <span className="text-muted-foreground text-[11px]">Protocol</span>
      <Input value={proto} placeholder="http / https / 空=任意" onChange={(e) => set('proto', e.target.value)} className="h-8 font-mono text-xs" />
      <span className="text-muted-foreground text-[11px]">Host</span>
      <Input value={host} placeholder="api.example.com" onChange={(e) => set('host', e.target.value)} className="h-8 font-mono text-xs" />
      <span className="text-muted-foreground text-[11px]">Port</span>
      <Input value={port} placeholder="空=任意/默认" onChange={(e) => set('port', e.target.value)} className="h-8 font-mono text-xs" />
      <span className="text-muted-foreground text-[11px]">Path</span>
      <Input value={path} placeholder="/api/* 或留空" onChange={(e) => set('path', e.target.value)} className="h-8 font-mono text-xs" />
      <span className="text-muted-foreground text-[11px]">Query</span>
      <Input value={query} placeholder="a=1 或留空" onChange={(e) => set('query', e.target.value)} className="h-8 font-mono text-xs" />
    </div>
  )
}

function emptyMapRule(seedHost: string): MitmMapRemoteRule {
  return {
    id: newMapID(),
    enabled: true,
    from_host: seedHost,
    to_host: '',
    preserve_host: false,
  }
}

function newMapID() {
  return `mr_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`
}

function fmtMapSide(r: MitmMapRemoteRule, side: 'from' | 'to') {
  const proto = side === 'from' ? r.from_proto : r.to_proto
  const host = side === 'from' ? r.from_host : r.to_host
  const port = side === 'from' ? r.from_port : r.to_port
  const path = side === 'from' ? r.from_path : r.to_path
  const query = side === 'from' ? r.from_query : r.to_query
  let s = `${proto || '*' }://${host || '*'}`
  if (port) s += `:${port}`
  if (path) s += path.startsWith('/') ? path : `/${path}`
  if (query) s += query.startsWith('?') ? query : `?${query}`
  return s
}

function headerFirst(h: Record<string, string[]> | undefined, name: string) {
  if (!h) return ''
  const hit = Object.entries(h).find(([k]) => k.toLowerCase() === name.toLowerCase())
  return hit?.[1]?.[0] || ''
}

function SettingsPanel(props: {
  profile: MitmProfile
  addr: string
  setAddr: (v: string) => void
  upstream: string
  setUpstream: (v: string) => void
  ignoreHosts: string
  setIgnoreHosts: (v: string) => void
  mediaBypass: boolean
  onToggleMediaBypass: () => void
  sslInsecure: boolean
  setSslInsecure: (v: boolean) => void
  setProfile: (p: MitmProfile) => void
  saving: boolean
  caBusy: boolean
  caBase64: string
  caImportText: string
  setCaImportText: (v: string) => void
  caImportPassword: string
  setCaImportPassword: (v: string) => void
  onSave: () => void
  onExportCA: (k: 'cert' | 'bundle') => void
  onImportCA: () => void
  onResetCA: () => void
  onCopy: (t: string, m: string) => void
}) {
  const {
    profile,
    addr,
    setAddr,
    upstream,
    setUpstream,
    ignoreHosts,
    setIgnoreHosts,
    mediaBypass,
    onToggleMediaBypass,
    sslInsecure,
    setSslInsecure,
    setProfile,
    saving,
    caBusy,
    caBase64,
    caImportText,
    setCaImportText,
    caImportPassword,
    setCaImportPassword,
    onSave,
    onExportCA,
    onImportCA,
    onResetCA,
    onCopy,
  } = props

  return (
    <div className="space-y-5">
      <section className="space-y-2">
        <h2 className="text-sm font-medium">可连接地址</h2>
        <p className="text-muted-foreground text-xs">手机 HTTP 代理；iOS 用 DER .cer + 证书信任设置。</p>
        <ul className="space-y-1.5">
          {(profile.connect || []).map((c: MitmConnectEndpoint) => (
            <li
              key={c.proxy}
              className="border-border flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2"
            >
              <code className="font-mono text-sm">{c.proxy}</code>
              <span className="text-muted-foreground text-xs">{c.local ? '本机' : c.interface || '局域网'}</span>
              <Button size="sm" variant="outline" className="ml-auto" onClick={() => onCopy(c.proxy, '已复制')}>
                复制
              </Button>
            </li>
          ))}
        </ul>
        <a
          className={cn(buttonVariants({ variant: 'outline', size: 'sm' }), 'inline-flex')}
          href={mitmCADownloadURL()}
          download="rocket-mitm-ca.cer"
        >
          下载 CA 证书（.cer）
        </a>
      </section>

      <section className="space-y-2 border-t pt-4">
        <h2 className="text-sm font-medium">CA Base64 导入 / 导出</h2>
        <p className="text-muted-foreground text-xs">
          仅 <strong>RSA 根 CA</strong>。叶证书 / EC / 异常日期会被拒绝。
        </p>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" disabled={caBusy} onClick={() => onExportCA('cert')}>
            导出证书 Base64
          </Button>
          <Button size="sm" variant="outline" disabled={caBusy} onClick={() => onExportCA('bundle')}>
            导出 Bundle Base64
          </Button>
          {caBase64 ? (
            <Button size="sm" variant="outline" onClick={() => onCopy(caBase64, '已复制 Base64')}>
              复制 Base64
            </Button>
          ) : null}
        </div>
        {caBase64 ? (
          <textarea
            className="border-border bg-muted/30 min-h-24 w-full rounded-md border p-2 font-mono text-xs"
            readOnly
            value={caBase64}
          />
        ) : null}
        <Label htmlFor="mitm-ca-import">导入内容（P12 Base64 / PEM Bundle）</Label>
        <textarea
          id="mitm-ca-import"
          className="border-border bg-background min-h-28 w-full rounded-md border p-2 font-mono text-xs"
          placeholder="粘贴 SSL「导出 Base64」的 P12，或 PEM Bundle"
          value={caImportText}
          onChange={(e) => setCaImportText(e.target.value)}
        />
        <div className="space-y-1.5">
          <Label htmlFor="mitm-ca-pwd">P12 密码</Label>
          <Input
            id="mitm-ca-pwd"
            type="password"
            autoComplete="off"
            placeholder="SSL 导出 P12 时填写的密码；PEM 可留空"
            value={caImportPassword}
            onChange={(e) => setCaImportPassword(e.target.value)}
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" disabled={caBusy} onClick={onImportCA}>
            {caBusy ? '处理中…' : '导入 CA'}
          </Button>
          <Button size="sm" variant="outline" disabled={caBusy} onClick={onResetCA}>
            重置为内置 CA
          </Button>
        </div>
      </section>

      <section className="space-y-3 border-t pt-4">
        <h2 className="text-sm font-medium">参数</h2>
        <label className="flex cursor-pointer items-center gap-2">
          <input
            type="checkbox"
            checked={profile.use}
            onChange={(e) => setProfile({ ...profile, use: e.target.checked })}
            className="size-4"
          />
          <span className="text-sm">启动时自动启用</span>
        </label>
        <div className="space-y-1.5">
          <Label htmlFor="mitm-addr">监听</Label>
          <Input id="mitm-addr" value={addr} onChange={(e) => setAddr(e.target.value)} placeholder=":19080" />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="mitm-up">上游（可选）</Label>
          <Input
            id="mitm-up"
            value={upstream}
            onChange={(e) => setUpstream(e.target.value)}
            placeholder="socks5://127.0.0.1:1091"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="mitm-ignore">不解密 Host</Label>
          <Input id="mitm-ignore" value={ignoreHosts} onChange={(e) => setIgnoreHosts(e.target.value)} />
          <label className="flex cursor-pointer items-center gap-2 pt-1">
            <input
              type="checkbox"
              checked={mediaBypass}
              onChange={() => onToggleMediaBypass()}
              className="size-4"
            />
            <span className="text-sm">媒体绕行（图片/视频 CDN 不解密，隧道直通）</span>
          </label>
        </div>
        <label className="flex cursor-pointer items-center gap-2">
          <input
            type="checkbox"
            checked={sslInsecure}
            onChange={(e) => setSslInsecure(e.target.checked)}
            className="size-4"
          />
          <span className="text-sm">不校验上游 TLS</span>
        </label>
        <p className="text-muted-foreground font-mono text-xs">{profile.ca_cert_path}</p>
        <Button onClick={onSave} disabled={saving}>
          {saving ? '保存中…' : '保存并应用'}
        </Button>
      </section>
    </div>
  )
}

const MEDIA_BYPASS_HOSTS = [
  'qpic.cn',
  'qlogo.cn',
  'gtimg.cn',
  'idqqimg.com',
  'v.qq.com',
  'video.qq.com',
  'tc.qq.com',
  'wxs.qq.com',
  'servicewechat.com',
]

function stripMediaHosts(current: string) {
  const skip = new Set(MEDIA_BYPASS_HOSTS)
  return current
    .split(/[,，\n]/)
    .map((s) => s.trim())
    .filter((s) => s && !skip.has(s.toLowerCase()))
    .join(', ')
}

function TabBtn({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'rounded-md px-3 py-1 text-sm transition-colors',
        active ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:text-foreground',
      )}
    >
      {children}
    </button>
  )
}

function Chip({
  active,
  onClick,
  children,
  title,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
  title?: string
}) {
  return (
    <button
      type="button"
      title={title}
      onClick={onClick}
      className={cn(
        'rounded border px-1.5 py-0.5 font-mono text-[11px] transition-colors',
        active
          ? 'border-foreground/30 bg-accent text-accent-foreground'
          : 'border-border text-muted-foreground hover:text-foreground',
      )}
    >
      {children}
    </button>
  )
}

function MiniBtn({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="border-border hover:bg-muted rounded border px-1.5 py-0.5 font-mono text-[11px]"
    >
      {children}
    </button>
  )
}

function HostRow({
  label,
  sub,
  active,
  pinned,
  onClick,
  onPin,
}: {
  label: string
  sub: string
  active: boolean
  pinned?: boolean
  onClick: () => void
  onPin?: () => void
}) {
  return (
    <div
      className={cn(
        'hover:bg-muted/40 flex w-full items-start gap-1 border-b border-border/60 px-1.5 py-1.5',
        active && 'bg-accent/50',
      )}
    >
      {onPin ? (
        <button
          type="button"
          className={cn('mt-0.5 shrink-0 px-0.5 text-[11px]', pinned ? 'text-amber-600' : 'text-muted-foreground')}
          onClick={(e) => {
            e.stopPropagation()
            onPin()
          }}
          title={pinned ? '取消置顶' : '置顶'}
        >
          ★
        </button>
      ) : (
        <span className="mt-0.5 w-4 shrink-0" />
      )}
      <button type="button" className="min-w-0 flex-1 text-left" onClick={onClick}>
        <div className="truncate font-mono text-[11px] font-medium">{label}</div>
        <div className="text-muted-foreground text-[10px]">{sub}</div>
      </button>
    </div>
  )
}

function CodeBlock({ label, text }: { label: string; text: string }) {
  return (
    <div>
      <div className="text-muted-foreground mb-1 font-mono text-[10px] tracking-wider uppercase">{label}</div>
      <pre className="bg-muted/40 max-h-[min(52vh,28rem)] overflow-auto rounded-md p-2 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all">
        {text}
      </pre>
    </div>
  )
}

function pathOf(url: string) {
  try {
    const u = new URL(url)
    return u.pathname + u.search
  } catch {
    return url
  }
}

function shortHost(host: string) {
  return host.replace(/^www\./, '')
}

function statusClass(code?: number) {
  if (!code) return 'text-muted-foreground'
  if (code >= 500) return 'text-red-600 dark:text-red-400'
  if (code >= 400) return 'text-amber-600 dark:text-amber-400'
  if (code >= 300) return 'text-sky-600 dark:text-sky-400'
  return 'text-emerald-700 dark:text-emerald-400'
}

function fmtSize(n: number) {
  if (!n) return '0'
  if (n < 1024) return `${n}`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)}k`
  return `${(n / 1024 / 1024).toFixed(1)}M`
}

function isStaticAsset(f: MitmFlowSummary) {
  return STATIC_RE.test(pathOf(f.url))
}

function looksLikeAPI(f: MitmFlowSummary) {
  if (f.method === 'OPTIONS') return false
  if (isStaticAsset(f)) return false
  const p = pathOf(f.url).toLowerCase()
  return (
    p.includes('/api') ||
    p.includes('/renter') ||
    p.includes('/graphql') ||
    p.includes('.json') ||
    f.method !== 'GET' ||
    (f.status_code != null && f.status_code !== 304)
  )
}

function formatHeaders(h?: Record<string, string[]>) {
  if (!h) return '（无）'
  return Object.entries(h)
    .map(([k, v]) => `${k}: ${v.join(', ')}`)
    .join('\n')
}

function prettyMaybe(text: string) {
  const t = text.trim()
  if (!t || t.startsWith('[binary hex]')) return text
  try {
    return JSON.stringify(JSON.parse(t), null, 2)
  } catch {
    return text
  }
}

function toCurl(d: MitmFlowDetail): string {
  const lines = [`curl -i -X ${d.method} '${d.url.replace(/'/g, `'\\''`)}'`]
  const hdrs = d.req_headers || {}
  for (const [k, vals] of Object.entries(hdrs)) {
    const lk = k.toLowerCase()
    if (lk === 'host' || lk === 'content-length' || lk === 'accept-encoding') continue
    for (const v of vals) {
      lines.push(`  -H '${k}: ${v.replace(/'/g, `'\\''`)}'`)
    }
  }
  if (d.req_body && d.req_body !== '（空）' && !d.req_body.startsWith('[binary hex]')) {
    lines.push(`  --data-raw '${d.req_body.replace(/'/g, `'\\''`)}'`)
  }
  return lines.join(' \\\n')
}

function loadVisited(): VisitedHost[] {
  try {
    const raw = localStorage.getItem(VISITED_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as VisitedHost[]
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter((x) => x && typeof x.host === 'string' && x.host)
      .map((x) => ({
        host: x.host,
        hits: typeof x.hits === 'number' && x.hits > 0 ? x.hits : 1,
        lastAt: typeof x.lastAt === 'number' ? x.lastAt : Date.now(),
      }))
      .sort((a, b) => b.lastAt - a.lastAt)
      .slice(0, MAX_VISITED)
  } catch {
    return []
  }
}

function saveVisited(list: VisitedHost[]) {
  try {
    localStorage.setItem(VISITED_KEY, JSON.stringify(list.slice(0, MAX_VISITED)))
  } catch {
    /* ignore */
  }
}

function touchVisited(prev: VisitedHost[], host: string): VisitedHost[] {
  const now = Date.now()
  const rest = prev.filter((x) => x.host !== host)
  const old = prev.find((x) => x.host === host)
  return [{ host, hits: (old?.hits || 0) + 1, lastAt: now }, ...rest].slice(0, MAX_VISITED)
}

function loadPinned(): string[] {
  try {
    const raw = localStorage.getItem(PINNED_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as string[]
    return Array.isArray(parsed) ? parsed.filter((x) => typeof x === 'string' && x) : []
  } catch {
    return []
  }
}

function savePinned(list: string[]) {
  try {
    localStorage.setItem(PINNED_KEY, JSON.stringify(list.slice(0, 40)))
  } catch {
    /* ignore */
  }
}

function loadPrefs(): MitmPrefs {
  try {
    const raw = localStorage.getItem(PREFS_KEY)
    if (!raw) return { ...DEFAULT_PREFS }
    return { ...DEFAULT_PREFS, ...(JSON.parse(raw) as Partial<MitmPrefs>) }
  } catch {
    return { ...DEFAULT_PREFS }
  }
}

function savePrefs(p: MitmPrefs) {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(p))
  } catch {
    /* ignore */
  }
}
