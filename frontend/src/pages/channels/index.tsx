// 通道页：列表 + 新建/编辑对话框 + 试发 + 删除
import { useCallback, useEffect, useState } from 'react'
import { Plus, Pencil, RefreshCw, Trash2, TestTube, Copy, ArrowUpDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Pagination } from '@/components/ui/pagination'
import { useToast } from '@/components/ui/use-toast'
import { ChannelEditDialog } from '@/components/channels/ChannelEditDialog'
import { ConfirmDialog } from '@/components/channels/ConfirmDialog'
import {
  listChannelsPaged,
  deleteChannel,
  testChannel,
  createChannel,
} from '@/lib/api'
import type { Channel, ChannelType } from '@/types'

function formatTime(dateStr?: string): string {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return '—'
  const y = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${y}/${month}/${day} ${h}:${min}`
}

function SortIcon({ active, direction }: { active: boolean; direction: string }) {
  return <ArrowUpDown className={`inline h-3 w-3 ml-0.5 ${active ? 'text-foreground' : 'text-muted-foreground/40'}`} style={active && direction === 'asc' ? { transform: 'rotate(180deg)' } : undefined} />
}

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
  const [testingId, setTestingId] = useState<number | null>(null)
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
      toast({ title: '加载通道失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }, [search, limit, offset, toast, sortBy, sortOrder])

  useEffect(() => {
    refresh()
  }, [refresh, searchVersion])

  async function handleDelete() {
    if (!toDelete) return
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
      toast({ title: '通道已删除', variant: 'success' })
    } catch (err) {
      toast({ title: '删除失败', description: String(err), variant: 'destructive' })
    } finally {
      setToDelete(null)
    }
  }

  async function handleTest(ch: Channel) {
    setTestingId(ch.id)
    try {
      const result = await testChannel(ch.id)
      if (result.success) {
        toast({ title: '试发成功', description: `通道 ${ch.name} 已成功发送`, variant: 'success' })
      } else {
        toast({ title: '试发失败', description: result.error ?? '未知错误', variant: 'destructive' })
      }
    } catch (err) {
      toast({ title: '试发请求异常', description: String(err), variant: 'destructive' })
    } finally {
      setTestingId(null)
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
      toast({ title: '通道已复制', variant: 'success' })
      refresh()
    } catch (err) {
      toast({ title: '复制失败', description: String(err), variant: 'destructive' })
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold tracking-tight">通知</h1>
        <Button onClick={() => setCreating(true)}>
          <Plus className="h-4 w-4 mr-1" />
          新建通道
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <form
          className="flex-1 max-w-md"
          onSubmit={(e) => {
            e.preventDefault()
            setOffset(0)
            setSearch(searchInput)
            setSearchVersion((v) => v + 1)
          }}
        >
          <Input
            placeholder="搜索通道名称…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
        </form>
        <span className="text-sm text-muted-foreground">共 {total} 条</span>
        <Button variant="outline" size="icon" onClick={refresh} title="刷新">
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : items.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            还没有通道，点击右上角新建一个吧。
          </CardContent>
        </Card>
      ) : (
        <Card>
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
                          <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => handleTest(ch)} disabled={testingId === ch.id} title="试发">
                            <TestTube className="h-4 w-4" />
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
        title="删除通道"
        description={toDelete ? `确认删除通道「${toDelete.name}」？该操作不可撤销。` : ''}
        confirmText="删除"
        destructive
        onConfirm={handleDelete}
        onCancel={() => setToDelete(null)}
      />
    </div>
  )
}
