<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import { NSelect as NSelectBase } from 'naive-ui'
import { matchText, usePinyinReadyVersion } from '@/utils/pinyinMatch'

defineOptions({ inheritAttrs: false })

type SelectOptionLike = {
  label?: string
  value?: string | number
  [key: string]: unknown
}

const attrs = useAttrs()
const pinyinReadyVersion = usePinyinReadyVersion()

const resolveLabelField = () => {
  const labelField = (attrs as Record<string, unknown>)['label-field']
  return (labelField as string) || (attrs.labelField as string) || 'label'
}

const resolveOptionText = (option: SelectOptionLike, labelField: string) => {
  if (!option) return ''
  const label = option[labelField] ?? option.label
  const value = option.value
  const pieces = [label, value].filter((item) => item !== undefined && item !== null && String(item).length > 0)
  return pieces.map((item) => String(item)).join(' ')
}

const resolvedAttrs = computed(() => {
  void pinyinReadyVersion.value
  const providedFilter = (attrs as { filter?: (pattern: string, option: SelectOptionLike) => boolean }).filter
  const labelField = resolveLabelField()
  const filter = typeof providedFilter === 'function'
    ? providedFilter
    : (pattern: string, option: SelectOptionLike) => {
        const keyword = (pattern ?? '').trim()
        if (!keyword) return true
        const text = resolveOptionText(option, labelField)
        return matchText(keyword, text)
      }
  return {
    ...attrs,
    filter,
  }
})
</script>

<template>
  <NSelectBase v-bind="resolvedAttrs">
    <template v-for="(_, name) in $slots" v-slot:[name]="slotProps">
      <slot :name="name" v-bind="slotProps" />
    </template>
  </NSelectBase>
</template>
