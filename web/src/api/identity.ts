import { request } from './http'

export type IdentityRole = 'node_admin' | 'tenant_admin' | 'operator' | 'viewer'
export type IdentityStatus = 'active' | 'disabled'

export type Tenant = {
  id: string
  name: string
  status: IdentityStatus
  created_at: string
  updated_at: string
}

export type Region = {
  id: string
  parent_id?: string
  name: string
  created_at: string
  children?: Region[]
}

export type IdentityUser = {
  id: string
  tenant_id: string
  username: string
  display_name: string
  status: IdentityStatus
  roles: string[]
  region_ids: string[]
  created_at: string
  updated_at: string
}

export type CreateUserInput = {
  tenant_id?: string
  username: string
  password: string
  display_name: string
  roles: string[]
  region_ids: string[]
}

export type UpdateUserInput = {
  display_name?: string
  status?: IdentityStatus
  roles?: string[]
  region_ids?: string[]
}

export const ROLE_META: Record<string, { label: string; hint: string }> = {
  node_admin: { label: '节点管理员', hint: '全部权限，含租户/用户/区域管理' },
  tenant_admin: { label: '租户管理员', hint: '设备全权，Access 查看与确认' },
  operator: { label: '操作员', hint: '设备查看与启用，Access 查看' },
  viewer: { label: '观察者', hint: '设备与 Access 只读' },
}

// --- tenants ---

export function listTenants(): Promise<Tenant[]> {
  return request('/api/v1/tenants')
}

export function createTenant(name: string): Promise<Tenant> {
  return request('/api/v1/tenants', { method: 'POST', body: JSON.stringify({ name }) })
}

export function setTenantStatus(id: string, status: IdentityStatus): Promise<Tenant> {
  return request(`/api/v1/tenants/${id}`, { method: 'PATCH', body: JSON.stringify({ status }) })
}

// --- regions ---

export function listRegions(): Promise<Region[]> {
  return request('/api/v1/regions')
}

export function createRegion(parentID: string, name: string): Promise<Region> {
  return request('/api/v1/regions', {
    method: 'POST',
    body: JSON.stringify({ parent_id: parentID, name }),
  })
}

export function renameRegion(id: string, name: string): Promise<Region> {
  return request(`/api/v1/regions/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) })
}

export function deleteRegion(id: string): Promise<void> {
  return request(`/api/v1/regions/${id}`, { method: 'DELETE' })
}

// --- users ---

export function listUsers(tenantID?: string): Promise<IdentityUser[]> {
  const suffix = tenantID ? `?tenant_id=${encodeURIComponent(tenantID)}` : ''
  return request(`/api/v1/users${suffix}`)
}

export function createUser(input: CreateUserInput): Promise<IdentityUser> {
  return request('/api/v1/users', { method: 'POST', body: JSON.stringify(input) })
}

export function updateUser(id: string, input: UpdateUserInput): Promise<IdentityUser> {
  return request(`/api/v1/users/${id}`, { method: 'PATCH', body: JSON.stringify(input) })
}

export function deleteUser(id: string): Promise<void> {
  return request(`/api/v1/users/${id}`, { method: 'DELETE' })
}

export function setUserPassword(id: string, password: string): Promise<void> {
  return request(`/api/v1/users/${id}/password`, {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

// --- roles ---

export function listRoles(): Promise<{ roles: string[] }> {
  return request('/api/v1/roles')
}
