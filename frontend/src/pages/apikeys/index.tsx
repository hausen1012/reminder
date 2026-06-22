// API Key 管理页
import { useEffect, useMemo, useState } from 'react'
import { Plus, Trash2, Copy, CheckCircle2, Pencil, RefreshCw, Terminal, Search } from 'lucide-react'
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
import { CurlUsageDialog } from '@/components/apikeys/CurlUsageDialog'
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
  const [enabled, setEnabled] = useState<string>('all')
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(10)
  const [offset, setOffset] = useState(0)
  const [copyingId, setCopyingId] = useState<number | null>(null)
  const [curlKeyId, setCurlKeyId] = useState<number | null>(null)
  const [curlPlaintext, setCurlPlaintext] = useState('')
  const [curlLoading, setCurlLoading] = useState(false)
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
    setDetailPlaintext('')
    try {
      const [detail, plaintext] = await Promise.all([
        getApiKey(key.id),
        getApiKeyPlaintext(key.id),
      ])
      setSelectedKey(detail)
      setDetailChannelIDs(detail.default_channel_ids ?? [])
      setDetailPlaintext(plaintext)
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
  }

  async function handleSaveDetail() {
    if (!selectedKey) return
    setDetailSaving(true)
    try {
      await updateApiKeyChannels(selectedKey.id, detailChannelIDs)
      const next = { ...selectedKey, default_channel_ids: detailChannelIDs }
      setSelectedKey(next)
      setItems((prev) => prev.map((it) => (it.id === next.id ? next : it)))
      toast({ title: '默认通知渠道已更新', variant: 'success' })
    } catch (err) {
      toast({ title: '保存失败', description: String(err), variant: 'destructive' })
    } finally {
      setDetailSaving(false)
    }
  }

  async function openCurl(key: APIKey) {
    setCurlKeyId(key.id)
    setCurlPlaintext('')
    setCurlLoading(true)
    try {
      const plaintext = await getApiKeyPlaintext(key.id)
      setCurlPlaintext(plaintext)
    } catch (err) {
      toast({ title: '加载密钥失败', description: String(err), variant: 'destructive' })
      setCurlKeyId(null)
    } finally {
      setCurlLoading(false)
    }
  }

  function closeCurl() {
    setCurlKeyId(null)
    setCurlPlaintext('')
  }

  async function handleCopyFromList(keyId: number) {
    setCopyingId(keyId)
    try {
      const plaintext = await getApiKeyPlaintext(keyId)
      await navigator.clipboard.writeText(plaintext)
      toast({ title: '密钥已复制' })
    } catch (err) {
      toast({ title: '复制失败', description: String(err), variant: 'destructive' })
    } finally {
      setCopyingId(null)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex-col items-start gap-2 md:flex-row md:items-center md:justify-between flex">
        <h1 className="text-3xl font-bold tracking-tight">API</h1>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="mr-1 h-4 w-4" />
          新建 Key
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-2">
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
        <form
          className="flex-1 min-w-0 flex gap-2 md:max-w-sm"
          onSubmit={(e) => {
            e.preventDefault()
            setSearch(searchInput)
          }}
        >
          <Input
            placeholder="搜索名称…"
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
      ) : filteredItems.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {search.trim() ? '没有匹配的 API Key。' : '还没有 API Key。'}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <div className="overflow-x-auto">
            <table className="w-full text-[13px] table-fixed">
              <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-4 py-2.5 w-[14rem]">名称</th>
                  <th className="px-4 py-2.5 w-[10rem]">密钥</th>
                  <th className="px-4 py-2.5 w-[10rem]">通知</th>
                  <th className="px-4 py-2.5 w-16 text-center">启用</th>
                  <th className="px-4 py-2.5 w-44">最近使用</th>
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
                    <td className="max-w-[15rem] px-4 py-2.5">
                      <div className="flex items-center gap-1.5">
                        <code className="truncate text-xs text-muted-foreground">{key.prefix}...</code>
                        <button
                          type="button"
                          onClick={() => handleCopyFromList(key.id)}
                          disabled={copyingId === key.id}
                          className="shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
                          title="复制完整密钥"
                        >
                          <Copy className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </td>
                    <td className="max-w-[10rem] px-4 py-2.5 text-xs text-muted-foreground">
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
                    <td className="px-4 py-2.5">
                      <div className="flex justify-end gap-0.5">
                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openDetail(key)} title="查看与编辑">
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openCurl(key)} title="curl 使用示例">
                          <Terminal className="h-4 w-4" />
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
            <DialogDescription>创建后密钥会立即展示，之后可在详情里查看。</DialogDescription>
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
                placeholder="未设置默认通知渠道"
                emptyText="还没有通知渠道，先到「通知」页面创建一个。"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={resetCreateDialog}>取消</Button>
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
            <DialogDescription>请立即复制并安全保存。</DialogDescription>
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
          </DialogHeader>
          {detailLoading || !selectedKey ? (
            <p className="text-sm text-muted-foreground">加载中…</p>
          ) : (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>明文 Key</Label>
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
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
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
                <Label>默认通知渠道</Label>
                <ChannelMultiSelect
                  channels={channels}
                  value={detailChannelIDs}
                  open={detailChannelsOpen}
                  onOpenChange={setDetailChannelsOpen}
                  onChange={setDetailChannelIDs}
                  placeholder="未设置默认通知渠道"
                  emptyText="还没有通知渠道，先到「通知」页面创建一个。"
                />
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={closeDetail}>关闭</Button>
            <Button onClick={handleSaveDetail} disabled={detailLoading || !selectedKey || detailSaving}>
              {detailSaving ? '保存中…' : '保存'}
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
            <Button variant="outline" onClick={() => setToDelete(null)} disabled={deleting}>取消</Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting ? '删除中…' : '确认删除'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CurlUsageDialog
        apiKey={curlPlaintext}
        open={curlKeyId !== null && !curlLoading}
        onClose={closeCurl}
      />
    </div>
  )
}