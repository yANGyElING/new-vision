import { request } from './http'

export type RuntimeState = {
  state: string
  reason?: string
  remote_address?: string
  expires_at?: string
  last_seen?: string
  session_epoch?: string
  stale?: boolean
}

export type Device = {
  id: string
  device_access_id: string
  sip_username: string
  sip_realm: string
  digest_algorithm: string
  enabled: boolean
  profile_version: number
  access_sync_status: 'pending' | 'synced'
  access_synced_version: number | null
  created_at: string
  updated_at: string
  runtime?: RuntimeState | null
}

export type APIError = {
  error: { code: string; message: string }
}

export function listDevices(): Promise<Device[]> {
  return request('/api/v1/devices')
}

export type CreateDeviceInput = {
  device_access_id: string
  sip_username: string
  sip_realm: string
  password: string
  enabled: boolean
}

export function createDevice(input: CreateDeviceInput): Promise<Device> {
  return request('/api/v1/devices', { method: 'POST', body: JSON.stringify(input) })
}

export function setDeviceEnabled(id: string, enabled: boolean): Promise<Device> {
  return request(`/api/v1/devices/${id}`, { method: 'PATCH', body: JSON.stringify({ enabled }) })
}

export function deleteDevice(id: string): Promise<void> {
  return request(`/api/v1/devices/${id}`, { method: 'DELETE' })
}

/** Generate a plausible 20-digit GB28181 device access ID. */
export function generateAccessID(): string {
  const center = '3402000000'
  const suffix = Array.from({ length: 10 }, () => Math.floor(Math.random() * 10)).join('')
  return center + suffix
}
