// API Key 管理页：列表 + 新建对话框 + 详情编辑 + 明文展示
import { useEffect, useState } from 'react'
import { Key, Plus, Trash2, Copy, CheckCircle2, Pencil, Eye } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ChannelMultiSelect } from '@/components/channels/ChannelMultiSelect'
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
        <div>
          <h1 className="text-3xl font-bold tracking-tight">API</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            管理外部 API 调用的密钥，可配置默认通知渠道；明文默认隐藏，点击时再查看。
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="mr-1 h-4 w-4" />
          新建 Key
        </Button>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : items.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            还没有 API Key。创建后可通过 Ingest API 外部调用创建提醒。
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {items.map((key) => (
            <Card key={key.id}>
              <CardContent className="flex items-center gap-4 py-4">
                <Key className="h-5 w-5 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium">{key.name}</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{key.prefix}...</code>
                    <Badge variant="outline" className="text-xs">{key.usage_24h}/24h</Badge>
                  </div>
                  <div className="mt-0.5 text-xs text-muted-foreground">
                    创建于 {new Date(key.created_at).toLocaleDateString()}
                    {key.last_used_at && ` · 最近使用 ${new Date(key.last_used_at).toLocaleDateString()}`}
                    {!key.enabled && <span> · 已禁用</span>}
                  </div>
                  <div className="mt-1 truncate text-xs text-muted-foreground" title={formatChannelNames(channels, key.default_channel_ids)}>
                    默认通道：{formatChannelNames(channels, key.default_channel_ids)}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Switch checked={key.enabled} onCheckedChange={() => handleToggle(key)} />
                  <Button variant="ghost" size="icon" onClick={() => openDetail(key)} title="查看与编辑">
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button variant="ghost" size="icon" onClick={() => setToDelete(key)} title="删除">
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
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
                <code className="select-all break-all text-sm">{createdResult.plaintext}</code>
              </div>
              <Button
                className="w-full"
                variant="outline"
                onClick={() => {
                  navigator.clipboard.writeText(createdResult.plaintext)
                  toast({ title: '密钥已复制' })
                }}
              >
                <Copy className="mr-1 h-4 w-4" />
                复制密钥
              </Button>
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
                  <p className="text-xs text-muted-foreground">前缀</p>
                  <code className="rounded bg-muted px-2 py-1 text-xs">{selectedKey.prefix}...</code>
                </div>
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">状态</p>
                  <p>{selectedKey.enabled ? '已启用' : '已禁用'}</p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">近 24 小时调用</p>
                  <p>{selectedKey.usage_24h}</p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">创建时间</p>
                  <p>{new Date(selectedKey.created_at).toLocaleString()}</p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">最近使用</p>
                  <p>{selectedKey.last_used_at ? new Date(selectedKey.last_used_at).toLocaleString() : '—'}</p>
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
                  <div className="space-y-2 rounded-md border bg-muted p-3">
                    <code className="block select-all break-all text-sm">{detailPlaintext}</code>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        navigator.clipboard.writeText(detailPlaintext)
                        toast({ title: '密钥已复制' })
                      }}
                    >
                      <Copy className="mr-1 h-4 w-4" />
                      复制密钥
                    </Button>
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
