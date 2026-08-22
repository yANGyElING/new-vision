<template>
  <main class="login-shell">
    <div class="login-bg-wrap" :style="bgStyle">
      <div class="login-bg" :class="{ 'is-loaded': bgLoaded }" :style="{ backgroundImage: `url(${bgUrl})` }" />
    </div>
    <div class="login-veil" />
    <div class="login-glow" />

    <section class="login-card" aria-labelledby="login-title">
      <div class="login-mark rise d1">
        <Monitor :size="19" :stroke-width="2.2" />
      </div>
      <h1 id="login-title" class="login-title rise d2">欢迎回来</h1>
      <p class="login-sub rise d2">登录以管理你的 GB/T 28181 视频接入节点</p>

      <form class="login-form" @submit.prevent="submit">
        <div class="login-field rise d3">
          <label for="login-tenant">租户</label>
          <input
            id="login-tenant"
            v-model="tenant"
            name="tenant"
            type="text"
            autocomplete="organization"
            placeholder="default"
            spellcheck="false"
            required
          />
        </div>

        <div class="login-field rise d3">
          <label for="login-username">用户名</label>
          <input
            id="login-username"
            ref="usernameInput"
            v-model="username"
            name="username"
            type="text"
            autocomplete="username"
            placeholder="请输入用户名"
            spellcheck="false"
            required
          />
        </div>

        <div class="login-field rise d4">
          <label for="login-password">密码</label>
          <div class="login-password">
            <input
              id="login-password"
              v-model="password"
              name="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
              required
            />
            <button
              type="button"
              class="login-eye"
              :aria-label="showPassword ? '隐藏密码' : '显示密码'"
              :aria-pressed="showPassword"
              @click="togglePassword"
            >
              <EyeOff v-if="showPassword" :size="17" />
              <Eye v-else :size="17" />
            </button>
          </div>
        </div>

        <p v-if="error" :key="shakeKey" class="login-error" role="alert">
          <AlertCircle :size="15" />
          {{ error }}
        </p>

        <button class="login-submit rise d5" type="submit" :disabled="loading">
          <Loader2 v-if="loading" class="login-spin" :size="17" />
          {{ loading ? '登录中…' : '登录' }}
        </button>

        <p class="login-help rise d5">忘记密码？请联系节点管理员重置</p>
      </form>
    </section>

    <footer class="login-foot rise d6">
      <span>GB/T 28181 视频接入节点</span>
      <span class="login-foot-dot" aria-hidden="true" />
      <span>New Vision</span>
    </footer>
  </main>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertCircle, Eye, EyeOff, Loader2, Monitor } from 'lucide-vue-next'
import { loginAndStore } from '@/api/auth'
// City night skyline — Pexels photo 18441167, free to use under the Pexels license.
import bgUrl from '@/assets/login-bg.jpg'

const tenant = ref('default')
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)
const showPassword = ref(false)
const shakeKey = ref(0)
const bgLoaded = ref(false)
const usernameInput = ref<HTMLInputElement | null>(null)
const bgShift = reactive({ x: 0, y: 0 })

const route = useRoute()
const router = useRouter()

const bgStyle = reactive({ transform: 'translate3d(0,0,0)' })

function togglePassword() {
  showPassword.value = !showPassword.value
}

function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function handlePointerMove(event: PointerEvent) {
  if (prefersReducedMotion()) return
  const dx = event.clientX / window.innerWidth - 0.5
  const dy = event.clientY / window.innerHeight - 0.5
  bgShift.x = Math.round(dx * -12)
  bgShift.y = Math.round(dy * -8)
  bgStyle.transform = `translate3d(${bgShift.x}px, ${bgShift.y}px, 0)`
}

function normalizeError(e: unknown): string {
  const message = e instanceof Error ? e.message : ''
  if (!message || /failed to fetch|network|load failed|^404/i.test(message)) {
    return '无法连接节点服务，请检查网络后重试'
  }
  if (/401|credential|unauthorized|invalid/i.test(message)) {
    return '租户、用户名或密码不正确'
  }
  return message
}

async function submit() {
  if (loading.value) return
  error.value = ''
  loading.value = true
  try {
    await loginAndStore(tenant.value.trim(), username.value.trim(), password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/devices'
    await router.replace(redirect)
  } catch (e) {
    error.value = normalizeError(e)
    shakeKey.value++
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  usernameInput.value?.focus()
  const img = new Image()
  img.onload = () => {
    bgLoaded.value = true
  }
  img.onerror = () => {
    bgLoaded.value = true
  }
  img.src = bgUrl
  if (img.complete) bgLoaded.value = true
  window.addEventListener('pointermove', handlePointerMove, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', handlePointerMove)
})
</script>

<style scoped>
.login-shell {
  position: relative;
  min-height: 100vh;
  min-height: 100svh;
  display: grid;
  place-items: center;
  padding: 40px 20px 76px;
  background: #0a0f16;
  overflow: hidden;
}

/* --- background layers --- */
.login-bg-wrap {
  position: absolute;
  inset: -28px;
  transition: transform 0.9s cubic-bezier(0.2, 0.6, 0.3, 1);
  will-change: transform;
}
.login-bg {
  width: 100%;
  height: 100%;
  background-color: #0a0f16;
  background-size: cover;
  background-position: center;
  filter: saturate(0.92);
  opacity: 0;
  transform: scale(1.08);
  transition: opacity 1s ease, transform 9s cubic-bezier(0.22, 1, 0.36, 1);
}
.login-bg.is-loaded {
  opacity: 1;
  transform: scale(1);
}
.login-veil {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    linear-gradient(180deg, rgba(7, 11, 17, 0.62) 0%, rgba(7, 11, 17, 0.36) 42%, rgba(7, 11, 17, 0.5) 74%, rgba(7, 11, 17, 0.78) 100%),
    radial-gradient(130% 100% at 50% 0%, rgba(16, 28, 48, 0.35), transparent 55%);
}
.login-glow {
  position: absolute;
  left: 50%;
  top: 44%;
  width: 760px;
  height: 560px;
  transform: translate(-50%, -50%);
  pointer-events: none;
  background: radial-gradient(closest-side, rgba(148, 178, 225, 0.15), transparent 72%);
}

/* --- card --- */
.login-card {
  position: relative;
  z-index: 2;
  width: min(408px, 100%);
  padding: 34px 34px 26px;
  text-align: center;
  background: #fff;
  border-radius: 22px;
  box-shadow: 0 30px 80px -16px rgba(4, 9, 18, 0.6), 0 4px 18px rgba(4, 9, 18, 0.28);
}
.login-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  color: #fff;
  background: linear-gradient(135deg, #2d3742, #1c242d);
  border: 1px solid #333f4b;
  border-radius: 12px;
  box-shadow: 0 6px 16px rgba(13, 18, 24, 0.22);
}
.login-title {
  margin-top: 18px;
  color: #10151b;
  font-size: 23px;
  font-weight: 700;
  letter-spacing: -0.01em;
  line-height: 1.25;
}
.login-sub {
  margin-top: 8px;
  color: #5d6772;
  font-size: 14px;
  line-height: 1.55;
}

/* --- form --- */
.login-form {
  display: grid;
  gap: 16px;
  margin-top: 24px;
  text-align: left;
}
.login-field {
  display: grid;
  gap: 7px;
  min-width: 0;
}
.login-field label {
  color: #454f5b;
  font-size: 12.5px;
  font-weight: 700;
}
.login-field input {
  width: 100%;
  height: 46px;
  padding: 0 13px;
  font: inherit;
  font-size: 14.5px;
  color: #10151b;
  background: #fff;
  border: 1px solid #c6ccd4;
  border-radius: 10px;
  transition: border-color 0.16s, box-shadow 0.16s;
}
.login-field input::placeholder {
  color: #9aa3ad;
}
.login-field input:hover {
  border-color: #a7aeb8;
}
.login-field input:focus {
  outline: none;
  border-color: #10151b;
  box-shadow: 0 0 0 4px rgba(16, 21, 27, 0.09);
}
.login-field input:-webkit-autofill {
  -webkit-box-shadow: 0 0 0 40px #fff inset;
  -webkit-text-fill-color: #10151b;
}
.login-password {
  position: relative;
}
.login-password input {
  padding-right: 46px;
}
.login-eye {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  color: #97a0ab;
  background: none;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  transition: color 0.15s, background 0.15s;
}
.login-eye:hover {
  color: #2b333d;
  background: #f2f4f6;
}

.login-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  color: #a14444;
  background: #fdf2f2;
  border: 1px solid #f2d6d6;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 600;
  animation: login-shake 0.34s cubic-bezier(0.36, 0.07, 0.19, 0.97);
}

.login-submit {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  height: 46px;
  margin-top: 2px;
  color: #fff;
  background: #0f141a;
  border: 0;
  border-radius: 999px;
  font-size: 14.5px;
  font-weight: 700;
  letter-spacing: 0.01em;
  cursor: pointer;
  transition: background 0.18s, box-shadow 0.18s, transform 0.08s;
}
.login-submit:hover:not(:disabled) {
  background: #262d36;
  box-shadow: 0 8px 22px -8px rgba(10, 15, 22, 0.55);
}
.login-submit:active:not(:disabled) {
  transform: scale(0.985);
}
.login-submit:disabled {
  cursor: wait;
  opacity: 0.82;
}
.login-spin {
  animation: login-rotate 0.9s linear infinite;
}
.login-help {
  color: #8b939d;
  font-size: 12.5px;
  text-align: center;
}

/* --- footer --- */
.login-foot {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 22px;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: rgba(233, 240, 248, 0.62);
  font-size: 12px;
  letter-spacing: 0.04em;
}
.login-foot-dot {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: rgba(233, 240, 248, 0.45);
}

/* --- motion --- */
.rise {
  opacity: 0;
  animation: login-rise 0.6s cubic-bezier(0.22, 1, 0.36, 1) forwards;
}
.d1 { animation-delay: 0.05s; }
.d2 { animation-delay: 0.13s; }
.d3 { animation-delay: 0.21s; }
.d4 { animation-delay: 0.29s; }
.d5 { animation-delay: 0.37s; }
.d6 { animation-delay: 0.5s; }

@keyframes login-rise {
  from {
    opacity: 0;
    transform: translateY(14px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
@keyframes login-shake {
  10%, 90% { transform: translateX(-1px); }
  20%, 80% { transform: translateX(2px); }
  30%, 50%, 70% { transform: translateX(-4px); }
  40%, 60% { transform: translateX(4px); }
}
@keyframes login-rotate {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .rise,
  .login-error,
  .login-spin {
    animation: none;
    opacity: 1;
  }
  .login-bg {
    transition: opacity 0.3s ease;
    transform: none;
  }
  .login-bg-wrap {
    transition: none;
    transform: none;
  }
  .login-submit {
    transition: none;
  }
}

@media (max-width: 480px) {
  .login-shell {
    padding: 24px 16px 64px;
  }
  .login-card {
    padding: 28px 22px 22px;
    border-radius: 18px;
  }
  .login-title {
    font-size: 21px;
  }
  .login-sub {
    font-size: 13.5px;
  }
  .login-foot {
    bottom: 16px;
  }
}
</style>
