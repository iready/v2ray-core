import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, Save, Server, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import {
  apiErrorMessage,
  fetchMyEditableServer,
  fetchMyEditableServers,
  saveMyEditableServer,
  type EditableServerDocument,
  type EditableServerSummary,
} from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select } from '@/components/ui/select'
import { TunFields } from './tun-fields'
import { EndpointFields } from './endpoint-fields'

const sections = [
  ['dns', 'DNS 解析'],
  ['routing', '路由（站点、出口、负载均衡）'],
  ['inbounds', '入站'],
  ['outbounds', '出站'],
  ['transport', '传输'],
  ['reverse', '穿透'],
  ['tun', '服务端 TUN 模板'],
  ['ext_link', '插件引用'],
  ['log', '日志配置'],
] as const

function formatSection(value: unknown): string {
  return value === undefined ? '' : JSON.stringify(value, null, 2)
}

const sectionKeys = new Set(sections.map(([key]) => key))
const lines = (value: string) => value.split('\n').map(item => item.trim()).filter(Boolean)

function DnsFields({value, onChange}: {value: unknown; onChange: (next: unknown) => void}) {
  if (value === undefined) return <Button size="sm" variant="outline" onClick={() => onChange({hosts: {}, servers: []})}>添加 DNS 配置</Button>
  if (typeof value === 'string') return <div className="space-y-2 rounded-lg border p-3"><p className="text-sm">DNS 配置库引用</p><Input value={value} onChange={e => onChange(e.target.value)} /><Button variant="outline" size="sm" onClick={() => onChange({hosts: {}, servers: []})}>改为内联 DNS</Button></div>
  const dns = value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, any> : {hosts: {}, servers: []}
  const hosts = dns.hosts && typeof dns.hosts === 'object' && !Array.isArray(dns.hosts) ? dns.hosts as Record<string, unknown> : {}
  const servers = Array.isArray(dns.servers) ? dns.servers : []
  const patch = (values: Record<string, unknown>) => onChange({...dns, ...values})
  const setHost = (key: string, text: string) => {
    const next = {...hosts}
    if (!text.trim()) delete next[key]
    else { const values = lines(text); next[key] = values.length > 1 ? values : text }
    patch({hosts: next})
  }
  const setServer = (index: number, next: unknown) => patch({servers: servers.map((item, i) => i === index ? next : item)})
  return <div className="space-y-4 rounded-lg border p-3">
    <div className="space-y-2"><div className="flex items-center justify-between"><Label>域名固定解析</Label><Button size="sm" variant="outline" onClick={() => patch({hosts: {...hosts, '': ''}})}>添加映射</Button></div>
      {Object.entries(hosts).map(([domain, answer], i) => <div key={`${domain}-${i}`} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]"><Input value={domain} placeholder="域名，如 dns.google" onChange={e => {const next={...hosts}; delete next[domain]; next[e.target.value]=answer; patch({hosts:next})}}/><Textarea value={Array.isArray(answer)?answer.join('\n'):String(answer ?? '')} placeholder="地址，如 8.8.8.8（多个地址每行一个）" onChange={e=>setHost(domain,e.target.value)}/><Button size="sm" variant="ghost" onClick={()=>{const next={...hosts};delete next[domain];patch({hosts:next})}}>删除</Button></div>)}
      {!Object.keys(hosts).length && <p className="text-muted-foreground text-xs">没有固定映射</p>}
    </div>
    <div className="space-y-2"><div className="flex items-center justify-between"><Label>DNS 服务器（按列表顺序）</Label><Button size="sm" variant="outline" onClick={()=>patch({servers:[...servers, '223.5.5.5']})}>添加 DNS</Button></div>
      {servers.map((server, i) => {
        const object = server && typeof server === 'object' && !Array.isArray(server) ? server as Record<string, any> : null
        const address = object ? String(object.address || '') : String(server || '')
        return <div key={i} className="space-y-2 rounded-md border p-3">
          <div className="flex gap-2"><Input list={`dns-providers-${i}`} value={address} placeholder="DNS 地址 / HTTPS DoH URL" onChange={e=>setServer(i, object ? {...object,address:e.target.value} : e.target.value)}/><datalist id={`dns-providers-${i}`}><option value="localhost"/><option value="223.5.5.5"/><option value="119.29.29.29"/><option value="114.114.114.114"/><option value="8.8.8.8"/><option value="https://1.1.1.1/dns-query"/><option value="https://dns.google/dns-query"/><option value="https://dns.alidns.com/dns-query"/><option value="https://doh.pub/dns-query"/></datalist><Button size="sm" variant="ghost" onClick={()=>patch({servers:servers.filter((_,n)=>n!==i)})}>移除</Button></div>
          {!object && <Button size="sm" variant="outline" onClick={()=>setServer(i,{address,domains:[],skipFallback:false})}>为此服务器添加域名条件</Button>}
          {object && <><Textarea value={Array.isArray(object.domains)?object.domains.join('\n'):''} placeholder="仅匹配这些域名时使用；留空表示不限定" onChange={e=>setServer(i,{...object,domains:lines(e.target.value)})}/><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={!!object.skipFallback} onChange={e=>setServer(i,{...object,skipFallback:e.target.checked})}/>仅用于匹配域名，不参与其他查询回退</label><details><summary className="text-muted-foreground cursor-pointer text-xs">结果校验 / 高级服务器参数</summary><Textarea className="mt-2" value={Array.isArray(object.expectIPs)?object.expectIPs.join('\n'):''} placeholder="expectIPs，可留空" onChange={e=>setServer(i,{...object,expectIPs:lines(e.target.value)})}/></details></>}
        </div>
      })}
    </div>
  </div>
}

function RoutingFields({value, outboundTags, onChange}: {value: unknown; outboundTags: string[]; onChange: (next: unknown) => void}) {
  if (value === undefined) return <Button size="sm" variant="outline" onClick={() => onChange({rules: []})}>添加路由配置</Button>
  const routing = value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, any> : {rules: []}
  const rules = Array.isArray(routing.rules) ? routing.rules : []
  const balancers = Array.isArray(routing.balancers) ? routing.balancers : []
  const patch = (values: Record<string, unknown>) => onChange({...routing,...values})
  const setRule = (index: number, next: unknown) => patch({rules: rules.map((rule,i)=>i===index?next:rule)})
  const setRuleList = (index: number, key: string, text: string) => {
    const next = {...rules[index]}
    const values = lines(text.replaceAll(',', '\n'))
    if (values.length) next[key] = values
    else delete next[key]
    setRule(index, next)
  }
  const setBalancer = (index: number, next: unknown) => patch({balancers: balancers.map((item,i)=>i===index?next:item)})
  return <div className="space-y-4 rounded-lg border p-3">
    <div className="grid gap-3 sm:grid-cols-2"><div><Label>域名解析策略</Label><Select value={routing.domainStrategy || ''} onChange={e=>patch({domainStrategy:e.target.value||undefined})}><option value="">AsIs（默认）</option><option>AsIs</option><option>IPIfNonMatch</option><option>IPOnDemand</option></Select></div><div><Label>域名匹配方式</Label><Select value={routing.domainMatcher || ''} onChange={e=>patch({domainMatcher:e.target.value||undefined})}><option value="">默认</option><option>hybrid</option><option>linear</option></Select></div></div>
    <div className="flex items-center justify-between"><Label>路由规则（从上往下，先匹配先生效）</Label><div className="flex gap-2"><Button size="sm" variant="outline" onClick={()=>patch({rules:[...rules,{type:'field'}]})}>添加规则</Button><Button size="sm" variant="ghost" onClick={()=>{if(window.confirm('清空全部路由规则？保存后生效。'))patch({rules:[]})}}>清空规则</Button></div></div>
    {rules.map((rule,i)=>{
      const targetKey=rule.balancerTag?'balancerTag':'outboundTag'; const target=String(rule[targetKey]||'')
      return <div key={i} className="space-y-2 rounded-md border p-3">
        <div className="flex items-center justify-between"><b className="text-sm">规则 {i+1}</b><Button size="sm" variant="ghost" onClick={()=>patch({rules:rules.filter((_,n)=>n!==i)})}>删除</Button></div>
        <div className="grid gap-2 sm:grid-cols-3"><div><Label>入口标签</Label><Input value={Array.isArray(rule.inboundTag)?rule.inboundTag.join(','):String(rule.inboundTag||'')} placeholder="如 tun-in" onChange={e=>setRuleList(i,'inboundTag',e.target.value)}/></div><div><Label>出口类型</Label><Select value={targetKey} onChange={e=>{const copy={...rule};delete copy.outboundTag;delete copy.balancerTag;copy[e.target.value]=target;setRule(i,copy)}}><option value="outboundTag">普通出站</option><option value="balancerTag">负载均衡</option></Select></div><div><Label>出口标签</Label><Input list={`outbound-tags-${i}`} value={target} placeholder="选择或输入 tag" onChange={e=>setRule(i,{...rule,[targetKey]:e.target.value})}/><datalist id={`outbound-tags-${i}`}>{[...outboundTags,...balancers.map(b=>String(b.tag||''))].filter(Boolean).map(tag=><option key={tag} value={tag}/>)}</datalist></div></div>
        <div className="grid gap-2 sm:grid-cols-2"><div><Label>域名条件（每行一项）</Label><Textarea value={Array.isArray(rule.domains)?rule.domains.join('\n'):''} onChange={e=>setRuleList(i,'domains',e.target.value)}/></div><div><Label>IP 条件（每行一项）</Label><Textarea value={Array.isArray(rule.ip)?rule.ip.join('\n'):''} onChange={e=>setRuleList(i,'ip',e.target.value)}/></div></div>
        <div className="grid gap-2 sm:grid-cols-3">
          <div><Label>网络</Label><Select value={rule.network || ''} onChange={e=>setRule(i,{...rule,network:e.target.value||undefined})}><option value="">不限</option><option value="tcp">TCP</option><option value="udp">UDP</option><option value="tcp,udp">TCP + UDP</option></Select></div>
          <div><Label>端口</Label><Input value={rule.port || ''} placeholder="如 443 或 80,443" onChange={e=>setRule(i,{...rule,port:e.target.value||undefined})}/></div>
          <div><Label>来源端口</Label><Input value={rule.sourcePort || ''} onChange={e=>setRule(i,{...rule,sourcePort:e.target.value||undefined})}/></div>
          <div><Label>来源 IP（每行一个）</Label><Textarea value={Array.isArray(rule.source)?rule.source.join('\n'):''} onChange={e=>setRuleList(i,'source',e.target.value)}/></div>
          <div><Label>协议（每行一个）</Label><Textarea value={Array.isArray(rule.protocol)?rule.protocol.join('\n'):''} onChange={e=>setRuleList(i,'protocol',e.target.value)}/></div>
          <div><Label>用户（每行一个）</Label><Textarea value={Array.isArray(rule.user)?rule.user.join('\n'):''} onChange={e=>setRuleList(i,'user',e.target.value)}/></div>
        </div>
      </div>
    })}
    <div className="flex items-center justify-between"><Label>负载均衡出口组</Label><Button size="sm" variant="outline" onClick={()=>patch({balancers:[...balancers,{tag:'',selector:[],strategy:{type:'random'}}]})}>添加负载均衡</Button></div>
    {balancers.map((balancer,i)=><div key={i} className="grid gap-2 rounded-md border p-3 sm:grid-cols-[1fr_1fr_1fr_auto]"><div><Label>组标签</Label><Input value={balancer.tag||''} onChange={e=>setBalancer(i,{...balancer,tag:e.target.value})}/></div><div><Label>候选出站 tag（每行一个）</Label><Textarea value={Array.isArray(balancer.selector)?balancer.selector.join('\n'):''} onChange={e=>setBalancer(i,{...balancer,selector:lines(e.target.value)})}/></div><div><Label>选择策略</Label><Select value={balancer.strategy?.type||'random'} onChange={e=>setBalancer(i,{...balancer,strategy:{...balancer.strategy,type:e.target.value}})}><option value="random">随机</option><option value="roundRobin">轮询</option><option value="leastPing">最低延迟</option><option value="leastLoad">最低负载</option></Select></div><Button size="sm" variant="ghost" onClick={()=>patch({balancers:balancers.filter((_,n)=>n!==i)})}>删除</Button></div>)}
  </div>
}

export default function MyServersPage() {
  const [rows, setRows] = useState<EditableServerSummary[]>([])
  const [owner, setOwner] = useState<{name: string; contact?: string; managed_by?: string} | null>(null)
  const [selected, setSelected] = useState<EditableServerDocument | null>(null)
  const [address, setAddress] = useState('')
  const [primaryOutboundTag, setPrimaryOutboundTag] = useState('')
  const [sectionsText, setSectionsText] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [polling, setPolling] = useState(false)
  const [syncError, setSyncError] = useState('')
  const [dirty, setDirty] = useState(false)

  const loadList = useCallback(async () => {
    setLoading(true)
    try {
      const result = await fetchMyEditableServers()
      setOwner(result.owner)
      setRows(result.rows || [])
    } catch (error) {
      toast.error(apiErrorMessage(error, '获取已授权配置失败'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void loadList() }, [loadList])

  async function openServer(row: EditableServerSummary): Promise<void> {
    if (dirty && !window.confirm('当前修改尚未保存，切换将丢弃这些修改。继续？')) return
    setLoading(true)
    try {
      const doc = await fetchMyEditableServer(row.id)
      setSelected(doc)
      setSyncError('')
      setAddress(doc.config.address == null ? doc.address : String(doc.config.address))
      setPrimaryOutboundTag(String(doc.config.primary_outbound_tag || ''))
      const other = Object.fromEntries(Object.entries(doc.config).filter(([key]) => !sectionKeys.has(key) && key !== 'address' && key !== 'primary_outbound_tag'))
      setSectionsText({...Object.fromEntries(sections.map(([key]) => [key, formatSection(doc.config[key])])), __other: JSON.stringify(other, null, 2)})
      setDirty(false)
    } catch (error) {
      toast.error(apiErrorMessage(error, '无法读取此 Server；可能已被管理员撤销授权'))
      await loadList()
    } finally {
      setLoading(false)
    }
  }

  function updateSection(key: string, value: string): void {
    setSectionsText(current => ({...current, [key]: value}))
    setDirty(true)
  }

  function sectionValue(key: string): unknown {
    const text = sectionsText[key]
    if (!text?.trim()) return undefined
    try { return JSON.parse(text) } catch { return null }
  }

  function updateStructuredSection(key: string, value: unknown): void {
    updateSection(key, JSON.stringify(value, null, 2))
  }

  const outboundTags = (selected?.outbound_tags || [])
  const selectedShared = rows.find(row => row.id === selected?.id)?.shared_token_count || 0

  async function save(): Promise<void> {
    if (!selected) return
    const config = {...selected.config}
    if (address.trim()) config.address = address.trim()
    else delete config.address
    try {
      for (const [key, text] of Object.entries(sectionsText)) {
        if (key === '__other') continue
        if (!text.trim()) delete config[key]
        else config[key] = JSON.parse(text)
      }
      const other = sectionsText.__other?.trim() ? JSON.parse(sectionsText.__other) : {}
      if (!other || typeof other !== 'object' || Array.isArray(other)) throw new Error('其他配置必须是 JSON 对象')
      for (const key of Object.keys(config)) if (!sectionKeys.has(key) && key !== 'address' && key !== 'primary_outbound_tag') delete config[key]
      Object.assign(config, other)
      if (primaryOutboundTag) config.primary_outbound_tag = primaryOutboundTag
      else delete config.primary_outbound_tag
    } catch (error) {
      toast.error(`JSON 格式错误：${error instanceof Error ? error.message : '请检查当前配置段'}`)
      return
    }

    setSaving(true)
    try {
      const result = await saveMyEditableServer(selected.id, selected.version, config)
      setSelected({...selected, address: String(config.address || ''), version: result.version, config})
      setDirty(false)
      if (result.sync_error) {
        setSyncError(result.sync_error)
        toast.error(result.sync_error)
        await loadList()
        return
      }
      setSyncError('')
      toast.success(`已保存到服务端 v${result.version}；${result.online_client_version == null ? '客户端离线，待下次连接拉取' : '正在等待客户端应用配置'}`)
      setPolling(true)
      const deadline = Date.now() + 30000
      while (Date.now() < deadline) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        const list = await fetchMyEditableServers()
        setRows(list.rows || [])
        const latest = list.rows?.find(item => item.id === selected.id)
        if (latest?.online && !latest.healthy) {
          toast.error(`配置已保存到服务端，但客户端报告异常：${latest.last_error || '未提供错误详情'}`)
          setPolling(false)
          return
        }
        if (latest?.synced) {
          toast.success(`客户端已应用配置 v${latest.token_version}`)
          setPolling(false)
          return
        }
      }
      setPolling(false)
      toast.message('配置已保存；暂未收到客户端已应用版本的确认，可稍后刷新状态')
    } catch (error) {
      toast.error(apiErrorMessage(error, '保存失败；服务端未确认写入'))
    } finally {
      setSaving(false)
      setPolling(false)
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">我的服务配置</h1>
          <p className="text-muted-foreground mt-1 text-sm">实际归属：{owner?.name || '未登记'}{owner?.contact ? ` · ${owner.contact}` : ''}{owner?.managed_by ? ` · 代配置管理员账号 ${owner.managed_by}` : ''}</p>
        </div>
        <Button variant="outline" onClick={() => void loadList()} disabled={loading}>
          <RefreshCw className="size-4" /> 刷新授权
        </Button>
      </div>
      {syncError && <Card className="border-destructive"><CardContent className="pt-4 text-destructive text-sm">{syncError}</CardContent></Card>}
      <Card className="border-amber-500/40">
        <CardContent className="pt-4 text-sm text-amber-200">
          本机页面按客户端 Token 授权，不单独验证操作者身份。持有同一归属人 Token 的客户端可以修改同一批 Server；请勿分享 Token。共享 Server 的修改会作用到所有下发了它的 Token。
        </CardContent>
      </Card>
      <div className="grid gap-5 xl:grid-cols-[300px_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base"><Server className="size-4" />归属人可管理的 Server</CardTitle>
            <CardDescription>当前 Token 的归属人被授权管理的 Server</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {!rows.length && <p className="text-muted-foreground text-sm">没有可管理的 Server。请联系管理员在归属人页分配。</p>}
            {rows.map(row => (
              <button key={row.id} onClick={() => void openServer(row)}
                className={`border-border hover:bg-accent w-full rounded-lg border p-3 text-left ${selected?.id === row.id ? 'bg-accent' : ''}`}>
                <div className="font-medium">{row.key}</div>
                <div className="text-muted-foreground truncate text-xs">{row.address}</div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <Badge variant={row.online ? 'success' : 'outline'}>{row.online ? '在线' : '离线'}</Badge>
                  <Badge variant={row.synced ? 'success' : 'outline'}>{row.synced ? `已应用 v${row.token_version}` : `服务端 v${row.token_version}`}</Badge>
                  {row.online && !row.healthy && <Badge variant="destructive">客户端报告异常</Badge>}
                  {row.shared_token_count > 0 && <Badge variant="outline">修改会影响其他 {row.shared_token_count} 个 Token</Badge>}
                </div>
              </button>
            ))}
          </CardContent>
        </Card>

        <Card>
          {!selected ? (
            <CardContent className="text-muted-foreground py-12 text-center">选择左侧已授权的 Server 开始编辑。</CardContent>
          ) : (
            <>
              <CardHeader className="border-border border-b">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <CardTitle className="text-base">{selected.key} · {selected.address}</CardTitle>
                    <CardDescription>基于服务端版本 v{selected.version} 编辑。保存采用版本校验，不会覆盖他人刚提交的修改。</CardDescription>
                    {selectedShared > 0 && <p className="mt-1 text-xs text-amber-200">修改会影响其他 {selectedShared} 个 Token。</p>}
                  </div>
                  <Button onClick={() => void save()} disabled={!dirty || saving || polling}>
                    <Save className="size-4" /> {polling ? '等待客户端应用' : saving ? '保存中' : '提交到服务端'}
                  </Button>
                </div>
              </CardHeader>
              <CardContent className="space-y-5 pt-5">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2"><Label htmlFor="server-address">备注地址</Label><Input id="server-address" value={address} onChange={event => {setAddress(event.target.value); setDirty(true)}} /></div>
                  <div className="space-y-2"><Label htmlFor="primary-outbound">主要出口（管理标记）</Label>
                    <Select id="primary-outbound" value={primaryOutboundTag} onChange={event => {setPrimaryOutboundTag(event.target.value); setDirty(true)}}>
                      <option value="">不指定</option>{outboundTags.map(tag => <option key={tag} value={tag}>{tag}</option>)}
                    </Select><p className="text-muted-foreground text-xs">主要出口标记不替代路由规则；实际流量仍按下方 routing 匹配。</p>
                  </div>
                </div>
                {sections.map(([key, label]) => {
                  const value = sectionValue(key)
                  const invalid = Boolean(sectionsText[key]?.trim()) && value === null
                  return <section key={key} className="space-y-2 rounded-lg border p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <Label htmlFor={`config-${key}`}>{label}</Label>
                      <div className="flex gap-2">
                        {key === 'inbounds' && <Button size="sm" variant="outline" onClick={() => updateSection(key, JSON.stringify([...(Array.isArray(value) ? value : []), {tag:'',protocol:'socks',listen:'127.0.0.1',port:1080,settings:{}}], null, 2))}>新增常用入站</Button>}
                        {key === 'outbounds' && <Button size="sm" variant="outline" onClick={() => updateSection(key, JSON.stringify([...(Array.isArray(value) ? value : []), {tag:'',protocol:'freedom',settings:{}}], null, 2))}>新增直连出站</Button>}
                        <Button size="sm" variant="ghost" onClick={() => updateSection(key, '')} disabled={!sectionsText[key]}><Trash2 className="size-3.5" /> 删除整段</Button>
                      </div>
                    </div>
                    {!invalid && key === 'dns' && <DnsFields value={value} onChange={next => updateStructuredSection(key, next)} />}
                    {!invalid && key === 'routing' && <RoutingFields value={value} outboundTags={outboundTags} onChange={next => updateStructuredSection(key, next)} />}
                    {!invalid && key === 'tun' && <TunFields value={value} onChange={next => updateStructuredSection(key, next)} />}
                    {!invalid && (key === 'inbounds' || key === 'outbounds') && <EndpointFields kind={key} value={value} onChange={next => updateStructuredSection(key, next)} />}
                    {invalid && <p className="text-sm text-destructive">当前 JSON 格式无效，请在高级编辑中修复后再保存。</p>}
                    <details>
                      <summary className="text-muted-foreground cursor-pointer text-xs">高级 JSON（完整字段）</summary>
                      <Textarea id={`config-${key}`} value={sectionsText[key] || ''} onChange={event => updateSection(key, event.target.value)}
                        placeholder={`${label} 未配置；留空表示移除该配置段`} className="mt-2 min-h-36 font-mono text-xs" spellCheck={false} />
                      <p className="text-muted-foreground mt-1 text-xs">删除整段请使用上方按钮；高级编辑可设置本界面未列出的业务字段。</p>
                    </details>
                  </section>
                })}
                <details className="border-border rounded-lg border p-3">
                  <summary className="cursor-pointer text-sm font-medium">其他完整配置字段</summary>
                  <p className="text-muted-foreground my-2 text-xs">只允许业务配置字段；Server 身份、归属、授权和远程指令不会提交。</p>
                  <Textarea value={sectionsText.__other || ''}
                    onChange={event => updateSection('__other', event.target.value)} className="min-h-48 font-mono text-xs" spellCheck={false} />
                </details>
              </CardContent>
            </>
          )}
        </Card>
      </div>
    </div>
  )
}
