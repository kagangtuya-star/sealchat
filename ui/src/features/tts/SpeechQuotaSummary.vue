<script setup lang="ts">
import { computed } from 'vue'
import type { SpeechQuota } from './types'
import { speechAvailableAmount } from './runtime'

const props = defineProps<{ quota: SpeechQuota }>()
const rows = computed(() => [
  { label: '日', limit: props.quota.policy.dailyLimit, used: props.quota.usage.DailySettled },
  { label: '月', limit: props.quota.policy.monthlyLimit, used: props.quota.usage.MonthlySettled },
  { label: '累计', limit: props.quota.policy.lifetimeLimit, used: props.quota.usage.LifetimeSettled },
].map(row => ({ ...row, available: speechAvailableAmount(row.limit, row.used, props.quota.usage.ActiveReserved) })))
function amount(value: number | null) {
  return value === null ? '未设上限' : value.toLocaleString('zh-CN', { maximumFractionDigits: 6 })
}
</script>

<template>
  <div class="speech-quota-summary">
    <div v-for="row in rows" :key="row.label">
      {{ row.label }}语音金额：已用 {{ amount(row.used) }} / 上限 {{ amount(row.limit) }} · 可用 {{ amount(row.available) }}
    </div>
    <div>有效预留 {{ amount(quota.usage.ActiveReserved) }}（已从各周期可用金额中扣除）</div>
    <div>已保存音色 {{ quota.saved }} / {{ quota.slots }} 槽位</div>
  </div>
</template>
