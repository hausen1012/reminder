import { describe, expect, it, beforeEach, vi } from 'vitest'

// mock axios 实例：拦截所有 api.xxx 调用，断言 URL / 参数 / 返回值透传
const mockRequest = vi.fn()
vi.mock('axios', () => {
  const instance = {
    get: (...args: unknown[]) => mockRequest('get', ...args),
    post: (...args: unknown[]) => mockRequest('post', ...args),
    put: (...args: unknown[]) => mockRequest('put', ...args),
    patch: (...args: unknown[]) => mockRequest('patch', ...args),
    delete: (...args: unknown[]) => mockRequest('delete', ...args),
    interceptors: {
      request: { use: vi.fn() },
      response: { use: vi.fn() },
    },
  }
  return {
    default: { create: () => instance },
  }
})

import * as api from '@/lib/api'

// 统一构造响应
function respond(data: unknown) {
  return Promise.resolve({ data })
}

describe('api 认证', () => {
  beforeEach(() => mockRequest.mockReset())

  it('login 提交用户名密码', async () => {
    mockRequest.mockReturnValue(respond({ code: 200, data: { token: 't', user: { id: 1 } } }))
    const res = await api.login('admin', 'admin123')

    expect(mockRequest).toHaveBeenCalledWith(
      'post',
      '/auth/login',
      { username: 'admin', password: 'admin123' },
    )
    expect(res.data.token).toBe('t')
  })

  it('getMe 请求 /auth/me', async () => {
    mockRequest.mockReturnValue(respond({ code: 200, data: { id: 1, username: 'admin' } }))
    const res = await api.getMe()
    expect(mockRequest).toHaveBeenCalledWith('get', '/auth/me')
    expect(res.data.username).toBe('admin')
  })

  it('updatePassword 使用 PUT 并映射字段', async () => {
    mockRequest.mockReturnValue(respond({ code: 200, data: null }))
    await api.updatePassword('old', 'new')
    expect(mockRequest).toHaveBeenCalledWith('put', '/auth/password', {
      old_password: 'old',
      new_password: 'new',
    })
  })

  it('logout 使用 POST', async () => {
    mockRequest.mockReturnValue(respond({ code: 200, data: null }))
    await api.logout()
    expect(mockRequest).toHaveBeenCalledWith('post', '/auth/logout')
  })
})

describe('api 通知渠道', () => {
  beforeEach(() => mockRequest.mockReset())

  it('listChannels 返回数组，data 为 null 时回落空数组', async () => {
    mockRequest.mockReturnValue(respond({ data: null }))
    expect(await api.listChannels()).toEqual([])

    mockRequest.mockReturnValue(respond({ data: [{ id: 1 }] }))
    expect(await api.listChannels()).toEqual([{ id: 1 }])
  })

  it('listChannelsPaged 透传查询参数', async () => {
    mockRequest.mockReturnValue(respond({ data: { items: [], total: 0 } }))
    await api.listChannelsPaged({ limit: 10, offset: 20, search: 'x' })

    expect(mockRequest).toHaveBeenCalledWith('get', '/channels', {
      params: { limit: 10, offset: 20, search: 'x' },
    })
  })

  it('getChannel 拼接 id 路径', async () => {
    mockRequest.mockReturnValue(respond({ data: { id: 7 } }))
    expect(await api.getChannel(7)).toEqual({ id: 7 })
    expect(mockRequest).toHaveBeenCalledWith('get', '/channels/7')
  })

  it('createChannel / updateChannel / deleteChannel / toggleChannel', async () => {
    mockRequest.mockReturnValue(respond({ data: { id: 1 } }))
    const input = { name: 'n', type: 'log' as const, config: {} }

    await api.createChannel(input)
    expect(mockRequest).toHaveBeenCalledWith('post', '/channels', input)

    await api.updateChannel(1, input)
    expect(mockRequest).toHaveBeenCalledWith('put', '/channels/1', input)

    await api.deleteChannel(1)
    expect(mockRequest).toHaveBeenCalledWith('delete', '/channels/1')

    await api.toggleChannel(1)
    expect(mockRequest).toHaveBeenCalledWith('patch', '/channels/1/toggle')
  })

  it('testChannelDryRun 提交配置', async () => {
    mockRequest.mockReturnValue(respond({ data: { success: true } }))
    const res = await api.testChannelDryRun({ type: 'log', config: {} })
    expect(mockRequest).toHaveBeenCalledWith('post', '/channels/test-dry', {
      type: 'log',
      config: {},
    })
    expect(res).toEqual({ success: true })
  })
})

describe('api 提醒', () => {
  beforeEach(() => mockRequest.mockReset())

  it('listReminders 透传过滤参数', async () => {
    mockRequest.mockReturnValue(respond({ data: { items: [], total: 0 } }))
    await api.listReminders({ source: 'web', enabled: true, limit: 5 })

    expect(mockRequest).toHaveBeenCalledWith('get', '/reminders', {
      params: { source: 'web', enabled: true, limit: 5 },
    })
  })

  it('提醒 CRUD 路径正确', async () => {
    mockRequest.mockReturnValue(respond({ data: { id: 1 } }))
    const input = { title: 't', schedule_type: 'once' as const, calendar: 'solar' as const }

    await api.getReminder(1)
    expect(mockRequest).toHaveBeenCalledWith('get', '/reminders/1')

    await api.createReminder(input as never)
    expect(mockRequest).toHaveBeenCalledWith('post', '/reminders', input)

    await api.updateReminder(1, input as never)
    expect(mockRequest).toHaveBeenCalledWith('put', '/reminders/1', input)

    await api.deleteReminder(1)
    expect(mockRequest).toHaveBeenCalledWith('delete', '/reminders/1')

    await api.toggleReminder(1)
    expect(mockRequest).toHaveBeenCalledWith('patch', '/reminders/1/toggle')
  })

  it('batchDeleteReminders 用 data 字段传 ids', async () => {
    mockRequest.mockReturnValue(respond({ data: null }))
    await api.batchDeleteReminders([1, 2])

    expect(mockRequest).toHaveBeenCalledWith('delete', '/reminders/batch', {
      data: { ids: [1, 2] },
    })
  })
})

describe('api 日志', () => {
  beforeEach(() => mockRequest.mockReset())

  it('listLogs / getLogDetail', async () => {
    mockRequest.mockReturnValue(respond({ data: { items: [], total: 0 } }))
    await api.listLogs({ status: 'success' })
    expect(mockRequest).toHaveBeenCalledWith('get', '/logs', {
      params: { status: 'success' },
    })

    mockRequest.mockReturnValue(respond({ data: { id: 3 } }))
    await api.getLogDetail(3)
    expect(mockRequest).toHaveBeenCalledWith('get', '/logs/3')
  })

  it('purgeLogs / countPurgeLogs 透传参数', async () => {
    mockRequest.mockReturnValue(respond({ data: { deleted: 1 } }))
    await api.purgeLogs('30d', false)
    expect(mockRequest).toHaveBeenCalledWith('delete', '/logs', {
      params: { older_than: '30d', all: false },
    })

    mockRequest.mockReturnValue(respond({ data: { count: 5 } }))
    await api.countPurgeLogs(undefined, true)
    expect(mockRequest).toHaveBeenCalledWith('get', '/logs/count', {
      params: { older_than: undefined, all: true },
    })
  })

  it('batchDeleteLogs 传 ids', async () => {
    mockRequest.mockReturnValue(respond({ data: null }))
    await api.batchDeleteLogs([9])
    expect(mockRequest).toHaveBeenCalledWith('delete', '/logs/batch', { data: { ids: [9] } })
  })
})

describe('api 令牌', () => {
  beforeEach(() => mockRequest.mockReset())

  it('listTokens 空响应回落默认结构', async () => {
    mockRequest.mockReturnValue(respond({ data: null }))
    expect(await api.listTokens()).toEqual({ items: [], total: 0 })
  })

  it('getTokenPlaintext 取出 plaintext', async () => {
    mockRequest.mockReturnValue(respond({ data: { plaintext: 'bdrk_x' } }))
    expect(await api.getTokenPlaintext(1)).toBe('bdrk_x')
    expect(mockRequest).toHaveBeenCalledWith('get', '/tokens/1/plaintext')
  })

  it('createToken 默认通道为空数组', async () => {
    mockRequest.mockReturnValue(respond({ data: { plaintext: 'p', key: { id: 1 } } }))
    await api.createToken('名称')
    expect(mockRequest).toHaveBeenCalledWith('post', '/tokens', {
      name: '名称',
      default_channel_ids: [],
    })
  })

  it('updateTokenChannels 用 PUT', async () => {
    mockRequest.mockReturnValue(respond({ data: null }))
    await api.updateTokenChannels(1, [2, 3])
    expect(mockRequest).toHaveBeenCalledWith('put', '/tokens/1/channels', {
      channel_ids: [2, 3],
    })
  })
})

describe('api 监控与配置', () => {
  beforeEach(() => mockRequest.mockReset())

  it('listTodayReminders 带 scope=today', async () => {
    mockRequest.mockReturnValue(respond({ data: [] }))
    await api.listTodayReminders()
    expect(mockRequest).toHaveBeenCalledWith('get', '/reminders/upcoming', {
      params: { scope: 'today' },
    })
  })

  it('listUpcomingReminders 默认 24h/10', async () => {
    mockRequest.mockReturnValue(respond({ data: [] }))
    await api.listUpcomingReminders()
    expect(mockRequest).toHaveBeenCalledWith('get', '/reminders/upcoming', {
      params: { within: '24h', limit: 10 },
    })
  })

  it('getConfig 空响应回落空对象', async () => {
    mockRequest.mockReturnValue(respond({ data: null }))
    expect(await api.getConfig()).toEqual({})
  })

  it('updateConfig 返回全量配置', async () => {
    mockRequest.mockReturnValue(respond({ data: { app_name: 'x' } }))
    const res = await api.updateConfig({ app_name: 'x' })
    expect(mockRequest).toHaveBeenCalledWith('put', '/config', { app_name: 'x' })
    expect(res).toEqual({ app_name: 'x' })
  })

  it('getSchedulerStatus 透传分页参数', async () => {
    mockRequest.mockReturnValue(respond({ data: {} }))
    await api.getSchedulerStatus({ limit: 10, offset: 0 })
    expect(mockRequest).toHaveBeenCalledWith('get', '/scheduler/status', {
      params: { limit: 10, offset: 0 },
    })
  })
})
