// e2e verification of the deployed auth system.
// usage: NV_E2E_BASE=http://host:8080 NV_E2E_ADMIN_PASSWORD=... node deploy/.e2e-test.mjs
const BASE = process.env.NV_E2E_BASE ?? 'http://106.52.72.48:8080'
const ADMIN_PASSWORD = process.env.NV_E2E_ADMIN_PASSWORD
if (!ADMIN_PASSWORD) {
  console.error('set NV_E2E_ADMIN_PASSWORD (the seeded admin password from the server env file)')
  process.exit(1)
}
const ADMIN = { tenant: 'default', username: 'admin', password: ADMIN_PASSWORD }
// unique per-run suffix so the suite is re-runnable against live data
const S = Date.now().toString(36)
const NAME = { region: `e2e-region-${S}`, viewer: `e2e-viewer-${S}`, tenant: `e2e-tenant-${S}`, cross: `e2e-op-${S}`, device: `e2e-device-${S}` }
const ROOT_REGION = '00000000-0000-0000-0000-000000000002'

const results = []
let step = 0
function report(name, pass, detail = '') {
  results.push({ name, pass, detail })
  console.log(`${pass ? 'PASS' : 'FAIL'}  [${String(++step).padStart(2, '0')}] ${name}${detail ? `  -- ${detail}` : ''}`)
}

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
  let json = null
  const text = await res.text()
  try { json = text ? JSON.parse(text) : null } catch { /* non-JSON */ }
  return { status: res.status, json, text }
}

// ---------- A. authentication ----------
const loginOK = await api('POST', '/api/v1/auth/login', { body: ADMIN })
report('admin login -> 200 + token', loginOK.status === 200 && !!loginOK.json?.token, `status=${loginOK.status}`)
const adminToken = loginOK.json?.token

const badLogin = await api('POST', '/api/v1/auth/login', { body: { ...ADMIN, password: 'wrong' } })
report('wrong password -> 401 uniform message', badLogin.status === 401 && badLogin.json?.error?.message === 'invalid tenant or credentials', `status=${badLogin.status} msg=${badLogin.json?.error?.message}`)

const meRes = await api('GET', '/api/v1/auth/me', { token: adminToken })
report('GET /auth/me -> admin + node_admin', meRes.status === 200 && meRes.json?.user?.username === 'admin' && meRes.json?.roles?.includes('node_admin'), `roles=${JSON.stringify(meRes.json?.roles)}`)

const noAuth = await api('GET', '/api/v1/users')
report('no token -> 401', noAuth.status === 401, `status=${noAuth.status}`)

// ---------- B. identity management (node_admin) ----------
const roles = await api('GET', '/api/v1/roles', { token: adminToken })
report('GET /roles -> 4 fixed roles', roles.status === 200 && JSON.stringify(roles.json?.roles) === JSON.stringify(['node_admin', 'tenant_admin', 'operator', 'viewer']), JSON.stringify(roles.json?.roles))

const regionsBefore = await api('GET', '/api/v1/regions', { token: adminToken })
report('GET /regions -> tree with root', regionsBefore.status === 200 && Array.isArray(regionsBefore.json) && regionsBefore.json.some((r) => r.id === ROOT_REGION))

const mkRegion = await api('POST', '/api/v1/regions', { token: adminToken, body: { parent_id: ROOT_REGION, name: NAME.region } })
report('create region -> 201', mkRegion.status === 201 && !!mkRegion.json?.id, `status=${mkRegion.status}`)
const regionID = mkRegion.json?.id

const mkViewer = await api('POST', '/api/v1/users', {
  token: adminToken,
  body: { tenant_id: '', username: NAME.viewer, password: 'ViewerPass123', display_name: 'E2E Viewer', roles: ['viewer'], region_ids: regionID ? [regionID] : [] },
})
report('create user (viewer) -> 201 + roles', mkViewer.status === 201 && JSON.stringify(mkViewer.json?.roles) === '["viewer"]', `status=${mkViewer.status}`)
const viewerID = mkViewer.json?.id

const listUsers = await api('GET', '/api/v1/users', { token: adminToken })
report('GET /users contains e2e-viewer', listUsers.status === 200 && listUsers.json?.some((u) => u.username === NAME.viewer))

// ---------- C. casbin enforcement (viewer) ----------
const viewerLogin = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: NAME.viewer, password: 'ViewerPass123' } })
report('viewer login -> 200', viewerLogin.status === 200 && !!viewerLogin.json?.token, `status=${viewerLogin.status}`)
const viewerToken = viewerLogin.json?.token

const vListUsers = await api('GET', '/api/v1/users', { token: viewerToken })
report('viewer GET /users -> 403 (identity:manage denied)', vListUsers.status === 403, `status=${vListUsers.status}`)

const vListDevices = await api('GET', '/api/v1/devices', { token: viewerToken })
report('viewer GET /devices -> 200 (device:view allowed)', vListDevices.status === 200, `status=${vListDevices.status}`)

const vMkDevice = await api('POST', '/api/v1/devices', {
  token: viewerToken,
  body: { region_id: regionID, center_code: '34029999', device_type: '132', device_name: 'x', manufacturer: 'x', sip_realm: '3402000000', password: 'x', enabled: true },
})
report('viewer POST /devices -> 403 (device:create denied)', vMkDevice.status === 403, `status=${vMkDevice.status}`)

// ---------- D. role change takes effect immediately ----------
const patchRole = await api('PATCH', `/api/v1/users/${viewerID}`, { token: adminToken, body: { roles: ['operator'] } })
report('PATCH viewer -> operator', patchRole.status === 200 && JSON.stringify(patchRole.json?.roles) === '["operator"]', `status=${patchRole.status}`)

const mkDevice = await api('POST', '/api/v1/devices', {
  token: adminToken,
  body: { region_id: regionID, center_code: '34029998', device_type: '132', device_name: NAME.device, manufacturer: 'e2e', sip_realm: '3402000000', password: 'devpass', enabled: false },
})
report('admin create device -> 201', mkDevice.status === 201 && !!mkDevice.json?.id, `status=${mkDevice.status} id=${mkDevice.json?.device_access_id}`)
const deviceID = mkDevice.json?.id

// same (old) viewer token, now operator: still no create, but enable allowed
const opMkDevice = await api('POST', '/api/v1/devices', {
  token: viewerToken,
  body: { region_id: regionID, center_code: '34029997', device_type: '132', device_name: 'x', manufacturer: 'x', sip_realm: '3402000000', password: 'x', enabled: true },
})
report('operator(old token) POST /devices -> 403 (still no create)', opMkDevice.status === 403, `status=${opMkDevice.status}`)

const opEnable = await api('PATCH', `/api/v1/devices/${deviceID}`, { token: viewerToken, body: { enabled: true } })
report('operator(old token) PATCH enable -> 200 (roles live-reloaded)', opEnable.status === 200 && opEnable.json?.enabled === true, `status=${opEnable.status}`)

// ---------- E. password reset + disable ----------
const resetPw = await api('POST', `/api/v1/users/${viewerID}/password`, { token: adminToken, body: { password: 'NewPass456' } })
report('reset password -> 204', resetPw.status === 204, `status=${resetPw.status}`)

const reLogin = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: NAME.viewer, password: 'NewPass456' } })
report('login with new password -> 200', reLogin.status === 200, `status=${reLogin.status}`)

const disable = await api('PATCH', `/api/v1/users/${viewerID}`, { token: adminToken, body: { status: 'disabled' } })
report('PATCH status=disabled -> 200', disable.status === 200 && disable.json?.status === 'disabled', `status=${disable.status}`)

const disabledLogin = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: NAME.viewer, password: 'NewPass456' } })
report('disabled user login -> 401', disabledLogin.status === 401, `status=${disabledLogin.status}`)

// ---------- F. multi-tenant ----------
const mkTenant = await api('POST', '/api/v1/tenants', { token: adminToken, body: { name: NAME.tenant } })
report('create tenant -> 201', mkTenant.status === 201 && !!mkTenant.json?.id, `status=${mkTenant.status}`)
const tenantID = mkTenant.json?.id

const mkCross = await api('POST', '/api/v1/users', {
  token: adminToken,
  body: { tenant_id: tenantID, username: NAME.cross, password: 'OpPass789', display_name: 'E2E Cross Tenant', roles: ['operator'], region_ids: [] },
})
report('node_admin creates user in other tenant -> 201', mkCross.status === 201 && mkCross.json?.tenant_id === tenantID, `status=${mkCross.status}`)
const crossUserID = mkCross.json?.id

const crossLogin = await api('POST', '/api/v1/auth/login', { body: { tenant: NAME.tenant, username: NAME.cross, password: 'OpPass789' } })
report('cross-tenant user login -> 200', crossLogin.status === 200, `status=${crossLogin.status}`)

const crossForbidden = await api('GET', '/api/v1/users', { token: crossLogin.json?.token })
report('cross-tenant operator GET /users -> 403', crossForbidden.status === 403, `status=${crossForbidden.status}`)

// tenant isolation on devices: e2e-op is in e2e-tenant, admin device belongs to default
const crossDevices = await api('GET', '/api/v1/devices', { token: crossLogin.json?.token })
const seesAdminDevice = (crossDevices.json ?? []).some((d) => d.id === deviceID)
report('cross-tenant devices isolated (no default-tenant device)', crossDevices.status === 200 && !seesAdminDevice, `count=${crossDevices.json?.length ?? 'n/a'}`)

// ---------- G. cleanup via API ----------
await api('PATCH', `/api/v1/users/${viewerID}`, { token: adminToken, body: { status: 'active' } }) // re-enable so delete path is plain
const delDevice = await api('DELETE', `/api/v1/devices/${deviceID}`, { token: adminToken })
report('cleanup: delete device -> 204', delDevice.status === 204)

const delViewer = await api('DELETE', `/api/v1/users/${viewerID}`, { token: adminToken })
report('cleanup: delete e2e-viewer -> 204', delViewer.status === 204)

const delCross = await api('DELETE', `/api/v1/users/${crossUserID}?tenant_id=${tenantID}`, { token: adminToken, body: undefined })
report('cleanup: delete cross-tenant user -> 204', delCross.status === 204)

const delRegion = await api('DELETE', `/api/v1/regions/${regionID}`, { token: adminToken })
report('cleanup: delete region -> 204', delRegion.status === 204)

// ---------- summary ----------
const failed = results.filter((r) => !r.pass)
console.log(`\n===== ${results.length - failed.length}/${results.length} passed =====`)
if (failed.length > 0) {
  console.log('FAILED:')
  for (const f of failed) console.log(`  - ${f.name} (${f.detail})`)
  process.exit(1)
}
