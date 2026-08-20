<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Activity, AlertTriangle, Check, CircleHelp, Clock3, Database, KeyRound, ListPlus,
  LogOut, Pause, Play, Plus, RefreshCw, Server, Trash2, WifiOff, X, Zap,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { fetchHealth, type CheckState, type HealthState } from '@/api/health'
import { listDevices, createDevice, setDeviceEnabled, deleteDevice, generateAccessID, type Device } from '@/api/devices'
import { getSnapshot, pollEvents, ackEvents, type RuntimeSnapshot, type AccessEvent } from '@/api/access'
import { sipRegister, sipKeepAlive, sipUnregister } from '@/api/test'

// ---------- health ----------
const health = ref<HealthState>({ kind: 'loading' })
const refreshingHealth = ref(false)
let healthRequestId = 0
let activeHealthController: AbortController | undefined

const healthTitle = computed(() => {
  switch (health.value.kind) {
    case 'ready': return '运行正常'
    case 'degraded': return '依赖异常'
    case 'unreachable': return '无法连接'
    case 'invalid': return '响应异常'
    default: return '检查中'
  }
})

const healthSummary = computed(() => {
  switch (health.value.kind) {
    case 'ready': return '节点服务和必要依赖均已就绪。'
    case 'degraded': return '节点仍在运行，但至少一个必要依赖不可用。'
    case 'unreachable':
    case 'invalid': return health.value.message
    default: return '正在读取节点当前状态。'
  }
})

function healthCheck(name: 'postgres' | 'redis'): CheckState | 'unknown' {
  if (health.value.kind === 'ready' || health.value.kind === 'degraded') return health.value.health.checks[name]
  return 'unknown'
}

async function refreshHealth() {
  const currentId = ++healthRequestId
  activeHealthController?.abort()
  activeHealthController = new AbortController()
  refreshingHealth.value = true
  health.value = { kind: 'loading' }
  const result = await fetchHealth(4000, activeHealthController.signal)
  if (currentId === healthRequestId) {
    health.value = result
    refreshingHealth.value = false
  }
}

// ---------- devices ----------
const devices = ref<Device[]>([])
const loadingDevices = ref(false)
const devicesError = ref('')

const createForm = ref({ device_access_id: '', sip_realm: '3402000000', password: '', enabled: true })
const creating = ref(false)
const createError = ref('')
const createSuccess = ref('')

const deviceBusy = ref<Record<string, string>>({})

async function loadDevices() {
  loadingDevices.value = true
  devicesError.value = ''
  try {
    devices.value = await listDevices()
  } catch (error) {
    devicesError.value = error instanceof Error ? error.message : '加载设备失败'
  } finally {
    loadingDevices.value = false
  }
}

function fillGeneratedID() {
  createForm.value.device_access_id = generateAccessID()
}

async function submitCreate() {
  createError.value = ''
  createSuccess.value = ''
  creating.value = true
  try {
    await createDevice({
      device_access_id: createForm.value.device_access_id,
      sip_username: createForm.value.device_access_id,
      sip_realm: createForm.value.sip_realm,
      password: createForm.value.password,
      enabled: createForm.value.enabled,
    })
    createSuccess.value = '设备创建成功，正在等待同步到接入层。'
    createForm.value.password = ''
    if (!createForm.value.device_access_id) fillGeneratedID()
    await loadDevices()
  } catch (error) {
    createError.value = error instanceof Error ? error.message : '创建失败'
  } finally {
    creating.value = false
  }
}

async function toggleEnabled(device: Device) {
  deviceBusy.value[device.id] = 'toggle'
  try {
    await setDeviceEnabled(device.id, !device.enabled)
    await loadDevices()
  } catch (error) {
    devicesError.value = error instanceof Error ? error.message : '操作失败'
  } finally {
    delete deviceBusy.value[device.id]
  }
}

async function removeDevice(device: Device) {
  if (!window.confirm(`确认删除设备 ${device.device_access_id}？\n此操作会移除数据库记录并通知接入层。`)) return
  deviceBusy.value[device.id] = 'delete'
  try {
    await deleteDevice(device.id)
    await loadDevices()
  } catch (error) {
    devicesError.value = error instanceof Error ? error.message : '删除失败'
  } finally {
    delete deviceBusy.value[device.id]
  }
}

// ---------- chain test ----------
type LogEntry = { at: string; device: string; action: string; ok: boolean; text: string }
const testLog = ref<LogEntry[]>([])

function pushLog(device: string, action: string, ok: boolean, text: string) {
  testLog.value.unshift({
    at: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
    device,
    action,
    ok,
    text,
  })
  if (testLog.value.length > 50) testLog.value.pop()
}

async function runSIPAction(device: Device, action: 'register' | 'keepalive' | 'unregister') {
  deviceBusy.value[device.id] = action
  const actionName = { register: '注册', keepalive: '保活', unregister: '注销' }[action]
  try {
    let status = ''
    if (action === 'register') status = (await sipRegister(device.device_access_id)).status
    else if (action === 'keepalive') status = (await sipKeepAlive(device.device_access_id)).status
    else status = (await sipUnregister(device.device_access_id)).status
    const ok = /^2\d\d/.test(status)
    pushLog(device.device_access_id, actionName, ok, ok ? `接入层响应 ${status}` : `未按预期成功：${status}`)
    if (ok && action !== 'keepalive') await loadDevices()
  } catch (error) {
    pushLog(device.device_access_id, actionName, false, error instanceof Error ? error.message : '测试请求失败')
  } finally {
    delete deviceBusy.value[device.id]
  }
}

// ---------- access runtime ----------
const snapshot = ref<RuntimeSnapshot | null>(null)
const snapshotLoading = ref(false)
const snapshotError = ref('')
const events = ref<AccessEvent[]>([])
const eventsLoading = ref(false)
const eventsError = ref('')
const acking = ref(false)
const cursor = ref(0)

async function refreshSnapshot() {
  snapshotLoading.value = true
  snapshotError.value = ''
  try {
    snapshot.value = await getSnapshot()
    cursor.value = snapshot.value.latest_sequence
  } catch (error) {
    snapshotError.value = error instanceof Error ? error.message : '获取快照失败'
  } finally {
    snapshotLoading.value = false
  }
}

async function refreshEvents() {
  eventsLoading.value = true
  eventsError.value = ''
  try {
    const result = await pollEvents(cursor.value, 100)
    events.value = [...result.events, ...events.value]
    cursor.value = result.latest_sequence
  } catch (error) {
    eventsError.value = error instanceof Error ? error.message : '获取事件失败'
  } finally {
    eventsLoading.value = false
  }
}

async function doAck() {
  acking.value = true
  try {
    await ackEvents(cursor.value)
  } catch (error) {
    eventsError.value = error instanceof Error ? error.message : 'ACK 失败'
  } finally {
    acking.value = false
  }
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return '—'
  return parsed.toLocaleString('zh-CN', { hour12: false })
}

function runtimeLabel(device: Device): string {
  const state = device.runtime?.state
  if (state === 'online') return '在线'
  if (state === 'offline') return '离线'
  return '未注册'
}

function syncLabel(device: Device): string {
  return device.access_sync_status === 'synced' ? '已同步' : '同步中'
}

onMounted(() => {
  void refreshHealth()
  void loadDevices()
  void refreshSnapshot()
})
onUnmounted(() => activeHealthController?.abort())
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <RouterLink class="brand" to="/" aria-label="返回测试控制台">
        <span class="brand-mark"><Activity :size="18" :stroke-width="2.2" /></span>
        <span>new-vision · 测试控制台</span>
      </RouterLink>
      <nav aria-label="主导航">
        <span class="test-notice"><Zap :size="14" />测试专用界面，生产前需启用认证</span>
      </nav>
    </header>

    <section class="content console-content">
      <div class="eyebrow">自治节点 · 测试控制台</div>
      <div class="heading-row">
        <div>
          <h1 id="page-title">链路测试台</h1>
          <p class="lede">创建设备，模拟 SIP 注册/保活/注销，并观察接入层运行时与事件流。</p>
        </div>
      </div>

      <!-- 1. 系统状态 -->
      <section class="console-section" aria-labelledby="section-health">
        <div class="section-heading">
          <div>
            <div class="eyebrow">01</div>
            <h2 id="section-health">系统状态</h2>
          </div>
          <span class="section-note">核心服务健康检查</span>
          <button class="icon-button" type="button" :disabled="refreshingHealth" aria-label="刷新系统状态" title="刷新" @click="refreshHealth">
            <RefreshCw :size="18" :class="{ spinning: refreshingHealth }" />
          </button>
        </div>
        <div class="status-band" :class="`status-${health.kind}`" aria-live="polite">
          <div class="status-icon">
            <Check v-if="health.kind === 'ready'" :size="28" />
            <AlertTriangle v-else-if="health.kind === 'degraded'" :size="28" />
            <WifiOff v-else-if="health.kind === 'unreachable' || health.kind === 'invalid'" :size="28" />
            <RefreshCw v-else :size="28" class="spinning" />
          </div>
          <div class="status-copy">
            <span class="status-label">当前状态</span>
            <strong>{{ healthTitle }}</strong>
            <p>{{ healthSummary }}</p>
          </div>
          <div class="dependency-pills">
            <span class="state-pill" :class="`pill-${healthCheck('postgres')}`"><Database :size="14" />PostgreSQL</span>
            <span class="state-pill" :class="`pill-${healthCheck('redis')}`"><Server :size="14" />Redis</span>
          </div>
        </div>
      </section>

      <!-- 2. 设备管理 -->
      <section class="console-section" aria-labelledby="section-devices">
        <div class="section-heading">
          <div>
            <div class="eyebrow">02</div>
            <h2 id="section-devices">设备管理</h2>
          </div>
          <span class="section-note">权威记录在 PostgreSQL，同步状态实时可见</span>
          <button class="icon-button" type="button" :disabled="loadingDevices" aria-label="刷新设备列表" title="刷新" @click="loadDevices">
            <RefreshCw :size="18" :class="{ spinning: loadingDevices }" />
          </button>
        </div>

        <form class="create-form" @submit.prevent="submitCreate">
          <div class="field">
            <label for="create-id">设备接入 ID（20 位数字）</label>
            <div class="input-with-action">
              <input id="create-id" v-model="createForm.device_access_id" inputmode="numeric" pattern="[0-9]{20}" required placeholder="34020000001320000001" />
              <button class="icon-button" type="button" aria-label="生成随机 20 位接入 ID" title="生成随机 ID" @click="fillGeneratedID"><ListPlus :size="18" /></button>
            </div>
          </div>
          <div class="field">
            <label for="create-realm">SIP Realm</label>
            <input id="create-realm" v-model="createForm.sip_realm" required placeholder="3402000000" />
          </div>
          <div class="field">
            <label for="create-password">密码（仅用于派生 HA1，不落库）</label>
            <input id="create-password" v-model="createForm.password" type="password" autocomplete="new-password" required placeholder="测试密码" />
          </div>
          <div class="field">
            <label class="checkbox-label">
              <input v-model="createForm.enabled" type="checkbox" />
              创建后立即启用
            </label>
          </div>
          <button class="primary-button" type="submit" :disabled="creating">
            <Plus :size="16" />{{ creating ? '创建中…' : '创建设备' }}
          </button>
          <p v-if="createError" class="form-error" role="alert">{{ createError }}</p>
          <p v-if="createSuccess" class="form-success" role="status">{{ createSuccess }}</p>
        </form>

        <p v-if="devicesError" class="form-error" role="alert">{{ devicesError }}</p>

        <div class="device-table-wrap">
          <table class="device-table">
            <thead>
              <tr>
                <th scope="col">接入 ID</th>
                <th scope="col">Realm</th>
                <th scope="col">状态</th>
                <th scope="col">同步</th>
                <th scope="col">运行时</th>
                <th scope="col">版本</th>
                <th scope="col" class="col-actions">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!loadingDevices && devices.length === 0">
                <td colspan="7" class="empty-row">还没有设备，用上方表单创建第一台。</td>
              </tr>
              <tr v-for="device in devices" :key="device.id">
                <td class="mono">{{ device.device_access_id }}</td>
                <td>{{ device.sip_realm }}</td>
                <td>
                  <span class="state-pill" :class="device.enabled ? 'pill-up' : 'pill-muted'">
                    <Play v-if="device.enabled" :size="14" /><Pause v-else :size="14" />
                    {{ device.enabled ? '启用' : '停用' }}
                  </span>
                </td>
                <td>
                  <span class="state-pill" :class="device.access_sync_status === 'synced' ? 'pill-up' : 'pill-unknown'">
                    {{ syncLabel(device) }}
                  </span>
                </td>
                <td>
                  <span class="state-pill" :class="device.runtime?.state === 'online' ? 'pill-up' : device.runtime?.state === 'offline' ? 'pill-down' : 'pill-unknown'">
                    {{ runtimeLabel(device) }}
                  </span>
                </td>
                <td class="mono">{{ device.profile_version }}</td>
                <td class="col-actions">
                  <div class="action-group">
                    <button class="icon-button small" type="button" :disabled="deviceBusy[device.id] === 'toggle'" :aria-label="device.enabled ? `停用 ${device.device_access_id}` : `启用 ${device.device_access_id}`" :title="device.enabled ? '停用' : '启用'" @click="toggleEnabled(device)">
                      <Pause v-if="device.enabled" :size="15" /><Play v-else :size="15" />
                    </button>
                    <button class="icon-button small" type="button" :disabled="deviceBusy[device.id] === 'register'" aria-label="模拟 SIP 注册" title="模拟注册（REGISTER + Digest）" @click="runSIPAction(device, 'register')"><KeyRound :size="15" /></button>
                    <button class="icon-button small" type="button" :disabled="deviceBusy[device.id] === 'keepalive'" aria-label="模拟心跳保活" title="模拟保活（MESSAGE Keepalive）" @click="runSIPAction(device, 'keepalive')"><Zap :size="15" /></button>
                    <button class="icon-button small" type="button" :disabled="deviceBusy[device.id] === 'unregister'" aria-label="模拟注销" title="模拟注销（Expires: 0）" @click="runSIPAction(device, 'unregister')"><LogOut :size="15" /></button>
                    <button class="icon-button small danger" type="button" :disabled="deviceBusy[device.id] === 'delete'" aria-label="删除设备" title="删除设备" @click="removeDevice(device)"><Trash2 :size="15" /></button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="test-log" aria-live="polite">
          <div class="test-log-heading">
            <div class="section-heading">
              <div>
                <div class="eyebrow">链路测试日志</div>
                <h3>最近 50 条</h3>
              </div>
            </div>
          </div>
          <ul v-if="testLog.length > 0" class="log-list">
            <li v-for="(entry, index) in testLog" :key="index" class="log-entry">
              <span class="log-time mono">{{ entry.at }}</span>
              <span class="log-device mono">{{ entry.device }}</span>
              <span class="log-action">{{ entry.action }}</span>
              <span class="log-result" :class="entry.ok ? 'log-ok' : 'log-fail'">{{ entry.ok ? '成功' : '失败' }}</span>
              <span class="log-text">{{ entry.text }}</span>
            </li>
          </ul>
          <p v-else class="log-empty">点击设备行的注册 / 保活 / 注销按钮，结果会显示在这里。</p>
        </div>
      </section>

      <!-- 3. Access 运行时 -->
      <section class="console-section" aria-labelledby="section-access">
        <div class="section-heading">
          <div>
            <div class="eyebrow">03</div>
            <h2 id="section-access">接入层运行时</h2>
          </div>
          <span class="section-note">直接查询 Kamailio 控制面（JSON-RPC 透传）</span>
          <button class="icon-button" type="button" :disabled="snapshotLoading" aria-label="刷新运行时快照" title="刷新快照" @click="refreshSnapshot">
            <RefreshCw :size="18" :class="{ spinning: snapshotLoading }" />
          </button>
        </div>

        <p v-if="snapshotError" class="form-error" role="alert">{{ snapshotError }}</p>

        <div class="snapshot-meta">
          <span class="mono">实例 {{ snapshot?.access_instance_id ?? '—' }}</span>
          <span class="mono">会话 {{ snapshot?.session_epoch ?? '—' }}</span>
          <span class="mono">最新序号 {{ snapshot?.latest_sequence ?? '—' }}</span>
          <span class="mono">快照时间 {{ snapshot ? formatTime(snapshot.snapshot_at) : '—' }}</span>
        </div>

        <div class="device-table-wrap">
          <table class="device-table">
            <thead>
              <tr>
                <th scope="col">设备 ID</th>
                <th scope="col">状态</th>
                <th scope="col">原因</th>
                <th scope="col">远端地址</th>
                <th scope="col">过期时间</th>
                <th scope="col">最后保活</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!snapshotLoading && (!snapshot || snapshot.registrations.length === 0)">
                <td colspan="6" class="empty-row">暂无注册设备。先创建并模拟注册一台。</td>
              </tr>
              <tr v-for="reg in snapshot?.registrations ?? []" :key="reg.device_access_id">
                <td class="mono">{{ reg.device_access_id }}</td>
                <td>
                  <span class="state-pill" :class="reg.state === 'online' ? 'pill-up' : 'pill-down'">{{ reg.state }}</span>
                </td>
                <td>{{ reg.reason || '—' }}</td>
                <td class="mono">{{ reg.remote_address || '—' }}</td>
                <td>{{ formatTime(reg.expires_at) }}</td>
                <td>{{ formatTime(reg.last_seen) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="events-panel">
          <div class="section-heading">
            <div>
              <div class="eyebrow">事件流</div>
              <h3>registration_changed</h3>
            </div>
            <span class="section-note">光标 {{ cursor }}</span>
            <button class="icon-button" type="button" :disabled="eventsLoading" aria-label="轮询新事件" title="轮询新事件" @click="refreshEvents">
              <RefreshCw :size="18" :class="{ spinning: eventsLoading }" />
            </button>
            <button class="text-button" type="button" :disabled="acking || cursor === 0" aria-label="确认已消费事件" title="确认已消费事件" @click="doAck">
              <Check :size="15" />ACK {{ cursor }}
            </button>
          </div>
          <p v-if="eventsError" class="form-error" role="alert">{{ eventsError }}</p>
          <ul v-if="events.length > 0" class="log-list">
            <li v-for="event in events" :key="event.event_id" class="log-entry">
              <span class="log-time mono">#{{ event.sequence }}</span>
              <span class="log-device mono">{{ event.device_access_id }}</span>
              <span class="log-action">{{ event.type }}</span>
              <span class="log-result" :class="event.payload.state === 'online' ? 'log-ok' : 'log-fail'">{{ event.payload.state }}</span>
              <span class="log-text">{{ event.payload.reason || '' }} {{ formatTime(event.occurred_at) }}</span>
            </li>
          </ul>
          <p v-else class="log-empty">还没有事件。模拟注册或注销后轮询。</p>
        </div>
      </section>
    </section>
  </main>
</template>
