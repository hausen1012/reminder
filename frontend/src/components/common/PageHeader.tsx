// 页面标题组件，统一移动端垂直/桌面端水平布局
import type { ReactNode } from 'react'

interface Props {
  title: string
  children?: ReactNode
}

export function PageHeader({ title, children }: Props) {
  return (
    <div className="flex flex-col items-stretch gap-3 md:flex-row md:items-center md:justify-between md:gap-2">
      <h1 className="text-2xl font-bold tracking-tight md:text-3xl">{title}</h1>
      {children && <div className="shrink-0">{children}</div>}
    </div>
  )
}
