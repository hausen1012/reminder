import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Activity, Loader2, AlertCircle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { getSchedulerStatus } from '@/lib/api'
import type { SchedulerStatus } from '@/types'

export default function SchedulerPage() {
  const navigate = useNavigate()
  const [status, setStatus] = useState<SchedulerStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const fetch = () => {
    getSchedulerStatus()
      .then(setStatus)
      .catch((e) => setError(e?.response?.data?.message || e.message || '加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetch()
    const id = setInterval(fetch, 10000)
    return () => clearInterval(id)
  }, [])

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
        <h1 className="text-2xl font-bold tracking-tight">调度器</h1>
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
        <CardHeader>
          <CardTitle className="text-base">注册任务</CardTitle>
        </CardHeader>
        <CardContent>
          {entries.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无注册任务。</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="pb-2 pr-4 font-medium">ID</th>
                    <th className="pb-2 pr-4 font-medium">类型</th>
                    <th className="pb-2 pr-4 font-medium">下次触发</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((e) => (
                    <tr
                      key={e.id}
                      className="border-b last:border-0 hover:bg-muted/50 cursor-pointer transition-colors"
                      onClick={() => navigate(`/reminders`)}
                    >
                      <td className="py-2 pr-4 font-medium text-primary">{e.id}</td>
                      <td className="py-2 pr-4">
                        <Badge variant="outline">{e.kind}</Badge>
                      </td>
                      <td className="py-2 pr-4 text-muted-foreground">
                        {e.next_fire_at
                          ? new Date(e.next_fire_at).toLocaleString('zh-CN')
                          : '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}