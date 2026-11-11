// AI 助手浮窗组件，会话记录暂存 sessionStorage。
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Bot,
  Check,
  Loader2,
  MessageSquare,
  Send,
  Sparkles,
  Square,
  Trash2,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import {
  applyAIAction,
  extractApiError,
  getAISettings,
  listAIActions,
  rejectAIAction,
  sendAIChatStream,
} from '@/lib/api'
import { AI_ACTION_LABEL } from '@/types'
import type { AIAction, AIChatMessage, AIChatProgress } from '@/types'
import { cn } from '@/lib/utils'

const STORAGE_KEY = 'ai-chat-history'
/** 只保留最近这么多条，避免 sessionStorage 无限增长。 */
const MAX_STORED = 40

const STATUS_LABEL: Record<string, string> = {
  pending: '待确认',
  applied: '已执行',
  rejected: '已拒绝',
  failed: '执行失败',
}

const KIND_DESC: Record<string, string> = {
  create_channel: '确认后会在「通知渠道」页新增该渠道',
  create_reminder: '确认后会在「提醒」页新增该提醒',
  update_reminder: '确认后会覆盖该提醒的对应字段',
  delete_reminder: '确认后该提醒会被永久删除',
}

function loadHistory(): AIChatMessage[] {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.filter(
      (m): m is AIChatMessage =>
        Boolean(m) &&
        typeof (m as AIChatMessage).content === 'string' &&
        ((m as AIChatMessage).role === 'user' || (m as AIChatMessage).role === 'assistant'),
    )
  } catch {
    // 隐私模式 / 存储被禁用时静默降级为无历史
    return []
  }
}

function saveHistory(history: AIChatMessage[]) {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(history.slice(-MAX_STORED)))
  } catch {
    // 存不进去不影响本轮对话
  }
}

interface AIChatWidgetProps {
  /** 启用状态变化时的回调。 */
  onEnabledChange?: (enabled: boolean) => void
}

export function AIChatWidget({ onEnabledChange }: AIChatWidgetProps = {}) {
  const { toast } = useToast()
  const [enabled, setEnabled] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const [open, setOpen] = useState(false)
  const [history, setHistory] = useState<AIChatMessage[]>(() => loadHistory())
  const [actions, setActions] = useState<AIAction[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  // 服务端推送的过程状态（思考中 / 第 n 次重试）；null 表示没有可显示的内容
  const [progress, setProgress] = useState<AIChatProgress | null>(null)
  // 当前会话选中的模型：只在本地生效，随每次请求的 model 字段发给后端，
  // 不写回设置——面板里的切换是「这次用哪个模型」，不是「以后都改用它」。
  const [currentModel, setCurrentModel] = useState<string>('')
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const bottomRef = useRef<HTMLDivElement>(null)
  // 当前进行中请求的取消句柄；null 表示没有可中止的请求
  const abortRef = useRef<AbortController | null>(null)

  const loadSettings = useCallback(async () => {
    try {
      const s = await getAISettings()
      if (!s) return
      setEnabled(Boolean(s.enabled))
      // 当前模型始终出现在下拉里：设置的模型可能不在 models 列表中
      const modelList = s.models && s.models.length > 0
        ? s.models
        : (s.model ? [s.model] : [])
      const selected = s.model || modelList[0] || ''
      const merged = selected && !modelList.includes(selected) ? [selected, ...modelList] : modelList
      setAvailableModels(merged)
      setCurrentModel((prev) => (merged.includes(prev) ? prev : selected))
    } catch {
      setEnabled(false)
    } finally {
      setLoaded(true)
    }
  }, [])

  useEffect(() => {
    loadSettings()
  }, [loadSettings])

  // 设置加载完成后再上报，避免首屏误加留白
  useEffect(() => {
    onEnabledChange?.(loaded && enabled)
  }, [loaded, enabled, onEnabledChange])

  const refreshActions = useCallback(async () => {
    try {
      setActions(await listAIActions('pending'))
    } catch {
      setActions([])
    }
  }, [])

  // 挂载后先拉一次待确认数量，用于气泡角标（不必等用户点开）
  useEffect(() => {
    if (!enabled) return
    refreshActions()
  }, [enabled, refreshActions])

  useEffect(() => {
    if (!open) return
    loadSettings()
    refreshActions()
  }, [open, loadSettings, refreshActions])

  useEffect(() => {
    if (!open) return
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [history, sending, open])

  function handleModelChange(val: string) {
    if (!val || val === currentModel) return
    setCurrentModel(val)
    toast({ title: '已切换模型', description: val })
  }

  async function handleSend() {
    const text = input.trim()
    if (!text || sending) return
    setSending(true)
    setProgress(null)
    const nextHistory = [...history, { role: 'user' as const, content: text }]
    setHistory(nextHistory)
    setInput('')

    // 每次发送都新建一个 controller：中止只影响本次请求
    const controller = new AbortController()
    abortRef.current = controller

    try {
      // 只上传对话文本；本次消息单列，历史不含它
      const res = await sendAIChatStream(
        text,
        history,
        currentModel || undefined,
        (p) => setProgress(p),
        controller.signal,
      )
      const withReply: AIChatMessage[] = [
        ...nextHistory,
        { role: 'assistant', content: res?.reply ?? '' },
      ]
      setHistory(withReply)
      saveHistory(withReply)
      if (res?.actions?.length) {
        setActions((prev) => mergeActions(prev, res.actions))
      }
      if (res?.retries && res.retries > 0) {
        toast({ title: `已自动重试 ${res.retries} 次后成功` })
      }
    } catch (err) {
      // 用户主动中止不是错误：把问题还给输入框，方便改一改再发
      if (controller.signal.aborted) {
        setHistory(history)
        setInput(text)
        toast({ title: '已中止本次请求' })
        return
      }
      // 失败时回滚这条用户消息，让输入框内容还在，用户可以直接重发
      setHistory(history)
      setInput(text)
      toast({ title: '请求失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null
      }
      setSending(false)
      setProgress(null)
    }
  }

  /** 中止正在进行的请求：断开 SSE 连接，服务端会随 ctx 取消一并清理本轮草稿。 */
  function handleAbort() {
    abortRef.current?.abort()
  }

  async function handleApply(action: AIAction) {
    try {
      await applyAIAction(action.id)
      setActions((prev) => prev.filter((a) => a.id !== action.id))
      toast({ title: '已执行', description: action.summary, variant: 'success' })
    } catch (err) {
      toast({ title: '执行失败', description: extractApiError(err), variant: 'destructive' })
    }
  }

  async function handleReject(action: AIAction) {
    try {
      await rejectAIAction(action.id)
      setActions((prev) => prev.filter((a) => a.id !== action.id))
    } catch (err) {
      toast({ title: '操作失败', description: extractApiError(err), variant: 'destructive' })
    }
  }

  async function handleRejectAll() {
    const pendingActions = actions.filter((a) => a.status === 'pending')
    if (pendingActions.length === 0) return
    try {
      await Promise.all(pendingActions.map((a) => rejectAIAction(a.id)))
      setActions([])
    } catch (err) {
      refreshActions()
      toast({ title: '操作失败', description: extractApiError(err), variant: 'destructive' })
    }
  }

  function clearHistory() {
    setHistory([])
    saveHistory([])
  }

  if (!loaded || !enabled) return null

  const pendingCount = actions.length

  return (
    <>
      {/* 悬浮按钮：面板打开时直接不渲染，避免它挡住面板或被抓取到 */}
      {!open && (
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label="打开 AI 助手"
        className={cn(
          'fixed right-4 z-40 flex h-12 w-12 items-center justify-center rounded-full md:right-5',
          // 移动端抬高到标签栏之上，避免遮挡导航
          'bottom-[calc(4.5rem+env(safe-area-inset-bottom))] md:bottom-5',
          'bg-primary text-primary-foreground shadow-lg transition-transform hover:scale-105 active:scale-95',
        )}
      >
        <Sparkles className="h-5 w-5" />
        {pendingCount > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-destructive px-1 text-[11px] font-medium text-destructive-foreground">
            {pendingCount}
          </span>
        )}
      </button>
      )}

      {/* 对话面板：桌面右下角浮层，移动端近全屏 */}
      {open && (
        <div
          className={cn(
            'fixed z-50 flex flex-col overflow-hidden border bg-background shadow-2xl',
            'bottom-0 right-0 h-[100dvh] w-full rounded-none',
            'sm:bottom-5 sm:right-5 sm:h-[620px] sm:max-h-[calc(100dvh-2.5rem)] sm:w-[400px] sm:rounded-xl',
          )}
        >
          <div className="flex items-center justify-between border-b px-4 py-3 pt-[calc(0.75rem+env(safe-area-inset-top))] sm:pt-3">
            <div className="flex items-center gap-2">
              <Bot className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm font-medium">AI 助手</span>
              {pendingCount > 0 && (
                <Badge variant="warning" className="text-[11px]">
                  {pendingCount} 条待确认
                </Badge>
              )}
            </div>
            <div className="flex items-center gap-0.5">
              {history.length > 0 && (
                <Button variant="ghost" size="icon" className="h-7 w-7" title="清空对话" onClick={clearHistory}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              )}
              <Button variant="ghost" size="icon" className="h-7 w-7" title="关闭" onClick={() => setOpen(false)}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>

          <div className="flex-1 space-y-3 overflow-y-auto scroll-area p-4">
            {history.length === 0 && actions.length === 0 && (
              <div className="py-8 text-center text-sm text-muted-foreground">
                <MessageSquare className="mx-auto mb-3 h-8 w-8 opacity-40" />
                用自然语言告诉我要做什么
                <div className="mt-3 space-y-1 text-xs">
                  <p>「每天早上 9 点提醒我喝水，发到钉钉」</p>
                  <p>「帮我建一个邮件通知渠道」</p>
                  <p>「把标题是『交房租』的提醒改成每月 1 号」</p>
                </div>
              </div>
            )}

            {history.map((m, i) => (
              <div
                key={`${m.role}-${i}`}
                className={cn('flex', m.role === 'user' ? 'justify-end' : 'justify-start')}
              >
                <div
                  className={cn(
                    'max-w-[85%] whitespace-pre-wrap rounded-lg px-3 py-2 text-sm',
                    m.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-secondary',
                  )}
                >
                  {m.content}
                </div>
              </div>
            ))}

            {sending && (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span className={progress?.kind === 'retry' ? 'text-amber-600' : undefined}>
                  {progressText(progress)}
                </span>
              </div>
            )}

            {actions.length > 0 && (
              <div className="space-y-2 border-t pt-3">
                <div className="flex items-center justify-between">
                  <p className="text-xs font-medium text-muted-foreground">待确认操作（点确认后才生效）</p>
                  {actions.filter((a) => a.status === 'pending').length > 1 && (
                    <button
                      type="button"
                      className="text-[11px] text-muted-foreground hover:text-foreground hover:underline"
                      onClick={handleRejectAll}
                    >
                      全部忽略
                    </button>
                  )}
                </div>
                {actions.map((a) => (
                  <ActionCard
                    key={a.id}
                    action={a}
                    onApply={() => handleApply(a)}
                    onReject={() => handleReject(a)}
                  />
                ))}
              </div>
            )}
            <div ref={bottomRef} />
          </div>

          <div className="space-y-2 border-t p-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] sm:pb-3">
            <Textarea
              rows={2}
              value={input}
              placeholder="描述你要的提醒或通知渠道…"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                // Ctrl/Cmd + Enter 发送，避免与中文输入法的回车冲突
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                  e.preventDefault()
                  handleSend()
                }
              }}
              disabled={sending}
            />
            <div className="flex items-center justify-between gap-2">
              <span className="shrink-0 text-[11px] text-muted-foreground">
                {sending ? '可随时中止' : 'Ctrl / ⌘ + Enter 发送'}
              </span>
              <div className="flex min-w-0 items-center gap-1.5">
                {/* 模型切换：与发送按钮同排，宽度设上限并截断，避免长模型名撑破输入区 */}
                {availableModels.length > 0 && (
                  <Select
                    value={availableModels.includes(currentModel) ? currentModel : ''}
                    onValueChange={handleModelChange}
                  >
                    <SelectTrigger
                      className="h-8 w-auto max-w-[150px] min-w-0 truncate border-none bg-muted/60 px-2 text-xs font-medium shadow-none hover:bg-muted"
                      aria-label="切换模型"
                      title={currentModel || '选择模型'}
                    >
                      <SelectValue placeholder={currentModel || '选择模型'} />
                    </SelectTrigger>
                    <SelectContent align="end" className="max-w-[280px]">
                      {availableModels.map((m) => (
                        <SelectItem key={m} value={m} className="text-xs">
                          {m}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                {sending ? (
                  // 发送中：换成红色中止按钮，点击断开流并回滚本条消息
                  <Button
                    size="icon"
                    variant="destructive"
                    className="h-8 w-8 shrink-0 rounded-full"
                    onClick={handleAbort}
                    aria-label="中止"
                    title="中止本次请求"
                  >
                    <Square className="h-3.5 w-3.5 fill-current" />
                  </Button>
                ) : (
                  <Button
                    size="icon"
                    className="h-8 w-8 shrink-0 rounded-full"
                    onClick={handleSend}
                    disabled={!input.trim()}
                    aria-label="发送"
                    title="发送"
                  >
                    <Send className="h-4 w-4" />
                  </Button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

function ActionCard({
  action,
  onApply,
  onReject,
}: {
  action: AIAction
  onApply: () => void
  onReject: () => void
}) {
  const pending = action.status === 'pending'
  return (
    <div className="rounded-md border bg-card/40 p-2.5">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant="outline" className="text-[11px]">
              {AI_ACTION_LABEL[action.kind] ?? action.kind}
            </Badge>
            <Badge
              variant={
                action.status === 'applied'
                  ? 'success'
                  : action.status === 'failed'
                    ? 'destructive'
                    : 'warning'
              }
              className="text-[11px]"
            >
              {STATUS_LABEL[action.status] ?? action.status}
            </Badge>
          </div>
          <p className="mt-1 truncate text-sm" title={action.summary}>
            {action.summary}
          </p>
          <p className="mt-0.5 text-[11px] text-muted-foreground">
            {action.status === 'failed' && action.error ? action.error : KIND_DESC[action.kind] ?? ''}
          </p>
        </div>
        {pending && (
          <div className="flex shrink-0 gap-1">
            <Button size="sm" className="h-7 px-2 text-xs" onClick={onApply}>
              <Check className="mr-0.5 h-3 w-3" />
              确认
            </Button>
            <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={onReject}>
              忽略
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}

/** 把服务端推送的进度翻译成一行给用户看的文案。 */
function progressText(p: AIChatProgress | null): string {
  if (!p) return '助手正在思考…'
  if (p.kind === 'retry') {
    return `第 ${p.retry.attempt}/${p.retry.max} 次重试：${p.retry.reason}`
  }
  return p.status.text || '助手正在思考…'
}

/** 合并草稿列表：按 id 去重，保留后端返回的最新状态。 */
function mergeActions(prev: AIAction[], next: AIAction[]): AIAction[] {
  const map = new Map<string, AIAction>()
  prev.forEach((a) => map.set(a.id, a))
  next.forEach((a) => map.set(a.id, a))
  return Array.from(map.values())
}
