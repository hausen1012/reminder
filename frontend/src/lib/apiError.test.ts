import { describe, expect, it } from 'vitest'
import { extractApiError, extractApiErrorCode, extractApiErrorField } from '@/lib/api'

// 后端统一错误格式：{ error: { code, message, field? } }
function unifiedError(code: string, message: string, field?: string) {
  return {
    response: {
      data: {
        error: { code, message, ...(field ? { field } : {}) },
      },
    },
  }
}

describe('extractApiError', () => {
  it('读取统一格式的 error.message', () => {
    expect(extractApiError(unifiedError('not_found', '提醒不存在'))).toBe('提醒不存在')
  })

  it('统一格式优先于旧格式', () => {
    // 同时存在两种结构时，必须取 error.message
    const err = {
      response: {
        data: {
          error: { code: 'validation_failed', message: '新格式消息' },
          message: '旧格式消息',
        },
      },
    }
    expect(extractApiError(err)).toBe('新格式消息')
  })

  it('兼容旧格式 data.message', () => {
    const err = { response: { data: { code: 400, message: '旧格式', data: null } } }
    expect(extractApiError(err)).toBe('旧格式')
  })

  it('error.message 为空串时回退到旧格式', () => {
    const err = {
      response: { data: { error: { code: 'x', message: '' }, message: '兜底' } },
    }
    expect(extractApiError(err)).toBe('兜底')
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

describe('extractApiErrorCode', () => {
  it('取出统一格式的 error.code', () => {
    expect(extractApiErrorCode(unifiedError('validation_failed', '必填'))).toBe('validation_failed')
  })

  it('各错误码都能取出', () => {
    for (const code of [
      'unauthorized',
      'forbidden',
      'not_found',
      'validation_failed',
      'conflict',
      'rate_limited',
      'internal_error',
    ]) {
      expect(extractApiErrorCode(unifiedError(code, 'x'))).toBe(code)
    }
  })

  it('旧格式（顶层 code 为数字）不误判为错误码', () => {
    const err = { response: { data: { code: 400, message: '旧格式' } } }
    expect(extractApiErrorCode(err)).toBe('')
  })

  it('无响应体时返回空串', () => {
    expect(extractApiErrorCode(null)).toBe('')
    expect(extractApiErrorCode(new Error('x'))).toBe('')
  })
})

describe('extractApiErrorField', () => {
  it('取出 error.field', () => {
    expect(extractApiErrorField(unifiedError('validation_failed', '必填', 'title'))).toBe('title')
  })

  it('无 field 时返回空串', () => {
    expect(extractApiErrorField(unifiedError('not_found', '不存在'))).toBe('')
  })

  it('字段名带嵌套路径时可原样取出', () => {
    const err = unifiedError('validation_failed', '缺少', 'config.host')
    expect(extractApiErrorField(err)).toBe('config.host')
  })
})
