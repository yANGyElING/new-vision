<template>
  <main class="shell centered-page">
    <section class="empty-state" aria-labelledby="login-title">
      <div class="empty-icon"><LogIn :size="24" /></div>
      <div class="eyebrow">身份认证</div>
      <h1 id="login-title">认证尚未启用</h1>
      <p>当前工程骨架不提供登录、会话或授权能力。此入口仅用于保留后续集成位置。</p>
      <RouterLink class="primary-link" to="/"><ArrowLeft :size="16" />返回节点状态</RouterLink>
    </section>
  </main>
</template>

<script setup lang="ts">
import { ArrowLeft, LogIn } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
</script>
