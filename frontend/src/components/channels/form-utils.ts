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

// 服务端在脱敏视图中把 _enc 字段统一替换为该占位；编辑时若用户未改动
// 敏感字段，前端把该占位回填到 payload，后端识别后保留原密文。
export const SECRET_PLACEHOLDER = '***'
