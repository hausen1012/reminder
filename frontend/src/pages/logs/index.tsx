// 日志页：toolbar（状态/来源/搜索）+ 表格 + 详情抽屉 + 清理
import { Fragment, useCallback, useEffect, useState } from 'react'
import {
  AlertCircle,
  CheckCircle2,
  Clock,
  PauseCircle,
  XCircle,
  ChevronDown,
  ChevronRight,
  Trash2,
  ExternalLink,
  Copy,
  Mail,
  MessageCircle,
  Building2,
  Link2,
  FileText,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Separator } from '@/components/ui/separator'
import { useToast } from '@/components/ui/use-toast'
import { Pagination } from '@/components/ui/pagination'
import { listLogs, getLogDetail, purgeLogs, countPurgeLogs } from '@/lib/api'
import type { DeliveryLog, LogFilter } from '@/types'

const STATUS_ICON: Record<string, typeof CheckCircle2> = {
  success: CheckCircle2,
  partial: AlertCircle,
  failed: XCircle,
  pending: Clock,
  expired: PauseCircle,
}

const STATUS_COLOR: Record<string, string> = {
  success: 'text-green-600',
  partial: 'text-yellow-600',
  failed: 'text-red-600',
  pending: 'text-blue-600',
  expired: 'text-gray-500',
}

const STATUS_LABEL: Record<string, string> = {
  success: '成功',
  partial: '部分成功',
  failed: '失败',
  pending: '发送中',
  expired: '已过期',
}

const CHANNEL_ICONS: Record<string, typeof Mail> = {
  smtp: Mail,
  dingtalk: MessageCircle,
  wecom: Building2,
  webhook: Link2,
  log: FileText,
}

function latencyColor(ms: number): string {
  if (ms < 100) return 'text-green-600'
  if (ms < 500) return 'text-yellow-600'
  return 'text-red-600'
}

export default function LogsPage() {
  const [items, setItems] = useState<DeliveryLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<LogFilter>({})
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(10)
  const [offset, setOffset] = useState(0)
  const [detailId, setDetailId] = useState<number | null>(null)
  const [detail, setDetail] = useState<DeliveryLog | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())
  const [purgeOpen, setPurgeOpen] = useState(false)
  const [purgeMode, setPurgeMode] = useState<'7d' | '30d' | 'all'>('7d')
  const [purgeCount, setPurgeCount] = useState(0)
  const [purging, setPurging] = useState(false)
  const { toast } = useToast()

  useEffect(() => {
    setOffset(0)
  }, [filter])

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const q: LogFilter = { ...filter }
      q.limit = limit
      q.offset = offset
      const data = await listLogs(q)
      setItems(data?.items ?? [])
      setTotal(data?.total ?? 0)
    } catch (err) {
      toast({ title: '加载日志失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter, offset, limit])

  useEffect(() => {
    refresh()
  }, [refresh])

  async function handleDetail(id: number) {
    setDetailId(id)
    setDetailLoading(true)
    try {
      const d = await getLogDetail(id)
      setDetail(d)
    } catch (err) {
      toast({ title: '加载日志详情失败', description: String(err), variant: 'destructive' })
    } finally {
      setDetailLoading(false)
    }
  }

  function toggleExpand(id: number) {
    setExpandedRows((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  // 按 confirm_chain_id 折叠
  const chainGroups = items.reduce<Record<string, DeliveryLog[]>>((acc, log) => {
    const key = log.confirm_chain_id ?? `_single_${log.id}`
    if (!acc[key]) acc[key] = []
    acc[key].push(log)
    return acc
  }, {})

  const displayRows = Object.values(chainGroups)
    .map((group) => {
      group.sort((a, b) => a.retry_round - b.retry_round)
      return { main: group[0], subs: group.slice(1), chainID: group[0].confirm_chain_id }
    })

  async function handlePurge(mode: '7d' | '30d' | 'all') {
    setPurging(true)
    try {
      const olderThan = mode === '7d' ? '168h' : mode === '30d' ? '720h' : undefined
      const all = mode === 'all'
      await purgeLogs(olderThan, all || undefined)
      toast({ title: '清理完成', variant: 'success' })
      setPurgeOpen(false)
      refresh()
    } catch (err) {
      toast({ title: '清理失败', description: String(err), variant: 'destructive' })
    } finally {
      setPurging(false)
    }
  }

  function openPurge(mode: '7d' | '30d' | 'all') {
    setPurgeMode(mode)
    const olderThan = mode === '7d' ? '168h' : mode === '30d' ? '720h' : undefined
    const all = mode === 'all'
    countPurgeLogs(olderThan, all || undefined)
      .then((d) => setPurgeCount(d?.count ?? 0))
      .catch(() => setPurgeCount(0))
    setPurgeOpen(true)
  }

  return (
    <div className="space-y-6">
      <div className="flex-col items-start gap-2 md:flex-row md:items-center md:justify-between flex">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">日志</h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={() => openPurge('7d')}>
            <Trash2 className="h-4 w-4 mr-1" />
            清理 7 天前
          </Button>
          <Button variant="outline" size="sm" onClick={() => openPurge('30d')}>
            清理 30 天前
          </Button>
          <Button variant="destructive" size="sm" onClick={() => openPurge('all')}>
            全部清理
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="w-full md:w-36">
          <Select
            value={filter.status ?? 'all'}
            onValueChange={(v) => setFilter((f) => ({ ...f, status: v === 'all' ? undefined : v }))}
          >
            <SelectTrigger>
              <SelectValue placeholder="全部状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              <SelectItem value="success">成功</SelectItem>
              <SelectItem value="partial">部分成功</SelectItem>
              <SelectItem value="failed">失败</SelectItem>
              <SelectItem value="pending">发送中</SelectItem>
              <SelectItem value="expired">已过期</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="w-full md:w-36">
          <Select value={filter.source ?? 'all'} onValueChange={(v) => setFilter((f) => ({ ...f, source: v === 'all' ? undefined : v }))}>
            <SelectTrigger>
              <SelectValue placeholder="全部来源" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部来源</SelectItem>
              <SelectItem value="web">Web</SelectItem>
              <SelectItem value="api">API</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <form
          className="flex-1 w-full md:max-w-md"
          onSubmit={(e) => {
            e.preventDefault()
            setOffset(0)
            setFilter((f) => ({ ...f, search: search.trim() || undefined }))
          }}
        >
          <Input
            placeholder="搜索标题或内容…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </form>
        <span className="text-sm text-muted-foreground">共 {total} 条</span>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : items.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            还没有发送日志。创建提醒并触发后这里会显示记录。
          </CardContent>
        </Card>
      ) : (
        <Card>
          <div className="overflow-x-auto">
            <table className="w-full text-sm table-fixed">
              <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 w-8" />
                  <th className="px-4 py-3 w-[20rem]">标题</th>
                  <th className="px-4 py-3 w-44">触发时间</th>
                  <th className="px-4 py-3 w-28">状态</th>
                  <th className="px-4 py-3 w-16">来源</th>
                  <th className="px-4 py-3 w-16">确认</th>
                  <th className="px-4 py-3 w-24 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {displayRows.map(({ main, subs }) => {
                  const MainStatusIcon = STATUS_ICON[main.status] ?? Clock
                  const mainColor = STATUS_COLOR[main.status] ?? ''
                  return (
                  <Fragment key={main.id}>
                    <tr className="border-b hover:bg-muted/30">
                      <td className="px-4 py-3">
                        {subs.length > 0 && (
                          <button onClick={() => toggleExpand(main.id)} className="p-1">
                            {expandedRows.has(main.id) ? (
                              <ChevronDown className="h-4 w-4" />
                            ) : (
                              <ChevronRight className="h-4 w-4" />
                            )}
                          </button>
                        )}
                      </td>
                      <td className="px-4 py-3 max-w-[20rem]">
                        <div className="font-medium truncate" title={main.title || main.reminder_title}>
                          {main.reminder_title || main.title}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs">
                        {new Date(main.fired_at).toLocaleString()}
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant="outline" className={`gap-1 ${mainColor}`}>
                          <MainStatusIcon className="h-3 w-3" />
                          {STATUS_LABEL[main.status] ?? main.status}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant="secondary">{main.source === 'api' ? 'API' : 'Web'}</Badge>
                      </td>
                      <td className="px-4 py-3 text-xs">
                        {main.confirmed ? (
                          <span className="text-green-600">已确认</span>
                        ) : main.confirm_chain_id ? (
                          <span className="text-muted-foreground">待确认</span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button size="sm" variant="ghost" onClick={() => handleDetail(main.id)}>
                          <ExternalLink className="h-4 w-4 mr-1" />
                          详情
                        </Button>
                      </td>
                    </tr>
                    {expandedRows.has(main.id) && subs.map((sub) => {
                      const SubStatusIcon = STATUS_ICON[sub.status] ?? Clock
                      const subColor = STATUS_COLOR[sub.status] ?? ''
                      return (
                      <tr key={sub.id} className="border-b bg-muted/20 text-xs">
                        <td />
                        <td className="px-4 py-2 pl-12 text-muted-foreground">
                          重发 #{sub.retry_round}
                        </td>
                        <td className="px-4 py-2">{new Date(sub.fired_at).toLocaleString()}</td>
                        <td className="px-4 py-2">
                          <Badge variant="outline" className={`gap-1 ${subColor}`}>
                            <SubStatusIcon className="h-3 w-3" />
                            {STATUS_LABEL[sub.status] ?? sub.status}
                          </Badge>
                        </td>
                        <td />
                        <td />
                        <td className="px-4 py-2 text-right">
                          <Button size="sm" variant="ghost" onClick={() => handleDetail(sub.id)}>
                            详情
                          </Button>
                        </td>
                      </tr>
                    )})}
                  </Fragment>
                )})}
              </tbody>
            </table>
          </div>
          <Pagination total={total} limit={limit} offset={offset} onPageChange={setOffset} onLimitChange={setLimit} />
        </Card>
      )}

      {/* 详情对话框 */}
      <Dialog open={detailId != null} onOpenChange={(o) => { if (!o) { setDetailId(null); setDetail(null) } }}>
        <DialogContent className="max-w-3xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>日志详情</DialogTitle>
          </DialogHeader>
          {detailLoading ? (
            <p className="text-sm text-muted-foreground">加载中…</p>
          ) : detail ? (
            <>
              {/* 顶部概览条 */}
              <div className="flex flex-wrap items-center gap-3 pb-2">
                {(() => {
                  const Icon = STATUS_ICON[detail.status] ?? Clock
                  return (
                    <Badge variant="outline" className={`gap-1.5 px-3 py-1 text-sm ${STATUS_COLOR[detail.status] ?? ''}`}>
                      <Icon className="h-4 w-4" />
                      {STATUS_LABEL[detail.status] ?? detail.status}
                    </Badge>
                  )
                })()}
                <Badge variant="secondary">{detail.source === 'api' ? 'API' : 'Web'}</Badge>
                {detail.confirmed ? (
                  <Badge className="bg-green-600">已确认</Badge>
                ) : detail.confirm_chain_id ? (
                  <Badge variant="outline" className="text-muted-foreground">待确认</Badge>
                ) : null}
                <span className="text-xs text-muted-foreground ml-auto">{new Date(detail.fired_at).toLocaleString()}</span>
              </div>
              <Separator className="my-1" />

              <Tabs defaultValue="basic" className="mt-2">
                <TabsList className="w-full justify-start">
                  <TabsTrigger value="basic">基本信息</TabsTrigger>
                  <TabsTrigger value="content">发送内容</TabsTrigger>
                  {detail.confirm_chain_id && (
                    <TabsTrigger value="confirm">确认信息</TabsTrigger>
                  )}
                  <TabsTrigger value="attempts">
                    投递尝试
                    {detail.attempts && detail.attempts.length > 0 && (
                      <span className="ml-1.5 rounded-full bg-muted-foreground/20 px-1.5 py-0.5 text-xs">
                        {detail.attempts.length}
                      </span>
                    )}
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="basic" className="space-y-3">
                  <div className="grid grid-cols-2 gap-x-6 gap-y-2.5 text-sm">
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground min-w-[5rem]">日志 ID</span>
                      <span className="font-mono">{detail.id}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground min-w-[5rem]">提醒 ID</span>
                      <span className="font-mono">{detail.reminder_id}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground min-w-[5rem]">提醒标题</span>
                      <span className="font-medium">{detail.reminder_title || '—'}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground min-w-[5rem]">来源</span>
                      <span>{detail.source === 'api' ? 'API' : 'Web'}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground min-w-[5rem]">触发时间</span>
                      <span>{new Date(detail.fired_at).toLocaleString()}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground min-w-[5rem]">重试轮次</span>
                      <span>{detail.retry_round > 0 ? `#${detail.retry_round}` : '首次'}</span>
                    </div>
                  </div>
                </TabsContent>

                <TabsContent value="content" className="space-y-4">
                  <div className="text-sm">
                    <span className="text-muted-foreground">发送标题：</span>
                    <span className="font-medium">{detail.title || '—'}</span>
                  </div>
                  <div className="text-sm">
                    <span className="text-muted-foreground">发送内容：</span>
                    <pre className="mt-1.5 max-h-80 overflow-y-auto whitespace-pre-wrap rounded-lg border bg-muted p-4 text-xs leading-relaxed">
                      {detail.content || '—'}
                    </pre>
                  </div>
                </TabsContent>

                {detail.confirm_chain_id && (
                  <TabsContent value="confirm" className="space-y-3">
                    <div className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
                      <div className="col-span-2 flex items-center gap-2">
                        <span className="text-muted-foreground min-w-[5rem]">确认链 ID</span>
                        <code className="rounded bg-muted px-2 py-0.5 text-xs font-mono">
                          {detail.confirm_chain_id}
                        </code>
                        {detail.confirmed && (
                          <Badge className="bg-green-600">已确认</Badge>
                        )}
                      </div>
                      {detail.confirmed_at && (
                        <div className="col-span-2 flex items-center gap-2">
                          <span className="text-muted-foreground min-w-[5rem]">确认时间</span>
                          <span>{new Date(detail.confirmed_at).toLocaleString()}</span>
                        </div>
                      )}
                      {detail.confirm_url && (
                        <div className="col-span-2">
                          <span className="text-muted-foreground">确认链接：</span>
                          <div className="mt-1.5 flex items-center gap-2">
                            <code className="flex-1 truncate rounded border bg-muted px-3 py-1.5 text-xs">
                              {detail.confirm_url}
                            </code>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => {
                                navigator.clipboard.writeText(detail.confirm_url ?? '')
                                toast({ title: '链接已复制' })
                              }}
                            >
                              <Copy className="h-3 w-3 mr-1" />
                              复制
                            </Button>
                          </div>
                        </div>
                      )}
                    </div>
                  </TabsContent>
                )}

                <TabsContent value="attempts">
                  {detail.attempts && detail.attempts.length > 0 ? (
                    <div className="rounded-lg border">
                      <table className="w-full text-xs table-fixed">
                        <thead>
                          <tr className="border-b bg-muted/30 text-left text-muted-foreground">
                            <th className="px-3 py-2">通道</th>
                            <th className="px-3 py-2">类型</th>
                            <th className="px-3 py-2">#</th>
                            <th className="px-3 py-2">发送时间</th>
                            <th className="px-3 py-2">状态</th>
                            <th className="px-3 py-2">延迟</th>
                            <th className="px-3 py-2">错误</th>
                          </tr>
                        </thead>
                        <tbody>
                          {detail.attempts.map((a) => {
                            const ChannelIcon = CHANNEL_ICONS[a.channel_type] ?? FileText
                            return (
                            <tr key={a.id} className="border-b last:border-0 hover:bg-muted/20">
                              <td className="px-3 py-2 font-medium">{a.channel_name}</td>
                              <td className="px-3 py-2 text-muted-foreground">
                                <span className="inline-flex items-center gap-1">
                                  <ChannelIcon className="h-3 w-3" />
                                  {a.channel_type}
                                </span>
                              </td>
                              <td className="px-3 py-2">{a.attempt}</td>
                              <td className="px-3 py-2 whitespace-nowrap">{new Date(a.created_at).toLocaleString()}</td>
                              <td className="px-3 py-2">
                                {a.status === 'success' ? (
                                  <span className="inline-flex items-center gap-1 text-green-600">
                                    <CheckCircle2 className="h-3 w-3" />
                                    成功
                                  </span>
                                ) : (
                                  <span className="inline-flex items-center gap-1 text-red-600">
                                    <XCircle className="h-3 w-3" />
                                    失败
                                  </span>
                                )}
                              </td>
                              <td className={`px-3 py-2 font-mono ${latencyColor(a.latency_ms)}`}>
                                {a.latency_ms}ms
                              </td>
                              <td className="px-3 py-2 max-w-[200px]" title={a.error}>
                                <div className="truncate">{a.error || '—'}</div>
                              </td>
                            </tr>
                          )})}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground py-4">暂无投递记录。</p>
                  )}
                </TabsContent>
              </Tabs>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">加载失败。</p>
          )}
        </DialogContent>
      </Dialog>

      {/* 清理确认对话框 */}
      <Dialog open={purgeOpen} onOpenChange={setPurgeOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认清理日志</DialogTitle>
            <DialogDescription>
              {purgeMode === 'all'
                ? `将删除全部发送日志（共 ${purgeCount} 条）。`
                : `将删除 ${purgeMode === '7d' ? '7 天' : '30 天'}前的发送日志（共 ${purgeCount} 条）。`}
              此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPurgeOpen(false)} disabled={purging}>
              取消
            </Button>
            <Button variant="destructive" onClick={() => handlePurge(purgeMode)} disabled={purging}>
              {purging ? '清理中…' : '确认清理'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}