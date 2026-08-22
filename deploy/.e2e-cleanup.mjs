// one-off cleanup of leftover e2e data (idempotent)
const BASE = 'http://106.52.72.48:8080'
const ADMIN_PASSWORD = process.env.NV_E2E_ADMIN_PASSWORD
if (!ADMIN_PASSWORD) { console.error('set NV_E2E_ADMIN_PASSWORD'); process.exit(1) }

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
  const text = await res.text()
  let json = null
  try { json = text ? JSON.parse(text) : null } catch {}
  return { status: res.status, json }
}

const login = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: 'admin', password: ADMIN_PASSWORD } })
if (login.status !== 200) { console.error('login failed', login.status); process.exit(1) }
const token = login.json.token

const tenants = await api('GET', '/api/v1/tenants', { token })
const e2eTenants = (tenants.json ?? []).filter((t) => t.name.startsWith('e2e-'))
console.log(`e2e tenants found: ${e2eTenants.map((t) => t.name).join(', ') || 'none'}`)

for (const t of e2eTenants) {
  const users = await api('GET', `/api/v1/users?tenant_id=${t.id}`, { token })
  for (const u of users.json ?? []) {
    if (u.username.startsWith('e2e-')) {
      const del = await api('DELETE', `/api/v1/users/${u.id}?tenant_id=${t.id}`, { token })
      console.log(`  delete user ${u.username}@${t.name}: ${del.status}`)
    }
  }
}

// also drop any e2e leftovers in the default tenant
const defUsers = await api('GET', '/api/v1/users', { token })
for (const u of defUsers.json ?? []) {
  if (u.username.startsWith('e2e-')) {
    const del = await api('DELETE', `/api/v1/users/${u.id}`, { token })
    console.log(`  delete user ${u.username}@default: ${del.status}`)
  }
}

// e2e devices first (their region_id blocks region deletion)
const devices = await api('GET', '/api/v1/devices', { token })
for (const d of devices.json ?? []) {
  if ((d.device_name ?? '').startsWith('e2e-')) {
    const del = await api('DELETE', `/api/v1/devices/${d.id}`, { token })
    console.log(`  delete device ${d.device_access_id}: ${del.status}`)
  }
}

// e2e regions leftovers
const regions = await api('GET', '/api/v1/regions', { token })
const flat = []
const walk = (nodes) => { for (const n of nodes) { flat.push(n); if (n.children) walk(n.children) } }
walk(regions.json ?? [])
for (const r of flat.filter((n) => n.name.startsWith('e2e-')).reverse()) {
  const del = await api('DELETE', `/api/v1/regions/${r.id}`, { token })
  console.log(`  delete region ${r.name}: ${del.status}`)
}
console.log('cleanup done (tenants themselves need SQL - no tenant delete API)')
