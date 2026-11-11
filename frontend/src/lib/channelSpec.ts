// 通道类型元数据的拉取与模块内缓存。
import { useEffect, useState } from 'react'
import { listChannelTypes } from '@/lib/api'
import type { ChannelFieldSpec, ChannelTypeSpec } from '@/types'

let cache: ChannelTypeSpec[] | null = null
let inflight: Promise<ChannelTypeSpec[]> | null = null
const listeners = new Set<(specs: ChannelTypeSpec[]) => void>()

// loadChannelTypes 拉取类型元数据；已缓存时直接返回。
export function loadChannelTypes(): Promise<ChannelTypeSpec[]> {
  if (cache) return Promise.resolve(cache)
  if (!inflight) {
    inflight = listChannelTypes()
      .then((specs) => {
        cache = specs
        listeners.forEach((fn) => fn(specs))
        return specs
      })
      .finally(() => {
        inflight = null
      })
  }
  return inflight
}

// 测试用：清空缓存，避免用例之间互相影响。
export function resetChannelTypesCache() {
  cache = null
  inflight = null
}

// useChannelTypes 返回类型元数据与加载状态。
export function useChannelTypes(): { specs: ChannelTypeSpec[]; loading: boolean } {
  const [specs, setSpecs] = useState<ChannelTypeSpec[]>(cache ?? [])
  const [loading, setLoading] = useState(!cache)

  useEffect(() => {
    if (cache) return
    let alive = true
    const onLoad = (next: ChannelTypeSpec[]) => {
      if (alive) setSpecs(next)
    }
    listeners.add(onLoad)
    loadChannelTypes()
      .then((next) => {
        if (alive) setSpecs(next)
      })
      .catch(() => {})
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
      listeners.delete(onLoad)
    }
  }, [])

  return { specs, loading }
}

// findSpec 按类型取元数据。
export function findSpec(specs: ChannelTypeSpec[], type: string): ChannelTypeSpec | undefined {
  return specs.find((s) => s.type === type)
}

// labelOf 取类型显示名；未加载到元数据时退回原始类型名。
export function labelOf(specs: ChannelTypeSpec[], type: string): string {
  return findSpec(specs, type)?.label ?? type
}

// defaultConfigOf 按字段定义生成新建通道的初始配置。
export function defaultConfigOf(spec: ChannelTypeSpec | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  if (!spec) return out
  for (const f of spec.fields) {
    out[f.key] = initialValueOf(f)
  }
  return out
}

function initialValueOf(f: ChannelFieldSpec): unknown {
  if (f.default !== undefined && f.default !== null) return f.default
  switch (f.kind) {
    case 'switch':
      return false
    case 'number':
      return 0
    case 'list':
      return []
    case 'select':
      return f.options?.[0]?.value ?? ''
    default:
      return ''
  }
}
