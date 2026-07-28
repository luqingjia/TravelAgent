import { describe, expect, it } from 'vitest'

import { parseSseFrame, parseSseStream, parseSseText } from './sse'

describe('parseSseFrame', () => {
  it('parses event name and data', () => {
    const event = parseSseFrame('event: message\ndata: {"content":"hi"}')
    expect(event).toEqual({ event: 'message', data: '{"content":"hi"}' })
  })

  it('joins multi-line data with newline', () => {
    const event = parseSseFrame('event: message\ndata: line1\ndata: line2')
    expect(event?.data).toBe('line1\nline2')
  })
})

describe('parseSseText', () => {
  it('parses multiple events and ignores incomplete trailing frame', () => {
    const text =
      'event: message\ndata: {"content":"a"}\n\n' +
      'event: message\ndata: {"content":"b"}\n\n' +
      'event: done\ndata: {"modelId":"m1"}\n\n' +
      'event: message\ndata: {"content":"partial'
    const events = parseSseText(text)
    expect(events.map((e) => e.event)).toEqual(['message', 'message', 'done'])
  })

  it('parses error events', () => {
    const events = parseSseText('event: error\ndata: {"code":"B000001","message":"boom"}\n\n')
    expect(events).toHaveLength(1)
    expect(events[0]?.event).toBe('error')
    expect(events[0]?.data).toContain('boom')
  })
})

describe('parseSseStream', () => {
  it('reassembles frames split across chunks', async () => {
    const encoder = new TextEncoder()
    const chunks = [
      encoder.encode('event: message\ndata: {"content":"hel'),
      encoder.encode('lo"}\n\nevent: done\ndata: {"modelId":"x"}\n\n'),
    ]
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(chunk)
        controller.close()
      },
    })

    const events = []
    for await (const event of parseSseStream(stream)) {
      events.push(event)
    }
    expect(events).toEqual([
      { event: 'message', data: '{"content":"hello"}' },
      { event: 'done', data: '{"modelId":"x"}' },
    ])
  })

  it('keeps incomplete frame until separator arrives', async () => {
    const encoder = new TextEncoder()
    let pullCount = 0
    const stream = new ReadableStream<Uint8Array>({
      pull(controller) {
        pullCount += 1
        if (pullCount === 1) {
          controller.enqueue(encoder.encode('event: message\ndata: {"content":"x"}'))
          return
        }
        if (pullCount === 2) {
          controller.enqueue(encoder.encode('\n\n'))
          controller.close()
        }
      },
    })

    const events = []
    for await (const event of parseSseStream(stream)) {
      events.push(event)
    }
    expect(events).toEqual([{ event: 'message', data: '{"content":"x"}' }])
  })
})
