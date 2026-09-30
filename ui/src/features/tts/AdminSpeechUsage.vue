<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { NAlert, NButton, NFormItem, NInput, NInputNumber, NSpace, NSwitch } from 'naive-ui'
import { speechAPI, speechError } from './api'
import type { SpeechPolicy, SpeechQuota, UnknownSpeechUsage } from './types'
import { SpeechEpoch } from './runtime'
import SpeechQuotaSummary from './SpeechQuotaSummary.vue'
const props = defineProps<{ userId?: string; policyOnly?: boolean }>()
const userId = ref('')
const policy = ref<SpeechPolicy | null>(null)
const quota = ref<SpeechQuota | null>(null)
const unknown = ref<UnknownSpeechUsage[]>([])
const note = ref('')
const units = ref<number | null>(null)
const error = ref('')
const notice = ref('')
const policyNotice = ref('')
const busy = ref(false)
let loadedUser = ''
const context = new SpeechEpoch()
async function run(fn: () => Promise<void>) {
  if (busy.value) return
  const epoch = context.capture()
  busy.value = true; error.value = ''; notice.value = ''; policyNotice.value = ''
  try { await fn() } catch (e) {
    if (context.current(epoch)) error.value = speechError(e)
  } finally {
    if (context.current(epoch)) busy.value = false
  }
}
async function loadUser() {
  const id = userId.value.trim()
  if (!id) return
  const epoch = context.capture()
  const value = await speechAPI.policy(id)
  if (!context.current(epoch) || id !== userId.value.trim()) return
  loadedUser = id; policy.value = value.policy; quota.value = value.quota
  policyNotice.value = '已加载用户语音策略。'
}
async function saveUser() {
  const id = loadedUser
  const epoch = context.capture()
  if (!policy.value || !id || id !== userId.value.trim()) throw new Error('请先查询当前用户')
  await speechAPI.savePolicy(id, { ...policy.value })
  if (context.current(epoch)) await loadUser()
}
watch(userId, () => {
  context.invalidate()
  loadedUser = ''; policy.value = null; quota.value = null; error.value = ''; policyNotice.value = ''; busy.value = false
}, { flush: 'sync' })
watch(() => props.userId, (id) => {
  if (id === undefined) return
  userId.value = id
  void run(loadUser)
}, { immediate: true })
onBeforeUnmount(() => context.invalidate())
async function loadUnknown() {
  const items = await speechAPI.unknown()
  unknown.value = items
  notice.value = items.length > 0
    ? `已查询到 ${items.length} 个待核对任务。`
    : '当前没有待核对的供应商用量。'
}
async function resolve(id: string, action: 'settle' | 'release') {
  if (!note.value.trim() || (action === 'settle' && units.value == null)) throw new Error('必须填写供应商核对依据；结算还需要确认实际用量。')
  await speechAPI.resolveUnknown(id, action, note.value, units.value ?? 0)
  unknown.value = await speechAPI.unknown()
}
</script>
<template>
  <NSpace vertical>
    <h4>用户语音用量与槽位</h4>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NInput v-if="props.userId === undefined" v-model:value="userId" placeholder="用户 ID" />
    <NButton :loading="busy" :disabled="!userId.trim()" @click="run(loadUser)">查询用户语音策略</NButton>
    <NAlert v-if="policyNotice" type="info">{{ policyNotice }}</NAlert>
    <template v-if="policy && quota">
      <SpeechQuotaSummary :quota="quota" />
      <NFormItem label="覆盖平台语音用量限制"><NSwitch v-model:value="policy.overrideEnabled" /></NFormItem>
      <NFormItem label="日限制"><NInputNumber v-model:value="policy.dailyLimit" :min="0" /></NFormItem>
      <NFormItem label="月限制"><NInputNumber v-model:value="policy.monthlyLimit" :min="0" /></NFormItem>
      <NFormItem label="累计限制"><NInputNumber v-model:value="policy.lifetimeLimit" :min="0" /></NFormItem>
      <NFormItem label="槽位覆盖（空继承，0 禁止保存）"><NInputNumber v-model:value="policy.slots" :min="0" /></NFormItem>
      <NAlert type="info">语音与文本数据相互独立；公开和私有的已保存个人音色都占槽位。降低槽位不会删除已有音色。</NAlert>
      <NButton :loading="busy" @click="run(saveUser)">保存语音策略</NButton>
    </template>
    <template v-if="!props.policyOnly">
    <h4>待核对供应商用量</h4>
    <NAlert type="warning">仅在供应商确认后处置。预留不会因普通日志清理或过期自动释放，也不会自动重新提交请求。</NAlert>
    <NButton :loading="busy" @click="run(loadUnknown)">查询待核对任务</NButton>
    <NAlert v-if="notice" type="info">{{ notice }}</NAlert>
    <NInput v-model:value="note" placeholder="核对依据和处置说明（必填，留作审计）" />
    <NInputNumber v-model:value="units" :min="0" placeholder="供应商确认的实际字符数或创建次数" />
    <NSpace v-for="item in unknown" :key="item.id" vertical>
      <span>{{ item.operation }} · {{ item.payerUserId }} · Request ID {{ item.providerRequestId || '未返回' }} · {{ item.errorCode }}</span>
      <NSpace>
        <NButton :loading="busy" @click="run(() => resolve(item.id, 'settle'))">确认供应商用量并结算</NButton>
        <NButton :loading="busy" @click="run(() => resolve(item.id, 'release'))">确认无用量并释放</NButton>
      </NSpace>
    </NSpace>
    </template>
  </NSpace>
</template>
