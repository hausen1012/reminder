import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertCircle, RefreshCw } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Pagination } from '@/components/common/pagination'
import { PageHeader } from '@/components/common/PageHeader'
import { extractApiError, getSchedulerStatus } from '@/lib/api'
import { formatInZone } from '@/lib/datetime'
import { useSiteTimezone } from '@/contexts/ConfigContext'
import type { SchedulerStatus } from '@/types'

const PAGE_LIMIT = 10

const TYPE_LABEL: Record<string, string> = {
  once: '单次',
  interval: '周期',
  cron: 'Cron',
}

export default function SchedulerPage() {
  const navigate = useNavigate()
  const tz = useSiteTimezone()
  const [status, setStatus] = useState<SchedulerStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [limit, setLimit] = useState(PAGE_LIMIT)
  const [offset, setOffset] = useState(0)

  const statusRef = useRef(status)
  statusRef.current = status

  const fetchData = useCallback(
    async (currentOffset: number, currentLimit: number, mode: 'initial' | 'manual' | 'silent' = 'initial') => {
      if (mode === 'initial') {
        setLoading(true)
      } else if (mode === 'manual') {
        setRefreshing(true)
      }

      try {
        const data = await getSchedulerStatus({ limit: currentLimit, offset: currentOffset })
        setStatus(data)
        setError('')
      } catch (e) {
        // 静默后台刷新且已有数据时，不冲掉当前页面数据
        if (mode !== 'silent' || !statusRef.current) {
          setError(extractApiError(e, '加载失败'))
        }
      } finally {
        if (mode === 'initial') {
          setLoading(false)
        } else if (mode === 'manual') {
          setRefreshing(false)
        }
      }
    },
    [],
  )

  useEffect(() => {
    // 首次或分页条件改变时，若已有数据则平滑更新
    fetchData(offset, limit, statusRef.current ? 'silent' : 'initial')
    const id = setInterval(() => {
      fetchData(offset, limit, 'silent')
    }, 10000)
    return () => clearInterval(id)
  }, [offset, limit, fetchData])

  const engine = status?.engine
  const sweeper = status?.sweeper
  const entries = engine?.entries ?? []

  return (
    <div className="space-y-6">
      <PageHeader title="监控">
        <Button
          variant="outline"
          size="sm"
          onClick={() => fetchData(offset, limit, 'manual')}
          disabled={loading || refreshing}
          className="gap-1.5"
        >
          <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
          刷新
        </Button>
      </PageHeader>

      {/* 概览卡片 */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">引擎状态</CardTitle>
          </CardHeader>
          <CardContent>
            <Badge variant={engine?.running ? 'success' : 'secondary'}>
              {engine?.running ? '运行中' : '已停止'}
            </Badge>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">注册任务</CardTitle>
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-bold">{engine?.registered_count ?? 0}</span>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">扫描器状态</CardTitle>
          </CardHeader>
          <CardContent>
            <Badge variant={sweeper?.running ? 'success' : 'secondary'}>
              {sweeper?.running ? '运行中' : '已停止'}
            </Badge>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">扫描间隔</CardTitle>
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-bold">{sweeper?.interval_seconds ?? '—'}s</span>
          </CardContent>
        </Card>
      </div>

      {/* 注册任务表格 */}
      <Card>
        {error && !status ? (
          <CardContent className="py-6">
            <div className="flex items-center gap-2 text-destructive">
              <AlertCircle className="h-5 w-5" />
              <span>{error}</span>
            </div>
          </CardContent>
        ) : loading && !status ? (
          <CardContent className="py-6">
            <p className="text-sm text-muted-foreground">加载中…</p>
          </CardContent>
        ) : entries.length === 0 ? (
          <CardContent className="py-6">
            <p className="text-sm text-muted-foreground">暂无注册任务。</p>
          </CardContent>
        ) : (
          <>
            {/* 桌面端表格 */}
            <div className="hidden md:block">
              <div className="overflow-x-auto">
                <table className="w-full text-[13px] table-fixed">
                  <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                    <tr>
                      <th className="px-4 py-2.5 w-[20rem]">标题</th>
                      <th className="px-4 py-2.5 w-28">类型</th>
                      <th className="px-4 py-2.5 w-48">通知渠道</th>
                      <th className="px-4 py-2.5 w-44">下次触发</th>
                      <th className="px-4 py-2.5 w-44">上次触发</th>
                    </tr>
                  </thead>
                  <tbody>
                    {entries.map((e) => {
                      const channelNames = (e.channels ?? []).map((c) => c.name).join('、')
                      return (
                        <tr
                          key={e.id}
                          className="border-b last:border-b-0 hover:bg-muted/30 cursor-pointer transition-colors"
                          onClick={() => navigate(`/reminders`)}
                        >
                          <td className="px-4 py-2.5 truncate max-w-[200px]" title={e.title}>{e.title || '—'}</td>
                          <td className="px-4 py-2.5">
                            {TYPE_LABEL[e.schedule_type] ?? e.schedule_type}
                          </td>
                          <td className="px-4 py-2.5 text-muted-foreground max-w-[12rem]">
                            {channelNames ? (
                              <span className="truncate block" title={channelNames}>
                                {channelNames}
                              </span>
                            ) : (
                              <span className="text-muted-foreground/50">—</span>
                            )}
                          </td>
                          <td className="px-4 py-2.5 text-muted-foreground">
                            {formatInZone(e.next_fire_at, tz)}
                          </td>
                          <td className="px-4 py-2.5 text-muted-foreground">
                            {formatInZone(e.last_fired_at, tz)}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>

            {/* 移动端卡片列表 */}
            <div className="divide-y md:hidden">
              {entries.map((e) => {
                const channelNames = (e.channels ?? []).map((c) => c.name).join('、')
                return (
                  <div
                    key={e.id}
                    className="px-4 py-3 space-y-1.5 cursor-pointer active:bg-muted/30 transition-colors"
                    onClick={() => navigate(`/reminders`)}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-xs font-medium text-muted-foreground">{TYPE_LABEL[e.schedule_type] ?? e.schedule_type}</span>
                    </div>
                    <p className="font-medium truncate" title={e.title}>{e.title || '—'}</p>
                    <div className="text-xs text-muted-foreground">
                      <span className="block">通知渠道</span>
                      <span className="text-foreground truncate block" title={channelNames}>
                        {channelNames || '—'}
                      </span>
                    </div>
                    <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                      <div>
                        <span className="block">下次触发</span>
                        <span className="text-foreground">
                          {formatInZone(e.next_fire_at, tz)}
                        </span>
                      </div>
                      <div>
                        <span className="block">上次触发</span>
                        <span className="text-foreground">
                          {formatInZone(e.last_fired_at, tz)}
                        </span>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>

            <Pagination
              total={status?.engine.entries_total ?? 0}
              limit={limit}
              offset={offset}
              onPageChange={setOffset}
              onLimitChange={setLimit}
            />
          </>
        )}
      </Card>
    </div>
  )
}