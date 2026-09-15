<script setup lang="ts">
/**
 * 圆环进度（D51）：12 点钟起弧、圆头线帽、线宽 2.5；
 * 进行中蓝色；完成时弧收拢为满环变绿 + ✓ 回弹浮现（450ms）；失败停在最后位置变红。
 * 无机制文案、无长条进度。prefers-reduced-motion 时动画退化为透明度。
 */
import { computed } from 'vue'
import { Check } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    /** 直径（px）：管理页 16、用户页横幅 24 */
    size?: number
    /** 进度 0..1；负值 = 未知（同步中但暂无数字） */
    progress?: number
    /** ing 进行中 / done 完成 / fail 失败停位 */
    state?: 'ing' | 'done' | 'fail'
    /** 圆环中心显示的百分比数字（仅大尺寸） */
    showPct?: boolean
  }>(),
  { size: 16, progress: -1, state: 'ing', showPct: false },
)

const radius = computed(() => props.size / 2 - 2)
const circumference = computed(() => 2 * Math.PI * radius.value)
const offset = computed(() => {
  if (props.progress < 0) return circumference.value * 0.75 // 未知进度：固定短弧
  const p = Math.min(Math.max(props.progress, 0), 1)
  return circumference.value * (1 - p)
})
const pctText = computed(() => (props.progress < 0 ? '' : `${Math.round(props.progress * 100)}`))
const iconSize = computed(() => Math.max(8, Math.round(props.size * 0.55)))
const pctFontSize = computed(() => `${Math.max(8, Math.round(props.size * 0.46))}px`)
</script>

<template>
  <span class="ring" :class="state" :style="{ width: `${size}px`, height: `${size}px` }" aria-hidden="true">
    <svg :width="size" :height="size">
      <circle class="track" :cx="size / 2" :cy="size / 2" :r="radius" fill="none" stroke-width="2.5" />
      <circle
        class="arc"
        :cx="size / 2"
        :cy="size / 2"
        :r="radius"
        fill="none"
        stroke-width="2.5"
        :stroke-dasharray="circumference"
        :stroke-dashoffset="state === 'done' ? 0 : offset"
      />
    </svg>
    <span v-if="showPct && pctText" class="pct" :style="{ fontSize: pctFontSize }">{{ pctText }}</span>
    <span class="tick"><Check :size="iconSize" :stroke-width="3" /></span>
  </span>
</template>

<style scoped>
.ring { position: relative; display: inline-block; vertical-align: -3px; flex-shrink: 0; }
.ring svg { display: block; transform: rotate(-90deg); }
.ring .track { stroke: var(--bg-subtle); }
.ring .arc { stroke: var(--accent-blue); stroke-linecap: round; transition: stroke-dashoffset 0.35s cubic-bezier(0.32, 0.72, 0, 1); }
.ring.done .arc { stroke: var(--accent-green); stroke-dashoffset: 0; }
.ring.fail .arc { stroke: var(--accent-red); }
.ring .tick { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; font-size: 9px; font-weight: 600; color: var(--accent-green); opacity: 0; transform: scale(0.4); transition: opacity 0.3s ease 0.12s, transform 0.45s cubic-bezier(0.34, 1.2, 0.28, 1) 0.12s; }
.ring.done .tick { opacity: 1; transform: scale(1); }
.ring .pct { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; font-weight: 600; color: var(--text-tertiary); font-variant-numeric: tabular-nums; }
@media (prefers-reduced-motion: reduce) {
  .ring .arc { transition: stroke-dashoffset 0.2s linear; }
  .ring .tick { transition: opacity 0.15s ease; transform: none; }
}
</style>
