<template>
  <main class="shell centered-page">
    <section class="panel" aria-labelledby="login-title">
      <div class="panel-header">
        <div class="eyebrow">身份认证</div>
        <h1 id="login-title">节点登录</h1>
      </div>
      <form class="form-stack" @submit.prevent="submit">
        <label class="field">
          <span>租户</span>
          <input v-model="tenant" name="tenant" type="text" autocomplete="organization" placeholder="default" required />
        </label>
        <label class="field">
          <span>用户名</span>
          <input v-model="username" name="username" type="text" autocomplete="username" required />
        </label>
        <label class="field">
          <span>密码</span>
          <input v-model="password" name="password" type="password" autocomplete="current-password" required />
        </label>
        <p v-if="error" class="form-error" role="alert">{{ error }}</p>
        <button class="primary-button" type="submit" :disabled="loading">
          {{ loading ? '登录中…' : '登录' }}
        </button>
      </form>
      <RouterLink class="primary-link" to="/"><ArrowLeft :size="16" />返回节点状态</RouterLink>
    </section>
  </main>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { loginAndStore } from '@/api/auth'

const tenant = ref('default')
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

const route = useRoute()
const router = useRouter()

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await loginAndStore(tenant.value.trim(), username.value.trim(), password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/devices'
    await router.replace(redirect)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '登录失败'
  } finally {
    loading.value = false
  }
}
</script>
