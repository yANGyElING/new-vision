<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  AlertTriangle, Building2, Check, Edit3, Monitor, Network, Pause, Play,
  Plus, RefreshCw, ShieldCheck, Trash2, X,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { fetchHealth, type HealthState } from '@/api/health'
import { me } from '@/api/auth'
import {
  createRegion, createTenant, deleteRegion, listRegions, listTenants,
  renameRegion, setTenantStatus,
  type Region, type Tenant,
} from '@/api/identity'

// ---------- health band ----------
const health = ref<HealthState>({ kind: 'loading' })
const healthTitle = computed(() => {
  switch (health.value.kind) {
    case 'ready': return '系统正常'
    case 'degraded': return '部分依赖异常'
    case 'unreachable': return '服务不可达'
    case 'invalid': return '响应异常'
    default: return '检测中…'
  }
})
const healthTone = computed(() => (health.value.kind === 'ready' ? 'up' : health.value.kind === 'degraded' ? 'warn' : 'down'))

async function refreshHealth() {
  try {
    health.value = await fetchHealth(4000)
  } catch {
    /* keep last known state; band is informational only */
  }
}

// ---------- current principal ----------
const nodeAdmin = ref(false)
const accessDenied = ref(false)

// ---------- tabs ----------
type TabKey = 'tenants' | 'regions'
const tab = ref<TabKey>('tenants')
const TABS: { key: TabKey; label: string }[] = [
  { key: 'tenants', label: '租户' },
  { key: 'regions', label: '区域' },
]

// ---------- flash ----------
const flash = ref('')
function flashMessage(msg: string) {
  flash.value = msg
  window.setTimeout(() => { if (flash.value === msg) flash.value = '' }, 3000)
}

// ---------- tenants ----------
const tenants = ref<Tenant[]>([])
const tenantsLoading = ref(false)
const tenantsError = ref('')

async function loadTenants() {
  tenantsLoading.value = true
  tenantsError.value = ''
  try {
    tenants.value = await listTenants()
  } catch (error) {
    tenantsError.value = error instanceof Error ? error.message : '加载租户列表失败'
  } finally {
    tenantsLoading.value = false
  }
}

// tenant create modal
const tenantModalOpen = ref(false)
const tenantFormName = ref('')
const tenantCreating = ref(false)
const tenantError = ref('')

function openTenantModal() {
  tenantFormName.value = ''
  tenantError.value = ''
  tenantModalOpen.value = true
}

async function submitTenant() {
  const name = tenantFormName.value.trim()
  if (!name) { tenantError.value = '请输入租户名称'; return }
  tenantCreating.value = true
  tenantError.value = ''
  try {
    const tenant = await createTenant(name)
    tenantModalOpen.value = false
    flashMessage(`租户「${tenant.name}」已创建`)
    await loadTenants()
  } catch (error) {
    tenantError.value = error instanceof Error ? error.message : '创建失败'
  } finally {
    tenantCreating.value = false
  }
}

const tenantBusy = ref<Record<string, boolean>>({})

async function toggleTenant(tenant: Tenant) {
  tenantBusy.value[tenant.id] = true
  try {
    const next = tenant.status === 'active' ? 'disabled' : 'active'
    await setTenantStatus(tenant.id, next)
    flashMessage(next === 'active' ? `租户「${tenant.name}」已启用` : `租户「${tenant.name}」已停用`)
    await loadTenants()
  } catch (error) {
    flashMessage(error instanceof Error ? error.message : '操作失败')
  } finally {
    delete tenantBusy.value[tenant.id]
  }
}

// ---------- regions ----------
const regionTree = ref<Region[]>([])
const regionsLoading = ref(false)
const regionsError = ref('')

type FlatRegion = { region: Region; depth: number; path: string }

const flatRegions = computed<FlatRegion[]>(() => {
  const out: FlatRegion[] = []
  const walk = (nodes: Region[], depth: number, prefix: string) => {
    for (const n of nodes) {
      const path = prefix ? `${prefix} / ${n.name}` : n.name
      out.push({ region: n, depth, path })
      if (n.children?.length) walk(n.children, depth + 1, path)
    }
  }
  walk(regionTree.value, 0, '')
  return out
})

function regionPath(id: string): string {
  return flatRegions.value.find((f) => f.region.id === id)?.path ?? id
}

async function loadRegions() {
  regionsLoading.value = true
  regionsError.value = ''
  try {
    regionTree.value = await listRegions()
  } catch (error) {
    regionsError.value = error instanceof Error ? error.message : '加载区域列表失败'
  } finally {
    regionsLoading.value = false
  }
}

// region create modal
const regionModalOpen = ref(false)
const regionParentID = ref('')
const regionFormName = ref('')
const regionCreating = ref(false)
const regionError = ref('')

function openRegionModal(parentID = '') {
  regionParentID.value = parentID
  regionFormName.value = ''
  regionError.value = ''
  regionModalOpen.value = true
}

async function submitRegion() {
  const name = regionFormName.value.trim()
  if (!name) { regionError.value = '请输入区域名称'; return }
  regionCreating.value = true
  regionError.value = ''
  try {
    await createRegion(regionParentID.value, name)
    regionModalOpen.value = false
    flashMessage(`区域「${name}」已创建`)
    await loadRegions()
  } catch (error) {
    regionError.value = error instanceof Error ? error.message : '创建失败'
  } finally {
    regionCreating.value = false
  }
}

// region rename modal
const renamingRegion = ref<FlatRegion | null>(null)
const renameFormName = ref('')
const renaming = ref(false)
const renameError = ref('')

function openRenameRegion(flat: FlatRegion) {
  renamingRegion.value = flat
  renameFormName.value = flat.region.name
  renameError.value = ''
}

async function submitRenameRegion() {
  if (!renamingRegion.value) return
  const name = renameFormName.value.trim()
  if (!name) { renameError.value = '请输入区域名称'; return }
  renaming.value = true
  renameError.value = ''
  try {
    await renameRegion(renamingRegion.value.region.id, name)
    renamingRegion.value = null
    flashMessage('区域名称已更新')
    await loadRegions()
  } catch (error) {
    renameError.value = error instanceof Error ? error.message : '重命名失败'
  } finally {
    renaming.value = false
  }
}

const regionBusy = ref<Record<string, boolean>>({})

async function removeRegion(flat: FlatRegion) {
  const hasChildren = (flat.region.children?.length ?? 0) > 0
  const message = hasChildren
    ? `区域「${flat.region.name}」包含子区域，需先删除子区域。确定继续删除？`
    : `确定删除区域「${flat.region.name}」？若仍有用户或设备引用该区域，删除将失败。`
  if (!window.confirm(message)) return
  regionBusy.value[flat.region.id] = true
  try {
    await deleteRegion(flat.region.id)
    flashMessage(`区域「${flat.region.name}」已删除`)
    await loadRegions()
  } catch (error) {
    flashMessage(error instanceof Error ? error.message : '删除失败')
  } finally {
    delete regionBusy.value[flat.region.id]
  }
}

// ---------- stats ----------
const stats = computed(() => {
  const active = tenants.value.filter((t) => t.status === 'active').length
  return {
    tenants: tenants.value.length,
    active,
    disabled: tenants.value.length - active,
    regions: flatRegions.value.length,
  }
})

const headAction = computed(() => {
  switch (tab.value) {
    case 'regions': return { label: '新增根区域', icon: Network, run: () => openRegionModal('') }
    default: return { label: '新增租户', icon: Building2, run: openTenantModal }
  }
})

// ---------- formatting ----------
function formatDate(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// ---------- boot ----------
let healthTimer = 0

onMounted(() => {
  void refreshHealth()
  healthTimer = window.setInterval(refreshHealth, 30000)
  void (async () => {
    try {
      const info = await me()
      nodeAdmin.value = (info.roles ?? []).includes('node_admin')
      if (!nodeAdmin.value) {
        accessDenied.value = true
        return
      }
      await Promise.all([loadTenants(), loadRegions()])
    } catch (error) {
      tenantsError.value = error instanceof Error ? error.message : '初始化失败'
    }
  })()
})

onUnmounted(() => {
  window.clearInterval(healthTimer)
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
        <RouterLink to="/users" class="prod-nav-link">用户管理</RouterLink>
        <RouterLink to="/identity" class="prod-nav-link active" aria-current="page">组织架构</RouterLink>
        <RouterLink to="/devices" class="prod-nav-link">设备管理</RouterLink>
        <RouterLink to="/" class="prod-nav-link">测试控制台</RouterLink>
      </nav>
      <div class="prod-topbar-right">
        <span class="prod-health" :class="`health-${healthTone}`" :title="healthTitle">
          <span class="prod-health-dot" /><span>{{ healthTitle }}</span>
        </span>
      </div>
    </header>

    <!-- glass sticky title bar -->
    <header class="prod-head">
      <div class="prod-head-inner">
        <h1>组织架构</h1>
        <button v-if="nodeAdmin" class="prod-button prod-button-primary" type="button" @click="headAction.run()">
          <component :is="headAction.icon" :size="16" :stroke-width="2.2" />{{ headAction.label }}
        </button>
      </div>
    </header>

    <main class="prod-main">

      <Transition name="toast">
        <div v-if="flash" class="prod-flash" role="status">
          <Check :size="15" />{{ flash }}
        </div>
      </Transition>

      <!-- access denied -->
      <div v-if="accessDenied" class="prod-empty denied" role="alert">
        <span class="prod-empty-icon"><ShieldCheck :size="26" /></span>
        <strong>需要节点管理员权限</strong>
        <p>组织架构管理仅对 node_admin 角色开放，请联系管理员调整你的角色。</p>
        <RouterLink class="prod-button" to="/devices">前往设备管理</RouterLink>
      </div>

      <template v-else>
        <!-- stats cards -->
        <div class="prod-stats" aria-label="组织数据统计">
          <div class="prod-stat">
            <span class="prod-stat-label"><Building2 :size="18" :stroke-width="2" />租户总数</span>
            <strong class="prod-stat-value">{{ stats.tenants }}</strong>
          </div>
          <div class="prod-stat">
            <span class="prod-stat-label"><span class="stat-dot dot-online" />启用租户</span>
            <strong class="prod-stat-value">{{ stats.active }}</strong>
          </div>
          <div class="prod-stat">
            <span class="prod-stat-label"><span class="stat-dot dot-offline" />停用租户</span>
            <strong class="prod-stat-value">{{ stats.disabled }}</strong>
          </div>
          <div class="prod-stat">
            <span class="prod-stat-label"><Network :size="18" :stroke-width="2" />区域节点</span>
            <strong class="prod-stat-value">{{ stats.regions }}</strong>
          </div>
        </div>

        <!-- tabs -->
        <div class="prod-tabs" role="tablist" aria-label="组织数据视图">
          <button
            v-for="t in TABS" :key="t.key" type="button" class="prod-tab"
            :class="{ active: tab === t.key }" role="tab" :aria-selected="tab === t.key"
            @click="tab = t.key"
          >{{ t.label }}</button>
        </div>

        <!-- ============ tenants tab ============ -->
        <section v-if="tab === 'tenants'" aria-label="租户管理">
          <div class="prod-toolbar-card">
            <span class="prod-toolbar-hint">租户是权限的顶层隔离域；停用租户后其下用户将无法登录。</span>
            <button class="prod-refresh" type="button" :disabled="tenantsLoading" aria-label="刷新租户列表" title="刷新" @click="loadTenants">
              <RefreshCw :size="16" :class="{ spinning: tenantsLoading }" />
            </button>
          </div>

          <div v-if="tenantsError" class="prod-error" role="alert">
            <AlertTriangle :size="18" />
            <div>
              <strong>加载失败</strong>
              <p>{{ tenantsError }}</p>
            </div>
            <button class="prod-button" type="button" @click="loadTenants"><RefreshCw :size="14" />重试</button>
          </div>

          <div v-else-if="tenantsLoading" class="prod-table-card" aria-label="加载中">
            <div class="prod-thead" role="row">
              <div role="columnheader">租户</div>
              <div role="columnheader">状态</div>
              <div role="columnheader">创建时间</div>
              <div role="columnheader" class="th-actions">操作</div>
            </div>
            <div v-for="n in 4" :key="n" class="prod-trow sk-row" role="row">
              <div role="cell"><span class="sk sk-name" /></div>
              <div role="cell"><span class="sk sk-pill" /></div>
              <div role="cell"><span class="sk sk-id" /></div>
              <div role="cell"><span class="sk sk-actions" /></div>
            </div>
          </div>

          <div v-else-if="tenants.length > 0" class="prod-table-card">
            <div class="prod-thead" role="row">
              <div role="columnheader">租户</div>
              <div role="columnheader">状态</div>
              <div role="columnheader">创建时间</div>
              <div role="columnheader" class="th-actions">操作</div>
            </div>
            <div v-for="tenant in tenants" :key="tenant.id" class="prod-trow" role="row">
              <div role="cell" class="prod-name-cell">
                <div class="prod-name prod-tenant-name">
                  <Building2 :size="14" class="prod-tenant-icon" />{{ tenant.name }}
                </div>
                <div class="prod-name-sub mono">{{ tenant.id }}</div>
              </div>
              <div role="cell">
                <span class="prod-pill" :class="tenant.status === 'active' ? 'pill-enable' : 'pill-muted'">
                  <span class="pill-dot" :class="tenant.status === 'active' ? 'dot-online' : 'dot-offline'" />
                  {{ tenant.status === 'active' ? '启用' : '停用' }}
                </span>
              </div>
              <div role="cell" class="prod-time mono">{{ formatDate(tenant.created_at) }}</div>
              <div role="cell" class="td-actions">
                <div class="prod-actions">
                  <button
                    class="prod-act" type="button"
                    :disabled="tenantBusy[tenant.id]"
                    :title="tenant.status === 'active' ? '停用租户' : '启用租户'"
                    :aria-label="tenant.status === 'active' ? '停用租户' : '启用租户'"
                    @click="toggleTenant(tenant)"
                  >
                    <Pause v-if="tenant.status === 'active'" :size="15" /><Play v-else :size="15" />
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div v-else class="prod-empty" role="status">
            <span class="prod-empty-icon"><Building2 :size="26" /></span>
            <strong>没有租户</strong>
            <p>点击「新增租户」创建第一个租户。</p>
            <button class="prod-button prod-button-primary" type="button" @click="openTenantModal"><Plus :size="15" />新增租户</button>
          </div>
        </section>

        <!-- ============ regions tab ============ -->
        <section v-else aria-label="区域管理">
          <div class="prod-toolbar-card">
            <span class="prod-toolbar-hint">区域是树形数据范围；给用户分配父区域即可覆盖其整个子树。</span>
            <button class="prod-refresh" type="button" :disabled="regionsLoading" aria-label="刷新区域列表" title="刷新" @click="loadRegions">
              <RefreshCw :size="16" :class="{ spinning: regionsLoading }" />
            </button>
          </div>

          <div v-if="regionsError" class="prod-error" role="alert">
            <AlertTriangle :size="18" />
            <div>
              <strong>加载失败</strong>
              <p>{{ regionsError }}</p>
            </div>
            <button class="prod-button" type="button" @click="loadRegions"><RefreshCw :size="14" />重试</button>
          </div>

          <div v-else-if="regionsLoading" class="prod-table-card" aria-label="加载中">
            <div class="prod-thead" role="row">
              <div role="columnheader">区域</div>
              <div role="columnheader">创建时间</div>
              <div role="columnheader" class="th-actions">操作</div>
            </div>
            <div v-for="n in 5" :key="n" class="prod-trow sk-row" role="row">
              <div role="cell"><span class="sk sk-name" /></div>
              <div role="cell"><span class="sk sk-id" /></div>
              <div role="cell"><span class="sk sk-actions" /></div>
            </div>
          </div>

          <div v-else-if="flatRegions.length > 0" class="prod-table-card">
            <div class="prod-thead" role="row">
              <div role="columnheader">区域</div>
              <div role="columnheader">创建时间</div>
              <div role="columnheader" class="th-actions">操作</div>
            </div>
            <div v-for="flat in flatRegions" :key="flat.region.id" class="prod-trow" role="row">
              <div role="cell">
                <div class="prod-region-cell" :style="{ paddingLeft: `${flat.depth * 22 + 2}px` }">
                  <Network :size="14" class="prod-region-icon" />
                  <span class="prod-region-name">{{ flat.region.name }}</span>
                  <span class="prod-region-path">{{ flat.path }}</span>
                </div>
              </div>
              <div role="cell" class="prod-time mono">{{ formatDate(flat.region.created_at) }}</div>
              <div role="cell" class="td-actions">
                <div class="prod-actions">
                  <button class="prod-act" type="button" title="添加子区域" aria-label="添加子区域" @click="openRegionModal(flat.region.id)"><Plus :size="15" /></button>
                  <button class="prod-act" type="button" title="重命名" aria-label="重命名" @click="openRenameRegion(flat)"><Edit3 :size="15" /></button>
                  <button class="prod-act danger" type="button" :disabled="regionBusy[flat.region.id]" title="删除" aria-label="删除" @click="removeRegion(flat)"><Trash2 :size="15" /></button>
                </div>
              </div>
            </div>
          </div>

          <div v-else class="prod-empty" role="status">
            <span class="prod-empty-icon"><Network :size="26" /></span>
            <strong>还没有区域</strong>
            <p>点击「新增根区域」创建第一个区域节点。</p>
            <button class="prod-button prod-button-primary" type="button" @click="openRegionModal('')"><Plus :size="15" />新增根区域</button>
          </div>
        </section>
      </template>
    </main>

    <!-- ============ tenant create modal ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="tenantModalOpen" class="prod-overlay" @click.self="tenantModalOpen = false">
          <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" aria-label="新增租户">
            <div class="prod-modal-head">
              <h2>新增租户</h2>
              <button class="prod-icon" type="button" aria-label="关闭" @click="tenantModalOpen = false"><X :size="16" /></button>
            </div>
            <form class="prod-form" @submit.prevent="submitTenant">
              <div class="prod-field">
                <label for="idm-tenant-name">租户名称</label>
                <input id="idm-tenant-name" v-model="tenantFormName" required maxlength="255" placeholder="如：华东运营中心" />
                <span class="prod-meta-hint">租户名全局唯一，登录时需要填写。</span>
              </div>
              <p v-if="tenantError" class="prod-error" role="alert">{{ tenantError }}</p>
              <div class="prod-modal-actions">
                <button class="prod-button" type="button" @click="tenantModalOpen = false">取消</button>
                <button class="prod-button prod-button-primary" type="submit" :disabled="tenantCreating">
                  <Plus :size="16" />{{ tenantCreating ? '创建中…' : '创建' }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============ region create modal ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="regionModalOpen" class="prod-overlay" @click.self="regionModalOpen = false">
          <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" aria-label="新增区域">
            <div class="prod-modal-head">
              <h2>{{ regionParentID ? '新增子区域' : '新增根区域' }}</h2>
              <button class="prod-icon" type="button" aria-label="关闭" @click="regionModalOpen = false"><X :size="16" /></button>
            </div>
            <form class="prod-form" @submit.prevent="submitRegion">
              <div class="prod-field">
                <label for="idm-region-name">区域名称</label>
                <input id="idm-region-name" v-model="regionFormName" required maxlength="255" placeholder="如：杭州仓" />
                <span class="prod-meta-hint">同级区域名称不可重复{{ regionParentID ? `，将创建在「${regionPath(regionParentID)}」之下` : '，将创建为根区域' }}。</span>
              </div>
              <p v-if="regionError" class="prod-error" role="alert">{{ regionError }}</p>
              <div class="prod-modal-actions">
                <button class="prod-button" type="button" @click="regionModalOpen = false">取消</button>
                <button class="prod-button prod-button-primary" type="submit" :disabled="regionCreating">
                  <Plus :size="16" />{{ regionCreating ? '创建中…' : '创建' }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============ region rename modal ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="renamingRegion" class="prod-overlay" @click.self="renamingRegion = null">
          <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" aria-label="重命名区域">
            <div class="prod-modal-head">
              <h2>重命名区域</h2>
              <button class="prod-icon" type="button" aria-label="关闭" @click="renamingRegion = null"><X :size="16" /></button>
            </div>
            <form class="prod-form" @submit.prevent="submitRenameRegion">
              <div class="prod-field">
                <label for="idm-region-rename">区域名称</label>
                <input id="idm-region-rename" v-model="renameFormName" required maxlength="255" />
                <span class="prod-meta-hint">当前路径：{{ renamingRegion.path }}</span>
              </div>
              <p v-if="renameError" class="prod-error" role="alert">{{ renameError }}</p>
              <div class="prod-modal-actions">
                <button class="prod-button" type="button" @click="renamingRegion = null">取消</button>
                <button class="prod-button prod-button-primary" type="submit" :disabled="renaming">
                  <Check :size="16" />{{ renaming ? '保存中…' : '保存' }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.prod-shell { min-height: 100vh; background: #F2F2F7; color: #1C1C1E; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; }
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
/* ---------- main ---------- */
.prod-main { max-width: 1200px; margin: 0 auto; padding: 0 32px 64px; }

/* ---------- glass sticky title bar ---------- */
.prod-head { position: sticky; top: 62px; z-index: 10; background: rgba(255,255,255,0.85); -webkit-backdrop-filter: blur(20px); backdrop-filter: blur(20px); border-bottom: 1px solid rgba(0,0,0,0.05); }
.prod-head-inner { max-width: 1200px; margin: 0 auto; padding: 20px 32px; display: flex; justify-content: space-between; align-items: flex-start; gap: 18px; }
.prod-head h1 { margin: 0; font-size: 28px; font-weight: 700; color: #1C1C1E; letter-spacing: -0.8px; }
/* ---------- toast ---------- */
.prod-flash { display: inline-flex; align-items: center; gap: 8px; margin-top: 18px; padding: 10px 15px; color: #1d714e; background: #ecf7f1; border: 1px solid #c5e6d5; border-radius: 9px; font-size: 13px; font-weight: 600; box-shadow: 0 4px 14px rgba(29,113,78,.08); }
.toast-enter-active, .toast-leave-active { transition: opacity .2s, transform .2s; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(-6px); }
/* ---------- metrics ---------- */
.prod-stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 28px; }
.prod-stat { display: flex; flex-direction: column; gap: 12px; padding: 20px 24px; background: #fff; border: 1px solid rgba(0,0,0,0.03); border-radius: 16px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); }
.prod-stat-label { display: inline-flex; align-items: center; gap: 8px; color: #8E8E93; font-size: 13px; font-weight: 500; }
.stat-dot { width: 8px; height: 8px; border-radius: 50%; }
.dot-online { background: #34C759; box-shadow: 0 0 0 3px rgba(52,199,89,0.15); }
.dot-offline { background: #C7C7CC; }
.prod-stat-value { font-size: 32px; font-weight: 700; letter-spacing: -1px; line-height: 1; color: #1C1C1E; font-variant-numeric: tabular-nums; }
/* ---------- tabs ---------- */
.prod-tabs { display: inline-flex; gap: 6px; margin-bottom: 16px; padding: 4px; background: #F2F2F7; border-radius: 12px; }
.prod-tab { padding: 8px 18px; color: #636366; background: none; border: 0; border-radius: 8px; font: inherit; font-size: 13px; font-weight: 600; cursor: pointer; transition: color .15s, background .15s; }
.prod-tab:hover { color: #1C1C1E; background: #E5E5EA; }
.prod-tab.active { color: #fff; background: #1C1C1E; }
.prod-tab:focus-visible { outline: 2px solid #007AFF; outline-offset: 1px; }
/* ---------- toolbar card ---------- */
.prod-toolbar-card { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; margin-bottom: 16px; padding: 16px 20px; background: #fff; border: 1px solid rgba(0,0,0,0.03); border-radius: 16px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); }
.prod-toolbar-hint { flex: 1; color: #8E8E93; font-size: 12.5px; }
.prod-refresh { display: inline-flex; align-items: center; justify-content: center; width: 36px; height: 36px; flex-shrink: 0; color: #636366; background: #fff; border: 1.5px solid #E5E5EA; border-radius: 10px; cursor: pointer; transition: background .15s, border-color .15s, color .15s; }
.prod-refresh:hover:not(:disabled) { background: #F2F2F7; border-color: #C7C7CC; }
.prod-refresh:disabled { cursor: wait; opacity: .5; }
.prod-refresh:focus-visible { outline: 2px solid #007AFF; outline-offset: 2px; }
/* ---------- buttons ---------- */
.prod-button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; padding: 10px 20px; color: #1C1C1E; background: #fff; border: 1px solid #E5E5EA; border-radius: 999px; font-size: 14px; font-weight: 600; text-decoration: none; cursor: pointer; transition: border-color .15s, background .15s, transform .05s; }
.prod-button:hover:not(:disabled) { border-color: #C7C7CC; }
.prod-button:active:not(:disabled) { transform: scale(.985); }
.prod-button:disabled { cursor: wait; opacity: .6; }
.prod-button-primary { color: #fff; background: #1C1C1E; border-color: #1C1C1E; box-shadow: 0 2px 8px rgba(0,0,0,0.12); }
.prod-button-primary:hover:not(:disabled) { background: #2c2c2e; border-color: #2c2c2e; }
.prod-button:focus-visible { outline: 2px solid #007AFF; outline-offset: 2px; }
.prod-icon { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; color: #5f6873; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; cursor: pointer; transition: color .15s, border-color .15s, background .15s; }
.prod-icon:hover:not(:disabled) { color: #1a1f26; border-color: #aeb7c1; }
.prod-icon.danger:hover:not(:disabled) { color: #b44444; border-color: #e9c1c1; background: #fdf6f6; }
.prod-icon:disabled { cursor: not-allowed; opacity: .45; }
/* ---------- table ---------- */
.prod-table-card { margin-bottom: 16px; overflow: hidden; background: #fff; border: 1px solid rgba(0,0,0,0.03); border-radius: 16px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); }
.prod-thead, .prod-trow { display: grid; align-items: center; }
.prod-trow { grid-template-columns: 2fr 1fr 1.5fr 100px; }
.prod-thead { grid-template-columns: 2fr 1fr 1.5fr 100px; }
.prod-thead { padding: 14px 24px; background: #FAFAFA; border-bottom: 1px solid #F2F2F7; font-size: 12px; font-weight: 600; color: #8E8E93; text-transform: uppercase; letter-spacing: 0.5px; }
.prod-thead .th-actions { text-align: right; }
.prod-trow { padding: 16px 24px; border-bottom: 1px solid #F2F2F7; background: #fff; transition: background 0.15s; }
.prod-trow:last-child { border-bottom: 0; }
.prod-trow:hover { background: #FAFAFA; }
.prod-trow:first-of-type { border-top: 0; }
.prod-name-cell { min-width: 0; }
.prod-name { font-weight: 600; color: #1C1C1E; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-name-sub { margin-top: 3px; color: #8E8E93; font-size: 11px; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-time { color: #8E8E93; font-size: 12px; white-space: nowrap; }
.prod-pill { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; white-space: nowrap; }
.pill-dot { width: 6px; height: 6px; border-radius: 50%; }
.pill-enable { color: #1d714e; background: #E9F6EF; }
.pill-muted { color: #65707b; background: #F0F2F4; }
.dot-online { background: #34C759; }
.dot-offline { background: #C7C7CC; }
.td-actions { text-align: right; }
.prod-actions { display: inline-flex; gap: 4px; opacity: 0; transition: opacity .15s; }
.prod-trow:hover .prod-actions, .prod-actions:focus-within { opacity: 1; }
.prod-act { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; color: #8E8E93; background: transparent; border: 0; border-radius: 8px; cursor: pointer; transition: all 0.15s; }
.prod-act:hover:not(:disabled) { background: #F2F2F7; color: #1C1C1E; }
.prod-act.danger:hover:not(:disabled) { background: #FFF2F2; color: #FF3B30; }
.prod-act:disabled { cursor: not-allowed; opacity: .4; }
.prod-act:focus-visible { outline: 2px solid #007AFF; outline-offset: 1px; }
.prod-tenant-name { display: inline-flex; align-items: center; gap: 7px; }
.prod-tenant-icon { color: #C7C7CC; flex-shrink: 0; }
.prod-region-cell { display: flex; align-items: center; gap: 8px; min-width: 260px; }
.prod-region-icon { color: #C7C7CC; flex-shrink: 0; }
.prod-region-name { font-weight: 600; color: #1C1C1E; white-space: nowrap; }
.prod-region-path { color: #8E8E93; font-size: 11.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* ---------- skeleton ---------- */
.sk-row { padding: 16px 24px; }
.sk { display: inline-block; background: linear-gradient(90deg, #F2F2F7 25%, #FAFAFA 37%, #F2F2F7 63%); background-size: 400% 100%; animation: sk-shimmer 1.3s ease infinite; border-radius: 6px; }
.sk-name { width: 120px; height: 14px; }
.sk-id { width: 150px; height: 12px; }
.sk-pill { width: 56px; height: 20px; border-radius: 999px; }
.sk-actions { width: 130px; height: 28px; }
@keyframes sk-shimmer { 0% { background-position: 100% 0; } 100% { background-position: -100% 0; } }
/* ---------- error ---------- */
.prod-error { display: flex; align-items: center; gap: 13px; margin-bottom: 16px; padding: 15px 18px; color: #a14444; background: #fdf2f2; border: 1px solid #f2d6d6; border-radius: 16px; }
.prod-error svg { flex-shrink: 0; }
.prod-error strong { font-size: 13.5px; }
.prod-error p { margin: 3px 0 0; color: #b06565; font-size: 12.5px; }
.prod-error .prod-button { margin-left: auto; color: #a14444; border-color: #e9c1c1; }
.prod-form .prod-error { margin-top: 14px; }
/* ---------- empty ---------- */
.prod-empty { display: flex; flex-direction: column; align-items: center; gap: 6px; margin-bottom: 16px; padding: 52px 20px; text-align: center; background: #fff; border: 1px solid rgba(0,0,0,0.03); border-radius: 16px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); }
.prod-empty-icon { display: inline-flex; align-items: center; justify-content: center; width: 52px; height: 52px; margin-bottom: 6px; color: #8E8E93; background: #F2F2F7; border-radius: 999px; }
.prod-empty strong { font-size: 14.5px; color: #1C1C1E; }
.prod-empty p { margin: 0 0 10px; color: #8E8E93; font-size: 13px; }
.prod-empty.denied { margin-top: 24px; }
/* ---------- overlay / modal ---------- */
.prod-overlay { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(0,0,0,.4); backdrop-filter: blur(4px); -webkit-backdrop-filter: blur(4px); }
.prod-modal { width: 100%; max-width: 560px; max-height: 88vh; overflow-y: auto; background: #fff; border-radius: 16px; box-shadow: 0 24px 64px rgba(0,0,0,.28); }
.prod-modal-narrow { max-width: 480px; }
.prod-modal-head { display: flex; align-items: center; justify-content: space-between; padding: 19px 22px; border-bottom: 1px solid #eef0f3; position: sticky; top: 0; background: #fff; z-index: 2; }
.prod-modal-head h2 { margin: 0; font-size: 16.5px; display: flex; align-items: center; gap: 10px; }
.prod-form { padding: 20px 22px; display: grid; gap: 14px; }
.prod-field { display: grid; gap: 6px; }
.prod-field label { font-size: 12px; font-weight: 700; color: #5f6873; }
.prod-field input { width: 100%; padding: 10px 11px; font: inherit; font-size: 13.5px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 8px; box-sizing: border-box; transition: border-color .15s, box-shadow .15s; }
.prod-field input:focus-visible { outline: none; border-color: #1a1f26; box-shadow: 0 0 0 3px rgba(26,31,38,.1); }
.prod-meta-hint { margin: 0; color: #8a939d; font-size: 12px; }
.prod-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }
/* ---------- modal transitions ---------- */
.modal-enter-active, .modal-leave-active { transition: opacity .18s; }
.modal-enter-active .prod-modal, .modal-leave-active .prod-modal { transition: transform .18s cubic-bezier(.2,.8,.3,1), opacity .18s; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-from .prod-modal, .modal-leave-to .prod-modal { transform: translateY(14px) scale(.98); opacity: 0; }
.mono { font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 12px; font-variant-numeric: tabular-nums; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* ---------- reduced motion ---------- */
@media (prefers-reduced-motion: reduce) {
  .sk, .spinning { animation: none; }
  .prod-trow, .prod-button, .prod-act, .prod-icon, .prod-nav-link, .prod-tab, .prod-refresh, .prod-pill, .prod-field input { transition: none; }
  .modal-enter-active, .modal-leave-active, .toast-enter-active, .toast-leave-active { transition: none; }
  .modal-enter-from, .modal-leave-to, .toast-enter-from, .toast-leave-to { opacity: 1; transform: none; }
}
/* ---------- responsive ---------- */
@media (max-width: 900px) {
  .prod-topbar { gap: 14px; padding: 0 16px; }
  .prod-brand-text span { display: none; }
  .prod-main { padding: 0 16px 48px; }
  .prod-head-inner { padding: 16px 16px; }
  .prod-stats { grid-template-columns: repeat(2, 1fr); gap: 12px; }
  .prod-toolbar-card { gap: 12px; }
  .prod-table-card { overflow-x: auto; }
  .prod-thead, .prod-trow { min-width: 640px; }
}
@media (max-width: 560px) {
  .prod-head-inner { flex-direction: column; align-items: stretch; }
  .prod-head .prod-button-primary { align-self: flex-start; }
}
</style>
