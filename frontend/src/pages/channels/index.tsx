// 通知页：列表 + 新建/编辑对话框 + 发送测试 + 删除
import { useCallback, useEffect, useState } from 'react'
import { Plus, Pencil, Trash2, Copy, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Pagination } from '@/components/common/pagination'
import { useToast } from '@/components/ui/use-toast'
import { formatInZone } from '@/lib/datetime'
import { useSiteTimezone } from '@/contexts/ConfigContext'
import { extractApiError } from '@/lib/api'
import { SortIcon } from '@/components/common/SortIcon'
import { PageHeader } from '@/components/common/PageHeader'
import { FilterToolbar } from '@/components/common/FilterToolbar'
import { ChannelEditDialog } from '@/components/channels/ChannelEditDialog'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import {
  listChannelsPaged,
  deleteChannel,
  createChannel,
  batchDeleteChannels,
} from '@/lib/api'
import type { Channel, ChannelType } from '@/types'

const TYPE_LABEL: Record<ChannelType, string> = {
  smtp: '邮件',
  dingtalk: '钉钉机器人',
  wecom: '企业微信机器人',
  webhook: 'WebHook',
  log: '控制台日志',
}

export default function ChannelsPage() {
  const tz = useSiteTimezone()
  const [items, setItems] = useState<Channel[]>([])
  const [total, setTotal] = useState(0)
  const [limit, setLimit] = useState(10)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchVersion, setSearchVersion] = useState(0)
  const [editing, setEditing] = useState<Channel | null>(null)
  const [creating, setCreating] = useState(false)
  const [toDelete, setToDelete] = useState<Channel | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [sortBy, setSortBy] = useState('created_at')
  const [sortOrder, setSortOrder] = useState('desc')
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [batchDeleting, setBatchDeleting] = useState(false)
  const [batchConfirmOpen, setBatchConfirmOpen] = useState(false)
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
      const data = await listChannelsPaged({
        search: search.trim() || undefined,
        limit,
        offset,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
      setItems(data?.items ?? [])
      setTotal(data?.total ?? 0)
      setLoadError('')
    } catch (err) {
      setLoadError(extractApiError(err, '加载失败'))
      toast({ title: '加载通知渠道失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }, [search, limit, offset, toast, sortBy, sortOrder])

  useEffect(() => {
    refresh()
  }, [refresh, searchVersion])

  async function handleDelete() {
    if (!toDelete) return
    setDeleting(true)
    try {
      await deleteChannel(toDelete.id)
      const nextTotal = Math.max(0, total - 1)
      const shouldGoPrevPage = items.length === 1 && offset > 0
      if (shouldGoPrevPage) {
        setTotal(nextTotal)
        setOffset(Math.max(0, offset - limit))
      } else {
        await refresh()
      }
      toast({ title: '通知渠道已删除', variant: 'success' })
    } catch (err) {
      toast({ title: '删除失败', description: extractApiError(err, '删除失败'), variant: 'destructive' })
    } finally {
      setDeleting(false)
      setToDelete(null)
    }
  }

  async function handleDuplicate(ch: Channel) {
    try {
      await createChannel({
        name: ch.name + '-copy',
        type: ch.type,
        config: ch.config,
        enabled: ch.enabled,
      })
      toast({ title: '通知渠道已复制', variant: 'success' })
      refresh()
    } catch (err) {
      toast({ title: '复制失败', description: extractApiError(err), variant: 'destructive' })
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
      setSelectedIds(new Set(items.map((ch) => ch.id)))
    }
  }

  async function handleBatchDelete() {
    setBatchDeleting(true)
    try {
      await batchDeleteChannels(Array.from(selectedIds))
      toast({ title: `已删除 ${selectedIds.size} 个通知渠道`, variant: 'success' })
      setSelectedIds(new Set())
      const remainAfterDelete = items.length - selectedIds.size
      if (remainAfterDelete === 0 && offset > 0) {
        setOffset(Math.max(0, offset - limit))
      } else {
        await refresh()
      }
    } catch (err) {
      toast({ title: '批量删除失败', description: extractApiError(err, '批量删除失败'), variant: 'destructive' })
    } finally {
      setBatchDeleting(false)
    }
  }

  return (
    <div className="space-y-4 md:space-y-6">
      <PageHeader title="通知渠道">
        <div className="flex flex-wrap items-center gap-2">
          {selectedIds.size > 0 && (
            <Button size="sm" variant="destructive" className="flex-1 md:flex-none" onClick={() => setBatchConfirmOpen(true)}>
              <Trash2 className="h-4 w-4 mr-1" />
              删除选中 ({selectedIds.size})
            </Button>
          )}
          <Button size="sm" className="flex-1 md:flex-none" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4 mr-1" />
            新建渠道
          </Button>
        </div>
      </PageHeader>

      <FilterToolbar
        searchValue={searchInput}
        onSearchChange={setSearchInput}
        onSearch={() => {
          setOffset(0)
          setSearch(searchInput)
          setSearchVersion((v) => v + 1)
        }}
        onRefresh={refresh}
        placeholder="搜索渠道名称…"
      />

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
            还没有通知渠道，点击上方「新建渠道」创建。
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
                    <th className="px-4 py-2.5">名称</th>
                    <th className="px-4 py-2.5 w-28">类型</th>
                    <th className="px-4 py-2.5 w-44 whitespace-nowrap cursor-pointer select-none" onClick={() => toggleSort('created_at')}>
                      创建时间<SortIcon active={sortBy === 'created_at'} direction={sortOrder} />
                    </th>
                    <th className="px-4 py-2.5 w-36 text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((ch) => {
                    return (
                      <tr key={ch.id} className="border-b last:border-b-0 hover:bg-muted/30">
                        <td className="px-4 py-2.5">
                          <Checkbox
                            checked={selectedIds.has(ch.id)}
                            onCheckedChange={() => toggleSelect(ch.id)}
                          />
                        </td>
                        <td className="px-4 py-2.5">
                          <span className="font-medium truncate block" title={ch.name}>{ch.name}</span>
                        </td>
                        <td className="px-4 py-2.5 text-xs text-muted-foreground">{TYPE_LABEL[ch.type]}</td>
                        <td className="px-4 py-2.5 text-xs whitespace-nowrap text-muted-foreground">
                          {formatInZone(ch.created_at, tz)}
                        </td>
                        <td className="px-4 py-2.5">
                          <div className="flex justify-end gap-0.5">
                            <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => handleDuplicate(ch)} title="复制">
                              <Copy className="h-4 w-4" />
                            </Button>
                            <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => setEditing(ch)} title="编辑">
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button size="icon" variant="ghost" className="h-8 w-8 text-destructive hover:text-destructive" onClick={() => setToDelete(ch)} title="删除">
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

          {/* 移动端卡片列表：名称 + 类型/创建时间两行，操作按钮收进底部一行 */}
          <div className="divide-y md:hidden">
            {items.map((ch) => (
              <div key={ch.id} className="px-3 py-2.5">
                <div className="flex items-center gap-2.5">
                  <Checkbox
                    checked={selectedIds.has(ch.id)}
                    onCheckedChange={() => toggleSelect(ch.id)}
                    className="shrink-0"
                    aria-label={`选择渠道 ${ch.name}`}
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium" title={ch.name}>{ch.name}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {TYPE_LABEL[ch.type]} · 创建于 {formatInZone(ch.created_at, tz)}
                    </p>
                  </div>
                  <span className={ch.enabled ? 'shrink-0 text-xs text-green-600' : 'shrink-0 text-xs text-muted-foreground'}>
                    {ch.enabled ? '已启用' : '已禁用'}
                  </span>
                </div>

                <div className="mt-1 flex justify-end gap-1 pl-6">
                  <Button size="sm" variant="ghost" className="h-8 px-2 text-xs" onClick={() => handleDuplicate(ch)}>
                    <Copy className="h-3.5 w-3.5 mr-1" />
                    复制
                  </Button>
                  <Button size="sm" variant="ghost" className="h-8 px-2 text-xs" onClick={() => setEditing(ch)}>
                    <Pencil className="h-3.5 w-3.5 mr-1" />
                    编辑
                  </Button>
                  <Button size="sm" variant="ghost" className="h-8 px-2 text-xs text-destructive hover:text-destructive" onClick={() => setToDelete(ch)}>
                    <Trash2 className="h-3.5 w-3.5 mr-1" />
                    删除
                  </Button>
                </div>
              </div>
            ))}
          </div>

          <Pagination total={total} limit={limit} offset={offset} onPageChange={setOffset} onLimitChange={setLimit} />
        </Card>
      )}

      {(creating || editing) && (
        <ChannelEditDialog
          channel={editing}
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
        title="删除通知"
        description={toDelete ? `确认删除通知「${toDelete.name}」？该操作不可撤销。` : ''}
        confirmText="删除"
        destructive
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setToDelete(null)}
      />

      <ConfirmDialog
        open={batchConfirmOpen}
        title="批量删除通知"
        description={`确认删除选中的 ${selectedIds.size} 条通知？该操作不可撤销。`}
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
