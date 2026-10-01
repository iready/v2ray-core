import {useEffect, useState} from 'react'
import {Button} from '@/components/ui/button'
import {Input} from '@/components/ui/input'
import {Label} from '@/components/ui/label'

function JsonApply({value, onApply}: {value: unknown; onApply: (next: unknown) => void}) {
  const [text, setText] = useState(JSON.stringify(value, null, 2))
  const [error, setError] = useState('')
  useEffect(() => { setText(JSON.stringify(value, null, 2)); setError('') }, [value])
  return <details className="mt-2">
    <summary className="text-muted-foreground cursor-pointer text-xs">完整对象 JSON（协议高级字段）</summary>
    <textarea className="border-input bg-background mt-2 min-h-40 w-full rounded-md border p-2 font-mono text-xs"
      value={text} onChange={event => setText(event.target.value)} spellCheck={false} />
    {error && <p className="text-destructive mt-1 text-xs">{error}</p>}
    <Button size="sm" variant="outline" className="mt-2" onClick={() => {
      try { onApply(JSON.parse(text)); setError('') }
      catch (e) { setError(e instanceof Error ? e.message : 'JSON 格式错误') }
    }}>应用此对象</Button>
  </details>
}

export function EndpointFields({kind, value, onChange}: {kind: 'inbounds' | 'outbounds'; value: unknown; onChange: (next: unknown) => void}) {
  if (value !== undefined && !Array.isArray(value)) return <p className="text-destructive text-sm">{kind} 应为列表；请用高级 JSON 修复。</p>
  const rows: unknown[] = Array.isArray(value) ? value : []
  const label = kind === 'inbounds' ? '入站' : '出站'
  function patch(index: number, next: unknown) { onChange(rows.map((row, i) => i === index ? next : row)) }
  function add() {
    const item = kind === 'inbounds'
      ? {tag: '', protocol: 'socks', listen: '127.0.0.1', port: 1080, settings: {}}
      : {tag: '', protocol: 'freedom', settings: {}}
    onChange([...rows, item])
  }
  return <div className="space-y-3 rounded-lg border p-3">
    <div className="flex items-center justify-between"><Label>{label}列表（链接引用不可在此改共享节点本体）</Label><Button size="sm" variant="outline" onClick={add}>新增常用{label}</Button></div>
    {rows.map((raw, index) => {
      if (typeof raw === 'string') return <div key={index} className="flex items-center justify-between rounded-md border p-3 text-sm">
        <span>共享配置引用：<code>{raw}</code>（只读引用）</span>
        <Button size="sm" variant="ghost" onClick={() => onChange(rows.filter((_, i) => i !== index))}>移除引用</Button>
      </div>
      if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return <p key={index} className="text-destructive text-sm">第 {index + 1} 项不是配置对象，请用高级 JSON 修复。</p>
      const item = raw as Record<string, any>
      return <div key={index} className="rounded-md border p-3">
        <div className="mb-3 flex items-center justify-between"><b className="text-sm">{item.tag || `${label} ${index + 1}`} · {item.protocol || '未选协议'}</b><Button size="sm" variant="ghost" onClick={() => onChange(rows.filter((_, i) => i !== index))}>删除</Button></div>
        <div className="grid gap-3 sm:grid-cols-3">
          <div><Label>协议</Label><Input value={item.protocol || ''} placeholder={kind === 'inbounds' ? 'socks / http / dokodemo-door' : 'freedom / trojan / vmess'} onChange={event => patch(index, {...item, protocol: event.target.value})}/></div>
          <div><Label>标签 tag</Label><Input value={item.tag || ''} onChange={event => patch(index, {...item, tag: event.target.value})}/></div>
          {kind === 'inbounds' && <div><Label>监听端口</Label><Input type="number" value={item.port ?? ''} onChange={event => patch(index, {...item, port: event.target.value ? Number(event.target.value) : undefined})}/></div>}
          {kind === 'inbounds' && <div className="sm:col-span-3"><Label>监听地址</Label><Input value={item.listen || ''} onChange={event => patch(index, {...item, listen: event.target.value})}/></div>}
        </div>
        <JsonApply value={item} onApply={next => patch(index, next)} />
      </div>
    })}
    {!rows.length && <p className="text-muted-foreground text-xs">没有{label}。</p>}
  </div>
}
