import {Button} from '@/components/ui/button'
import {Input} from '@/components/ui/input'
import {Label} from '@/components/ui/label'

export function TunFields({value, onChange}: {value: unknown; onChange: (next: unknown) => void}) {
  if (value === undefined) return <Button size="sm" variant="outline" onClick={() => onChange({publish: false, name: 'tun0', tag: 'tun-in', mtu: 1500, proxy_outbound_tag: '', proxy_route_refs: []})}>添加 TUN 模板</Button>
  if (!value || typeof value !== 'object' || Array.isArray(value)) return <p className="text-sm text-destructive">TUN 配置不是对象；请在高级配置中修复。</p>
  const tun = value as Record<string, any>
  const patch = (values: Record<string, unknown>) => onChange({...tun, ...values})
  const refs = Array.isArray(tun.proxy_route_refs) ? tun.proxy_route_refs : []
  return <div className="space-y-3 rounded-lg border p-3">
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={!!tun.publish} onChange={event => patch({publish: event.target.checked})}/>发布 TUN 模板到客户端（这不会自动打开客户端本地 TUN）</label>
    <div className="grid gap-3 sm:grid-cols-3">
      <div><Label>网卡名称</Label><Input value={tun.name || ''} onChange={event => patch({name: event.target.value})}/></div>
      <div><Label>入站标签</Label><Input value={tun.tag || ''} onChange={event => patch({tag: event.target.value})}/></div>
      <div><Label>MTU</Label><Input type="number" value={tun.mtu ?? 1500} onChange={event => patch({mtu: Number(event.target.value) || 1500})}/></div>
      <div className="sm:col-span-3"><Label>代理出口 tag</Label><Input value={tun.proxy_outbound_tag || ''} onChange={event => patch({proxy_outbound_tag: event.target.value})}/></div>
    </div>
    <div className="space-y-2">
      <div className="flex items-center justify-between"><Label>代理名单路由库（只改本 Server 的引用，不改共享路由库内容）</Label><Button size="sm" variant="outline" onClick={() => patch({proxy_route_refs: [...refs, '']})}>添加引用</Button></div>
      {refs.map((ref: unknown, index: number) => <div key={index} className="flex gap-2"><Input value={typeof ref === 'string' ? ref : String((ref as any)?.name || (ref as any)?._id || '')} onChange={event => patch({proxy_route_refs: refs.map((item: unknown, i: number) => i === index ? event.target.value : item)})}/><Button size="sm" variant="ghost" onClick={() => patch({proxy_route_refs: refs.filter((_: unknown, i: number) => i !== index)})}>移除</Button></div>)}
      {!refs.length && <p className="text-xs text-muted-foreground">尚未关联代理名单路由库。</p>}
    </div>
  </div>
}
