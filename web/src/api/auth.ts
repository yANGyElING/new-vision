import { request, setToken, getToken } from './http'

export type User = {
  id: string
  tenant_id: string
  username: string
  display_name: string
  status: string
  all_orgs: boolean
  roles: string[]
  org_ids: string[]
}

export type LoginResponse = {
  token: string
  expires_at: string
  user: User
  roles: string[]
}

export function login(tenant: string, username: string, password: string): Promise<LoginResponse> {
  return request('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ tenant, username, password }),
  })
}

export async function loginAndStore(tenant: string, username: string, password: string): Promise<User> {
  const result = await login(tenant, username, password)
  setToken(result.token)
  return result.user
}

export function me(): Promise<{ user: User; roles: string[]; org_scopes: string[]; all_orgs: boolean }> {
  return request('/api/v1/auth/me')
}

export function currentToken(): string | null {
  return getToken()
}