// 通用筛选栏：Select 筛选器 + 搜索框 + 刷新按钮
import type { ReactNode, FormEvent } from 'react'
import { Search, RefreshCw } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

interface Props {
  searchValue: string
  onSearchChange: (value: string) => void
  onSearch: () => void
  onRefresh: () => void
  placeholder?: string
  children?: ReactNode
}

export function FilterToolbar({
  searchValue,
  onSearchChange,
  onSearch,
  onRefresh,
  placeholder = '搜索…',
  children,
}: Props) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {children}
      <form
        className="flex-1 min-w-0 flex gap-2 md:max-w-sm"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          onSearch()
        }}
      >
        <Input
          placeholder={placeholder}
          value={searchValue}
          onChange={(e) => onSearchChange(e.target.value)}
          className="flex-1 min-w-0"
        />
        <Button type="submit" variant="outline" size="icon" title="搜索">
          <Search className="h-4 w-4" />
        </Button>
      </form>
      <Button variant="outline" size="icon" onClick={onRefresh} title="刷新" className="ml-auto shrink-0">
        <RefreshCw className="h-4 w-4" />
      </Button>
    </div>
  )
}
