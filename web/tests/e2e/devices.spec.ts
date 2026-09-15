import type { Page, Route } from '@playwright/test'
import { test as base, expect } from '@playwright/test'

/**
 * 管理页（/devices）四态渲染 + 组织树 + 暗色 + 减动效。
 * 夹具按 D51：✓ 时间+路数 / 圆环+百分比·实收/预计 / ✗ 红环停位 / —
 */

type Fixture = (route: Route) => Promise<void>

const orgTree = [
  {
    id: '11111111-1111-4111-8111-111111111111',
    name: '鼓楼片区',
    children: [
      { id: '22222222-2222-4222-8222-222222222222', name: '五凤泵站' },
      { id: '33333333-3333-4333-8333-333333333333', name: '洪山桥泵站' },
    ],
  },
  { id: '44444444-4444-4444-8444-444444444444', name: '第一水厂' },
]

const devicesFixture: Fixture = async (route) => {
  await route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify([
      {
        id: 'd1', tenant_id: 't1', org_unit_id: '22222222-2222-4222-8222-222222222222',
        device_access_id: '34020000001180000001', device_name: '五凤泵站机房 NVR', manufacturer: '海康威视',
        device_type: '118', sip_username: '34020000001180000001', sip_realm: '3402000000',
        digest_algorithm: 'MD5', enabled: true, profile_version: 1, access_sync_status: 'synced', access_synced_version: 1,
        catalog_state: 'ok', catalog_last_ok_at: '2026-09-15T03:00:00Z', catalog_last_count: 8,
        catalog_channels: { total: 8, online: 7, missing: 1 },
        created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-15T03:00:00Z',
        runtime: { state: 'online' },
      },
      {
        id: 'd2', tenant_id: 't1', org_unit_id: '33333333-3333-4333-8333-333333333333',
        device_access_id: '34020000001180000002', device_name: '洪山桥泵站 NVR', manufacturer: '大华',
        device_type: '118', sip_username: '34020000001180000002', sip_realm: '3402000000',
        digest_algorithm: 'MD5', enabled: true, profile_version: 1, access_sync_status: 'synced', access_synced_version: 1,
        catalog_state: 'in_progress', catalog_last_query_at: '2026-09-15T03:05:00Z',
        catalog_received: 20, catalog_total: 32,
        catalog_channels: { total: 4, online: 4, missing: 0 },
        created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-15T03:05:00Z',
        runtime: { state: 'online' },
      },
      {
        id: 'd3', tenant_id: 't1', org_unit_id: '44444444-4444-4444-8444-444444444444',
        device_access_id: '34020000001180000007', device_name: '第一水厂 NVR', manufacturer: '宇视',
        device_type: '118', sip_username: '34020000001180000007', sip_realm: '3402000000',
        digest_algorithm: 'MD5', enabled: true, profile_version: 1, access_sync_status: 'synced', access_synced_version: 1,
        catalog_state: 'failed', catalog_last_error: '超时', catalog_last_query_at: '2026-09-15T04:04:00Z',
        catalog_received: 26, catalog_total: 32,
        catalog_channels: { total: 2, online: 0, missing: 0 },
        created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-15T04:04:00Z',
        runtime: { state: 'online' },
      },
      {
        id: 'd4', tenant_id: 't1', org_unit_id: null,
        device_access_id: '34020000001320000009', device_name: '苍霞泵站枪机', manufacturer: '海康威视',
        device_type: '132', sip_username: '34020000001320000009', sip_realm: '3402000000',
        digest_algorithm: 'MD5', enabled: true, profile_version: 1, access_sync_status: 'pending', access_synced_version: null,
        catalog_state: 'never',
        created_at: '2026-09-15T04:00:00Z', updated_at: '2026-09-15T04:00:00Z',
        runtime: { state: 'online' },
      },
    ]),
  })
}

async function mockAPI(page: Page, devices: Fixture = devicesFixture) {
  await page.route('**/api/v1/auth/me', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        user: { id: 'u1', tenant_id: 't1', username: 'vision', display_name: 'Platform Admin', status: 'active', all_orgs: true, roles: ['node_admin'], org_ids: [] },
        roles: ['node_admin'],
        org_scopes: [],
        all_orgs: true,
      }),
    }))
  await page.route('**/api/health', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ service: 'node-app', status: 'ready', checks: { postgres: 'up', redis: 'up' } }),
    }))
  await page.route('**/api/v1/org-units', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(orgTree) }))
  await page.route('**/api/v1/tenants', (route) =>
    route.fulfill({ contentType: 'application/json', body: JSON.stringify([{ id: 't1', name: '福州水务集团', status: 'active' }]) }))
  await page.route('**/api/v1/devices', devices)
  await page.addInitScript(() => localStorage.setItem('nv_token', 'e2e-token'))
}

const test = base.extend<{ devicesPage: Page }>({
  devicesPage: async ({ page }, use) => {
    await mockAPI(page)
    await page.goto('/devices')
    await expect(page.getByRole('row').filter({ hasText: '五凤泵站机房 NVR' })).toBeVisible()
    await use(page)
  },
})

test('四态行渲染：✓ 时间+路数 / 圆环+数字 / ✗ 红环停位 / —', async ({ devicesPage }) => {
  const rows = devicesPage.locator('.prod-trow')
  await expect(rows).toHaveCount(4)

  // ok：✓ 时间 · 8 路
  const okRow = rows.filter({ hasText: '五凤泵站机房 NVR' })
  await expect(okRow.locator('.prod-catalog.ok')).toContainText('8 路')
  await expect(okRow.locator('.prod-catalog.ok svg')).toBeVisible()

  // in_progress：圆环 + 63% · 20/32 路
  const ingRow = rows.filter({ hasText: '洪山桥泵站 NVR' })
  await expect(ingRow.locator('.prod-catalog.ing .ring .arc')).toBeVisible()
  await expect(ingRow.locator('.prod-catalog.ing')).toContainText('20/32 路')

  // failed：红环 + 超时文案
  const failRow = rows.filter({ hasText: '第一水厂 NVR' })
  await expect(failRow.locator('.prod-catalog.bad .ring.fail')).toBeVisible()
  await expect(failRow.locator('.prod-catalog.bad')).toContainText('超时')

  // never：—
  const neverRow = rows.filter({ hasText: '苍霞泵站枪机' })
  await expect(neverRow.locator('.prod-catalog.never')).toHaveText('—')

  // 通道点阵：8 路 = 7 绿 + 1 空心红（缺失），计数 7/8 在线
  await expect(okRow.locator('.prod-dot.on')).toHaveCount(7)
  await expect(okRow.locator('.prod-dot.miss')).toHaveCount(1)
  await expect(okRow.locator('.prod-channel-count')).toHaveText('7/8 在线')

  // never 设备：通道列 —
  await expect(neverRow.locator('.prod-channel-none')).toHaveText('—')
})

test('同步中的设备：刷新按钮禁用；其余设备可点', async ({ devicesPage }) => {
  const ingRow = devicesPage.locator('.prod-trow').filter({ hasText: '洪山桥泵站 NVR' })
  await expect(ingRow.locator('.catalog-refresh')).toBeDisabled()
  const okRow = devicesPage.locator('.prod-trow').filter({ hasText: '五凤泵站机房 NVR' })
  await expect(okRow.locator('.catalog-refresh')).toBeEnabled()
})

test('组织树：嵌套导轨 + 父节点折叠 / 叶子小圆点', async ({ devicesPage }) => {
  const tree = devicesPage.locator('.prod-fold-tree')
  await expect(tree).toBeVisible()
  // 父节点（鼓楼片区）带折叠箭头，叶子（五凤泵站）为小圆点
  const gulu = tree.getByRole('button', { name: /鼓楼片区/ })
  await expect(gulu.locator('.org-chev')).toBeVisible()
  await expect(tree.getByRole('button', { name: /五凤泵站/ }).locator('.org-leaf-dot')).toBeVisible()
  // 折叠后子节点隐藏，再展开恢复
  await gulu.locator('.org-chev').click()
  await expect(tree.getByRole('button', { name: /五凤泵站/ })).toBeHidden()
  await gulu.locator('.org-chev').click()
  await expect(tree.getByRole('button', { name: /五凤泵站/ })).toBeVisible()
})

test('node_admin 可见所属租户列', async ({ devicesPage }) => {
  await expect(devicesPage.getByRole('columnheader', { name: '所属租户' })).toBeVisible()
  await expect(devicesPage.locator('.prod-trow').first()).toContainText('福州水务集团')
})

test('暗色模式：令牌翻转（卡片背景非浅色）', async ({ browser }) => {
  const context = await browser.newContext({ colorScheme: 'dark' })
  const page = await context.newPage()
  await mockAPI(page)
  await page.goto('/devices')
  const card = page.locator('.prod-table-card').first()
  await expect(card).toBeVisible()
  const bg = await card.evaluate((el) => getComputedStyle(el).backgroundColor)
  // 深色令牌 --bg-card = #151b23 → rgb(21, 27, 35)；浅色为 rgb(255, 255, 255)
  expect(bg).toBe('rgb(21, 27, 35)')
  await context.close()
})

test('prefers-reduced-motion：无位移动画', async ({ browser }) => {
  const context = await browser.newContext({ reducedMotion: 'reduce' })
  const page = await context.newPage()
  await mockAPI(page)
  await page.goto('/devices')
  const ring = page.locator('.prod-catalog.ing .ring').first()
  await expect(ring).toBeVisible()
  // demo 基线：reduce 下 arc 保留 0.2s 线性透明度过渡，但位移类（卡片 hover）全部退化
  const row = page.locator('.prod-trow').first()
  const rowTransition = await row.evaluate((el) => getComputedStyle(el).transition)
  expect(rowTransition).toContain('none')
  const transition = await ring.evaluate((el) => {
    const arc = el.querySelector('.arc') as SVGCircleElement | null
    return arc ? getComputedStyle(arc).transitionDuration : ''
  })
  expect(transition).toBeTruthy()
  await context.close()
})
