import { computed, ref, watch } from 'vue'
import type { MessageFavoriteItem } from './api'

// 收藏窗口的批量选择状态，只属于当前窗口实例。
// 选择始终是当前已加载列表的子集：列表刷新、筛选结果替换或项目被移除后，
// 不再可见的 ID 会被立即剔除，避免对看不到的收藏执行批量操作。

export const useFavoriteSelection = (items: () => MessageFavoriteItem[]) => {
  const active = ref(false)
  const selected = ref(new Set<string>())

  const replace = (ids: Iterable<string>) => {
    selected.value = new Set(ids)
  }

  const clear = () => {
    if (selected.value.size) replace([])
  }

  const enter = () => {
    clear()
    active.value = true
  }

  const exit = () => {
    clear()
    active.value = false
  }

  const isSelected = (id: string) => selected.value.has(id)

  const toggle = (id: string) => {
    if (!active.value) return
    const next = new Set(selected.value)
    if (next.has(id)) next.delete(id)
    else if (items().some((item) => item.id === id)) next.add(id)
    else return
    replace(next)
  }

  const selectAll = () => {
    if (active.value) replace(items().map((item) => item.id))
  }

  // 一键选中当前已加载的不可查看收藏（原消息已删除或已无权查看），替换现有选择。
  const selectUnavailable = () => {
    if (active.value) replace(items().filter((item) => item.status !== 'available').map((item) => item.id))
  }

  const deselect = (ids: Iterable<string>) => {
    const next = new Set(selected.value)
    let changed = false
    for (const id of ids) changed = next.delete(id) || changed
    if (changed) replace(next)
  }

  const prune = () => {
    if (!selected.value.size) return
    const visible = new Set(items().map((item) => item.id))
    const kept = [...selected.value].filter((id) => visible.has(id))
    if (kept.length !== selected.value.size) replace(kept)
  }

  watch(items, prune, { flush: 'sync' })

  // 按列表顺序给出选中的 ID。
  const selectedIds = computed(() => items().filter((item) => selected.value.has(item.id)).map((item) => item.id))
  const count = computed(() => selectedIds.value.length)
  const unavailableCount = computed(() => items().filter((item) => item.status !== 'available').length)
  const allSelected = computed(() => items().length > 0 && count.value === items().length)

  return {
    active,
    selectedIds,
    count,
    unavailableCount,
    allSelected,
    enter,
    exit,
    clear,
    isSelected,
    toggle,
    selectAll,
    selectUnavailable,
    deselect,
    prune,
  }
}
