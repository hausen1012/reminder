// 提醒列表页：toolbar（来源筛选/状态/搜索/新建）+ 表格 + 编辑/试发/删除。
import { useEffect, useState, useMemo, useCallback } from 'react'
import { Checkbox } from '@/components/ui/checkbox'
import { Plus, Pencil, Trash2, Copy, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import { Pagination } from '@/components/ui/pagination'
import { ReminderEditDialog } from '@/components/reminders/ReminderEditDialog'
import { ConfirmDialog } from '@/components/channels/ConfirmDialog'
import {
  batchDeleteReminders,
  createReminder,
  deleteReminder,
  listChannels,
  listReminders,
  toggleReminder,
  type ListRemindersQuery,
} from '@/lib/api'
import { formatReminderDetail, formatTime, formatNextFire } from '@/lib/utils'
import { SortIcon } from '@/components/ui/SortIcon'
import { PageHeader } from '@/components/ui/PageHeader'
import { FilterToolbar } from '@/components/ui/FilterToolbar'
import type { Channel, Reminder } from '@/types'

const SOURCE_LABEL: Record<string, string> = {
  web: 'Web',
  api: 'API',
}

const TYPE_LABEL: Record<string, string> = {
  once: '单次',
  interval: '周期',
  cron: 'Cron',
}

export default function RemindersPage() {
  const [items, setItems] = useState<Reminder[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [editing, setEditing] = useState<Reminder | null>(null)
  const [creating, setCreating] = useState(false)
  const [toDelete, setToDelete] = useState<Reminder | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [batchDeleting, setBatchDeleting] = useState(false)
  const [batchConfirmOpen, setBatchConfirmOpen] = useState(false)

  const [source, setSource] = useState<string>('web')
  const [enabled, setEnabled] = useState<string>('all')
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(10)
  const [offset, setOffset] = useState(0)
  const [sortBy, setSortBy] = useState('created_at')
  const [sortOrder, setSortOrder] = useState('desc')

  const [channels, setChannels] = useState<Channel[]>([])

  const { toast } = useToast()

  function toggleSort(field: string) {
    if (sortBy === field) {
      setSortOrder((o) => (o === 'desc' ? 'asc' : 'desc'))
    } else {
      setSortBy(field)
      setSortOrder('desc')
    }
    setOffset(0)
  }

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const q: ListRemindersQuery = {}
      if (source !== 'all') q.source = source as 'web' | 'api'
      if (enabled !== 'all') q.enabled = enabled === 'true'
      if (search.trim()) q.search = search.trim()
      q.limit = limit
      q.offset = offset
      q.sort_by = sortBy
      q.sort_order = sortOrder
      const data = await listReminders(q)
      setItems(data?.items ?? [])
      setTotal(data?.total ?? 0)
      setLoadError('')
    } catch (err) {
      setLoadError(String(err))
      toast({ title: '加载提醒失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }, [source, enabled, search, limit, offset, sortBy, sortOrder, toast])

  useEffect(() => {
    listChannels().then(setChannels).catch(() => setChannels([]))
  }, [])

  useEffect(() => {
    setOffset(0)
  }, [source, enabled])

  useEffect(() => {
    refresh()
  }, [refresh])

  async function handleToggle(r: Reminder) {
    try {
      const next = await toggleReminder(r.id)
      setItems((prev) => prev.map((it) => (it.id === r.id ? next : it)))
    } catch (err) {
      toast({ title: '切换状态失败', description: String(err), variant: 'destructive' })
    }
  }

  async function handleDelete() {
    if (!toDelete) return
    setDeleting(true)
    try {
      await deleteReminder(toDelete.id)
      setItems((prev) => prev.filter((it) => it.id !== toDelete.id))
      setTotal((n) => n - 1)
      toast({ title: '提醒已删除', variant: 'success' })
    } catch (err) {
      toast({ title: '删除失败', description: String(err), variant: 'destructive' })
    } finally {
      setDeleting(false)
      setToDelete(null)
    }
  }

  async function handleDuplicate(r: Reminder) {
    try {
      await createReminder({
        title: r.title + '-copy',
        content: r.content,
        content_format: r.content_format,
        calendar: r.calendar,
        schedule_type: r.schedule_type,
        schedule_spec: r.schedule_spec,
        timezone: r.timezone,
        channel_ids: r.channel_ids,
        require_confirm: r.require_confirm,
        confirm_retry_interval_sec: r.confirm_retry_interval_sec,
        confirm_max_retries: r.confirm_max_retries,
        source: 'web',
      })
      toast({ title: '提醒已复制', variant: 'success' })
      refresh()
    } catch (err) {
      toast({ title: '复制失败', description: String(err), variant: 'destructive' })
    }
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
    if (selectedIds.size === items.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(items.map((r) => r.id)))
    }
  }

  async function handleBatchDelete() {
    setBatchDeleting(true)
    try {
      await batchDeleteReminders(Array.from(selectedIds))
      toast({ title: `已删除 ${selectedIds.size} 条提醒`, variant: 'success' })
      setSelectedIds(new Set())
      setTotal((n) => Math.max(0, n - selectedIds.size))
      const remainAfterDelete = items.length - selectedIds.size
      if (remainAfterDelete === 0 && offset > 0) {
        setOffset(Math.max(0, offset - limit))
      } else {
        await refresh()
      }
    } catch (err) {
      toast({ title: '批量删除失败', description: String(err), variant: 'destructive' })
    } finally {
      setBatchDeleting(false)
    }
  }

  const channelMap = useMemo(() => {
    const map = new Map<number, string>()
    channels.forEach((ch) => map.set(ch.id, ch.name))
    return map
  }, [channels])

  return (
    <div className="space-y-6">
      <PageHeader title="提醒">
          <div className="flex items-center gap-2">
            {selectedIds.size > 0 && (
              <Button size="sm" variant="destructive" onClick={() => setBatchConfirmOpen(true)}>
                <Trash2 className="h-4 w-4 mr-1" />
                删除选中 ({selectedIds.size})
              </Button>
            )}
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4 mr-1" />
              新建提醒
            </Button>
          </div>
        </PageHeader>

      <FilterToolbar
        searchValue={search}
        onSearchChange={setSearch}
        onSearch={refresh}
        onRefresh={refresh}
        placeholder="搜索标题或内容…"
      >
        <div className="w-[calc(50%-0.25rem)] md:w-32">
          <Select value={source} onValueChange={setSource}>
            <SelectTrigger>
              <SelectValue placeholder="来源" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部来源</SelectItem>
              <SelectItem value="web">Web</SelectItem>
              <SelectItem value="api">API</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="w-[calc(50%-0.25rem)] md:w-32">
          <Select value={enabled} onValueChange={setEnabled}>
            <SelectTrigger>
              <SelectValue placeholder="状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              <SelectItem value="true">已启用</SelectItem>
              <SelectItem value="false">已禁用</SelectItem>
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
            还没有提醒，点击右上角新建一个。
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
                    <th className="px-4 py-2.5 w-[12rem]">标题</th>
                    <th className="px-4 py-2.5 w-16">类型</th>
                    <th className="px-4 py-2.5 w-[12rem]">详情</th>
                    <th className="px-4 py-2.5 w-40">下次触发</th>
                    <th className="px-4 py-2.5 w-[10rem]">通知</th>
                    <th className="px-4 py-2.5 w-16 text-center">启用</th>
                    <th className="px-4 py-2.5 w-16 pl-16">来源</th>
                    <th className="px-4 py-2.5 w-44 whitespace-nowrap cursor-pointer select-none" onClick={() => toggleSort('created_at')}>
                      创建时间<SortIcon active={sortBy === 'created_at'} direction={sortOrder} />
                    </th>
                    <th className="px-4 py-2.5 w-36 text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((r) => {
                    const chNames = r.channel_ids.map((cid) => channelMap.get(cid) || `#${cid}`)
                    return (
                      <tr key={r.id} className="border-b last:border-b-0 hover:bg-muted/30">
                        <td className="px-4 py-2.5">
                          <Checkbox
                            checked={selectedIds.has(r.id)}
                            onCheckedChange={() => toggleSelect(r.id)}
                          />
                        </td>
                        <td className="px-4 py-2.5 max-w-[12rem]">
                          <div className="font-medium truncate" title={r.title}>
                            {r.title}
                          </div>
                          {r.content && (
                            <div className="text-xs text-muted-foreground truncate" title={r.content}>
                              {r.content}
                            </div>
                          )}
                        </td>
                        <td className="px-4 py-2.5">
                          {TYPE_LABEL[r.schedule_type] ?? r.schedule_type}
                        </td>
                        <td className="px-4 py-2.5 text-xs text-muted-foreground max-w-[16rem]">
                          <span className="truncate block" title={formatReminderDetail(r)}>
                            {formatReminderDetail(r)}
                          </span>
                        </td>
                        <td className="px-4 py-2.5 text-xs whitespace-nowrap">
                          {r.enabled ? formatNextFire(r.next_fire_at) : '—'}
                        </td>
                        <td className="px-4 py-2.5 max-w-[10rem]">
                          {chNames.length === 0 ? (
                            <span className="text-xs text-muted-foreground">—</span>
                          ) : (
                            <span className="truncate block text-xs" title={chNames.join(', ')}>
                              {chNames.join(', ')}
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-2.5 text-center">
                          <Switch checked={r.enabled} onCheckedChange={() => handleToggle(r)} />
                        </td>
                        <td className="px-4 py-2.5 text-xs text-muted-foreground pl-16">
                          {SOURCE_LABEL[r.source] ?? r.source}
                        </td>
                        <td className="px-4 py-2.5 text-xs whitespace-nowrap text-muted-foreground">
                          {formatTime(r.created_at)}
                        </td>
                        <td className="px-4 py-2.5">
                          <div className="flex justify-end gap-0.5">
                            <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => handleDuplicate(r)} title="复制">
                              <Copy className="h-4 w-4" />
                            </Button>
                            <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => setEditing(r)} title="编辑">
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              size="icon"
                              variant="ghost"
                              className="h-8 w-8 text-destructive hover:text-destructive"
                              onClick={() => setToDelete(r)}
                              title="删除"
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {/* 移动端卡片列表 */}
          <div className="divide-y md:hidden">
            {items.map((r) => {
              const chNames = r.channel_ids.map((cid) => channelMap.get(cid) || `#${cid}`)
              return (
                <div key={r.id} className="px-4 py-3 space-y-2">
                  <div className="flex items-start justify-between gap-2">
                    <Checkbox
                      checked={selectedIds.has(r.id)}
                      onCheckedChange={() => toggleSelect(r.id)}
                      className="mt-1"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="font-medium truncate" title={r.title}>{r.title}</p>
                      {r.content && (
                        <p className="text-xs text-muted-foreground truncate mt-0.5" title={r.content}>
                          {r.content}
                        </p>
                      )}
                    </div>
                    <Switch checked={r.enabled} onCheckedChange={() => handleToggle(r)} />
                  </div>

                  <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
                    <div>
                      <span className="text-muted-foreground">类型</span>
                      <p className="mt-0.5">{TYPE_LABEL[r.schedule_type] ?? r.schedule_type}</p>
                    </div>
                    <div>
                      <span className="text-muted-foreground">来源</span>
                      <p className="mt-0.5">{SOURCE_LABEL[r.source] ?? r.source}</p>
                    </div>
                    <div className="col-span-2">
                      <span className="text-muted-foreground">详情</span>
                      <p className="truncate mt-0.5" title={formatReminderDetail(r)}>{formatReminderDetail(r)}</p>
                    </div>
                    <div className="col-span-2">
                      <span className="text-muted-foreground">下次触发</span>
                      <p className="mt-0.5">{r.enabled ? formatNextFire(r.next_fire_at) : '—'}</p>
                    </div>
                    <div className="col-span-2">
                      <span className="text-muted-foreground">通知渠道</span>
                      <p className="truncate mt-0.5" title={chNames.join(', ')}>
                        {chNames.length === 0 ? '—' : chNames.join(', ')}
                      </p>
                    </div>
                    <div className="col-span-2">
                      <span className="text-muted-foreground">创建时间</span>
                      <p className="mt-0.5">{formatTime(r.created_at)}</p>
                    </div>
                  </div>

                  <div className="flex justify-end gap-0.5 pt-1 border-t">
                    <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => handleDuplicate(r)} title="复制">
                      <Copy className="h-4 w-4" />
                    </Button>
                    <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => setEditing(r)} title="编辑">
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => setToDelete(r)}
                      title="删除"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              )
            })}
          </div>

          <Pagination total={total} limit={limit} offset={offset} onPageChange={setOffset} onLimitChange={setLimit} />
        </Card>
      )}

      {(creating || editing) && (
        <ReminderEditDialog
          reminder={editing}
          open
          onClose={() => {
            setCreating(false)
            setEditing(null)
          }}
          onSaved={() => {
            refresh()
            setCreating(false)
            setEditing(null)
          }}
        />
      )}

      <ConfirmDialog
        open={Boolean(toDelete)}
        title="删除提醒"
        description={toDelete ? `确认删除提醒「${toDelete.title}」？该操作不可撤销。` : ''}
        confirmText="删除"
        destructive
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setToDelete(null)}
      />

      <ConfirmDialog
        open={batchConfirmOpen}
        title="批量删除提醒"
        description={`确认删除选中的 ${selectedIds.size} 条提醒？该操作不可撤销。`}
        confirmText="删除"
        destructive
        loading={batchDeleting}
        onConfirm={() => {
          handleBatchDelete()
          setBatchConfirmOpen(false)
        }}
        onCancel={() => setBatchConfirmOpen(false)}
      />
    </div>
  )
}
