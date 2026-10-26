import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PageHeader } from '@/components/ui/PageHeader'
import { SortIcon } from '@/components/ui/SortIcon'
import { AppLogo } from '@/components/ui/AppLogo'

describe('PageHeader', () => {
  it('渲染标题', () => {
    render(<PageHeader title="提醒列表" />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('提醒列表')
  })

  it('有 children 时渲染右侧操作区', () => {
    render(
      <PageHeader title="标题">
        <button>新建</button>
      </PageHeader>,
    )
    expect(screen.getByRole('button', { name: '新建' })).toBeInTheDocument()
  })

  it('无 children 时不渲染额外容器', () => {
    const { container } = render(<PageHeader title="标题" />)
    // 只有 h1 一个子元素
    expect(container.firstChild?.childNodes).toHaveLength(1)
  })
})

describe('SortIcon', () => {
  it('未激活时两个箭头都是弱色', () => {
    const { container } = render(<SortIcon active={false} direction="asc" />)
    const svgs = container.querySelectorAll('svg')
    expect(svgs).toHaveLength(2)
    svgs.forEach((svg) => {
      expect(svg.getAttribute('class')).toContain('text-muted-foreground/25')
    })
  })

  it('asc 方向时下方箭头高亮', () => {
    const { container } = render(<SortIcon active direction="asc" />)
    const svgs = container.querySelectorAll('svg')
    // 第 2 个（下方/向上箭头）高亮
    expect(svgs[1].getAttribute('class')).toContain('text-foreground')
    expect(svgs[0].getAttribute('class')).toContain('text-muted-foreground/25')
  })

  it('desc 方向时上方箭头高亮', () => {
    const { container } = render(<SortIcon active direction="desc" />)
    const svgs = container.querySelectorAll('svg')
    expect(svgs[0].getAttribute('class')).toContain('text-foreground')
    expect(svgs[1].getAttribute('class')).toContain('text-muted-foreground/25')
  })
})

describe('AppLogo', () => {
  it('svg 为空时渲染 null', () => {
    const { container } = render(<AppLogo svg="" />)
    expect(container.firstChild).toBeNull()
  })

  it('data URL 时渲染 img', () => {
    render(<AppLogo svg="data:image/png;base64,AAAA" alt="站点图标" />)
    const img = screen.getByRole('img', { name: '站点图标' })
    expect(img).toHaveAttribute('src', 'data:image/png;base64,AAAA')
  })

  it('SVG 代码时内联渲染', () => {
    const { container } = render(<AppLogo svg='<svg data-testid="inline-svg"></svg>' />)
    expect(container.querySelector('[data-testid="inline-svg"]')).not.toBeNull()
  })
})
