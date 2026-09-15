<script setup lang="ts">
/**
 * 组织树节点（D51）：嵌套容器 + 左侧 1px 导轨线（VSCode/Notion 式）；
 * 父节点行首 ▸ 折叠箭头（可收起），叶子节点小圆点；不用肘部连接线。
 * 通过文件名递归引用自身。
 */
import type { OrgUnit } from '@/api/identity'

const props = defineProps<{
  nodes: OrgUnit[]
  /** 每个节点的设备计数 */
  countOf: (orgID: string) => number
  /** 选中节点（null = 全部） */
  active: string | null
  /** 折叠集合（v-model 语义：组件只发事件） */
  collapsed: Set<string>
}>()

const emit = defineEmits<{
  select: [orgID: string]
  toggle: [orgID: string]
}>()
</script>

<template>
  <div v-for="node in nodes" :key="node.id" class="org-subtree">
    <button
      class="org-row"
      type="button"
      :class="{ active: active === node.id }"
      :title="node.name"
      @click="emit('select', node.id)"
    >
      <span
        v-if="node.children?.length"
        class="org-chev"
        role="button"
        :tabindex="-1"
        :aria-label="collapsed.has(node.id) ? '展开' : '收起'"
        :aria-expanded="!collapsed.has(node.id)"
        @click.stop="emit('toggle', node.id)"
      />
      <span v-else class="org-leaf-dot" aria-hidden="true" />
      <span class="org-name">{{ node.name }}</span>
      <span class="org-count mono">{{ countOf(node.id) }}</span>
    </button>
    <div v-if="node.children?.length && !collapsed.has(node.id)" class="org-branch">
      <OrgTreeNode
        :nodes="node.children"
        :count-of="countOf"
        :active="active"
        :collapsed="collapsed"
        @select="(id: string) => emit('select', id)"
        @toggle="(id: string) => emit('toggle', id)"
      />
    </div>
  </div>
</template>

<style scoped>
.org-subtree { display: block; }
.org-row { display: flex; align-items: center; gap: 7px; width: 100%; padding: 7px 10px; color: var(--text-secondary); background: transparent; border: 0; border-radius: var(--r-sm); font: inherit; font-size: 13.5px; text-align: left; cursor: pointer; box-sizing: border-box; transition: background 0.15s, color 0.15s; }
.org-row:hover { background: var(--bg-hover); }
.org-row.active { background: var(--bg-subtle); color: var(--text-primary); font-weight: 600; }
.org-row:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
.org-chev { width: 13px; height: 13px; flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center; }
.org-chev::before { content: ''; border: solid transparent; border-width: 3.5px 0 3.5px 4.5px; border-left-color: var(--text-tertiary); transition: transform 0.18s cubic-bezier(0.32, 0.72, 0, 1); }
.org-leaf-dot { width: 4px; height: 4px; margin: 0 4px 0 5px; border-radius: 50%; background: color-mix(in srgb, var(--text-tertiary) 45%, transparent); flex-shrink: 0; }
.org-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.org-count { color: var(--text-tertiary); font-size: 11px; font-variant-numeric: tabular-nums; }
.org-row.active .org-count { color: var(--text-secondary); }
/* 嵌套容器：左侧 1px 导轨线 */
.org-branch { margin-left: 6px; padding-left: 11px; border-left: 1px solid color-mix(in srgb, var(--text-tertiary) 25%, transparent); display: block; }
@media (prefers-reduced-motion: reduce) { .org-chev::before { transition: none; } }
</style>
