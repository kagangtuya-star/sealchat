import { nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { applyCalendarActiveDayHighlights, collectCalendarMonthKeys } from '@/utils/calendarActiveDayHighlight'
import { createCalendarActiveDayCache } from '@/utils/calendarActiveDayCache'
import '@/assets/calendarActiveDay.scss'

/** 调用方提供自己拥有的 panel/容器；只观察日期结构，不监听叠层自身的 attribute 变更。 */
export function useCalendarActiveDays(options: {
  root: Ref<HTMLElement | null>
  enabled: () => boolean
  context: () => string
  locale: () => string
  load: (month: string, context: string) => Promise<string[]>
  canLoad?: (month: string) => boolean
}) {
  const cache = createCalendarActiveDayCache(options.load)
  const failed = ref(false)
  let observer: MutationObserver | null = null
  let disposed = false
  let sequence = 0

  async function refresh() {
    const seq = ++sequence
    await nextTick()
    const root = options.root.value
    if (disposed || !root || !options.enabled() || seq !== sequence) return
    const context = options.context()
    const locale = options.locale()
    const apply = () => applyCalendarActiveDayHighlights({ panel: root, activeDaySetByMonth: cache.get(context), locale })
    apply()
    failed.value = false
    const months = collectCalendarMonthKeys(root, locale).filter(month => options.canLoad?.(month) ?? true)
    const results = await Promise.allSettled(months.map(month => cache.ensure(month, context)))
    if (disposed || seq !== sequence || root !== options.root.value || !options.enabled() || context !== options.context()) return
    failed.value = results.some(result => result.status === 'rejected')
    apply()
  }

  watch([options.root, options.enabled, options.context, options.locale], () => {
    observer?.disconnect()
    observer = null
    sequence++
    const root = options.root.value
    if (!root) return
    if (!options.enabled()) {
      applyCalendarActiveDayHighlights({ panel: root, activeDaySetByMonth: {} })
      return
    }
    observer = new MutationObserver(() => { void refresh() })
    observer.observe(root, { childList: true, characterData: true, subtree: true })
    void refresh()
  }, { flush: 'post', immediate: true })

  onBeforeUnmount(() => {
    disposed = true
    sequence++
    observer?.disconnect()
  })
  return { failed, refresh }
}
