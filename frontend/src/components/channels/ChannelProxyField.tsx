// 通道网络代理选择与配置组件：提供系统全局代理、直连与自定义代理三态切换。
import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export type ProxyMode = 'global' | 'direct' | 'custom'

export interface ChannelProxyFieldProps {
  value: string
  onChange: (value: string) => void
  globalProxy?: string
  onValidityChange?: (isValid: boolean) => void
  showError?: boolean
}

// detectProxyMode 根据 proxy_url 的字面值推导当前模式：
// 1. 空或未设置 -> 系统全局代理
// 2. direct（忽略大小写）-> 直连
// 3. 其它任意字符串 -> 自定义代理
export function detectProxyMode(value: string): ProxyMode {
  const trimmed = value.trim()
  if (!trimmed) {
    return 'global'
  }
  if (trimmed.toLowerCase() === 'direct') {
    return 'direct'
  }
  return 'custom'
}

export function ChannelProxyField({
  value,
  onChange,
  globalProxy,
  onValidityChange,
  showError = false,
}: ChannelProxyFieldProps) {
  const [mode, setMode] = useState<ProxyMode>(() => detectProxyMode(value))
  const [customUrl, setCustomUrl] = useState(() =>
    detectProxyMode(value) === 'custom' ? value : '',
  )
  const [touched, setTouched] = useState(false)

  // 记录最近一次向外派发的值，用于区分“外部传入新值（如切换渠道）”与“内部用户输入导致父组件重渲”。
  const emittedRef = useRef(value)

  const isCustomEmpty = mode === 'custom' && !customUrl.trim()
  const isValid = !isCustomEmpty

  useEffect(() => {
    onValidityChange?.(isValid)
  }, [isValid, onValidityChange])

  useEffect(() => {
    if (value !== emittedRef.current) {
      emittedRef.current = value
      const nextMode = detectProxyMode(value)
      setMode(nextMode)
      if (nextMode === 'custom') {
        setCustomUrl(value)
      }
      setTouched(false)
    }
  }, [value])

  function handleModeChange(nextMode: ProxyMode) {
    setMode(nextMode)
    setTouched(false)
    let nextValue = ''
    if (nextMode === 'direct') {
      nextValue = 'direct'
    } else if (nextMode === 'custom') {
      nextValue = customUrl
    }
    emittedRef.current = nextValue
    onChange(nextValue)
  }

  function handleCustomUrlChange(e: ChangeEvent<HTMLInputElement>) {
    const nextVal = e.target.value
    setCustomUrl(nextVal)
    emittedRef.current = nextVal
    onChange(nextVal)
  }

  const trimmedGlobal = globalProxy?.trim()
  const hasError = isCustomEmpty && (touched || showError)

  return (
    <div className="space-y-3 pt-2 border-t">
      <div className="space-y-2">
        <Label htmlFor="channel-proxy-mode">网络代理</Label>
        <Select value={mode} onValueChange={(v) => handleModeChange(v as ProxyMode)}>
          <SelectTrigger id="channel-proxy-mode">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="global">
              {trimmedGlobal ? `系统全局代理 (${trimmedGlobal})` : '系统全局代理'}
            </SelectItem>
            <SelectItem value="direct">直连</SelectItem>
            <SelectItem value="custom">自定义代理</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {mode === 'custom' && (
        <div className="space-y-2">
          <Label htmlFor="channel-proxy-url" className="text-xs text-muted-foreground">
            自定义代理地址
          </Label>
          <Input
            id="channel-proxy-url"
            value={customUrl}
            onChange={handleCustomUrlChange}
            onBlur={() => setTouched(true)}
            placeholder="例如: http://127.0.0.1:7890 或 socks5://127.0.0.1:1080"
            required
            autoFocus
            aria-invalid={hasError}
          />
          {hasError && (
            <p className="text-xs text-destructive">自定义代理地址不能为空</p>
          )}
        </div>
      )}
    </div>
  )
}
