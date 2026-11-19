/** 排序箭头图标组件（带视觉中线补偿） */
export function SortIcon({ active, direction }: { active: boolean; direction: string }) {
  return (
    <span className="inline-flex flex-col justify-center ml-1.5 shrink-0 -translate-y-[0.5px] align-middle">
      <svg className={`h-[6.5px] w-[8px] ${active && direction === 'desc' ? 'text-foreground' : 'text-muted-foreground/30'}`} viewBox="0 0 10 6" fill="currentColor"><path d="M5 0l5 6H0z" /></svg>
      <svg className={`h-[6.5px] w-[8px] mt-[1.5px] ${active && direction === 'asc' ? 'text-foreground' : 'text-muted-foreground/30'}`} viewBox="0 0 10 6" fill="currentColor"><path d="M5 6L0 0h10z" /></svg>
    </span>
  )
}
