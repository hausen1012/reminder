// AI 设置卡片：厂商、模型、Base URL 与 API Key。
//
// 密钥由后端加密存储，读回时恒为 "***"；用户不动输入框就原样回传占位符，
// 后端据此保留原密钥（与通知通道的敏感字段是同一套约定）。
import { useEffect, useState, type FormEvent } from 'react'
import { Loader2 } from 'lucide-react'
import {
  extractApiError,
  getAISettings,
  testAIConnection,
  updateAISettings,
} from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import type { AIProvider } from '@/types'

const PROVIDER_LABEL: Record<AIProvider, string> = {
  anthropic: 'Anthropic（Claude）',
  openai: 'OpenAI 兼容协议（DeepSeek / Moonshot / Ollama 等）',
}

// 常见厂商的默认地址与模型，选中后自动填入，仍可手工修改
const PRESETS: Record<string, { base_url: string; model: string }> = {
  anthropic: { base_url: '', model: 'claude-opus-5-5' },
  deepseek: { base_url: 'https://api.deepseek.com', model: 'deepseek-chat' },
  moonshot: { base_url: 'https://api.moonshot.cn', model: 'moonshot-v1-8k' },
  openai: { base_url: 'https://api.openai.com', model: 'gpt-4o-mini' },
  ollama: { base_url: 'http://localhost:11434', model: 'qwen2.5' },
}

export function AISettingsCard() {
  const { toast } = useToast()
  const [enabled, setEnabled] = useState(false)
  const [provider, setProvider] = useState<AIProvider>('anthropic')
  const [baseURL, setBaseURL] = useState('')
  const [model, setModel] = useState('claude-opus-5-5')
  const [apiKey, setApiKey] = useState('')
  const [hasKey, setHasKey] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)

  useEffect(() => {
    getAISettings()
      .then((s) => {
        if (!s) return
        setEnabled(Boolean(s.enabled))
        setProvider(s.provider === 'openai' ? 'openai' : 'anthropic')
        setBaseURL(s.base_url ?? '')
        setModel(s.model ?? '')
        setHasKey(s.api_key === '***')
        setApiKey('') // 输入框留空表示"不修改"；只有 hasKey 时才有原值
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  function toInput() {
    // 输入框留空时回传占位符，让后端保留原密钥
    const key = apiKey.trim() === '' && hasKey ? '***' : apiKey.trim()
    return { enabled, provider, base_url: baseURL.trim(), model: model.trim(), api_key: key }
  }

  async function handleSave(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      const saved = await updateAISettings(toInput())
      setHasKey(saved.api_key === '***')
      setApiKey('')
      toast({ title: 'AI 设置已保存' })
    } catch (err) {
      toast({ title: '保存失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  async function handleTest() {
    setTesting(true)
    try {
      const res = await testAIConnection(toInput())
      toast({ title: '连接成功', description: `模型回复：${res?.reply ?? ''}` })
    } catch (err) {
      toast({ title: '连接失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setTesting(false)
    }
  }

  function applyPreset(name: string) {
    const p = PRESETS[name]
    if (!p) return
    setBaseURL(p.base_url)
    setModel(p.model)
    setProvider(name === 'anthropic' ? 'anthropic' : 'openai')
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>AI 助手</CardTitle>
        <CardDescription>
          开启后可用自然语言对话来创建通知通道与提醒；助手生成的写操作需要你确认后才生效
        </CardDescription>
      </CardHeader>
      <CardContent>
        {loading ? (
          <p className="text-sm text-muted-foreground">加载中…</p>
        ) : (
          <form onSubmit={handleSave} className="max-w-sm space-y-5">
            <div className="flex items-center justify-between">
              <Label htmlFor="ai-enabled">启用 AI 助手</Label>
              <Switch id="ai-enabled" checked={enabled} onCheckedChange={setEnabled} />
            </div>

            <div className="space-y-2">
              <Label>快速填充</Label>
              <Select onValueChange={applyPreset}>
                <SelectTrigger>
                  <SelectValue placeholder="选择常见厂商…" />
                </SelectTrigger>
                <SelectContent>
                  {Object.keys(PRESETS).map((k) => (
                    <SelectItem key={k} value={k}>
                      {k === 'anthropic'
                        ? 'Anthropic'
                        : k === 'openai'
                          ? 'OpenAI'
                          : k === 'deepseek'
                            ? 'DeepSeek'
                            : k === 'moonshot'
                              ? 'Moonshot'
                              : 'Ollama（本地）'}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">仅用于填入默认值与模型名，之后仍可修改</p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="ai-provider">厂商协议</Label>
              <Select value={provider} onValueChange={(v) => setProvider(v as AIProvider)}>
                <SelectTrigger id="ai-provider">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(['anthropic', 'openai'] as AIProvider[]).map((p) => (
                    <SelectItem key={p} value={p}>
                      {PROVIDER_LABEL[p]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="ai-base-url">Base URL（可选）</Label>
              <Input
                id="ai-base-url"
                placeholder={provider === 'anthropic' ? '留空使用官方地址' : '例如 https://api.deepseek.com'}
                value={baseURL}
                onChange={(e) => setBaseURL(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                使用中转站或本地模型时填写；Anthropic 协议留空即走官方地址
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="ai-model">模型</Label>
              <Input
                id="ai-model"
                placeholder={provider === 'anthropic' ? 'claude-opus-5-5' : 'deepseek-chat'}
                value={model}
                onChange={(e) => setModel(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="ai-api-key">API Key</Label>
              <Input
                id="ai-api-key"
                type="password"
                placeholder={hasKey ? '已配置，留空表示不修改' : '填写厂商提供的 API Key'}
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                密钥加密后保存；本地 Ollama 等无需鉴权的服务可以留空
              </p>
            </div>

            <div className="flex gap-2">
              <Button type="submit" size="sm" disabled={saving}>
                {saving ? '保存中...' : '保存'}
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={handleTest} disabled={testing}>
                {testing && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
                测试连接
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  )
}
