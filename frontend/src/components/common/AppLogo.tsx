// Logo 公共组件：支持 data URL（图片）和 SVG 代码两种格式
interface Props {
  svg: string
  className?: string
  alt?: string
}

export function AppLogo({ svg, className = 'h-5 w-auto shrink-0', alt = 'Logo' }: Props) {
  if (!svg) return null
  if (svg.startsWith('data:image/')) {
    return <img src={svg} alt={alt} className={className} />
  }
  return (
    <span
      className="flex h-5 w-5 shrink-0 items-center justify-center"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}