// 通知页：列表 + 新建/编辑对话框 + 发送测试 + 删除
import { useCallback, useEffect, useState } from 'react'
import { Plus, Pencil, Trash2, Copy, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Pagination } from '@/components/ui/pagination'
import { useToast } from '@/components/ui/use-toast'
import { formatTime } from '@/lib/utils'
import { SortIcon } from '@/components/ui/SortIcon'
import { PageHeader } from '@/components/ui/PageHeader'
import { FilterToolbar } from '@/components/ui/FilterToolbar'
import { ChannelEditDialog } from '@/components/channels/ChannelEditDialog'
import { ConfirmDialog } from '@/components/channels/ConfirmDialog'
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
      setLoadError(String(err))
      toast({ title: '加载通知失败', description: String(err), variant: 'destructive' })
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
      toast({ title: '通知已删除', variant: 'success' })
    } catch (err) {
      toast({ title: '删除失败', description: String(err), variant: 'destructive' })
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
      toast({ title: '通知已复制', variant: 'success' })
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
      setSelectedIds(new Set(items.map((ch) => ch.id)))
    }
  }

  async function handleBatchDelete() {
    setBatchDeleting(true)
    try {
      await batchDeleteChannels(Array.from(selectedIds))
      toast({ title: `已删除 ${selectedIds.size} 条通知`, variant: 'success' })
      setSelectedIds(new Set())
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

  return (
    <div className="space-y-6">
      <PageHeader title="通知">
        <div className="flex items-center gap-2">
          {selectedIds.size > 0 && (
            <Button size="sm" variant="destructive" onClick={() => setBatchConfirmOpen(true)}>
              <Trash2 className="h-4 w-4 mr-1" />
              删除选中 ({selectedIds.size})
            </Button>
          )}
          <Button size="sm" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4 mr-1" />
            新建通知
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
        placeholder="搜索通知名称…"
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
            还没有通知，点击右上角新建一个吧。
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
                    <th className="px-4 py-2.5 w-[14rem]">名称</th>
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
                          <span className="font-medium truncate" title={ch.name}>{ch.name}</span>
                        </td>
                        <td className="px-4 py-2.5 text-xs text-muted-foreground">{TYPE_LABEL[ch.type]}</td>
                        <td className="px-4 py-2.5 text-xs whitespace-nowrap text-muted-foreground">
                          {formatTime(ch.created_at)}
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

          {/* 移动端卡片列表 */}
          <div className="divide-y md:hidden">
            {items.map((ch) => (
              <div key={ch.id} className="px-4 py-3 space-y-2">
                <div className="flex items-start justify-between gap-2">
                  <Checkbox
                    checked={selectedIds.has(ch.id)}
                    onCheckedChange={() => toggleSelect(ch.id)}
                    className="mt-1"
                  />
                  <div className="min-w-0 flex-1">
                    <p className="font-medium truncate" title={ch.name}>{ch.name}</p>
                    <p className="text-xs text-muted-foreground mt-0.5">{TYPE_LABEL[ch.type]}</p>
                  </div>
                  <div className="flex gap-0.5 shrink-0">
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
                </div>
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>创建于 {formatTime(ch.created_at)}</span>
                  <span className={ch.enabled ? 'text-green-600' : 'text-muted-foreground'}>
                    {ch.enabled ? '已启用' : '已禁用'}
                  </span>
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
