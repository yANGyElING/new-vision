<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  AlertTriangle, Building2, Check, ChevronLeft, ChevronRight, Edit3, Eye, EyeOff,
  KeyRound, Monitor, Pause, Play, RefreshCw, Search, ShieldCheck, UserPlus, Users, X,
} from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { fetchHealth, type HealthState } from '@/api/health'
import { me } from '@/api/auth'
import {
  ROLE_META, createUser, deleteUser, listRegions, listRoles, listTenants, listUsers,
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

// ---------- flash ----------
const flash = ref('')
function flashMessage(msg: string) {
  flash.value = msg
  window.setTimeout(() => { if (flash.value === msg) flash.value = '' }, 3000)
}

// ---------- tenants (filter + create form) ----------
const tenants = ref<Tenant[]>([])
const tenantNameByID = computed(() => {
  const map = new Map<string, string>()
  for (const t of tenants.value) map.set(t.id, t.name)
  return map
})

async function loadTenants() {
  try {
    tenants.value = await listTenants()
  } catch {
    /* keep empty list; table falls back to raw tenant ids */
  }
}

// ---------- regions (scope picker) ----------
const regionTree = ref<Region[]>([])

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
  try {
    regionTree.value = await listRegions()
  } catch {
    /* keep empty tree; picker shows a hint */
  }
}

// ---------- users ----------
const users = ref<IdentityUser[]>([])
const usersLoading = ref(false)
const usersError = ref('')
const userSearch = ref('')
const statusFilter = ref<'' | 'active' | 'disabled'>('')
const tenantFilter = ref('')
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

const filterActive = computed(() => Boolean(userSearch.value.trim() || statusFilter.value || tenantFilter.value))

function clearFilters() {
  userSearch.value = ''
  statusFilter.value = ''
  tenantFilter.value = ''
  resetUserPage()
  void loadUsers()
}

const filteredUsers = computed(() => {
  const q = userSearch.value.trim().toLowerCase()
  let list = users.value
  if (statusFilter.value) list = list.filter((u) => u.status === statusFilter.value)
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

// The role matrix is cumulative (viewer ⊂ operator ⊂ tenant_admin ⊂
// node_admin), so one role fully describes a user's privileges; pick the
// highest when editing a user that somehow holds several.
const ROLE_ORDER = ['node_admin', 'tenant_admin', 'operator', 'viewer']

const currentRole = computed(() => {
  for (const role of ROLE_ORDER) {
    if (userForm.value.roles.includes(role)) return role
  }
  return ''
})

function pickRole(role: string) {
  userForm.value.roles = role ? [role] : []
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
  const admins = users.value.filter((u) => u.roles.includes('node_admin') || u.roles.includes('tenant_admin')).length
  return {
    total: users.value.length,
    active,
    disabled: users.value.length - active,
    admins,
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
  <div class="page-shell">
    <header class="page-topbar">
      <div class="page-brand">
        <span class="page-logo"><Monitor :size="18" :stroke-width="2.2" /></span>
        <div class="page-brand-text">
          <strong>new-vision</strong>
          <span>节点管理系统</span>
        </div>
      </div>
      <nav class="page-nav" aria-label="主导航">
        <RouterLink to="/users" class="page-nav-link active" aria-current="page">用户管理</RouterLink>
        <RouterLink to="/identity" class="page-nav-link">组织架构</RouterLink>
        <RouterLink to="/devices" class="page-nav-link">设备管理</RouterLink>
        <RouterLink to="/" class="page-nav-link">测试控制台</RouterLink>
      </nav>
      <div class="page-topbar-right">
        <span class="page-health" :class="`health-${healthTone}`" :title="healthTitle">
          <span class="page-health-dot" /><span>{{ healthTitle }}</span>
        </span>
      </div>
    </header>

    <main class="cg-main">
      <!-- page head -->
      <div class="cg-head">
        <div class="cg-head-copy">
          <h1>用户管理</h1>
          <p>管理用户账号、角色分配与数据范围，基于 Casbin 域内 RBAC 实时生效。</p>
        </div>
        <button v-if="nodeAdmin" class="cg-btn-primary" type="button" @click="openUserCreate">
          <UserPlus :size="16" :stroke-width="2.2" />新增用户
        </button>
      </div>

      <Transition name="toast">
        <div v-if="flash" class="cg-toast" role="status">
          <Check :size="15" />{{ flash }}
        </div>
      </Transition>

      <!-- access denied -->
      <div v-if="accessDenied" class="cg-empty denied" role="alert">
        <span class="cg-empty-icon"><ShieldCheck :size="26" /></span>
        <strong>需要节点管理员权限</strong>
        <p>用户管理仅对 node_admin 角色开放，请联系管理员调整你的角色。</p>
        <RouterLink class="cg-btn-quiet" to="/devices">前往设备管理</RouterLink>
      </div>

      <template v-else>
        <!-- metrics strip -->
        <div class="cg-stats" aria-label="用户数据统计">
          <div class="cg-stat">
            <span class="cg-stat-label"><Users :size="14" />用户总数</span>
            <strong class="cg-stat-value">{{ stats.total }}</strong>
          </div>
          <div class="cg-stat">
            <span class="cg-stat-label"><span class="cg-dot on" />启用中</span>
            <strong class="cg-stat-value">{{ stats.active }}</strong>
          </div>
          <div class="cg-stat">
            <span class="cg-stat-label"><span class="cg-dot off" />已停用</span>
            <strong class="cg-stat-value">{{ stats.disabled }}</strong>
          </div>
          <div class="cg-stat">
            <span class="cg-stat-label"><ShieldCheck :size="14" />管理员</span>
            <strong class="cg-stat-value">{{ stats.admins }}</strong>
          </div>
        </div>

        <!-- toolbar -->
        <div class="cg-toolbar">
          <div class="cg-search">
            <Search :size="15" class="cg-search-icon" />
            <input v-model="userSearch" placeholder="搜索用户名、显示名或角色" @input="resetUserPage" aria-label="搜索用户" />
            <button v-if="userSearch" class="cg-search-clear" type="button" aria-label="清空搜索" @click="userSearch = ''; resetUserPage()"><X :size="13" /></button>
          </div>
          <select v-model="tenantFilter" class="cg-select" aria-label="按租户筛选" @change="resetUserPage(); loadUsers()">
            <option value="">本租户</option>
            <option v-for="t in tenants" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
          <select v-model="statusFilter" class="cg-select" aria-label="按状态筛选" @change="resetUserPage">
            <option value="">全部状态</option>
            <option value="active">启用</option>
            <option value="disabled">停用</option>
          </select>
          <button class="cg-icon-btn cg-refresh" type="button" :disabled="usersLoading" aria-label="刷新用户列表" title="刷新" @click="loadUsers">
            <RefreshCw :size="16" :class="{ spinning: usersLoading }" />
          </button>
        </div>

        <!-- error -->
        <div v-if="usersError" class="cg-alert" role="alert">
          <AlertTriangle :size="18" />
          <div>
            <strong>加载失败</strong>
            <p>{{ usersError }}</p>
          </div>
          <button class="cg-btn-quiet" type="button" @click="loadUsers">重试</button>
        </div>

        <!-- loading -->
        <div v-else-if="usersLoading" class="cg-card cg-table-wrap" aria-label="加载中">
          <table class="cg-table">
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

        <!-- table -->
        <div v-else-if="filteredUsers.length > 0" class="cg-card cg-table-wrap">
          <table class="cg-table">
            <thead>
              <tr>
                <th scope="col">用户</th><th scope="col">租户</th><th scope="col">角色</th>
                <th scope="col">区域范围</th><th scope="col">状态</th><th scope="col">创建时间</th>
                <th scope="col" class="col-actions">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="user in pageUsers" :key="user.id" class="cg-row">
                <td class="cg-name-cell">
                  <div class="cg-name">{{ user.username }}<span v-if="user.id === currentUser?.id" class="cg-self-badge">本人</span></div>
                  <div class="cg-name-sub">{{ user.display_name || '未设置显示名' }}</div>
                </td>
                <td class="cg-tenant-cell">
                  <span class="cg-tenant-inline"><Building2 :size="13" class="cg-tenant-icon" />{{ tenantNameByID.get(user.tenant_id) ?? user.tenant_id }}</span>
                </td>
                <td>
                  <div class="cg-role-pills">
                    <span v-for="role in user.roles" :key="role" class="cg-pill" :class="`pill-role-${role}`" :title="roleHint(role)">
                      {{ roleLabel(role) }}
                    </span>
                    <span v-if="user.roles.length === 0" class="cg-pill pill-none">无角色</span>
                  </div>
                </td>
                <td>
                  <span v-if="user.region_ids.length > 0" class="cg-scope" :title="user.region_ids.map(regionPath).join('\n')">
                    {{ user.region_ids.length }} 个区域
                  </span>
                  <span v-else class="cg-scope-none">未分配</span>
                </td>
                <td>
                  <span class="cg-status">
                    <span class="cg-dot" :class="user.status === 'active' ? 'on' : 'off'" />
                    {{ user.status === 'active' ? '启用' : '停用' }}
                  </span>
                </td>
                <td class="mono cg-time">{{ formatDate(user.created_at) }}</td>
                <td class="col-actions">
                  <div class="cg-actions">
                    <button class="cg-act" type="button" title="编辑用户" aria-label="编辑用户" @click="openUserEdit(user)"><Edit3 :size="15" /></button>
                    <button class="cg-act" type="button" title="重置密码" aria-label="重置密码" @click="openPasswordModal(user)"><KeyRound :size="15" /></button>
                    <button class="cg-act" type="button" :disabled="userBusy[user.id] === 'status' || user.id === currentUser?.id" :title="user.status === 'active' ? '停用' : '启用'" :aria-label="user.status === 'active' ? '停用' : '启用'" @click="toggleUserStatus(user)">
                      <Pause v-if="user.status === 'active'" :size="15" /><Play v-else :size="15" />
                    </button>
                    <button
                      class="cg-act danger" type="button"
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

        <!-- empty -->
        <div v-else class="cg-empty" role="status">
          <span class="cg-empty-icon"><Search v-if="filterActive" :size="26" /><Users v-else :size="26" /></span>
          <strong>{{ filterActive ? '没有符合条件的用户' : '还没有用户' }}</strong>
          <p>{{ filterActive ? '试试调整搜索词或筛选条件。' : '点击「新增用户」创建第一个账号。' }}</p>
          <button v-if="!filterActive" class="cg-btn-primary" type="button" @click="openUserCreate">
            <UserPlus :size="15" />新增用户
          </button>
          <button v-else class="cg-btn-quiet" type="button" @click="clearFilters">清除筛选</button>
        </div>

        <!-- pagination -->
        <div v-if="filteredUsers.length > userPageSize" class="cg-pagination">
          <span class="cg-page-count mono">{{ (userPage - 1) * userPageSize + 1 }}-{{ Math.min(userPage * userPageSize, filteredUsers.length) }} / {{ filteredUsers.length }}</span>
          <div class="cg-page-btns">
            <button class="cg-icon-btn" type="button" :disabled="userPage <= 1" aria-label="上一页" @click="userPage--"><ChevronLeft :size="15" /></button>
            <span class="cg-page-info mono">{{ userPage }} / {{ userTotalPages }}</span>
            <button class="cg-icon-btn" type="button" :disabled="userPage >= userTotalPages" aria-label="下一页" @click="userPage++"><ChevronRight :size="15" /></button>
          </div>
        </div>
      </template>
    </main>

    <footer class="cg-footer">
      <span>new-vision 节点管理系统</span>
      <span class="cg-footer-deps"><ShieldCheck :size="13" />Casbin RBAC · 角色变更实时生效</span>
    </footer>

    <!-- ============ user create / edit modal ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="userModalOpen" class="cg-overlay" @click.self="closeUserModal">
          <div class="cg-modal" role="dialog" aria-modal="true" :aria-label="userModalMode === 'create' ? '新增用户' : '编辑用户'">
            <div class="cg-modal-head">
              <h2>{{ userModalMode === 'create' ? '新增用户' : `编辑用户` }}<span v-if="userModalMode === 'edit'" class="cg-user-badge">{{ userForm.username }}</span></h2>
              <button class="cg-icon-btn" type="button" aria-label="关闭" @click="closeUserModal"><X :size="16" /></button>
            </div>
            <form class="cg-form" @submit.prevent="submitUserForm">
              <div class="cg-form-grid">
                <div v-if="userModalMode === 'create'" class="cg-field">
                  <label for="usr-username">用户名</label>
                  <input id="usr-username" v-model="userForm.username" required maxlength="64" autocomplete="off" placeholder="登录用户名" />
                </div>
                <div v-if="userModalMode === 'create' && tenants.length > 1" class="cg-field">
                  <label for="usr-tenant">所属租户</label>
                  <select id="usr-tenant" v-model="userForm.tenant_id" class="cg-select-full">
                    <option v-for="t in tenants" :key="t.id" :value="t.id">{{ t.name }}</option>
                  </select>
                </div>
                <div class="cg-field" :class="{ 'cg-field-full': userModalMode === 'edit' }">
                  <label for="usr-display">显示名</label>
                  <input id="usr-display" v-model="userForm.display_name" maxlength="255" placeholder="如：张三（运维）" />
                </div>
                <div v-if="userModalMode === 'edit'" class="cg-field">
                  <label for="usr-status">账号状态</label>
                  <select id="usr-status" v-model="userForm.status" class="cg-select-full">
                    <option value="active">启用</option>
                    <option value="disabled">停用</option>
                  </select>
                </div>
                <div v-if="userModalMode === 'create'" class="cg-field cg-field-full">
                  <label for="usr-password">初始密码</label>
                  <div class="cg-input-wrap">
                    <input
                      id="usr-password" v-model="userForm.password"
                      :type="passwordVisible ? 'text' : 'password'"
                      required maxlength="256" autocomplete="new-password" placeholder="至少 1 位，建议使用生成器"
                    />
                    <button class="cg-inset-btn" type="button" :aria-label="passwordVisible ? '隐藏密码' : '显示密码'" :title="passwordVisible ? '隐藏密码' : '显示密码'" @click="passwordVisible = !passwordVisible">
                      <Eye v-if="passwordVisible" :size="15" /><EyeOff v-else :size="15" />
                    </button>
                    <button class="cg-inset-btn far" type="button" aria-label="生成随机密码" title="生成随机密码" @click="generatePassword"><KeyRound :size="15" /></button>
                  </div>
                </div>
                <div class="cg-divider" aria-hidden="true"><span>权限配置 · 可不选，创建后随时调整</span></div>
                <div class="cg-field cg-field-full">
                  <span class="cg-field-label">角色</span>
                  <div class="cg-role-row" role="radiogroup" aria-label="角色选择">
                    <label class="cg-role-pill" :class="{ on: currentRole === '' }">
                      <input type="radio" name="usr-role" value="" :checked="currentRole === ''" @change="pickRole('')" />
                      暂不分配
                    </label>
                    <label
                      v-for="role in availableRoles" :key="role" class="cg-role-pill"
                      :class="{ on: currentRole === role }"
                    >
                      <input type="radio" name="usr-role" :value="role" :checked="currentRole === role" @change="pickRole(role)" />
                      {{ roleLabel(role) }}
                    </label>
                  </div>
                  <span class="cg-hint">{{ currentRole ? roleHint(currentRole) : '暂不分配：用户可登录，但暂时没有任何功能与数据权限。' }}</span>
                </div>
                <div class="cg-field cg-field-full">
                  <span class="cg-field-label">区域范围（数据权限，可多选）</span>
                  <div v-if="flatRegions.length > 0" class="cg-region-picker" role="group" aria-label="区域范围选择">
                    <label
                      v-for="flat in flatRegions" :key="flat.region.id" class="cg-region"
                      :style="{ paddingLeft: `${flat.depth * 20 + 12}px` }"
                    >
                      <input type="checkbox" :checked="userForm.region_ids.includes(flat.region.id)" @change="toggleRegionScope(flat.region.id)" />
                      <span class="cg-region-name">{{ flat.region.name }}</span>
                      <span class="cg-region-path">{{ flat.path }}</span>
                    </label>
                  </div>
                  <p v-else class="cg-hint">暂无区域可分配，可先在「组织架构」页创建。</p>
                  <span v-if="flatRegions.length > 0" class="cg-hint">勾选父区域即覆盖其整个子树；不选则该用户看不到任何设备。</span>
                </div>
              </div>
              <p v-if="userFormError" class="cg-alert slim" role="alert">{{ userFormError }}</p>
              <div class="cg-modal-actions">
                <button class="cg-btn-quiet" type="button" @click="closeUserModal">取消</button>
                <button class="cg-btn-primary" type="submit" :disabled="userSaving">
                  <Check :size="16" />{{ userSaving ? '保存中…' : userModalMode === 'create' ? '创建用户' : '保存修改' }}
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
        <div v-if="passwordModalUser" class="cg-overlay" @click.self="passwordModalUser = null">
          <div class="cg-modal cg-modal-narrow" role="dialog" aria-modal="true" :aria-label="`重置 ${passwordModalUser.username} 的密码`">
            <div class="cg-modal-head">
              <h2>重置密码<span class="cg-user-badge">{{ passwordModalUser.username }}</span></h2>
              <button class="cg-icon-btn" type="button" aria-label="关闭" @click="passwordModalUser = null"><X :size="16" /></button>
            </div>
            <form class="cg-form" @submit.prevent="submitPassword">
              <div class="cg-field">
                <label for="usr-new-password">新密码</label>
                <div class="cg-input-wrap">
                  <input
                    id="usr-new-password" v-model="newPassword"
                    :type="newPasswordVisible ? 'text' : 'password'"
                    required maxlength="256" autocomplete="new-password" placeholder="至少 1 位，建议使用生成器"
                  />
                  <button class="cg-inset-btn" type="button" :aria-label="newPasswordVisible ? '隐藏密码' : '显示密码'" :title="newPasswordVisible ? '隐藏密码' : '显示密码'" @click="newPasswordVisible = !newPasswordVisible">
                    <Eye v-if="newPasswordVisible" :size="15" /><EyeOff v-else :size="15" />
                  </button>
                  <button class="cg-inset-btn far" type="button" aria-label="生成随机密码" title="生成随机密码" @click="generateAdminPassword"><KeyRound :size="15" /></button>
                </div>
                <span class="cg-hint">重置后原密码立即失效，请将新密码安全地告知用户。</span>
              </div>
              <p v-if="passwordError" class="cg-alert slim" role="alert">{{ passwordError }}</p>
              <div class="cg-modal-actions">
                <button class="cg-btn-quiet" type="button" @click="passwordModalUser = null">取消</button>
                <button class="cg-btn-primary" type="submit" :disabled="passwordSaving">
                  <KeyRound :size="16" />{{ passwordSaving ? '重置中…' : '重置密码' }}
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
/* =============== shared chrome (topbar / footer) =============== */
.page-shell { min-height: 100vh; background: #f7f8fa; color: #10151b; font-family: 'DM Sans', 'Noto Sans SC', system-ui, sans-serif; }
.page-topbar { display: flex; align-items: center; gap: 30px; padding: 0 32px; height: 62px; background: #11161c; color: #e8ebee; border-bottom: 1px solid #1f2730; position: sticky; top: 0; z-index: 30; }
.page-brand { display: flex; align-items: center; gap: 11px; }
.page-logo { display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px; color: #fff; background: linear-gradient(135deg, #2d3742, #1c242d); border: 1px solid #333f4b; border-radius: 10px; }
.page-brand-text { display: grid; gap: 1px; }
.page-brand-text strong { font-size: 14px; letter-spacing: .01em; }
.page-brand-text span { font-size: 11px; color: #8b98a5; }
.page-nav { display: flex; gap: 4px; margin-left: 6px; }
.page-nav-link { padding: 8px 14px; color: #9aa7b3; font-size: 13.5px; font-weight: 600; text-decoration: none; border-radius: 999px; transition: color .15s, background .15s; }
.page-nav-link:hover { color: #fff; background: rgba(255,255,255,.07); }
.page-nav-link.active { color: #fff; background: rgba(255,255,255,.13); }
.page-nav-link:focus-visible { outline: 2px solid #fff; outline-offset: 1px; }
.page-topbar-right { margin-left: auto; display: flex; align-items: center; gap: 12px; }
.page-health { display: inline-flex; align-items: center; gap: 7px; font-size: 12.5px; font-weight: 600; color: #9aa7b3; }
.page-health-dot { width: 7px; height: 7px; border-radius: 50%; }
.health-up .page-health-dot { background: #34d399; box-shadow: 0 0 0 3px rgba(52,211,153,.15); }
.health-warn .page-health-dot { background: #fbbf24; box-shadow: 0 0 0 3px rgba(251,191,36,.15); }
.health-down .page-health-dot { background: #f87171; box-shadow: 0 0 0 3px rgba(248,113,113,.15); }

/* =============== layout =============== */
.cg-main { max-width: 1120px; margin: 0 auto; padding: 40px 32px 64px; }
.cg-footer { max-width: 1120px; margin: 0 auto; padding: 20px 32px 32px; display: flex; align-items: center; justify-content: space-between; color: #9aa3ad; font-size: 12px; }
.cg-footer-deps { display: inline-flex; align-items: center; gap: 6px; }

/* =============== page head =============== */
.cg-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 18px; }
.cg-head h1 { margin: 0; font-size: 25px; font-weight: 700; letter-spacing: -.015em; color: #10151b; }
.cg-head p { margin: 8px 0 0; color: #5d6772; font-size: 13.5px; }

/* =============== buttons =============== */
.cg-btn-primary { display: inline-flex; align-items: center; justify-content: center; gap: 7px; height: 40px; padding: 0 18px; color: #fff; background: #0f141a; border: 0; border-radius: 999px; font-size: 13.5px; font-weight: 700; letter-spacing: .01em; text-decoration: none; cursor: pointer; transition: background .18s, box-shadow .18s, transform .08s; }
.cg-btn-primary:hover:not(:disabled) { background: #262d36; box-shadow: 0 8px 22px -8px rgba(10,15,22,.5); }
.cg-btn-primary:active:not(:disabled) { transform: scale(.985); }
.cg-btn-primary:disabled { cursor: wait; opacity: .8; }
.cg-btn-quiet { display: inline-flex; align-items: center; justify-content: center; gap: 7px; height: 38px; padding: 0 16px; color: #454f5b; background: transparent; border: 0; border-radius: 999px; font-size: 13.5px; font-weight: 600; text-decoration: none; cursor: pointer; transition: color .15s, background .15s; }
.cg-btn-quiet:hover { color: #10151b; background: #f2f4f6; }
.cg-icon-btn { display: inline-flex; align-items: center; justify-content: center; width: 36px; height: 36px; color: #5d6772; background: transparent; border: 0; border-radius: 10px; cursor: pointer; transition: color .15s, background .15s; }
.cg-icon-btn:hover:not(:disabled) { color: #10151b; background: #f2f4f6; }
.cg-icon-btn:disabled { cursor: not-allowed; opacity: .45; }
.cg-icon-btn:focus-visible { outline: 2px solid #10151b; outline-offset: 1px; }

/* =============== toast =============== */
.cg-toast { position: fixed; top: 78px; left: 50%; transform: translateX(-50%); z-index: 60; display: inline-flex; align-items: center; gap: 8px; padding: 10px 18px; color: #fff; background: #0f141a; border-radius: 999px; font-size: 13px; font-weight: 600; box-shadow: 0 12px 32px -8px rgba(4,9,18,.45); }
.cg-toast svg { color: #6ee7b7; }
.toast-enter-active, .toast-leave-active { transition: opacity .22s, transform .22s; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translate(-50%, -8px); }

/* =============== stats strip =============== */
.cg-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); margin-top: 28px; background: #fff; border: 1px solid #eceef1; border-radius: 16px; box-shadow: 0 1px 2px rgba(16,24,40,.04); overflow: hidden; }
.cg-stat { display: flex; flex-direction: column; gap: 9px; padding: 18px 22px; }
.cg-stat + .cg-stat { border-left: 1px solid #f0f2f4; }
.cg-stat-label { display: inline-flex; align-items: center; gap: 7px; color: #8b939d; font-size: 12px; font-weight: 600; }
.cg-stat-value { font-size: 25px; font-weight: 700; letter-spacing: -.02em; color: #10151b; font-variant-numeric: tabular-nums; line-height: 1; }
.cg-dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
.cg-dot.on { background: #10b981; }
.cg-dot.off { background: #c3cad2; }

/* =============== toolbar =============== */
.cg-toolbar { display: flex; gap: 10px; margin-top: 22px; align-items: center; }
.cg-search { position: relative; flex: 1; max-width: 360px; display: flex; align-items: center; height: 40px; padding: 0 38px 0 14px; background: #f2f4f6; border: 1px solid transparent; border-radius: 999px; transition: background .16s, border-color .16s, box-shadow .16s; }
.cg-search:focus-within { background: #fff; border-color: #c6ccd4; box-shadow: 0 0 0 4px rgba(16,21,27,.07); }
.cg-search-icon { color: #8b939d; pointer-events: none; margin-right: 8px; }
.cg-search input { flex: 1; min-width: 0; height: 100%; font: inherit; font-size: 13.5px; color: #10151b; background: none; border: 0; outline: none; }
.cg-search input::placeholder { color: #9aa3ad; }
.cg-search-clear { position: absolute; right: 8px; top: 50%; transform: translateY(-50%); display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; color: #8b939d; background: none; border: 0; border-radius: 999px; cursor: pointer; }
.cg-search-clear:hover { color: #10151b; background: #eceff2; }
.cg-select { height: 40px; padding: 0 34px 0 14px; font: inherit; font-size: 13px; font-weight: 500; color: #10151b; background: #fff; border: 1px solid #e3e6ea; border-radius: 999px; cursor: pointer; transition: border-color .15s, box-shadow .15s; }
.cg-select:hover { border-color: #c6ccd4; }
.cg-select:focus-visible { outline: none; border-color: #10151b; box-shadow: 0 0 0 4px rgba(16,21,27,.07); }
.cg-refresh { margin-left: auto; }

/* =============== table =============== */
.cg-card { background: #fff; border: 1px solid #eceef1; border-radius: 16px; box-shadow: 0 1px 2px rgba(16,24,40,.04); }
.cg-table-wrap { margin-top: 16px; overflow: hidden; }
.cg-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.cg-table th { padding: 13px 18px; text-align: left; color: #8b939d; font-size: 12px; font-weight: 600; letter-spacing: .02em; border-bottom: 1px solid #eceef1; white-space: nowrap; }
.cg-table td { padding: 15px 18px; border-bottom: 1px solid #f2f4f5; vertical-align: middle; }
.cg-table tr:last-child td { border-bottom: 0; }
.cg-row { transition: background .12s; }
.cg-row:hover { background: #f8f9fb; }
.cg-name-cell { min-width: 150px; }
.cg-name { display: flex; align-items: center; gap: 7px; font-weight: 600; color: #10151b; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cg-self-badge { flex-shrink: 0; padding: 1px 8px; color: #1d4ed8; background: #e8effd; border-radius: 999px; font-size: 10.5px; font-weight: 700; }
.cg-name-sub { margin-top: 3px; color: #9aa3ad; font-size: 11.5px; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cg-tenant-cell { color: #454f5b; }
.cg-tenant-inline { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
.cg-tenant-icon { color: #9aa3ad; flex-shrink: 0; }
.cg-time { color: #8b939d; font-size: 12px; white-space: nowrap; }
.cg-role-pills { display: flex; flex-wrap: wrap; gap: 5px; max-width: 240px; }
.cg-pill { display: inline-flex; align-items: center; padding: 3.5px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; white-space: nowrap; }
.pill-role-node_admin { color: #6d28d9; background: #f1eafd; }
.pill-role-tenant_admin { color: #1d4ed8; background: #e8effd; }
.pill-role-operator { color: #92400e; background: #fdf0e0; }
.pill-role-viewer { color: #4b5563; background: #f0f2f4; }
.pill-none { color: #9aa3ad; background: #f2f4f6; }
.cg-scope { color: #454f5b; font-size: 12.5px; cursor: help; }
.cg-scope-none { color: #b3bac2; font-size: 12.5px; }
.cg-status { display: inline-flex; align-items: center; gap: 7px; color: #454f5b; font-size: 12.5px; font-weight: 600; white-space: nowrap; }
.cg-table .col-actions { text-align: right; }
.cg-actions { display: inline-flex; gap: 2px; opacity: .5; transition: opacity .15s; }
.cg-row:hover .cg-actions, .cg-actions:focus-within { opacity: 1; }
.cg-act { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; color: #5d6772; background: transparent; border: 0; border-radius: 9px; cursor: pointer; transition: color .15s, background .15s; }
.cg-act:hover:not(:disabled) { color: #10151b; background: #f2f4f6; }
.cg-act.danger:hover:not(:disabled) { color: #b44444; background: #fdf3f3; }
.cg-act:disabled { cursor: not-allowed; opacity: .4; }
.cg-act:focus-visible { outline: 2px solid #10151b; outline-offset: 1px; }

/* =============== skeleton =============== */
.skeleton-row td { padding: 16px 18px; }
.sk { display: inline-block; background: linear-gradient(90deg, #f0f2f4 25%, #f7f8f9 37%, #f0f2f4 63%); background-size: 400% 100%; animation: sk-shimmer 1.3s ease infinite; border-radius: 6px; }
.sk-name { width: 120px; height: 13px; }
.sk-id { width: 150px; height: 12px; }
.sk-pill { width: 56px; height: 18px; border-radius: 999px; }
.sk-actions { width: 128px; height: 28px; }
@keyframes sk-shimmer { 0% { background-position: 100% 0; } 100% { background-position: -100% 0; } }

/* =============== alerts / empty =============== */
.cg-alert { display: flex; align-items: center; gap: 13px; margin-top: 16px; padding: 15px 18px; color: #a14444; background: #fdf2f2; border: 1px solid #f2d6d6; border-radius: 14px; }
.cg-alert svg { flex-shrink: 0; }
.cg-alert strong { font-size: 13.5px; }
.cg-alert p { margin: 3px 0 0; color: #b06565; font-size: 12.5px; }
.cg-alert .cg-btn-quiet { margin-left: auto; color: #a14444; }
.cg-alert .cg-btn-quiet:hover { color: #7c3535; background: #fbe7e7; }
.cg-alert.slim { display: block; margin: 0; padding: 10px 13px; font-size: 13px; font-weight: 600; }
.cg-empty { display: flex; flex-direction: column; align-items: center; gap: 6px; margin-top: 16px; padding: 56px 20px; text-align: center; background: #fff; border: 1px solid #eceef1; border-radius: 16px; }
.cg-empty-icon { display: inline-flex; align-items: center; justify-content: center; width: 54px; height: 54px; margin-bottom: 6px; color: #8b939d; background: #f4f6f8; border-radius: 999px; }
.cg-empty strong { font-size: 14.5px; color: #10151b; }
.cg-empty p { margin: 0 0 12px; color: #8b939d; font-size: 13px; }
.cg-empty.denied { margin-top: 28px; }

/* =============== pagination =============== */
.cg-pagination { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 14px; }
.cg-page-count { color: #8b939d; font-size: 12px; }
.cg-page-btns { display: flex; align-items: center; gap: 8px; }
.cg-page-info { color: #5d6772; font-size: 12px; }

/* =============== overlay / modal =============== */
.cg-overlay { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(7,11,17,.5); backdrop-filter: blur(3px); }
.cg-modal { width: 100%; max-width: 580px; max-height: 88vh; overflow-y: auto; background: #fff; border-radius: 20px; box-shadow: 0 30px 80px -16px rgba(4,9,18,.5), 0 4px 18px rgba(4,9,18,.2); }
.cg-modal-narrow { max-width: 460px; }
.cg-modal-head { display: flex; align-items: center; justify-content: space-between; padding: 20px 24px; border-bottom: 1px solid #f0f2f4; position: sticky; top: 0; background: #fff; z-index: 2; }
.cg-modal-head h2 { margin: 0; font-size: 17px; font-weight: 700; letter-spacing: -.01em; color: #10151b; display: flex; align-items: center; gap: 10px; }
.cg-user-badge { display: inline-block; max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 3px 10px; color: #454f5b; background: #f2f4f6; border-radius: 999px; font-size: 11.5px; font-weight: 700; }
.cg-form { padding: 22px 24px 24px; display: grid; gap: 16px; }
.cg-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.cg-field { display: grid; gap: 7px; }
.cg-field-full { grid-column: 1 / -1; }
.cg-field label, .cg-field-label { font-size: 12.5px; font-weight: 700; color: #454f5b; }
.cg-field input { width: 100%; height: 44px; padding: 0 13px; font: inherit; font-size: 14px; color: #10151b; background: #fff; border: 1px solid #c6ccd4; border-radius: 10px; box-sizing: border-box; transition: border-color .16s, box-shadow .16s; }
.cg-field input::placeholder { color: #9aa3ad; }
.cg-field input:hover { border-color: #a7aeb8; }
.cg-field input:focus-visible { outline: none; border-color: #10151b; box-shadow: 0 0 0 4px rgba(16,21,27,.09); }
.cg-select-full { width: 100%; height: 44px; padding: 0 12px; font: inherit; font-size: 14px; color: #10151b; background: #fff; border: 1px solid #c6ccd4; border-radius: 10px; cursor: pointer; transition: border-color .16s, box-shadow .16s; }
.cg-select-full:hover { border-color: #a7aeb8; }
.cg-select-full:focus-visible { outline: none; border-color: #10151b; box-shadow: 0 0 0 4px rgba(16,21,27,.09); }
.cg-input-wrap { position: relative; }
.cg-input-wrap input { padding-right: 92px; }
.cg-inset-btn { position: absolute; right: 6px; top: 5px; display: grid; place-items: center; width: 34px; height: 34px; color: #97a0ab; background: none; border: 0; border-radius: 8px; cursor: pointer; transition: color .15s, background .15s; }
.cg-inset-btn:hover { color: #2b333d; background: #f2f4f6; }
.cg-inset-btn:focus-visible { outline: 2px solid #10151b; outline-offset: 1px; }
.cg-inset-btn.far { right: 44px; }
.cg-hint { margin: 0; color: #8b939d; font-size: 12px; }
.cg-modal-actions { display: flex; justify-content: flex-end; align-items: center; gap: 8px; margin-top: 4px; }

/* =============== role selection =============== */
.cg-divider { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; margin-top: 2px; color: #9aa3ad; font-size: 11px; font-weight: 700; letter-spacing: .04em; }
.cg-divider::after { content: ''; flex: 1; height: 1px; background: #eef0f3; }
.cg-role-row { display: flex; flex-wrap: wrap; gap: 8px; }
.cg-role-pill { position: relative; display: inline-flex; align-items: center; padding: 8px 15px; background: #fff; border: 1px solid #e3e6ea; border-radius: 999px; font-size: 13px; font-weight: 600; color: #454f5b; cursor: pointer; transition: color .15s, border-color .15s, background .15s; }
.cg-role-pill:hover { border-color: #a7aeb8; }
.cg-role-pill:focus-within { outline: 2px solid #10151b; outline-offset: 1px; }
.cg-role-pill.on { color: #fff; background: #0f141a; border-color: #0f141a; }
.cg-role-pill input { position: absolute; opacity: 0; width: 1px; height: 1px; }

/* =============== region picker =============== */
.cg-region-picker { display: grid; gap: 1px; max-height: 216px; overflow-y: auto; padding: 5px; background: #fafbfc; border: 1px solid #eef0f3; border-radius: 12px; }
.cg-region { display: flex; align-items: center; gap: 9px; padding: 7px 10px; border-radius: 8px; font-size: 13px; cursor: pointer; }
.cg-region:hover { background: #f2f4f6; }
.cg-region:focus-within { outline: 2px solid #10151b; outline-offset: -2px; }
.cg-region input { accent-color: #0f141a; margin: 0; flex-shrink: 0; width: 15px; height: 15px; }
.cg-region-name { font-weight: 600; color: #10151b; white-space: nowrap; }
.cg-region-path { color: #9aa3ad; font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* =============== modal transitions =============== */
.modal-enter-active, .modal-leave-active { transition: opacity .2s; }
.modal-enter-active .cg-modal, .modal-leave-active .cg-modal { transition: transform .2s cubic-bezier(.22,1,.36,1), opacity .2s; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-from .cg-modal, .modal-leave-to .cg-modal { transform: translateY(16px) scale(.98); opacity: 0; }

/* =============== misc =============== */
.mono { font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 12px; font-variant-numeric: tabular-nums; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* =============== reduced motion =============== */
@media (prefers-reduced-motion: reduce) {
  .sk, .spinning { animation: none; }
  .cg-row, .cg-btn-primary, .cg-act, .cg-icon-btn, .cg-btn-quiet, .page-nav-link, .cg-role-pill, .cg-search, .cg-select, .cg-field input { transition: none; }
  .modal-enter-active, .modal-leave-active, .toast-enter-active, .toast-leave-active { transition: none; }
  .modal-enter-from, .modal-leave-to, .toast-enter-from, .toast-leave-to { opacity: 1; transform: none; }
  .toast-enter-from, .toast-leave-to { transform: translate(-50%, 0); }
}

/* =============== responsive =============== */
@media (max-width: 900px) {
  .page-topbar { gap: 14px; padding: 0 16px; }
  .page-brand-text span { display: none; }
  .cg-main { padding: 28px 16px 48px; }
  .cg-head { flex-direction: column; align-items: stretch; }
  .cg-head .cg-btn-primary { align-self: flex-start; }
  .cg-stats { grid-template-columns: 1fr 1fr; }
  .cg-stat + .cg-stat { border-left: 0; }
  .cg-stat:nth-child(even) { border-left: 1px solid #f0f2f4; }
  .cg-stat:nth-child(n+3) { border-top: 1px solid #f0f2f4; }
  .cg-toolbar { flex-wrap: wrap; }
  .cg-search { max-width: none; flex-basis: 100%; }
  .cg-refresh { margin-left: 0; }
  .cg-card.cg-table-wrap { overflow-x: auto; }
  .cg-table { min-width: 860px; }
  .cg-form-grid { grid-template-columns: 1fr; }
  .cg-modal { border-radius: 16px; }
}
</style>
