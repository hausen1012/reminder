// 列表页加载与失败占位，四个列表页共用同一套结构，避免筛选时整块表格跳动。
import { AlertCircle, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

export function ListLoading() {
  return (
    <Card>
      <CardContent className="flex items-center justify-center gap-2 py-16 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        加载中…
      </CardContent>
    </Card>
  )
}

interface ListErrorProps {
  message: string
  onRetry: () => void
}

export function ListError({ message, onRetry }: ListErrorProps) {
  return (
    <Card>
      <CardContent className="py-10 text-center text-sm">
        <div className="flex items-center justify-center gap-2 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>加载失败：{message}</span>
        </div>
        <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
          重试
        </Button>
      </CardContent>
    </Card>
  )
}