export type APIErrorBody = {
  error: { code: string; message: string }
}

const TOKEN_KEY = 'nv_token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string | null): void {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
}

export function isAuthenticated(): boolean {
  return getToken() !== null
}

export async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(options.body ? { 'Content-Type': 'application/json' } : {}),
  }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`

  const response = await fetch(url, {
    ...options,
    headers: { ...headers, ...(options.headers as Record<string, string> | undefined) },
  })
  if (response.status === 401 && !url.endsWith('/api/v1/auth/login')) {
    setToken(null)
    const current = window.location.pathname + window.location.search
    window.location.assign(`/login?redirect=${encodeURIComponent(current)}`)
    throw new Error('登录已过期，请重新登录')
  }
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`
    try {
      const body: APIErrorBody = await response.json()
      if (body.error?.message) message = body.error.message
    } catch {
      // keep the default status text
    }
    throw new Error(message)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
