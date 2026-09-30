<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NAlert, NButton, NInput, NModal, NPagination, NSelect, NSwitch, useThemeVars } from 'naive-ui'
import { speechAPI, speechError } from './api'
import { useSpeechStore } from './store'
import { speechPlayer } from './player'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import type { SpeechJob, SpeechVoice, VoiceDirectory } from './types'
import SpeechQuotaSummary from './SpeechQuotaSummary.vue'
import VoicePicker from './VoicePicker.vue'
import { requestVoiceFields, voiceKindLabel, type VoiceSelection } from './voice-catalog'

const speech = useSpeechStore()
const chat = useChatStore()
const user = useUserStore()
const theme = useThemeVars()
const currentChannel = computed(() => speech.scopeChannel || chat.curChannel?.id)
const queue = ref<Awaited<ReturnType<typeof speechAPI.queue>> | null>(null)
let queueRevision = 0
watch(currentChannel, () => { queueRevision++; queue.value = null }, { flush: 'sync' })
async function loadQueue() {
  const channelId = currentChannel.value
  const revision = ++queueRevision
  if (!channelId) return
  const value = await speechAPI.queue(channelId)
  if (alive && revision === queueRevision && currentChannel.value === channelId) queue.value = value
}
type PanelTab = 'catalog' | 'create' | 'mine' | 'settings'
const tabs: ReadonlyArray<{ value: PanelTab; label: string }> = [
  { value: 'catalog', label: '音色目录' },
  { value: 'create', label: '创建音色' },
  { value: 'mine', label: '我的音色' },
  { value: 'settings', label: '朗读设置' },
]
const lifecycleLabels: Readonly<Record<string, string>> = { creating: '创建中', preview: '预览', saved: '已保存', delete_pending: '删除中' }
const tab = ref<PanelTab>('catalog')
// Lifecycles move in the background (creating -> preview -> expiry); refresh on entry.
watch(tab, value => { if (value === 'mine') void run(load) })
const error = ref('')
const busy = ref(false)
const text = ref('你好，欢迎来到我们的冒险故事。')
// The catalog picks the voice for auditions and design/clone requests; the
// platform default stands in when nothing explicit is chosen.
const selection = ref<VoiceSelection>({ type: 'inherit' })
const presetModelIds = computed(() => speech.quota ? [speech.quota.defaultModel] : [])
const picker = ref<InstanceType<typeof VoicePicker> | null>(null)
const MINE_PAGE_SIZE = 20
const page = ref(1)
const directory = ref<VoiceDirectory>({ items: [], system: [], total: 0, catalogVersion: '' })
const name = ref('')
const description = ref('')
const operation = ref<'design' | 'clone'>('design')
const source = ref<File | null>(null)
let uploadedSource: { file: File; id: string } | null = null
const authorized = ref(false)
const replaceId = ref('')
const replacePage = ref(1)
const replaceSearchInput = ref('')
const replaceSearch = ref('')
const replaceLoading = ref(false)
const replaceDirectory = ref<VoiceDirectory>({ items: [], system: [], total: 0, catalogVersion: '' })
const replaceSelected = ref<SpeechVoice | null>(null)
let replaceSerial = 0
const replaceOptions = computed(() => {
  const items = [...replaceDirectory.value.items]
  if (replaceSelected.value && !items.some(v => v.id === replaceSelected.value!.id)) items.unshift(replaceSelected.value)
  return [
    { label: '保存为新音色', value: '' },
    ...items.map(v => ({ label: `替换 ${v.name || '未命名音色'}（不额外占永久槽位）`, value: v.id })),
  ]
})
const job = ref<SpeechJob | null>(null)
// "我的音色" edits one voice at a time; the selection follows the loaded page
// and falls back to its first item when the voice is no longer on it.
const selectedId = ref('')
const selectedVoice = computed(() => directory.value.items.find(v => v.id === selectedId.value) ?? null)
watch(() => directory.value.items, items => {
  if (!items.some(v => v.id === selectedId.value)) selectedId.value = items[0]?.id ?? ''
})
watch(selectedId, () => {
  replaceId.value = ''
  replaceSelected.value = null
  replacePage.value = 1
  replaceSearchInput.value = ''
  replaceSearch.value = ''
  replaceDirectory.value = { items: [], system: [], total: 0, catalogVersion: '' }
  if (selectedVoice.value?.lifecycle === 'preview') void loadReplaceOptions()
})
let alive = true
let serial = 0
let timer: ReturnType<typeof setTimeout> | undefined
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try { await action() } catch (e) { if (alive) error.value = speechError(e) }
  finally { if (alive) busy.value = false }
}
// Own voices of every lifecycle, for management. Browsing lives in VoicePicker.
async function load() {
  const current = ++serial
  const requestedPage = page.value
  const value = await speechAPI.voices({ mine: true, page: requestedPage })
  if (!alive || current !== serial) return
  const lastPage = Math.max(1, Math.ceil(value.total / MINE_PAGE_SIZE))
  if (value.items.length === 0 && value.total > 0 && requestedPage > lastPage) {
    page.value = lastPage
    await load()
    return
  }
  directory.value = value
}
async function loadReplaceOptions() {
  const current = ++replaceSerial
  replaceLoading.value = true
  try {
    const value = await speechAPI.voices({ scope: 'mine', page: replacePage.value, search: replaceSearch.value || undefined })
    if (alive && current === replaceSerial) replaceDirectory.value = value
  } catch (e) {
    if (alive && current === replaceSerial) error.value = speechError(e)
  } finally {
    if (current === replaceSerial) replaceLoading.value = false
  }
}
function searchReplaceOptions() {
  const next = replaceSearchInput.value.trim()
  if (next === replaceSearch.value && replacePage.value === 1) return
  replaceSearch.value = next
  replacePage.value = 1
  void loadReplaceOptions()
}
function setReplaceId(value: string) {
  replaceId.value = value
  replaceSelected.value = value ? replaceDirectory.value.items.find(v => v.id === value) ?? replaceSelected.value : null
}
function setReplacePage(value: number) {
  replacePage.value = value
  void loadReplaceOptions()
}
async function reloadVoices() {
  picker.value?.reload()
  await load()
  if (selectedVoice.value?.lifecycle === 'preview') await loadReplaceOptions()
}
async function poll(id: string) {
  try {
    const value = await speechAPI.job(id)
    if (!alive || job.value?.id !== id) return
    job.value = value
    if (['queued', 'running', 'storage_pending'].includes(value.status)) {
      timer = setTimeout(() => void poll(id), 1500)
    } else { await reloadVoices(); await speech.refresh() }
  } catch (e) { if (alive) error.value = speechError(e) }
}
async function submit(kind: 'audition' | 'design' | 'clone') {
  const userId = user.info.id
  const selectedFile = source.value
  let sourceResourceId: string | undefined
  if (kind === 'clone' && selectedFile && authorized.value) {
    sourceResourceId = uploadedSource?.file === selectedFile ? uploadedSource.id : await speechAPI.source(selectedFile)
    if (!alive || userId !== user.info.id || selectedFile !== source.value) return
    uploadedSource = { file: selectedFile, id: sourceResourceId }
  }
  if (!alive || userId !== user.info.id) return
  if (kind === 'clone' && !sourceResourceId) throw new Error('请选择自己有权使用的样本并确认授权。')
  const value = await speechAPI.submit(kind, {
    requestKey: crypto.randomUUID(), text: text.value, name: name.value, description: description.value,
    sourceResourceId,
    ...requestVoiceFields(selection.value, speech.quota?.defaultVoice),
  })
  if (!alive || userId !== user.info.id) return
  job.value = value
  void poll(value.id)
}
async function save(voice: SpeechVoice) { await speechAPI.save(voice.id, replaceId.value); replaceId.value = ''; await reloadVoices(); await speech.refresh() }
async function remove(voice: SpeechVoice) { await speechAPI.remove(voice.id); await reloadVoices(); await speech.refresh() }
async function update(voice: SpeechVoice) { if (voice.parameters) JSON.parse(voice.parameters); await speechAPI.update(voice); await reloadVoices() }
onMounted(() => void run(async () => { await speech.refresh(); await load() }))
onBeforeUnmount(() => { alive = false; serial++; replaceSerial++; clearTimeout(timer) })
</script>

<template>
  <NModal v-model:show="speech.visible" :auto-focus="false">
    <section class="sp-shell" role="dialog" aria-modal="true" aria-labelledby="speech-panel-title" :style="{ '--vp-accent': `var(--primary-color, ${theme.primaryColor})`, '--sp-warning': theme.warningColor }">
      <header class="sp-head">
        <h2 id="speech-panel-title">语音朗读</h2>
        <nav class="sp-tabs" aria-label="语音朗读分区">
          <button
            v-for="item in tabs"
            :key="item.value"
            type="button"
            class="sp-tab"
            :class="{ 'is-active': tab === item.value }"
            :aria-current="tab === item.value ? 'page' : undefined"
            @click="tab = item.value"
          >{{ item.label }}</button>
        </nav>
        <button type="button" class="sp-close" aria-label="关闭" @click="speech.visible = false">✕</button>
      </header>
      <div v-if="error || speechPlayer.state.error || (speech.quota && !speech.quota.enabled)" class="sp-alerts">
        <NAlert v-if="error || speechPlayer.state.error" type="error">{{ error || speechPlayer.state.error }}</NAlert>
        <NAlert v-if="speech.quota && !speech.quota.enabled" type="warning">平台未启用新的语音操作；已有语音仍可播放。</NAlert>
      </div>

      <div v-show="tab === 'catalog'" class="sp-body sp-catalog">
        <VoicePicker ref="picker" v-model="selection" mode="browse" :preset-model-ids="presetModelIds" class="sp-picker" />
        <aside class="sp-side">
          <h3>合成试听</h3>
          <p class="sp-hint">使用左侧当前选择的音色合成新文本；已有文件可重放。</p>
          <NInput v-model:value="text" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }" placeholder="输入新的试听文字（最多 500 字）" />
          <NButton type="primary" :disabled="!speech.quota?.enabled || speech.quota.characterPrice == null" :loading="busy" @click="run(() => submit('audition'))">确认合成试听</NButton>
        </aside>
      </div>

      <div v-if="tab === 'create'" class="sp-body sp-scroll">
        <div class="sp-form">
          <div class="sp-segment" role="group" aria-label="创建方式">
            <button type="button" class="sp-tab" :class="{ 'is-active': operation === 'design' }" :aria-pressed="operation === 'design'" @click="operation = 'design'">声音设计</button>
            <button type="button" class="sp-tab" :class="{ 'is-active': operation === 'clone' }" :aria-pressed="operation === 'clone'" @click="operation = 'clone'">样本复刻</button>
          </div>
          <label class="sp-field"><span>音色名称</span><NInput v-model:value="name" placeholder="音色名称" /></label>
          <label class="sp-field"><span>声音描述</span><NInput v-model:value="description" type="textarea" placeholder="描述希望设计的声音" /></label>
          <label class="sp-field"><span>预览文本</span><NInput v-model:value="text" type="textarea" placeholder="15–200 字预览文本" /></label>
          <p class="sp-hint">创建完成后会生成限时预览，可在“我的音色”中保存或替换已有音色。已保存音色 {{ speech.quota?.saved ?? 0 }} / {{ speech.quota?.slots ?? 0 }}。</p>
          <NButton v-if="operation === 'design'" type="primary" :disabled="!speech.quota?.enabled || speech.quota.designPrice == null" :loading="busy" @click="run(() => submit('design'))">确认设计</NButton>
          <template v-else>
            <input type="file" accept="audio/wav,audio/mpeg" @change="event => { source = (event.target as HTMLInputElement).files?.[0] ?? null }" />
            <label class="sp-check"><input v-model="authorized" type="checkbox" />我确认拥有此样本的复刻授权（10–60 秒 WAV/MP3）</label>
            <NButton type="primary" :disabled="!authorized || !source || !speech.quota?.enabled || speech.quota.clonePrice == null" :loading="busy" @click="run(() => submit('clone'))">确认复刻</NButton>
          </template>
        </div>
      </div>

      <div v-if="tab === 'mine'" class="sp-body sp-mine">
        <p class="sp-hint">已保存音色 {{ speech.quota?.saved ?? 0 }} / {{ speech.quota?.slots ?? 0 }}。公开自己的音色仍占槽位；删除音色不影响已经生成的消息文件。</p>
        <p v-if="!directory.items.length" class="sp-empty">还没有个人音色。</p>
        <div v-else class="sp-mine__split">
          <div class="sp-mine__list">
            <div class="sp-mine__items" role="listbox" aria-label="我的音色">
              <button
                v-for="voice in directory.items"
                :key="voice.id"
                type="button"
                role="option"
                class="sp-mine__item"
                :class="{ 'is-active': voice.id === selectedId }"
                :aria-selected="voice.id === selectedId"
                @click="selectedId = voice.id"
              >
                <strong>{{ voice.name || '未命名音色' }}</strong>
                <small><span :class="`is-${voice.lifecycle}`">{{ lifecycleLabels[voice.lifecycle] ?? voice.lifecycle }}</span> · {{ voice.isPublic ? '公开' : '私有' }}</small>
              </button>
            </div>
            <NPagination v-if="directory.total > 20" v-model:page="page" :item-count="directory.total" :page-size="20" :page-slot="5" size="small" @update:page="run(load)" />
          </div>
          <article v-if="selectedVoice" :key="selectedVoice.id" class="sp-voice">
            <div class="sp-voice__head">
              <strong>{{ selectedVoice.name || '未命名音色' }}</strong>
              <span class="sp-voice__meta">{{ voiceKindLabel(selectedVoice.kind) }} · {{ lifecycleLabels[selectedVoice.lifecycle] ?? selectedVoice.lifecycle }} · {{ selectedVoice.providerStatus }} · {{ selectedVoice.isPublic ? '公开' : '私有' }}</span>
            </div>
            <span class="sp-voice__meta">{{ selectedVoice.targetModel }}<template v-if="selectedVoice.tags"> · {{ selectedVoice.tags }}</template></span>
            <span v-if="selectedVoice.previewExpiresAt" class="sp-voice__meta">预览到期：{{ selectedVoice.previewExpiresAt }}</span>
            <template v-if="selectedVoice.lifecycle === 'saved'">
              <div class="sp-voice__edit">
                <label class="sp-field"><span>名称</span><NInput v-model:value="selectedVoice.name" placeholder="音色名称" /></label>
                <label class="sp-field"><span>标签</span><NInput v-model:value="selectedVoice.tags" placeholder="标签" /></label>
                <label class="sp-field is-wide"><span>描述</span><NInput v-model:value="selectedVoice.description" placeholder="描述" /></label>
                <label class="sp-field is-wide"><span>默认参数</span><NInput v-model:value="selectedVoice.parameters" type="textarea" placeholder='默认参数 JSON，例如 {"rate":1,"pitch":1,"volume":50}' /></label>
                <label class="sp-check">公开音色 <NSwitch v-model:value="selectedVoice.isPublic" /></label>
              </div>
              <div class="sp-voice__actions">
                <NButton v-if="selectedVoice.previewResourceId" size="small" @click="speechPlayer.play('resources', selectedVoice.previewResourceId)">重放 / 停止</NButton>
                <NButton size="small" type="primary" :loading="busy" @click="run(() => update(selectedVoice!))">保存本地名称、标签、公开状态与参数</NButton>
                <NButton size="small" :loading="busy" @click="run(() => remove(selectedVoice!))">删除</NButton>
              </div>
            </template>
            <template v-else-if="selectedVoice.lifecycle === 'preview'">
              <div class="sp-replace">
                <div class="sp-replace__search">
                  <NInput v-model:value="replaceSearchInput" clearable placeholder="搜索要替换的已保存音色" @keyup.enter="searchReplaceOptions" @clear="() => { replaceSearchInput = ''; searchReplaceOptions() }" />
                  <NButton size="small" :loading="replaceLoading" @click="searchReplaceOptions">搜索</NButton>
                </div>
                <NSelect :value="replaceId" :options="replaceOptions" :loading="replaceLoading" @update:value="setReplaceId" />
                <NPagination v-if="replaceDirectory.total > MINE_PAGE_SIZE" :page="replacePage" :item-count="replaceDirectory.total" :page-size="MINE_PAGE_SIZE" :page-slot="5" size="small" :disabled="replaceLoading" @update:page="setReplacePage" />
              </div>
              <small v-if="replaceId" class="sp-hint">替换将保留正在执行任务的旧版本；原角色绑定失效后需重新选择新版本，不自动换声。</small>
              <div class="sp-voice__actions">
                <NButton v-if="selectedVoice.previewResourceId" size="small" @click="speechPlayer.play('resources', selectedVoice.previewResourceId)">试听 / 停止</NButton>
                <NButton size="small" type="primary" :loading="busy" @click="run(() => save(selectedVoice!))">{{ replaceId ? '替换所选音色' : '保存为新音色' }}</NButton>
                <NButton size="small" :loading="busy" @click="run(() => remove(selectedVoice!))">丢弃预览</NButton>
              </div>
            </template>
            <p v-else-if="selectedVoice.lifecycle === 'creating'" class="sp-hint">音色创建中，完成后会生成限时预览，届时可在此试听并保存。</p>
            <p v-else-if="selectedVoice.lifecycle === 'delete_pending'" class="sp-hint">音色删除中，完成后会从列表移除。</p>
          </article>
        </div>
      </div>

      <div v-if="tab === 'settings'" class="sp-body sp-scroll">
        <div class="sp-settings">
          <label class="sp-check">自动合成 <NSwitch :value="speech.quota?.autoSynthesis ?? false" :loading="busy" @update:value="value => run(async () => { await speechAPI.settings(value); await speech.refresh() })" /></label>
          <label class="sp-check">当前浏览器自动播放 <NSwitch :value="speechPlayer.state.preferred" @update:value="value => run(() => speechPlayer.setAutomatic(value))" /></label>
          <span v-if="speechPlayer.state.preferred && !speechPlayer.state.automatic" class="autoplay-pending">等待页面交互后自动启用</span>
          <span>已保存音色 {{ speech.quota?.saved ?? 0 }} / {{ speech.quota?.slots ?? 0 }}</span>
        </div>
        <SpeechQuotaSummary v-if="speech.quota" :quota="speech.quota" />
        <NAlert type="info">PCM16 WAV 支持增量播放（约 300ms 预缓冲）；MP3 使用完整文件兼容模式。当前生产格式仅支持 WAV / MP3。自动播放不会合成新音频；手动重放或试听会暂停本地自动播放。</NAlert>
        <div v-if="currentChannel" class="sp-settings">
          <NButton @click="run(loadQueue)">查询频道朗读队列</NButton>
          <span v-if="queue">可见待处理任务 {{ queue.items.length }}</span>
          <template v-if="queue?.canControl">
            <NButton @click="run(async () => { await speechAPI.control(currentChannel!, 'skip'); await loadQueue() })">GM 跳过当前</NButton>
            <NButton @click="run(async () => { await speechAPI.control(currentChannel!, 'clear'); await loadQueue() })">GM 清空队列</NButton>
          </template>
        </div>
      </div>

      <footer v-if="job" class="sp-foot">
        <span>任务：{{ job.status }} {{ job.errorCode }}</span>
        <div class="sp-foot__actions">
          <NButton v-if="job.audioResourceId" size="small" @click="speechPlayer.play('resources', job.audioResourceId)">播放 / 停止已有试听</NButton>
          <NButton size="small" @click="run(() => poll(job!.id))">查询状态</NButton>
        </div>
      </footer>
    </section>
  </NModal>
</template>

<style scoped>
/* NModal adds `.n-modal` to this root, which custom themes paint with
   --sc-bg-elevated; every palette uses the same surface here. */
.sp-shell {
  display: flex;
  flex-direction: column;
  width: min(1000px, 94vw);
  height: min(760px, 88vh);
  overflow: hidden;
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  background: var(--sc-bg-elevated);
  color: var(--sc-text-primary);
  box-shadow: 0 16px 40px rgba(0, 0, 0, .25);
}
.sp-head { display: flex; flex: none; align-items: center; gap: 16px; padding: 16px 22px 12px; }
.sp-head h2 { flex: none; margin: 0; font-size: 18px; font-weight: 600; }
.sp-tabs, .sp-segment { display: flex; flex: 1; flex-wrap: wrap; gap: 4px; min-width: 0; }
.sp-segment { flex: none; }
.sp-tab {
  padding: 5px 12px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--sc-text-secondary);
  font: inherit;
  font-size: 14px;
  cursor: pointer;
  transition: color .15s ease, background-color .15s ease;
}
.sp-tab:hover { color: var(--sc-text-primary); }
.sp-tab.is-active { background: color-mix(in srgb, var(--vp-accent) 14%, transparent); color: var(--sc-text-primary); }
.sp-tab:focus-visible, .sp-close:focus-visible { outline: 2px solid var(--vp-accent); outline-offset: 1px; }
.sp-close {
  flex: none;
  width: 32px;
  height: 32px;
  margin-left: auto;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--sc-text-secondary);
  font-size: 16px;
  cursor: pointer;
}
.sp-close:hover { background: color-mix(in srgb, var(--sc-text-primary) 8%, transparent); color: var(--sc-text-primary); }
.sp-alerts { display: flex; flex: none; flex-direction: column; gap: 8px; padding: 0 22px 12px; }
.sp-body { flex: 1 1 auto; min-height: 0; padding: 0 22px 16px; }
.sp-scroll { display: flex; flex-direction: column; gap: 14px; overflow-y: auto; }
.sp-catalog { display: grid; grid-template-columns: minmax(0, 1fr) 280px; grid-template-rows: minmax(0, 1fr); gap: 22px; }
.sp-picker { min-height: 0; }
.sp-side { display: flex; flex-direction: column; gap: 12px; min-height: 0; padding-left: 22px; overflow-y: auto; border-left: 1px solid var(--sc-border-mute); }
.sp-side h3 { margin: 0; font-size: 14px; font-weight: 600; }
.sp-hint { margin: 0; font-size: 12px; line-height: 1.6; color: var(--sc-text-secondary); }
.sp-empty { margin: 24px 0; text-align: center; color: var(--sc-text-secondary); }
.sp-form { display: flex; flex-direction: column; gap: 14px; max-width: 640px; }
.sp-field { display: flex; flex-direction: column; gap: 4px; min-width: 0; font-size: 12px; color: var(--sc-text-secondary); }
.sp-check { display: inline-flex; align-items: center; gap: 8px; font-size: 13px; }
/* Master-detail: the list and the single detail scroll independently. */
.sp-mine { display: flex; flex-direction: column; gap: 12px; }
.sp-mine__split { display: grid; flex: 1 1 auto; grid-template-columns: 280px minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); gap: 16px; min-height: 0; }
.sp-mine__list { display: flex; flex-direction: column; gap: 8px; min-width: 0; min-height: 0; }
.sp-mine__items { display: flex; flex: 1 1 auto; flex-direction: column; gap: 2px; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
.sp-mine__item {
  display: flex;
  flex: none;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
  min-height: 52px;
  padding: 6px 10px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: var(--sc-text-primary);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.sp-mine__item:hover { background: color-mix(in srgb, var(--sc-text-primary) 6%, transparent); }
.sp-mine__item.is-active { border-color: color-mix(in srgb, var(--vp-accent) 55%, transparent); background: color-mix(in srgb, var(--vp-accent) 12%, transparent); }
.sp-mine__item:focus-visible { outline: 2px solid var(--vp-accent); outline-offset: 1px; }
.sp-mine__item strong { overflow: hidden; font-size: 14px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.sp-mine__item small { font-size: 12px; color: var(--sc-text-secondary); }
.sp-mine__item .is-preview, .sp-mine__item .is-creating { color: var(--vp-accent); }
.sp-mine__item .is-delete_pending { color: var(--sp-warning); }
.sp-voice { display: flex; flex-direction: column; gap: 8px; min-width: 0; min-height: 0; padding: 14px 16px; overflow-y: auto; border: 1px solid var(--sc-border-mute); border-radius: 8px; }
.sp-voice__head { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 12px; }
.sp-voice__meta { font-size: 12px; color: var(--sc-text-secondary); overflow-wrap: anywhere; }
.sp-voice__edit { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.sp-voice__edit .is-wide { grid-column: 1 / -1; }
.sp-voice__actions { display: flex; flex-wrap: wrap; gap: 6px; }
.sp-replace { display: flex; flex-direction: column; gap: 8px; }
.sp-replace__search { display: flex; align-items: center; gap: 8px; }
.sp-replace__search > :first-child { flex: 1 1 auto; min-width: 0; }
.sp-settings { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 20px; font-size: 13px; }
.sp-foot {
  display: flex;
  flex: none;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 12px;
  padding: 10px 22px;
  border-top: 1px solid var(--sc-border-mute);
  font-size: 13px;
}
.sp-foot__actions { display: flex; flex-wrap: wrap; gap: 6px; }
.autoplay-pending { font-size: 12px; opacity: .6; }

@media (max-width: 720px) {
  .sp-shell { width: 100vw; max-width: 100%; height: 100vh; height: 100dvh; border: 0; border-radius: 0; }
  .sp-head { flex-wrap: wrap; gap: 8px 12px; padding: 12px 16px 8px; }
  .sp-tabs { order: 3; flex-basis: 100%; }
  .sp-alerts { padding: 0 16px 10px; }
  .sp-body { padding: 0 16px 16px; }
  .sp-catalog { display: block; overflow-y: auto; }
  .sp-side { margin-top: 20px; padding: 16px 0 0; overflow: visible; border-top: 1px solid var(--sc-border-mute); border-left: 0; }
  .sp-mine { overflow-y: auto; }
  .sp-mine__split { display: flex; flex: none; flex-direction: column; gap: 12px; }
  .sp-mine__items { flex: none; max-height: 220px; }
  .sp-voice { flex: none; overflow: visible; }
  .sp-voice__edit { grid-template-columns: minmax(0, 1fr); }
  .sp-replace__search { align-items: stretch; }
  .sp-foot { padding: 10px 16px calc(10px + env(safe-area-inset-bottom)); }
}
</style>
