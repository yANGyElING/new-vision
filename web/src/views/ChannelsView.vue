<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  AlertTriangle, Check, Film, Monitor, Play, RefreshCw, Search, Video, X, Zap,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import ProgressRing from '@/components/ProgressRing.vue'
import { me } from '@/api/auth'
import { listChannels, type ChannelView, type ChannelDeviceBrief } from '@/api/channels'

/**
 * 用户页面（D49）：主语是通道，不出现「设备/NVR」概念。
 * - 横幅三态：同步中（圆环）/ 失败（重试）/ 从未同步（骨架占位）
 * - 离线设备的通道整组灰显（保留最后快照）；缺失通道带徽标，可看历史录像，播放禁用
 * - 组织以横排筛选片呈现（不要侧边树）
 * Phase 1：不挂 SSE，静态渲染；「更新通道列表」为客户端重新拉取（服务端触发在任务 7）。
 */
const channels = ref<ChannelView[]>([])
const devices = ref<ChannelDeviceBrief[]>([])
const loading = ref(false)
const loadError = ref('')
const flash = ref('')
const nodeAdmin = ref(false)

const search = ref('')
const statusFilter = ref<'all' | 'online'>('all')
const orgFilter = ref<string | null>(null)

function flashMessage(msg: string) {
  flash.value = msg
  window.setTimeout(() => { if (flash.value === msg) flash.value = '' }, 3000)
}

async function loadChannels() {
  loading.value = true
  loadError.value = ''
  try {
    const result = await listChannels()
    devices.value = result.devices
    channels.value = result.channels
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '加载通道列表失败'
  } finally {
    loading.value = false
  }
}

const deviceByID = computed<Map<string, ChannelDeviceBrief>>(() => {
  const map = new Map<string, ChannelDeviceBrief>()
  for (const d of devices.value) map.set(d.id, d)
  return map
})

// 横幅三态：任一设备同步中 → 同步横幅；否则任一失败 → 失败横幅
const banner = computed<'none' | 'ing' | 'bad'>(() => {
  if (devices.value.some((d) => d.catalog_state === 'in_progress')) return 'ing'
  if (devices.value.some((d) => d.catalog_state === 'failed')) return 'bad'
  return 'none'
})

// 从未同步：有可见设备但零通道且全部从未同步 → 骨架占位
const neverSynced = computed(() =>
  channels.value.length === 0 &&
  devices.value.length > 0 &&
  devices.value.every((d) => d.catalog_state === 'never'),
)

const orgs = computed<{ id: string; name: string }[]>(() => {
  const seen = new Map<string, string>()
  for (const ch of channels.value) {
    if (ch.org_unit_id && ch.org_unit_name) seen.set(ch.org_unit_id, ch.org_unit_name)
  }
  return [...seen.entries()].map(([id, name]) => ({ id, name }))
})

function isChannelOnline(ch: ChannelView): boolean {
  if (ch.missing) return false
  const device = deviceByID.value.get(ch.device_id)
  if (!device || device.state !== 'online') return false
  return ch.reported_status === 'ON'
}

const filteredChannels = computed(() => {
  let list = channels.value
  const q = search.value.trim().toLowerCase()
  if (q) list = list.filter((ch) => ch.name.toLowerCase().includes(q) || (ch.channel_code ?? '').includes(q))
  if (statusFilter.value === 'online') list = list.filter((ch) => isChannelOnline(ch))
  if (orgFilter.value) list = list.filter((ch) => ch.org_unit_id === orgFilter.value)
  return list
})

const onlineCount = computed(() => filteredChannels.value.filter((ch) => isChannelOnline(ch)).length)

function deviceOf(ch: ChannelView): ChannelDeviceBrief | undefined {
  return deviceByID.value.get(ch.device_id)
}

function soon(action: string) {
  flashMessage(`${action}功能将在后续版本提供`)
}

onMounted(() => {
  void loadChannels()
  void me().then((info) => {
    nodeAdmin.value = (info.roles ?? []).includes('node_admin')
  }).catch(() => {})
})
</script>

<template>
  <div class="user-shell">
    <header class="user-topbar">
      <div class="user-brand">
        <span class="user-logo"><Monitor :size="18" :stroke-width="2.2" /></span>
        <div class="user-brand-text">
          <strong>new-vision</strong>
          <span>监控</span>
        </div>
      </div>
      <nav class="user-nav" aria-label="主导航">
        <RouterLink to="/channels" class="user-nav-link active" aria-current="page">监控</RouterLink>
        <RouterLink to="/devices" class="user-nav-link">设备管理</RouterLink>
        <RouterLink v-if="nodeAdmin" to="/users" class="user-nav-link">用户管理</RouterLink>
        <RouterLink v-if="nodeAdmin" to="/identity" class="user-nav-link">组织架构</RouterLink>
      </nav>
    </header>

    <header class="user-head">
      <div class="user-head-inner">
        <h1>监控</h1>
        <span class="user-head-sub">{{ filteredChannels.length }} 路通道 · {{ onlineCount }} 路在线</span>
      </div>
    </header>

    <main class="user-main">
      <Transition name="toast">
        <div v-if="flash" class="user-flash" role="status">
          <Check :size="15" />{{ flash }}
        </div>
      </Transition>

      <!-- toolbar -->
      <div class="user-toolbar">
        <div class="user-search">
          <Search :size="16" :stroke-width="2.5" class="user-search-icon" />
          <input v-model="search" placeholder="搜通道名" aria-label="搜索通道" />
          <button v-if="search" class="user-search-clear" type="button" aria-label="清空搜索" @click="search = ''"><X :size="13" /></button>
        </div>
        <select v-model="statusFilter" class="user-select" aria-label="按状态筛选">
          <option value="all">全部状态</option>
          <option value="online">仅在线</option>
        </select>
        <span class="user-toolbar-spacer" />
        <button class="user-btn" type="button" :disabled="loading" @click="loadChannels">
          <RefreshCw :size="15" :class="{ spinning: loading }" />更新通道列表
        </button>
      </div>

      <!-- error -->
      <div v-if="loadError" class="user-error" role="alert">
        <AlertTriangle :size="18" />
        <div>
          <strong>加载失败</strong>
          <p>{{ loadError }}</p>
        </div>
        <button class="user-btn" type="button" @click="loadChannels"><RefreshCw :size="14" />重试</button>
      </div>

      <!-- 同步中横幅（圆环无数字：用户页只说人话） -->
      <div v-else-if="banner === 'ing'" class="user-banner ing" role="status">
        <ProgressRing :size="24" :progress="-1" state="ing" />
        <div class="user-banner-txt">
          正在获取最新通道列表
          <span class="user-banner-note">已连接设备的正常播放不受影响</span>
        </div>
        <button type="button" @click="loadChannels">刷新</button>
      </div>

      <!-- 失败横幅（保留旧快照 + 重试） -->
      <div v-else-if="banner === 'bad'" class="user-banner bad" role="alert">
        <AlertTriangle :size="18" />
        <div class="user-banner-txt">
          通道列表更新失败
          <span class="user-banner-note">当前显示的是最后一次成功同步的版本</span>
        </div>
        <button type="button" @click="loadChannels">重试</button>
      </div>

      <!-- 组织筛选片（D49：横排筛选片，不要侧边树） -->
      <div v-if="orgs.length > 0" class="user-chips" role="group" aria-label="按组织筛选">
        <button class="user-chip" :class="{ on: orgFilter === null }" type="button" @click="orgFilter = null">全部</button>
        <button
          v-for="org in orgs" :key="org.id"
          class="user-chip" :class="{ on: orgFilter === org.id }"
          type="button" @click="orgFilter = org.id"
        >{{ org.name }}</button>
      </div>

      <!-- 骨架占位：从未同步 -->
      <div v-if="loading && channels.length === 0" class="user-grid" aria-label="加载中">
        <div v-for="n in 6" :key="n" class="user-card skeleton">
          <span class="sk-bar" />
          <span class="sk-bar short" />
        </div>
      </div>
      <div v-else-if="neverSynced" class="user-grid" aria-label="等待目录同步">
        <div v-for="n in 6" :key="n" class="user-card skeleton">
          <span class="sk-bar" />
          <span class="sk-bar short" />
        </div>
      </div>

      <!-- 通道宫格 -->
      <div v-else-if="filteredChannels.length > 0" class="user-grid">
        <div
          v-for="ch in filteredChannels"
          :key="ch.id"
          class="user-card"
          :class="{ dead: deviceOf(ch)?.state !== 'online' }"
        >
          <div class="user-card-loc">{{ ch.org_unit_name || '未分组' }}<template v-if="ch.missing"> · 已缺失</template></div>
          <div class="user-card-name">
            <span class="user-dot" :class="isChannelOnline(ch) ? 'on' : 'off'" />
            {{ ch.name || ch.channel_code }}
            <span v-if="ch.missing" class="user-miss-tag">缺失</span>
          </div>
          <div v-if="deviceOf(ch)?.state !== 'online'" class="user-card-why">设备离线</div>
          <div class="user-card-acts">
            <button class="user-icon-btn" type="button" :disabled="!isChannelOnline(ch)" @click="soon('播放')">
              <Play :size="14" :stroke-width="2.5" />播放
            </button>
            <button class="user-icon-btn" type="button" @click="soon('录像')">
              <Film :size="14" :stroke-width="2.5" />{{ ch.missing || deviceOf(ch)?.state !== 'online' ? '历史录像' : '录像' }}
            </button>
          </div>
        </div>
      </div>

      <!-- 空态 -->
      <div v-else-if="!loading" class="user-empty" role="status">
        <span class="user-empty-icon"><Video :size="26" /></span>
        <strong>{{ search || statusFilter !== 'all' || orgFilter ? '没有符合条件的通道' : '还没有可见的通道' }}</strong>
        <p>{{ search || statusFilter !== 'all' || orgFilter ? '试试调整搜索词或筛选条件。' : '设备完成目录同步后，通道会出现在这里。' }}</p>
        <button v-if="search || statusFilter !== 'all' || orgFilter" class="user-btn" type="button" @click="search = ''; statusFilter = 'all'; orgFilter = null">
          <X :size="15" />清除筛选
        </button>
      </div>
    </main>

    <footer class="user-footer">
      <span class="user-footer-note"><Zap :size="13" />{{ channels.length }} 路通道</span>
    </footer>
  </div>
</template>

<style scoped>
.user-shell { min-height: 100vh; background: var(--bg-page); color: var(--text-primary); font-family: var(--font-sans); }
/* ---------- topbar（与设备页一致的深色铬合金） ---------- */
.user-topbar { display: flex; align-items: center; gap: 30px; padding: 0 32px; height: 62px; background: #11161c; color: #e8ebee; border-bottom: 1px solid #1f2730; position: sticky; top: 0; z-index: 30; }
.user-brand { display: flex; align-items: center; gap: 11px; }
.user-logo { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; color: #fff; background: linear-gradient(135deg, #2d3742, #1c242d); border: 1px solid #333f4b; border-radius: 9px; }
.user-brand-text { display: grid; gap: 1px; }
.user-brand-text strong { font-size: 14px; }
.user-brand-text span { font-size: 11px; color: #8b98a5; }
.user-nav { display: flex; gap: 4px; margin-left: 6px; }
.user-nav-link { padding: 8px 13px; color: #9aa7b3; font-size: 13.5px; font-weight: 600; text-decoration: none; border-radius: 7px; transition: color .15s, background .15s; }
.user-nav-link:hover { color: #fff; background: rgba(255,255,255,.07); }
.user-nav-link.active { color: #fff; background: rgba(255,255,255,.11); }
/* ---------- head ---------- */
.user-head { position: sticky; top: 62px; z-index: 10; background: color-mix(in srgb, var(--bg-card) 85%, transparent); -webkit-backdrop-filter: blur(20px); backdrop-filter: blur(20px); border-bottom: 1px solid var(--border-faint); }
.user-head-inner { max-width: 1060px; margin: 0 auto; padding: 20px 32px; display: flex; align-items: baseline; gap: 12px; }
.user-head h1 { margin: 0; font-size: 28px; font-weight: 700; letter-spacing: -0.01em; }
.user-head-sub { color: var(--text-tertiary); font-size: 15px; font-variant-numeric: tabular-nums; }
.user-main { max-width: 1060px; margin: 0 auto; padding: 0 32px 64px; }
/* ---------- toast ---------- */
.user-flash { display: inline-flex; align-items: center; gap: 8px; margin-top: 18px; padding: 10px 15px; color: var(--success); background: var(--success-bg); border: 1px solid var(--success-border); border-radius: 9px; font-size: 13px; font-weight: 600; }
.toast-enter-active, .toast-leave-active { transition: opacity .2s, transform .2s; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(-6px); }
/* ---------- toolbar ---------- */
.user-toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin: 18px 0 14px; }
.user-toolbar-spacer { flex: 1; }
.user-search { position: relative; width: 220px; }
.user-search-icon { position: absolute; left: 13px; top: 50%; transform: translateY(-50%); color: var(--text-muted); pointer-events: none; }
.user-search input { width: 100%; padding: 9px 34px 9px 38px; border: 1px solid var(--border-input); border-radius: var(--r-sm); background: var(--bg-card); font-size: 14px; color: var(--text-primary); outline: none; box-sizing: border-box; font-family: inherit; transition: border-color .15s, box-shadow .15s; }
.user-search input:focus { border-color: var(--border-hover); box-shadow: 0 0 0 3px var(--focus-ring); }
.user-search-clear { position: absolute; right: 8px; top: 50%; transform: translateY(-50%); display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; color: var(--text-tertiary); background: none; border: 0; border-radius: 999px; cursor: pointer; }
.user-search-clear:hover { color: var(--text-primary); background: var(--bg-hover); }
.user-select { height: 38px; padding: 0 12px; font: inherit; font-size: 13.5px; color: var(--text-secondary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: var(--r-sm); cursor: pointer; }
.user-select:focus-visible { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px var(--focus-ring); }
.user-btn { display: inline-flex; align-items: center; gap: 7px; padding: 9px 14px; color: var(--text-primary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: var(--r-sm); font-size: 14px; font-weight: 500; cursor: pointer; transition: transform .1s ease, opacity .15s ease, border-color .15s; }
.user-btn:hover:not(:disabled) { border-color: var(--border-hover); }
.user-btn:active { transform: scale(0.97); }
.user-btn:disabled { opacity: .45; cursor: default; transform: none; }
.user-btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
/* ---------- banner 三态（D50/D51） ---------- */
.user-banner { display: flex; align-items: center; gap: 12px; margin-bottom: 14px; padding: 12px 16px; border-radius: 12px; font-size: 14px; }
.user-banner.ing { background: color-mix(in srgb, var(--accent-blue) 10%, var(--bg-card)); color: var(--text-primary); }
.user-banner.bad { background: color-mix(in srgb, var(--accent-red) 9%, var(--bg-card)); color: var(--text-primary); }
.user-banner .user-banner-txt { min-width: 0; }
.user-banner .user-banner-note { display: block; margin-top: 2px; font-size: 12px; color: var(--text-tertiary); }
.user-banner button { margin-left: auto; flex-shrink: 0; border: 0; background: none; color: var(--accent-blue); font-size: 14px; font-weight: 500; padding: 6px 10px; border-radius: 8px; cursor: pointer; transition: transform .1s ease, background .15s ease; }
.user-banner button:active { transform: scale(0.97); background: var(--bg-subtle); }
.user-banner button:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
/* ---------- org chips（D49：横排筛选片） ---------- */
.user-chips { display: flex; gap: 8px; margin: 4px 0 16px; flex-wrap: wrap; align-items: center; }
.user-chip { padding: 6px 14px; border: 1px solid var(--border-input); border-radius: 999px; background: var(--bg-card); color: var(--text-secondary); font-size: 13px; font-weight: 500; cursor: pointer; transition: transform .1s ease, background .15s ease, color .15s; }
.user-chip:active { transform: scale(0.96); }
.user-chip.on { background: var(--btn-primary-bg); color: var(--bg-card); border-color: transparent; }
.user-chip:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
/* ---------- 通道宫格 ---------- */
.user-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); gap: 14px; }
.user-card { background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); padding: 16px 18px; box-shadow: var(--shadow-card); transition: transform .18s cubic-bezier(0.32, 0.72, 0, 1), box-shadow .18s ease; }
.user-card:hover { transform: translateY(-1px); box-shadow: var(--shadow-elevated); }
.user-card-loc { font-size: 13px; color: var(--text-tertiary); margin-bottom: 6px; }
.user-card-name { font-weight: 600; font-size: 17px; display: flex; align-items: center; gap: 8px; letter-spacing: -0.01em; min-width: 0; }
.user-card-name > :first-child { flex-shrink: 0; }
.user-card-why { font-size: 13px; color: var(--text-tertiary); margin-top: 4px; }
.user-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.user-dot.on { background: var(--accent-green); box-shadow: 0 0 6px color-mix(in srgb, var(--accent-green) 60%, transparent); }
.user-dot.off { background: var(--text-muted); }
.user-miss-tag { font-size: 12px; color: var(--accent-red); font-weight: 500; flex-shrink: 0; }
.user-card.dead { opacity: .55; }
.user-card.dead .user-card-name { color: var(--text-tertiary); }
.user-card-acts { display: flex; gap: 8px; margin-top: 13px; }
.user-icon-btn { flex: 1; display: inline-flex; align-items: center; justify-content: center; gap: 5px; padding: 6px 11px; border: 1px solid var(--border-input); background: var(--bg-card); border-radius: 8px; font-size: 13px; font-weight: 500; color: var(--text-primary); cursor: pointer; transition: transform .1s ease, opacity .15s ease, border-color .15s; }
.user-icon-btn:hover:not(:disabled) { border-color: var(--border-hover); }
.user-icon-btn:active:not(:disabled) { transform: scale(0.97); }
.user-icon-btn:disabled { opacity: .45; cursor: default; transform: none; }
.user-icon-btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
/* ---------- skeleton ---------- */
.user-card.skeleton { border-style: dashed; box-shadow: none; }
.sk-bar { display: block; height: 16px; border-radius: 8px; background: var(--bg-subtle); margin-top: 10px; animation: shimmer 1.4s ease infinite; }
.sk-bar.short { width: 55%; }
@keyframes shimmer { 0%, 100% { opacity: 1; } 50% { opacity: .55; } }
/* ---------- error / empty ---------- */
.user-error { display: flex; align-items: center; gap: 13px; margin-bottom: 16px; padding: 15px 18px; color: var(--danger); background: var(--danger-bg); border: 1px solid var(--danger-border); border-radius: var(--r-xl); }
.user-error svg { flex-shrink: 0; }
.user-error strong { font-size: 13.5px; }
.user-error p { margin: 3px 0 0; color: var(--danger); opacity: .8; font-size: 12.5px; }
.user-error .user-btn { margin-left: auto; }
.user-empty { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 52px 20px; text-align: center; background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); box-shadow: var(--shadow-card); }
.user-empty-icon { display: inline-flex; align-items: center; justify-content: center; width: 52px; height: 52px; margin-bottom: 6px; color: var(--text-tertiary); background: var(--bg-subtle); border-radius: 999px; }
.user-empty strong { font-size: 14.5px; color: var(--text-primary); }
.user-empty p { margin: 0 0 10px; color: var(--text-tertiary); font-size: 13px; }
/* ---------- footer ---------- */
.user-footer { max-width: 1100px; margin: 0 auto; padding: 20px 32px 32px; display: flex; justify-content: space-between; color: var(--text-tertiary); font-size: 12px; }
.user-footer-note { display: inline-flex; align-items: center; gap: 6px; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* ---------- reduced motion（D51：位移退化为透明度） ---------- */
@media (prefers-reduced-motion: reduce) {
  .user-card, .user-card:hover { transform: none; }
  .sk-bar { animation: none; }
  .user-btn, .user-chip, .user-icon-btn, .user-nav-link, .user-search input, .user-select { transition: none; }
  .toast-enter-active, .toast-leave-active { transition: none; }
  .toast-enter-from, .toast-leave-to { opacity: 1; transform: none; }
}
/* ---------- responsive ---------- */
@media (max-width: 640px) {
  .user-topbar { gap: 14px; padding: 0 16px; }
  .user-brand-text span { display: none; }
  .user-head-inner { padding: 16px 16px; }
  .user-main { padding: 0 16px 48px; }
  .user-search { width: 100%; }
  .user-toolbar-spacer { display: none; }
  .user-icon-btn { min-height: 44px; }
}
</style>
