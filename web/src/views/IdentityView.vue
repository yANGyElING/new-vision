<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  AlertTriangle, Building2, Check, ChevronLeft, ChevronRight, Edit3, Eye, EyeOff,
  KeyRound, Monitor, Network, Pause, Play, Plus, RefreshCw, Search, ShieldCheck,
  Trash2, UserPlus, Users, X,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { fetchHealth, type HealthState } from '@/api/health'
import { me } from '@/api/auth'
import {
  ROLE_META, createUser, createRegion, createTenant, deleteUser, deleteRegion,
  listRegions, listRoles, listTenants, listUsers, renameRegion, setTenantStatus,
  setUserPassword, updateUser,
  type IdentityUser, type Region, type Tenant,
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
const currentUser = ref<{ id: string; tenantID: string } | null>(null)
const nodeAdmin = ref(false)
const accessDenied = ref(false)

// ---------- tabs ----------
type TabKey = 'users' | 'tenants' | 'regions'
const tab = ref<TabKey>('users')
const TABS: { key: TabKey; label: string }[] = [
  { key: 'users', label: '用户' },
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
const tenantFilter = ref('')
const tenantNameByID = computed(() => {
  const map = new Map<string, string>()
  for (const t of tenants.value) map.set(t.id, t.name)
  return map
})

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

const regionNameByID = computed(() => {
  const map = new Map<string, string>()
  for (const f of flatRegions.value) map.set(f.region.id, f.path)
  return map
})

function regionPath(id: string): string {
  return regionNameByID.value.get(id) ?? id
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

// ---------- users ----------
const users = ref<IdentityUser[]>([])
const usersLoading = ref(false)
const usersError = ref('')
const userSearch = ref('')
const userPage = ref(1)
const userPageSize = 10
const userBusy = ref<Record<string, string>>({})

async function loadUsers() {
  usersLoading.value = true
  usersError.value = ''
  try {
    users.value = await listUsers(tenantFilter.value || undefined)
  } catch (error) {
    usersError.value = error instanceof Error ? error.message : '加载用户列表失败'
  } finally {
    usersLoading.value = false
  }
}

function resetUserPage() { userPage.value = 1 }

const filteredUsers = computed(() => {
  const q = userSearch.value.trim().toLowerCase()
  let list = users.value
  if (q) {
    list = list.filter((u) =>
      u.username.toLowerCase().includes(q)
      || u.display_name.toLowerCase().includes(q)
      || u.roles.some((r) => r.toLowerCase().includes(q)))
  }
  return [...list].sort((a, b) => a.username.localeCompare(b.username))
})

const userTotalPages = computed(() => Math.max(1, Math.ceil(filteredUsers.value.length / userPageSize)))
const pageUsers = computed(() => filteredUsers.value.slice((userPage.value - 1) * userPageSize, userPage.value * userPageSize))

// ---------- roles ----------
const availableRoles = ref<string[]>([])

async function loadRoles() {
  try {
    const result = await listRoles()
    availableRoles.value = result.roles
  } catch {
    availableRoles.value = Object.keys(ROLE_META)
  }
}

function roleLabel(role: string): string {
  return ROLE_META[role]?.label ?? role
}
function roleHint(role: string): string {
  return ROLE_META[role]?.hint ?? role
}

// ---------- user create / edit modal ----------
const userModalOpen = ref(false)
const userModalMode = ref<'create' | 'edit'>('create')
const userSaving = ref(false)
const userFormError = ref('')
const userForm = ref<{
  id: string; tenant_id: string; username: string; display_name: string
  password: string; status: 'active' | 'disabled'; roles: string[]; region_ids: string[]
}>({
  id: '', tenant_id: '', username: '', display_name: '',
  password: '', status: 'active', roles: [], region_ids: [],
})
const passwordVisible = ref(false)

function openUserCreate() {
  userModalMode.value = 'create'
  userForm.value = {
    id: '', tenant_id: currentUser.value?.tenantID ?? '', username: '', display_name: '',
    password: '', status: 'active', roles: [], region_ids: [],
  }
  passwordVisible.value = false
  userFormError.value = ''
  userModalOpen.value = true
}

function openUserEdit(user: IdentityUser) {
  userModalMode.value = 'edit'
  userForm.value = {
    id: user.id,
    tenant_id: user.tenant_id,
    username: user.username,
    display_name: user.display_name,
    password: '',
    status: user.status,
    roles: [...user.roles],
    region_ids: [...user.region_ids],
  }
  passwordVisible.value = false
  userFormError.value = ''
  userModalOpen.value = true
}

function closeUserModal() {
  userModalOpen.value = false
  userFormError.value = ''
}

function toggleRole(role: string) {
  const roles = userForm.value.roles
  const idx = roles.indexOf(role)
  if (idx >= 0) roles.splice(idx, 1)
  else roles.push(role)
}

function toggleRegionScope(id: string) {
  const ids = userForm.value.region_ids
  const idx = ids.indexOf(id)
  if (idx >= 0) ids.splice(idx, 1)
  else ids.push(id)
}

function generatePassword() {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
  const bytes = new Uint32Array(16)
  crypto.getRandomValues(bytes)
  userForm.value.password = Array.from(bytes, (b) => chars[b % chars.length]).join('')
  passwordVisible.value = true
}

async function submitUserForm() {
  userFormError.value = ''
  const username = userForm.value.username.trim()
  const displayName = userForm.value.display_name.trim()
  if (userModalMode.value === 'create') {
    if (!username) { userFormError.value = '请输入用户名'; return }
    if (!userForm.value.password) { userFormError.value = '请设置初始密码'; return }
  }
  userSaving.value = true
  try {
    if (userModalMode.value === 'create') {
      await createUser({
        tenant_id: userForm.value.tenant_id || undefined,
        username,
        password: userForm.value.password,
        display_name: displayName,
        roles: userForm.value.roles,
        region_ids: userForm.value.region_ids,
      })
      flashMessage(`用户 ${username} 已创建`)
    } else {
      await updateUser(userForm.value.id, {
        display_name: displayName,
        status: userForm.value.status,
        roles: userForm.value.roles,
        region_ids: userForm.value.region_ids,
      })
      flashMessage(`用户 ${username} 已更新`)
    }
    userModalOpen.value = false
    await loadUsers()
  } catch (error) {
    userFormError.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    userSaving.value = false
  }
}

// ---------- user password modal ----------
const passwordModalUser = ref<IdentityUser | null>(null)
const newPassword = ref('')
const newPasswordVisible = ref(false)
const passwordSaving = ref(false)
const passwordError = ref('')

function openPasswordModal(user: IdentityUser) {
  passwordModalUser.value = user
  newPassword.value = ''
  newPasswordVisible.value = false
  passwordError.value = ''
}

function generateAdminPassword() {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
  const bytes = new Uint32Array(16)
  crypto.getRandomValues(bytes)
  newPassword.value = Array.from(bytes, (b) => chars[b % chars.length]).join('')
  newPasswordVisible.value = true
}

async function submitPassword() {
  if (!passwordModalUser.value) return
  if (!newPassword.value) { passwordError.value = '请输入新密码'; return }
  passwordSaving.value = true
  passwordError.value = ''
  try {
    await setUserPassword(passwordModalUser.value.id, newPassword.value)
    flashMessage(`用户 ${passwordModalUser.value.username} 的密码已重置`)
    passwordModalUser.value = null
  } catch (error) {
    passwordError.value = error instanceof Error ? error.message : '重置失败'
  } finally {
    passwordSaving.value = false
  }
}

// ---------- user actions ----------
async function toggleUserStatus(user: IdentityUser) {
  userBusy.value[user.id] = 'status'
  try {
    const next = user.status === 'active' ? 'disabled' : 'active'
    await updateUser(user.id, { status: next })
    flashMessage(next === 'active' ? `用户 ${user.username} 已启用` : `用户 ${user.username} 已停用`)
    await loadUsers()
  } catch (error) {
    flashMessage(error instanceof Error ? error.message : '操作失败')
  } finally {
    delete userBusy.value[user.id]
  }
}

async function removeUser(user: IdentityUser) {
  if (!window.confirm(`确定删除用户 ${user.username}（${user.display_name || '未设置显示名'}）？此操作不可恢复。`)) return
  userBusy.value[user.id] = 'delete'
  try {
    await deleteUser(user.id)
    flashMessage(`用户 ${user.username} 已删除`)
    await loadUsers()
  } catch (error) {
    flashMessage(error instanceof Error ? error.message : '删除失败')
  } finally {
    delete userBusy.value[user.id]
  }
}

// ---------- stats ----------
const stats = computed(() => {
  const active = users.value.filter((u) => u.status === 'active').length
  const regionsTotal = flatRegions.value.length
  return {
    users: users.value.length,
    active,
    disabled: users.value.length - active,
    tenants: tenants.value.length,
    regions: regionsTotal,
  }
})

const headAction = computed(() => {
  switch (tab.value) {
    case 'tenants': return { label: '新增租户', icon: Building2, run: openTenantModal }
    case 'regions': return { label: '新增根区域', icon: Network, run: () => openRegionModal('') }
    default: return { label: '新增用户', icon: UserPlus, run: openUserCreate }
  }
})

function refreshCurrentTab() {
  switch (tab.value) {
    case 'tenants': return void loadTenants()
    case 'regions': return void loadRegions()
    default: return void loadUsers()
  }
}

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
      currentUser.value = { id: info.user.id, tenantID: info.user.tenant_id }
      nodeAdmin.value = (info.roles ?? []).includes('node_admin')
      if (!nodeAdmin.value) {
        accessDenied.value = true
        return
      }
      await Promise.all([loadTenants(), loadRegions(), loadRoles()])
      await loadUsers()
    } catch (error) {
      usersError.value = error instanceof Error ? error.message : '初始化失败'
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
        <RouterLink to="/identity" class="prod-nav-link active" aria-current="page">权限管理</RouterLink>
        <RouterLink to="/devices" class="prod-nav-link">设备管理</RouterLink>
        <RouterLink to="/" class="prod-nav-link">测试控制台</RouterLink>
      </nav>
      <div class="prod-topbar-right">
        <span class="prod-health" :class="`health-${healthTone}`" :title="healthTitle">
          <span class="prod-health-dot" /><span>{{ healthTitle }}</span>
        </span>
      </div>
    </header>

    <main class="prod-main">
      <div class="prod-page-head">
        <div class="prod-heading">
          <span class="prod-eyebrow">IDENTITY / RBAC</span>
          <h1>权限管理</h1>
          <p>管理租户、区域、用户与角色分配，基于 Casbin 域内 RBAC 实时生效。</p>
        </div>
        <button v-if="nodeAdmin" class="prod-button prod-button-primary" type="button" @click="headAction.run()">
          <component :is="headAction.icon" :size="16" :stroke-width="2.2" />{{ headAction.label }}
        </button>
      </div>

      <Transition name="toast">
        <div v-if="flash" class="prod-flash" role="status">
          <Check :size="15" />{{ flash }}
        </div>
      </Transition>

      <!-- access denied -->
      <div v-if="accessDenied" class="prod-empty denied" role="alert">
        <span class="prod-empty-icon"><ShieldCheck :size="26" /></span>
        <strong>需要节点管理员权限</strong>
        <p>身份与权限管理仅对 node_admin 角色开放，请联系管理员调整你的角色。</p>
        <RouterLink class="prod-button" to="/devices">前往设备管理</RouterLink>
      </div>

      <template v-else>
        <!-- metrics -->
        <div class="prod-stats" aria-label="身份数据统计">
          <div class="prod-stat">
            <span class="prod-stat-label"><Users :size="14" />用户</span>
            <strong class="prod-stat-value">{{ stats.users }}</strong>
          </div>
          <div class="prod-stat">
            <span class="prod-stat-label"><span class="stat-dot dot-online" />启用中</span>
            <strong class="prod-stat-value stat-online">{{ stats.active }}</strong>
          </div>
          <div class="prod-stat">
            <span class="prod-stat-label"><span class="stat-dot dot-offline" />已停用</span>
            <strong class="prod-stat-value stat-offline">{{ stats.disabled }}</strong>
          </div>
          <div class="prod-stat">
            <span class="prod-stat-label"><Building2 :size="14" />租户</span>
            <strong class="prod-stat-value">{{ stats.tenants }}</strong>
          </div>
          <div class="prod-stat">
            <span class="prod-stat-label"><Network :size="14" />区域</span>
            <strong class="prod-stat-value">{{ stats.regions }}</strong>
          </div>
        </div>

        <!-- tabs -->
        <div class="prod-tabs" role="tablist" aria-label="身份数据视图">
          <button
            v-for="t in TABS" :key="t.key" type="button" class="prod-tab"
            :class="{ active: tab === t.key }" role="tab" :aria-selected="tab === t.key"
            @click="tab = t.key"
          >{{ t.label }}</button>
        </div>

        <!-- ============ users tab ============ -->
        <section v-if="tab === 'users'" aria-label="用户管理">
          <div class="prod-toolbar">
            <div class="prod-search">
              <Search :size="15" class="prod-search-icon" />
              <input v-model="userSearch" placeholder="搜索用户名、显示名或角色" @input="resetUserPage" aria-label="搜索用户" />
              <button v-if="userSearch" class="prod-search-clear" type="button" aria-label="清空搜索" @click="userSearch = ''; resetUserPage()"><X :size="13" /></button>
            </div>
            <select v-model="tenantFilter" class="prod-select" aria-label="按租户筛选" @change="resetUserPage(); loadUsers()">
              <option value="">全部租户（本租户）</option>
              <option v-for="t in tenants" :key="t.id" :value="t.id">{{ t.name }}</option>
            </select>
            <button class="prod-icon prod-refresh" type="button" :disabled="usersLoading" aria-label="刷新用户列表" title="刷新" @click="loadUsers">
              <RefreshCw :size="16" :class="{ spinning: usersLoading }" />
            </button>
          </div>

          <div v-if="usersError" class="prod-error" role="alert">
            <AlertTriangle :size="18" />
            <div>
              <strong>加载失败</strong>
              <p>{{ usersError }}</p>
            </div>
            <button class="prod-button" type="button" @click="loadUsers"><RefreshCw :size="14" />重试</button>
          </div>

          <div v-else-if="usersLoading" class="prod-table-wrap" aria-label="加载中">
            <table class="prod-table">
              <thead>
                <tr>
                  <th scope="col">用户</th><th scope="col">租户</th><th scope="col">角色</th>
                  <th scope="col">区域范围</th><th scope="col">状态</th><th scope="col">创建时间</th>
                  <th scope="col" class="col-actions">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="n in 6" :key="n" class="skeleton-row">
                  <td><span class="sk sk-name" /></td>
                  <td><span class="sk sk-pill" /></td>
                  <td><span class="sk sk-pill" /></td>
                  <td><span class="sk sk-pill" /></td>
                  <td><span class="sk sk-pill" /></td>
                  <td><span class="sk sk-id" /></td>
                  <td><span class="sk sk-actions" /></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else-if="filteredUsers.length > 0" class="prod-table-wrap">
            <table class="prod-table">
              <thead>
                <tr>
                  <th scope="col">用户</th><th scope="col">租户</th><th scope="col">角色</th>
                  <th scope="col">区域范围</th><th scope="col">状态</th><th scope="col">创建时间</th>
                  <th scope="col" class="col-actions">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="user in pageUsers" :key="user.id" class="prod-row">
                  <td class="prod-name-cell">
                    <div class="prod-name">{{ user.username }}</div>
                    <div class="prod-name-sub">{{ user.display_name || '未设置显示名' }}</div>
                  </td>
                  <td class="prod-tenant-cell">{{ tenantNameByID.get(user.tenant_id) ?? user.tenant_id }}</td>
                  <td>
                    <div class="prod-role-pills">
                      <span v-for="role in user.roles" :key="role" class="prod-pill" :class="`pill-role-${role}`" :title="roleHint(role)">
                        {{ roleLabel(role) }}
                      </span>
                      <span v-if="user.roles.length === 0" class="prod-pill pill-muted">无角色</span>
                    </div>
                  </td>
                  <td>
                    <span v-if="user.region_ids.length > 0" class="prod-scope" :title="user.region_ids.map(regionPath).join('\n')">
                      {{ user.region_ids.length }} 个区域
                    </span>
                    <span v-else class="prod-scope prod-scope-none">未分配</span>
                  </td>
                  <td>
                    <span class="prod-pill" :class="user.status === 'active' ? 'pill-enable' : 'pill-muted'">
                      <span class="pill-dot" :class="user.status === 'active' ? 'dot-online' : 'dot-offline'" />
                      {{ user.status === 'active' ? '启用' : '停用' }}
                    </span>
                  </td>
                  <td class="mono prod-time">{{ formatDate(user.created_at) }}</td>
                  <td class="col-actions">
                    <div class="prod-actions">
                      <button class="prod-icon" type="button" title="编辑用户" aria-label="编辑用户" @click="openUserEdit(user)"><Edit3 :size="15" /></button>
                      <button class="prod-icon" type="button" title="重置密码" aria-label="重置密码" @click="openPasswordModal(user)"><KeyRound :size="15" /></button>
                      <button class="prod-icon" type="button" :disabled="userBusy[user.id] === 'status'" :title="user.status === 'active' ? '停用' : '启用'" :aria-label="user.status === 'active' ? '停用' : '启用'" @click="toggleUserStatus(user)">
                        <Pause v-if="user.status === 'active'" :size="15" /><Play v-else :size="15" />
                      </button>
                      <button
                        class="prod-icon danger" type="button"
                        :disabled="userBusy[user.id] === 'delete' || user.id === currentUser?.id"
                        :title="user.id === currentUser?.id ? '不能删除自己的账号' : '删除'"
                        :aria-label="user.id === currentUser?.id ? '不能删除自己的账号' : '删除'"
                        @click="removeUser(user)"
                      ><Trash2 :size="15" /></button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else class="prod-empty" role="status">
            <span class="prod-empty-icon"><Search v-if="userSearch" :size="26" /><Users v-else :size="26" /></span>
            <strong>{{ userSearch ? '没有符合条件的用户' : '还没有用户' }}</strong>
            <p>{{ userSearch ? '试试调整搜索词或筛选条件。' : '点击「新增用户」创建第一个账号。' }}</p>
            <button v-if="!userSearch" class="prod-button prod-button-primary" type="button" @click="openUserCreate">
              <UserPlus :size="15" />新增用户
            </button>
            <button v-else class="prod-button" type="button" @click="userSearch = ''; resetUserPage()"><X :size="15" />清除筛选</button>
          </div>

          <div v-if="filteredUsers.length > userPageSize" class="prod-pagination">
            <span class="prod-page-count mono">{{ (userPage - 1) * userPageSize + 1 }}-{{ Math.min(userPage * userPageSize, filteredUsers.length) }} / {{ filteredUsers.length }}</span>
            <div class="prod-page-btns">
              <button class="prod-icon" type="button" :disabled="userPage <= 1" aria-label="上一页" @click="userPage--"><ChevronLeft :size="15" /></button>
              <span class="prod-page-info mono">{{ userPage }} / {{ userTotalPages }}</span>
              <button class="prod-icon" type="button" :disabled="userPage >= userTotalPages" aria-label="下一页" @click="userPage++"><ChevronRight :size="15" /></button>
            </div>
          </div>
        </section>

        <!-- ============ tenants tab ============ -->
        <section v-else-if="tab === 'tenants'" aria-label="租户管理">
          <div class="prod-toolbar">
            <div class="prod-toolbar-hint">租户是权限的顶层隔离域；停用租户后其下用户将无法登录。</div>
            <button class="prod-icon prod-refresh" type="button" :disabled="tenantsLoading" aria-label="刷新租户列表" title="刷新" @click="loadTenants">
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

          <div v-else-if="tenantsLoading" class="prod-table-wrap" aria-label="加载中">
            <table class="prod-table">
              <thead>
                <tr><th scope="col">租户</th><th scope="col">状态</th><th scope="col">创建时间</th><th scope="col" class="col-actions">操作</th></tr>
              </thead>
              <tbody>
                <tr v-for="n in 4" :key="n" class="skeleton-row">
                  <td><span class="sk sk-name" /></td><td><span class="sk sk-pill" /></td>
                  <td><span class="sk sk-id" /></td><td><span class="sk sk-actions" /></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else-if="tenants.length > 0" class="prod-table-wrap">
            <table class="prod-table">
              <thead>
                <tr><th scope="col">租户</th><th scope="col">状态</th><th scope="col">创建时间</th><th scope="col" class="col-actions">操作</th></tr>
              </thead>
              <tbody>
                <tr v-for="tenant in tenants" :key="tenant.id" class="prod-row">
                  <td class="prod-name-cell">
                    <div class="prod-name prod-tenant-name">
                      <Building2 :size="14" class="prod-tenant-icon" />{{ tenant.name }}
                    </div>
                    <div class="prod-name-sub mono">{{ tenant.id }}</div>
                  </td>
                  <td>
                    <span class="prod-pill" :class="tenant.status === 'active' ? 'pill-enable' : 'pill-muted'">
                      <span class="pill-dot" :class="tenant.status === 'active' ? 'dot-online' : 'dot-offline'" />
                      {{ tenant.status === 'active' ? '启用' : '停用' }}
                    </span>
                  </td>
                  <td class="mono prod-time">{{ formatDate(tenant.created_at) }}</td>
                  <td class="col-actions">
                    <div class="prod-actions">
                      <button
                        class="prod-icon" type="button"
                        :disabled="tenantBusy[tenant.id]"
                        :title="tenant.status === 'active' ? '停用租户' : '启用租户'"
                        :aria-label="tenant.status === 'active' ? '停用租户' : '启用租户'"
                        @click="toggleTenant(tenant)"
                      >
                        <Pause v-if="tenant.status === 'active'" :size="15" /><Play v-else :size="15" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
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
          <div class="prod-toolbar">
            <div class="prod-toolbar-hint">区域是树形数据范围；给用户分配父区域即可覆盖其整个子树。</div>
            <button class="prod-icon prod-refresh" type="button" :disabled="regionsLoading" aria-label="刷新区域列表" title="刷新" @click="loadRegions">
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

          <div v-else-if="regionsLoading" class="prod-table-wrap" aria-label="加载中">
            <table class="prod-table">
              <thead>
                <tr><th scope="col">区域</th><th scope="col">创建时间</th><th scope="col" class="col-actions">操作</th></tr>
              </thead>
              <tbody>
                <tr v-for="n in 5" :key="n" class="skeleton-row">
                  <td><span class="sk sk-name" /></td><td><span class="sk sk-id" /></td><td><span class="sk sk-actions" /></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else-if="flatRegions.length > 0" class="prod-table-wrap">
            <table class="prod-table">
              <thead>
                <tr><th scope="col">区域</th><th scope="col">创建时间</th><th scope="col" class="col-actions">操作</th></tr>
              </thead>
              <tbody>
                <tr v-for="flat in flatRegions" :key="flat.region.id" class="prod-row">
                  <td>
                    <div class="prod-region-cell" :style="{ paddingLeft: `${flat.depth * 22 + 2}px` }">
                      <Network :size="14" class="prod-region-icon" />
                      <span class="prod-region-name">{{ flat.region.name }}</span>
                      <span class="prod-region-path">{{ flat.path }}</span>
                    </div>
                  </td>
                  <td class="mono prod-time">{{ formatDate(flat.region.created_at) }}</td>
                  <td class="col-actions">
                    <div class="prod-actions">
                      <button class="prod-icon" type="button" title="添加子区域" aria-label="添加子区域" @click="openRegionModal(flat.region.id)"><Plus :size="15" /></button>
                      <button class="prod-icon" type="button" title="重命名" aria-label="重命名" @click="openRenameRegion(flat)"><Edit3 :size="15" /></button>
                      <button class="prod-icon danger" type="button" :disabled="regionBusy[flat.region.id]" title="删除" aria-label="删除" @click="removeRegion(flat)"><Trash2 :size="15" /></button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
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

    <footer class="prod-footer">
      <span>new-vision 节点管理系统</span>
      <span class="prod-footer-deps"><ShieldCheck :size="13" />Casbin RBAC · 角色变更实时生效</span>
    </footer>

    <!-- ============ user create / edit modal ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="userModalOpen" class="prod-overlay" @click.self="closeUserModal">
          <div class="prod-modal" role="dialog" aria-modal="true" :aria-label="userModalMode === 'create' ? '新增用户' : '编辑用户'">
            <div class="prod-modal-head">
              <h2>{{ userModalMode === 'create' ? '新增用户' : `编辑用户 ${userForm.username}` }}</h2>
              <button class="prod-icon" type="button" aria-label="关闭" @click="closeUserModal"><X :size="16" /></button>
            </div>
            <form class="prod-form" @submit.prevent="submitUserForm">
              <div class="prod-form-grid">
                <div v-if="userModalMode === 'create'" class="prod-field">
                  <label for="idm-user-username">用户名</label>
                  <input id="idm-user-username" v-model="userForm.username" required maxlength="64" autocomplete="off" placeholder="登录用户名" />
                </div>
                <div v-if="userModalMode === 'create' && tenants.length > 1" class="prod-field">
                  <label for="idm-user-tenant">所属租户</label>
                  <select id="idm-user-tenant" v-model="userForm.tenant_id" class="prod-select prod-select-full">
                    <option v-for="t in tenants" :key="t.id" :value="t.id">{{ t.name }}</option>
                  </select>
                </div>
                <div class="prod-field" :class="{ 'prod-field-full': userModalMode === 'edit' }">
                  <label for="idm-user-display">显示名</label>
                  <input id="idm-user-display" v-model="userForm.display_name" maxlength="255" placeholder="如：张三（运维）" />
                </div>
                <div v-if="userModalMode === 'edit'" class="prod-field">
                  <label for="idm-user-status">账号状态</label>
                  <select id="idm-user-status" v-model="userForm.status" class="prod-select prod-select-full">
                    <option value="active">启用</option>
                    <option value="disabled">停用</option>
                  </select>
                </div>
                <div v-if="userModalMode === 'create'" class="prod-field prod-field-full">
                  <label for="idm-user-password">初始密码</label>
                  <div class="prod-input-action">
                    <input
                      id="idm-user-password" v-model="userForm.password"
                      :type="passwordVisible ? 'text' : 'password'"
                      required maxlength="256" autocomplete="new-password" placeholder="至少 1 位，建议使用生成器"
                    />
                    <button class="prod-icon" type="button" :aria-label="passwordVisible ? '隐藏密码' : '显示密码'" :title="passwordVisible ? '隐藏密码' : '显示密码'" @click="passwordVisible = !passwordVisible">
                      <Eye v-if="passwordVisible" :size="15" /><EyeOff v-else :size="15" />
                    </button>
                    <button class="prod-icon" type="button" aria-label="生成随机密码" title="生成随机密码" @click="generatePassword"><KeyRound :size="15" /></button>
                  </div>
                </div>
                <div class="prod-field prod-field-full">
                  <span class="prod-field-label">角色（可多选）</span>
                  <div class="prod-role-chips" role="group" aria-label="角色选择">
                    <label
                      v-for="role in availableRoles" :key="role" class="prod-role-chip"
                      :class="{ checked: userForm.roles.includes(role) }" :title="roleHint(role)"
                    >
                      <input type="checkbox" :checked="userForm.roles.includes(role)" @change="toggleRole(role)" />
                      <span class="prod-role-chip-label">{{ roleLabel(role) }}</span>
                      <span class="prod-role-chip-code mono">{{ role }}</span>
                    </label>
                  </div>
                  <span class="prod-meta-hint">角色决定功能权限，矩阵由服务端固定，变更立即生效。</span>
                </div>
                <div class="prod-field prod-field-full">
                  <span class="prod-field-label">区域范围（数据权限，可多选）</span>
                  <div v-if="flatRegions.length > 0" class="prod-region-picker" role="group" aria-label="区域范围选择">
                    <label
                      v-for="flat in flatRegions" :key="flat.region.id" class="prod-region-check"
                      :style="{ paddingLeft: `${flat.depth * 20 + 10}px` }"
                    >
                      <input type="checkbox" :checked="userForm.region_ids.includes(flat.region.id)" @change="toggleRegionScope(flat.region.id)" />
                      <span class="prod-region-check-name">{{ flat.region.name }}</span>
                      <span class="prod-region-check-path">{{ flat.path }}</span>
                    </label>
                  </div>
                  <p v-else class="prod-meta-hint">暂无区域可分配，可先在「区域」页签创建。</p>
                  <span v-if="flatRegions.length > 0" class="prod-meta-hint">勾选父区域即覆盖其整个子树；不选则该用户看不到任何设备。</span>
                </div>
              </div>
              <p v-if="userFormError" class="prod-error" role="alert">{{ userFormError }}</p>
              <div class="prod-modal-actions">
                <button class="prod-button" type="button" @click="closeUserModal">取消</button>
                <button class="prod-button prod-button-primary" type="submit" :disabled="userSaving">
                  <Check :size="16" />{{ userSaving ? '保存中…' : userModalMode === 'create' ? '创建' : '保存' }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============ reset password modal ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="passwordModalUser" class="prod-overlay" @click.self="passwordModalUser = null">
          <div class="prod-modal prod-modal-narrow" role="dialog" aria-modal="true" :aria-label="`重置 ${passwordModalUser.username} 的密码`">
            <div class="prod-modal-head">
              <h2>重置密码 <span class="prod-type-badge">{{ passwordModalUser.username }}</span></h2>
              <button class="prod-icon" type="button" aria-label="关闭" @click="passwordModalUser = null"><X :size="16" /></button>
            </div>
            <form class="prod-form" @submit.prevent="submitPassword">
              <div class="prod-field">
                <label for="idm-new-password">新密码</label>
                <div class="prod-input-action">
                  <input
                    id="idm-new-password" v-model="newPassword"
                    :type="newPasswordVisible ? 'text' : 'password'"
                    required maxlength="256" autocomplete="new-password" placeholder="至少 1 位，建议使用生成器"
                  />
                  <button class="prod-icon" type="button" :aria-label="newPasswordVisible ? '隐藏密码' : '显示密码'" :title="newPasswordVisible ? '隐藏密码' : '显示密码'" @click="newPasswordVisible = !newPasswordVisible">
                    <Eye v-if="newPasswordVisible" :size="15" /><EyeOff v-else :size="15" />
                  </button>
                  <button class="prod-icon" type="button" aria-label="生成随机密码" title="生成随机密码" @click="generateAdminPassword"><KeyRound :size="15" /></button>
                </div>
                <span class="prod-meta-hint">重置后原密码立即失效，请将新密码安全地告知用户。</span>
              </div>
              <p v-if="passwordError" class="prod-error" role="alert">{{ passwordError }}</p>
              <div class="prod-modal-actions">
                <button class="prod-button" type="button" @click="passwordModalUser = null">取消</button>
                <button class="prod-button prod-button-primary" type="submit" :disabled="passwordSaving">
                  <KeyRound :size="16" />{{ passwordSaving ? '重置中…' : '重置密码' }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

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
.prod-stat-value { font-size: 26px; font-weight: 700; letter-spacing: -.02em; font-variant-numeric: tabular-nums; line-height: 1; }
.stat-online { color: #0e9f6e; }
.stat-offline { color: #7c8792; }
/* ---------- tabs ---------- */
.prod-tabs { display: inline-flex; gap: 4px; margin-top: 24px; padding: 4px; background: #fff; border: 1px solid #e4e7eb; border-radius: 10px; }
.prod-tab { padding: 8px 18px; color: #5f6873; background: none; border: 0; border-radius: 7px; font: inherit; font-size: 13px; font-weight: 600; cursor: pointer; transition: color .15s, background .15s; }
.prod-tab:hover { color: #1a1f26; background: #f3f5f7; }
.prod-tab.active { color: #fff; background: #1a1f26; }
.prod-tab:focus-visible { outline: 2px solid #1a1f26; outline-offset: 1px; }
/* ---------- toolbar ---------- */
.prod-toolbar { display: flex; gap: 10px; margin-top: 20px; align-items: center; }
.prod-toolbar-hint { flex: 1; color: #8a939d; font-size: 12.5px; }
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
.prod-button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; padding: 10px 16px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; font-size: 13.5px; font-weight: 600; text-decoration: none; cursor: pointer; transition: border-color .15s, background .15s, transform .05s; }
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
.prod-tenant-cell { color: #4b5563; }
.prod-time { color: #8a939d; font-size: 12px; white-space: nowrap; }
.prod-pill { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; white-space: nowrap; }
.pill-dot { width: 6px; height: 6px; border-radius: 50%; }
.pill-enable { color: #1d714e; background: #e9f6ef; }
.pill-muted { color: #65707b; background: #f0f2f4; }
.prod-role-pills { display: flex; flex-wrap: wrap; gap: 5px; max-width: 240px; }
.pill-role-node_admin { color: #6d28d9; background: #f1eafd; }
.pill-role-tenant_admin { color: #1d4ed8; background: #e8effd; }
.pill-role-operator { color: #92400e; background: #fdf0e0; }
.pill-role-viewer { color: #4b5563; background: #f0f2f4; }
.prod-scope { color: #4b5563; font-size: 12.5px; cursor: help; }
.prod-scope-none { color: #a6b0ba; cursor: default; }
.prod-table .col-actions { text-align: right; }
.prod-actions { display: inline-flex; gap: 5px; opacity: .55; transition: opacity .15s; }
.prod-row:hover .prod-actions, .prod-actions:focus-within { opacity: 1; }
.prod-tenant-name { display: inline-flex; align-items: center; gap: 7px; }
.prod-tenant-icon { color: #9aa3ac; flex-shrink: 0; }
.prod-region-cell { display: flex; align-items: center; gap: 8px; min-width: 260px; }
.prod-region-icon { color: #9aa3ac; flex-shrink: 0; }
.prod-region-name { font-weight: 600; color: #1a1f26; white-space: nowrap; }
.prod-region-path { color: #9aa3ac; font-size: 11.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
.prod-empty.denied { margin-top: 24px; }
/* ---------- pagination ---------- */
.prod-pagination { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 14px; }
.prod-page-count { color: #8a939d; font-size: 12px; }
.prod-page-btns { display: flex; align-items: center; gap: 8px; }
.prod-page-info { color: #5f6873; font-size: 12px; }
/* ---------- overlay / modal ---------- */
.prod-overlay { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(13,18,24,.48); backdrop-filter: blur(2px); }
.prod-modal { width: 100%; max-width: 560px; max-height: 88vh; overflow-y: auto; background: #fff; border-radius: 14px; box-shadow: 0 24px 64px rgba(13,18,24,.28); }
.prod-modal-narrow { max-width: 480px; }
.prod-modal-head { display: flex; align-items: center; justify-content: space-between; padding: 19px 22px; border-bottom: 1px solid #eef0f3; position: sticky; top: 0; background: #fff; z-index: 2; }
.prod-modal-head h2 { margin: 0; font-size: 16.5px; display: flex; align-items: center; gap: 10px; }
.prod-type-badge { display: inline-block; padding: 3px 9px; color: #1a1f26; background: #eef1f4; border-radius: 999px; font-size: 11.5px; font-weight: 700; }
.prod-form { padding: 20px 22px; display: grid; gap: 14px; }
.prod-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.prod-field { display: grid; gap: 6px; }
.prod-field-full { grid-column: 1 / -1; }
.prod-field label, .prod-field-label { font-size: 12px; font-weight: 700; color: #5f6873; }
.prod-field input { width: 100%; padding: 10px 11px; font: inherit; font-size: 13.5px; color: #1a1f26; background: #fff; border: 1px solid #d9dee4; border-radius: 8px; box-sizing: border-box; transition: border-color .15s, box-shadow .15s; }
.prod-field input:focus-visible { outline: none; border-color: #1a1f26; box-shadow: 0 0 0 3px rgba(26,31,38,.1); }
.prod-select-full { width: 100%; }
.prod-select-full:focus-visible { outline: none; border-color: #1a1f26; box-shadow: 0 0 0 3px rgba(26,31,38,.1); }
.prod-input-action { display: flex; gap: 6px; }
.prod-input-action input { flex: 1; }
.prod-meta-hint { margin: 0; color: #8a939d; font-size: 12px; }
.prod-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }
/* ---------- role chips ---------- */
.prod-role-chips { display: flex; flex-wrap: wrap; gap: 8px; }
.prod-role-chip { display: inline-flex; align-items: center; gap: 8px; padding: 8px 12px; background: #fff; border: 1px solid #d9dee4; border-radius: 9px; font-size: 12.5px; font-weight: 600; color: #4b5563; cursor: pointer; transition: border-color .15s, background .15s; }
.prod-role-chip:hover { border-color: #aeb7c1; }
.prod-role-chip input { accent-color: #1a1f26; margin: 0; }
.prod-role-chip:focus-within { outline: 2px solid #1a1f26; outline-offset: 1px; border-radius: 9px; }
.prod-role-chip.checked { color: #1a1f26; background: #f1f3f5; border-color: #1a1f26; }
.prod-role-chip-code { color: #9aa3ac; font-size: 10.5px; }
/* ---------- region picker ---------- */
.prod-region-picker { display: grid; max-height: 220px; overflow-y: auto; padding: 6px; background: #f8f9fa; border: 1px solid #e4e7eb; border-radius: 9px; }
.prod-region-check { display: flex; align-items: center; gap: 8px; padding: 7px 10px; border-radius: 6px; font-size: 13px; cursor: pointer; }
.prod-region-check:hover { background: #eef1f4; }
.prod-region-check input { accent-color: #1a1f26; margin: 0; flex-shrink: 0; }
.prod-region-check-name { font-weight: 600; color: #1a1f26; white-space: nowrap; }
.prod-region-check-path { color: #9aa3ac; font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.prod-region-check:focus-within { outline: 2px solid #1a1f26; outline-offset: -2px; }
/* ---------- modal transitions ---------- */
.modal-enter-active, .modal-leave-active { transition: opacity .18s; }
.modal-enter-active .prod-modal, .modal-leave-active .prod-modal { transition: transform .18s cubic-bezier(.2,.8,.3,1), opacity .18s; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-from .prod-modal, .modal-leave-to .prod-modal { transform: translateY(14px) scale(.98); opacity: 0; }
/* ---------- footer ---------- */
.prod-footer { max-width: 1240px; margin: 0 auto; padding: 20px 32px 32px; display: flex; align-items: center; justify-content: space-between; color: #9aa3ac; font-size: 12px; }
.prod-footer-deps { display: inline-flex; align-items: center; gap: 6px; }
.mono { font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 12px; font-variant-numeric: tabular-nums; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* ---------- reduced motion ---------- */
@media (prefers-reduced-motion: reduce) {
  .sk, .spinning { animation: none; }
  .prod-row, .prod-button, .prod-icon, .prod-nav-link, .prod-tab, .prod-role-chip, .prod-search input, .prod-select, .prod-field input { transition: none; }
  .modal-enter-active, .modal-leave-active, .toast-enter-active, .toast-leave-active { transition: none; }
  .modal-enter-from, .modal-leave-to, .toast-enter-from, .toast-leave-to { opacity: 1; transform: none; }
}
/* ---------- responsive ---------- */
@media (max-width: 900px) {
  .prod-topbar { gap: 14px; padding: 0 16px; }
  .prod-brand-text span { display: none; }
  .prod-main { padding: 24px 16px 48px; }
  .prod-stats { grid-template-columns: repeat(2, 1fr); gap: 10px; }
  .prod-toolbar { flex-wrap: wrap; }
  .prod-search { max-width: none; flex-basis: 100%; }
  .prod-refresh { margin-left: 0; }
  .prod-table-wrap { overflow-x: auto; }
  .prod-table { min-width: 860px; }
  .prod-page-head { flex-direction: column; align-items: stretch; }
  .prod-page-head .prod-button { align-self: flex-start; }
  .prod-form-grid { grid-template-columns: 1fr; }
}
</style>
