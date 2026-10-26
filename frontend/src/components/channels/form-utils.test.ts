import { describe, expect, it } from 'vitest'
import { cfgArr, cfgNum, cfgObj, cfgStr } from '@/components/channels/form-utils'

describe('cfgStr', () => {
  it('返回字符串值', () => {
    expect(cfgStr({ a: 'x' }, 'a')).toBe('x')
  })

  it('非字符串时返回 fallback', () => {
    expect(cfgStr({ a: 1 }, 'a')).toBe('')
    expect(cfgStr({ a: null }, 'a', 'def')).toBe('def')
    expect(cfgStr({}, 'missing', 'def')).toBe('def')
  })
})

describe('cfgNum', () => {
  it('返回数字值', () => {
    expect(cfgNum({ p: 587 }, 'p', 25)).toBe(587)
  })

  it('非数字时返回 fallback', () => {
    expect(cfgNum({ p: '587' }, 'p', 25)).toBe(25)
    expect(cfgNum({ p: null }, 'p', 25)).toBe(25)
    expect(cfgNum({}, 'p', 25)).toBe(25)
  })

  it('数字 0 应被保留（不是 fallback）', () => {
    expect(cfgNum({ p: 0 }, 'p', 25)).toBe(0)
  })
})

describe('cfgArr', () => {
  it('返回数组', () => {
    expect(cfgArr<string>({ to: ['a', 'b'] }, 'to')).toEqual(['a', 'b'])
  })

  it('非数组时返回空数组', () => {
    expect(cfgArr({ to: 'a' }, 'to')).toEqual([])
    expect(cfgArr({}, 'to')).toEqual([])
  })

  it('支持自定义 fallback', () => {
    expect(cfgArr({}, 'to', ['x'])).toEqual(['x'])
  })

  it('空数组应原样返回（不被 fallback 覆盖）', () => {
    expect(cfgArr({ to: [] }, 'to', ['x'])).toEqual([])
  })
})

describe('cfgObj', () => {
  it('返回对象', () => {
    expect(cfgObj({ h: { a: '1' } }, 'h')).toEqual({ a: '1' })
  })

  it('数组不算对象', () => {
    expect(cfgObj({ h: ['a'] }, 'h')).toEqual({})
  })

  it('null / 非对象返回 fallback', () => {
    expect(cfgObj({ h: null }, 'h')).toEqual({})
    expect(cfgObj({ h: 'str' }, 'h')).toEqual({})
    expect(cfgObj({}, 'h', { d: 'v' })).toEqual({ d: 'v' })
  })
})
