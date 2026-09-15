// e2e verification of the Catalog channel sync chain (Phase 1, tasks 1-4):
// SIP Catalog frames -> gb28181 C module aggregation -> Redis events ->
// node-app consumer -> channels/devices rows -> dual-view APIs.
// usage: NV_E2E_BASE=http://host:8080 NV_E2E_SIP_HOST=host NV_E2E_SIP_PORT=5060 \
//        NV_E2E_ADMIN_PASSWORD=... node deploy/.e2e-catalog-test.mjs
import dgram from 'node:dgram'
import crypto from 'node:crypto'

const BASE = process.env.NV_E2E_BASE ?? 'http://localhost:8080'
const SIP_HOST = process.env.NV_E2E_SIP_HOST ?? new URL(BASE).hostname
const SIP_PORT = Number(process.env.NV_E2E_SIP_PORT ?? 5060)
const ADMIN_USERNAME = process.env.NV_E2E_ADMIN_USERNAME ?? 'vision'
const ADMIN_PASSWORD = process.env.NV_E2E_ADMIN_PASSWORD
if (!ADMIN_PASSWORD) {
  console.error('set NV_E2E_ADMIN_PASSWORD (the seeded admin password from the server env file)')
  process.exit(1)
}
const ADMIN = { tenant: 'default', username: ADMIN_USERNAME, password: ADMIN_PASSWORD }
// unique per-run suffix so the suite is re-runnable against live data
const S = Date.now().toString(36)
const NAME = { org: `e2e-cat-org-${S}`, otherOrg: `e2e-cat-other-${S}`, viewer: `e2e-cat-viewer-${S}`, device: `e2e-cat-dev-${S}` }

const results = []
let step = 0
function report(name, pass, detail = '') {
  results.push({ name, pass, detail })
  console.log(`${pass ? 'PASS' : 'FAIL'}  [${String(++step).padStart(2, '0')}] ${name}${detail ? '  -- ' + detail : ''}`)
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

// ---------- SIP MESSAGE sender (no digest auth required on MESSAGE) ----------
function sipMessage(accessID, realm, xmlBody) {
  const callID = crypto.randomBytes(8).toString('hex') + '@cat'
  const branch = 'z9hG4bK' + crypto.randomBytes(12).toString('hex')
  const tag = crypto.randomBytes(8).toString('hex')
  const requestURI = `sip:${accessID}@${realm}`
  const lines = [
    `MESSAGE ${requestURI} SIP/2.0`,
    `Via: SIP/2.0/UDP 127.0.0.1:9;branch=${branch};rport`,
    `From: <sip:${accessID}@${realm}>;tag=${tag}`,
    `To: <sip:${accessID}@${realm}>`,
    `Call-ID: ${callID}`,
    'CSeq: 1 MESSAGE',
    'Max-Forwards: 70',
    'Content-Type: application/MANSCDP+xml',
    `Content-Length: ${Buffer.byteLength(xmlBody)}`,
    '',
    xmlBody,
  ]
  return lines.join('\r\n')
}

function catalogFrame(accessID, sn, sumNum, items) {
  const list = items
    .map((item) => `<Item><DeviceID>${item.code}</DeviceID><Name>${item.name}</Name>${item.status ? `<Status>${item.status}</Status>` : ''}</Item>`)
    .join('')
  return `<?xml version="1.0" encoding="UTF-8"?>\r\n<Response><CmdType>Catalog</CmdType><SN>${sn}</SN><DeviceID>${accessID}</DeviceID><SumNum>${sumNum}</SumNum><DeviceList Num="${items.length}">${list}</DeviceList></Response>`
}

function sendSIP(packet) {
  return new Promise((resolve, reject) => {
    const socket = dgram.createSocket('udp4')
    const done = (err, status) => { try { socket.close() } catch { /* already closed */ } err ? reject(err) : resolve(status) }
    socket.on('message', (msg) => done(null, msg.toString().split('\r\n')[0]))
    socket.on('error', done)
    socket.send(packet, SIP_PORT, SIP_HOST, (err) => { if (err) done(err) })
    setTimeout(() => done(new Error('sip response timeout')), 3000)
  })
}

async function sendCatalog(accessID, realm, sn, sumNum, items) {
  return sendSIP(sipMessage(accessID, realm, catalogFrame(accessID, sn, sumNum, items)))
}

// ---------- fixtures ----------
const CH1 = { code: '34020000001320000101', name: '大门口', status: 'ON' }
const CH2 = { code: '34020000001320000102', name: '泵房主视角', status: 'ON' }

async function waitFor(predicate, { tries = 20, intervalMs = 500 } = {}) {
  for (let i = 0; i < tries; i++) {
    const value = await predicate()
    if (value) return value
    await new Promise((r) => setTimeout(r, intervalMs))
  }
  return null
}

// ---------- A. setup ----------
const login = await api('POST', '/api/v1/auth/login', { body: ADMIN })
report('admin login -> 200 + token', login.status === 200 && !!login.json?.token, `status=${login.status}`)
const adminToken = login.json?.token

const org = await api('POST', '/api/v1/org-units', { token: adminToken, body: { parent_id: '', name: NAME.org } })
report('create org-unit -> 201', org.status === 201 && !!org.json?.id, `status=${org.status}`)
const otherOrg = await api('POST', '/api/v1/org-units', { token: adminToken, body: { parent_id: '', name: NAME.otherOrg } })
report('create second org-unit -> 201', otherOrg.status === 201 && !!otherOrg.json?.id, `status=${otherOrg.status}`)

const device = await api('POST', '/api/v1/devices', {
  token: adminToken,
  body: { org_unit_id: org.json?.id, center_code: '34020000', device_type: '118', device_name: NAME.device, manufacturer: 'e2e', sip_realm: '3402000000', password: 'catalog-e2e-pass', enabled: true },
})
report('create device -> 201 + access id', device.status === 201 && /^\d{20}$/.test(device.json?.device_access_id ?? ''), `status=${device.status}`)
const dev = device.json
const deviceID = dev?.id
const accessID = dev?.device_access_id

const viewer = await api('POST', '/api/v1/users', {
  token: adminToken,
  body: { tenant_id: '', username: NAME.viewer, password: 'viewer-pass-123', display_name: '小李', roles: ['viewer'], org_ids: [org.json?.id] },
})
report('create scoped viewer -> 201', viewer.status === 201 && !!viewer.json?.id, `status=${viewer.status} body=${viewer.text?.slice(0, 120)}`)

// ---------- B. multi-frame catalog -> progress + result ----------
// SN=1 分两帧：第一帧 1 路，第二帧 1 路；SumNum=2。设备未注册也允许上报（MESSAGE 只查 profile）。
const sipRealm = dev?.sip_realm ?? '3402000000'
const r1 = await sendCatalog(accessID, sipRealm, 1, 2, [CH1])
report('catalog frame 1 -> 200', /200/.test(r1 ?? ''), `resp=${r1}`)
const r2 = await sendCatalog(accessID, sipRealm, 1, 2, [CH2])
report('catalog frame 2 (batch complete) -> 200', /200/.test(r2 ?? ''), `resp=${r2}`)

const firstSync = await waitFor(async () => {
  const res = await api('GET', `/api/v1/devices/${deviceID}/channels`, { token: adminToken })
  if (res.status !== 200) return null
  const result = res.json
  return result?.catalog_state === 'ok' && result?.channels?.length === 2 ? result : null
})
report('device channels after sync: state=ok, 2 channels', !!firstSync,
  `state=${firstSync?.catalog_state} channels=${firstSync?.channels?.length}`)
const byCode = Object.fromEntries((firstSync?.channels ?? []).map((c) => [c.channel_code, c]))
report('channel identity + report_name landed', byCode[CH1.code]?.report_name === '大门口' && byCode[CH2.code]?.report_name === '泵房主视角',
  `ch1=${JSON.stringify(byCode[CH1.code])}`)

// progress 事件只标记 in_progress，最终收敛到 ok（事件顺序在流内保序）
const deviceAfter = await api('GET', `/api/v1/devices/${deviceID}`, { token: adminToken })
report('device catalog_last_count = 2', deviceAfter.json?.catalog_last_count === 2, `count=${deviceAfter.json?.catalog_last_count}`)

// ---------- C. 缺失（少报一路）与复活（重新补报） ----------
const r3 = await sendCatalog(accessID, sipRealm, 2, 1, [CH1])
report('re-sync with 1 channel -> 200', /200/.test(r3 ?? ''), `resp=${r3}`)
const missing = await waitFor(async () => {
  const res = await api('GET', `/api/v1/devices/${deviceID}/channels`, { token: adminToken })
  const ch2 = res.json?.channels?.find((c) => c.channel_code === CH2.code)
  return ch2?.missing === true ? res.json : null
})
report('absent channel marked missing (D45)', !!missing, `ch2=${JSON.stringify(missing?.channels?.find((c) => c.channel_code === CH2.code))}`)

const r4 = await sendCatalog(accessID, sipRealm, 3, 2, [CH1, CH2])
report('re-sync with both channels -> 200', /200/.test(r4 ?? ''), `resp=${r4}`)
const revived = await waitFor(async () => {
  const res = await api('GET', `/api/v1/devices/${deviceID}/channels`, { token: adminToken })
  const ch2 = res.json?.channels?.find((c) => c.channel_code === CH2.code)
  return ch2?.missing === false ? res.json : null
})
report('channel revived (missing=false, history kept)', !!revived, `ch2.missing=${revived?.channels?.find((c) => c.channel_code === CH2.code)?.missing}`)

// ---------- D. 字段保留（D43）：Name 缺省时旧值不清 ----------
const r5 = await sendCatalog(accessID, sipRealm, 4, 1, [{ code: CH1.code, name: '', status: 'OFF' }])
report('re-sync with empty name -> 200', /200/.test(r5 ?? ''), `resp=${r5}`)
const kept = await waitFor(async () => {
  const res = await api('GET', `/api/v1/devices/${deviceID}/channels`, { token: adminToken })
  const ch1 = res.json?.channels?.find((c) => c.channel_code === CH1.code)
  return ch1?.report_name === '大门口' && ch1?.reported_status === 'OFF' ? ch1 : null
})
report('D43: absent name keeps old value, status refreshes', !!kept, `ch1=${JSON.stringify(kept)}`)

// ---------- E. 双视角权限（D28/D36） ----------
const viewerLogin = await api('POST', '/api/v1/auth/login', { body: { tenant: 'default', username: NAME.viewer, password: 'viewer-pass-123' } })
report('viewer login -> 200', viewerLogin.status === 200 && !!viewerLogin.json?.token, `status=${viewerLogin.status}`)
const viewerToken = viewerLogin.json?.token

const viewerChannels = await api('GET', '/api/v1/channels', { token: viewerToken })
const viewerVisible = viewerChannels.json?.channels ?? []
report('viewer sees only in-scope channels (2, own org)', viewerChannels.status === 200 && viewerVisible.length === 2 &&
  viewerVisible.every((c) => c.org_unit_id === org.json?.id), `count=${viewerVisible.length} org=${viewerVisible[0]?.org_unit_name}`)
report('viewer channel name = COALESCE(display, report)', viewerVisible.every((c) => c.name === c.report_name), `names=${viewerVisible.map((c) => c.name).join(',')}`)

// 设备可见性：viewer 对自己范围内设备可查通道；对范围外设备 403
const scoped = await api('GET', `/api/v1/devices/${deviceID}/channels`, { token: viewerToken })
report('viewer device-channels in scope -> 200', scoped.status === 200, `status=${scoped.status}`)
const foreignDevice = await api('POST', '/api/v1/devices', {
  token: adminToken,
  body: { org_unit_id: otherOrg.json?.id, center_code: '34020000', device_type: '132', device_name: 'e2e-other', manufacturer: 'e2e', sip_realm: '3402000000', password: 'catalog-e2e-pass', enabled: true },
})
const foreignAccess = await sendCatalog(foreignDevice.json?.device_access_id, sipRealm, 1, 1, [{ code: '34020000001320000201', name: '他方通道', status: 'ON' }])
await waitFor(async () => {
  const res = await api('GET', `/api/v1/devices/${foreignDevice.json?.id}/channels`, { token: adminToken })
  return res.json?.catalog_state === 'ok'
})
const forbidden = await api('GET', `/api/v1/devices/${foreignDevice.json?.id}/channels`, { token: viewerToken })
report('viewer device-channels out of scope -> 403', forbidden.status === 403, `status=${forbidden.status}`)
const viewerAll = await api('GET', '/api/v1/channels', { token: viewerToken })
report('viewer flat list still excludes foreign org channels', (viewerAll.json?.channels ?? []).every((c) => c.org_unit_id !== otherOrg.json?.id),
  `count=${viewerAll.json?.channels?.length}`)

// admin 全量可见
const adminList = await api('GET', '/api/v1/channels', { token: adminToken })
report('node_admin sees all tenant channels', (adminList.json?.channels ?? []).length >= 3, `count=${adminList.json?.channels?.length}`)

// ---------- F. cleanup ----------
await api('DELETE', `/api/v1/devices/${deviceID}`, { token: adminToken })
await api('DELETE', `/api/v1/devices/${foreignDevice.json?.id}`, { token: adminToken })
await api('DELETE', `/api/v1/users/${viewer.json?.id}`, { token: adminToken })
await api('DELETE', `/api/v1/org-units/${org.json?.id}`, { token: adminToken })
await api('DELETE', `/api/v1/org-units/${otherOrg.json?.id}`, { token: adminToken })

const failed = results.filter((r) => !r.pass)
console.log(`\n${failed.length === 0 ? 'ALL PASS' : failed.length + ' FAILED'} (${results.length} checks)`)
if (failed.length > 0) process.exit(1)
