/** 排序箭头图标组件 */
export function SortIcon({ active, direction }: { active: boolean; direction: string }) {
  return (
    <span className="inline-flex flex-col align-middle leading-none ml-1 -mt-0.5">
      <svg className={`h-[7px] w-[9px] ${active && direction === 'desc' ? 'text-foreground' : 'text-muted-foreground/25'}`} viewBox="0 0 10 6" fill="currentColor"><path d="M5 0l5 6H0z" /></svg>
      <svg className={`h-[7px] w-[9px] -mt-[1px] ${active && direction === 'asc' ? 'text-foreground' : 'text-muted-foreground/25'}`} viewBox="0 0 10 6" fill="currentColor"><path d="M5 6L0 0h10z" /></svg>
    </span>
  )
}
