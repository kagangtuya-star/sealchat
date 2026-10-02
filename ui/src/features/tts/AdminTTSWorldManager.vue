<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { NAlert, NButton, NInput, NInputNumber, NPagination, NSwitch } from 'naive-ui'
import type { AdminAIUsageLogListResult } from '@/types'
import { speechAPI, speechError } from './api'
import type { AdminSpeechWorld, AdminSpeechWorldList, SpeechWorldPatch } from './types'
import TTSWorldOverlay from './TTSWorldOverlay.vue'
const emit = defineEmits<{ close: [] }>()
const search = ref('')
const page = ref(1)
const pageSize = 12
const list = ref<AdminSpeechWorldList>({ items: [], total: 0, page: 1, pageSize })
const selected = ref<AdminSpeechWorld | null>(null)
const draft = ref<SpeechWorldPatch | null>(null)
const loading = ref(false)
const saving = ref(false)
const detailLoading = ref(false)
const error = ref('')
const notice = ref('')
const provider = ref('')
const model = ref('')
const feature = ref('')
const logPage = ref(1)
const logs = ref<AdminAIUsageLogListResult | null>(null)
const logsLoading = ref(false)
let alive = true, listEpoch = 0, detailEpoch = 0, logEpoch = 0
onBeforeUnmount(() => { alive = false; listEpoch++; detailEpoch++; logEpoch++ })
const money = (value: number) => value.toLocaleString('zh-CN', { maximumFractionDigits: 6 })
const date = (value: string | null) => value ? new Date(value).toLocaleString('zh-CN') : '尚未使用'
function quotaStatus(world: AdminSpeechWorld) {
  const p = world.policy, u = world.usage
  if (!p.quotaOverrideEnabled) return '无额外世界限额'
  const limits = [[u.DailySettled, p.dailyLimit], [u.MonthlySettled, p.monthlyLimit], [u.LifetimeSettled, p.lifetimeLimit]]
  return limits.some(([used, limit]) => limit != null && used != null && used + u.ActiveReserved >= limit) ? '世界额度已用尽' : '独立世界额度可用'
}
async function loadList() {
  const epoch = ++listEpoch
  loading.value = true; error.value = ''
  try {
    const value = await speechAPI.worlds({ page: page.value, pageSize, search: search.value.trim() })
    if (alive && epoch === listEpoch) list.value = value
  } catch (e) { if (alive && epoch === listEpoch) error.value = speechError(e) }
  finally { if (alive && epoch === listEpoch) loading.value = false }
}
function find() { page.value = 1; void loadList() }
function changePage(value: number) { page.value = value; void loadList() }
async function select(world: AdminSpeechWorld) {
  const epoch = ++detailEpoch
  logEpoch++; selected.value = world; draft.value = null; logs.value = null
  detailLoading.value = true; error.value = ''; notice.value = ''
  provider.value = model.value = feature.value = ''; logPage.value = 1
  try {
    const value = await speechAPI.world(world.worldId)
    if (!alive || epoch !== detailEpoch) return
    selected.value = value
    draft.value = { allowlisted: value.policy.allowlisted, quotaOverrideEnabled: value.policy.quotaOverrideEnabled, dailyLimit: value.policy.dailyLimit, monthlyLimit: value.policy.monthlyLimit, lifetimeLimit: value.policy.lifetimeLimit }
    void loadLogs()
  } catch (e) { if (alive && epoch === detailEpoch) error.value = speechError(e) }
  finally { if (alive && epoch === detailEpoch) detailLoading.value = false }
}
async function save() {
  if (!draft.value || !selected.value || saving.value) return
  const id = selected.value.worldId, epoch = detailEpoch
  saving.value = true; error.value = ''; notice.value = ''
  try {
    const value = await speechAPI.saveWorld(id, { ...draft.value })
    if (!alive || epoch !== detailEpoch) return
    selected.value = value
    const index = list.value.items.findIndex(world => world.worldId === id)
    if (index >= 0) list.value.items[index] = value
    notice.value = '世界白名单与额度已保存。'
  } catch (e) { if (alive && epoch === detailEpoch) error.value = speechError(e) }
  finally { if (alive) saving.value = false }
}
async function loadLogs() {
  if (!selected.value) return
  const epoch = ++logEpoch, worldId = selected.value.worldId
  logsLoading.value = true
  try {
    const result = await speechAPI.logs({ worldId, page: logPage.value, pageSize: 10, providerId: provider.value.trim() || undefined, model: model.value.trim() || undefined, featureKey: feature.value.trim() || undefined })
    if (alive && epoch === logEpoch && selected.value?.worldId === worldId) logs.value = result.data
  } catch (e) { if (alive && epoch === logEpoch) error.value = speechError(e) }
  finally { if (alive && epoch === logEpoch) logsLoading.value = false }
}
function filterLogs() { logPage.value = 1; void loadLogs() }
function changeLogPage(value: number) { logPage.value = value; void loadLogs() }
onMounted(() => void loadList())
</script>

<template>
  <TTSWorldOverlay title="世界白名单与额度" wide @close="emit('close')">
    <NAlert v-if="error" type="error" class="wm-alert">{{ error }}</NAlert>
    <div class="wm-layout" :class="{ 'wm-layout--detail': selected }">
      <section class="wm-list" aria-label="世界列表">
        <form class="wm-search" @submit.prevent="find"><NInput v-model:value="search" clearable placeholder="搜索世界名或 ID" /><NButton attr-type="submit" :loading="loading">搜索</NButton></form>
        <div class="wm-worlds" :aria-busy="loading">
          <p v-if="!loading && !list.items.length">没有匹配的世界。</p>
          <button v-for="world in list.items" :key="world.worldId" type="button" class="wm-world" :class="{ 'wm-world--selected': selected?.worldId === world.worldId }" :disabled="saving" @click="select(world)">
            <div class="wm-world-title"><strong>{{ world.name }}</strong><span :class="{ 'wm-allowed': world.policy.allowlisted }">{{ world.policy.allowlisted ? '白名单' : '未加入' }}</span></div>
            <small>{{ world.worldId }}</small>
            <dl class="wm-usage"><div><dt>今日</dt><dd>{{ money(world.usage.DailySettled) }}</dd></div><div><dt>本月</dt><dd>{{ money(world.usage.MonthlySettled) }}</dd></div><div><dt>累计</dt><dd>{{ money(world.usage.LifetimeSettled) }}</dd></div><div><dt>预留</dt><dd>{{ money(world.usage.ActiveReserved) }}</dd></div></dl>
            <p>{{ quotaStatus(world) }}<template v-if="world.policy.quotaOverrideEnabled"> · 日 {{ world.policy.dailyLimit ?? '无限' }} / 月 {{ world.policy.monthlyLimit ?? '无限' }} / 累计 {{ world.policy.lifetimeLimit ?? '无限' }}</template></p>
            <small>最近使用：{{ date(world.lastUsedAt) }}</small>
          </button>
        </div>
        <NPagination :page="page" :page-size="pageSize" :item-count="list.total" simple @update:page="changePage" />
      </section>
      <section class="wm-detail" :aria-busy="detailLoading" aria-label="世界详情">
        <NButton class="wm-back" text @click="selected = null; draft = null; detailEpoch++; logEpoch++">← 返回世界列表</NButton>
        <template v-if="selected">
          <h3>{{ selected.name }}</h3><p class="wm-meta">{{ selected.worldId }} · 世界主：{{ selected.ownerNickname || selected.ownerUsername || selected.ownerId }}</p>
          <dl class="wm-usage wm-usage--detail"><div><dt>今日使用</dt><dd>{{ money(selected.usage.DailySettled) }}</dd></div><div><dt>月度使用</dt><dd>{{ money(selected.usage.MonthlySettled) }}</dd></div><div><dt>累计使用</dt><dd>{{ money(selected.usage.LifetimeSettled) }}</dd></div><div><dt>Active reserved</dt><dd>{{ money(selected.usage.ActiveReserved) }}</dd></div></dl>
          <form v-if="draft" class="wm-policy" @submit.prevent="save">
            <label class="wm-toggle">加入世界白名单<NSwitch v-model:value="draft.allowlisted" :disabled="saving" /></label>
            <label class="wm-toggle">启用独立世界额度<NSwitch v-model:value="draft.quotaOverrideEnabled" :disabled="saving" /></label>
            <p class="wm-meta">世界额度与用户额度同时生效；金额沿用语音消费口径，留空表示不限。</p>
            <div class="wm-limits"><label>日限额<NInputNumber v-model:value="draft.dailyLimit" :min="0" :disabled="!draft.quotaOverrideEnabled || saving" placeholder="不限" /></label><label>月限额<NInputNumber v-model:value="draft.monthlyLimit" :min="0" :disabled="!draft.quotaOverrideEnabled || saving" placeholder="不限" /></label><label>累计限额<NInputNumber v-model:value="draft.lifetimeLimit" :min="0" :disabled="!draft.quotaOverrideEnabled || saving" placeholder="不限" /></label></div>
            <NAlert v-if="notice" type="success">{{ notice }}</NAlert>
            <div><NButton attr-type="submit" type="primary" :loading="saving">保存世界策略</NButton></div>
          </form>
          <section class="wm-logs">
            <h4>消费明细</h4>
            <form class="wm-log-filters" @submit.prevent="filterLogs"><NInput v-model:value="provider" placeholder="Provider ID" /><NInput v-model:value="model" placeholder="Model" /><NInput v-model:value="feature" placeholder="Feature" /><NButton attr-type="submit" :loading="logsLoading">筛选</NButton></form>
            <div class="wm-log-table"><table><thead><tr><th>时间</th><th>用户</th><th>Provider / Model</th><th>Feature</th><th>金额</th></tr></thead><tbody><tr v-for="log in logs?.items || []" :key="log.id"><td>{{ date(log.finishedAt) }}</td><td>{{ log.nicknameSnapshot || log.usernameSnapshot || log.userId }}</td><td>{{ log.providerId }}<br />{{ log.model }}</td><td>{{ log.featureKey }}</td><td>{{ money(log.totalCost) }}</td></tr></tbody></table><p v-if="logs && !logs.items.length">暂无消费明细。</p></div>
            <NPagination :page="logPage" :page-size="10" :item-count="logs?.total || 0" simple @update:page="changeLogPage" />
          </section>
        </template>
        <p v-else class="wm-empty">选择世界，查看用量并编辑白名单与额度。</p>
      </section>
    </div>
  </TTSWorldOverlay>
</template>

<style scoped>
.wm-layout { display: grid; grid-template-columns: minmax(320px, .9fr) minmax(0, 1.3fr); gap: 28px; flex: 1; min-height: 0; }
.wm-list { display: flex; flex-direction: column; min-height: 0; gap: 16px; }
.wm-search { display: flex; gap: 10px; }
.wm-worlds { overflow: auto; flex: 1; min-height: 0; }
.wm-world { display: block; width: 100%; text-align: left; color: inherit; padding: 18px 14px; border: 0; border-bottom: 1px solid var(--tw-border); background: transparent; cursor: pointer; }
.wm-world--selected { background: color-mix(in srgb, var(--tw-accent) 10%, transparent); box-shadow: inset 3px 0 var(--tw-accent); }
.wm-world-title { display: flex; justify-content: space-between; gap: 12px; align-items: center; margin-bottom: 4px; }
.wm-world-title span { font-size: 12px; opacity: .65; white-space: nowrap; }
.wm-world-title .wm-allowed { color: var(--tw-accent); opacity: 1; }
.wm-world p { font-size: 12px; margin: 8px 0; }
small, .wm-meta { opacity: .65; overflow-wrap: anywhere; }
.wm-usage { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin: 14px 0; font-variant-numeric: tabular-nums; }
dt { font-size: 12px; opacity: .6; } dd { margin: 4px 0 0; overflow-wrap: anywhere; }
.wm-detail { overflow: auto; min-width: 0; border-left: 1px solid var(--tw-border); padding-left: 28px; }
h3 { margin: 0; font-size: 22px; } h4 { margin-top: 28px; }
.wm-usage--detail { padding: 16px 0; border-block: 1px solid var(--tw-border); }
.wm-policy { display: grid; gap: 16px; }
.wm-toggle { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.wm-limits { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.wm-limits label { display: grid; gap: 8px; }
.wm-log-filters { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)) auto; gap: 8px; }
.wm-log-table { overflow-x: auto; margin: 16px 0; }
table { width: 100%; text-align: left; border-collapse: collapse; font-size: 12px; }
th, td { padding: 10px 8px; border-bottom: 1px solid var(--tw-border); overflow-wrap: anywhere; }
.wm-alert { margin-bottom: 16px; } .wm-empty { padding-top: 80px; text-align: center; opacity: .6; }
.wm-back { display: none; }
@media (max-width: 900px) {
  .wm-layout { display: flex; flex-direction: column; }
  .wm-detail { display: none; padding-left: 0; border-left: 0; }
  .wm-layout--detail .wm-list { display: none; }
  .wm-layout--detail .wm-detail { display: block; }
  .wm-back { display: inline-flex; margin-bottom: 18px; }
  .wm-limits { grid-template-columns: 1fr; }
  .wm-log-filters { grid-template-columns: 1fr 1fr; }
}
</style>
