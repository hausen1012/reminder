// 分页组件
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface Props {
  total: number
  limit: number
  offset: number
  onPageChange: (offset: number) => void
  onLimitChange?: (limit: number) => void
  limitOptions?: number[]
}

export function Pagination({
  total,
  limit,
  offset,
  onPageChange,
  onLimitChange,
  limitOptions = [10, 20, 50, 100],
}: Props) {
  const totalPages = Math.max(1, Math.ceil(total / limit))
  const currentPage = Math.min(Math.floor(offset / limit) + 1, totalPages)
  const start = total === 0 ? 0 : offset + 1
  const end = Math.min(offset + limit, total)
  const canChangeLimit = Boolean(onLimitChange)

  function goTo(page: number) {
    const nextPage = Math.max(1, Math.min(page, totalPages))
    onPageChange((nextPage - 1) * limit)
  }

  function handleLimitChange(value: string) {
    if (!onLimitChange) return
    const nextLimit = Number(value)
    if (!Number.isFinite(nextLimit) || nextLimit <= 0) return
    const nextOffset = total === 0 ? 0 : Math.floor(offset / nextLimit) * nextLimit
    onLimitChange(nextLimit)
    onPageChange(nextOffset)
  }

  return (
    <div className="flex flex-col gap-1.5 border-t px-4 py-2 text-sm md:flex-row md:items-center md:justify-between">
      <div className="text-muted-foreground">
        共 {total} 条&nbsp;&nbsp;第 {start}-{end} 条
      </div>
      <div className="flex flex-wrap items-center gap-1.5 md:justify-end">
        {canChangeLimit && (
          <div className="flex items-center gap-1 text-muted-foreground">
            <span>每页</span>
            <Select value={String(limit)} onValueChange={handleLimitChange}>
              <SelectTrigger className="h-8 w-[72px] px-2 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {limitOptions.map((option) => (
                  <SelectItem key={option} value={String(option)}>
                    {option}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="flex items-center gap-0.5">
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            disabled={currentPage <= 1}
            onClick={() => goTo(currentPage - 1)}
            aria-label="上一页"
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="min-w-[3rem] text-center text-foreground">
            {currentPage} / {totalPages}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            disabled={currentPage >= totalPages}
            onClick={() => goTo(currentPage + 1)}
            aria-label="下一页"
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  )
}
