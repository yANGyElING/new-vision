<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Activity, AlertTriangle, Check, ChevronLeft, ChevronRight, Cpu, Database,
  Edit3, Eye, HardDrive, ListPlus, Monitor, Pause, Play, Plus, Radio, RefreshCw,
  Search, Server, ShieldCheck, Trash2, WifiOff, X, Zap,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { fetchHealth, type HealthState } from '@/api/health'
import { me } from '@/api/auth'
import {
  listDevices, createDevice, setDeviceEnabled, updateDeviceMeta, deleteDevice,
  previewAccessID, deviceTypeLabel, DEVICE_TYPES, type Device,
} from '@/api/devices'

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

const filteredDevices = computed(() => {
  const q = search.value.trim().toLowerCase()
  let list = devices.value
  if (q) {
    list = list.filter((d) => d.device_access_id.toLowerCase().includes(q) || (d.device_name || '').toLowerCase().includes(q))
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
const regionScopes = ref<string[]>([])
const createForm = ref<{
  device_type: string; center_code: string; device_name: string; manufacturer: string
  sip_realm: string; password: string; enabled: boolean; region_id: string
}>({
  device_type: DEVICE_TYPES[0].code, center_code: '34020000', device_name: '', manufacturer: '',
  sip_realm: '3402000000', password: '', enabled: true, region_id: '',
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
  if (!createForm.value.region_id) {
    createError.value = '请选择区域'
    return
  }
  creating.value = true
  try {
    const device = await createDevice({
      region_id: createForm.value.region_id,
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
const editForm = ref({ device_name: '', manufacturer: '' })
const savingEdit = ref(false)
const editError = ref('')

function openEdit(device: Device) {
  editingDevice.value = device
  editForm.value = { device_name: device.device_name, manufacturer: device.manufacturer }
  editError.value = ''
}
function closeEdit() { editingDevice.value = null; editError.value = '' }
async function submitEdit() {
  if (!editingDevice.value) return
  editError.value = ''
  savingEdit.value = true
  try {
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

onMounted(() => {
  void loadDevices()
  void refreshHealth()
  void me().then((info) => {
    regionScopes.value = info.region_scopes ?? []
    nodeAdmin.value = (info.roles ?? []).includes('node_admin')
    if (regionScopes.value.length > 0) createForm.value.region_id = regionScopes.value[0]
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
        <RouterLink v-if="nodeAdmin" to="/identity" class="prod-nav-link">权限管理</RouterLink>
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

    <main class="prod-main">
      <div class="prod-page-head">
        <div class="prod-heading">
          <span class="prod-eyebrow">DEVICES / {{ stats.total }}</span>
          <h1>设备管理</h1>
          <p>管理接入节点的视频设备：新增、编辑、启停与删除。</p>
        </div>
        <button class="prod-button prod-button-primary" type="button" @click="openTypePicker">
          <Plus :size="16" :stroke-width="2.2" />新增设备
        </button>
      </div>

      <Transition name="toast">
        <div v-if="flash" class="prod-flash" role="status">
          <Check :size="15" />{{ flash }}
        </div>
      </Transition>

      <!-- metrics -->
      <div class="prod-stats" aria-label="设备统计">
        <div class="prod-stat">
          <span class="prod-stat-label"><Cpu :size="14" />总设备</span>
          <strong class="prod-stat-value">{{ stats.total }}</strong>
        </div>
        <div class="prod-stat">
          <span class="prod-stat-label"><span class="stat-dot dot-online" />在线</span>
          <strong class="prod-stat-value stat-online">{{ stats.online }}</strong>
        </div>
        <div class="prod-stat">
          <span class="prod-stat-label"><span class="stat-dot dot-offline" />离线</span>
          <strong class="prod-stat-value stat-offline">{{ stats.offline }}</strong>
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

      <!-- toolbar -->
      <div class="prod-toolbar">
        <div class="prod-search">
          <Search :size="15" class="prod-search-icon" />
          <input v-model="search" placeholder="搜索名称或接入 ID" @input="resetPage" aria-label="搜索设备" />
          <button v-if="search" class="prod-search-clear" type="button" aria-label="清空搜索" @click="search = ''; resetPage()"><X :size="13" /></button>
        </div>
        <select v-model="typeFilter" class="prod-select" @change="resetPage" aria-label="按类型筛选">
          <option value="">全部类型</option>
          <option v-for="t in DEVICE_TYPES" :key="t.code" :value="t.code">{{ t.label }}</option>
        </select>
        <select v-model="syncFilter" class="prod-select" @change="resetPage" aria-label="按同步状态筛选">
          <option value="">全部同步</option>
          <option value="synced">已同步</option>
          <option value="pending">同步中</option>
        </select>
        <select v-model="runtimeFilter" class="prod-select" @change="resetPage" aria-label="按运行状态筛选">
          <option value="">全部运行</option>
          <option value="online">在线</option>
          <option value="offline">离线</option>
        </select>
        <button class="prod-icon prod-refresh" type="button" :disabled="loading" aria-label="刷新设备列表" title="刷新" @click="loadDevices">
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
      <div v-else-if="loading" class="prod-table-wrap" aria-label="加载中">
        <table class="prod-table">
          <thead>
            <tr>
              <th scope="col">名称</th><th scope="col">接入 ID</th><th scope="col">类型</th>
              <th scope="col">厂商</th><th scope="col">状态</th><th scope="col">同步</th>
              <th scope="col">运行时</th><th scope="col" class="col-actions">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="n in 6" :key="n" class="skeleton-row">
              <td><span class="sk sk-name" /></td>
              <td><span class="sk sk-id" /></td>
              <td><span class="sk sk-pill" /></td>
              <td><span class="sk sk-name" /></td>
              <td><span class="sk sk-pill" /></td>
              <td><span class="sk sk-pill" /></td>
              <td><span class="sk sk-pill" /></td>
              <td><span class="sk sk-actions" /></td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- table -->
      <div v-else-if="filteredDevices.length > 0" class="prod-table-wrap">
        <table class="prod-table">
          <thead>
            <tr>
              <th scope="col">名称</th>
              <th scope="col">接入 ID</th>
              <th scope="col">类型</th>
              <th scope="col">厂商</th>
              <th scope="col">状态</th>
              <th scope="col">同步</th>
              <th scope="col">运行时</th>
              <th scope="col" class="col-actions">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="device in pageDevices" :key="device.id" class="prod-row" @dblclick="openDetail(device)">
              <td class="prod-name-cell">
                <div class="prod-name">{{ device.device_name || '未命名' }}</div>
                <div class="prod-name-sub mono">{{ device.manufacturer || '—' }}</div>
              </td>
              <td class="mono prod-id">{{ device.device_access_id }}</td>
              <td>
                <span class="prod-type">
                  <component :is="typeIcon(device.device_type)" :size="13" />
                  {{ deviceTypeLabel(device.device_type) }}
                </span>
              </td>
              <td class="prod-manufacturer">{{ device.manufacturer || '—' }}</td>
              <td>
                <span class="prod-pill" :class="device.enabled ? 'pill-enable' : 'pill-muted'">
                  <span class="pill-dot" :class="device.enabled ? 'dot-online' : 'dot-offline'" />
                  {{ device.enabled ? '启用' : '停用' }}
                </span>
              </td>
              <td>
                <span class="prod-pill" :class="device.access_sync_status === 'synced' ? 'pill-synced' : 'pill-pending'">
                  <span class="pill-dot" :class="device.access_sync_status === 'synced' ? 'dot-online' : 'dot-pending'" />
                  {{ syncLabel(device) }}
                </span>
              </td>
              <td>
                <span class="prod-pill" :class="runtimeState(device) === 'online' ? 'pill-runtime-on' : runtimeState(device) === 'offline' ? 'pill-runtime-off' : 'pill-muted'">
                  <span class="pill-dot" :class="runtimeState(device) === 'online' ? 'dot-online' : 'dot-offline'" />
                  {{ runtimeLabel(device) }}
                </span>
              </td>
              <td class="col-actions">
                <div class="prod-actions">
                  <button class="prod-icon" type="button" title="查看详情" aria-label="查看详情" @click="openDetail(device)"><Eye :size="15" /></button>
                  <button class="prod-icon" type="button" title="编辑" aria-label="编辑" @click="openEdit(device)"><Edit3 :size="15" /></button>
                  <button class="prod-icon" type="button" :disabled="busy[device.id] === 'toggle'" :title="device.enabled ? '停用' : '启用'" :aria-label="device.enabled ? '停用' : '启用'" @click="toggleEnabled(device)">
                    <Pause v-if="device.enabled" :size="15" /><Play v-else :size="15" />
                  </button>
                  <button class="prod-icon danger" type="button" :disabled="busy[device.id] === 'delete'" title="删除" aria-label="删除" @click="removeDevice(device)"><Trash2 :size="15" /></button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
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
          <button class="prod-icon" type="button" :disabled="page <= 1" aria-label="上一页" @click="page--"><ChevronLeft :size="15" /></button>
          <span class="prod-page-info mono">{{ page }} / {{ totalPages }}</span>
          <button class="prod-icon" type="button" :disabled="page >= totalPages" aria-label="下一页" @click="page++"><ChevronRight :size="15" /></button>
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
                    <label for="prod-create-region">区域</label>
                    <select id="prod-create-region" v-model="createForm.region_id" class="prod-select prod-select-full" required>
                      <option v-for="rid in regionScopes" :key="rid" :value="rid">{{ rid }}</option>
                    </select>
                    <span v-if="regionScopes.length === 0" class="prod-meta-hint">当前账号没有可用区域范围，请联系管理员分配。</span>
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
                <div><dt>同步状态</dt><dd>{{ syncLabel(detailDevice) }}</dd></div>
                <div><dt>同步版本</dt><dd class="mono">{{ detailDevice.access_synced_version ?? '—' }}</dd></div>
                <div><dt>运行时状态</dt><dd>
                  <span class="prod-pill" :class="runtimeState(detailDevice) === 'online' ? 'pill-runtime-on' : runtimeState(detailDevice) === 'offline' ? 'pill-runtime-off' : 'pill-muted'">
                    <span class="pill-dot" :class="runtimeState(detailDevice) === 'online' ? 'dot-online' : 'dot-offline'" />
                    {{ runtimeLabel(detailDevice) }}
                  </span>
                </dd></div>
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
      <span>new-vision 节点管理系统</span>
      <span class="prod-footer-deps">
        <Zap :size="13" />node-app · {{ devices.length }} 台设备
      </span>
    </footer>
  </div>
</template>

<style scoped>
.prod-shell { min-height: 100vh; background: #f3f4f6; color: #1a1f26; font-family: 'DM Sans', 'Noto Sans SC', system-ui, sans-serif; }
/* ---------- topbar ---------- */
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
.prod-main { max-width: 1240px; margin: 0 auto; padding: 34px 32px 64px; }
.prod-page-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 18px; }
.prod-heading .prod-eyebrow { display: inline-flex; align-items: center; gap: 8px; color: #8a939d; font-size: 11px; font-weight: 700; letter-spacing: .12em; }
.prod-heading h1 { margin: 7px 0 0; font-size: 24px; letter-spacing: -.01em; }
.prod-heading p { margin: 7px 0 0; color: #6b7683; font-size: 13.5px; }
/* ---------- toast ---------- */
.prod-flash { display: inline-flex; align-items: center; gap: 8px; margin-top: 18px; padding: 10px 15px; color: #1d714e; background: #ecf7f1; border: 1px solid #c5e6d5; border-radius: 9px; font-size: 13px; font-weight: 600; box-shadow: 0 4px 14px rgba(29,113,78,.08); }
.toast-enter-active, .toast-leave-active { transition: opacity .2s, transform .2s; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(-6px); }
/* ---------- metrics ---------- */
.prod-stats { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; margin-top: 24px; }
.prod-stat { display: flex; flex-direction: column; gap: 8px; padding: 16px 18px; background: #fff; border: 1px solid #e4e7eb; border-radius: 11px; }
.prod-stat-label { display: inline-flex; align-items: center; gap: 7px; color: #6b7683; font-size: 12px; font-weight: 600; }
.stat-dot { width: 7px; height: 7px; border-radius: 50%; }
.dot-online { background: #10b981; }
.dot-offline { background: #a6b0ba; }
.dot-sync { background: #10b981; }
.dot-pending { background: #d97706; }
.prod-stat-value { font-size: 26px; font-weight: 700; letter-spacing: -.02em; font-variant-numeric: tabular-nums; line-height: 1; }
.stat-online { color: #0e9f6e; }
.stat-offline { color: #7c8792; }
/* ---------- toolbar ---------- */
.prod-toolbar { display: flex; gap: 10px; margin-top: 20px; align-items: center; }
.prod-search { position: relative; flex: 1; max-width: 340px; }
.prod-search-icon { position: absolute; left: 12px; top: 50%; transform: translateY(-50%); color: #8a939d; pointer-events: none; }
.prod-search input { width: 100%; padding: 10px 34px 10px 35px; font: inherit; font-size: 13.5px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; transition: border-color .15s, box-shadow .15s; }
.prod-search input:focus-visible { outline: none; border-color: #1a1f26; box-shadow: 0 0 0 3px rgba(26,31,38,.1); }
.prod-search-clear { position: absolute; right: 9px; top: 50%; transform: translateY(-50%); display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; color: #8a939d; background: none; border: 0; border-radius: 5px; cursor: pointer; }
.prod-search-clear:hover { color: #1a1f26; background: #f0f2f4; }
.prod-select { padding: 10px 11px; font: inherit; font-size: 13px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; cursor: pointer; }
.prod-select:focus-visible { outline: none; border-color: #1a1f26; box-shadow: 0 0 0 3px rgba(26,31,38,.1); }
.prod-refresh { margin-left: auto; }
/* ---------- buttons & icons ---------- */
.prod-button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; padding: 10px 16px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; font-size: 13.5px; font-weight: 600; cursor: pointer; transition: border-color .15s, background .15s, transform .05s; }
.prod-button:hover:not(:disabled) { border-color: #aeb7c1; }
.prod-button:active:not(:disabled) { transform: translateY(1px); }
.prod-button:disabled { cursor: wait; opacity: .6; }
.prod-button-primary { color: #fff; background: #1a1f26; border-color: #1a1f26; }
.prod-button-primary:hover:not(:disabled) { background: #2d3540; border-color: #2d3540; }
.prod-icon { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; color: #5f6873; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; cursor: pointer; transition: color .15s, border-color .15s, background .15s; }
.prod-icon:hover:not(:disabled) { color: #1a1f26; border-color: #aeb7c1; }
.prod-icon.danger:hover:not(:disabled) { color: #b44444; border-color: #e9c1c1; background: #fdf6f6; }
.prod-icon:disabled { cursor: not-allowed; opacity: .45; }
/* ---------- table ---------- */
.prod-table-wrap { margin-top: 16px; overflow: hidden; background: #fff; border: 1px solid #e4e7eb; border-radius: 12px; box-shadow: 0 1px 2px rgba(16,24,40,.04); }
.prod-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.prod-table th { padding: 12px 16px; text-align: left; color: #8a939d; font-size: 11px; font-weight: 700; letter-spacing: .06em; border-bottom: 1px solid #e4e7eb; white-space: nowrap; background: #fafbfc; }
.prod-table td { padding: 13px 16px; border-bottom: 1px solid #f0f2f4; vertical-align: middle; }
.prod-table tr:last-child td { border-bottom: 0; }
.prod-row { transition: background .12s; }
.prod-row:hover { background: #f8fafb; }
.prod-row:hover .prod-actions .prod-icon { border-color: #cdd4db; }
.prod-name-cell { min-width: 150px; }
.prod-name { font-weight: 600; color: #1a1f26; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-name-sub { margin-top: 3px; color: #9aa3ac; font-size: 11px; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-id { color: #4b5563; font-variant-numeric: tabular-nums; white-space: nowrap; }
.prod-type { display: inline-flex; align-items: center; gap: 6px; color: #4b5563; white-space: nowrap; }
.prod-type svg { color: #9aa3ac; }
.prod-manufacturer { color: #4b5563; }
.prod-pill { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; white-space: nowrap; }
.pill-dot { width: 6px; height: 6px; border-radius: 50%; }
.pill-enable { color: #1d714e; background: #e9f6ef; }
.pill-muted { color: #65707b; background: #f0f2f4; }
.pill-synced { color: #1d714e; background: #e9f6ef; }
.pill-pending { color: #92400e; background: #fdf0e0; }
.pill-runtime-on { color: #1d714e; background: #e9f6ef; }
.pill-runtime-off { color: #65707b; background: #f0f2f4; }
.dot-online { background: #10b981; }
.dot-offline { background: #a6b0ba; }
.dot-pending { background: #d97706; }
.prod-table .col-actions { text-align: right; }
.prod-actions { display: inline-flex; gap: 5px; opacity: .55; transition: opacity .15s; }
.prod-row:hover .prod-actions, .prod-actions:focus-within { opacity: 1; }
/* ---------- skeleton ---------- */
.skeleton-row td { padding: 16px; }
.sk { display: inline-block; background: linear-gradient(90deg, #eef0f2 25%, #f6f7f8 37%, #eef0f2 63%); background-size: 400% 100%; animation: sk-shimmer 1.3s ease infinite; border-radius: 6px; }
.sk-name { width: 120px; height: 13px; }
.sk-id { width: 150px; height: 12px; }
.sk-pill { width: 56px; height: 18px; border-radius: 999px; }
.sk-actions { width: 130px; height: 28px; }
@keyframes sk-shimmer { 0% { background-position: 100% 0; } 100% { background-position: -100% 0; } }
/* ---------- error ---------- */
.prod-error { display: flex; align-items: center; gap: 13px; margin-top: 16px; padding: 15px 18px; color: #9a4646; background: #fdf3f3; border: 1px solid #f0d2d2; border-radius: 11px; }
.prod-error svg { flex-shrink: 0; }
.prod-error strong { font-size: 13.5px; }
.prod-error p { margin: 3px 0 0; color: #b06565; font-size: 12.5px; }
.prod-error .prod-button { margin-left: auto; color: #9a4646; border-color: #e9c1c1; }
.prod-form .prod-error { margin-top: 14px; }
/* ---------- empty ---------- */
.prod-empty { display: flex; flex-direction: column; align-items: center; gap: 6px; margin-top: 16px; padding: 52px 20px; text-align: center; background: #fff; border: 1px dashed #d4d9df; border-radius: 12px; }
.prod-empty-icon { display: inline-flex; align-items: center; justify-content: center; width: 52px; height: 52px; margin-bottom: 6px; color: #9aa3ac; background: #f4f6f8; border-radius: 13px; }
.prod-empty strong { font-size: 14.5px; }
.prod-empty p { margin: 0 0 10px; color: #8a939d; font-size: 13px; }
/* ---------- pagination ---------- */
.prod-pagination { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 14px; }
.prod-page-count { color: #8a939d; font-size: 12px; }
.prod-page-btns { display: flex; align-items: center; gap: 8px; }
.prod-page-info { color: #5f6873; font-size: 12px; }
/* ---------- overlay / modal / drawer ---------- */
.prod-overlay { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(13,18,24,.48); backdrop-filter: blur(2px); }
.prod-modal { width: 100%; max-width: 560px; max-height: 88vh; overflow-y: auto; background: #fff; border-radius: 14px; box-shadow: 0 24px 64px rgba(13,18,24,.28); }
.prod-modal-narrow { max-width: 480px; }
.prod-modal-head { display: flex; align-items: center; justify-content: space-between; padding: 19px 22px; border-bottom: 1px solid #eef0f3; }
.prod-modal-head h2 { margin: 0; font-size: 16.5px; display: flex; align-items: center; gap: 10px; }
.prod-type-badge { display: inline-block; padding: 3px 9px; color: #1a1f26; background: #eef1f4; border-radius: 999px; font-size: 11.5px; font-weight: 700; }
.prod-form { padding: 20px 22px; }
.prod-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.prod-field { display: grid; gap: 6px; }
.prod-field-full { grid-column: 1 / -1; }
.prod-field label { font-size: 12px; font-weight: 700; color: #5f6873; }
.prod-field input { width: 100%; padding: 10px 11px; font: inherit; font-size: 13.5px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 8px; box-sizing: border-box; transition: border-color .15s, box-shadow .15s; }
.prod-field input:focus-visible { outline: none; border-color: #1a1f26; box-shadow: 0 0 0 3px rgba(26,31,38,.1); }
.prod-select-full { width: 100%; }
.prod-select-full:focus-visible { outline: none; border-color: #1a1f26; box-shadow: 0 0 0 3px rgba(26,31,38,.1); }
.prod-checkbox { display: inline-flex; align-items: center; gap: 8px; font-size: 13px !important; font-weight: 600 !important; color: #1a1f26 !important; padding-top: 22px; cursor: pointer; }
.prod-input-action { display: flex; gap: 6px; }
.prod-input-action input { flex: 1; }
.prod-manufacturer { position: relative; }
.prod-manufacturer-menu { position: absolute; z-index: 60; top: calc(100% + 4px); left: 0; right: 0; max-height: 220px; overflow-y: auto; padding: 6px; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; box-shadow: 0 10px 28px rgba(13,18,24,.16); }
.prod-manufacturer-option { display: block; width: 100%; padding: 9px 10px; text-align: left; color: #1a1f26; background: none; border: 0; border-radius: 6px; font-size: 13px; cursor: pointer; }
.prod-manufacturer-option:hover { background: #f1f3f5; }
.prod-manufacturer-add { display: block; width: 100%; padding: 9px 10px; margin-top: 4px; text-align: left; color: #1a1f26; background: none; border: 0; border-top: 1px solid #e7eaed; border-radius: 0; font-size: 13px; font-weight: 600; cursor: pointer; }
.prod-manufacturer-add:hover { color: #0a6dd4; }
.prod-manufacturer-add:disabled { cursor: not-allowed; opacity: .5; }
.prod-manufacturer-new { width: 100%; padding: 8px 10px; font: inherit; font-size: 13px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 6px; box-sizing: border-box; }
.prod-code-preview { display: flex; align-items: center; gap: 8px; margin: 14px 0 0; padding: 10px 12px; color: #5f6873; background: #f4f6f8; border-radius: 8px; }
.prod-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px; }
.prod-meta-hint { margin: 0; color: #8a939d; font-size: 12px; }
.prod-type-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding: 20px 22px; }
.prod-type-card { display: grid; gap: 5px; padding: 17px 16px; text-align: left; color: #1a1f26; background: #f8f9fa; border: 1px solid #e1e5e9; border-radius: 10px; cursor: pointer; transition: border-color .15s, background .15s, transform .05s; }
.prod-type-card:hover { border-color: #1a1f26; background: #fff; }
.prod-type-card:active { transform: translateY(1px); }
.prod-type-card-icon { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; margin-bottom: 4px; color: #1a1f26; background: #fff; border: 1px solid #e1e5e9; border-radius: 8px; }
.prod-type-card strong { font-size: 14.5px; }
.prod-type-card .mono { color: #8a939d; }
.prod-drawer-overlay { justify-content: flex-end; padding: 0; }
.prod-drawer { width: 100%; max-width: 440px; height: 100vh; overflow-y: auto; background: #fff; box-shadow: -24px 0 64px rgba(13,18,24,.18); }
.prod-drawer-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 22px 24px; border-bottom: 1px solid #eef0f3; }
.prod-drawer-title { display: flex; align-items: center; gap: 12px; }
.prod-drawer-icon { display: inline-flex; align-items: center; justify-content: center; width: 42px; height: 42px; color: #1a1f26; background: #f1f3f5; border: 1px solid #e4e7eb; border-radius: 10px; }
.prod-drawer-title h2 { margin: 0 0 5px; font-size: 17.5px; }
.prod-detail-list { margin: 0; padding: 8px 24px 16px; }
.prod-detail-list div { display: flex; justify-content: space-between; gap: 16px; padding: 11.5px 0; border-bottom: 1px solid #f0f2f4; font-size: 13px; }
.prod-detail-list dt { color: #7a838e; flex-shrink: 0; }
.prod-detail-list dd { margin: 0; color: #1a1f26; text-align: right; word-break: break-all; }
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
.prod-footer { max-width: 1240px; margin: 0 auto; padding: 20px 32px 32px; display: flex; align-items: center; justify-content: space-between; color: #9aa3ac; font-size: 12px; }
.prod-footer-deps { display: inline-flex; align-items: center; gap: 6px; }
.mono { font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 12px; font-variant-numeric: tabular-nums; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* ---------- reduced motion ---------- */
@media (prefers-reduced-motion: reduce) {
  .sk, .spinning { animation: none; }
  .prod-row, .prod-button, .prod-icon, .prod-nav-link, .prod-type-card, .prod-search input, .prod-select, .prod-field input { transition: none; }
  .modal-enter-active, .modal-leave-active, .drawer-enter-active, .drawer-leave-active, .toast-enter-active, .toast-leave-active { transition: none; }
  .modal-enter-from, .modal-leave-to, .drawer-enter-from, .drawer-leave-to, .toast-enter-from, .toast-leave-to { opacity: 1; transform: none; }
}
/* ---------- responsive ---------- */
@media (max-width: 900px) {
  .prod-topbar { gap: 14px; padding: 0 16px; }
  .prod-brand-text span { display: none; }
  .prod-deps { display: none; }
  .prod-main { padding: 24px 16px 48px; }
  .prod-stats { grid-template-columns: repeat(2, 1fr); gap: 10px; }
  .prod-toolbar { flex-wrap: wrap; }
  .prod-search { max-width: none; flex-basis: 100%; }
  .prod-refresh { margin-left: 0; }
  .prod-table-wrap { overflow-x: auto; }
  .prod-table { min-width: 860px; }
  .prod-page-head { flex-direction: column; align-items: stretch; }
  .prod-page-head .prod-button { align-self: flex-start; }
}
</style>
