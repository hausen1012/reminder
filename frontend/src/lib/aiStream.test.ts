import { describe, expect, it, beforeEach, vi, afterEach } from 'vitest'
import { sendAIChatStream } from '@/lib/api'
import type { AIChatProgress } from '@/types'

/**
 * sendAIChatStream 用 fetch + ReadableStream 解析 SSE。
 *
 * 这里手工拼一个分块的响应体，验证三件事：
 *  1. 跨 chunk 的事件能被正确拼接（分块边界可能切在 data 中间）；
 *  2. status / retry 事件透传给 onProgress，done 事件的正文作为返回值；
 *  3. error 事件转成带 error.message 的错误，前端能走 extractApiError。
 */

/** 把若干段文本包装成 fetch 能读的流式响应。 */
function streamResponse(chunks: string[], init?: { status?: number; contentType?: string }) {
  const encoder = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c))
      controller.close()
    },
  })
  return {
    ok: (init?.status ?? 200) < 400,
    status: init?.status ?? 200,
    headers: new Headers({
      'content-type': init?.contentType ?? 'text/event-stream; charset=utf-8',
    }),
    body,
    text: async () => chunks.join(''),
  } as unknown as Response
}

describe('sendAIChatStream', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
    localStorage.setItem('token', 'test-token')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('拼接跨 chunk 事件并返回 done 的正文', async () => {
    fetchMock.mockResolvedValue(
      streamResponse([
        'event: status\ndata: {"phase":"thinking","text":"助手正在思考…"}\n\n',
        // 故意在 JSON 中间断开
        'event: retry\ndata: {"attempt":1,"max":10,"delay_ms":2',
        '00,"reason":"服务繁忙（限流）"}\n\nevent: done\ndata: {"reply":"好了","actions":[],"retries":1}\n\n',
      ]),
    )

    const seen: AIChatProgress[] = []
    const res = await sendAIChatStream('你好', [], 'claude-opus-5-5', (p) => seen.push(p))

    expect(res.reply).toBe('好了')
    expect(res.retries).toBe(1)
    expect(seen).toHaveLength(2)
    expect(seen[0]).toEqual({
      kind: 'status',
      status: { phase: 'thinking', text: '助手正在思考…' },
    })
    expect(seen[1]).toEqual({
      kind: 'retry',
      retry: { attempt: 1, max: 10, delay_ms: 200, reason: '服务繁忙（限流）' },
    })
  })

  it('带上鉴权头与请求体', async () => {
    fetchMock.mockResolvedValue(
      streamResponse(['event: done\ndata: {"reply":"x","actions":[]}\n\n']),
    )

    await sendAIChatStream('消息', [{ role: 'user', content: '上一句' }], 'm')

    const [, init] = fetchMock.mock.calls[0]
    expect(fetchMock.mock.calls[0][0]).toBe('/api/ai/chat/stream')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer test-token')
    expect(JSON.parse(init.body as string)).toEqual({
      message: '消息',
      history: [{ role: 'user', content: '上一句' }],
      model: 'm',
    })
  })

  it('error 事件转成可被 extractApiError 读取的错误', async () => {
    fetchMock.mockResolvedValue(
      streamResponse([
        'event: error\ndata: {"code":"validation_failed","message":"AI 调用失败: 429"}\n\n',
      ]),
    )

    await expect(sendAIChatStream('你好', [], undefined)).rejects.toMatchObject({
      response: {
        data: { error: { code: 'validation_failed', message: 'AI 调用失败: 429' } },
      },
    })
  })

  it('推流前就失败（普通 JSON 错误体）时抛同样结构', async () => {
    fetchMock.mockResolvedValue(
      streamResponse(['{"error":{"code":"validation_failed","message":"消息不能为空"}}'], {
        status: 400,
        contentType: 'application/json',
      }),
    )

    await expect(sendAIChatStream(' ', [], undefined)).rejects.toMatchObject({
      response: {
        status: 400,
        data: { error: { code: 'validation_failed', message: '消息不能为空' } },
      },
    })
  })

  it('没有 done 事件时报错，避免把半截当成功', async () => {
    fetchMock.mockResolvedValue(
      streamResponse(['event: status\ndata: {"phase":"thinking","text":"…"}\n\n']),
    )

    await expect(sendAIChatStream('你好', [], undefined)).rejects.toThrow('未正常结束')
  })
})
