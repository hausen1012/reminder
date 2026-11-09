// 关于分区：当前版本、构建时间、更新检查与项目链接。
//
// 更新检查由后端每日自动执行并把结论缓存在内存里，这里只读取与触发手动检查；
// 关闭自动检查后后端不再出网，手动检查仍然可用。
import { useState } from 'react'
import { BookOpen, CheckCircle2, ExternalLink, Github, RefreshCw } from 'lucide-react'
import { extractApiError } from '@/lib/api'
import { useAbout } from '@/contexts/AboutContext'
import { useSiteTimezone } from '@/contexts/ConfigContext'
import { DOCS_URL, GITHUB_URL, RELEASES_URL } from '@/lib/constants'
import { formatInZone } from '@/lib/datetime'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useToast } from '@/components/ui/use-toast'

export function AboutSection() {
  const { info, checkNow, setEnabled } = useAbout()
  const { toast } = useToast()
  const tz = useSiteTimezone()
  const [checking, setChecking] = useState(false)
  const [toggling, setToggling] = useState(false)

  const update = info?.update
  const version = info?.version || '—'

  async function handleCheck() {
    setChecking(true)
    try {
      await checkNow()
      toast({ title: '检查完成' })
    } catch (err) {
      toast({ title: '检查失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setChecking(false)
    }
  }

  async function handleToggle(enabled: boolean) {
    setToggling(true)
    try {
      await setEnabled(enabled)
      toast({ title: enabled ? '已开启自动检查' : '已关闭自动检查' })
    } catch (err) {
      toast({ title: '保存失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setToggling(false)
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base font-medium">版本信息</CardTitle>
          <CardDescription>当前运行的构建版本与更新时间</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-3 text-sm sm:grid-cols-2">
            <div className="space-y-1">
              <dt className="text-muted-foreground">当前版本</dt>
              <dd className="font-medium">{version}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-muted-foreground">构建时间</dt>
              <dd className="font-medium">
                {info?.build_time ? formatInZone(info.build_time, tz, 'datetime-second') : '—'}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base font-medium">更新检查</CardTitle>
          <CardDescription>每天自动查询一次新版本，也可随时手动检查</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            {update?.error ? (
              <span className="text-destructive">检查失败：{update.error}</span>
            ) : !update ? (
              <span className="text-muted-foreground">尚未检查</span>
            ) : (
              <>
                {!update.comparable ? (
                  <span className="text-muted-foreground">当前为开发构建，不参与版本比较</span>
                ) : update.update_available ? (
                  <>
                    <Badge variant="warning">有新版本</Badge>
                    <span>{update.latest_version}</span>
                    <a
                      href={update.release_url}
                      target="_blank"
                      rel="noreferrer noopener"
                      className="inline-flex items-center gap-1 text-primary hover:underline"
                    >
                      查看版本
                      <ExternalLink className="h-3 w-3" />
                    </a>
                  </>
                ) : update.latest_version ? (
                  <>
                    <Badge variant="success">
                      <CheckCircle2 className="mr-1 h-3 w-3" />
                      已是最新
                    </Badge>
                    <span className="text-muted-foreground">{update.latest_version}</span>
                  </>
                ) : (
                  <span className="text-muted-foreground">尚未检查</span>
                )}
              </>
            )}
          </div>

          <p className="text-xs text-muted-foreground">
            最近检查：
            {update?.checked_at ? formatInZone(update.checked_at, tz, 'datetime-second') : '—'}
          </p>

          <div className="flex items-center justify-between gap-4">
            <div className="space-y-1">
              <Label htmlFor="update-check-enabled">每天自动检查更新</Label>
              <p className="text-xs text-muted-foreground">关闭后不再请求 GitHub，手动检查仍然可用</p>
            </div>
            <Switch
              id="update-check-enabled"
              checked={Boolean(update?.enabled)}
              disabled={!info || toggling}
              onCheckedChange={handleToggle}
            />
          </div>

          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={handleCheck} disabled={checking}>
              <RefreshCw className={`mr-1 h-4 w-4 ${checking ? 'animate-spin' : ''}`} />
              {checking ? '检查中…' : '立即检查'}
            </Button>
            <Button size="sm" variant="ghost" asChild>
              <a href={RELEASES_URL} target="_blank" rel="noreferrer noopener">
                全部版本
                <ExternalLink className="ml-1 h-3 w-3" />
              </a>
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base font-medium">关于项目</CardTitle>
          <CardDescription>Reminder 开源仓库与完整文档</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noreferrer noopener"
            className="flex items-center gap-2 text-sm text-primary hover:underline"
          >
            <Github className="h-4 w-4" />
            github.com/hausen1012/reminder
            <ExternalLink className="h-3 w-3" />
          </a>
          <a
            href={DOCS_URL}
            target="_blank"
            rel="noreferrer noopener"
            className="flex items-center gap-2 text-sm text-primary hover:underline"
          >
            <BookOpen className="h-4 w-4" />
            hausen1012.github.io/reminder
            <ExternalLink className="h-3 w-3" />
          </a>
          <p className="text-xs text-muted-foreground">
            遇到问题或有建议，欢迎到 GitHub 提 Issue；具体用法见文档站点。
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
