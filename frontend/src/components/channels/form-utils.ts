// 各类型表单的公共类型与小工具
import type { Dispatch, SetStateAction } from 'react'

export interface SubFormProps {
  config: Record<string, unknown>
  onChange: Dispatch<SetStateAction<Record<string, unknown>>>
  isEdit: boolean
}

export function updateField<T>(
  onChange: SubFormProps['onChange'],
  key: string,
  value: T,
) {
  onChange((prev) => ({ ...prev, [key]: value }))
}

/** 从 config 中安全读取字符串字段 */
export function cfgStr(config: Record<string, unknown>, key: string, fallback = ''): string {
  const v = config[key]
  return typeof v === 'string' ? v : fallback
}

/** 从 config 中安全读取数字字段 */
export function cfgNum(config: Record<string, unknown>, key: string, fallback: number): number {
  const v = config[key]
  return typeof v === 'number' ? v : fallback
}

/** 从 config 中安全读取数组字段 */
export function cfgArr<T>(config: Record<string, unknown>, key: string, fallback: T[] = []): T[] {
  const v = config[key]
  return Array.isArray(v) ? v as T[] : fallback
}

/** 从 config 中安全读取对象字段 */
export function cfgObj(config: Record<string, unknown>, key: string, fallback: Record<string, string> = {}): Record<string, string> {
  const v = config[key]
  return v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, string> : fallback
}
