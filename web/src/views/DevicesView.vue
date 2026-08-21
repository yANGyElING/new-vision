<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Activity, AlertTriangle, Check, ChevronLeft, ChevronRight, CircleHelp, Clock3, Database,
  Edit3, Eye, ListPlus, Monitor, Pause, Play, Plus, RefreshCw, Search, Server, Trash2, WifiOff, X,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { fetchHealth, type HealthState } from '@/api/health'
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
  try {
    const state = await fetchHealth(4000, activeHealthController?.signal)
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
  if (runtimeFilter.value) list = list.filter((d) => (d.runtime?.state ?? 'offline') === runtimeFilter.value)
  return [...list].sort((a, b) => a.device_access_id.localeCompare(b.device_access_id))
})

const totalPages = computed(() => Math.max(1, Math.ceil(filteredDevices.value.length / pageSize)))
const pageDevices = computed(() => filteredDevices.value.slice((page.value - 1) * pageSize, page.value * pageSize))

function resetPage() { page.value = 1 }

// ---------- create ----------
const createOpen = ref(false)
const typePickerOpen = ref(false)
const creating = ref(false)
const createError = ref('')
const createForm = ref<{
  device_type: string; center_code: string; device_name: string; manufacturer: string
  sip_realm: string; password: string; enabled: boolean
}>({
  device_type: DEVICE_TYPES[0].code, center_code: '34020000', device_name: '', manufacturer: '',
  sip_realm: '3402000000', password: '', enabled: true,
})

const manufacturerOptions = ref(['海康威视', '大华', '宇视', '华为', '天地伟业', '科达', '其他'])
const manufacturerFilter = ref('')
const manufacturerOpen = ref(false)
const addingManufacturer = ref(false)
const newManufacturer = ref('')

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

onMounted(() => {
  void loadDevices()
  void refreshHealth()
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
        <span class="prod-logo"><Monitor :size="18" /></span>
        <div>
          <strong>new-vision 节点管理</strong>
          <span class="prod-subtitle">视频接入节点 · 设备管理</span>
        </div>
      </div>
      <nav class="prod-nav" aria-label="主导航">
        <RouterLink to="/devices" class="prod-nav-link active">设备管理</RouterLink>
        <RouterLink to="/" class="prod-nav-link">测试控制台</RouterLink>
      </nav>
      <div class="prod-topbar-right">
        <span class="prod-health" :class="`health-${healthTone}`" :title="healthSummary">
          <span class="prod-health-dot" />{{ healthTitle }}
        </span>
      </div>
    </header>

    <main class="prod-main">
      <div class="prod-page-head">
        <div>
          <h1>设备管理</h1>
          <p class="prod-page-desc">管理接入节点的视频设备：新增、编辑、启停与删除。</p>
        </div>
        <button class="prod-button prod-button-primary" type="button" @click="openTypePicker">
          <Plus :size="16" />新增设备
        </button>
      </div>

      <p v-if="flash" class="prod-flash" role="status">{{ flash }}</p>
      <p v-if="loadError" class="prod-error" role="alert">{{ loadError }}</p>

      <!-- filters -->
      <div class="prod-toolbar">
        <div class="prod-search">
          <Search :size="16" class="prod-search-icon" />
          <input v-model="search" placeholder="搜索名称或接入 ID" @input="resetPage" />
        </div>
        <select v-model="typeFilter" class="prod-select" @change="resetPage" aria-label="按类型筛选">
          <option value="">全部类型</option>
          <option v-for="t in DEVICE_TYPES" :key="t.code" :value="t.code">{{ t.label }}</option>
        </select>
        <select v-model="syncFilter" class="prod-select" @change="resetPage" aria-label="按同步状态筛选">
          <option value="">全部同步状态</option>
          <option value="synced">已同步</option>
          <option value="pending">同步中</option>
        </select>
        <select v-model="runtimeFilter" class="prod-select" @change="resetPage" aria-label="按运行状态筛选">
          <option value="">全部运行状态</option>
          <option value="online">在线</option>
          <option value="offline">离线</option>
        </select>
        <button class="prod-button" type="button" :disabled="loading" aria-label="刷新设备列表" title="刷新" @click="loadDevices">
          <RefreshCw :size="16" :class="{ spinning: loading }" />
        </button>
      </div>

      <!-- table -->
      <div class="prod-table-wrap">
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
            <tr v-if="!loading && filteredDevices.length === 0">
              <td colspan="8" class="prod-empty-row">
                {{ search || typeFilter || syncFilter || runtimeFilter ? '没有符合筛选条件的设备' : '还没有设备，点击“新增设备”创建第一台。' }}
              </td>
            </tr>
            <tr v-for="device in pageDevices" :key="device.id">
              <td class="prod-name">{{ device.device_name || '—' }}</td>
              <td class="mono">{{ device.device_access_id }}</td>
              <td>{{ deviceTypeLabel(device.device_type) }}</td>
              <td>{{ device.manufacturer || '—' }}</td>
              <td>
                <span class="prod-pill" :class="device.enabled ? 'pill-up' : 'pill-muted'">
                  <Play v-if="device.enabled" :size="13" /><Pause v-else :size="13" />
                  {{ device.enabled ? '启用' : '停用' }}
                </span>
              </td>
              <td>
                <span class="prod-pill" :class="device.access_sync_status === 'synced' ? 'pill-up' : 'pill-unknown'">
                  {{ syncLabel(device) }}
                </span>
              </td>
              <td>
                <span class="prod-pill" :class="runtimeState(device) === 'online' ? 'pill-up' : runtimeState(device) === 'offline' ? 'pill-down' : 'pill-unknown'">
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

      <!-- pagination -->
      <div v-if="filteredDevices.length > pageSize" class="prod-pagination">
        <button class="prod-icon" type="button" :disabled="page <= 1" aria-label="上一页" @click="page--"><ChevronLeft :size="15" /></button>
        <span class="prod-page-info">{{ page }} / {{ totalPages }}</span>
        <button class="prod-icon" type="button" :disabled="page >= totalPages" aria-label="下一页" @click="page++"><ChevronRight :size="15" /></button>
      </div>

      <!-- type picker -->
      <div v-if="typePickerOpen" class="prod-overlay" @click.self="typePickerOpen = false">
        <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" aria-label="选择设备类型">
          <div class="prod-modal-head">
            <h2>选择设备类型</h2>
            <button class="prod-icon" type="button" aria-label="关闭" @click="typePickerOpen = false"><X :size="16" /></button>
          </div>
          <div class="prod-type-grid">
            <button v-for="t in DEVICE_TYPES" :key="t.code" class="prod-type-card" type="button" @click="chooseType(t.code)">
              <strong>{{ t.label }}</strong>
              <span class="mono">类型码 {{ t.code }}</span>
            </button>
          </div>
        </div>
      </div>

      <!-- create modal -->
      <div v-if="createOpen" class="prod-overlay" @click.self="closeCreate">
        <div class="prod-modal" role="dialog" aria-modal="true" aria-label="新增设备">
          <div class="prod-modal-head">
            <h2>新增设备 <span class="prod-type-badge">{{ deviceTypeLabel(createForm.device_type) }}</span></h2>
            <button class="prod-icon" type="button" aria-label="关闭" @click="closeCreate"><X :size="16" /></button>
          </div>
          <form class="prod-form" @submit.prevent="submitCreate">
            <div class="prod-form-grid">
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
            <p v-if="accessIDPreview" class="prod-code-preview mono">编码预览：{{ accessIDPreview }}·序号由系统分配</p>
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

      <!-- edit modal -->
      <div v-if="editingDevice" class="prod-overlay" @click.self="closeEdit">
        <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" aria-label="编辑设备">
          <div class="prod-modal-head">
            <h2>编辑设备</h2>
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

      <!-- detail drawer -->
      <div v-if="detailDevice" class="prod-overlay prod-drawer-overlay" @click.self="closeDetail">
        <aside class="prod-drawer" role="dialog" aria-modal="true" aria-label="设备详情">
          <div class="prod-drawer-head">
            <div>
              <h2>{{ detailDevice.device_name || '未命名设备' }}</h2>
              <span class="prod-type-badge">{{ deviceTypeLabel(detailDevice.device_type) }}</span>
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
              <span class="prod-pill" :class="runtimeState(detailDevice) === 'online' ? 'pill-up' : runtimeState(detailDevice) === 'offline' ? 'pill-down' : 'pill-unknown'">
                {{ runtimeLabel(detailDevice) }}
              </span>
            </dd></div>
            <div v-if="detailDevice.runtime?.last_seen"><dt>最近活跃</dt><dd class="mono">{{ detailDevice.runtime.last_seen }}</dd></div>
            <div v-if="detailDevice.runtime?.remote_address"><dt>远端地址</dt><dd class="mono">{{ detailDevice.runtime.remote_address }}</dd></div>
            <div><dt>创建时间</dt><dd class="mono">{{ detailDevice.created_at }}</dd></div>
            <div><dt>更新时间</dt><dd class="mono">{{ detailDevice.updated_at }}</dd></div>
          </dl>
          <div class="prod-drawer-actions">
            <button class="prod-button" type="button" @click="openEdit(detailDevice); closeDetail()">
              <Edit3 :size="15" />编辑
            </button>
          </div>
        </aside>
      </div>
    </main>

    <footer class="prod-footer">
      <span>new-vision 节点管理 · {{ devices.length }} 台设备</span>
      <span class="prod-footer-deps">
        <Database :size="13" />PostgreSQL <Check :size="13" class="health-ok" />&nbsp;
        <Server :size="13" />Redis <Check :size="13" class="health-ok" />
      </span>
    </footer>
  </div>
</template>

<style scoped>
.prod-shell { min-height: 100vh; background: #f5f6f8; color: #20252c; font-family: 'DM Sans', 'Noto Sans SC', system-ui, sans-serif; }
.prod-topbar { display: flex; align-items: center; gap: 28px; padding: 0 28px; height: 60px; background: #fff; border-bottom: 1px solid #e2e6ea; position: sticky; top: 0; z-index: 30; }
.prod-brand { display: flex; align-items: center; gap: 10px; }
.prod-logo { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; color: #fff; background: #20252c; border-radius: 8px; }
.prod-brand strong { font-size: 15px; display: block; line-height: 1.2; }
.prod-subtitle { font-size: 11px; color: #8a939d; }
.prod-nav { display: flex; gap: 4px; margin-left: 8px; }
.prod-nav-link { padding: 8px 12px; color: #5f6873; font-size: 14px; font-weight: 600; text-decoration: none; border-radius: 7px; }
.prod-nav-link:hover { background: #f1f3f5; color: #20252c; }
.prod-nav-link.active { color: #20252c; background: #eef1f4; }
.prod-topbar-right { margin-left: auto; display: flex; align-items: center; }
.prod-health { display: inline-flex; align-items: center; gap: 7px; font-size: 13px; font-weight: 600; color: #5f6873; }
.prod-health-dot { width: 8px; height: 8px; border-radius: 50%; }
.health-up .prod-health-dot { background: #1d9e62; }
.health-warn .prod-health-dot { background: #d99a26; }
.health-down .prod-health-dot { background: #c04545; }
.health-ok { color: #1d9e62; }
.prod-main { max-width: 1200px; margin: 0 auto; padding: 28px 28px 60px; }
.prod-page-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; }
.prod-page-head h1 { margin: 0; font-size: 22px; }
.prod-page-desc { margin: 6px 0 0; color: #7a838e; font-size: 13px; }
.prod-flash { margin: 14px 0 0; padding: 10px 14px; color: #1d714e; background: #ecf7f1; border: 1px solid #bfe3d1; border-radius: 7px; font-size: 13px; }
.prod-error { margin: 14px 0 0; color: #9a4646; font-size: 13px; }
.prod-toolbar { display: flex; gap: 10px; margin-top: 20px; align-items: center; }
.prod-search { position: relative; flex: 1; max-width: 320px; }
.prod-search-icon { position: absolute; left: 11px; top: 50%; transform: translateY(-50%); color: #8a939d; }
.prod-search input { width: 100%; padding: 9px 11px 9px 34px; font: inherit; font-size: 14px; color: #20252c; background: #fff; border: 1px solid #d9dee4; border-radius: 7px; }
.prod-search input:focus-visible { outline: 3px solid #84b9ff; outline-offset: 1px; }
.prod-select { padding: 9px 11px; font: inherit; font-size: 13px; color: #20252c; background: #fff; border: 1px solid #d9dee4; border-radius: 7px; }
.prod-button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; padding: 9px 15px; color: #20252c; background: #fff; border: 1px solid #d9dee4; border-radius: 7px; font-size: 14px; font-weight: 600; cursor: pointer; }
.prod-button:hover:not(:disabled) { border-color: #aeb7c1; }
.prod-button:disabled { cursor: wait; opacity: .65; }
.prod-button-primary { color: #fff; background: #20252c; border-color: #20252c; }
.prod-button-primary:hover:not(:disabled) { background: #3b444e; border-color: #3b444e; }
.prod-table-wrap { margin-top: 14px; overflow-x: auto; background: #fff; border: 1px solid #e2e6ea; border-radius: 10px; }
.prod-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.prod-table th { padding: 11px 14px; text-align: left; color: #7a838e; font-size: 12px; font-weight: 700; border-bottom: 1px solid #e2e6ea; white-space: nowrap; background: #fafbfc; }
.prod-table td { padding: 12px 14px; border-bottom: 1px solid #eef0f3; vertical-align: middle; }
.prod-table tr:last-child td { border-bottom: 0; }
.prod-table .col-actions { text-align: right; }
.prod-name { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-empty-row { padding: 34px 14px; text-align: center; color: #8a939d; }
.prod-pill { display: inline-flex; align-items: center; gap: 5px; padding: 4px 9px; border-radius: 999px; font-size: 12px; font-weight: 600; }
.pill-up { color: #1d714e; background: #e6f4ec; }
.pill-down { color: #9a4646; background: #fbeaea; }
.pill-unknown { color: #7a6a2f; background: #f6f0dd; }
.pill-muted { color: #65707b; background: #eceff2; }
.prod-actions { display: inline-flex; gap: 5px; }
.prod-icon { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; color: #5f6873; background: #fff; border: 1px solid #d9dee4; border-radius: 7px; cursor: pointer; }
.prod-icon:hover:not(:disabled) { color: #20252c; border-color: #aeb7c1; }
.prod-icon.danger:hover:not(:disabled) { color: #9a4646; border-color: #edc1c1; }
.prod-icon:disabled { cursor: not-allowed; opacity: .5; }
.prod-pagination { display: flex; align-items: center; justify-content: flex-end; gap: 10px; margin-top: 14px; }
.prod-page-info { font-size: 13px; color: #5f6873; }
.prod-overlay { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(15, 22, 30, .45); }
.prod-modal { width: 100%; max-width: 560px; max-height: 88vh; overflow-y: auto; background: #fff; border-radius: 12px; box-shadow: 0 20px 60px rgba(15, 22, 30, .25); }
.prod-modal-narrow { max-width: 480px; }
.prod-modal-head { display: flex; align-items: center; justify-content: space-between; padding: 18px 22px; border-bottom: 1px solid #eef0f3; }
.prod-modal-head h2 { margin: 0; font-size: 17px; display: flex; align-items: center; gap: 10px; }
.prod-type-badge { display: inline-block; padding: 3px 9px; color: #20252c; background: #eef1f4; border-radius: 999px; font-size: 12px; font-weight: 700; }
.prod-form { padding: 20px 22px; }
.prod-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.prod-field { display: grid; gap: 6px; }
.prod-field-full { grid-column: 1 / -1; }
.prod-field label { font-size: 12px; font-weight: 700; color: #5f6873; }
.prod-field input { width: 100%; padding: 9px 11px; font: inherit; font-size: 14px; color: #20252c; background: #fff; border: 1px solid #d9dee4; border-radius: 7px; box-sizing: border-box; }
.prod-field input:focus-visible { outline: 3px solid #84b9ff; outline-offset: 1px; }
.prod-checkbox { display: inline-flex; align-items: center; gap: 8px; font-size: 13px !important; font-weight: 600 !important; color: #20252c !important; padding-top: 22px; }
.prod-input-action { display: flex; gap: 6px; }
.prod-input-action input { flex: 1; }
.prod-manufacturer { position: relative; }
.prod-manufacturer-menu { position: absolute; z-index: 60; top: calc(100% + 4px); left: 0; right: 0; max-height: 220px; overflow-y: auto; padding: 6px; background: #fff; border: 1px solid #d9dee4; border-radius: 8px; box-shadow: 0 8px 24px rgba(23, 32, 44, .14); }
.prod-manufacturer-option { display: block; width: 100%; padding: 9px 10px; text-align: left; color: #20252c; background: none; border: 0; border-radius: 6px; font-size: 13px; cursor: pointer; }
.prod-manufacturer-option:hover { background: #f1f3f5; }
.prod-manufacturer-add { display: block; width: 100%; padding: 9px 10px; margin-top: 4px; text-align: left; color: #20252c; background: none; border: 0; border-top: 1px solid #e7eaed; border-radius: 0; font-size: 13px; font-weight: 600; cursor: pointer; }
.prod-manufacturer-add:hover { color: #0a6dd4; }
.prod-manufacturer-add:disabled { cursor: not-allowed; opacity: .5; }
.prod-manufacturer-new { width: 100%; padding: 8px 10px; font: inherit; font-size: 13px; color: #20252c; background: #fff; border: 1px solid #d9dee4; border-radius: 6px; box-sizing: border-box; }
.prod-code-preview { margin: 14px 0 0; padding: 9px 12px; color: #5f6873; background: #f4f6f8; border-radius: 7px; }
.prod-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px; }
.prod-meta-hint { margin: 0; color: #8a939d; font-size: 12px; }
.prod-type-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding: 20px 22px; }
.prod-type-card { display: grid; gap: 6px; padding: 18px; text-align: left; color: #20252c; background: #f8f9fa; border: 1px solid #d9dee4; border-radius: 9px; cursor: pointer; }
.prod-type-card:hover { border-color: #20252c; background: #fff; }
.prod-type-card strong { font-size: 15px; }
.prod-type-card .mono { color: #7a838e; }
.prod-drawer-overlay { justify-content: flex-end; padding: 0; }
.prod-drawer { width: 100%; max-width: 440px; height: 100vh; overflow-y: auto; background: #fff; box-shadow: -20px 0 60px rgba(15, 22, 30, .18); }
.prod-drawer-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 22px 24px; border-bottom: 1px solid #eef0f3; }
.prod-drawer-head h2 { margin: 0 0 6px; font-size: 18px; }
.prod-detail-list { margin: 0; padding: 8px 24px 16px; }
.prod-detail-list div { display: flex; justify-content: space-between; gap: 16px; padding: 11px 0; border-bottom: 1px solid #f0f2f4; font-size: 13px; }
.prod-detail-list dt { color: #7a838e; flex-shrink: 0; }
.prod-detail-list dd { margin: 0; color: #20252c; text-align: right; word-break: break-all; }
.prod-drawer-actions { padding: 16px 24px 24px; }
.prod-footer { max-width: 1200px; margin: 0 auto; padding: 18px 28px 30px; display: flex; align-items: center; justify-content: space-between; color: #8a939d; font-size: 12px; }
.prod-footer-deps { display: inline-flex; align-items: center; gap: 5px; }
.mono { font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 12px; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 860px) {
  .prod-topbar { gap: 12px; padding: 0 16px; }
  .prod-subtitle { display: none; }
  .prod-nav { margin-left: 0; }
  .prod-main { padding: 20px 16px 40px; }
  .prod-toolbar { flex-wrap: wrap; }
  .prod-search { max-width: none; flex-basis: 100%; }
  .prod-form-grid { grid-template-columns: 1fr; }
}
</style>
