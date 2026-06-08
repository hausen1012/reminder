import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Bell, ScrollText, Send, Key, ArrowRight, CheckCircle2, XCircle, Loader2 } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useAuth } from '@/contexts/AuthContext'
import { listUpcomingReminders, listLogs, listChannelStats, listApiKeyStats } from '@/lib/api'
import { formatReminderDetail } from '@/lib/utils'
import type { Reminder, DeliveryLog } from '@/types'
import type { ChannelStats } from '@/lib/api'

function MiniLoader() {
  return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
}

export default function Dashboard() {
  const { user } = useAuth()

  const [upcoming, setUpcoming] = useState<Reminder[]>([])
  const [upcomingLoading, setUpcomingLoading] = useState(true)
  const [logs, setLogs] = useState<DeliveryLog[]>([])
  const [logsLoading, setLogsLoading] = useState(true)
  const [channelStats, setChannelStats] = useState<ChannelStats[]>([])
  const [channelStatsLoading, setChannelStatsLoading] = useState(true)
  const [apiKeyStats, setApiKeyStats] = useState<{ id: number; name: string; usage_24h: number }[]>([])
  const [apiKeyStatsLoading, setApiKeyStatsLoading] = useState(true)

  useEffect(() => {
    listUpcomingReminders('24h', 10).then(setUpcoming).finally(() => setUpcomingLoading(false))
    listLogs({ limit: 10 }).then((r) => setLogs(r.items ?? [])).finally(() => setLogsLoading(false))
    listChannelStats('24h').then(setChannelStats).finally(() => setChannelStatsLoading(false))
    listApiKeyStats().then(setApiKeyStats).finally(() => setApiKeyStatsLoading(false))
  }, [])

  const totalChannelSend = channelStats.reduce((s, c) => s + c.total, 0)
  const totalChannelSuccess = channelStats.reduce((s, c) => s + c.success, 0)
  const channelSuccessRate = totalChannelSend > 0 ? ((totalChannelSuccess / totalChannelSend) * 100).toFixed(1) : '—'

  const statusBadge = (status: string) => {
    const map: Record<string, string> = {
      success: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
      failed: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
      partial: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
      pending: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
      expired: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
    }
    return map[status] ?? 'bg-gray-100 text-gray-800'
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">仪表盘</h1>
          <p className="text-sm text-muted-foreground mt-1">你好，{user?.username}。这是今天的概况。</p>
        </div>
      </div>

      {/* 四张统计卡 */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {/* 今日待发 */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">今日待发提醒</CardTitle>
            <Bell className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {upcomingLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="text-2xl font-bold">{upcoming.length}</div>
                <p className="text-xs text-muted-foreground mt-1">未来 24 小时内待触发</p>
                {upcoming.length > 0 && (
                  <Link to="/reminders" className="text-xs text-primary hover:underline inline-flex items-center gap-1 mt-2">
                    查看全部 <ArrowRight className="h-3 w-3" />
                  </Link>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* 最近发送 */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">最近发送</CardTitle>
            <ScrollText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {logsLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="text-2xl font-bold">{logs.length}</div>
                <p className="text-xs text-muted-foreground mt-1">最近 10 条发送记录</p>
                {logs.length > 0 && (
                  <Link to="/logs" className="text-xs text-primary hover:underline inline-flex items-center gap-1 mt-2">
                    查看全部 <ArrowRight className="h-3 w-3" />
                  </Link>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* 通道健康 */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">通道健康</CardTitle>
            <Send className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {channelStatsLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="text-2xl font-bold">{channelSuccessRate}%</div>
                <p className="text-xs text-muted-foreground mt-1">过去 24h 发送成功率</p>
                {totalChannelSend > 0 && (
                  <div className="flex items-center gap-2 mt-2 text-xs text-muted-foreground">
                    <span className="inline-flex items-center gap-0.5"><CheckCircle2 className="h-3 w-3 text-green-500" />{totalChannelSuccess}</span>
                    <span className="inline-flex items-center gap-0.5"><XCircle className="h-3 w-3 text-red-500" />{totalChannelSend - totalChannelSuccess}</span>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* API Key 调用 */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">API Key 调用</CardTitle>
            <Key className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {apiKeyStatsLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="text-2xl font-bold">{apiKeyStats.reduce((s, k) => s + k.usage_24h, 0)}</div>
                <p className="text-xs text-muted-foreground mt-1">过去 24h API 调用次数</p>
                {apiKeyStats.length > 0 && (
                  <Link to="/apikeys" className="text-xs text-primary hover:underline inline-flex items-center gap-1 mt-2">
                    查看详情 <ArrowRight className="h-3 w-3" />
                  </Link>
                )}
              </>
            )}
          </CardContent>
        </Card>
      </div>

      {/* 详情行 */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* 今日待发列表 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">今日待发提醒</CardTitle>
          </CardHeader>
          <CardContent>
            {upcomingLoading ? (
              <p className="text-sm text-muted-foreground">加载中…</p>
            ) : upcoming.length === 0 ? (
              <p className="text-sm text-muted-foreground">未来 24 小时内没有待触发的提醒。</p>
            ) : (
              <div className="space-y-2 max-h-[420px] overflow-y-auto pr-1">
                {upcoming.map((r) => (
                  <Link
                    key={r.id}
                    to={`/reminders`}
                    className="block rounded-md border p-3 hover:bg-muted/50 transition-colors"
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium truncate">{r.title}</span>
                      <Badge variant="outline" className="text-xs shrink-0 ml-2">
                        {r.next_fire_at ? new Date(r.next_fire_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : '—'}
                      </Badge>
                    </div>
                    <div className="text-xs text-muted-foreground mt-1">
                      {formatReminderDetail(r)}
                      {r.channel_ids.length > 0 && ` · ${r.channel_ids.length} 个通道`}
                    </div>
                  </Link>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* 最近发送列表 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">最近发送</CardTitle>
          </CardHeader>
          <CardContent>
            {logsLoading ? (
              <p className="text-sm text-muted-foreground">加载中…</p>
            ) : logs.length === 0 ? (
              <p className="text-sm text-muted-foreground">暂无发送记录。</p>
            ) : (
              <div className="space-y-2 max-h-[420px] overflow-y-auto pr-1">
                {logs.map((log) => (
                  <Link
                    key={log.id}
                    to={`/logs`}
                    className="block rounded-md border p-3 hover:bg-muted/50 transition-colors"
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium truncate">{log.reminder_title || log.title}</span>
                      <span className={`text-xs px-1.5 py-0.5 rounded ${statusBadge(log.status)}`}>{log.status}</span>
                    </div>
                    <div className="text-xs text-muted-foreground mt-1">
                      {new Date(log.fired_at).toLocaleString('zh-CN')}
                      {log.source && ` · ${log.source}`}
                    </div>
                  </Link>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}