/**
 * POST SSE 帧解析：跨 chunk 拆包、多事件、多行 data、残缺 frame 缓存。
 * 不使用 EventSource（仅支持 GET）。
 */

/** 解析后的单个 SSE 事件。 */
export interface ParsedSseEvent {
  event: string
  data: string
}

/**
 * 从 ReadableStream 异步产出完整 SSE 帧。
 * 残缺帧保留在缓冲中，直到出现 `\n\n` 分隔。
 */
export async function* parseSseStream(
  stream: ReadableStream<Uint8Array>,
  options?: { signal?: AbortSignal },
): AsyncGenerator<ParsedSseEvent, void, unknown> {
  const reader = stream.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      if (options?.signal?.aborted) {
        const abortError = new DOMException('The operation was aborted.', 'AbortError')
        throw abortError
      }

      const { done, value } = await reader.read()
      if (done) {
        break
      }

      buffer += decoder.decode(value, { stream: true })
      buffer = buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n')

      let separator = buffer.indexOf('\n\n')
      while (separator !== -1) {
        const rawFrame = buffer.slice(0, separator)
        buffer = buffer.slice(separator + 2)
        const parsed = parseSseFrame(rawFrame)
        if (parsed) {
          yield parsed
        }
        separator = buffer.indexOf('\n\n')
      }
    }
  } finally {
    reader.releaseLock()
  }
}

/**
 * 解析单个 SSE frame 文本（不含结尾空行）。
 * 多行 data 按 SSE 规范用 `\n` 拼接。
 */
export function parseSseFrame(rawFrame: string): ParsedSseEvent | null {
  const lines = rawFrame.split('\n')
  let eventName = 'message'
  const dataLines: string[] = []
  let hasField = false

  for (const line of lines) {
    if (line === '' || line.startsWith(':')) {
      continue
    }

    const colon = line.indexOf(':')
    const field = colon === -1 ? line : line.slice(0, colon)
    let value = colon === -1 ? '' : line.slice(colon + 1)
    if (value.startsWith(' ')) {
      value = value.slice(1)
    }

    hasField = true
    if (field === 'event') {
      eventName = value
    } else if (field === 'data') {
      dataLines.push(value)
    }
  }

  if (!hasField || (dataLines.length === 0 && eventName === 'message' && !rawFrame.includes('event:'))) {
    // 空注释帧或无字段帧丢弃
    if (!hasField) {
      return null
    }
  }

  if (dataLines.length === 0 && eventName === 'message') {
    return null
  }

  return {
    event: eventName,
    data: dataLines.join('\n'),
  }
}

/**
 * 同步解析完整 SSE 文本（便于单测拆包场景）。
 */
export function parseSseText(text: string): ParsedSseEvent[] {
  const normalized = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const frames = normalized.split('\n\n')
  const events: ParsedSseEvent[] = []
  for (const frame of frames) {
    if (!frame.trim()) {
      continue
    }
    // 最后一个残缺 frame（文本未以 \n\n 结束）不在完整 split 末尾处理为事件，
    // 调用方应只传入完整帧序列；带 trailing 残缺时最后一段无 \n\n 不会单独出现除非文本不以 \n\n 结束。
    const parsed = parseSseFrame(frame)
    if (parsed) {
      events.push(parsed)
    }
  }
  // 若原文不以 \n\n 结尾，最后一段是残缺 frame，应剔除
  if (!normalized.endsWith('\n\n') && frames.length > 0) {
    const last = frames[frames.length - 1] ?? ''
    if (last.trim()) {
      const maybeIncomplete = parseSseFrame(last)
      const top = events[events.length - 1]
      if (maybeIncomplete && top && top.event === maybeIncomplete.event && top.data === maybeIncomplete.data) {
        events.pop()
      }
    }
  }
  return events
}
