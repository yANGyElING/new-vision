import type { Page, Route } from '@playwright/test'
import { test as base, expect } from '@playwright/test'

/**
 * 用户页（/channels）：横幅三态、离线灰显、缺失徽标、骨架占位。
 * D49：主语是通道，不出现设备概念。
 */

type Fixture = (route: Route) => Promise<void>

async function mockChannels(page: Page, body: unknown) {
  await page.route('**/api/v1/auth/me', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        user: { id: 'u2', tenant_id: 't1', username: 'xiaoli', display_name: '小李', status: 'active', all_orgs: false, roles: ['viewer'], org_ids: ['22222222-2222-4222-8222-222222222222'] },
        roles: ['viewer'],
        org_scopes: [],
        all_orgs: false,
      }),
    }))
  await page.route('**/api/health', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ service: 'node-app', status: 'ready', checks: { postgres: 'up', redis: 'up' } }),
    }))
  await page.route('**/api/v1/channels', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) }))
  await page.addInitScript(() => localStorage.setItem('nv_token', 'e2e-token'))
}

const basePayload = {
  devices: [
    { id: 'd1', device_name: '五凤泵站 NVR', state: 'online', catalog_state: 'ok', catalog_last_ok_at: '2026-09-15T03:00:00Z' },
    { id: 'd2', device_name: '苍霞泵站枪机', state: 'offline', catalog_state: 'ok', catalog_last_ok_at: '2026-09-15T03:00:00Z' },
  ],
  channels: [
    { id: 'c1', device_id: 'd1', channel_code: '34020000001320000001', name: '大门口', report_name: '大门口', display_name: null, reported_status: 'ON', missing: false, last_seen_in_catalog_at: '2026-09-15T03:00:00Z', org_unit_id: '22222222-2222-4222-8222-222222222222', org_unit_name: '五凤泵站' },
    { id: 'c2', device_id: 'd1', channel_code: '34020000001320000002', name: '泵房主视角', report_name: '泵房主视角', display_name: null, reported_status: 'ON', missing: false, last_seen_in_catalog_at: '2026-09-15T03:00:00Z', org_unit_id: '22222222-2222-4222-8222-222222222222', org_unit_name: '五凤泵站' },
    { id: 'c3', device_id: 'd1', channel_code: '34020000001320000003', name: '后院围墙', report_name: '后院围墙', display_name: null, reported_status: 'OFF', missing: true, last_seen_in_catalog_at: '2026-09-15T02:00:00Z', org_unit_id: '22222222-2222-4222-8222-222222222222', org_unit_name: '五凤泵站' },
    { id: 'c4', device_id: 'd2', channel_code: '34020000001320000004', name: '江堤枪机', report_name: '江堤枪机', display_name: null, reported_status: 'ON', missing: false, last_seen_in_catalog_at: '2026-09-15T03:00:00Z', org_unit_id: null, org_unit_name: null },
  ],
}

const test = base.extend<{ channelsPage: Page }>({
  channelsPage: async ({ page }, use) => {
    await mockChannels(page, basePayload)
    await page.goto('/channels')
    await expect(page.locator('.user-card').first()).toBeVisible()
    await use(page)
  },
})

test('通道宫格：在线点、缺失徽标、离线设备整组灰显', async ({ channelsPage }) => {
  const cards = channelsPage.locator('.user-card')
  await expect(cards).toHaveCount(4)

  const gate = cards.filter({ hasText: '大门口' })
  await expect(gate.locator('.user-dot.on')).toBeVisible()

  // 缺失通道：徽标 + 播放禁用 + 历史录像
  const missing = cards.filter({ hasText: '后院围墙' })
  await expect(missing.locator('.user-miss-tag')).toHaveText('缺失')
  await expect(missing.getByRole('button', { name: /播放/ })).toBeDisabled()
  await expect(missing.getByRole('button', { name: /历史录像/ })).toBeEnabled()

  // 设备离线：整组灰显 + 说明 + 播放禁用
  const dead = cards.filter({ hasText: '江堤枪机' })
  await expect(dead).toHaveClass(/dead/)
  await expect(dead.locator('.user-card-why')).toHaveText('设备离线')
  await expect(dead.getByRole('button', { name: /播放/ })).toBeDisabled()

  // 顶部计数：4 路 · 2 路在线
  await expect(channelsPage.locator('.user-head-sub')).toContainText('4 路通道')
  await expect(channelsPage.locator('.user-head-sub')).toContainText('2 路在线')
})

test('组织筛选片：点击过滤宫格', async ({ channelsPage }) => {
  await expect(channelsPage.locator('.user-chip')).toHaveCount(2) // 全部 + 五凤泵站
  await channelsPage.getByRole('button', { name: '五凤泵站', exact: true }).click()
  await expect(channelsPage.locator('.user-card')).toHaveCount(3)
  await channelsPage.getByRole('button', { name: '全部', exact: true }).click()
  await expect(channelsPage.locator('.user-card')).toHaveCount(4)
})

test('同步中横幅：圆环出现，无数字（用户页只说人话）', async ({ page }) => {
  await mockChannels(page, {
    devices: [{ ...basePayload.devices[0], catalog_state: 'in_progress' }],
    channels: basePayload.channels,
  })
  await page.goto('/channels')
  const banner = page.locator('.user-banner.ing')
  await expect(banner).toBeVisible()
  await expect(banner.locator('.ring .arc')).toBeVisible()
  await expect(banner).toContainText('正在获取最新通道列表')
})

test('失败横幅：提示 + 重试按钮，通道保留旧快照', async ({ page }) => {
  await mockChannels(page, {
    devices: [{ ...basePayload.devices[0], catalog_state: 'failed' }],
    channels: basePayload.channels.slice(0, 2),
  })
  await page.goto('/channels')
  const banner = page.locator('.user-banner.bad')
  await expect(banner).toBeVisible()
  await expect(banner).toContainText('通道列表更新失败')
  await expect(banner.getByRole('button', { name: '重试' })).toBeVisible()
  // 旧快照仍在渲染
  await expect(page.locator('.user-card')).toHaveCount(2)
})

test('从未同步：骨架占位，不显示横幅', async ({ page }) => {
  await mockChannels(page, {
    devices: [{ ...basePayload.devices[0], catalog_state: 'never' }],
    channels: [],
  })
  await page.goto('/channels')
  await expect(page.locator('.user-card.skeleton')).toHaveCount(6)
  await expect(page.locator('.user-banner')).toHaveCount(0)
})
