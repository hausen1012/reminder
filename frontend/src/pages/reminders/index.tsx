// 提醒列表页：toolbar（来源筛选/状态/搜索/新建）+ 表格 + 编辑/试发/删除。
import { useEffect, useState, useMemo } from 'react'
import { Plus, Pencil, Trash2, TestTube } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import { ReminderEditDialog } from '@/components/reminders/ReminderEditDialog'
import { ConfirmDialog } from '@/components/channels/ConfirmDialog'
import {
  deleteReminder,
  listChannels,
  listReminders,
  testReminder,
  toggleReminder,
  type ListRemindersQuery,
} from '@/lib/api'
import type { Channel, Reminder } from '@/types'

const SOURCE_LABEL: Record<string, string> = {
  manual: '手动',
  api: 'API',
}

const TYPE_LABEL: Record<string, string> = {
  once: '一次性',
  interval: '周期',
  cron: 'Cron',
}

export default function RemindersPage() {
  const [items, setItems] = useState<Reminder[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<Reminder | null>(null)
  const [creating, setCreating] = useState(false)
  const [toDelete, setToDelete] = useState<Reminder | null>(null)
  const [testingId, setTestingId] = useState<number | null>(null)

  const [source, setSource] = useState<string>('all')
  const [enabled, setEnabled] = useState<string>('all')
  const [search, setSearch] = useState('')

  const [channels, setChannels] = useState<Channel[]>([])

  const { toast } = useToast()

  async function refresh() {
    setLoading(true)
    try {
      const q: ListRemindersQuery = {}
      if (source !== 'all') q.source = source as 'manual' | 'api'
      if (enabled !== 'all') q.enabled = enabled === 'true'
      if (search.trim()) q.search = search.trim()
      const data = await listReminders(q)
      setItems(data?.items ?? [])
      setTotal(data?.total ?? 0)
    } catch (err) {
      toast({ title: '加载提醒失败', description: String(err), variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    listChannels().then(setChannels).catch(() => setChannels([]))
  }, [])

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, enabled])

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
    try {
      await deleteReminder(toDelete.id)
      setItems((prev) => prev.filter((it) => it.id !== toDelete.id))
      setTotal((n) => n - 1)
      toast({ title: '提醒已删除', variant: 'success' })
    } catch (err) {
      toast({ title: '删除失败', description: String(err), variant: 'destructive' })
    } finally {
      setToDelete(null)
    }
  }

  async function handleTest(r: Reminder) {
    setTestingId(r.id)
    try {
      await testReminder(r.id)
      toast({ title: '已立即触发一次', description: `查看日志页确认通道送达结果。`, variant: 'success' })
    } catch (err) {
      toast({ title: '触发失败', description: String(err), variant: 'destructive' })
    } finally {
      setTestingId(null)
    }
  }

  const channelMap = useMemo(() => {
    const map = new Map<number, string>()
    channels.forEach((ch) => map.set(ch.id, ch.name))
    return map
  }, [channels])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">提醒</h1>
          <p className="text-sm text-muted-foreground mt-1">
            创建一次性 / 周期 / Cron 提醒；到达触发时间通过所选通道发送。
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <Plus className="h-4 w-4 mr-1" />
          新建提醒
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="w-40">
          <Select value={source} onValueChange={setSource}>
            <SelectTrigger>
              <SelectValue placeholder="来源" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部来源</SelectItem>
              <SelectItem value="manual">手动</SelectItem>
              <SelectItem value="api">API</SelectItem>
            </SelectContent>
          </Select>
        </div>
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
          className="flex-1 max-w-md"
          onSubmit={(e) => {
            e.preventDefault()
            refresh()
          }}
        >
          <Input
            placeholder="搜索标题或内容…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </form>
        <span className="text-sm text-muted-foreground">共 {total} 条</span>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : items.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            还没有提醒，点击右上角新建一个。
          </CardContent>
        </Card>
      ) : (
        <Card>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-4 py-3">标题</th>
                  <th className="px-4 py-3">调度</th>
                  <th className="px-4 py-3">通道</th>
                  <th className="px-4 py-3">下次触发</th>
                  <th className="px-4 py-3">来源</th>
                  <th className="px-4 py-3 text-center">启用</th>
                  <th className="px-4 py-3 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((r) => (
                  <tr key={r.id} className="border-b last:border-b-0 hover:bg-muted/30">
                    <td className="px-4 py-3 max-w-[20rem]">
                      <div className="font-medium truncate" title={r.title}>
                        {r.title}
                      </div>
                      {r.content && (
                        <div className="text-xs text-muted-foreground truncate" title={r.content}>
                          {r.content}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant="outline">
                        {r.calendar === 'lunar' ? '农历' : '公历'} · {TYPE_LABEL[r.schedule_type] ?? r.schedule_type}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        {r.channel_ids.length === 0 ? (
                          <span className="text-xs text-muted-foreground">—</span>
                        ) : (
                          r.channel_ids.map((cid) => (
                            <Badge key={cid} variant="secondary" className="text-xs">
                              {channelMap.get(cid) || `#${cid}`}
                            </Badge>
                          ))
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-xs">
                      {r.next_fire_at ? new Date(r.next_fire_at).toLocaleString() : '—'}
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant="secondary">{SOURCE_LABEL[r.source] ?? r.source}</Badge>
                    </td>
                    <td className="px-4 py-3 text-center">
                      <Switch checked={r.enabled} onCheckedChange={() => handleToggle(r)} />
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleTest(r)}
                          disabled={testingId === r.id}
                          title="立即触发一次"
                        >
                          <TestTube className="h-4 w-4 mr-1" />
                          {testingId === r.id ? '发送中…' : '试触发'}
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setEditing(r)}>
                          <Pencil className="h-4 w-4 mr-1" />
                          编辑
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-destructive hover:text-destructive"
                          onClick={() => setToDelete(r)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
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
        onConfirm={handleDelete}
        onCancel={() => setToDelete(null)}
      />
    </div>
  )
}
