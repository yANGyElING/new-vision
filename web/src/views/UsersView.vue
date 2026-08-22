<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  AlertTriangle, Building2, Check, ChevronDown, ChevronLeft, ChevronRight, Edit3, Eye, EyeOff,
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
  closeAllPopovers()
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
  closeAllPopovers()
  userModalOpen.value = true
}

function closeUserModal() {
  userModalOpen.value = false
  userFormError.value = ''
  closeAllPopovers()
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

// ---------- comboboxes (Notion style: search-in-popover, no create) ----------
const roleOpen = ref(false)
const roleQuery = ref('')
const roleSearchEl = ref<HTMLInputElement | null>(null)
const tenantOpen = ref(false)
const statusOpen = ref(false)
const regionOpen = ref(false)
const regionQuery = ref('')
const regionSearchEl = ref<HTMLInputElement | null>(null)

const STATUS_OPTIONS: { value: 'active' | 'disabled'; label: string }[] = [
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '停用' },
]

const roleOptions = computed(() => [
  { value: '', label: '暂不分配', hint: '用户可登录，暂无功能权限' },
  ...availableRoles.value.map((r) => ({ value: r, label: roleLabel(r), hint: roleHint(r) })),
])

const filteredRoles = computed(() => {
  const q = roleQuery.value.trim().toLowerCase()
  if (!q) return roleOptions.value
  return roleOptions.value.filter((o) => o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q))
})

const filteredRegions = computed(() => {
  const q = regionQuery.value.trim().toLowerCase()
  if (!q) return flatRegions.value
  return flatRegions.value.filter((f) => f.region.name.toLowerCase().includes(q) || f.path.toLowerCase().includes(q))
})

function regionNameOf(id: string): string {
  return flatRegions.value.find((f) => f.region.id === id)?.region.name ?? id
}

function tenantLabel(id: string): string {
  return tenantNameByID.value.get(id) ?? id
}

function closeAllPopovers() {
  roleOpen.value = false
  tenantOpen.value = false
  statusOpen.value = false
  regionOpen.value = false
  roleQuery.value = ''
  regionQuery.value = ''
}

function pickFirstRole() {
  const first = filteredRoles.value[0]
  if (first) {
    pickRole(first.value)
    roleOpen.value = false
  }
}

watch(roleOpen, async (open) => {
  if (open) { await nextTick(); roleSearchEl.value?.focus() } else roleQuery.value = ''
})
watch(regionOpen, async (open) => {
  if (open) { await nextTick(); regionSearchEl.value?.focus() } else regionQuery.value = ''
})

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
  document.addEventListener('click', closeAllPopovers)
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
  document.removeEventListener('click', closeAllPopovers)
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

    <!-- ============ user create / edit modal (Notion style) ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="userModalOpen" class="cg-overlay" @click.self="closeUserModal">
          <div class="nt-modal" role="dialog" aria-modal="true" :aria-label="userModalMode === 'create' ? '新增用户' : '编辑用户'">
            <div class="nt-modal-head">
              <div>
                <h2>{{ userModalMode === 'create' ? '新增用户' : '编辑用户' }}</h2>
                <p class="nt-sub">{{ userModalMode === 'create' ? '创建后可随时调整角色与数据范围' : `调整 ${userForm.username} 的账号信息与权限` }}</p>
              </div>
              <button class="nt-x" type="button" aria-label="关闭" @click="closeUserModal"><X :size="15" /></button>
            </div>
            <form class="nt-form" @submit.prevent="submitUserForm" @click.stop>
              <div v-if="userModalMode === 'create'" class="nt-row">
                <div class="nt-label">用户名</div>
                <div class="nt-ctrl">
                  <input id="usr-username" v-model="userForm.username" class="nt-input" aria-label="用户名" required maxlength="64" autocomplete="off" placeholder="登录用户名">
                </div>
              </div>

              <div v-if="userModalMode === 'create' && tenants.length > 1" class="nt-row">
                <div class="nt-label">所属租户</div>
                <div class="nt-ctrl">
                  <button type="button" class="nt-combo" :class="{ open: tenantOpen }" aria-haspopup="listbox" :aria-expanded="tenantOpen" @click.stop="closeAllPopovers(); tenantOpen = !tenantOpen">
                    <span class="nt-val">{{ tenantLabel(userForm.tenant_id) }}</span>
                    <ChevronDown :size="13" />
                  </button>
                  <Transition name="ntpop">
                    <div v-if="tenantOpen" class="nt-pop" @click.stop>
                      <div class="nt-list" role="listbox">
                        <button v-for="t in tenants" :key="t.id" type="button" role="option" class="nt-opt" :class="{ sel: userForm.tenant_id === t.id }" @click="userForm.tenant_id = t.id; tenantOpen = false">
                          <Check :size="14" class="nt-ck" />{{ t.name }}
                        </button>
                      </div>
                    </div>
                  </Transition>
                </div>
              </div>

              <div class="nt-row">
                <div class="nt-label">显示名</div>
                <div class="nt-ctrl">
                  <input id="usr-display" v-model="userForm.display_name" class="nt-input" aria-label="显示名" maxlength="255" placeholder="选填">
                </div>
              </div>

              <div v-if="userModalMode === 'edit'" class="nt-row">
                <div class="nt-label">账号状态</div>
                <div class="nt-ctrl">
                  <button type="button" class="nt-combo" :class="{ open: statusOpen }" aria-haspopup="listbox" :aria-expanded="statusOpen" @click.stop="closeAllPopovers(); statusOpen = !statusOpen">
                    <span class="nt-val">{{ userForm.status === 'active' ? '启用' : '停用' }}</span>
                    <ChevronDown :size="13" />
                  </button>
                  <Transition name="ntpop">
                    <div v-if="statusOpen" class="nt-pop" @click.stop>
                      <div class="nt-list" role="listbox">
                        <button v-for="s in STATUS_OPTIONS" :key="s.value" type="button" role="option" class="nt-opt" :class="{ sel: userForm.status === s.value }" @click="userForm.status = s.value; statusOpen = false">
                          <Check :size="14" class="nt-ck" />{{ s.label }}
                        </button>
                      </div>
                    </div>
                  </Transition>
                </div>
              </div>

              <div v-if="userModalMode === 'create'" class="nt-row">
                <div class="nt-label">初始密码</div>
                <div class="nt-ctrl nt-pw">
                  <input id="usr-password" v-model="userForm.password" class="nt-input" aria-label="初始密码" :type="passwordVisible ? 'text' : 'password'" required maxlength="256" autocomplete="new-password" placeholder="点击右侧钥匙生成">
                  <button class="nt-ib eye" type="button" :aria-label="passwordVisible ? '隐藏密码' : '显示密码'" :title="passwordVisible ? '隐藏密码' : '显示密码'" @click="passwordVisible = !passwordVisible">
                    <Eye v-if="passwordVisible" :size="14" /><EyeOff v-else :size="14" />
                  </button>
                  <button class="nt-ib gen" type="button" aria-label="生成随机密码" title="生成随机密码" @click="generatePassword"><KeyRound :size="14" /></button>
                </div>
              </div>

              <div class="nt-row">
                <div class="nt-label">角色</div>
                <div class="nt-ctrl">
                  <button type="button" class="nt-combo" :class="{ open: roleOpen }" aria-haspopup="listbox" :aria-expanded="roleOpen" @click.stop="closeAllPopovers(); roleOpen = !roleOpen">
                    <span class="nt-val">{{ currentRole ? roleLabel(currentRole) : '暂不分配' }}</span>
                    <ChevronDown :size="13" />
                  </button>
                  <Transition name="ntpop">
                    <div v-if="roleOpen" class="nt-pop" @click.stop>
                      <div class="nt-search">
                        <Search :size="13" />
                        <input v-model="roleQuery" aria-label="搜索角色" placeholder="搜索角色…" @keydown.enter.prevent="pickFirstRole" @keydown.esc="roleOpen = false">
                      </div>
                      <div class="nt-list" role="listbox">
                        <button v-for="o in filteredRoles" :key="o.value || 'none'" type="button" role="option" class="nt-opt" :class="{ sel: currentRole === o.value }" @click="pickRole(o.value); roleOpen = false">
                          <Check :size="14" class="nt-ck" />{{ o.label }}<span class="nt-opt-hint">{{ o.hint }}</span>
                        </button>
                        <div v-if="filteredRoles.length === 0" class="nt-empty">无匹配角色</div>
                      </div>
                    </div>
                  </Transition>
                </div>
              </div>

              <div class="nt-row">
                <div class="nt-label">区域范围</div>
                <div class="nt-ctrl">
                  <div class="nt-multi" :class="{ open: regionOpen }" role="button" tabindex="0" aria-haspopup="listbox" :aria-expanded="regionOpen" @click.stop="closeAllPopovers(); regionOpen = !regionOpen" @keydown.enter.prevent="closeAllPopovers(); regionOpen = !regionOpen">
                    <span v-for="id in userForm.region_ids" :key="id" class="nt-chip">
                      {{ regionNameOf(id) }}
                      <button type="button" :aria-label="`移除 ${regionNameOf(id)}`" @click.stop="toggleRegionScope(id)"><X :size="10" /></button>
                    </span>
                    <span v-if="userForm.region_ids.length === 0" class="nt-ph">选择区域，可多选</span>
                  </div>
                  <Transition name="ntpop">
                    <div v-if="regionOpen" class="nt-pop" @click.stop>
                      <div class="nt-search">
                        <Search :size="13" />
                        <input v-model="regionQuery" aria-label="搜索区域" placeholder="搜索区域…" @keydown.esc="regionOpen = false">
                      </div>
                      <div class="nt-list" role="listbox">
                        <button v-for="f in filteredRegions" :key="f.region.id" type="button" role="option" class="nt-opt" :class="{ sel: userForm.region_ids.includes(f.region.id) }" @click="toggleRegionScope(f.region.id)">
                          <Check :size="14" class="nt-ck" />{{ f.region.name }}<span class="nt-opt-hint">{{ f.path }}</span>
                        </button>
                        <div v-if="filteredRegions.length === 0" class="nt-empty">无匹配区域</div>
                      </div>
                    </div>
                  </Transition>
                  <p v-if="flatRegions.length === 0" class="nt-note">暂无区域可分配，可先在「组织架构」页创建。</p>
                </div>
              </div>

              <p v-if="userFormError" class="nt-error" role="alert">{{ userFormError }}</p>
              <div class="nt-foot">
                <button class="nt-btn ghost" type="button" @click="closeUserModal">取消</button>
                <button class="nt-btn go" type="submit" :disabled="userSaving">{{ userSaving ? '保存中…' : userModalMode === 'create' ? '创建用户' : '保存修改' }}</button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============ reset password modal (Notion style) ============ -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="passwordModalUser" class="cg-overlay" @click.self="passwordModalUser = null">
          <div class="nt-modal nt-modal-narrow" role="dialog" aria-modal="true" :aria-label="`重置 ${passwordModalUser.username} 的密码`">
            <div class="nt-modal-head">
              <div>
                <h2>重置密码</h2>
                <p class="nt-sub">{{ passwordModalUser.username }} · 重置后原密码立即失效</p>
              </div>
              <button class="nt-x" type="button" aria-label="关闭" @click="passwordModalUser = null"><X :size="15" /></button>
            </div>
            <form class="nt-form" @submit.prevent="submitPassword" @click.stop>
              <div class="nt-row">
                <div class="nt-label">新密码</div>
                <div class="nt-ctrl nt-pw">
                  <input id="usr-new-password" v-model="newPassword" class="nt-input" aria-label="新密码" :type="newPasswordVisible ? 'text' : 'password'" required maxlength="256" autocomplete="new-password" placeholder="点击右侧钥匙生成">
                  <button class="nt-ib eye" type="button" :aria-label="newPasswordVisible ? '隐藏密码' : '显示密码'" :title="newPasswordVisible ? '隐藏密码' : '显示密码'" @click="newPasswordVisible = !newPasswordVisible">
                    <Eye v-if="newPasswordVisible" :size="14" /><EyeOff v-else :size="14" />
                  </button>
                  <button class="nt-ib gen" type="button" aria-label="生成随机密码" title="生成随机密码" @click="generateAdminPassword"><KeyRound :size="14" /></button>
                </div>
              </div>
              <p v-if="passwordError" class="nt-error" role="alert">{{ passwordError }}</p>
              <div class="nt-foot">
                <button class="nt-btn ghost" type="button" @click="passwordModalUser = null">取消</button>
                <button class="nt-btn go" type="submit" :disabled="passwordSaving">{{ passwordSaving ? '重置中…' : '重置密码' }}</button>
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

/* =============== overlay =============== */
.cg-overlay { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(15,15,15,.45); backdrop-filter: blur(3px); }

/* =============== modal (Notion style) =============== */
.nt-modal { width: 100%; max-width: 620px; max-height: 88vh; overflow-y: auto; background: #fff; border: 1px solid #e9e9e7; border-radius: 10px; box-shadow: 0 24px 64px -16px rgba(15,15,15,.28), 0 4px 16px rgba(15,15,15,.08); }
.nt-modal-narrow { max-width: 480px; }
.nt-modal-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; padding: 24px 28px 0; position: sticky; top: 0; background: #fff; z-index: 2; }
.nt-modal-head h2 { margin: 0; font-size: 17px; font-weight: 600; letter-spacing: -.01em; color: #37352f; }
.nt-sub { margin: 4px 0 0; font-size: 12.5px; color: #787774; }
.nt-x { display: grid; place-items: center; width: 28px; height: 28px; flex-shrink: 0; color: #9b9a97; background: none; border: 0; border-radius: 6px; cursor: pointer; transition: color .12s, background .12s; }
.nt-x:hover { color: #37352f; background: #f1f1ef; }
.nt-form { padding: 20px 28px 24px; display: grid; gap: 18px; }
.nt-row { display: flex; align-items: flex-start; gap: 18px; }
.nt-label { flex: 0 0 120px; padding-top: 7px; font-size: 13.5px; font-weight: 500; color: #37352f; }
.nt-ctrl { flex: 1; min-width: 0; position: relative; }

.nt-input { width: 100%; height: 34px; padding: 0 10px; font: inherit; font-size: 13.5px; color: #37352f; background: #fff; border: 1px solid #e3e2e0; border-radius: 6px; box-sizing: border-box; transition: border-color .12s, box-shadow .12s; }
.nt-input::placeholder { color: #b9b8b4; }
.nt-input:hover { border-color: #d3d1cb; }
.nt-input:focus-visible { outline: none; border-color: #b3d4f2; box-shadow: 0 0 0 3px rgba(35,131,226,.14); }
.nt-pw { position: relative; }
.nt-pw .nt-input { padding-right: 74px; }
.nt-ib { position: absolute; top: 4px; display: grid; place-items: center; width: 26px; height: 26px; color: #9b9a97; background: none; border: 0; border-radius: 5px; cursor: pointer; transition: color .12s, background .12s; }
.nt-ib:hover { color: #37352f; background: #f1f1ef; }
.nt-ib:focus-visible { outline: 2px solid #37352f; outline-offset: 1px; }
.nt-ib.eye { right: 5px; }
.nt-ib.gen { right: 34px; }

.nt-combo { display: flex; align-items: center; justify-content: space-between; gap: 8px; width: 100%; height: 34px; padding: 0 8px 0 10px; font: inherit; font-size: 13.5px; color: #37352f; text-align: left; background: #fff; border: 1px solid #e3e2e0; border-radius: 6px; cursor: pointer; transition: border-color .12s, box-shadow .12s; }
.nt-combo:hover { border-color: #d3d1cb; }
.nt-combo.open { border-color: #b3d4f2; box-shadow: 0 0 0 3px rgba(35,131,226,.14); }
.nt-combo:focus-visible { outline: none; border-color: #b3d4f2; box-shadow: 0 0 0 3px rgba(35,131,226,.14); }
.nt-combo .nt-val { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.nt-combo svg { flex-shrink: 0; color: #9b9a97; }

.nt-pop { position: absolute; top: calc(100% + 6px); left: 0; right: 0; z-index: 5; background: #fff; border-radius: 6px; box-shadow: 0 0 0 1px rgba(15,15,15,.04), 0 3px 6px rgba(15,15,15,.1), 0 12px 28px -6px rgba(15,15,15,.18); }
.nt-search { display: flex; align-items: center; gap: 7px; padding: 8px 10px; color: #9b9a97; border-bottom: 1px solid #f1f1ef; }
.nt-search input { flex: 1; height: 28px; padding: 0 8px; font: inherit; font-size: 13px; color: #37352f; background: #f7f7f6; border: 0; border-radius: 5px; }
.nt-search input:focus { outline: 2px solid rgba(35,131,226,.4); outline-offset: -1px; }
.nt-search input::placeholder { color: #b9b8b4; }
.nt-list { max-height: 216px; overflow-y: auto; padding: 5px; }
.nt-opt { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 8px; font: inherit; font-size: 13px; color: #37352f; text-align: left; background: none; border: 0; border-radius: 4px; cursor: pointer; }
.nt-opt:hover { background: #f7f7f6; }
.nt-opt .nt-ck { visibility: hidden; color: #2383e2; flex-shrink: 0; }
.nt-opt.sel .nt-ck { visibility: visible; }
.nt-opt-hint { margin-left: auto; flex-shrink: 0; font-size: 11px; color: #9b9a97; white-space: nowrap; }
.nt-empty { padding: 12px 10px; font-size: 12px; color: #9b9a97; }

.nt-multi { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; width: 100%; min-height: 34px; padding: 4px 8px; background: #fff; border: 1px solid #e3e2e0; border-radius: 6px; cursor: pointer; transition: border-color .12s, box-shadow .12s; }
.nt-multi:hover { border-color: #d3d1cb; }
.nt-multi.open { border-color: #b3d4f2; box-shadow: 0 0 0 3px rgba(35,131,226,.14); }
.nt-multi:focus-visible { outline: none; border-color: #b3d4f2; box-shadow: 0 0 0 3px rgba(35,131,226,.14); }
.nt-chip { display: inline-flex; align-items: center; gap: 5px; padding: 2px 6px 2px 8px; font-size: 12px; color: #37352f; background: #f1f1ef; border-radius: 4px; }
.nt-chip button { display: grid; place-items: center; width: 16px; height: 16px; color: #787774; background: none; border: 0; border-radius: 3px; cursor: pointer; }
.nt-chip button:hover { color: #37352f; background: #e3e2e0; }
.nt-ph { font-size: 13px; color: #b9b8b4; padding-left: 2px; }
.nt-note { margin: 6px 0 0; font-size: 12px; color: #9b9a97; }

.nt-error { margin: 0; padding: 9px 12px; font-size: 12.5px; font-weight: 500; color: #c4564e; background: #fdf4f3; border-radius: 6px; }
.nt-foot { display: flex; justify-content: flex-end; gap: 10px; margin-top: 6px; }
.nt-btn { display: inline-flex; align-items: center; justify-content: center; height: 34px; padding: 0 16px; font: inherit; font-size: 13px; font-weight: 500; border-radius: 6px; cursor: pointer; transition: background .12s; }
.nt-btn.ghost { color: #37352f; background: #f1f1ef; border: 0; }
.nt-btn.ghost:hover { background: #e8e7e4; }
.nt-btn.go { color: #fff; background: #2383e2; border: 0; }
.nt-btn.go:hover:not(:disabled) { background: #1b74c9; }
.nt-btn.go:disabled { cursor: wait; opacity: .7; }
.nt-btn:focus-visible { outline: 2px solid #37352f; outline-offset: 1px; }

/* =============== modal transitions =============== */
.modal-enter-active, .modal-leave-active { transition: opacity .2s; }
.modal-enter-active .nt-modal, .modal-leave-active .nt-modal { transition: transform .2s cubic-bezier(.22,1,.36,1), opacity .2s; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-from .nt-modal, .modal-leave-to .nt-modal { transform: translateY(16px) scale(.98); opacity: 0; }
.ntpop-enter-active, .ntpop-leave-active { transition: opacity .12s, transform .12s; }
.ntpop-enter-from, .ntpop-leave-to { opacity: 0; transform: translateY(-4px); }

/* =============== misc =============== */
.mono { font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 12px; font-variant-numeric: tabular-nums; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* =============== reduced motion =============== */
@media (prefers-reduced-motion: reduce) {
  .sk, .spinning { animation: none; }
  .cg-row, .cg-btn-primary, .cg-act, .cg-icon-btn, .cg-btn-quiet, .page-nav-link, .cg-search, .cg-select, .nt-input, .nt-combo, .nt-multi, .nt-x, .nt-ib, .nt-btn { transition: none; }
  .modal-enter-active, .modal-leave-active, .toast-enter-active, .toast-leave-active, .ntpop-enter-active, .ntpop-leave-active { transition: none; }
  .modal-enter-from, .modal-leave-to, .toast-enter-from, .toast-leave-to, .ntpop-enter-from, .ntpop-leave-to { opacity: 1; transform: none; }
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
  .nt-form { padding: 16px 20px 20px; }
  .nt-modal-head { padding: 20px 20px 0; }
  .nt-modal { border-radius: 8px; }
}
@media (max-width: 560px) {
  .nt-row { flex-direction: column; gap: 6px; }
  .nt-label { flex-basis: auto; padding-top: 0; }
}
</style>
