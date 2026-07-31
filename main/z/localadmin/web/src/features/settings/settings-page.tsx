import { useEffect, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { fetchConfig, saveConfig, type AddressPreset, type AgentConfig } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'

const emptyPreset = (): AddressPreset => ({ name: '', url: '' })

export default function SettingsPage() {
  const [cfg, setCfg] = useState<AgentConfig | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    fetchConfig().then(setCfg).catch(console.error)
  }, [])

  if (!cfg) {
    return <p className="text-muted-foreground text-sm">加载中...</p>
  }

  const updatePreset = (index: number, field: keyof AddressPreset, value: string) => {
    const presets = [...cfg.presets]
    presets[index] = { ...presets[index], [field]: value }
    setCfg({ ...cfg, presets })
  }

  const addPreset = () => setCfg({ ...cfg, presets: [...cfg.presets, emptyPreset()] })

  const removePreset = (index: number) => {
    const presets = cfg.presets.filter((_, i) => i !== index)
    const active = cfg.active === cfg.presets[index]?.name ? '' : cfg.active
    setCfg({ ...cfg, presets, active })
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const saved = await saveConfig(cfg)
      setCfg(saved)
      toast.success('配置已保存')
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">连接配置</h1>
        <p className="text-muted-foreground mt-1 text-sm">配置 Rocket WebSocket 地址、Token 与 mTLS</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Rocket 地址预设</CardTitle>
          <CardDescription>可维护多个环境地址，并选择当前生效项</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {cfg.presets.map((p, i) => (
            <div key={i} className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Input
                placeholder="名称"
                value={p.name}
                onChange={(e) => updatePreset(i, 'name', e.target.value)}
                className="sm:w-32"
              />
              <Input
                placeholder="ws://host/rpc"
                value={p.url}
                onChange={(e) => updatePreset(i, 'url', e.target.value)}
                className="flex-1"
              />
              <Button type="button" variant="destructive" size="sm" onClick={() => removePreset(i)}>
                <Trash2 />
                删除
              </Button>
            </div>
          ))}
          <Button type="button" variant="outline" onClick={addPreset}>
            <Plus />
            添加预设
          </Button>

          <Separator />

          <div className="space-y-2">
            <Label htmlFor="active">当前地址</Label>
            <Select
              id="active"
              value={cfg.active}
              onChange={(e) => setCfg({ ...cfg, active: e.target.value })}
            >
              <option value="">选择预设</option>
              {cfg.presets
                .filter((p) => p.name)
                .map((p) => (
                  <option key={p.name} value={p.name}>
                    {p.name} ({p.url})
                  </option>
                ))}
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="token">Token</Label>
            <Input
              id="token"
              value={cfg.token}
              onChange={(e) => setCfg({ ...cfg, token: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="sign">Sign（可选）</Label>
            <Input
              id="sign"
              value={cfg.sign}
              onChange={(e) => setCfg({ ...cfg, sign: e.target.value })}
            />
          </div>

          <Separator />

          <div className="space-y-3">
            <Label>mTLS（仅 wss）</Label>
            <div className="flex flex-wrap gap-2">
              {(['off', 'embed', 'file'] as const).map((mode) => (
                <Button
                  key={mode}
                  type="button"
                  size="sm"
                  variant={cfg.tls.mode === mode ? 'default' : 'outline'}
                  onClick={() => setCfg({ ...cfg, tls: { ...cfg.tls, mode } })}
                >
                  {mode === 'off' ? '关闭' : mode === 'embed' ? '内嵌证书' : '文件'}
                </Button>
              ))}
            </div>
            {cfg.tls.mode === 'embed' && (
              <p className="text-muted-foreground text-xs">
                使用编译时打入二进制的根 CA 与 P12 客户端证书。
              </p>
            )}
            {cfg.tls.mode === 'file' && (
              <div className="space-y-3">
                <Input
                  placeholder="CA 文件路径（默认 ~/Downloads/顶级证书.pem）"
                  value={cfg.tls.ca_file || ''}
                  onChange={(e) => setCfg({ ...cfg, tls: { ...cfg.tls, ca_file: e.target.value } })}
                />
                <Input
                  placeholder="客户端证书 PEM"
                  value={cfg.tls.cert_file || ''}
                  onChange={(e) => setCfg({ ...cfg, tls: { ...cfg.tls, cert_file: e.target.value } })}
                />
                <Input
                  placeholder="客户端私钥"
                  value={cfg.tls.key_file || ''}
                  onChange={(e) => setCfg({ ...cfg, tls: { ...cfg.tls, key_file: e.target.value } })}
                />
                <Input
                  placeholder="P12 文件路径（与 PEM 二选一）"
                  value={cfg.tls.p12_file || ''}
                  onChange={(e) => setCfg({ ...cfg, tls: { ...cfg.tls, p12_file: e.target.value } })}
                />
                <Input
                  type="password"
                  placeholder="P12 密码"
                  value={cfg.tls.p12_pass || ''}
                  onChange={(e) => setCfg({ ...cfg, tls: { ...cfg.tls, p12_pass: e.target.value } })}
                />
                <Input
                  placeholder="Server Name（可选）"
                  value={cfg.tls.server_name || ''}
                  onChange={(e) => setCfg({ ...cfg, tls: { ...cfg.tls, server_name: e.target.value } })}
                />
              </div>
            )}
          </div>

          <Button onClick={handleSave} disabled={saving}>
            {saving ? '保存中...' : '保存配置'}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
