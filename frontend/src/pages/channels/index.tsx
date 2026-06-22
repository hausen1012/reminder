// 通知页：列表 + 新建/编辑对话框 + 发送测试 + 删除
import { useCallback, useEffect, useState } from 'react'
import { Plus, Pencil, RefreshCw, Trash2, Copy, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Pagination } from '@/components/ui/pagination'
import { useToast } from '@/components/ui/use-toast'
import { formatTime } from '@/lib/utils'
import { SortIcon } from '@/components/ui/SortIcon'
import { ChannelEditDialog } from '@/components/channels/ChannelEditDialog'
import { ConfirmDialog } from '@/components/channels/ConfirmDialog'
import {
  listChannelsPaged,
  deleteChannel,
  createChannel,
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
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchVersion, setSearchVersion] = useState(0)
  const [editing, setEditing] = useState<Channel | null>(null)
  const [creating, setCreating] = useState(false)
  const [toDelete, setToDelete] = useState<Channel | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [sortBy, setSortBy] = useState('created_at')
  const [sortOrder, setSortOrder] = useState('desc')
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
    } catch (err) {
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

  return (
    <div className="space-y-6">
      <div className="flex-col items-start gap-2 md:flex-row md:items-center md:justify-between flex">
        <h1 className="text-3xl font-bold tracking-tight">通知</h1>
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus className="h-4 w-4 mr-1" />
          新建通知
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <form
          className="flex-1 min-w-0 flex gap-2 md:max-w-sm"
          onSubmit={(e) => {
            e.preventDefault()
            setOffset(0)
            setSearch(searchInput)
            setSearchVersion((v) => v + 1)
          }}
        >
          <Input
            placeholder="搜索通知名称…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="flex-1 min-w-0"
          />
          <Button type="submit" variant="outline" size="icon" title="搜索">
            <Search className="h-4 w-4" />
          </Button>
        </form>
        <Button variant="outline" size="icon" onClick={refresh} title="刷新" className="ml-auto shrink-0">
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
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
    </div>
  )
}
