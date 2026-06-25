// curl 使用示例折叠面板
import { useMemo } from 'react'
import { Copy, Terminal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useToast } from '@/components/ui/use-toast'

interface Props {
  token: string
  open: boolean
  onClose: () => void
}

function oneHourLater(): string {
  const d = new Date(Date.now() + 3600_000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:00`
}

function copy(text: string, toast: ReturnType<typeof useToast>['toast']) {
  navigator.clipboard.writeText(text)
  toast({ title: '已复制' })
}

function CurlBlock({ curl, label }: { curl: string; label: string }) {
  const { toast } = useToast()
  return (
    <div className="relative">
      <div className="mb-2 text-xs text-muted-foreground">{label}</div>
      <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg border bg-slate-950 p-3 text-xs leading-relaxed text-slate-50 sm:p-4">
        <code>{curl}</code>
      </pre>
      <Button
        variant="ghost"
        size="icon"
        className="absolute right-1 top-7 h-7 w-7 text-slate-400 hover:text-slate-50 sm:right-2 sm:top-8"
        onClick={() => copy(curl, toast)}
        title="复制 curl 命令"
      >
        <Copy className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}

export function CurlUsageDialog({ token, open, onClose }: Props) {
  const at = useMemo(() => oneHourLater(), [open])

  const curlSimple = useMemo(
    () => `curl -X POST /api/external/v1/reminders \\
  -H "X-AUTH: ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "title": "提醒标题",
  "content": "提醒内容",
  "schedule_spec": {"at": "${at}"}
}'`,
    [token, at],
  )

  const curlOnce = useMemo(
    () => `curl -X POST /api/external/v1/reminders \\
  -H "X-AUTH: ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "title": "提醒标题",
  "content": "提醒内容",
  "schedule_spec": {"at": "${at}"},
  "timezone": "Asia/Shanghai",
  "channel_ids": [1],
  "require_confirm": false
}'`,
    [token, at],
  )

  const curlInterval = useMemo(
    () => `curl -X POST /api/external/v1/reminders \\
  -H "X-AUTH: ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "title": "提醒标题",
  "content": "提醒内容",
  "schedule_type": "interval",
  "schedule_spec": {"every": 1, "unit": "hour"},
  "timezone": "Asia/Shanghai",
  "channel_ids": [1],
  "require_confirm": false
}'`,
    [token],
  )

  const curlCron = useMemo(
    () => `curl -X POST /api/external/v1/reminders \\
  -H "X-AUTH: ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{
  "title": "提醒标题",
  "content": "提醒内容",
  "schedule_type": "cron",
  "schedule_spec": {"expr": "0 * * * *"},
  "timezone": "Asia/Shanghai",
  "channel_ids": [1],
  "require_confirm": false
}'`,
    [token],
  )

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl sm:max-w-2xl max-sm:max-w-[calc(100%-1rem)] max-sm:p-4">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Terminal className="h-5 w-5" />
            curl 使用示例
          </DialogTitle>
        </DialogHeader>
        <Tabs defaultValue="simple" className="w-full">
          <TabsList className="w-full grid grid-cols-4 h-auto">
            <TabsTrigger value="simple" className="text-xs sm:text-sm px-1 sm:px-3 py-1.5">简单</TabsTrigger>
            <TabsTrigger value="once" className="text-xs sm:text-sm px-1 sm:px-3 py-1.5">单次</TabsTrigger>
            <TabsTrigger value="interval" className="text-xs sm:text-sm px-1 sm:px-3 py-1.5">周期</TabsTrigger>
            <TabsTrigger value="cron" className="text-xs sm:text-sm px-1 sm:px-3 py-1.5">CRON</TabsTrigger>
          </TabsList>
          <TabsContent value="simple">
            <CurlBlock
              curl={curlSimple}
              label="仅包含标题、内容和时间，适合快速测试。"
            />
          </TabsContent>
          <TabsContent value="once">
            <CurlBlock
              curl={curlOnce}
              label="完整的单次提醒，可指定时区、通知渠道等。"
            />
          </TabsContent>
          <TabsContent value="interval">
            <CurlBlock
              curl={curlInterval}
              label="每小时重复提醒，支持自定义间隔和单位。"
            />
          </TabsContent>
          <TabsContent value="cron">
            <CurlBlock
              curl={curlCron}
              label="使用 Cron 表达式，每小时执行一次。"
            />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}