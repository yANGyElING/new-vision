<template>
  <main class="shell centered-page">
    <section class="empty-state" aria-labelledby="not-found-title">
      <div class="empty-code">404</div>
      <div class="eyebrow">页面不存在</div>
      <h1 id="not-found-title">找不到这个页面</h1>
      <p>当前地址没有对应的节点管理页面。</p>
      <RouterLink class="primary-link" to="/"><ArrowLeft :size="16" />返回节点状态</RouterLink>
    </section>
  </main>
</template>

<script setup lang="ts">
import { ArrowLeft } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
</script>
