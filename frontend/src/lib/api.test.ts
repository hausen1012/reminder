import { describe, expect, it } from 'vitest'
import { extractApiError } from '@/lib/api'

describe('extractApiError', () => {
  it('优先取 data.message', () => {
    const err = { response: { data: { message: '通道名称已存在' } } }
    expect(extractApiError(err)).toBe('通道名称已存在')
  })

  it('兼容 { error: { message } } 结构', () => {
    const err = { response: { data: { error: { message: '令牌无效' } } } }
    expect(extractApiError(err)).toBe('令牌无效')
  })

  it('无响应体时回退到 Error.message', () => {
    expect(extractApiError(new Error('网络错误'))).toBe('网络错误')
  })

  it('完全无法识别时用默认文案', () => {
    expect(extractApiError(null, '操作失败')).toBe('操作失败')
    expect(extractApiError({}, '操作失败')).toBe('操作失败')
    expect(extractApiError(42, '操作失败')).toBe('操作失败')
  })

  it('响应体结构异常时不抛错', () => {
    expect(extractApiError({ response: { data: { error: 'oops' } } }, '兜底')).toBe('兜底')
    expect(extractApiError({ response: { data: 'plain string' } }, '兜底')).toBe('兜底')
  })
})
