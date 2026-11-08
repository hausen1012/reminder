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
  Send,
  Bell,
  Share2,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { PageHeader } from '@/components/common/PageHeader'
import { FilterToolbar } from '@/components/common/FilterToolbar'
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
import { Pagination } from '@/components/common/pagination'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { listLogs, getLogDetail, purgeLogs, countPurgeLogs, batchDeleteLogs, extractApiError } from '@/lib/api'
import { formatInZone } from '@/lib/datetime'
import { useSiteTimezone } from '@/contexts/ConfigContext'
import { SOURCE_BADGE_CLASS, sourceLabel } from '@/lib/constants'
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
  feishu: MessageCircle,
  telegram: Send,
  bark: Bell,
  ntfy: Bell,
  apprise: Share2,
  webhook: Link2,
  log: FileText,
}

function latencyColor(ms: number): string {
  if (ms < 100) return 'text-green-600'
  if (ms < 500) return 'text-yellow-600'
  return 'text-red-600'
}

export default function LogsPage() {
  const tz = useSiteTimezone()
  const [items, setItems] = useState<DeliveryLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [filter, setFilter] = useState<LogFilter>({})
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(10)
  const [offset, setOffset] = useState(0)
  const [detailId, setDetailId] = useState<number | null>(null)
  const [detail, setDetail] = useState<DeliveryLog | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())
  const [purgeOpen, setPurgeOpen] = useState(false)
  const [purgeMenuOpen, setPurgeMenuOpen] = useState(false)
  const [purgeMode, setPurgeMode] = useState<'7d' | '30d' | 'all'>('7d')
  const [purgeCount, setPurgeCount] = useState(0)
  const [purging, setPurging] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [batchDeleting, setBatchDeleting] = useState(false)
  const [batchConfirmOpen, setBatchConfirmOpen] = useState(false)
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
      setLoadError('')
    } catch (err) {
      setLoadError(extractApiError(err, '加载失败'))
      toast({ title: '加载日志失败', description: extractApiError(err), variant: 'destructive' })
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
      toast({ title: '加载日志详情失败', description: extractApiError(err), variant: 'destructive' })
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
      toast({ title: '清理失败', description: extractApiError(err), variant: 'destructive' })
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

  function toggleSelect(id: number) {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleSelectAll() {
    const allIds = new Set(items.map((log) => log.id))
    if (selectedIds.size === items.length && [...selectedIds].every((id) => allIds.has(id))) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(allIds)
    }
  }

  async function handleBatchDelete() {
    setBatchDeleting(true)
    try {
      await batchDeleteLogs(Array.from(selectedIds))
      toast({ title: `已删除 ${selectedIds.size} 条日志`, variant: 'success' })
      setSelectedIds(new Set())
      const remainAfterDelete = items.length - selectedIds.size
      if (remainAfterDelete === 0 && offset > 0) {
        setOffset(Math.max(0, offset - limit))
      } else {
        await refresh()
      }
    } catch (err) {
      toast({ title: '批量删除失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setBatchDeleting(false)
    }
  }

  return (
    <div className="space-y-4 md:space-y-6">
      <PageHeader title="日志">
        <div className="flex flex-wrap items-center gap-2">
          {selectedIds.size > 0 && (
            <Button size="sm" variant="destructive" className="flex-1 md:flex-none" onClick={() => setBatchConfirmOpen(true)}>
              <Trash2 className="h-4 w-4 mr-1" />
              删除选中 ({selectedIds.size})
            </Button>
          )}
          {/* 桌面端：三种清理范围并排 */}
          <div className="hidden flex-wrap items-center gap-2 md:flex">
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
          {/* 移动端：收进一个入口，避免按钮撑满页头 */}
          <Button variant="outline" size="sm" className="flex-1 md:hidden" onClick={() => setPurgeMenuOpen(true)}>
            <Trash2 className="h-4 w-4 mr-1" />
            清理日志
          </Button>
        </div>
      </PageHeader>

      <FilterToolbar
        searchValue={search}
        onSearchChange={setSearch}
        onSearch={() => {
          setOffset(0)
          setFilter((f) => ({ ...f, search: search.trim() || undefined }))
        }}
        onRefresh={refresh}
        placeholder="搜索标题或内容…"
      >
        <div className="w-[calc(50%-0.25rem)] md:w-32">
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
        <div className="w-[calc(50%-0.25rem)] md:w-32">
          <Select value={filter.source ?? 'all'} onValueChange={(v) => setFilter((f) => ({ ...f, source: v === 'all' ? undefined : v }))}>
            <SelectTrigger>
              <SelectValue placeholder="全部来源" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部来源</SelectItem>
              <SelectItem value="web">Web</SelectItem>
              <SelectItem value="ai">AI</SelectItem>
              <SelectItem value="api">API</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </FilterToolbar>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : loadError ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-destructive">
            <div className="flex items-center justify-center gap-2">
              <AlertCircle className="h-5 w-5" />
              <span>加载失败：{loadError}</span>
            </div>
            <Button variant="outline" size="sm" className="mt-3" onClick={refresh}>
              重试
            </Button>
          </CardContent>
        </Card>
      ) : items.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            还没有发送日志。创建提醒并触发后这里会显示记录。
          </CardContent>
        </Card>
      ) : (
        <Card>
          {/* 桌面端表格 */}
          <div className="hidden md:block">
            <div className="overflow-x-auto">
              <table className="w-full text-[13px] table-fixed">
                <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                  <tr>
                    <th className="px-4 py-2.5 w-10">
                      <Checkbox
                        checked={items.length > 0 && selectedIds.size === items.length}
                        onCheckedChange={toggleSelectAll}
                      />
                    </th>
                    <th className="px-4 py-2.5 w-[20rem]">标题</th>
                    <th className="px-4 py-2.5 w-44">触发时间</th>
                    <th className="px-4 py-2.5 w-28">状态</th>
                    <th className="px-4 py-2.5 w-16">来源</th>
                    <th className="px-4 py-2.5 w-16">确认</th>
                    <th className="px-4 py-2.5 w-24 text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {displayRows.map(({ main, subs }) => {
                    const MainStatusIcon = STATUS_ICON[main.status] ?? Clock
                    const mainColor = STATUS_COLOR[main.status] ?? ''
                    return (
                    <Fragment key={main.id}>
                      <tr className="border-b hover:bg-muted/30">
                        <td className="px-4 py-2.5">
                          <div className="flex items-center gap-1">
                            <Checkbox
                              checked={selectedIds.has(main.id)}
                              onCheckedChange={() => toggleSelect(main.id)}
                            />
                            {subs.length > 0 && (
                              <button onClick={() => toggleExpand(main.id)} className="p-1">
                                {expandedRows.has(main.id) ? (
                                  <ChevronDown className="h-4 w-4" />
                                ) : (
                                  <ChevronRight className="h-4 w-4" />
                                )}
                              </button>
                            )}
                          </div>
                        </td>
                        <td className="px-4 py-2.5 max-w-[20rem]">
                          <div className="font-medium truncate" title={main.title || main.reminder_title}>
                            {main.reminder_title || main.title}
                          </div>
                        </td>
                        <td className="px-4 py-2.5 text-xs">
                          {formatInZone(main.fired_at, tz)}
                        </td>
                        <td className="px-4 py-2.5">
                          <Badge variant="outline" className={`gap-1 ${mainColor}`}>
                            <MainStatusIcon className="h-3 w-3" />
                            {STATUS_LABEL[main.status] ?? main.status}
                          </Badge>
                        </td>
                        <td className="px-4 py-2.5">
                          <Badge variant="secondary" className={SOURCE_BADGE_CLASS[main.source]}>
                            {sourceLabel(main.source)}
                          </Badge>
                        </td>
                        <td className="px-4 py-2.5 text-xs">
                          {main.confirmed ? (
                            <span className="text-green-600">已确认</span>
                          ) : main.confirm_chain_id ? (
                            <span className="text-muted-foreground">待确认</span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </td>
                        <td className="px-4 py-2.5 text-right">
                          <Button size="sm" variant="ghost" className="h-8" onClick={() => handleDetail(main.id)}>
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
                          <td className="px-4 py-2">{formatInZone(sub.fired_at, tz)}</td>
                          <td className="px-4 py-2">
                            <Badge variant="outline" className={`gap-1 ${subColor}`}>
                              <SubStatusIcon className="h-3 w-3" />
                              {STATUS_LABEL[sub.status] ?? sub.status}
                            </Badge>
                          </td>
                          <td />
                          <td />
                          <td className="px-4 py-2 text-right">
                            <Button size="sm" variant="ghost" className="h-7 text-xs" onClick={() => handleDetail(sub.id)}>
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
          </div>

          {/* 移动端卡片列表：标题/状态、时间/来源、确认与重试入口各占一行 */}
          <div className="divide-y md:hidden">
            {displayRows.map(({ main, subs }) => {
              const MainStatusIcon = STATUS_ICON[main.status] ?? Clock;
              const mainColor = STATUS_COLOR[main.status] ?? '';
              const isExpanded = expandedRows.has(main.id);
              return (
                <div key={main.id}>
                  {/* 主卡片 */}
                  <div className="px-3 py-2.5">
                    <div className="flex items-start gap-2.5">
                      <Checkbox
                        checked={selectedIds.has(main.id)}
                        onCheckedChange={() => toggleSelect(main.id)}
                        className="mt-1 shrink-0"
                        aria-label={`选择日志 ${main.reminder_title || main.title}`}
                      />
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium" title={main.title || main.reminder_title}>
                          {main.reminder_title || main.title}
                        </p>
                        <p className="mt-0.5 truncate text-xs text-muted-foreground">
                          {formatInZone(main.fired_at, tz)}
                          {' · '}
                          {sourceLabel(main.source)}
                          {main.confirmed ? ' · 已确认' : main.confirm_chain_id ? ' · 待确认' : ''}
                        </p>
                      </div>
                      <Badge variant="outline" className={`gap-1 shrink-0 text-[11px] ${mainColor}`}>
                        <MainStatusIcon className="h-3 w-3" />
                        {STATUS_LABEL[main.status] ?? main.status}
                      </Badge>
                    </div>

                    <div className="mt-1 flex items-center justify-between pl-6">
                      {subs.length > 0 ? (
                        <button
                          onClick={() => toggleExpand(main.id)}
                          className="inline-flex items-center gap-1 text-xs text-muted-foreground active:text-foreground"
                        >
                          {isExpanded ? (
                            <ChevronDown className="h-3.5 w-3.5" />
                          ) : (
                            <ChevronRight className="h-3.5 w-3.5" />
                          )}
                          重试 ({subs.length})
                        </button>
                      ) : (
                        <span />
                      )}
                      <Button size="sm" variant="ghost" className="h-8 px-2 text-xs" onClick={() => handleDetail(main.id)}>
                        <ExternalLink className="h-3 w-3 mr-1" />
                        详情
                      </Button>
                    </div>
                  </div>

                  {/* 展开的重试子项 */}
                  {isExpanded && subs.map((sub) => {
                    const SubStatusIcon = STATUS_ICON[sub.status] ?? Clock;
                    const subColor = STATUS_COLOR[sub.status] ?? '';
                    return (
                      <div key={sub.id} className="flex items-center gap-2 border-t bg-muted/20 px-3 py-2 pl-9">
                        <span className="text-xs text-muted-foreground">重发 #{sub.retry_round}</span>
                        <span className="truncate text-xs text-muted-foreground">{formatInZone(sub.fired_at, tz)}</span>
                        <Badge variant="outline" className={`ml-auto gap-1 shrink-0 text-[11px] ${subColor}`}>
                          <SubStatusIcon className="h-3 w-3" />
                          {STATUS_LABEL[sub.status] ?? sub.status}
                        </Badge>
                        <Button size="sm" variant="ghost" className="h-7 shrink-0 px-2 text-xs" onClick={() => handleDetail(sub.id)}>
                          详情
                        </Button>
                      </div>
                    );
                  })}
                </div>
              );
            })}
          </div>

          <Pagination total={total} limit={limit} offset={offset} onPageChange={setOffset} onLimitChange={setLimit} />
        </Card>
      )}

      {/* 详情对话框 */}
      <Dialog open={detailId != null} onOpenChange={(o) => { if (!o) { setDetailId(null); setDetail(null) } }}>
        <DialogContent className="max-w-3xl md:max-h-[90dvh] md:overflow-y-auto">
          <DialogHeader className="shrink-0">
            <DialogTitle>日志详情</DialogTitle>
          </DialogHeader>
          {detailLoading ? (
            <p className="text-sm text-muted-foreground py-4 sm:py-0">加载中…</p>
          ) : detail ? (
            <>
              <Separator className="my-1" />

          <Tabs defaultValue="basic" className="mt-2">
            <TabsList className="w-full justify-start overflow-x-auto flex-nowrap gap-0">
              <TabsTrigger value="basic" className="shrink-0">基本信息</TabsTrigger>
              <TabsTrigger value="content" className="shrink-0">发送内容</TabsTrigger>
              {detail.confirm_chain_id && (
                <TabsTrigger value="confirm" className="shrink-0">确认信息</TabsTrigger>
              )}
              <TabsTrigger value="attempts" className="shrink-0">
                投递详情
                {detail.attempts && detail.attempts.length > 0 && (
                  <span className="ml-1.5 rounded-full bg-muted-foreground/20 px-1.5 py-0.5 text-xs">
                    {detail.attempts.length}
                  </span>
                )}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="basic" className="space-y-3">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2.5 text-sm">
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
                  <span>{sourceLabel(detail.source)}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground min-w-[5rem]">触发时间</span>
                  <span>{formatInZone(detail.fired_at, tz)}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground min-w-[5rem]">重试轮次</span>
                  <span>{detail.retry_round > 0 ? `#${detail.retry_round}` : '首次'}</span>
                </div>
                {detail.error && (
                  <div className="col-span-1 sm:col-span-2 flex items-start gap-2">
                    <span className="text-muted-foreground min-w-[5rem] shrink-0">异常说明</span>
                    <span className="text-red-600 dark:text-red-400 break-words">{detail.error}</span>
                  </div>
                )}
              </div>
            </TabsContent>

            <TabsContent value="content" className="space-y-3">
              <div className="flex items-center gap-2 text-sm">
                <span className="text-muted-foreground min-w-[5rem]">标题：</span>
                <span className="font-medium">{detail.title || '—'}</span>
              </div>
              <div className="text-sm">
                <span className="text-muted-foreground">内容：</span>
                <pre className="mt-1.5 max-h-80 overflow-y-auto scroll-area whitespace-pre-wrap break-words rounded-lg border bg-muted p-3 text-xs leading-relaxed md:p-4">
                  {detail.content || '—'}
                </pre>
              </div>
            </TabsContent>

            {detail.confirm_chain_id && (
              <TabsContent value="confirm" className="space-y-3">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 text-sm">
                  <div className="col-span-2 flex flex-wrap items-center gap-2">
                    <span className="text-muted-foreground min-w-[5rem]">确认链 ID</span>
                    <code className="rounded bg-muted px-2 py-0.5 text-xs font-mono break-all">
                      {detail.confirm_chain_id}
                    </code>
                    {detail.confirmed && (
                      <Badge className="bg-green-600">已确认</Badge>
                    )}
                  </div>
                  {detail.confirmed_at && (
                    <div className="col-span-2 flex items-center gap-2">
                      <span className="text-muted-foreground min-w-[5rem]">确认时间</span>
                      <span>{formatInZone(detail.confirmed_at, tz)}</span>
                    </div>
                  )}
                  {detail.confirm_url && (
                    <div className="col-span-2">
                      <span className="text-muted-foreground">确认链接：</span>
                      <div className="mt-1.5 flex flex-col gap-2 sm:flex-row sm:items-center">
                        <code className="min-w-0 flex-1 truncate rounded border bg-muted px-3 py-1.5 text-xs">
                          {detail.confirm_url}
                        </code>
                        <Button
                          size="sm"
                          variant="outline"
                          className="self-start sm:self-auto"
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
                <>
                  {/* 桌面端表格 */}
                  <div className="hidden sm:block rounded-lg border overflow-x-auto">
                    <table className="w-full text-xs whitespace-nowrap">
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
                          <td className="px-3 py-2 whitespace-nowrap">{formatInZone(a.created_at, tz)}</td>
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

                  {/* 移动端投递尝试卡片 */}
                  <div className="sm:hidden space-y-2">
                    {detail.attempts.map((a) => {
                      const ChannelIcon = CHANNEL_ICONS[a.channel_type] ?? FileText
                      return (
                        <div key={a.id} className="rounded-lg border p-2.5 text-sm">
                          <div className="flex items-center justify-between gap-2">
                            <span className="truncate font-medium">{a.channel_name}</span>
                            {a.status === 'success' ? (
                              <span className="inline-flex shrink-0 items-center gap-1 text-xs text-green-600">
                                <CheckCircle2 className="h-3 w-3" />
                                成功
                              </span>
                            ) : (
                              <span className="inline-flex shrink-0 items-center gap-1 text-xs text-red-600">
                                <XCircle className="h-3 w-3" />
                                失败
                              </span>
                            )}
                          </div>
                          <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
                            <span className="inline-flex items-center gap-1">
                              <ChannelIcon className="h-3 w-3" />
                              {a.channel_type}
                            </span>
                            <span>#{a.attempt}</span>
                            <span>{formatInZone(a.created_at, tz)}</span>
                            <span className={`font-mono ${latencyColor(a.latency_ms)}`}>{a.latency_ms}ms</span>
                          </div>
                          {a.error && (
                            <p className="mt-1 break-words text-xs text-destructive">{a.error}</p>
                          )}
                        </div>
                      )
                    })}
                  </div>
                </>
              ) : (
                <p className="text-sm text-muted-foreground py-4">暂无投递记录。</p>
              )}
            </TabsContent>
          </Tabs>
        </>
      ) : (
        <p className="text-sm text-muted-foreground py-4 sm:py-0">加载失败。</p>
      )}
        </DialogContent>
      </Dialog>

      {/* 批量删除确认对话框 */}
      <ConfirmDialog
        open={batchConfirmOpen}
        title="批量删除日志"
        description={`确认删除选中的 ${selectedIds.size} 条日志？该操作不可撤销。`}
        confirmText="删除"
        destructive
        loading={batchDeleting}
        onConfirm={() => {
          handleBatchDelete()
          setBatchConfirmOpen(false)
        }}
        onCancel={() => setBatchConfirmOpen(false)}
      />

      {/* 移动端：清理范围选择 */}
      <Dialog open={purgeMenuOpen} onOpenChange={setPurgeMenuOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>清理日志</DialogTitle>
            <DialogDescription>选择要删除的时间范围，删除后不可恢复。</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Button
              variant="outline"
              className="justify-start"
              onClick={() => {
                setPurgeMenuOpen(false)
                openPurge('7d')
              }}
            >
              清理 7 天前
            </Button>
            <Button
              variant="outline"
              className="justify-start"
              onClick={() => {
                setPurgeMenuOpen(false)
                openPurge('30d')
              }}
            >
              清理 30 天前
            </Button>
            <Button
              variant="destructive"
              className="justify-start"
              onClick={() => {
                setPurgeMenuOpen(false)
                openPurge('all')
              }}
            >
              全部清理
            </Button>
          </div>
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