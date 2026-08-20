import { request } from './http'

export type RuntimeRegistration = {
  device_access_id: string
  state: string
  reason?: string
  remote_address?: string
  expires_at?: string
  last_seen?: string
}

export type RuntimeSnapshot = {
  access_instance_id: string
  session_epoch: string
  snapshot_at: string
  latest_sequence: number
  registrations: RuntimeRegistration[]
}

export type AccessEvent = {
  event_id: string
  sequence: number
  access_instance_id: string
  session_epoch: string
  type: string
  occurred_at: string
  device_access_id: string
  payload: {
    state: string
    reason?: string
    remote_address?: string
    expires_at?: string
    last_seen?: string
  }
}

export type PollResult = {
  access_instance_id: string
  session_epoch: string
  latest_sequence: number
  events: AccessEvent[]
}

export function getSnapshot(): Promise<RuntimeSnapshot> {
  return request('/api/v1/access/snapshot')
}

export function pollEvents(after: number, limit = 50): Promise<PollResult> {
  return request(`/api/v1/access/events?after=${after}&limit=${limit}`)
}

export function ackEvents(through: number): Promise<{ status: string }> {
  return request('/api/v1/access/ack', { method: 'POST', body: JSON.stringify({ through_sequence: through }) })
}
