// AI 设置分区：厂商、API 地址与 Key、默认模型与模型列表管理、自定义提示词。
//
// 密钥由后端加密存储，读回时恒为 "***"；用户不动输入框就原样回传占位符，
// 后端据此保留原密钥（与通知通道的敏感字段是同一套约定）。
// 自定义提示词会追加在内置规则之后，只用于补充风格与业务约定。
import { useEffect, useState, type FormEvent } from 'react'
import {
  Eye,
  EyeOff,
  Loader2,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import {
  extractApiError,
  fetchAIModels,
  getAISettings,
  testAIConnection,
  updateAISettings,
} from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
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
  openai: 'OpenAI',
  anthropic: 'Anthropic',
}

// 与后端 services 包的边界保持一致，前端只做提示，真正的校验在后端
const MIN_MAX_TOKENS = 256
const MAX_MAX_TOKENS = 200000
const MAX_MAX_RETRIES = 100

// 常见厂商的默认地址与模型，选中后自动填入，仍可手工修改
const PRESETS: Record<string, { base_url: string; model: string; models: string[] }> = {
  anthropic: {
    base_url: '',
    model: 'claude-3-5-sonnet-20241022',
    models: ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022', 'claude-opus-5-5'],
  },
  deepseek: {
    base_url: 'https://api.deepseek.com',
    model: 'deepseek-chat',
    models: ['deepseek-chat', 'deepseek-reasoner'],
  },
  moonshot: {
    base_url: 'https://api.moonshot.cn/v1',
    model: 'moonshot-v1-8k',
    models: ['moonshot-v1-8k', 'moonshot-v1-32k', 'moonshot-v1-128k'],
  },
  openai: {
    base_url: 'https://api.openai.com/v1',
    model: 'gpt-4o-mini',
    models: ['gpt-4o-mini', 'gpt-4o', 'o1-mini'],
  },
  ollama: {
    base_url: 'http://localhost:11434/v1',
    model: 'qwen2.5',
    models: ['qwen2.5', 'llama3.1', 'deepseek-r1'],
  },
}

export function AISettingsSection() {
  const { toast } = useToast()
  const [enabled, setEnabled] = useState(false)
  const [provider, setProvider] = useState<AIProvider>('anthropic')
  const [baseURL, setBaseURL] = useState('')
  const [model, setModel] = useState('')
  const [models, setModels] = useState<string[]>([])
  const [apiKey, setApiKey] = useState('')
  const [showKey, setShowKey] = useState(false)
  const [customPrompt, setCustomPrompt] = useState('')
  // 单次回复的输出上限（max_tokens）；0 表示交给后端默认值
  const [maxTokens, setMaxTokens] = useState(4096)
  // 接口失败后的最大重试次数；0 表示不重试（后端只在字段缺省时才用默认的 10）
  const [maxRetries, setMaxRetries] = useState(10)
  const [hasKey, setHasKey] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [fetchingModels, setFetchingModels] = useState(false)

  useEffect(() => {
    getAISettings()
      .then((s) => {
        if (!s) return
        setEnabled(Boolean(s.enabled))
        setProvider(s.provider === 'openai' ? 'openai' : 'anthropic')
        setBaseURL(s.base_url ?? '')
        setModel(s.model ?? '')
        setModels(s.models ?? [])
        setHasKey(s.api_key === '***')
        setApiKey('') // 输入框留空表示"不修改"；只有 hasKey 时才有原值
        setCustomPrompt(s.custom_prompt ?? '')
        setMaxTokens(s.max_tokens || 4096)
        setMaxRetries(s.max_retries ?? 10)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  function toInput() {
    const key = apiKey.trim() === '' && hasKey ? '***' : apiKey.trim()
    const cleanedModels = models.map((m) => m.trim()).filter(Boolean)

    return {
      enabled,
      provider,
      base_url: baseURL.trim(),
      model: model.trim(),
      models: cleanedModels,
      api_key: key,
      custom_prompt: customPrompt.trim(),
      max_tokens: maxTokens,
      max_retries: maxRetries,
    }
  }

  async function handleSave(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      const saved = await updateAISettings(toInput())
      setHasKey(saved.api_key === '***')
      setApiKey('')
      if (saved.models && saved.models.length > 0) {
        setModels(saved.models)
      }
      // 后端会把 0 归一化成默认值，回填让输入框显示真正生效的数字
      if (saved.max_tokens > 0) setMaxTokens(saved.max_tokens)
      if (typeof saved.max_retries === 'number' && saved.max_retries >= 0) {
        setMaxRetries(saved.max_retries)
      }
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

  async function handleFetchModels() {
    setFetchingModels(true)
    try {
      const list = await fetchAIModels(toInput())
      if (!list || list.length === 0) {
        toast({ title: '未获取到模型', description: '接口返回的模型列表为空' })
        return
      }
      const merged = Array.from(new Set([...list, ...models.filter(Boolean)]))
      setModels(merged)
      if (!model || !merged.includes(model)) {
        setModel(list[0])
      }
      toast({ title: '刷新成功', description: `共获取到 ${list.length} 个模型` })
    } catch (err) {
      toast({ title: '刷新模型失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setFetchingModels(false)
    }
  }

  function handleAddModelRow() {
    setModels((prev) => [...prev, ''])
  }

  function handleModelItemChange(index: number, val: string) {
    setModels((prev) => {
      const next = [...prev]
      const old = next[index]
      next[index] = val
      if (model === old) {
        setModel(val)
      }
      return next
    })
  }

  function handleRemoveModelAt(index: number) {
    setModels((prev) => {
      const removed = prev[index]
      const next = prev.filter((_, i) => i !== index)
      if (model === removed) {
        setModel(next[0] ?? '')
      }
      return next
    })
  }

  function applyPreset(name: string) {
    const p = PRESETS[name]
    if (!p) return
    setBaseURL(p.base_url)
    setModel(p.model)
    if (p.models && p.models.length > 0) {
      setModels(p.models)
    }
    setProvider(name === 'anthropic' ? 'anthropic' : 'openai')
  }

  const validModels = Array.from(new Set(models.map((m) => m.trim()).filter(Boolean)))

  if (loading) {
    return <p className="text-sm text-muted-foreground">加载中…</p>
  }

  return (
    <form onSubmit={handleSave} className="grid gap-5 sm:grid-cols-2">
      <div className="flex items-center justify-between sm:col-span-2">
        <Label htmlFor="ai-enabled">启用 AI 助手</Label>
        <Switch id="ai-enabled" checked={enabled} onCheckedChange={setEnabled} />
      </div>

      <div className="space-y-2">
        <Label>供应商预设</Label>
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
        <p className="text-xs text-muted-foreground">用于快速填入推荐地址与模型，之后仍可修改</p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="ai-provider">厂商协议</Label>
        <Select value={provider} onValueChange={(v) => setProvider(v as AIProvider)}>
          <SelectTrigger id="ai-provider">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(['openai', 'anthropic'] as AIProvider[]).map((p) => (
              <SelectItem key={p} value={p}>
                {PROVIDER_LABEL[p]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* API 地址与 API Key 放在同一行 */}
      <div className="space-y-2">
        <Label htmlFor="ai-base-url">API 地址（Base URL）</Label>
        <Input
          id="ai-base-url"
          placeholder={provider === 'anthropic' ? '留空使用官方地址' : 'https://api.openai.com/v1'}
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
        />
        <p className="text-xs text-muted-foreground">
          {provider === 'anthropic'
            ? 'Anthropic Messages API 的服务根地址，例如 https://api.anthropic.com。'
            : '兼容 OpenAI 协议的 API 端点（OpenAI/DeepSeek/Qwen/Kimi/ollama 等）。'}
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="ai-api-key">API Key</Label>
        <div className="relative">
          <Input
            id="ai-api-key"
            type={showKey ? 'text' : 'password'}
            placeholder={hasKey ? '已配置，留空表示不修改' : '填写厂商提供的 API Key'}
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            className="pr-10"
          />
          <button
            type="button"
            onClick={() => setShowKey(!showKey)}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            tabIndex={-1}
            aria-label={showKey ? '隐藏密钥' : '显示密钥'}
          >
            {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
          </button>
        </div>
        <p className="text-xs text-muted-foreground">
          密钥加密后保存；本地 Ollama 等服务可留空
        </p>
      </div>

      {/* 默认模型：输入框 + 下拉快速选择 */}
      <div className="space-y-2 sm:col-span-2">
        <Label htmlFor="ai-model">默认模型</Label>
        <div className="flex gap-2">
          <Input
            id="ai-model"
            placeholder={provider === 'anthropic' ? 'claude-opus-5-5' : 'deepseek-chat'}
            value={model}
            onChange={(e) => setModel(e.target.value)}
            className="flex-1"
          />
          {validModels.length > 0 && (
            <Select
              value={validModels.includes(model) ? model : ''}
              onValueChange={(val) => {
                if (val) setModel(val)
              }}
            >
              <SelectTrigger className="w-[180px] shrink-0" aria-label="选择模型">
                <SelectValue placeholder="从列表快速选择" />
              </SelectTrigger>
              <SelectContent className="max-h-60">
                {validModels.map((m) => (
                  <SelectItem key={m} value={m}>
                    {m}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          对话默认使用的模型，可在输入框直接填写，或从右侧下拉选择
        </p>
      </div>

      {/* 模型列表：与截图完全一致的头部按钮和输入行设计 */}
      <div className="space-y-2.5 sm:col-span-2">
        <div className="flex items-center justify-between">
          <span className="text-sm font-semibold">模型列表</span>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={handleFetchModels}
              disabled={fetchingModels}
              className="h-8 gap-1.5"
            >
              {fetchingModels ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="h-3.5 w-3.5" />
              )}
              刷新模型
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={handleAddModelRow}
              className="h-8 gap-1.5"
            >
              <Plus className="h-3.5 w-3.5" />
              添加模型
            </Button>
          </div>
        </div>

        <div className="space-y-2 max-h-64 overflow-y-auto pr-1">
          {models.length === 0 ? (
            <div className="py-4 text-center text-xs text-muted-foreground border rounded-md border-dashed">
              暂无模型。点击右上角「刷新模型」自动获取，或「添加模型」手动填写
            </div>
          ) : (
            models.map((m, index) => (
              <div key={index} className="flex items-center gap-2">
                <Input
                  value={m}
                  onChange={(e) => handleModelItemChange(index, e.target.value)}
                  placeholder="输入模型名称（例如 deepseek-v4.1-flash）"
                  className="flex-1"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-9 w-9 shrink-0 text-muted-foreground hover:text-destructive"
                  onClick={() => handleRemoveModelAt(index)}
                  title="删除"
                  aria-label={`删除模型 ${m || index}`}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))
          )}
        </div>
      </div>

      <div className="space-y-2 sm:col-span-2">
        <Label htmlFor="ai-custom-prompt">自定义提示词（可选）</Label>
        <Textarea
          id="ai-custom-prompt"
          rows={5}
          maxLength={4000}
          placeholder="例如：称呼我为「老板」；回复尽量简短，一句话说完；提醒标题统一加上「⏰」前缀。"
          value={customPrompt}
          onChange={(e) => setCustomPrompt(e.target.value)}
        />
        <p className="text-xs text-muted-foreground">
          追加在内置规则之后，用于补充称呼、语气与业务约定（最多 4000 字）。
          它不能改变工具使用与「待确认后才生效」等硬性规则；留空表示只用内置提示词。
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="ai-max-tokens">最大上下文窗口（Token）</Label>
        <Input
          id="ai-max-tokens"
          type="number"
          min={MIN_MAX_TOKENS}
          max={MAX_MAX_TOKENS}
          step={256}
          value={maxTokens}
          onChange={(e) => setMaxTokens(Number(e.target.value))}
        />
        <p className="text-xs text-muted-foreground">
          单次回复的输出上限（max_tokens），默认 4096。模型达到该值会停止生成，
          本轮不再继续工具调用，避免把截断的参数写成草稿。取值范围 {MIN_MAX_TOKENS}–{MAX_MAX_TOKENS}。
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="ai-max-retries">最大重试次数</Label>
        <Input
          id="ai-max-retries"
          type="number"
          min={0}
          max={MAX_MAX_RETRIES}
          step={1}
          value={maxRetries}
          onChange={(e) => setMaxRetries(Number(e.target.value))}
        />
        <p className="text-xs text-muted-foreground">
          接口失败（限流、超时、网络错误、服务端 5xx）后的自动重试次数，默认 10。
          填 0 表示不重试；密钥错误、参数错误这类不会重试。
        </p>
      </div>

      <div className="flex gap-2 sm:col-span-2">
        <Button type="submit" size="sm" disabled={saving}>
          {saving ? '保存中...' : '保存'}
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={handleTest} disabled={testing}>
          {testing && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
          测试连接
        </Button>
      </div>
    </form>
  )
}
