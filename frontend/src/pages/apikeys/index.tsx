// API Key 管理页：列表 + 新建对话框 + 明文展示 + 删除
import { useEffect, useState } from 'react'
import { Key, Plus, Trash2, Copy, CheckCircle2 } from 'lucide-react'
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
import { useToast } from '@/components/ui/use-toast'
import { listApiKeys, createApiKey, toggleApiKey, deleteApiKey } from '@/lib/api'
import type { APIKey } from '@/types'

export default function ApiKeysPage() {
  const [items, setItems] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [newKeyName, setNewKeyName] = useState('')
  const [creating, setCreating] = useState(false)
  const [createdResult, setCreatedResult] = useState<{ plaintext: string; name: string } | null>(null)
  const [toDelete, setToDelete] = useState<APIKey | null>(null)
  const [deleting, setDeleting] = useState(false)
  const { toast } = useToast()

  async function refresh() {
    setLoading(true)
    try {
      const list = await listApiKeys()
      setItems(list)
    } catch (err) {
      toast({ title: '加载 API Key 失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  async function handleCreate() {
    const name = newKeyName.trim()
    if (!name) {
      toast({ title: '名称必填', variant: 'destructive' })
      return
    }
    setCreating(true)
    try {
      const result = await createApiKey(name)
      setCreatedResult({ plaintext: result.plaintext, name: result.key.name })
      setNewKeyName('')
      refresh()
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
    } catch (err) {
      toast({ title: '删除失败', description: String(err), variant: 'destructive' })
    } finally {
      setDeleting(false)
      setToDelete(null)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">API</h1>
          <p className="text-sm text-muted-foreground mt-1">
            管理外部 API 调用的密钥。创建后密钥仅展示一次。
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4 mr-1" />
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
                <Key className="h-5 w-5 text-muted-foreground shrink-0" />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium truncate">{key.name}</span>
                    <code className="text-xs px-1.5 py-0.5 rounded bg-muted">{key.prefix}...</code>
                    <Badge variant="outline" className="text-xs">{key.usage_24h}/24h</Badge>
                  </div>
                  <div className="text-xs text-muted-foreground mt-0.5">
                    创建于 {new Date(key.created_at).toLocaleDateString()}
                    {key.last_used_at && ` · 最近使用 ${new Date(key.last_used_at).toLocaleDateString()}`}
                    {!key.enabled && <span> · 已禁用</span>}
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Switch checked={key.enabled} onCheckedChange={() => handleToggle(key)} />
                  <Button variant="ghost" size="icon" onClick={() => setToDelete(key)}>
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* 新建对话框 */}
      <Dialog
        open={createOpen && !createdResult}
        onOpenChange={(o) => { if (!o) { setCreateOpen(false); setCreatedResult(null) } }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新建 API Key</DialogTitle>
            <DialogDescription>
              创建后密钥仅展示一次，请立即复制保存。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <Label htmlFor="ak-name">名称</Label>
            <Input
              id="ak-name"
              value={newKeyName}
              onChange={(e) => setNewKeyName(e.target.value)}
              placeholder="例如：生产环境"
              maxLength={64}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setCreateOpen(false); setCreatedResult(null) }}>
              取消
            </Button>
            <Button onClick={handleCreate} disabled={creating}>
              {creating ? '创建中…' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 创建成功：展示明文 */}
      <Dialog
        open={Boolean(createdResult)}
        onOpenChange={(o) => { if (!o) { setCreateOpen(false); setCreatedResult(null) } }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>密钥已创建</DialogTitle>
            <DialogDescription>
              请立即复制并安全保存。关闭后密钥将不可见。
            </DialogDescription>
          </DialogHeader>
          {createdResult && (
            <div className="space-y-3">
              <div className="rounded-md border bg-muted p-3">
                <div className="text-xs text-muted-foreground mb-1">{createdResult.name}</div>
                <code className="text-sm break-all select-all">{createdResult.plaintext}</code>
              </div>
              <Button
                className="w-full"
                variant="outline"
                onClick={() => {
                  navigator.clipboard.writeText(createdResult.plaintext)
                  toast({ title: '密钥已复制' })
                }}
              >
                <Copy className="h-4 w-4 mr-1" />
                复制密钥
              </Button>
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => { setCreateOpen(false); setCreatedResult(null) }}>
              <CheckCircle2 className="h-4 w-4 mr-1" />
              我已保存，关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 删除确认 */}
      <Dialog open={Boolean(toDelete)} onOpenChange={(o) => { if (!o) setToDelete(null) }}>
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
