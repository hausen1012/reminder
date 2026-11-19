import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  Bell,
  ScrollText,
  Send,
  Cpu,
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
import { EmptyState } from '@/components/common/EmptyState'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { listTodayReminders, listLogs, listChannelStats, getSchedulerStatus } from '@/lib/api'
import { formatReminderDetail } from '@/lib/utils'
import { formatInZone } from '@/lib/datetime'
import { useSiteTimezone } from '@/contexts/ConfigContext'
import { PageHeader } from '@/components/common/PageHeader'
import { logStatusLabel, logStatusVariant, sourceLabel } from '@/lib/constants'
import { ReminderEditDialog } from '@/components/reminders/ReminderEditDialog'
import { useToast } from '@/components/ui/use-toast'
import type { Reminder, DeliveryLog, SchedulerStatus } from '@/types'
import type { ChannelStats } from '@/lib/api'

function MiniLoader() {
  return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
}

export default function Dashboard() {
  const tz = useSiteTimezone()
  const { toast } = useToast()

  const [todayCount, setTodayCount] = useState(0)
  const [todayReminders, setTodayReminders] = useState<Reminder[]>([])
  const [todayLoading, setTodayLoading] = useState(true)

  const [logs, setLogs] = useState<DeliveryLog[]>([])
  const [logsTotal, setLogsTotal] = useState(0)
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
      .then((r) => {
        setLogs(r.items ?? [])
        setLogsTotal(r.total ?? 0)
      })
      .catch(() => {
        setLogs([])
        setLogsTotal(0)
      })
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
  // 列表只拉取最近 200 条用于展示，卡片计数用服务端总数
  const logsCount = Math.max(logsTotal, logs.length)

  const engineRunning = schedulerStatus?.engine?.running ?? false
  const engineRegistered = schedulerStatus?.engine?.registered_count ?? 0

  const nextUpcoming = todayReminders[0]

  return (
    <div className="space-y-5 md:space-y-6">
      {/* 头部区：标题及全局快捷操作 */}
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between md:gap-4">
        <PageHeader title="首页" />
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={refreshing}
            className="flex-1 gap-1.5 text-xs md:flex-none"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} />
            刷新
          </Button>
          <Button
            size="sm"
            onClick={() => setCreateDialogOpen(true)}
            className="flex-1 gap-1.5 text-xs md:flex-none"
          >
            <Plus className="h-4 w-4" />
            新建提醒
          </Button>
        </div>
      </div>

      {/* 四张核心指标卡：整卡可点跳转对应页面，移动端两列，减少纵向滚动 */}
      <div className="grid grid-cols-2 gap-3 md:gap-4 lg:grid-cols-4">
        {/* 卡片 1：今日待发 */}
        <Link to="/reminders" className="group block rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <Card className="h-full transition-colors group-hover:border-primary/40">
            <CardHeader className="flex flex-row items-center justify-between gap-1 pb-2">
              <CardTitle className="min-w-0 truncate text-xs font-medium md:text-sm">
                今日待发
              </CardTitle>
              <Bell className="h-4 w-4 shrink-0 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              {todayLoading ? (
                <MiniLoader />
              ) : (
                <>
                  <div className="text-xl font-bold md:text-2xl">{todayCount}</div>
                  <p className="text-xs text-muted-foreground mt-1 truncate">
                    {nextUpcoming
                      ? `下个触发：${formatInZone(nextUpcoming.next_fire_at, tz, 'clock')}`
                      : '今日无待触发提醒'}
                  </p>
                </>
              )}
            </CardContent>
          </Card>
        </Link>

        {/* 卡片 2：最近发送 */}
        <Link to="/logs" className="group block rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <Card className="h-full transition-colors group-hover:border-primary/40">
            <CardHeader className="flex flex-row items-center justify-between gap-1 pb-2">
              <CardTitle className="min-w-0 truncate text-xs font-medium md:text-sm">
                最近发送
              </CardTitle>
              <ScrollText className="h-4 w-4 shrink-0 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              {logsLoading ? (
                <MiniLoader />
              ) : (
                <>
                  <div className="text-xl font-bold md:text-2xl">{logsCount}</div>
                  <div className="text-xs text-muted-foreground mt-1 flex items-center gap-1.5">
                    <span>过去 24 小时</span>
                    {logsFailed > 0 ? (
                      <span className="text-destructive font-medium">· {logsFailed} 次异常</span>
                    ) : logs.length > 0 ? (
                      <span className="text-emerald-600 dark:text-emerald-400 font-medium">· 全送达</span>
                    ) : null}
                  </div>
                </>
              )}
            </CardContent>
          </Card>
        </Link>

        {/* 卡片 3：发送成功率 */}
        <Link to="/channels" className="group block rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <Card className="h-full transition-colors group-hover:border-primary/40">
            <CardHeader className="flex flex-row items-center justify-between gap-1 pb-2">
              <CardTitle className="min-w-0 truncate text-xs font-medium md:text-sm">发送成功率</CardTitle>
              <Send className="h-4 w-4 shrink-0 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              {channelStatsLoading ? (
                <MiniLoader />
              ) : (
                <>
                  <div className="text-xl font-bold md:text-2xl">{channelSuccessRate}%</div>
                  {totalChannelSend > 0 ? (
                    <div className="flex items-center gap-2.5 mt-1 text-xs text-muted-foreground">
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
                    <p className="text-xs text-muted-foreground mt-1">过去 24h 无发送记录</p>
                  )}
                </>
              )}
            </CardContent>
          </Card>
        </Link>

        {/* 卡片 4：调度引擎 */}
        <Link to="/scheduler" className="group block rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <Card className="h-full transition-colors group-hover:border-primary/40">
            <CardHeader className="flex flex-row items-center justify-between gap-1 pb-2">
              <CardTitle className="min-w-0 truncate text-xs font-medium md:text-sm">调度引擎</CardTitle>
              <Cpu className="h-4 w-4 shrink-0 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              {schedulerLoading ? (
                <MiniLoader />
              ) : (
                <>
                  <div className="flex items-center gap-2">
                    <span
                      className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ${
                        engineRunning ? 'bg-emerald-500 animate-pulse' : 'bg-destructive'
                      }`}
                    />
                    <span className="text-xl font-bold md:text-2xl">{engineRunning ? '运行中' : '已停止'}</span>
                  </div>
                  <p className="text-xs text-muted-foreground mt-1 truncate">
                    {engineRunning ? `已装载 ${engineRegistered} 个调度任务` : '调度器离线，提醒将无法触发'}
                  </p>
                </>
              )}
            </CardContent>
          </Card>
        </Link>
      </div>

      {/* 详情主区域 */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* 今日待发列表 */}
        <Card className="min-w-0">
          <CardHeader className="flex flex-row items-center justify-between gap-2">
            <CardTitle className="min-w-0 truncate text-base">
              今日待发提醒 <span className="text-xs font-normal text-muted-foreground">({todayCount})</span>
            </CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setCreateDialogOpen(true)}
              className="h-8 shrink-0 px-2 text-xs text-primary"
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
              <EmptyState
                variant="dashed"
                size="compact"
                icon={CalendarCheck}
                title="今天没有待触发的提醒"
                action={{
                  label: '新建提醒',
                  icon: Plus,
                  onClick: () => setCreateDialogOpen(true),
                  variant: 'outline',
                }}
              />
            ) : (
              <div className="space-y-2 max-h-[420px] overflow-y-auto overflow-x-hidden scroll-area pr-1 md:space-y-2.5">
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
          <CardHeader className="flex flex-row items-center justify-between gap-2">
            <CardTitle className="min-w-0 truncate text-base">
              最近发送 <span className="text-xs font-normal text-muted-foreground">({logsCount})</span>
            </CardTitle>
            <Link to="/logs" className="shrink-0 text-xs text-muted-foreground hover:text-foreground">
              全部记录
            </Link>
          </CardHeader>
          <CardContent>
            {logsLoading ? (
              <div className="py-8 flex justify-center">
                <MiniLoader />
              </div>
            ) : logs.length === 0 ? (
              <EmptyState
                variant="dashed"
                size="compact"
                icon={Inbox}
                title="过去 24 小时暂无发送记录"
              />
            ) : (
              <div className="space-y-2 max-h-[420px] overflow-y-auto overflow-x-hidden scroll-area pr-1 md:space-y-2.5">
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
                        <Badge variant={logStatusVariant(log.status)} className="shrink-0 text-xs">
                          {logStatusLabel(log.status)}
                        </Badge>
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