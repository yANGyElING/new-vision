import { computed, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

/**
 * 设备识别：单代码库适配 Web + 手机的核心。
 *
 * - matchMedia 响应式检测（640px = 手机/平板分界，900px = 平板/桌面分界）
 * - 支持 URL 参数 `?mode=forceMobile` / `?mode=forceDesktop` 显式覆盖（外部 H5 嵌入时使用）
 * - matchMedia 不可用时默认桌面（isMobile = false）
 */
export function useDevice() {
  const route = useRoute()

  const isMobile = ref(false)
  const _isTablet = ref(false)
  const forceMode = ref<'forceMobile' | 'forceDesktop' | null>(null)

  const mqMobile = typeof window !== 'undefined' ? window.matchMedia('(max-width: 640px)') : null
  const mqTablet =
    typeof window !== 'undefined' ? window.matchMedia('(max-width: 900px) and (min-width: 641px)') : null

  const sync = () => {
    isMobile.value = mqMobile?.matches ?? false
    _isTablet.value = mqTablet?.matches ?? false
  }

  // 响应视口变化（含横竖屏切换）
  mqMobile?.addEventListener('change', sync)
  mqTablet?.addEventListener('change', sync)
  sync()

  // 组件卸载时清理监听器
  onUnmounted(() => {
    mqMobile?.removeEventListener('change', sync)
    mqTablet?.removeEventListener('change', sync)
  })

  // URL 参数显式覆盖：?mode=forceMobile / ?mode=forceDesktop
  watch(
    () => route.query.mode,
    (mode) => {
      forceMode.value =
        mode === 'forceMobile' ? 'forceMobile' : mode === 'forceDesktop' ? 'forceDesktop' : null
    },
    { immediate: true },
  )

  const isMobileEffective = computed(() => forceMode.value === 'forceMobile' || (forceMode.value === null && isMobile.value))
  const isDesktopEffective = computed(() => forceMode.value === 'forceDesktop' || (forceMode.value === null && !isMobile.value && !_isTablet.value))

  return {
    isMobile: isMobileEffective,
    isTablet: computed(() => forceMode.value === null && _isTablet.value),
    isDesktop: isDesktopEffective,
    forceMode,
  }
}
