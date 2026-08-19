<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Activity, AlertTriangle, Check, CircleHelp, Clock3, Database, LogIn, RefreshCw, Server, WifiOff, X } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { fetchHealth, type CheckState, type HealthState } from '@/api/health'

const state = ref<HealthState>({ kind: 'loading' })
const refreshing = ref(false)
let requestId = 0
let activeController: AbortController | undefined

const title = computed(() => {
  switch (state.value.kind) {
    case 'ready': return '运行正常'
    case 'degraded': return '依赖异常'
    case 'unreachable': return '无法连接'
    case 'invalid': return '响应异常'
    default: return '检查中'
  }
})

const summary = computed(() => {
  switch (state.value.kind) {
    case 'ready': return '节点服务和必要依赖均已就绪。'
    case 'degraded': return '节点仍在运行，但至少一个必要依赖不可用。'
    case 'unreachable':
    case 'invalid': return state.value.message
    default: return '正在读取节点当前状态。'
  }
})

const checkedAt = computed(() => {
  if (state.value.kind === 'loading') return ''
  return state.value.checkedAt.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
})

function checkState(name: 'postgres' | 'redis'): CheckState | 'unknown' {
  if (state.value.kind === 'ready' || state.value.kind === 'degraded') return state.value.health.checks[name]
  return 'unknown'
}

async function refresh() {
  const currentId = ++requestId
  activeController?.abort()
  activeController = new AbortController()
  refreshing.value = true
  state.value = { kind: 'loading' }
  const result = await fetchHealth(4000, activeController.signal)
  if (currentId === requestId) {
    state.value = result
    refreshing.value = false
  }
}

onMounted(() => {
  void refresh()
})
onUnmounted(() => activeController?.abort())
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <RouterLink class="brand" to="/" aria-label="返回节点状态">
        <span class="brand-mark"><Activity :size="18" :stroke-width="2.2" /></span>
        <span>new-vision</span>
      </RouterLink>
      <nav aria-label="主导航">
        <RouterLink class="nav-link" to="/login"><LogIn :size="16" />认证入口</RouterLink>
      </nav>
    </header>

    <section class="content" aria-labelledby="page-title">
      <div class="eyebrow">自治节点 · 运行概览</div>
      <div class="heading-row">
        <div>
          <h1 id="page-title">节点状态</h1>
          <p class="lede">查看核心服务与运行依赖的即时状态。</p>
        </div>
        <button class="icon-button" type="button" :disabled="refreshing" aria-label="刷新节点状态" title="刷新" @click="refresh">
          <RefreshCw :size="18" :class="{ spinning: refreshing }" />
        </button>
      </div>

      <section class="status-band" :class="`status-${state.kind}`" aria-live="polite">
        <div class="status-icon">
          <Check v-if="state.kind === 'ready'" :size="28" />
          <AlertTriangle v-else-if="state.kind === 'degraded'" :size="28" />
          <WifiOff v-else-if="state.kind === 'unreachable' || state.kind === 'invalid'" :size="28" />
          <RefreshCw v-else :size="28" class="spinning" />
        </div>
        <div class="status-copy">
          <span class="status-label">当前状态</span>
          <strong>{{ title }}</strong>
          <p>{{ summary }}</p>
        </div>
        <div v-if="checkedAt" class="checked-at"><Clock3 :size="15" />{{ checkedAt }}</div>
      </section>

      <section class="dependencies" aria-labelledby="dependencies-title">
        <div class="section-heading">
          <div>
            <div class="eyebrow">基础设施</div>
            <h2 id="dependencies-title">必要依赖</h2>
          </div>
          <span class="section-note">实时探测</span>
        </div>
        <div class="dependency-list">
          <div class="dependency-row">
            <div class="dependency-icon"><Database :size="19" /></div>
            <div class="dependency-copy"><strong>PostgreSQL</strong><span>持久化数据服务</span></div>
            <span class="state-pill" :class="`pill-${checkState('postgres')}`">
              <Check v-if="checkState('postgres') === 'up'" :size="14" />
              <X v-else-if="checkState('postgres') === 'down'" :size="14" />
              <CircleHelp v-else :size="14" />
              {{ checkState('postgres') === 'up' ? '正常' : checkState('postgres') === 'down' ? '不可用' : '待检查' }}
            </span>
          </div>
          <div class="dependency-row">
            <div class="dependency-icon"><Server :size="19" /></div>
            <div class="dependency-copy"><strong>Redis</strong><span>运行状态服务</span></div>
            <span class="state-pill" :class="`pill-${checkState('redis')}`">
              <Check v-if="checkState('redis') === 'up'" :size="14" />
              <X v-else-if="checkState('redis') === 'down'" :size="14" />
              <CircleHelp v-else :size="14" />
              {{ checkState('redis') === 'up' ? '正常' : checkState('redis') === 'down' ? '不可用' : '待检查' }}
            </span>
          </div>
        </div>
      </section>
    </section>
  </main>
</template>
