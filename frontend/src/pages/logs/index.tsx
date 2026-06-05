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
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { useToast } from '@/components/ui/use-toast'
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

export default function LogsPage() {
  const [items, setItems] = useState<DeliveryLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<LogFilter>({})
  const [search, setSearch] = useState('')
  const [detailId, setDetailId] = useState<number | null>(null)
  const [detail, setDetail] = useState<DeliveryLog | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())
  const [purgeOpen, setPurgeOpen] = useState(false)
  const [purgeMode, setPurgeMode] = useState<'7d' | '30d' | 'all'>('7d')
  const [purgeCount, setPurgeCount] = useState(0)
  const [purging, setPurging] = useState(false)
  const { toast } = useToast()

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const q: LogFilter = { ...filter }
      if (search.trim()) q.search = search.trim()
      const data = await listLogs(q)
      setItems(data?.items ?? [])
      setTotal(data?.total ?? 0)
    } catch (err) {
      toast({ title: '加载日志失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter])

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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">发送日志</h1>
          <p className="text-sm text-muted-foreground mt-1">
            查看提醒触发的发送记录与各通道的投递详情。
          </p>
        </div>
        <div className="flex gap-2">
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
        <div className="w-36">
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
        <div className="w-36">
          <Select value={filter.source ?? 'all'} onValueChange={(v) => setFilter((f) => ({ ...f, source: v === 'all' ? undefined : v }))}>
            <SelectTrigger>
              <SelectValue placeholder="全部来源" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部来源</SelectItem>
              <SelectItem value="manual">手动</SelectItem>
              <SelectItem value="api">API</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <form
          className="flex-1 max-w-md"
          onSubmit={(e) => {
            e.preventDefault()
            refresh()
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
            <table className="w-full text-sm">
              <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 w-8" />
                  <th className="px-4 py-3">标题</th>
                  <th className="px-4 py-3">触发时间</th>
                  <th className="px-4 py-3">状态</th>
                  <th className="px-4 py-3">来源</th>
                  <th className="px-4 py-3">确认</th>
                  <th className="px-4 py-3 text-right">操作</th>
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
                        <Badge variant="secondary">{main.source === 'api' ? 'API' : '手动'}</Badge>
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
        </Card>
      )}

      {/* 详情抽屉 */}
      <Drawer open={detailId != null} onOpenChange={(o) => { if (!o) { setDetailId(null); setDetail(null) } } }>
        <DrawerContent className="max-h-[85vh]">
          <DrawerHeader>
            <DrawerTitle>日志详情</DrawerTitle>
          </DrawerHeader>
          <div className="px-6 pb-6 overflow-y-auto max-h-[70vh] space-y-4">
            {detailLoading ? (
              <p className="text-sm text-muted-foreground">加载中…</p>
            ) : detail ? (
              <>
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div><span className="text-muted-foreground">日志 ID：</span>{detail.id}</div>
                  <div><span className="text-muted-foreground">提醒 ID：</span>{detail.reminder_id}</div>
                  <div><span className="text-muted-foreground">提醒标题：</span>{detail.reminder_title}</div>
                  <div>
                    <span className="text-muted-foreground">状态：</span>
                    <Badge variant="outline" className={STATUS_COLOR[detail.status] ?? ''}>
                      {STATUS_LABEL[detail.status] ?? detail.status}
                    </Badge>
                  </div>
                  <div className="col-span-2"><span className="text-muted-foreground">触发时间：</span>{new Date(detail.fired_at).toLocaleString()}</div>
                  <div className="col-span-2"><span className="text-muted-foreground">发送标题：</span>{detail.title || '—'}</div>
                  <div className="col-span-2">
                    <span className="text-muted-foreground">发送内容：</span>
                    <pre className="whitespace-pre-wrap text-xs mt-1 rounded bg-muted p-2">{detail.content || '—'}</pre>
                  </div>
                  {detail.confirm_chain_id && (
                    <div className="col-span-2">
                      <span className="text-muted-foreground">确认链：</span>
                      {detail.confirm_chain_id}
                      {detail.confirmed && <Badge className="ml-2 bg-green-600">已确认</Badge>}
                    </div>
                  )}
                  {detail.confirm_url && (
                    <div className="col-span-2">
                      <span className="text-muted-foreground">确认链接：</span>
                      <div className="mt-1 flex items-center gap-2">
                        <code className="flex-1 truncate rounded bg-muted px-2 py-1 text-xs">
                          {detail.confirm_url}
                        </code>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            navigator.clipboard.writeText(detail.confirm_url ?? '')
                            toast({ title: '链接已复制' })
                          }}
                        >
                          <Copy className="h-3 w-3" />
                        </Button>
                      </div>
                    </div>
                  )}
                </div>

                <div>
                  <h4 className="text-sm font-medium mb-2">投递尝试</h4>
                  {detail.attempts && detail.attempts.length > 0 ? (
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="border-b text-left text-muted-foreground">
                          <th className="py-1 pr-2">通道</th>
                          <th className="py-1 pr-2">类型</th>
                          <th className="py-1 pr-2">#</th>
                          <th className="py-1 pr-2">发送时间</th>
                          <th className="py-1 pr-2">状态</th>
                          <th className="py-1 pr-2">延迟</th>
                          <th className="py-1 pr-2">错误</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.attempts.map((a) => (
                          <tr key={a.id} className="border-b">
                            <td className="py-1 pr-2 font-medium">{a.channel_name}</td>
                            <td className="py-1 pr-2 text-muted-foreground">{a.channel_type}</td>
                            <td className="py-1 pr-2">{a.attempt}</td>
                            <td className="py-1 pr-2 text-xs whitespace-nowrap">{new Date(a.created_at).toLocaleString()}</td>
                            <td className="py-1 pr-2">
                              {a.status === 'success' ? (
                                <span className="text-green-600">成功</span>
                              ) : (
                                <span className="text-red-600">失败</span>
                              )}
                            </td>
                            <td className="py-1 pr-2">{a.latency_ms}ms</td>
                            <td className="py-1 pr-2 max-w-[200px] truncate" title={a.error}>
                              {a.error || '—'}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  ) : (
                    <p className="text-xs text-muted-foreground">暂无投递记录。</p>
                  )}
                </div>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">加载失败。</p>
            )}
          </div>
        </DrawerContent>
      </Drawer>

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