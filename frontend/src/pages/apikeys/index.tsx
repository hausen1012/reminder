// API Key 管理页：列表 + 搜索 + 分页 + 新建对话框 + 详情编辑 + 明文展示
import { useEffect, useMemo, useState } from 'react'
import { Plus, Trash2, Copy, CheckCircle2, Pencil, Eye, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Pagination } from '@/components/ui/pagination'
import { ChannelMultiSelect } from '@/components/channels/ChannelMultiSelect'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import {
  listApiKeys,
  createApiKey,
  toggleApiKey,
  deleteApiKey,
  listChannels,
  getApiKey,
  getApiKeyPlaintext,
  updateApiKeyChannels,
} from '@/lib/api'
import type { APIKey, Channel } from '@/types'

function formatChannelNames(channels: Channel[], ids: number[]): string {
  if (ids.length === 0) return '未设置'
  return ids.map((id) => channels.find((c) => c.id === id)?.name ?? `#${id}`).join(', ')
}

function formatRecentTime(dateStr?: string): string {
  return dateStr ? new Date(dateStr).toLocaleString() : '—'
}

export default function ApiKeysPage() {
  const [items, setItems] = useState<APIKey[]>([])
  const [channels, setChannels] = useState<Channel[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [newKeyName, setNewKeyName] = useState('')
  const [newChannelIDs, setNewChannelIDs] = useState<number[]>([])
  const [newChannelsOpen, setNewChannelsOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createdResult, setCreatedResult] = useState<{ plaintext: string; name: string } | null>(null)
  const [toDelete, setToDelete] = useState<APIKey | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailSaving, setDetailSaving] = useState(false)
  const [detailChannelsOpen, setDetailChannelsOpen] = useState(false)
  const [selectedKey, setSelectedKey] = useState<APIKey | null>(null)
  const [detailChannelIDs, setDetailChannelIDs] = useState<number[]>([])
  const [detailPlaintext, setDetailPlaintext] = useState('')
  const [plaintextLoading, setPlaintextLoading] = useState(false)
  const [enabled, setEnabled] = useState<string>('all')
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(10)
  const [offset, setOffset] = useState(0)
  const { toast } = useToast()

  async function refresh() {
    setLoading(true)
    try {
      const [list, allChannels] = await Promise.all([listApiKeys(), listChannels()])
      setItems(list)
      setChannels(allChannels)
    } catch (err) {
      toast({ title: '加载 API Key 失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  const filteredItems = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return items.filter((item) => {
      if (enabled === 'true' && !item.enabled) return false
      if (enabled === 'false' && item.enabled) return false
      if (!keyword) return true
      return item.name.toLowerCase().includes(keyword)
    })
  }, [items, enabled, search])

  const pagedItems = useMemo(() => {
    return filteredItems.slice(offset, offset + limit)
  }, [filteredItems, offset, limit])

  useEffect(() => {
    setOffset(0)
  }, [enabled, search, limit])

  function resetCreateDialog() {
    setCreateOpen(false)
    setCreatedResult(null)
    setNewKeyName('')
    setNewChannelIDs([])
    setNewChannelsOpen(false)
  }

  async function handleCreate() {
    const name = newKeyName.trim()
    if (!name) {
      toast({ title: '名称必填', variant: 'destructive' })
      return
    }
    setCreating(true)
    try {
      const result = await createApiKey(name, newChannelIDs)
      setCreatedResult({ plaintext: result.plaintext, name: result.key.name })
      setNewKeyName('')
      setNewChannelIDs([])
      await refresh()
    } catch (err) {
      toast({ title: '创建失败', description: String(err), variant: 'destructive' })
    } finally {
      setCreating(false)
    }
  }

  async function handleToggle(key: APIKey) {
    try {
      const next = await toggleApiKey(key.id)
      setItems((prev) => prev.map((it) => (it.id === key.id ? { ...it, enabled: next.enabled } : it)))
      if (selectedKey?.id === key.id) {
        setSelectedKey((prev) => (prev ? { ...prev, enabled: next.enabled } : prev))
      }
    } catch (err) {
      toast({ title: '切换状态失败', description: String(err), variant: 'destructive' })
    }
  }

  async function handleDelete() {
    if (!toDelete) return
    setDeleting(true)
    try {
      await deleteApiKey(toDelete.id)
      setItems((prev) => prev.filter((it) => it.id !== toDelete.id))
      toast({ title: 'API Key 已删除', variant: 'success' })
      if (selectedKey?.id === toDelete.id) {
        setDetailOpen(false)
        setSelectedKey(null)
        setDetailPlaintext('')
      }
    } catch (err) {
      toast({ title: '删除失败', description: String(err), variant: 'destructive' })
    } finally {
      setDeleting(false)
      setToDelete(null)
    }
  }

  async function openDetail(key: APIKey) {
    setDetailOpen(true)
    setDetailLoading(true)
    setDetailChannelsOpen(false)
    setDetailPlaintext('')
    try {
      const detail = await getApiKey(key.id)
      setSelectedKey(detail)
      setDetailChannelIDs(detail.default_channel_ids ?? [])
    } catch (err) {
      toast({ title: '加载详情失败', description: String(err), variant: 'destructive' })
      setDetailOpen(false)
    } finally {
      setDetailLoading(false)
    }
  }

  function closeDetail() {
    setDetailOpen(false)
    setDetailLoading(false)
    setDetailSaving(false)
    setDetailChannelsOpen(false)
    setSelectedKey(null)
    setDetailChannelIDs([])
    setDetailPlaintext('')
    setPlaintextLoading(false)
  }

  async function handleSaveDetail() {
    if (!selectedKey) return
    setDetailSaving(true)
    try {
      await updateApiKeyChannels(selectedKey.id, detailChannelIDs)
      const next = { ...selectedKey, default_channel_ids: detailChannelIDs }
      setSelectedKey(next)
      setItems((prev) => prev.map((it) => (it.id === next.id ? next : it)))
      toast({ title: '默认通道已更新', variant: 'success' })
    } catch (err) {
      toast({ title: '保存失败', description: String(err), variant: 'destructive' })
    } finally {
      setDetailSaving(false)
    }
  }

  async function handleLoadPlaintext() {
    if (!selectedKey) return
    setPlaintextLoading(true)
    try {
      const plaintext = await getApiKeyPlaintext(selectedKey.id)
      setDetailPlaintext(plaintext)
    } catch (err) {
      toast({ title: '查看明文失败', description: String(err), variant: 'destructive' })
    } finally {
      setPlaintextLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold tracking-tight">API</h1>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="mr-1 h-4 w-4" />
          新建 Key
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="w-40">
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
        <form
          className="max-w-md flex-1"
          onSubmit={(e) => {
            e.preventDefault()
            setSearch(searchInput)
          }}
        >
          <Input
            placeholder="搜索名称…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
        </form>
        <span className="text-sm text-muted-foreground">共 {filteredItems.length} 条</span>
        <Button variant="outline" size="icon" onClick={refresh} title="刷新">
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : filteredItems.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {search.trim() ? '没有匹配的 API Key。' : '还没有 API Key。创建后可通过 Ingest API 外部调用创建提醒。'}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <div className="overflow-x-auto">
            <table className="w-full text-[13px] table-fixed">
              <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-4 py-2.5 w-[14rem]">名称</th>
                  <th className="px-4 py-2.5 w-[14rem]">通道</th>
                  <th className="px-4 py-2.5 w-16 text-center">启用</th>
                  <th className="px-4 py-2.5 w-44">最近使用</th>
                  <th className="px-4 py-2.5 w-44">创建时间</th>
                  <th className="px-4 py-2.5 w-36 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {pagedItems.map((key) => (
                  <tr key={key.id} className="border-b last:border-b-0 hover:bg-muted/30">
                    <td className="max-w-[14rem] px-4 py-2.5">
                      <div className="truncate font-medium" title={key.name}>
                        {key.name}
                      </div>
                    </td>
                    <td className="max-w-[14rem] px-4 py-2.5 text-xs text-muted-foreground">
                      <span className="block truncate" title={formatChannelNames(channels, key.default_channel_ids)}>
                        {formatChannelNames(channels, key.default_channel_ids)}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-center">
                      <Switch checked={key.enabled} onCheckedChange={() => handleToggle(key)} />
                    </td>
                    <td className="whitespace-nowrap px-4 py-2.5 text-xs text-muted-foreground">
                      {formatRecentTime(key.last_used_at)}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2.5 text-xs text-muted-foreground">
                      {new Date(key.created_at).toLocaleString()}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex justify-end gap-0.5">
                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openDetail(key)} title="查看与编辑">
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive hover:text-destructive" onClick={() => setToDelete(key)} title="删除">
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination total={filteredItems.length} limit={limit} offset={offset} onPageChange={setOffset} onLimitChange={setLimit} />
        </Card>
      )}

      <Dialog open={createOpen && !createdResult} onOpenChange={(open) => !open && resetCreateDialog()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新建 API Key</DialogTitle>
            <DialogDescription>
              创建后密钥会立即展示，之后默认隐藏，可在详情里按需再次查看。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="ak-name">名称</Label>
              <Input
                id="ak-name"
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
                placeholder="例如：生产环境"
                maxLength={64}
              />
            </div>
            <div className="space-y-2">
              <Label>默认通知渠道</Label>
              <ChannelMultiSelect
                channels={channels}
                value={newChannelIDs}
                open={newChannelsOpen}
                onOpenChange={setNewChannelsOpen}
                onChange={setNewChannelIDs}
                placeholder="未设置默认通道"
                emptyText="还没有通道，先到「通知」页面创建一个。"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={resetCreateDialog}>
              取消
            </Button>
            <Button onClick={handleCreate} disabled={creating}>
              {creating ? '创建中…' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(createdResult)} onOpenChange={(open) => !open && resetCreateDialog()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>密钥已创建</DialogTitle>
            <DialogDescription>
              请立即复制并安全保存。详情里默认隐藏明文，点击按钮后才会再次显示。
            </DialogDescription>
          </DialogHeader>
          {createdResult && (
            <div className="space-y-3">
              <div className="rounded-md border bg-muted p-3">
                <div className="mb-1 text-xs text-muted-foreground">{createdResult.name}</div>
                <div className="flex items-start gap-2">
                  <code className="min-w-0 flex-1 select-all break-all text-sm">{createdResult.plaintext}</code>
                  <button
                    type="button"
                    onClick={() => {
                      navigator.clipboard.writeText(createdResult.plaintext)
                      toast({ title: '密钥已复制' })
                    }}
                    className="rounded p-1 text-muted-foreground transition-colors hover:bg-background hover:text-foreground"
                    title="复制密钥"
                  >
                    <Copy className="h-4 w-4" />
                  </button>
                </div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button onClick={resetCreateDialog}>
              <CheckCircle2 className="mr-1 h-4 w-4" />
              我已保存，关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={detailOpen} onOpenChange={(open) => !open && closeDetail()}>
        <DialogContent className="overflow-visible">
          <DialogHeader>
            <DialogTitle>API Key 详情</DialogTitle>
            <DialogDescription>
              可查看密钥信息、重复查看明文，并编辑默认通知渠道。
            </DialogDescription>
          </DialogHeader>
          {detailLoading || !selectedKey ? (
            <p className="text-sm text-muted-foreground">加载中…</p>
          ) : (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">名称</p>
                  <p className="font-medium">{selectedKey.name}</p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">状态</p>
                  <p>{selectedKey.enabled ? '已启用' : '已禁用'}</p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">最近使用</p>
                  <p>{formatRecentTime(selectedKey.last_used_at)}</p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">创建时间</p>
                  <p>{new Date(selectedKey.created_at).toLocaleString()}</p>
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <Label>明文 Key</Label>
                  <Button variant="outline" size="sm" onClick={handleLoadPlaintext} disabled={plaintextLoading}>
                    <Eye className="mr-1 h-4 w-4" />
                    {plaintextLoading ? '读取中…' : detailPlaintext ? '重新查看' : '查看明文'}
                  </Button>
                </div>
                {detailPlaintext ? (
                  <div className="rounded-md border bg-muted p-3">
                    <div className="flex items-start gap-2">
                      <code className="min-w-0 flex-1 select-all break-all text-sm">{detailPlaintext}</code>
                      <button
                        type="button"
                        onClick={() => {
                          navigator.clipboard.writeText(detailPlaintext)
                          toast({ title: '密钥已复制' })
                        }}
                        className="rounded p-1 text-muted-foreground transition-colors hover:bg-background hover:text-foreground"
                        title="复制密钥"
                      >
                        <Copy className="h-4 w-4" />
                      </button>
                    </div>
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">点击按钮后按需读取并展示明文。</p>
                )}
              </div>

              <div className="space-y-2">
                <Label>默认通知渠道</Label>
                <ChannelMultiSelect
                  channels={channels}
                  value={detailChannelIDs}
                  open={detailChannelsOpen}
                  onOpenChange={setDetailChannelsOpen}
                  onChange={setDetailChannelIDs}
                  placeholder="未设置默认通道"
                  emptyText="还没有通道，先到「通知」页面创建一个。"
                />
                <p className="text-xs text-muted-foreground">
                  当外部调用未显式传入 <code>channel_ids</code> 时，会回退到这里配置的默认通道。
                </p>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={closeDetail}>
              关闭
            </Button>
            <Button onClick={handleSaveDetail} disabled={detailLoading || !selectedKey || detailSaving}>
              {detailSaving ? '保存中…' : '保存默认通道'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(toDelete)} onOpenChange={(open) => !open && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除 API Key</DialogTitle>
            <DialogDescription>
              确认删除 Key「{toDelete?.name}」？使用该 Key 的所有外部调用将立即失败。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)} disabled={deleting}>
              取消
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting ? '删除中…' : '确认删除'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
