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
