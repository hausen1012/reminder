// 通道页：列表 + 新建/编辑对话框 + 试发 + 删除
import { useEffect, useState } from 'react'
import { Mail, MessageSquare, Webhook, Terminal, Plus, Pencil, Trash2, TestTube } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { useToast } from '@/components/ui/use-toast'
import { ChannelEditDialog } from '@/components/channels/ChannelEditDialog'
import { ConfirmDialog } from '@/components/channels/ConfirmDialog'
import {
  listChannels,
  toggleChannel,
  deleteChannel,
  testChannel,
} from '@/lib/api'
import type { Channel, ChannelType } from '@/types'

const TYPE_LABEL: Record<ChannelType, string> = {
  smtp: '邮件 SMTP',
  dingtalk: '钉钉机器人',
  wecom: '企业微信机器人',
  webhook: '通用 Webhook',
  log: '日志输出',
}

const TYPE_ICON: Record<ChannelType, typeof Mail> = {
  smtp: Mail,
  dingtalk: MessageSquare,
  wecom: MessageSquare,
  webhook: Webhook,
  log: Terminal,
}

export default function ChannelsPage() {
  const [items, setItems] = useState<Channel[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<Channel | null>(null)
  const [creating, setCreating] = useState(false)
  const [toDelete, setToDelete] = useState<Channel | null>(null)
  const [testingId, setTestingId] = useState<number | null>(null)
  const { toast } = useToast()

  async function refresh() {
    setLoading(true)
    try {
      const list = await listChannels()
      setItems(list)
    } catch (err) {
      toast({ title: '加载通道失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  async function handleToggle(ch: Channel) {
    try {
      const next = await toggleChannel(ch.id)
      setItems((prev) => prev.map((it) => (it.id === ch.id ? next : it)))
    } catch (err) {
      toast({ title: '切换状态失败', description: String(err), variant: 'destructive' })
    }
  }

  async function handleDelete() {
    if (!toDelete) return
    try {
      await deleteChannel(toDelete.id)
      setItems((prev) => prev.filter((it) => it.id !== toDelete.id))
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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">通知</h1>
          <p className="text-sm text-muted-foreground mt-1">
            管理用于发送提醒的通道：邮件、钉钉、企微、Webhook、日志。
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <Plus className="h-4 w-4 mr-1" />
          新建通道
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
            <table className="w-full text-sm">
              <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-4 py-3">名称</th>
                  <th className="px-4 py-3">类型</th>
                  <th className="px-4 py-3 text-center">启用</th>
                  <th className="px-4 py-3 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((ch) => {
                  const Icon = TYPE_ICON[ch.type]
                  return (
                    <tr key={ch.id} className="border-b last:border-b-0 hover:bg-muted/30">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <Icon className="h-4 w-4 text-muted-foreground shrink-0" />
                          <span className="font-medium truncate" title={ch.name}>{ch.name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs text-muted-foreground">{TYPE_LABEL[ch.type]}</td>
                      <td className="px-4 py-3 text-center">
                        <Switch checked={ch.enabled} onCheckedChange={() => handleToggle(ch)} aria-label="启用/禁用通道" />
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex justify-end gap-1">
                          <Button size="sm" variant="ghost" onClick={() => handleTest(ch)} disabled={testingId === ch.id} title="试发">
                            <TestTube className="h-4 w-4" />
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setEditing(ch)} title="编辑">
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" onClick={() => setToDelete(ch)} title="删除">
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
        </Card>
      )}

      {/* 编辑/新建对话框 */}
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
