<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Activity, AlertTriangle, Check, ChevronLeft, ChevronRight, Cpu, Database,
  Edit3, Eye, Folder, FolderOpen, HardDrive, ListPlus, Monitor, Network, Pause, Play, Plus, Radio, RefreshCw,
  Search, Server, ShieldCheck, Trash2, WifiOff, X, Zap,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { useDevice } from '@/composables/useDevice'
import ProgressRing from '@/components/ProgressRing.vue'
import OrgTreeNode from '@/components/OrgTreeNode.vue'
import { fetchHealth, type HealthState } from '@/api/health'
import { me } from '@/api/auth'
import {
  listDevices, createDevice, setDeviceEnabled, updateDeviceMeta, updateDeviceOrg, deleteDevice,
  previewAccessID, deviceTypeLabel, DEVICE_TYPES, type Device, type ChannelSummary,
} from '@/api/devices'
import { listOrgUnits, listTenants, type OrgUnit, type Tenant } from '@/api/identity'

// ---------- health band ----------
const health = ref<HealthState>({ kind: 'loading' })
let healthRequestId = 0
let activeHealthController: AbortController | undefined

const healthTitle = computed(() => {
  switch (health.value.kind) {
    case 'ready': return '系统正常'
    case 'degraded': return '部分依赖异常'
    case 'unreachable': return '服务不可达'
    case 'invalid': return '响应异常'
    default: return '检测中…'
  }
})
const healthSummary = computed(() => {
  switch (health.value.kind) {
    case 'ready': return '节点服务运行正常'
    case 'degraded': return '依赖服务存在异常，请检查'
    case 'unreachable': return '无法连接节点服务'
    case 'invalid': return '健康检查响应格式异常'
    default: return '正在检测节点服务状态'
  }
})
const healthTone = computed(() => (health.value.kind === 'ready' ? 'up' : health.value.kind === 'degraded' ? 'warn' : 'down'))

// ---------- device detection ----------
const { isDesktop } = useDevice()

function healthCheck(name: string): string {
  if (health.value.kind === 'ready' || health.value.kind === 'degraded') {
    return health.value.health.checks[name as 'postgres' | 'redis'] ?? 'unknown'
  }
  return 'unknown'
}

async function refreshHealth() {
  const id = ++healthRequestId
  activeHealthController?.abort()
  activeHealthController = new AbortController()
  try {
    const state = await fetchHealth(4000, activeHealthController.signal)
    if (id === healthRequestId) health.value = state
  } catch (error) {
    if (id === healthRequestId) health.value = { kind: 'unreachable', message: error instanceof Error ? error.message : '请求失败', checkedAt: new Date() }
  }
}

// ---------- devices ----------
const devices = ref<Device[]>([])
const loading = ref(false)
const loadError = ref('')

const search = ref('')
const typeFilter = ref('')
const syncFilter = ref('')
const runtimeFilter = ref('')
const orgFilter = ref<string | null>(null) // null = all visible (incl. unassigned)
const orgFilterTree = ref<OrgUnit[]>([])
const page = ref(1)
const pageSize = 10

const busy = ref<Record<string, string>>({})
const flash = ref('')

function flashMessage(msg: string) {
  flash.value = msg
  window.setTimeout(() => { if (flash.value === msg) flash.value = '' }, 3000)
}

async function loadDevices() {
  loading.value = true
  loadError.value = ''
  try {
    devices.value = await listDevices()
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '加载设备列表失败'
  } finally {
    loading.value = false
  }
}

// Org tree (visible scope) for the folder-style sidebar + org filter.
// Selected org id = show devices in that subtree; null = all visible.
type FlatOrg = { org: OrgUnit; depth: number; path: string }
const flatOrgTree = computed<FlatOrg[]>(() => {
  const out: FlatOrg[] = []
  const walk = (nodes: OrgUnit[], depth: number, prefix: string) => {
    for (const n of nodes) {
      const path = prefix ? `${prefix} / ${n.name}` : n.name
      out.push({ org: n, depth, path })
      if (n.children?.length) walk(n.children, depth + 1, path)
    }
  }
  walk(orgFilterTree.value, 0, '')
  return out
})

// 树节点折叠（D51：父节点 ▸ 可收起；叶子小圆点）——由 OrgTreeNode 递归渲染，
// 这里只持有折叠集合。
const collapsedOrgs = ref<Set<string>>(new Set())
function toggleOrgCollapse(id: string) {
  const next = new Set(collapsedOrgs.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsedOrgs.value = next
}

// ids in the selected org subtree
function subtreeIDs(rootID: string): Set<string> {
  const ids = new Set<string>()
  const walk = (nodes: OrgUnit[]) => {
    for (const n of nodes) {
      ids.add(n.id)
      if (n.children?.length) walk(n.children)
    }
  }
  const node = flatOrgTree.value.find((f) => f.org.id === rootID)
  walk(node ? [node.org] : [])
  return ids
}

// device count in an org subtree (sidebar badge)
function orgCount(rootID: string): number {
  const ids = subtreeIDs(rootID)
  return devices.value.filter((d) => d.org_unit_id && ids.has(d.org_unit_id)).length
}

async function loadOrgTree() {
  try {
    orgFilterTree.value = await listOrgUnits()
  } catch {
    orgFilterTree.value = []
  }
}

const filteredDevices = computed(() => {
  const q = search.value.trim().toLowerCase()
  let list = devices.value
  if (q) {
    list = list.filter((d) => d.device_access_id.toLowerCase().includes(q) || (d.device_name || '').toLowerCase().includes(q))
  }
  if (orgFilter.value) {
    const ids = subtreeIDs(orgFilter.value)
    list = list.filter((d) => d.org_unit_id && ids.has(d.org_unit_id))
  }
  if (typeFilter.value) list = list.filter((d) => d.device_type === typeFilter.value)
  if (syncFilter.value) list = list.filter((d) => d.access_sync_status === syncFilter.value)
  if (runtimeFilter.value) list = list.filter((d) => runtimeState(d) === runtimeFilter.value)
  return [...list].sort((a, b) => a.device_access_id.localeCompare(b.device_access_id))
})

const totalPages = computed(() => Math.max(1, Math.ceil(filteredDevices.value.length / pageSize)))
const pageDevices = computed(() => filteredDevices.value.slice((page.value - 1) * pageSize, page.value * pageSize))

function resetPage() { page.value = 1 }

// ---------- metrics ----------
const stats = computed(() => {
  let online = 0, offline = 0, synced = 0, pending = 0
  for (const d of devices.value) {
    if (runtimeState(d) === 'online') online++; else offline++
    if (d.access_sync_status === 'synced') synced++; else pending++
  }
  return { total: devices.value.length, online, offline, synced, pending }
})

// ---------- create ----------
const createOpen = ref(false)
const typePickerOpen = ref(false)
const creating = ref(false)
const createError = ref('')
const orgTree = ref<OrgUnit[]>([])
const createForm = ref<{
  device_type: string; center_code: string; device_name: string; manufacturer: string
  sip_realm: string; password: string; enabled: boolean; org_unit_id: string
}>({
  device_type: DEVICE_TYPES[0].code, center_code: '34020000', device_name: '', manufacturer: '',
  sip_realm: '3402000000', password: '', enabled: true, org_unit_id: '',
})

const manufacturerOptions = ref(['海康威视', '大华', '宇视', '华为', '天地伟业', '科达', '其他'])
const manufacturerFilter = ref('')
const manufacturerOpen = ref(false)
const addingManufacturer = ref(false)
const newManufacturer = ref('')

const nodeAdmin = ref(false)

const filteredManufacturers = computed(() => {
  const q = manufacturerFilter.value.trim().toLowerCase()
  if (!q) return manufacturerOptions.value
  return manufacturerOptions.value.filter((m) => m.toLowerCase().includes(q))
})
const canAddManufacturer = computed(() => {
  const v = newManufacturer.value.trim()
  return v !== '' && !manufacturerOptions.value.includes(v)
})

function openTypePicker() { typePickerOpen.value = true; createError.value = '' }
function chooseType(code: string) {
  createForm.value.device_type = code
  typePickerOpen.value = false
  createOpen.value = true
  manufacturerOpen.value = false
}
function toggleManufacturerList() {
  manufacturerOpen.value = !manufacturerOpen.value
  addingManufacturer.value = false
  newManufacturer.value = ''
  manufacturerFilter.value = ''
}
function pickManufacturer(m: string) { createForm.value.manufacturer = m; manufacturerOpen.value = false }
function startAddManufacturer() { addingManufacturer.value = true; manufacturerFilter.value = '' }
function confirmAddManufacturer() {
  const v = newManufacturer.value.trim()
  if (v === '') return
  if (!manufacturerOptions.value.includes(v)) manufacturerOptions.value.push(v)
  createForm.value.manufacturer = v
  manufacturerOpen.value = false
  addingManufacturer.value = false
  newManufacturer.value = ''
}
const accessIDPreview = computed(() => {
  if (createForm.value.center_code.length !== 8) return ''
  return previewAccessID(createForm.value.center_code, createForm.value.device_type)
})
function closeCreate() { createOpen.value = false; createError.value = '' }

async function submitCreate() {
  createError.value = ''
  creating.value = true
  try {
    const device = await createDevice({
      org_unit_id: createForm.value.org_unit_id || undefined,
      center_code: createForm.value.center_code,
      device_type: createForm.value.device_type,
      device_name: createForm.value.device_name.trim(),
      manufacturer: createForm.value.manufacturer,
      sip_realm: createForm.value.sip_realm,
      password: createForm.value.password,
      enabled: createForm.value.enabled,
    })
    closeCreate()
    createForm.value.password = ''
    createForm.value.device_name = ''
    createForm.value.manufacturer = ''
    flashMessage(`设备 ${device.device_access_id} 创建成功`)
    await loadDevices()
  } catch (error) {
    createError.value = error instanceof Error ? error.message : '创建失败'
  } finally {
    creating.value = false
  }
}

// ---------- edit ----------
const editingDevice = ref<Device | null>(null)
const editForm = ref({ device_name: '', manufacturer: '', org_unit_id: '' })
const savingEdit = ref(false)
const editError = ref('')

function openEdit(device: Device) {
  editingDevice.value = device
  editForm.value = {
    device_name: device.device_name,
    manufacturer: device.manufacturer,
    org_unit_id: device.org_unit_id ?? '',
  }
  editError.value = ''
}
function closeEdit() { editingDevice.value = null; editError.value = '' }
async function submitEdit() {
  if (!editingDevice.value) return
  editError.value = ''
  savingEdit.value = true
  try {
    if (editForm.value.org_unit_id !== (editingDevice.value.org_unit_id ?? '')) {
      await updateDeviceOrg(editingDevice.value.id, editForm.value.org_unit_id || null)
    }
    await updateDeviceMeta(editingDevice.value.id, {
      device_name: editForm.value.device_name.trim(),
      manufacturer: editForm.value.manufacturer.trim(),
    })
    flashMessage('设备信息已更新')
    closeEdit()
    await loadDevices()
  } catch (error) {
    editError.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    savingEdit.value = false
  }
}

// ---------- detail drawer ----------
const detailDevice = ref<Device | null>(null)
function openDetail(device: Device) { detailDevice.value = device }
function closeDetail() { detailDevice.value = null }

// ---------- actions ----------
async function toggleEnabled(device: Device) {
  busy.value[device.id] = 'toggle'
  try {
    await setDeviceEnabled(device.id, !device.enabled)
    flashMessage(device.enabled ? `设备 ${device.device_access_id} 已停用` : `设备 ${device.device_access_id} 已启用`)
    await loadDevices()
  } catch (error) {
    flashMessage(error instanceof Error ? error.message : '操作失败')
  } finally {
    delete busy.value[device.id]
  }
}

async function removeDevice(device: Device) {
  if (!window.confirm(`确定删除设备 ${device.device_access_id}（${device.device_name || '未命名'}）？此操作不可恢复。`)) return
  busy.value[device.id] = 'delete'
  try {
    await deleteDevice(device.id)
    flashMessage(`设备 ${device.device_access_id} 已删除`)
    await loadDevices()
  } catch (error) {
    flashMessage(error instanceof Error ? error.message : '删除失败')
  } finally {
    delete busy.value[device.id]
  }
}

function runtimeState(device: Device): string {
  return device.runtime?.state ?? 'offline'
}
function orgNameOf(id: string): string {
  return flatOrgTree.value.find((f) => f.org.id === id)?.org.name ?? id
}
function runtimeLabel(device: Device): string {
  switch (runtimeState(device)) {
    case 'online': return '在线'
    case 'offline': return '离线'
    default: return '未知'
  }
}
function syncLabel(device: Device): string {
  return device.access_sync_status === 'synced' ? '已同步' : '同步中'
}
function typeIcon(code: string) {
  switch (code) {
    case '132': return Radio
    case '118': case '111': return HardDrive
    case '200': return Server
    default: return Cpu
  }
}

// ---------- catalog sync (D50/D51) ----------
// 四态：✓ 时间·路数 / 圆环+百分比·实收/预计 / ✗ 红环停位 / —
function catalogState(d: Device): 'never' | 'ok' | 'in_progress' | 'failed' {
  return d.catalog_state ?? 'never'
}
function catalogPct(d: Device): number {
  if (d.catalog_received == null || d.catalog_total == null || d.catalog_total <= 0) return -1
  return d.catalog_received / d.catalog_total
}
function catalogOkTime(d: Device): string {
  if (!d.catalog_last_ok_at) return ''
  const date = new Date(d.catalog_last_ok_at)
  const hh = String(date.getHours()).padStart(2, '0')
  const mm = String(date.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}
function catalogFailText(d: Device): string {
  if (!d.catalog_last_error) return '同步失败'
  const time = d.catalog_last_query_at ? new Date(d.catalog_last_query_at) : null
  const hhmm = time ? `${String(time.getHours()).padStart(2, '0')}:${String(time.getMinutes()).padStart(2, '0')}` : ''
  const counts =
    d.catalog_received != null && d.catalog_total != null && d.catalog_total > 0
      ? ` · ${Math.round((d.catalog_received / d.catalog_total) * 100)}% · ${d.catalog_received}/${d.catalog_total} 路`
      : ''
  return `${hhmm ? hhmm + ' ' : ''}${d.catalog_last_error}${counts}`
}
// 通道点阵（D51）：最多 16 点，超出用 +N 提示；绿=上报在线，灰=上报离线，空心红=缺失
function channelDots(d: Device): { kind: 'on' | 'off' | 'miss'; title: string }[] {
  const s: ChannelSummary | null | undefined = d.catalog_channels
  if (!s || s.total <= 0) return []
  const dots: { kind: 'on' | 'off' | 'miss'; title: string }[] = []
  for (let i = 0; i < s.online; i++) dots.push({ kind: 'on', title: '在线' })
  const off = Math.max(0, s.total - s.online - s.missing)
  for (let i = 0; i < off; i++) dots.push({ kind: 'off', title: '离线' })
  for (let i = 0; i < s.missing; i++) dots.push({ kind: 'miss', title: '缺失' })
  return dots.slice(0, 16)
}
function channelOverflow(d: Device): number {
  const s = d.catalog_channels
  if (!s) return 0
  return Math.max(0, s.total - 16)
}
function channelSummaryText(d: Device): string {
  const s = d.catalog_channels
  if (!s || s.total <= 0) return '—'
  return `${s.online}/${s.total} 在线`
}
// 所属组织只显直接父级，完整路径进 title（D51）
const orgByID = computed<Map<string, OrgUnit>>(() => {
  const map = new Map<string, OrgUnit>()
  const walk = (nodes: OrgUnit[]) => {
    for (const n of nodes) {
      map.set(n.id, n)
      if (n.children?.length) walk(n.children)
    }
  }
  walk(orgFilterTree.value)
  return map
})
function orgDirectName(id?: string | null): string {
  if (!id) return '未分配'
  return orgByID.value.get(id)?.name ?? '—'
}
function orgFullPath(id?: string | null): string {
  if (!id) return '未分配'
  return flatOrgTree.value.find((f) => f.org.id === id)?.path ?? '—'
}

// 所属租户列：仅平台运维（node_admin）可见（D49）
const tenants = ref<Tenant[]>([])
const tenantNameByID = computed<Map<string, string>>(() => {
  const map = new Map<string, string>()
  for (const t of tenants.value) map.set(t.id, t.name)
  return map
})
function tenantName(id: string): string {
  return tenantNameByID.value.get(id) ?? id.slice(0, 8)
}

onMounted(() => {
  void loadDevices()
  void refreshHealth()
  void me().then((info) => {
    nodeAdmin.value = (info.roles ?? []).includes('node_admin')
    if (nodeAdmin.value) {
      void listTenants().then((list) => { tenants.value = list }).catch(() => {})
    }
    return loadOrgTree()
  }).catch(() => {})
  const timer = window.setInterval(refreshHealth, 30000)
  onUnmounted(() => {
    window.clearInterval(timer)
    activeHealthController?.abort()
  })
})
</script>

<template>
  <div class="prod-shell">
    <header class="prod-topbar">
      <div class="prod-brand">
        <span class="prod-logo"><Monitor :size="18" :stroke-width="2.2" /></span>
        <div class="prod-brand-text">
          <strong>new-vision</strong>
          <span>节点管理系统</span>
        </div>
      </div>
      <nav class="prod-nav" aria-label="主导航">
        <RouterLink to="/devices" class="prod-nav-link active" aria-current="page">设备管理</RouterLink>
        <RouterLink v-if="nodeAdmin" to="/users" class="prod-nav-link">用户管理</RouterLink>
        <RouterLink v-if="nodeAdmin" to="/identity" class="prod-nav-link">组织架构</RouterLink>
        <RouterLink to="/" class="prod-nav-link">测试控制台</RouterLink>
      </nav>
      <div class="prod-topbar-right">
        <span class="prod-health" :class="`health-${healthTone}`" :title="healthSummary">
          <span class="prod-health-dot" /><span>{{ healthTitle }}</span>
        </span>
        <span class="prod-health-sep" aria-hidden="true" />
        <span class="prod-deps" :title="`PostgreSQL ${healthCheck('postgres')} · Redis ${healthCheck('redis')}`">
          <Database :size="13" /><span class="dep-dot" :class="`dep-${healthCheck('postgres')}`" />
          <Server :size="13" /><span class="dep-dot" :class="`dep-${healthCheck('redis')}`" />
        </span>
      </div>
    </header>

    <!-- glass sticky title bar -->
    <header class="prod-head">
      <div class="prod-head-inner">
        <h1>设备管理</h1>
        <button class="prod-button prod-button-primary" type="button" @click="openTypePicker">
          <Plus :size="16" :stroke-width="2.2" />新增设备
        </button>
      </div>
    </header>

    <main class="prod-main">

      <Transition name="toast">
        <div v-if="flash" class="prod-flash" role="status">
          <Check :size="15" />{{ flash }}
        </div>
      </Transition>

      <!-- stats cards -->
      <div class="prod-stats" aria-label="设备统计">
        <div class="prod-stat">
          <span class="prod-stat-label"><Cpu :size="18" :stroke-width="2" />总设备</span>
          <strong class="prod-stat-value">{{ stats.total }}</strong>
        </div>
        <div class="prod-stat">
          <span class="prod-stat-label"><span class="stat-dot dot-online" />在线</span>
          <strong class="prod-stat-value">{{ stats.online }}</strong>
        </div>
        <div class="prod-stat">
          <span class="prod-stat-label"><span class="stat-dot dot-offline" />离线</span>
          <strong class="prod-stat-value">{{ stats.offline }}</strong>
        </div>
        <div class="prod-stat">
          <span class="prod-stat-label"><span class="stat-dot dot-sync" />已同步</span>
          <strong class="prod-stat-value">{{ stats.synced }}</strong>
        </div>
        <div class="prod-stat">
          <span class="prod-stat-label"><span class="stat-dot dot-pending" />同步中</span>
          <strong class="prod-stat-value">{{ stats.pending }}</strong>
        </div>
      </div>

      <!-- folder-style org sidebar + device list -->
      <div class="prod-fold">
        <aside class="prod-fold-tree" aria-label="组织架构">
          <div class="prod-fold-tree-head">
            <span class="prod-fold-tree-title"><Network :size="14" />组织</span>
            <button class="prod-fold-tree-clear" type="button" :disabled="orgFilter === null" title="显示全部" aria-label="显示全部" @click="orgFilter = null; resetPage()"><X :size="13" /></button>
          </div>
          <div class="prod-fold-tree-body">
            <button class="prod-fold-node" :class="{ active: orgFilter === null }" type="button" @click="orgFilter = null; resetPage()">
              <span class="prod-fold-node-icon"><FolderOpen :size="13" /></span>
              <span class="prod-fold-node-name">全部设备</span>
              <span class="prod-fold-node-count mono">{{ devices.length }}</span>
            </button>
            <OrgTreeNode
              :nodes="orgFilterTree"
              :count-of="orgCount"
              :active="orgFilter"
              :collapsed="collapsedOrgs"
              @select="(id: string) => { orgFilter = id; resetPage() }"
              @toggle="toggleOrgCollapse"
            />
            <button v-if="flatOrgTree.length === 0" class="prod-fold-node prod-fold-node-empty" type="button" @click="orgFilter = null; resetPage()">
              <span class="prod-fold-node-name">还没有组织</span>
            </button>
          </div>
        </aside>
        <div class="prod-fold-main">

      <!-- search & filter card -->
      <div class="prod-toolbar-card">
        <div class="prod-search">
          <Search :size="16" :stroke-width="2.5" class="prod-search-icon" />
          <input v-model="search" placeholder="搜索名称或接入 ID" @input="resetPage" aria-label="搜索设备" />
          <button v-if="search" class="prod-search-clear" type="button" aria-label="清空搜索" @click="search = ''; resetPage()"><X :size="13" /></button>
        </div>
        <select v-model="typeFilter" class="prod-select" @change="resetPage" aria-label="按类型筛选">
          <option value="">全部类型</option>
          <option v-for="t in DEVICE_TYPES" :key="t.code" :value="t.code">{{ t.label }}</option>
        </select>
        <div class="prod-pills" role="group" aria-label="按同步状态筛选">
          <button class="prod-pill" :class="{ active: syncFilter === '' }" type="button" @click="syncFilter = ''; resetPage()">全部同步</button>
          <button class="prod-pill" :class="{ active: syncFilter === 'synced' }" type="button" @click="syncFilter = 'synced'; resetPage()">已同步</button>
          <button class="prod-pill" :class="{ active: syncFilter === 'pending' }" type="button" @click="syncFilter = 'pending'; resetPage()">同步中</button>
        </div>
        <div class="prod-pills" role="group" aria-label="按运行状态筛选">
          <button class="prod-pill" :class="{ active: runtimeFilter === '' }" type="button" @click="runtimeFilter = ''; resetPage()">全部运行</button>
          <button class="prod-pill" :class="{ active: runtimeFilter === 'online' }" type="button" @click="runtimeFilter = 'online'; resetPage()">在线</button>
          <button class="prod-pill" :class="{ active: runtimeFilter === 'offline' }" type="button" @click="runtimeFilter = 'offline'; resetPage()">离线</button>
        </div>
        <button class="prod-refresh" type="button" :disabled="loading" aria-label="刷新设备列表" title="刷新" @click="loadDevices">
          <RefreshCw :size="16" :class="{ spinning: loading }" />
        </button>
      </div>

      <!-- error -->
      <div v-if="loadError" class="prod-error" role="alert">
        <AlertTriangle :size="18" />
        <div>
          <strong>加载失败</strong>
          <p>{{ loadError }}</p>
        </div>
        <button class="prod-button" type="button" @click="loadDevices"><RefreshCw :size="14" />重试</button>
      </div>

      <!-- skeleton -->
      <div v-else-if="loading" class="prod-table-card" :class="{ 'with-tenant': nodeAdmin }" aria-label="加载中">
        <div class="prod-thead" role="row">
          <div role="columnheader">设备</div>
          <div role="columnheader">接入 ID</div>
          <div v-if="nodeAdmin" role="columnheader">所属租户</div>
          <div role="columnheader">所属组织</div>
          <div role="columnheader">通道</div>
          <div role="columnheader">目录同步</div>
          <div role="columnheader" class="th-actions">操作</div>
        </div>
        <div v-for="n in 6" :key="n" class="prod-trow sk-row" role="row">
          <div role="cell"><span class="sk sk-name" /></div>
          <div role="cell"><span class="sk sk-id" /></div>
          <div v-if="nodeAdmin" role="cell"><span class="sk sk-pill" /></div>
          <div role="cell"><span class="sk sk-pill" /></div>
          <div role="cell"><span class="sk sk-dots" /></div>
          <div role="cell"><span class="sk sk-pill" /></div>
          <div role="cell"><span class="sk sk-actions" /></div>
        </div>
      </div>

      <!-- table -->
      <div v-else-if="isDesktop && filteredDevices.length > 0" class="prod-table-card" :class="{ 'with-tenant': nodeAdmin }">
        <div class="prod-thead" role="row">
          <div role="columnheader">设备</div>
          <div role="columnheader">接入 ID</div>
          <div v-if="nodeAdmin" role="columnheader">所属租户</div>
          <div role="columnheader">所属组织</div>
          <div role="columnheader">通道</div>
          <div role="columnheader">目录同步</div>
          <div role="columnheader" class="th-actions">操作</div>
        </div>
        <div v-for="device in pageDevices" :key="device.id" class="prod-trow" role="row" @dblclick="openDetail(device)">
          <div role="cell" class="prod-name-cell">
            <div class="prod-name-line">
              <span
                class="prod-runtime-dot"
                :class="runtimeState(device) === 'online' ? 'dot-on' : 'dot-off'"
                :title="runtimeState(device) === 'online' ? '在线' : '离线'"
              />
              <span class="prod-name">{{ device.device_name || '未命名' }}</span>
              <span class="prod-type-tag">{{ deviceTypeLabel(device.device_type) }}</span>
              <span v-if="!device.enabled" class="prod-disabled-tag">停用</span>
            </div>
            <div class="prod-name-sub">{{ device.manufacturer || '—' }}</div>
          </div>
          <div role="cell" class="prod-id-cell mono">{{ device.device_access_id }}</div>
          <div v-if="nodeAdmin" role="cell" class="prod-tenant-cell">{{ tenantName(device.tenant_id) }}</div>
          <div role="cell" :title="orgFullPath(device.org_unit_id)">{{ orgDirectName(device.org_unit_id) }}</div>
          <div role="cell">
            <div v-if="channelDots(device).length" class="prod-channel-cell" :class="{ dead: runtimeState(device) !== 'online' }">
              <span class="prod-dots" aria-hidden="true">
                <i v-for="(dot, i) in channelDots(device)" :key="i" class="prod-dot" :class="dot.kind" /><i v-if="channelOverflow(device) > 0" class="prod-dot-overflow mono">+{{ channelOverflow(device) }}</i>
              </span>
              <span class="prod-channel-count mono">{{ channelSummaryText(device) }}</span>
            </div>
            <span v-else class="prod-channel-none">—</span>
          </div>
          <div role="cell">
            <!-- 四态：✓ 时间·路数 / 圆环+数字 / ✗ 红环停位 / — -->
            <span v-if="catalogState(device) === 'ok'" class="prod-catalog ok">
              <Check :size="13" :stroke-width="2.5" />
              <span class="mono">{{ catalogOkTime(device) }} · {{ device.catalog_last_count ?? '—' }} 路</span>
            </span>
            <span v-else-if="catalogState(device) === 'in_progress'" class="prod-catalog ing">
              <ProgressRing :size="16" :progress="catalogPct(device)" state="ing" />
              <span v-if="catalogPct(device) >= 0" class="mono">{{ Math.round(catalogPct(device) * 100) }}% · {{ device.catalog_received }}/{{ device.catalog_total }} 路</span>
              <span v-else>同步中</span>
            </span>
            <span v-else-if="catalogState(device) === 'failed'" class="prod-catalog bad">
              <ProgressRing :size="16" :progress="catalogPct(device)" state="fail" />
              <span class="prod-catalog-error" :title="device.catalog_last_error ?? ''">{{ catalogFailText(device) }}</span>
            </span>
            <span v-else class="prod-catalog never">—</span>
          </div>
          <div role="cell" class="td-actions">
            <div class="prod-actions">
              <button
                class="prod-act catalog-refresh"
                type="button"
                :disabled="catalogState(device) === 'in_progress'"
                :title="catalogState(device) === 'in_progress' ? '目录同步中' : '刷新目录（即将上线）'"
                aria-label="刷新目录"
                @click="flashMessage('目录刷新将在下一版本提供')"
              >
                <RefreshCw :size="15" :class="{ spinning: catalogState(device) === 'in_progress' }" />
              </button>
              <button class="prod-act" type="button" title="查看详情" aria-label="查看详情" @click="openDetail(device)"><Eye :size="15" /></button>
              <button class="prod-act" type="button" title="编辑" aria-label="编辑" @click="openEdit(device)"><Edit3 :size="15" /></button>
              <button class="prod-act" type="button" :disabled="busy[device.id] === 'toggle'" :title="device.enabled ? '停用' : '启用'" :aria-label="device.enabled ? '停用' : '启用'" @click="toggleEnabled(device)">
                <Pause v-if="device.enabled" :size="15" /><Play v-else :size="15" />
              </button>
              <button class="prod-act danger" type="button" :disabled="busy[device.id] === 'delete'" title="删除" aria-label="删除" @click="removeDevice(device)"><Trash2 :size="15" /></button>
            </div>
          </div>
        </div>
      </div>

      <!-- mobile card list -->
      <div v-else-if="!isDesktop && filteredDevices.length > 0" class="prod-card-list" aria-label="设备列表">
        <div v-for="device in pageDevices" :key="device.id" class="prod-card-item">
          <div class="prod-card-main">
            <span class="prod-card-name">
              <span class="prod-runtime-dot" :class="runtimeState(device) === 'online' ? 'dot-on' : 'dot-off'" />
              {{ device.device_name || '未命名' }}
            </span>
            <span class="prod-card-sub">{{ deviceTypeLabel(device.device_type) }} · {{ device.manufacturer || '—' }} · {{ orgDirectName(device.org_unit_id) }}</span>
          </div>
          <div class="prod-card-id mono">{{ device.device_access_id }}</div>
          <div class="prod-card-status">
            <span v-if="!device.enabled" class="prod-pill pill-muted">停用</span>
            <span v-if="channelDots(device).length" class="prod-channel-count mono" :class="{ dead: runtimeState(device) !== 'online' }">{{ channelSummaryText(device) }}</span>
            <span v-if="catalogState(device) === 'ok'" class="prod-pill pill-synced">目录 ✓ {{ device.catalog_last_count ?? '—' }} 路</span>
            <span v-else-if="catalogState(device) === 'in_progress'" class="prod-pill pill-pending">目录同步中</span>
            <span v-else-if="catalogState(device) === 'failed'" class="prod-pill pill-failed">目录同步失败</span>
          </div>
          <div class="prod-card-actions">
            <button class="prod-act" type="button" title="查看详情" aria-label="查看详情" @click="openDetail(device)"><Eye :size="18" /></button>
            <button class="prod-act" type="button" title="编辑" aria-label="编辑" @click="openEdit(device)"><Edit3 :size="18" /></button>
            <button class="prod-act" type="button" :disabled="busy[device.id] === 'toggle'" :title="device.enabled ? '停用' : '启用'" :aria-label="device.enabled ? '停用' : '启用'" @click="toggleEnabled(device)">
              <Pause v-if="device.enabled" :size="18" /><Play v-else :size="18" />
            </button>
            <button class="prod-act danger" type="button" :disabled="busy[device.id] === 'delete'" title="删除" aria-label="删除" @click="removeDevice(device)"><Trash2 :size="18" /></button>
          </div>
        </div>
      </div>

      <!-- empty -->
      <div v-else class="prod-empty" role="status">
        <span class="prod-empty-icon"><Search v-if="search || typeFilter || syncFilter || runtimeFilter" :size="26" /><Cpu v-else :size="26" /></span>
        <strong>{{ search || typeFilter || syncFilter || runtimeFilter ? '没有符合条件的设备' : '还没有设备' }}</strong>
        <p>{{ search || typeFilter || syncFilter || runtimeFilter ? '试试调整搜索词或筛选条件。' : '点击“新增设备”创建第一台接入设备。' }}</p>
        <button v-if="!search && !typeFilter && !syncFilter && !runtimeFilter" class="prod-button prod-button-primary" type="button" @click="openTypePicker">
          <Plus :size="15" />新增设备
        </button>
        <button v-else class="prod-button" type="button" @click="search = ''; typeFilter = ''; syncFilter = ''; runtimeFilter = ''; resetPage()">
          <X :size="15" />清除筛选
        </button>
      </div>

      <!-- pagination -->
      <div v-if="filteredDevices.length > pageSize" class="prod-pagination">
        <span class="prod-page-count mono">{{ (page - 1) * pageSize + 1 }}-{{ Math.min(page * pageSize, filteredDevices.length) }} / {{ filteredDevices.length }}</span>
        <div class="prod-page-btns">
          <button class="prod-page-btn" type="button" :disabled="page <= 1" aria-label="上一页" @click="page--"><ChevronLeft :size="15" /></button>
          <span class="prod-page-info mono">{{ page }} / {{ totalPages }}</span>
          <button class="prod-page-btn" type="button" :disabled="page >= totalPages" aria-label="下一页" @click="page++"><ChevronRight :size="15" /></button>
        </div>
      </div>
        </div>
      </div>

      <!-- type picker -->
      <Teleport to="body">
        <Transition name="modal">
          <div v-if="typePickerOpen" class="prod-overlay" @click.self="typePickerOpen = false">
            <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" aria-label="选择设备类型">
              <div class="prod-modal-head">
                <h2>选择设备类型</h2>
                <button class="prod-icon" type="button" aria-label="关闭" @click="typePickerOpen = false"><X :size="16" /></button>
              </div>
              <div class="prod-type-grid">
                <button v-for="t in DEVICE_TYPES" :key="t.code" class="prod-type-card" type="button" @click="chooseType(t.code)">
                  <span class="prod-type-card-icon"><component :is="typeIcon(t.code)" :size="18" /></span>
                  <strong>{{ t.label }}</strong>
                  <span class="mono">类型码 {{ t.code }}</span>
                </button>
              </div>
            </div>
          </div>
        </Transition>
      </Teleport>

      <!-- create modal -->
      <Teleport to="body">
        <Transition name="modal">
          <div v-if="createOpen" class="prod-overlay" @click.self="closeCreate">
            <div class="prod-modal" role="dialog" aria-modal="true" aria-label="新增设备">
              <div class="prod-modal-head">
                <h2>新增设备 <span class="prod-type-badge">{{ deviceTypeLabel(createForm.device_type) }}</span></h2>
                <button class="prod-icon" type="button" aria-label="关闭" @click="closeCreate"><X :size="16" /></button>
              </div>
              <form class="prod-form" @submit.prevent="submitCreate">
                <div class="prod-form-grid">
                  <div class="prod-field prod-field-full">
                    <label for="prod-create-org">归属组织</label>
                    <select id="prod-create-org" v-model="createForm.org_unit_id" class="prod-select prod-select-full">
                      <option value="">未分配（稍后归位）</option>
                      <option v-for="f in flatOrgTree" :key="f.org.id" :value="f.org.id">{{ f.path }}</option>
                    </select>
                    <span class="prod-meta-hint">不选则设备创建后暂不归属任何组织，仅管理员可见。</span>
                  </div>
                  <div class="prod-field">
                    <label for="prod-create-name">设备名称</label>
                    <input id="prod-create-name" v-model="createForm.device_name" required maxlength="255" placeholder="如：东门 1 号摄像机" />
                  </div>
                  <div class="prod-field">
                    <label for="prod-create-manufacturer">厂商</label>
                    <div class="prod-manufacturer" :class="{ open: manufacturerOpen }">
                      <div class="prod-input-action">
                        <input id="prod-create-manufacturer" v-model="createForm.manufacturer" required maxlength="255" placeholder="选择或输入厂商" @focus="manufacturerOpen = true" />
                        <button class="prod-icon" type="button" aria-label="选择厂商" title="选择厂商" @click="toggleManufacturerList"><ListPlus :size="16" /></button>
                      </div>
                      <div v-if="manufacturerOpen" class="prod-manufacturer-menu" role="listbox">
                        <template v-if="!addingManufacturer">
                          <button v-for="m in filteredManufacturers" :key="m" class="prod-manufacturer-option" type="button" role="option" @click="pickManufacturer(m)">{{ m }}</button>
                          <button class="prod-manufacturer-add" type="button" @click="startAddManufacturer">+ 新增厂商</button>
                        </template>
                        <template v-else>
                          <input v-model="newManufacturer" class="prod-manufacturer-new" placeholder="输入新厂商名称" @keyup.enter="confirmAddManufacturer" />
                          <button class="prod-manufacturer-add" type="button" :disabled="!canAddManufacturer" @click="confirmAddManufacturer">确认新增</button>
                        </template>
                      </div>
                    </div>
                  </div>
                  <div class="prod-field">
                    <label for="prod-create-center">中心编码（8 位）</label>
                    <input id="prod-create-center" v-model="createForm.center_code" inputmode="numeric" pattern="[0-9]{8}" required maxlength="8" placeholder="34020000" />
                  </div>
                  <div class="prod-field">
                    <label for="prod-create-realm">SIP Realm</label>
                    <input id="prod-create-realm" v-model="createForm.sip_realm" required placeholder="3402000000" />
                  </div>
                  <div class="prod-field">
                    <label for="prod-create-password">密码</label>
                    <input id="prod-create-password" v-model="createForm.password" type="password" autocomplete="new-password" required placeholder="仅用于派生认证摘要" />
                  </div>
                  <div class="prod-field">
                    <label class="prod-checkbox">
                      <input v-model="createForm.enabled" type="checkbox" />
                      创建后立即启用
                    </label>
                  </div>
                </div>
                <p v-if="accessIDPreview" class="prod-code-preview mono"><ShieldCheck :size="14" />编码预览：{{ accessIDPreview }} · 序号由系统分配</p>
                <p v-if="createError" class="prod-error" role="alert">{{ createError }}</p>
                <div class="prod-modal-actions">
                  <button class="prod-button" type="button" @click="closeCreate">取消</button>
                  <button class="prod-button prod-button-primary" type="submit" :disabled="creating">
                    <Plus :size="16" />{{ creating ? '创建中…' : '创建' }}
                  </button>
                </div>
              </form>
            </div>
          </div>
        </Transition>
      </Teleport>

      <!-- edit modal -->
      <Teleport to="body">
        <Transition name="modal">
          <div v-if="editingDevice" class="prod-overlay" @click.self="closeEdit">
            <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" aria-label="编辑设备">
              <div class="prod-modal-head">
                <h2>编辑设备 <span class="prod-type-badge">{{ deviceTypeLabel(editingDevice.device_type) }}</span></h2>
                <button class="prod-icon" type="button" aria-label="关闭" @click="closeEdit"><X :size="16" /></button>
              </div>
              <form class="prod-form" @submit.prevent="submitEdit">
                <div class="prod-form-grid">
                  <div class="prod-field prod-field-full">
                    <label for="prod-edit-name">设备名称</label>
                    <input id="prod-edit-name" v-model="editForm.device_name" required maxlength="255" />
                  </div>
                  <div class="prod-field prod-field-full">
                    <label for="prod-edit-manufacturer">厂商</label>
                    <input id="prod-edit-manufacturer" v-model="editForm.manufacturer" required maxlength="255" />
                  </div>
                  <div class="prod-field prod-field-full">
                    <label for="prod-edit-org">归属组织</label>
                    <select id="prod-edit-org" v-model="editForm.org_unit_id" class="prod-select prod-select-full">
                      <option value="">未分配（仅管理员可见）</option>
                      <option v-for="f in flatOrgTree" :key="f.org.id" :value="f.org.id">{{ f.path }}</option>
                    </select>
                    <span class="prod-meta-hint">变更归属后，设备按新组织进入对应可见范围。</span>
                  </div>
                  <p class="prod-field-full prod-meta-hint">接入 ID、类型与 SIP Realm 不可修改。</p>
                </div>
                <p v-if="editError" class="prod-error" role="alert">{{ editError }}</p>
                <div class="prod-modal-actions">
                  <button class="prod-button" type="button" @click="closeEdit">取消</button>
                  <button class="prod-button prod-button-primary" type="submit" :disabled="savingEdit">
                    <Check :size="16" />{{ savingEdit ? '保存中…' : '保存' }}
                  </button>
                </div>
              </form>
            </div>
          </div>
        </Transition>
      </Teleport>

      <!-- detail drawer -->
      <Teleport to="body">
        <Transition name="drawer">
          <div v-if="detailDevice" class="prod-overlay prod-drawer-overlay" @click.self="closeDetail">
            <aside class="prod-drawer" role="dialog" aria-modal="true" aria-label="设备详情">
              <div class="prod-drawer-head">
                <div class="prod-drawer-title">
                  <span class="prod-drawer-icon"><component :is="typeIcon(detailDevice.device_type)" :size="20" /></span>
                  <div>
                    <h2>{{ detailDevice.device_name || '未命名设备' }}</h2>
                    <span class="prod-type-badge">{{ deviceTypeLabel(detailDevice.device_type) }}</span>
                  </div>
                </div>
                <button class="prod-icon" type="button" aria-label="关闭" @click="closeDetail"><X :size="16" /></button>
              </div>
              <dl class="prod-detail-list">
                <div><dt>接入 ID</dt><dd class="mono">{{ detailDevice.device_access_id }}</dd></div>
                <div><dt>设备名称</dt><dd>{{ detailDevice.device_name || '—' }}</dd></div>
                <div><dt>类型</dt><dd>{{ deviceTypeLabel(detailDevice.device_type) }}</dd></div>
                <div><dt>厂商</dt><dd>{{ detailDevice.manufacturer || '—' }}</dd></div>
                <div><dt>SIP Realm</dt><dd class="mono">{{ detailDevice.sip_realm }}</dd></div>
                <div><dt>认证算法</dt><dd>{{ detailDevice.digest_algorithm }}</dd></div>
                <div><dt>启用状态</dt><dd>{{ detailDevice.enabled ? '启用' : '停用' }}</dd></div>
                <div><dt>运行时状态</dt><dd>
                  <span class="prod-pill" :class="runtimeState(detailDevice) === 'online' ? 'pill-runtime-on' : 'pill-runtime-off'">
                    {{ runtimeLabel(detailDevice) }}
                  </span>
                </dd></div>
                <div><dt>目录同步</dt><dd>{{ catalogState(detailDevice) === 'ok' ? `✓ ${catalogOkTime(detailDevice)} · ${detailDevice.catalog_last_count ?? '—'} 路` : catalogState(detailDevice) === 'in_progress' ? '同步中' : catalogState(detailDevice) === 'failed' ? '失败' : '从未同步' }}</dd></div>
                <div v-if="catalogState(detailDevice) === 'failed' && detailDevice.catalog_last_error"><dt>同步错误</dt><dd>{{ detailDevice.catalog_last_error }}</dd></div>
                <div><dt>配置同步</dt><dd>{{ syncLabel(detailDevice) }} · 版本 <span class="mono">{{ detailDevice.access_synced_version ?? '—' }}</span></dd></div>
                <div v-if="detailDevice.runtime?.last_seen"><dt>最近活跃</dt><dd class="mono">{{ detailDevice.runtime.last_seen }}</dd></div>
                <div v-if="detailDevice.runtime?.remote_address"><dt>远端地址</dt><dd class="mono">{{ detailDevice.runtime.remote_address }}</dd></div>
                <div><dt>创建时间</dt><dd class="mono">{{ detailDevice.created_at }}</dd></div>
                <div><dt>更新时间</dt><dd class="mono">{{ detailDevice.updated_at }}</dd></div>
              </dl>
              <div class="prod-drawer-actions">
                <button class="prod-button prod-button-primary" type="button" @click="openEdit(detailDevice); closeDetail()">
                  <Edit3 :size="15" />编辑
                </button>
              </div>
            </aside>
          </div>
        </Transition>
      </Teleport>
    </main>

    <footer class="prod-footer">
      <span class="prod-footer-deps">
        <Zap :size="13" />node-app · {{ devices.length }} 台设备
      </span>
    </footer>
  </div>
</template>

<style scoped>
/* 颜色一律引用 main.css 设计令牌（design-system spec）；顶栏深色铬合金为既有设计，双模式下保持深色。 */
.prod-shell { min-height: 100vh; background: var(--bg-page); color: var(--text-primary); font-family: var(--font-sans); }
/* ---------- topbar（深色铬合金，双模式一致） ---------- */
.prod-topbar { display: flex; align-items: center; gap: 30px; padding: 0 32px; height: 62px; background: #11161c; color: #e8ebee; border-bottom: 1px solid #1f2730; position: sticky; top: 0; z-index: 30; }
.prod-brand { display: flex; align-items: center; gap: 11px; }
.prod-logo { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; color: #fff; background: linear-gradient(135deg, #2d3742, #1c242d); border: 1px solid #333f4b; border-radius: 9px; }
.prod-brand-text { display: grid; gap: 1px; }
.prod-brand-text strong { font-size: 14px; letter-spacing: .01em; }
.prod-brand-text span { font-size: 11px; color: #8b98a5; }
.prod-nav { display: flex; gap: 4px; margin-left: 6px; }
.prod-nav-link { padding: 8px 13px; color: #9aa7b3; font-size: 13.5px; font-weight: 600; text-decoration: none; border-radius: 7px; transition: color .15s, background .15s; }
.prod-nav-link:hover { color: #fff; background: rgba(255,255,255,.07); }
.prod-nav-link.active { color: #fff; background: rgba(255,255,255,.11); }
.prod-topbar-right { margin-left: auto; display: flex; align-items: center; gap: 12px; }
.prod-health { display: inline-flex; align-items: center; gap: 7px; font-size: 12.5px; font-weight: 600; color: #9aa7b3; }
.prod-health-dot { width: 7px; height: 7px; border-radius: 50%; }
.health-up .prod-health-dot { background: #34d399; box-shadow: 0 0 0 3px rgba(52,211,153,.15); }
.health-warn .prod-health-dot { background: #fbbf24; box-shadow: 0 0 0 3px rgba(251,191,36,.15); }
.health-down .prod-health-dot { background: #f87171; box-shadow: 0 0 0 3px rgba(248,113,113,.15); }
.prod-health-sep { width: 1px; height: 18px; background: #2a333d; }
.prod-deps { display: inline-flex; align-items: center; gap: 6px; color: #7d8a96; }
.dep-dot { width: 6px; height: 6px; border-radius: 50%; background: #6b7a87; }
.dep-up { background: #34d399; }
.dep-down { background: #f87171; }
.dep-unknown { background: #6b7a87; }
/* ---------- main ---------- */
.prod-main { max-width: 1200px; margin: 0 auto; padding: 0 32px 64px; }

/* ---------- glass sticky title bar ---------- */
.prod-head { position: sticky; top: 62px; z-index: 10; background: color-mix(in srgb, var(--bg-card) 85%, transparent); -webkit-backdrop-filter: blur(20px); backdrop-filter: blur(20px); border-bottom: 1px solid var(--border-faint); }
.prod-head-inner { max-width: 1200px; margin: 0 auto; padding: 20px 32px; display: flex; justify-content: space-between; align-items: flex-start; gap: 18px; }
.prod-head h1 { margin: 0; font-size: 28px; font-weight: 700; color: var(--text-primary); letter-spacing: -0.01em; }
/* ---------- toast ---------- */
.prod-flash { display: inline-flex; align-items: center; gap: 8px; margin-top: 18px; padding: 10px 15px; color: var(--success); background: var(--success-bg); border: 1px solid var(--success-border); border-radius: 9px; font-size: 13px; font-weight: 600; box-shadow: 0 4px 14px rgba(29,113,78,.08); }
.toast-enter-active, .toast-leave-active { transition: opacity .2s, transform .2s; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(-6px); }
/* ---------- metrics ---------- */
.prod-stats { display: grid; grid-template-columns: repeat(5, 1fr); gap: 16px; margin-bottom: 28px; }
.prod-stat { display: flex; flex-direction: column; gap: 12px; padding: 20px 24px; background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); box-shadow: var(--shadow-card); }
.prod-stat-label { display: inline-flex; align-items: center; gap: 8px; color: var(--text-tertiary); font-size: 13px; font-weight: 500; }
.stat-dot { width: 8px; height: 8px; border-radius: 50%; }
.dot-online { background: var(--accent-green); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-green) 15%, transparent); }
.dot-offline { background: var(--text-muted); }
.dot-sync { background: var(--accent-green); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-green) 15%, transparent); }
.dot-pending { background: var(--accent-yellow); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-yellow) 18%, transparent); }
.prod-stat-value { font-size: 32px; font-weight: 700; letter-spacing: -1px; line-height: 1; color: var(--text-primary); font-variant-numeric: tabular-nums; }

/* ---------- search & filter card ---------- */
.prod-toolbar-card { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; margin-bottom: 16px; padding: 16px 20px; background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); box-shadow: var(--shadow-card); }
.prod-search { position: relative; flex: 1; min-width: 240px; }
.prod-search-icon { position: absolute; left: 14px; top: 50%; transform: translateY(-50%); color: var(--text-muted); pointer-events: none; }
.prod-search input { width: 100%; padding: 10px 14px 10px 40px; border: 1.5px solid var(--border-input); border-radius: 12px; font-size: 15px; color: var(--text-primary); background: var(--bg-input); outline: none; box-sizing: border-box; letter-spacing: -0.2px; font-family: inherit; transition: border-color .15s, background .15s, box-shadow .15s; }
.prod-search input::placeholder { color: var(--text-muted); }
.prod-search input:focus { border-color: var(--border-hover); background: var(--bg-card); box-shadow: 0 0 0 3px var(--focus-ring); }
.prod-search-clear { position: absolute; right: 10px; top: 50%; transform: translateY(-50%); display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; color: var(--text-tertiary); background: none; border: 0; border-radius: 999px; cursor: pointer; }
.prod-search-clear:hover { color: var(--text-primary); background: var(--bg-hover); }
.prod-select { height: 40px; padding: 0 14px; font: inherit; font-size: 13px; font-weight: 500; color: var(--text-secondary); background: var(--bg-card); border: 1.5px solid var(--border-input); border-radius: 12px; cursor: pointer; transition: border-color .15s, box-shadow .15s; }
.prod-select:focus-visible { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px var(--focus-ring); }
.prod-pills { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.prod-pill { padding: 8px 16px; border: 1px solid var(--border-input); border-radius: 999px; font-size: 13px; font-weight: 500; letter-spacing: -0.2px; color: var(--text-secondary); background: var(--bg-subtle); cursor: pointer; font-family: inherit; transition: background .15s, color .15s, border-color .15s; }
.prod-pill:hover { border-color: var(--border-hover); }
.prod-pill.active { background: var(--btn-primary-bg); color: var(--bg-card); border-color: var(--btn-primary-bg); font-weight: 600; }
.prod-pill:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.prod-refresh { display: inline-flex; align-items: center; justify-content: center; width: 36px; height: 36px; flex-shrink: 0; color: var(--text-secondary); background: var(--bg-card); border: 1.5px solid var(--border-input); border-radius: var(--r-sm); cursor: pointer; transition: background .15s, border-color .15s, color .15s; }
.prod-refresh:hover:not(:disabled) { background: var(--bg-hover); border-color: var(--border-hover); }
.prod-refresh:disabled { cursor: wait; opacity: .5; }
.prod-refresh:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
/* ---------- buttons ---------- */
.prod-button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; padding: 10px 20px; color: var(--text-primary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: 999px; font-size: 14px; font-weight: 600; cursor: pointer; transition: border-color .15s, background .15s, transform .05s; }
.prod-button:hover:not(:disabled) { border-color: var(--border-hover); }
.prod-button:active:not(:disabled) { transform: scale(.985); }
.prod-button:disabled { cursor: wait; opacity: .6; }
.prod-button-primary { color: var(--bg-card); background: var(--btn-primary-bg); border-color: var(--btn-primary-bg); box-shadow: var(--shadow-button); }
.prod-button-primary:hover:not(:disabled) { background: var(--btn-primary-hover); border-color: var(--btn-primary-hover); }
.prod-button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.prod-icon { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; color: var(--text-secondary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: 9px; cursor: pointer; transition: color .15s, border-color .15s, background .15s; }
.prod-icon:hover:not(:disabled) { color: var(--text-primary); border-color: var(--border-hover); }
.prod-icon.danger:hover:not(:disabled) { color: var(--danger); border-color: var(--danger-border); background: var(--danger-bg); }
.prod-icon:disabled { cursor: not-allowed; opacity: .45; }
/* ---------- table ---------- */
.prod-table-card { margin-bottom: 16px; overflow-x: auto; background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); box-shadow: var(--shadow-card); }
.prod-thead, .prod-trow { display: grid; grid-template-columns: 2fr 2fr 1.2fr 1.7fr 1.9fr 178px; align-items: center; min-width: 980px; }
.prod-table-card.with-tenant .prod-thead, .prod-table-card.with-tenant .prod-trow { grid-template-columns: 2fr 2fr 1.1fr 1.1fr 1.5fr 1.9fr 178px; min-width: 1100px; }
.prod-thead { padding: 14px 24px; background: var(--bg-table-header); border-bottom: 1px solid var(--border-faint); font-size: 12px; font-weight: 600; color: var(--text-tertiary); text-transform: uppercase; letter-spacing: 0.5px; }
.prod-thead .th-actions { text-align: right; }
.prod-trow { padding: 16px 24px; border-bottom: 1px solid var(--border-faint); background: var(--bg-card); transition: background 0.15s; }
.prod-trow:last-child { border-bottom: 0; }
.prod-trow:hover { background: var(--bg-hover); }
.prod-name-cell { min-width: 0; }
.prod-name-line { display: flex; align-items: center; gap: 8px; min-width: 0; }
.prod-runtime-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.prod-runtime-dot.dot-on { background: var(--accent-green); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-green) 15%, transparent); }
.prod-runtime-dot.dot-off { background: var(--text-muted); }
.prod-name { font-weight: 600; color: var(--text-primary); max-width: 170px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-type-tag { flex-shrink: 0; padding: 2px 7px; border-radius: 999px; background: var(--bg-subtle); color: var(--text-tertiary); font-size: 11px; font-weight: 600; white-space: nowrap; }
.prod-disabled-tag { flex-shrink: 0; padding: 2px 7px; border-radius: 999px; background: var(--danger-bg); color: var(--danger); font-size: 11px; font-weight: 600; white-space: nowrap; }
.prod-name-sub { margin-top: 3px; color: var(--text-tertiary); font-size: 11px; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-id-cell { color: var(--text-secondary); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.prod-tenant-cell { color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
/* 通道点阵（D51） */
.prod-channel-cell { display: flex; align-items: center; gap: 8px; min-width: 0; }
.prod-channel-cell.dead { opacity: .55; }
.prod-dots { display: inline-flex; align-items: center; gap: 3px; flex-shrink: 0; }
.prod-dot { width: 7px; height: 7px; border-radius: 50%; display: inline-block; }
.prod-dot.on { background: var(--accent-green); }
.prod-dot.off { background: var(--text-muted); }
.prod-dot.miss { background: transparent; border: 1.5px solid var(--accent-red); box-sizing: border-box; }
.prod-dot-overflow { color: var(--text-tertiary); font-size: 10px; }
.prod-channel-count { color: var(--text-tertiary); font-size: 12.5px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.prod-channel-count.dead { color: var(--text-tertiary); opacity: .8; }
.prod-channel-none { color: var(--text-muted); font-size: 13px; }
/* 目录同步四态（D51）：数字段 tabular-nums 固定宽度防抖动 */
.prod-catalog { display: inline-flex; align-items: center; gap: 8px; white-space: nowrap; font-size: 13px; font-variant-numeric: tabular-nums; }
.prod-catalog.ok { color: var(--text-tertiary); }
.prod-catalog.ok svg { color: var(--accent-green); }
.prod-catalog.ing { color: var(--text-primary); }
.prod-catalog.bad { color: var(--danger); }
.prod-catalog.never { color: var(--text-muted); font-size: 13px; }
.prod-catalog-error { max-width: 220px; overflow: hidden; text-overflow: ellipsis; }
.prod-pill { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; white-space: nowrap; }
.pill-dot { width: 6px; height: 6px; border-radius: 50%; }
.pill-enable { color: var(--success); background: var(--success-bg); }
.pill-muted { color: var(--text-secondary); background: var(--bg-subtle); }
.pill-synced { color: var(--success); background: var(--success-bg); }
.pill-pending { color: var(--warning); background: var(--warning-bg); }
.pill-failed { color: var(--danger); background: var(--danger-bg); }
.pill-runtime-on { color: var(--success); background: var(--success-bg); }
.pill-runtime-off { color: var(--text-secondary); background: var(--bg-subtle); }
.td-actions { text-align: right; }
.prod-actions { display: inline-flex; gap: 4px; opacity: 0; transition: opacity .15s; }
.prod-trow:hover .prod-actions, .prod-actions:focus-within { opacity: 1; }
.prod-act { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; color: var(--text-tertiary); background: transparent; border: 0; border-radius: 8px; cursor: pointer; transition: all 0.15s; }
.prod-act:hover:not(:disabled) { background: var(--bg-hover); color: var(--text-primary); }
.prod-act.danger:hover:not(:disabled) { background: var(--danger-bg); color: var(--accent-red); }
.prod-act:disabled { cursor: not-allowed; opacity: .4; }
.prod-act:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
.prod-act.catalog-refresh { color: var(--text-secondary); }
/* ---------- skeleton ---------- */
.sk-row { padding: 16px 24px; }
.sk { display: inline-block; background: linear-gradient(90deg, var(--bg-subtle) 25%, var(--bg-hover) 37%, var(--bg-subtle) 63%); background-size: 400% 100%; animation: sk-shimmer 1.3s ease infinite; border-radius: 6px; }
.sk-name { width: 120px; height: 14px; }
.sk-id { width: 150px; height: 12px; }
.sk-pill { width: 56px; height: 20px; border-radius: 999px; }
.sk-dots { width: 90px; height: 14px; }
.sk-actions { width: 130px; height: 28px; }
@keyframes sk-shimmer { 0% { background-position: 100% 0; } 100% { background-position: -100% 0; } }
/* ---------- error ---------- */
.prod-error { display: flex; align-items: center; gap: 13px; margin-bottom: 16px; padding: 15px 18px; color: var(--danger); background: var(--danger-bg); border: 1px solid var(--danger-border); border-radius: var(--r-xl); }
.prod-error svg { flex-shrink: 0; }
.prod-error strong { font-size: 13.5px; }
.prod-error p { margin: 3px 0 0; color: var(--danger); opacity: .8; font-size: 12.5px; }
.prod-error .prod-button { margin-left: auto; color: var(--danger); border-color: var(--danger-border); }
.prod-form .prod-error { margin-top: 14px; }
/* ---------- mobile card list ---------- */
.prod-card-list { display: flex; flex-direction: column; gap: 12px; }
.prod-card-item { display: flex; flex-direction: column; gap: 8px; padding: 16px; background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); box-shadow: var(--shadow-card); }
.prod-card-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.prod-card-name { font-size: 15px; font-weight: 600; color: var(--text-primary); display: inline-flex; align-items: center; gap: 8px; }
.prod-card-sub { font-size: 12px; color: var(--text-tertiary); }
.prod-card-id { font-size: 12px; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-card-status { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.prod-card-actions { display: flex; gap: 6px; margin-top: 2px; }
.prod-card-actions .prod-act { width: 44px; height: 44px; }
/* ---------- empty ---------- */
.prod-empty { display: flex; flex-direction: column; align-items: center; gap: 6px; margin-bottom: 16px; padding: 52px 20px; text-align: center; background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); box-shadow: var(--shadow-card); }
.prod-empty-icon { display: inline-flex; align-items: center; justify-content: center; width: 52px; height: 52px; margin-bottom: 6px; color: var(--text-tertiary); background: var(--bg-subtle); border-radius: 999px; }
.prod-empty strong { font-size: 14.5px; color: var(--text-primary); }
.prod-empty p { margin: 0 0 10px; color: var(--text-tertiary); font-size: 13px; }
/* ---------- pagination ---------- */
.prod-pagination { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 14px; padding: 0 4px; }
.prod-page-count { color: var(--text-tertiary); font-size: 12px; }
.prod-page-btns { display: flex; align-items: center; gap: 8px; }
.prod-page-info { color: var(--text-secondary); font-size: 12px; }
.prod-page-btn { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; color: var(--text-tertiary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: 8px; cursor: pointer; transition: background .15s, border-color .15s, color .15s; }
.prod-page-btn:hover:not(:disabled) { background: var(--bg-hover); color: var(--text-primary); }
.prod-page-btn:disabled { cursor: not-allowed; opacity: .45; }
.prod-page-btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
/* ---------- overlay / modal / drawer ---------- */
.prod-overlay { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(0,0,0,.4); backdrop-filter: blur(4px); -webkit-backdrop-filter: blur(4px); }
.prod-modal { width: 100%; max-width: 560px; max-height: 88vh; overflow-y: auto; background: var(--bg-card); border-radius: var(--r-xl); box-shadow: var(--shadow-xl); }
.prod-modal-narrow { max-width: 480px; }
.prod-modal-head { display: flex; align-items: center; justify-content: space-between; padding: 19px 22px; border-bottom: 1px solid var(--border-faint); }
.prod-modal-head h2 { margin: 0; font-size: 16.5px; display: flex; align-items: center; gap: 10px; }
.prod-type-badge { display: inline-block; padding: 3px 9px; color: var(--text-primary); background: var(--bg-subtle); border-radius: 999px; font-size: 11.5px; font-weight: 700; }
.prod-form { padding: 20px 22px; }
.prod-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.prod-field { display: grid; gap: 6px; }
.prod-field-full { grid-column: 1 / -1; }
.prod-field label { font-size: 12px; font-weight: 700; color: var(--text-secondary); }
.prod-field input { width: 100%; padding: 10px 11px; font: inherit; font-size: 13.5px; color: var(--text-primary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: 8px; box-sizing: border-box; transition: border-color .15s, box-shadow .15s; }
.prod-field input:focus-visible { outline: none; border-color: var(--text-primary); box-shadow: 0 0 0 3px var(--focus-ring); }
.prod-select-full { width: 100%; }
.prod-select-full:focus-visible { outline: none; border-color: var(--text-primary); box-shadow: 0 0 0 3px var(--focus-ring); }
.prod-checkbox { display: inline-flex; align-items: center; gap: 8px; font-size: 13px !important; font-weight: 600 !important; color: var(--text-primary) !important; padding-top: 22px; cursor: pointer; }
.prod-input-action { display: flex; gap: 6px; }
.prod-input-action input { flex: 1; }
.prod-manufacturer { position: relative; }
.prod-manufacturer-menu { position: absolute; z-index: 60; top: calc(100% + 4px); left: 0; right: 0; max-height: 220px; overflow-y: auto; padding: 6px; background: var(--bg-card); border: 1px solid var(--border-input); border-radius: 9px; box-shadow: var(--shadow-lg); }
.prod-manufacturer-option { display: block; width: 100%; padding: 9px 10px; text-align: left; color: var(--text-primary); background: none; border: 0; border-radius: 6px; font-size: 13px; cursor: pointer; }
.prod-manufacturer-option:hover { background: var(--bg-hover); }
.prod-manufacturer-add { display: block; width: 100%; padding: 9px 10px; margin-top: 4px; text-align: left; color: var(--text-primary); background: none; border: 0; border-top: 1px solid var(--border-faint); border-radius: 0; font-size: 13px; font-weight: 600; cursor: pointer; }
.prod-manufacturer-add:hover { color: var(--accent); }
.prod-manufacturer-add:disabled { cursor: not-allowed; opacity: .5; }
.prod-manufacturer-new { width: 100%; padding: 8px 10px; font: inherit; font-size: 13px; color: var(--text-primary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: 6px; box-sizing: border-box; }
.prod-code-preview { display: flex; align-items: center; gap: 8px; margin: 14px 0 0; padding: 10px 12px; color: var(--text-secondary); background: var(--bg-subtle); border-radius: 8px; }
.prod-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px; }
.prod-meta-hint { margin: 0; color: var(--text-tertiary); font-size: 12px; }
.prod-type-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding: 20px 22px; }
.prod-type-card { display: grid; gap: 5px; padding: 17px 16px; text-align: left; color: var(--text-primary); background: var(--bg-subtle); border: 1px solid var(--border-input); border-radius: var(--r-sm); cursor: pointer; transition: border-color .15s, background .15s, transform .05s; }
.prod-type-card:hover { border-color: var(--text-primary); background: var(--bg-card); }
.prod-type-card:active { transform: translateY(1px); }
.prod-type-card-icon { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; margin-bottom: 4px; color: var(--text-primary); background: var(--bg-card); border: 1px solid var(--border-input); border-radius: 8px; }
.prod-type-card strong { font-size: 14.5px; }
.prod-type-card .mono { color: var(--text-tertiary); }
.prod-drawer-overlay { justify-content: flex-end; padding: 0; }
.prod-drawer { width: 100%; max-width: 440px; height: 100vh; overflow-y: auto; background: var(--bg-card); box-shadow: -24px 0 64px rgba(0,0,0,.18); border-radius: 20px 0 0 20px; }
.prod-drawer-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 22px 24px; border-bottom: 1px solid var(--border-faint); }
.prod-drawer-title { display: flex; align-items: center; gap: 12px; }
.prod-drawer-icon { display: inline-flex; align-items: center; justify-content: center; width: 42px; height: 42px; color: var(--text-primary); background: var(--bg-subtle); border: 1px solid var(--border-input); border-radius: var(--r-sm); }
.prod-drawer-title h2 { margin: 0 0 5px; font-size: 17.5px; }
.prod-detail-list { margin: 0; padding: 8px 24px 16px; }
.prod-detail-list div { display: flex; justify-content: space-between; gap: 16px; padding: 11.5px 0; border-bottom: 1px solid var(--border-faint); font-size: 13px; }
.prod-detail-list dt { color: var(--text-tertiary); flex-shrink: 0; }
.prod-detail-list dd { margin: 0; color: var(--text-primary); text-align: right; word-break: break-all; }
.prod-drawer-actions { padding: 16px 24px 24px; }
/* ---------- modal & drawer transitions ---------- */
.modal-enter-active, .modal-leave-active { transition: opacity .18s; }
.modal-enter-active .prod-modal, .modal-leave-active .prod-modal { transition: transform .18s cubic-bezier(.2,.8,.3,1), opacity .18s; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-from .prod-modal, .modal-leave-to .prod-modal { transform: translateY(14px) scale(.98); opacity: 0; }
.drawer-enter-active, .drawer-leave-active { transition: opacity .2s; }
.drawer-enter-active .prod-drawer, .drawer-leave-active .prod-drawer { transition: transform .24s cubic-bezier(.2,.8,.3,1); }
.drawer-enter-from, .drawer-leave-to { opacity: 0; }
.drawer-enter-from .prod-drawer, .drawer-leave-to .prod-drawer { transform: translateX(100%); }
/* ---------- footer ---------- */
.prod-footer { max-width: 1240px; margin: 0 auto; padding: 20px 32px 32px; display: flex; align-items: center; justify-content: space-between; color: var(--text-tertiary); font-size: 12px; }
.prod-footer-deps { display: inline-flex; align-items: center; gap: 6px; }
.mono { font-family: var(--font-mono); font-size: 12px; font-variant-numeric: tabular-nums; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* ---------- folder-style org sidebar ---------- */
.prod-fold { display: grid; grid-template-columns: 230px 1fr; gap: 16px; align-items: start; margin-bottom: 16px; }
.prod-fold-tree { background: var(--bg-card); border: 1px solid var(--border-card); border-radius: var(--r-xl); box-shadow: var(--shadow-card); overflow: hidden; position: sticky; top: 132px; max-height: calc(100vh - 160px); display: flex; flex-direction: column; }
.prod-fold-tree-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 12px 14px; border-bottom: 1px solid var(--border-faint); }
.prod-fold-tree-title { display: inline-flex; align-items: center; gap: 7px; color: var(--text-tertiary); font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.4px; }
.prod-fold-tree-clear { display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; color: var(--text-tertiary); background: transparent; border: 0; border-radius: 6px; cursor: pointer; }
.prod-fold-tree-clear:hover:not(:disabled) { background: var(--bg-hover); color: var(--text-primary); }
.prod-fold-tree-clear:disabled { opacity: .35; cursor: default; }
.prod-fold-tree-body { overflow-y: auto; padding: 6px; }
.prod-fold-node { display: flex; align-items: center; gap: 7px; width: 100%; padding: 8px 10px; color: var(--text-secondary); background: transparent; border: 0; border-radius: 8px; font: inherit; font-size: 13px; text-align: left; cursor: pointer; box-sizing: border-box; transition: background .15s, color .15s; }
.prod-fold-node:hover { background: var(--bg-hover); }
.prod-fold-node.active { background: var(--btn-primary-bg); color: var(--bg-card); }
.prod-fold-node-icon { color: var(--text-muted); flex-shrink: 0; }
.prod-fold-node.active .prod-fold-node-icon { color: var(--bg-card); }
.prod-fold-node-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-fold-node-count { color: var(--text-tertiary); font-size: 11px; font-variant-numeric: tabular-nums; }
.prod-fold-node.active .prod-fold-node-count { color: color-mix(in srgb, var(--bg-card) 70%, transparent); }
.prod-fold-node-empty { color: var(--text-tertiary); cursor: default; }
.prod-fold-node:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
@media (max-width: 900px) { .prod-fold { grid-template-columns: 1fr; } .prod-fold-tree { position: static; max-height: 260px; } }
/* ---------- reduced motion ---------- */
@media (prefers-reduced-motion: reduce) {
  .sk, .spinning { animation: none; }
  .prod-trow, .prod-button, .prod-act, .prod-icon, .prod-nav-link, .prod-type-card, .prod-search input, .prod-select, .prod-pill, .prod-refresh, .prod-page-btn, .prod-field input, .prod-fold-node { transition: none; }
  .modal-enter-active, .modal-leave-active, .drawer-enter-active, .drawer-leave-active, .toast-enter-active, .toast-leave-active { transition: none; }
  .modal-enter-from, .modal-leave-to, .drawer-enter-from, .drawer-leave-to, .toast-enter-from, .toast-leave-to { opacity: 1; transform: none; }
}
/* ---------- responsive ---------- */
@media (max-width: 900px) {
  .prod-topbar { gap: 14px; padding: 0 16px; }
  .prod-brand-text span { display: none; }
  .prod-deps { display: none; }
  .prod-main { padding: 0 16px 48px; }
  .prod-head-inner { padding: 16px 16px; }
  .prod-stats { grid-template-columns: repeat(2, 1fr); gap: 12px; }
  .prod-toolbar-card { gap: 12px; }
  .prod-search { flex-basis: 100%; }
  .prod-table-card { overflow-x: auto; }
  .prod-thead, .prod-trow { min-width: 980px; }
}
@media (max-width: 640px) {
  .prod-head-inner { flex-direction: column; align-items: stretch; }
  .prod-head .prod-button-primary { align-self: flex-start; }
  /* 移动端：操作按钮常显 + 触控目标 ≥44px */
  .prod-act { opacity: 1 !important; width: 44px; height: 44px; }
  .prod-actions { opacity: 1; }
}
</style>
