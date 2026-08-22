// one-off: rotate the seeded admin password (the old one hit git history)
const BASE = process.env.NV_E2E_BASE ?? 'http://106.52.72.48:8080'
const OLD = process.env.NV_E2E_ADMIN_PASSWORD
const NEW = process.env.NV_E2E_NEW_PASSWORD
if (!OLD || !NEW) { console.error('need NV_E2E_ADMIN_PASSWORD and NV_E2E_NEW_PASSWORD'); process.exit(1) }

async function api(method, path, { token, body } = {}) {
  const res = await fetch(BASE + path, {
    method,
    headers: {
      Accept: 'application/json',
      ...(body ? { 'Content-Type': 'application/json' } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  return { status: res.status, json: await res.json().catch(() => null) }
}

const login = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: 'admin', password: OLD } })
if (login.status !== 200) { console.error('old login failed:', login.status); process.exit(1) }
const token = login.json.token

const users = await api('GET', '/api/v1/users', { token })
const admin = (users.json ?? []).find((u) => u.username === 'admin')
if (!admin) { console.error('admin user not found'); process.exit(1) }

const reset = await api('POST', `/api/v1/users/${admin.id}/password`, { token, body: { password: NEW } })
console.log('reset:', reset.status)
if (reset.status !== 204) process.exit(1)

const verify = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: 'admin', password: NEW } })
console.log('login with new password:', verify.status === 200 ? 'OK' : `FAIL ${verify.status}`)
const oldVerify = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: 'admin', password: OLD } })
console.log('old password rejected:', oldVerify.status === 401 ? 'OK' : `FAIL ${oldVerify.status}`)
process.exit(verify.status === 200 && oldVerify.status === 401 ? 0 : 1)
