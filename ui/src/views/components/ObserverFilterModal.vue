<script setup lang="ts">
import { computed, ref, watch } from 'vue'

interface FilterState {
  icFilter: 'all' | 'ic' | 'ooc'
  showArchived: boolean
  roleIds: string[]
  whisperOnly: boolean
  fromTime: number | null
  toTime: number | null
}

interface RoleOption {
  label: string
  value: string
}

const props = defineProps<{
  show: boolean
  filters: FilterState
  roles: RoleOption[]
}>()

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
  (event: 'apply', value: FilterState): void
}>()

const createDraft = (): FilterState => ({
  icFilter: props.filters.icFilter,
  showArchived: props.filters.showArchived,
  roleIds: [...props.filters.roleIds],
  whisperOnly: props.filters.whisperOnly,
  fromTime: props.filters.fromTime,
  toTime: props.filters.toTime,
})

const draft = ref<FilterState>(createDraft())

const timeRange = computed<[number, number] | null>({
  get() {
    if (draft.value.fromTime === null || draft.value.toTime === null) {
      return null
    }
    return [draft.value.fromTime, draft.value.toTime]
  },
  set(value) {
    draft.value.fromTime = Array.isArray(value) && value.length > 0 ? Number(value[0]) : null
    draft.value.toTime = Array.isArray(value) && value.length > 1 ? Number(value[1]) : null
  },
})

const activeFiltersCount = computed(() => {
  let count = 0
  if (draft.value.whisperOnly) count += 1
  if (draft.value.showArchived) count += 1
  if (draft.value.icFilter !== 'all') count += 1
  if (draft.value.roleIds.length > 0) count += 1
  if (draft.value.fromTime !== null || draft.value.toTime !== null) count += 1
  return count
})

const resetDraft = () => {
  draft.value = {
    icFilter: 'all',
    showArchived: false,
    roleIds: [],
    whisperOnly: false,
    fromTime: null,
    toTime: null,
  }
}

const close = () => emit('update:show', false)

const apply = () => {
  emit('apply', {
    ...draft.value,
    roleIds: [...draft.value.roleIds],
  })
  close()
}

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      draft.value = createDraft()
    }
  },
)
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="更多筛选"
    class="observer-filter-modal"
    :mask-closable="false"
    style="width: min(30rem, calc(100vw - 1.5rem));"
    @update:show="emit('update:show', $event)"
  >
    <n-form label-placement="left" label-width="7.5rem" require-mark-placement="right-hanging">
      <n-form-item label="仅显示悄悄话">
        <n-switch v-model:value="draft.whisperOnly" />
      </n-form-item>

      <n-form-item label="隐藏归档">
        <n-switch :value="!draft.showArchived" @update:value="draft.showArchived = !$event" />
      </n-form-item>

      <n-form-item label="频道角色">
        <n-select
          v-model:value="draft.roleIds"
          :options="roles"
          multiple
          clearable
          filterable
          max-tag-count="responsive"
          placeholder="全部角色"
        />
      </n-form-item>

      <n-form-item label="消息范围">
        <n-radio-group v-model:value="draft.icFilter">
          <n-space>
            <n-radio value="all">全部</n-radio>
            <n-radio value="ic">仅场内</n-radio>
            <n-radio value="ooc">仅场外</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>

      <n-form-item label="时间范围">
        <n-date-picker
          v-model:value="timeRange"
          type="datetimerange"
          clearable
          format="yyyy-MM-dd HH:mm"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
        />
      </n-form-item>
    </n-form>

    <template #footer>
      <div class="observer-filter-modal__footer">
        <n-button quaternary @click="resetDraft">重置</n-button>
        <span class="observer-filter-modal__count">已选 {{ activeFiltersCount }} 项</span>
        <div class="observer-filter-modal__actions">
          <n-button @click="close">取消</n-button>
          <n-button type="primary" @click="apply">应用</n-button>
        </div>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.observer-filter-modal__footer {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.observer-filter-modal__count {
  flex: 1;
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.observer-filter-modal__actions {
  display: inline-flex;
  gap: 0.5rem;
}
</style>
