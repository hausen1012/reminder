// 分页组件
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface Props {
  total: number
  limit: number
  offset: number
  onPageChange: (offset: number) => void
}

export function Pagination({ total, limit, offset, onPageChange }: Props) {
  const totalPages = Math.max(1, Math.ceil(total / limit))
  const currentPage = Math.floor(offset / limit) + 1

  if (totalPages <= 1) return null

  function goTo(page: number) {
    onPageChange((page - 1) * limit)
  }

  function pageNumbers(): (number | 'ellipsis')[] {
    const pages: (number | 'ellipsis')[] = []
    if (totalPages <= 5) {
      for (let i = 1; i <= totalPages; i++) pages.push(i)
      return pages
    }
    pages.push(1)
    if (currentPage > 3) pages.push('ellipsis')
    const start = Math.max(2, currentPage - 1)
    const end = Math.min(totalPages - 1, currentPage + 1)
    for (let i = start; i <= end; i++) pages.push(i)
    if (currentPage < totalPages - 2) pages.push('ellipsis')
    pages.push(totalPages)
    return pages
  }

  return (
    <div className="flex items-center justify-center gap-1 pt-4">
      <Button
        variant="ghost"
        size="sm"
        disabled={currentPage <= 1}
        onClick={() => goTo(currentPage - 1)}
      >
        <ChevronLeft className="h-4 w-4" />
      </Button>
      {pageNumbers().map((p, i) =>
        p === 'ellipsis' ? (
          <span key={`e-${i}`} className="px-1 text-xs text-muted-foreground">…</span>
        ) : (
          <Button
            key={p}
            variant={p === currentPage ? 'default' : 'ghost'}
            size="sm"
            className="min-w-[2rem]"
            onClick={() => goTo(p)}
          >
            {p}
          </Button>
        ),
      )}
      <Button
        variant="ghost"
        size="sm"
        disabled={currentPage >= totalPages}
        onClick={() => goTo(currentPage + 1)}
      >
        <ChevronRight className="h-4 w-4" />
      </Button>
      <span className="ml-2 text-xs text-muted-foreground">共 {total} 条</span>
    </div>
  )
}