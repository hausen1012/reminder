import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Activity, Loader2, AlertCircle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Pagination } from '@/components/ui/pagination'
import { getSchedulerStatus } from '@/lib/api'
import type { SchedulerStatus } from '@/types'

const PAGE_LIMIT = 10

export default function SchedulerPage() {
  const navigate = useNavigate()
  const [status, setStatus] = useState<SchedulerStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [limit, setLimit] = useState(PAGE_LIMIT)
  const [offset, setOffset] = useState(0)

  const fetch = (currentOffset: number, currentLimit: number) => {
    setLoading(true)
    getSchedulerStatus({ limit: currentLimit, offset: currentOffset })
      .then(setStatus)
      .catch((e) => setError(e?.response?.data?.message || e.message || '加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetch(offset, limit)
    const id = setInterval(() => fetch(offset, limit), 10000)
    return () => clearInterval(id)
  }, [offset, limit])

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center gap-2 text-destructive">
        <AlertCircle className="h-5 w-5" />
        <span>{error}</span>
      </div>
    )
  }

  const engine = status?.engine
  const sweeper = status?.sweeper
  const entries = engine?.entries ?? []

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <Activity className="h-5 w-5" />
        <h1 className="text-2xl font-bold tracking-tight">监控</h1>
      </div>

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
        {entries.length === 0 && !loading ? (
          <CardContent>
            <p className="text-sm text-muted-foreground">暂无注册任务。</p>
          </CardContent>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-[13px] table-fixed">
                <thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                  <tr>
                    <th className="px-4 py-2.5 w-16">ID</th>
                    <th className="px-4 py-2.5 w-28">类型</th>
                    <th className="px-4 py-2.5">下次触发</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((e) => (
                    <tr
                      key={e.id}
                      className="border-b last:border-b-0 hover:bg-muted/30 cursor-pointer transition-colors"
                      onClick={() => navigate(`/reminders`)}
                    >
                      <td className="px-4 py-2.5 font-medium text-primary">{e.id}</td>
                      <td className="px-4 py-2.5">
                        <Badge variant="outline">{e.kind}</Badge>
                      </td>
                      <td className="px-4 py-2.5 text-muted-foreground">
                        {e.next_fire_at
                          ? new Date(e.next_fire_at).toLocaleString('zh-CN')
                          : '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
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