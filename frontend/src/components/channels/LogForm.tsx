// 日志通知表单 — 无需额外配置
import { type SubFormProps } from './form-utils'

export function LogForm({}: SubFormProps) {
  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <p className="text-sm text-muted-foreground">
        日志通知无需额外配置。该通知仅将内容输出到服务器控制台，不会调用任何外部服务。
      </p>
    </div>
  )
}
