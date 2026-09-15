import { request } from './http'
import type { CatalogState } from './devices'

/**
 * 通道（用户页面视角）——设计 D49：用户页面以通道为主语。
 * name 已是后端 COALESCE(display_name, report_name) 的结果（D42）。
 */
export type ChannelView = {
  id: string
  device_id: string
  channel_code: string
  name: string
  report_name: string | null
  display_name: string | null
  reported_status: string | null
  missing: boolean
  last_seen_in_catalog_at: string | null
  org_unit_id?: string | null
  org_unit_name?: string | null
}

/** 设备摘要：横幅三态与离线分组需要设备级事实（含还没有通道的设备）。 */
export type ChannelDeviceBrief = {
  id: string
  device_name: string
  state: 'online' | 'offline'
  catalog_state: CatalogState
  catalog_last_ok_at: string | null
}

export type ChannelListResult = {
  devices: ChannelDeviceBrief[]
  channels: ChannelView[]
}

/** 管理页：单设备的全列通道 + 目录同步状态。 */
export type StoredChannel = {
  id: string
  device_id: string
  channel_code: string
  report_name: string | null
  display_name: string | null
  org_unit_id?: string | null
  reported_status: string | null
  missing: boolean
  last_seen_in_catalog_at: string | null
  created_at: string
  updated_at: string
}

export type DeviceCatalogResult = {
  device_id: string
  catalog_state: CatalogState
  catalog_last_ok_at: string | null
  catalog_last_count: number | null
  catalog_last_error?: string | null
  channels: StoredChannel[]
}

/** 用户页：可见范围内的全部通道（可按组织子树过滤）。 */
export function listChannels(orgUnitID?: string): Promise<ChannelListResult> {
  const query = orgUnitID ? `?org_unit_id=${encodeURIComponent(orgUnitID)}` : ''
  return request(`/api/v1/channels${query}`)
}

/** 管理页：某台设备的通道全列 + 目录同步状态。 */
export function listDeviceChannels(deviceID: string): Promise<DeviceCatalogResult> {
  return request(`/api/v1/devices/${deviceID}/channels`)
}
