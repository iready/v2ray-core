import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import {
  apiErrorMessage,
  fetchLogProfile,
  saveLogProfile,
  type LogProfile,
} from '@/api/client'

const LOG_LEVELS = ['warning', 'info', 'error', 'debug', 'none'] as const

export default function LogPage() {
  const [profile, setProfile] = useState<LogProfile | null>(null)
  const [loglevel, setLoglevel] = useState('warning')
  const [access, setAccess] = useState('')
  const [errorLog, setErrorLog] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const p = await fetchLogProfile()
      setProfile(p)
      setLoglevel(p.loglevel ?? 'warning')
      setAccess(p.access ?? '')
      setErrorLog(p.error ?? '')
    } catch (e) {
      toast.error(apiErrorMessage(e, '加载日志配置失败'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const handleSave = async () => {
    if (!profile) return
    setSaving(true)
    try {
      const payload: LogProfile = {
        use: profile.use,
        server_loglevel: profile.server_loglevel,
      }
      if (profile.use) {
        payload.loglevel = loglevel
        const accessTrim = access.trim()
        const errorTrim = errorLog.trim()
        if (accessTrim) payload.access = accessTrim
        if (errorTrim) payload.error = errorTrim
      }
      const saved = await saveLogProfile(payload)
      setProfile(saved)
      setLoglevel(saved.loglevel ?? 'warning')
      setAccess(saved.access ?? '')
      setErrorLog(saved.error ?? '')
      toast.success('已保存并应用日志配置')
    } catch (e) {
      toast.error(apiErrorMessage(e, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  if (loading || !profile) {
    return <p className="text-muted-foreground text-sm">加载中…</p>
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">v2ray 日志</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          本地覆盖优先于 Rocket 服务端下发的 log 配置；仅影响 v2ray 实例，不影响 rocket 进程自身日志。
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>本地覆盖</CardTitle>
          <CardDescription>
            服务端当前 loglevel：
            <span className="font-mono ml-1">{profile.server_loglevel || '—'}</span>
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <label className="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              checked={profile.use}
              onChange={(e) => setProfile({ ...profile, use: e.target.checked })}
              className="size-4"
            />
            <span className="text-sm">启用本地日志覆盖（优先于服务端）</span>
          </label>

          {profile.use ? (
            <>
              <div className="space-y-2">
                <Label htmlFor="loglevel">日志级别</Label>
                <Select
                  id="loglevel"
                  value={loglevel}
                  onChange={(e) => setLoglevel(e.target.value)}
                >
                  {LOG_LEVELS.map((lvl) => (
                    <option key={lvl} value={lvl}>
                      {lvl}
                    </option>
                  ))}
                </Select>
                <p className="text-muted-foreground text-xs">
                  推荐 warning，可减少 TUN 下 UDP dispatch 等 debug 刷屏。
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="access-log">访问日志 access</Label>
                <Input
                  id="access-log"
                  value={access}
                  onChange={(e) => setAccess(e.target.value)}
                  placeholder="留空不覆盖；none 关闭"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="error-log">错误日志 error</Label>
                <Input
                  id="error-log"
                  value={errorLog}
                  onChange={(e) => setErrorLog(e.target.value)}
                  placeholder="留空不覆盖；none 关闭"
                />
              </div>
            </>
          ) : (
            <p className="text-muted-foreground text-sm">未启用时完全使用服务端 log 配置。</p>
          )}
        </CardContent>
      </Card>

      <Button onClick={handleSave} disabled={saving}>
        {saving ? '保存中…' : '保存并应用'}
      </Button>
    </div>
  )
}
