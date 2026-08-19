export type CheckState = 'up' | 'down'

export type HealthPayload = {
  service: 'node-app'
  status: 'ready' | 'not_ready'
  checks: {
    postgres: CheckState
    redis: CheckState
  }
}

export type HealthState =
  | { kind: 'loading' }
  | { kind: 'ready'; health: HealthPayload; checkedAt: Date }
  | { kind: 'degraded'; health: HealthPayload; checkedAt: Date }
  | { kind: 'unreachable'; message: string; checkedAt: Date }
  | { kind: 'invalid'; message: string; checkedAt: Date }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isHealthPayload(value: unknown): value is HealthPayload {
  if (!isRecord(value) || value.service !== 'node-app') return false
  if (value.status !== 'ready' && value.status !== 'not_ready') return false
  if (!isRecord(value.checks)) return false
  return (value.checks.postgres === 'up' || value.checks.postgres === 'down') &&
    (value.checks.redis === 'up' || value.checks.redis === 'down')
}

export async function fetchHealth(timeoutMs = 4000, signal?: AbortSignal): Promise<HealthState> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), timeoutMs)
  const requestSignal = signal ? AbortSignal.any([signal, controller.signal]) : controller.signal
  try {
    const response = await fetch('/api/health', {
      headers: { Accept: 'application/json' },
      signal: requestSignal,
    })
    let payload: unknown
    try {
      payload = await response.json()
    } catch {
      return { kind: 'invalid', message: '服务返回了无法解析的响应。', checkedAt: new Date() }
    }
    if (!isHealthPayload(payload)) {
      return { kind: 'invalid', message: '服务返回的数据格式不符合健康检查契约。', checkedAt: new Date() }
    }
    if (response.status === 200 && payload.status === 'ready') {
      return { kind: 'ready', health: payload, checkedAt: new Date() }
    }
    if (response.status === 503 && payload.status === 'not_ready') {
      return { kind: 'degraded', health: payload, checkedAt: new Date() }
    }
    return { kind: 'invalid', message: '服务状态码与健康检查结果不一致。', checkedAt: new Date() }
  } catch (error) {
    const message = error instanceof DOMException && error.name === 'AbortError'
      ? '健康检查请求超时。'
      : '无法连接到节点服务。'
    return { kind: 'unreachable', message, checkedAt: new Date() }
  } finally {
    window.clearTimeout(timeout)
  }
}
