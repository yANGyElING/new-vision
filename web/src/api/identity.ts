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

export type OrgUnit = {
  id: string
  tenant_id: string
  parent_id?: string
  name: string
  created_at: string
  children?: OrgUnit[]
}

export type IdentityUser = {
  id: string
  tenant_id: string
  username: string
  display_name: string
  status: IdentityStatus
  all_orgs: boolean
  roles: string[]
  org_ids: string[]
  created_at: string
  updated_at: string
}

export type CreateUserInput = {
  tenant_id?: string
  username: string
  password: string
  display_name: string
  all_orgs?: boolean
  roles: string[]
  org_ids: string[]
}

export type UpdateUserInput = {
  display_name?: string
  status?: IdentityStatus
  all_orgs?: boolean
  roles?: string[]
  org_ids?: string[]
}

export const ROLE_META: Record<string, { label: string; hint: string }> = {
  node_admin: { label: '节点管理员', hint: '全部权限，含租户/用户/组织管理' },
  tenant_admin: { label: '租户管理员', hint: '设备全权、组织架构管理，Access 查看与确认' },
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

// --- org units ---

export function listOrgUnits(tenantID?: string): Promise<OrgUnit[]> {
  const suffix = tenantID ? `?tenant_id=${encodeURIComponent(tenantID)}` : ''
  return request(`/api/v1/org-units${suffix}`)
}

export function createOrgUnit(tenantID: string | undefined, parentID: string, name: string): Promise<OrgUnit> {
  const suffix = tenantID ? `?tenant_id=${encodeURIComponent(tenantID)}` : ''
  return request(`/api/v1/org-units${suffix}`, {
    method: 'POST',
    body: JSON.stringify({ parent_id: parentID, name }),
  })
}

export function renameOrgUnit(id: string, name: string): Promise<OrgUnit> {
  return request(`/api/v1/org-units/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) })
}

export function moveOrgUnit(id: string, parentID: string): Promise<OrgUnit> {
  return request(`/api/v1/org-units/${id}`, { method: 'PATCH', body: JSON.stringify({ parent_id: parentID }) })
}

export function deleteOrgUnit(id: string): Promise<void> {
  return request(`/api/v1/org-units/${id}`, { method: 'DELETE' })
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
