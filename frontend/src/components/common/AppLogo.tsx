// Logo 公共组件：支持 data URL（图片）和 SVG 代码两种格式
import { cn } from '@/lib/utils'

interface Props {
  svg: string
  className?: string
  alt?: string
}

// 两种格式都按调用方传入的 className 控制尺寸；内联 SVG 撑满容器高度，宽度按原始比例且不超出容器
export function AppLogo({ svg, className = 'h-5 w-auto shrink-0', alt = 'Logo' }: Props) {
  if (!svg) return null
  if (svg.startsWith('data:image/')) {
    return <img src={svg} alt={alt} className={className} />
  }
  return (
    <span
      className={cn('inline-flex items-center justify-center [&>svg]:h-full [&>svg]:w-auto [&>svg]:max-w-full', className)}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}