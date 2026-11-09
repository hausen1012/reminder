import type { ElementType, ReactNode } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface EmptyStateAction {
  label: string
  onClick: () => void
  icon?: ElementType
  variant?: 'default' | 'outline' | 'secondary'
}

export interface EmptyStateProps {
  /** 顶部展示的图标组件（来自 lucide-react） */
  icon: ElementType
  /** 主标题 */
  title: string
  /** 副描述文案或自定义节点 */
  description?: ReactNode
  /** 主操作按钮配置 */
  action?: EmptyStateAction
  /** 次要操作按钮配置 */
  secondaryAction?: EmptyStateAction
  /** 额外的容器 className */
  className?: string
  /**
   * 展示形态：
   * - 'card': 独立大卡片，适合主页面内容区（默认）
   * - 'dashed': 虚线内嵌框，适合放置在已有 CardContent 或弹窗中
   * - 'plain': 无边框纯内容，适合紧凑容器
   */
  variant?: 'card' | 'dashed' | 'plain'
  /** 尺寸控制：'default' 适合主页面，'compact' 适合仪表盘卡片或弹窗内部 */
  size?: 'default' | 'compact'
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  secondaryAction,
  className,
  variant = 'card',
  size = 'default',
}: EmptyStateProps) {
  const isCompact = size === 'compact'
  const ActionIcon = action?.icon
  const SecondaryActionIcon = secondaryAction?.icon

  const content = (
    <div
      className={cn(
        'flex flex-col items-center justify-center text-center select-none',
        isCompact
          ? 'min-h-[180px] px-4 py-8'
          : 'min-h-[280px] md:min-h-[340px] px-6 py-12 md:py-16',
      )}
    >
      <div
        className={cn(
          'flex items-center justify-center rounded-2xl bg-muted/70 ring-1 ring-border/50 text-muted-foreground/80 shadow-xs',
          isCompact ? 'mb-3 h-10 w-10 rounded-xl' : 'mb-4 h-14 w-14',
        )}
      >
        <Icon className={cn(isCompact ? 'h-5 w-5' : 'h-7 w-7 text-muted-foreground/80')} />
      </div>

      <h3
        className={cn(
          'font-medium text-foreground tracking-tight',
          isCompact ? 'text-sm' : 'text-base md:text-lg font-semibold',
        )}
      >
        {title}
      </h3>

      {description && (
        <p
          className={cn(
            'text-muted-foreground leading-relaxed',
            isCompact ? 'mt-1 text-xs max-w-xs' : 'mt-1.5 text-xs md:text-sm max-w-sm md:max-w-md',
          )}
        >
          {description}
        </p>
      )}

      {(action || secondaryAction) && (
        <div
          className={cn(
            'flex flex-wrap items-center justify-center gap-2',
            isCompact ? 'mt-3' : 'mt-5',
          )}
        >
          {action && (
            <Button
              size="sm"
              variant={action.variant ?? 'default'}
              onClick={action.onClick}
              className={isCompact ? 'h-8 px-3 text-xs' : 'h-9 px-4 text-xs md:text-sm'}
            >
              {ActionIcon && <ActionIcon className="mr-1.5 h-3.5 w-3.5" />}
              {action.label}
            </Button>
          )}
          {secondaryAction && (
            <Button
              size="sm"
              variant={secondaryAction.variant ?? 'outline'}
              onClick={secondaryAction.onClick}
              className={isCompact ? 'h-8 px-3 text-xs' : 'h-9 px-4 text-xs md:text-sm'}
            >
              {SecondaryActionIcon && <SecondaryActionIcon className="mr-1.5 h-3.5 w-3.5" />}
              {secondaryAction.label}
            </Button>
          )}
        </div>
      )}
    </div>
  )

  if (variant === 'plain') {
    return <div className={className}>{content}</div>
  }

  if (variant === 'dashed') {
    return (
      <div
        className={cn(
          'rounded-lg border border-dashed border-border/80 bg-muted/20',
          className,
        )}
      >
        {content}
      </div>
    )
  }

  // 默认：独立 Card 形态
  return (
    <Card className={cn('border border-border/60 bg-card shadow-sm', className)}>
      <CardContent className="p-0">{content}</CardContent>
    </Card>
  )
}
