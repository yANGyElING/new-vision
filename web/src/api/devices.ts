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
  device_name: string
  manufacturer: string
  device_type: string
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

// GB/T 28181 device type codes (positions 11-13 of the 20-digit access id).
export const DEVICE_TYPES = [
  { code: '132', label: 'IPC 摄像机' },
  { code: '118', label: 'NVR' },
  { code: '111', label: 'DVR' },
  { code: '200', label: '中心服务器' },
] as const

export type DeviceTypeCode = (typeof DEVICE_TYPES)[number]['code']

export function deviceTypeLabel(code: string): string {
  return DEVICE_TYPES.find((t) => t.code === code)?.label ?? code
}

export type CreateDeviceInput = {
  region_id: string
  center_code: string
  device_type: string
  device_name: string
  manufacturer: string
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

export function updateDeviceMeta(id: string, data: { device_name?: string; manufacturer?: string }): Promise<Device> {
  return request(`/api/v1/devices/${id}`, { method: 'PATCH', body: JSON.stringify(data) })
}

export function deleteDevice(id: string): Promise<void> {
  return request(`/api/v1/devices/${id}`, { method: 'DELETE' })
}

/**
 * Build the 14-digit prefix (center + industry 00 + type + network 0) and
 * preview the next 20-digit GB/T 28181 code the backend will allocate.
 * The sequence shown is a preview only; the backend owns allocation.
 */
export function previewAccessID(centerCode: string, deviceType: string, nextSequence = 1): string {
  const seq = String(nextSequence).padStart(6, '0')
  return centerCode + '00' + deviceType + '0' + seq
}
