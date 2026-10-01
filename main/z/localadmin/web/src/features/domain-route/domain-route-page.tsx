import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, Send } from 'lucide-react'
import { toast } from 'sonner'
import {
  apiErrorMessage,
  fetchDomainRouteRequests,
  flushDomainRouteRequests,
  submitDomainRouteRequest,
  type DomainRouteRequestItem,
} from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'

function statusBadge(status: string) {
  switch (status) {
    case 'pending':
      return <Badge variant="warning">待上报</Badge>
    case 'submitted':
      return <Badge variant="muted">待审批</Badge>
    case 'approved':
      return <Badge variant="success">已通过</Badge>
    case 'rejected':
      return <Badge variant="destructive">已驳回</Badge>
    default:
      return <Badge variant="muted">{status}</Badge>
  }
}

export default function DomainRoutePage() {
  const [text, setText] = useState('')
  const [remark, setRemark] = useState('')
  const [items, setItems] = useState<DomainRouteRequestItem[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setItems(await fetchDomainRouteRequests())
    } catch (e) {
      toast.error(apiErrorMessage(e, '加载失败'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
    const t = setInterval(() => void load(), 8000)
    return () => clearInterval(t)
  }, [load])

  const onSubmit = async () => {
    setSubmitting(true)
    try {
      await submitDomainRouteRequest(text, remark)
      setText('')
      setRemark('')
      toast.success('已提交（未连线时会本地排队）')
      await load()
    } catch (e) {
      toast.error(apiErrorMessage(e, '提交失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const onFlush = async () => {
    try {
      const res = await flushDomainRouteRequests()
      toast.success(`已同步 ${res.flushed} 条`)
      if (res.error) toast.message(res.error)
      setItems(res.items ?? [])
    } catch (e) {
      toast.error(apiErrorMessage(e, '同步失败'))
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>域名录入申请</CardTitle>
          <CardDescription>翻墙打不开的域名可在此申请；管理员审批后写入路由库（配置需另行发布才生效）。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="domains">域名（每行一个，可一次多个）</Label>
            <Textarea
              id="domains"
              rows={5}
              placeholder={'openai.com\nchatgpt.com'}
              value={text}
              onChange={(e) => setText(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="remark">备注（可选）</Label>
            <Input id="remark" value={remark} onChange={(e) => setRemark(e.target.value)} placeholder="例如：办公需要" />
          </div>
          <div className="flex gap-2">
            <Button onClick={() => void onSubmit()} disabled={submitting || !text.trim()}>
              <Send className="size-4" />
              提交申请
            </Button>
            <Button variant="outline" onClick={() => void onFlush()}>
              <RefreshCw className="size-4" />
              立即同步待上报
            </Button>
            <Button variant="ghost" onClick={() => void load()} disabled={loading}>
              刷新列表
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">本机申请记录</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {items.length === 0 ? (
            <p className="text-muted-foreground text-sm">暂无记录</p>
          ) : (
            items.map((it) => (
              <div key={it.client_req_id} className="border-border rounded-lg border p-3 space-y-1">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  {statusBadge(it.status)}
                  <span className="text-muted-foreground text-xs">{it.updated_at}</span>
                </div>
                <div className="text-sm break-all">{(it.domains || []).join(' · ')}</div>
                {it.remark ? <div className="text-muted-foreground text-xs">备注：{it.remark}</div> : null}
                {it.reject_reason ? <div className="text-destructive text-xs">驳回：{it.reject_reason}</div> : null}
                {it.route_names?.length ? (
                  <div className="text-muted-foreground text-xs">写入：{it.route_names.join(', ')}</div>
                ) : null}
                {it.last_error ? <div className="text-destructive text-xs">上报错误：{it.last_error}</div> : null}
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}
