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
import { copyToClipboard } from '@/lib/utils'
import { extractApiError } from '@/lib/api'
import { wallClockAfter } from '@/lib/datetime'
import { useSiteTimezone } from '@/contexts/ConfigContext'

interface Props {
  token: string
  open: boolean
  onClose: () => void
}

async function copy(text: string, toast: ReturnType<typeof useToast>['toast']) {
  try {
    await copyToClipboard(text)
    toast({ title: '已复制' })
  } catch (err) {
    toast({ title: '复制失败', description: extractApiError(err), variant: 'destructive' })
  }
}

function CurlBlock({ curl }: { curl: string }) {
  const { toast } = useToast()
  return (
    <div className="relative mt-2">
      <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg border bg-muted p-3 pr-10 text-xs leading-relaxed sm:p-4 sm:pr-12">
        <code>{curl}</code>
      </pre>
      <Button
        variant="ghost"
        size="icon"
        className="absolute right-1 top-1.5 h-7 w-7 text-muted-foreground hover:text-foreground sm:right-2 sm:top-2"
        onClick={() => copy(curl, toast)}
        title="复制 curl 命令"
      >
        <Copy className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}

export function CurlUsageDialog({ token, open, onClose }: Props) {
  const tz = useSiteTimezone()
  // 示例里的 at 按站点时区生成，与后端解释一致
  const at = useMemo(() => wallClockAfter(tz, 60), [tz])

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
  "channel_ids": [1],
  "require_confirm": false
}'`,
    [token],
  )

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl md:max-h-[90dvh] md:overflow-y-auto">
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
            <CurlBlock curl={curlSimple} />
          </TabsContent>
          <TabsContent value="once">
            <CurlBlock curl={curlOnce} />
          </TabsContent>
          <TabsContent value="interval">
            <CurlBlock curl={curlInterval} />
          </TabsContent>
          <TabsContent value="cron">
            <CurlBlock curl={curlCron} />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}