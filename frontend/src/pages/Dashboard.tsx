import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  Bell,
  ScrollText,
  Send,
  Cpu,
  ArrowRight,
  CheckCircle2,
  XCircle,
  Loader2,
  Plus,
  RefreshCw,
  CalendarCheck,
  Inbox,
  AlertTriangle,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/AuthContext'
import { listTodayReminders, listLogs, listChannelStats, getSchedulerStatus } from '@/lib/api'
import { formatReminderDetail } from '@/lib/utils'
import { formatInZone } from '@/lib/datetime'
import { useSiteTimezone } from '@/contexts/ConfigContext'
import { PageHeader } from '@/components/common/PageHeader'
import { sourceLabel } from '@/lib/constants'
import { ReminderEditDialog } from '@/components/reminders/ReminderEditDialog'
import { useToast } from '@/components/ui/use-toast'
import type { Reminder, DeliveryLog, SchedulerStatus } from '@/types'
import type { ChannelStats } from '@/lib/api'

function MiniLoader() {
  return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
}

export default function Dashboard() {
  const { user } = useAuth()
  const tz = useSiteTimezone()
  const { toast } = useToast()

  const [todayCount, setTodayCount] = useState(0)
  const [todayReminders, setTodayReminders] = useState<Reminder[]>([])
  const [todayLoading, setTodayLoading] = useState(true)

  const [logs, setLogs] = useState<DeliveryLog[]>([])
  const [logsLoading, setLogsLoading] = useState(true)

  const [channelStats, setChannelStats] = useState<ChannelStats[]>([])
  const [channelStatsLoading, setChannelStatsLoading] = useState(true)

  const [schedulerStatus, setSchedulerStatus] = useState<SchedulerStatus | null>(null)
  const [schedulerLoading, setSchedulerLoading] = useState(true)

  const [refreshing, setRefreshing] = useState(false)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)

  const fetchAll = useCallback(async () => {
    const since24h = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()

    const pReminders = listTodayReminders()
      .then((items) => {
        setTodayReminders(items)
        setTodayCount(items.length)
      })
      .catch(() => {
        setTodayReminders([])
        setTodayCount(0)
      })
      .finally(() => setTodayLoading(false))

    const pLogs = listLogs({ limit: 200, since: since24h })
      .then((r) => setLogs(r.items ?? []))
      .catch(() => setLogs([]))
      .finally(() => setLogsLoading(false))

    const pChannels = listChannelStats('24h')
      .then(setChannelStats)
      .catch(() => setChannelStats([]))
      .finally(() => setChannelStatsLoading(false))

    const pScheduler = getSchedulerStatus({ limit: 1 })
      .then(setSchedulerStatus)
      .catch(() => setSchedulerStatus(null))
      .finally(() => setSchedulerLoading(false))

    await Promise.allSettled([pReminders, pLogs, pChannels, pScheduler])
  }, [])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  async function handleRefresh() {
    setRefreshing(true)
    await fetchAll()
    setRefreshing(false)
  }

  const totalChannelSend = channelStats.reduce((s, c) => s + c.total, 0)
  const totalChannelSuccess = channelStats.reduce((s, c) => s + c.success, 0)
  const channelSuccessRate = totalChannelSend > 0 ? ((totalChannelSuccess / totalChannelSend) * 100).toFixed(1) : '—'

  const logsFailed = logs.filter((l) => l.status === 'failed' || l.status === 'partial').length

  const engineRunning = schedulerStatus?.engine?.running ?? false
  const engineRegistered = schedulerStatus?.engine?.registered_count ?? 0

  const statusBadge = (status: string) => {
    const map: Record<string, string> = {
      success: 'bg-green-100 text-green-800 dark:bg-green-900/60 dark:text-green-300',
      failed: 'bg-red-100 text-red-800 dark:bg-red-900/60 dark:text-red-300',
      partial: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/60 dark:text-yellow-300',
      pending: 'bg-blue-100 text-blue-800 dark:bg-blue-900/60 dark:text-blue-300',
      expired: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300',
    }
    return map[status] ?? 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300'
  }

  const nextUpcoming = todayReminders[0]

  return (
    <div className="space-y-6">
      {/* 头部区：标题、问候语及全局快捷操作 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <PageHeader title="首页" />
          <p className="text-sm text-muted-foreground mt-1">你好，{user?.username}。这是今天的概况。</p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={refreshing}
            className="gap-1.5 text-xs"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} />
            刷新
          </Button>
          <Button
            size="sm"
            onClick={() => setCreateDialogOpen(true)}
            className="gap-1.5 text-xs"
          >
            <Plus className="h-4 w-4" />
            新建提醒
          </Button>
        </div>
      </div>

      {/* 四张核心指标卡 */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {/* 卡片 1：今日待发 */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">
              今日待发提醒 <span className="ml-1 text-xs text-muted-foreground">({todayCount})</span>
            </CardTitle>
            <Bell className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {todayLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="text-2xl font-bold">{todayCount}</div>
                <p className="text-xs text-muted-foreground mt-1 truncate">
                  {nextUpcoming
                    ? `下个触发：${formatInZone(nextUpcoming.next_fire_at, tz, 'clock')}`
                    : '今天待触发任务已全部完成'}
                </p>
                <Link
                  to="/reminders"
                  className="text-xs text-primary hover:underline inline-flex items-center gap-1 mt-2 font-medium"
                >
                  查看全部 <ArrowRight className="h-3 w-3" />
                </Link>
              </>
            )}
          </CardContent>
        </Card>

        {/* 卡片 2：最近发送 */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">
              最近发送 <span className="ml-1 text-xs text-muted-foreground">({logs.length})</span>
            </CardTitle>
            <ScrollText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {logsLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="text-2xl font-bold">{logs.length}</div>
                <div className="text-xs text-muted-foreground mt-1 flex items-center gap-1.5">
                  <span>过去 24 小时</span>
                  {logsFailed > 0 ? (
                    <span className="text-destructive font-medium">· {logsFailed} 次异常</span>
                  ) : logs.length > 0 ? (
                    <span className="text-emerald-600 dark:text-emerald-400 font-medium">· 全送达</span>
                  ) : null}
                </div>
                <Link
                  to="/logs"
                  className="text-xs text-primary hover:underline inline-flex items-center gap-1 mt-2 font-medium"
                >
                  查看日志 <ArrowRight className="h-3 w-3" />
                </Link>
              </>
            )}
          </CardContent>
        </Card>

        {/* 卡片 3：通知健康 */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">通知健康</CardTitle>
            <Send className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {channelStatsLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="text-2xl font-bold">{channelSuccessRate}%</div>
                <p className="text-xs text-muted-foreground mt-1">过去 24h 发送成功率</p>
                {totalChannelSend > 0 ? (
                  <div className="flex items-center gap-2.5 mt-2 text-xs text-muted-foreground">
                    <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                      <CheckCircle2 className="h-3.5 w-3.5" />
                      {totalChannelSuccess}
                    </span>
                    <span className="inline-flex items-center gap-1 text-destructive">
                      <XCircle className="h-3.5 w-3.5" />
                      {totalChannelSend - totalChannelSuccess}
                    </span>
                  </div>
                ) : (
                  <Link
                    to="/channels"
                    className="text-xs text-primary hover:underline inline-flex items-center gap-1 mt-2 font-medium"
                  >
                    管理渠道 <ArrowRight className="h-3 w-3" />
                  </Link>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* 卡片 4：调度引擎（替换原有无用令牌调用） */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">调度引擎</CardTitle>
            <Cpu className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {schedulerLoading ? (
              <MiniLoader />
            ) : (
              <>
                <div className="flex items-center gap-2">
                  <span
                    className={`inline-block h-2.5 w-2.5 rounded-full ${
                      engineRunning ? 'bg-emerald-500 animate-pulse' : 'bg-destructive'
                    }`}
                  />
                  <span className="text-2xl font-bold">{engineRunning ? '运行中' : '已停止'}</span>
                </div>
                <p className="text-xs text-muted-foreground mt-1 truncate">
                  {engineRunning ? `已装载 ${engineRegistered} 个调度任务` : '调度器离线，提醒将无法触发'}
                </p>
                <Link
                  to="/scheduler"
                  className="text-xs text-primary hover:underline inline-flex items-center gap-1 mt-2 font-medium"
                >
                  系统监控 <ArrowRight className="h-3 w-3" />
                </Link>
              </>
            )}
          </CardContent>
        </Card>
      </div>

      {/* 详情主区域 */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* 今日待发列表 */}
        <Card className="min-w-0">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-base">
              今日待发提醒 <span className="text-xs font-normal text-muted-foreground">({todayCount})</span>
            </CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setCreateDialogOpen(true)}
              className="h-8 px-2 text-xs text-primary"
            >
              <Plus className="h-3.5 w-3.5 mr-1" />
              添加
            </Button>
          </CardHeader>
          <CardContent>
            {todayLoading ? (
              <div className="py-8 flex justify-center">
                <MiniLoader />
              </div>
            ) : todayReminders.length === 0 ? (
              <div className="py-8 text-center flex flex-col items-center justify-center space-y-2 border border-dashed rounded-lg bg-muted/20">
                <CalendarCheck className="h-8 w-8 text-muted-foreground/60" />
                <p className="text-sm font-medium text-muted-foreground">今天没有待触发的提醒</p>
                <p className="text-xs text-muted-foreground">日程很轻松，或者现在安排一个新提醒</p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCreateDialogOpen(true)}
                  className="mt-2 text-xs"
                >
                  <Plus className="h-3.5 w-3.5 mr-1" />
                  新建提醒
                </Button>
              </div>
            ) : (
              <div className="space-y-2.5 max-h-[420px] overflow-y-auto overflow-x-hidden pr-1">
                {todayReminders.map((r, idx) => (
                  <Link
                    key={r.id}
                    to="/reminders"
                    className="block rounded-lg border p-3 hover:bg-muted/50 transition-colors shadow-sm"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex items-center gap-2 min-w-0">
                        {idx === 0 && (
                          <span className="shrink-0 text-[10px] font-semibold bg-primary/10 text-primary px-1.5 py-0.5 rounded">
                            下个
                          </span>
                        )}
                        <span className="text-sm font-medium truncate">{r.title}</span>
                      </div>
                      <Badge variant="outline" className="text-xs shrink-0 font-mono">
                        {formatInZone(r.next_fire_at, tz, 'clock')}
                      </Badge>
                    </div>
                    <div className="text-xs text-muted-foreground mt-1.5 truncate">
                      {formatReminderDetail(r)}
                      {r.channel_ids.length > 0 && ` · ${r.channel_ids.length} 个渠道`}
                    </div>
                  </Link>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* 最近发送列表 */}
        <Card className="min-w-0">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-base">
              最近发送 <span className="text-xs font-normal text-muted-foreground">({logs.length})</span>
            </CardTitle>
            <Link to="/logs" className="text-xs text-muted-foreground hover:text-foreground">
              全部记录
            </Link>
          </CardHeader>
          <CardContent>
            {logsLoading ? (
              <div className="py-8 flex justify-center">
                <MiniLoader />
              </div>
            ) : logs.length === 0 ? (
              <div className="py-8 text-center flex flex-col items-center justify-center space-y-2 border border-dashed rounded-lg bg-muted/20">
                <Inbox className="h-8 w-8 text-muted-foreground/60" />
                <p className="text-sm font-medium text-muted-foreground">过去 24 小时暂无发送记录</p>
                <p className="text-xs text-muted-foreground">有提醒触发后将在此处显示投递日志</p>
              </div>
            ) : (
              <div className="space-y-2.5 max-h-[420px] overflow-y-auto overflow-x-hidden pr-1">
                {logs.map((log) => {
                  const isFailed = log.status === 'failed' || log.status === 'partial'
                  return (
                    <Link
                      key={log.id}
                      to="/logs"
                      className={`block rounded-lg border p-3 hover:bg-muted/50 transition-colors shadow-sm overflow-hidden ${
                        isFailed ? 'border-destructive/30 bg-destructive/5' : ''
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-1.5 min-w-0">
                          {isFailed && <AlertTriangle className="h-3.5 w-3.5 text-destructive shrink-0" />}
                          <span className="text-sm font-medium truncate min-w-0">
                            {log.reminder_title || log.title}
                          </span>
                        </div>
                        <span
                          className={`text-xs px-2 py-0.5 rounded-full font-medium shrink-0 whitespace-nowrap ${statusBadge(
                            log.status,
                          )}`}
                        >
                          {log.status}
                        </span>
                      </div>
                      <div className="text-xs text-muted-foreground mt-1.5 truncate">
                        {formatInZone(log.fired_at, tz)}
                        {log.source && ` · ${sourceLabel(log.source)}`}
                      </div>
                    </Link>
                  )
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* 快捷新建提醒弹窗 */}
      {createDialogOpen && (
        <ReminderEditDialog
          reminder={null}
          open={createDialogOpen}
          onClose={() => setCreateDialogOpen(false)}
          onSaved={() => {
            setCreateDialogOpen(false)
            toast({ title: '提醒创建成功', variant: 'success' })
            fetchAll()
          }}
        />
      )}
    </div>
  )
}