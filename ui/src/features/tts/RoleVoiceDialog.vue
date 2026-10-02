<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NAlert, NButton, NIcon, NInput, NInputNumber, NModal, NSelect } from 'naive-ui'
import { Volume, Volume2 } from '@vicons/tabler'
import { speechAPI, speechError } from './api'
import { useSpeechStore } from './store'
import type { RoleSpeechConfig, SpeechJob } from './types'
import { speechPlayer } from './player'
import { useUserStore } from '@/stores/user'
import VoicePicker from './VoicePicker.vue'
import { compatibleSpeechLanguage, isTencentMPSModel, roleVoiceFields, roleVoiceSelection, speechLanguageOptions, type VoiceSelection } from './voice-catalog'
const props = defineProps<{ identityId: string; identityName?: string }>()
const speech = useSpeechStore()
const user = useUserStore()
let generation = 0
const visible = ref(false)
const error = ref('')
const busy = ref(false)
const text = ref('你好，这是我的角色语音试听。')
const job = ref<SpeechJob | null>(null)
const auditionPending = ref(false)
const auditionResourceId = ref('')
const auditionResultKey = ref('')
const auditionDraftReady = ref(false)
const auditionReplayAvailable = computed(() => !!auditionResourceId.value && auditionResultKey.value === auditionDraftKey.value)
const auditionActive = computed(() => auditionReplayAvailable.value && speechPlayer.state.key === `resources:${auditionResourceId.value}`)
let auditionTimer: ReturnType<typeof setTimeout> | undefined
let auditionTimeout: ReturnType<typeof setTimeout> | undefined
function clearAuditionTimers() {
  clearTimeout(auditionTimer)
  clearTimeout(auditionTimeout)
  auditionTimer = auditionTimeout = undefined
}
function clearAuditionRuntime() {
  clearAuditionTimers()
  // Ownership survives a draft change even though replay is already invalid.
  if (auditionResourceId.value && speechPlayer.state.key === `resources:${auditionResourceId.value}`) speechPlayer.stop()
  auditionPending.value = false
  job.value = null
}
function clearAuditionResult() {
  auditionResourceId.value = ''
  auditionResultKey.value = ''
  try { sessionStorage.removeItem(auditionStorageKey.value) } catch { /* Storage is optional. */ }
}
const role = ref<RoleSpeechConfig | null>(null)
const selection = ref<VoiceSelection>({ type: 'inherit' })
const auditionDraftKey = computed(() => JSON.stringify({
  ...roleVoiceFields(selection.value),
  text: text.value.trim(),
  speechLanguage: role.value?.speechLanguage ?? '',
  instruction: role.value?.instruction ?? '',
  rate: role.value?.rate ?? 1,
  pitch: role.value?.pitch ?? 1,
  volume: role.value?.volume ?? 50,
}))
const auditionStorageKey = computed(() => `sealchat:tts:audition:${user.info.id}:${props.identityId}`)
function saveAuditionResult() {
  try {
    sessionStorage.setItem(auditionStorageKey.value, JSON.stringify({ version: 1, draftKey: auditionResultKey.value, resourceId: auditionResourceId.value, expiresAt: Date.now() + 30 * 60 * 1000 }))
  } catch { /* Storage is optional. */ }
}
function restoreAuditionResult() {
  try {
    const stored = JSON.parse(sessionStorage.getItem(auditionStorageKey.value) ?? 'null')
    if (stored?.version === 1 && typeof stored.resourceId === 'string' && stored.resourceId.trim() && typeof stored.expiresAt === 'number' && stored.expiresAt > Date.now() && stored.draftKey === auditionDraftKey.value) {
      auditionResourceId.value = stored.resourceId
      auditionResultKey.value = stored.draftKey
      return
    }
  } catch { /* Invalid or unavailable storage must not interrupt auditions. */ }
  clearAuditionResult()
}
const speechLanguages = ref<string[] | null>(null)
const systemModel = ref('')
const tencentTraditional = computed(() => systemModel.value === 'tencent-tts-classic' || systemModel.value === 'tencent-tts-large')
const tencentMPS = computed(() => isTencentMPSModel(systemModel.value))
const instructionDisabled = computed(() => tencentTraditional.value || tencentMPS.value)
const incompatibleTencentParameters = computed(() => tencentTraditional.value && !!role.value && (role.value.pitch !== 1 || role.value.instruction !== ''))
const incompatibleMPSInstruction = computed(() => tencentMPS.value && !!role.value?.instruction)
const incompatibleParameters = computed(() => incompatibleTencentParameters.value || incompatibleMPSInstruction.value)
function clearMPSInstruction() { if (role.value) role.value.instruction = '' }
function resetTencentParameters() {
  if (role.value) { role.value.pitch = 1; role.value.instruction = '' }
}
const languageOptions = computed(() => speechLanguageOptions(speechLanguages.value ?? []))
watch([speechLanguages, () => role.value?.speechLanguage], () => {
  if (role.value && speechLanguages.value !== null) {
    role.value.speechLanguage = compatibleSpeechLanguage(role.value.speechLanguage ?? '', speechLanguages.value)
  }
}, { flush: 'sync' })
watch([() => user.info.id, () => props.identityId, () => speech.quota?.enabled], () => { generation++; visible.value = false; role.value = null; selection.value = { type: 'inherit' }; job.value = null; busy.value = false }, { flush: 'sync' })
watch(visible, () => {
  generation++
  busy.value = false
  auditionDraftReady.value = false
  clearAuditionRuntime()
  auditionResourceId.value = ''
  auditionResultKey.value = ''
}, { flush: 'sync' })
onBeforeUnmount(() => { generation++; clearAuditionRuntime() })
// Picker metadata arrives asynchronously. Wait for its language/model emits
// and language normalization before comparing the initialized draft to storage.
watch([busy, speechLanguages, systemModel, visible], () => {
  if (!visible.value || busy.value || !role.value || speechLanguages.value === null || auditionDraftReady.value) return
  if (selection.value.type === 'system' && !systemModel.value) return
  restoreAuditionResult()
  auditionDraftReady.value = true
}, { flush: 'post' })
watch(auditionDraftKey, key => {
  if (!auditionDraftReady.value || key === auditionResultKey.value) return
  if (auditionPending.value) generation++
  clearAuditionRuntime()
  clearAuditionResult()
}, { flush: 'sync' })
const titleId = computed(() => `role-voice-title-${props.identityId}`)
// Keep the picker-side selection separate from the persisted role fields so a
// system voice can retain its model/provider-qualified identity while editing.
const voiceContext = computed(() => speech.quota?.voiceContext ?? null)
const voiceContexts = computed(() => speech.quota?.voiceContexts?.length ? speech.quota.voiceContexts : voiceContext.value ? [voiceContext.value] : [])
async function open() {
  if (visible.value || !speech.quota?.enabled) return
  visible.value = true
  busy.value = true
  error.value = ''
  role.value = null
  speechLanguages.value = null
  systemModel.value = ''
  job.value = null
  const identityId = props.identityId
  const current = generation
  try {
    await speech.refresh(speech.scopeChannel)
    if (current !== generation || !visible.value || identityId !== props.identityId) return
    const value = await speechAPI.role(identityId)
    if (current !== generation || !visible.value || identityId !== props.identityId) return
    selection.value = roleVoiceSelection(value)
    role.value = { ...value, speechLanguage: value.speechLanguage ?? '' }
  } catch (e) { if (current === generation) error.value = speechError(e) }
  finally { if (current === generation) busy.value = false }
}
async function save() {
  if (!role.value || busy.value || incompatibleParameters.value) return
  busy.value = true
  error.value = ''
  const current = generation
  try {
    await speechAPI.saveRole(props.identityId, { ...role.value, ...roleVoiceFields(selection.value), speechLanguage: role.value.speechLanguage })
    if (current === generation) visible.value = false
  }
  catch (e) { if (current === generation) error.value = speechError(e) }
  finally { if (current === generation) busy.value = false }
}
async function audition() {
  if (!role.value || !auditionDraftReady.value || busy.value || auditionPending.value || incompatibleParameters.value) return
  clearAuditionRuntime()
  clearAuditionResult()
  auditionPending.value = true
  error.value = ''
  const current = ++generation
  try {
    await speechPlayer.unlock()
    if (current !== generation || !visible.value || !auditionPending.value) return
    // Stop accepting an in-flight GET/POST if the frontend deadline expires.
    auditionTimeout = setTimeout(() => {
      if (current !== generation || !visible.value || !auditionPending.value) return
      clearAuditionTimers()
      auditionPending.value = false
      error.value = '试听生成超时，请稍后再试。'
    }, 3 * 60 * 1000)
    const value = await speechAPI.submit('audition', { ...role.value, ...roleVoiceFields(selection.value), speechLanguage: role.value.speechLanguage, text: text.value, requestKey: crypto.randomUUID() })
    if (current !== generation || !visible.value || !auditionPending.value || job.value !== null) return
    job.value = value
    followAudition(current, value.id)
  }
  catch (e) {
    if (current !== generation || !visible.value || !auditionPending.value || job.value !== null) return
    clearAuditionTimers()
    auditionPending.value = false
    error.value = speechError(e)
  }
}
function followAudition(current: number, id: string) {
  if (current !== generation || !visible.value || !auditionPending.value || job.value?.id !== id) return
  const value = job.value
  if (['pending', 'queued', 'running', 'storage_pending', 'archiving'].includes(value.status)) {
    auditionTimer = setTimeout(() => { auditionTimer = undefined; void pollAudition(current, id) }, 500)
    return
  }
  clearAuditionTimers()
  auditionPending.value = false
  if (value.status === 'succeeded' && value.audioResourceId) {
    auditionResourceId.value = value.audioResourceId
    auditionResultKey.value = auditionDraftKey.value
    saveAuditionResult()
    playAudition(value.audioResourceId, current)
  } else {
    error.value = value.message || '试听生成失败，请稍后再试。'
  }
}
function playAudition(resourceId: string, current: number) {
  if (current !== generation || !visible.value || !auditionReplayAvailable.value || auditionResourceId.value !== resourceId) return
  error.value = ''
  void speechPlayer.play('resources', resourceId, false, result => {
    if (result !== 'failed' || current !== generation || !visible.value || !auditionReplayAvailable.value || auditionResourceId.value !== resourceId) return
    error.value = speechPlayer.state.error || '试听播放失败，请点击声音图标重试。'
  })
}
function replayAudition() {
  if (auditionReplayAvailable.value) playAudition(auditionResourceId.value, generation)
}
async function pollAudition(current: number, id: string) {
  if (current !== generation || !visible.value || !auditionPending.value || job.value?.id !== id) return
  try {
    const value = await speechAPI.job(id)
    if (current !== generation || !visible.value || !auditionPending.value || job.value?.id !== id) return
    job.value = value
    followAudition(current, id)
  } catch (e) {
    if (current !== generation || !visible.value || !auditionPending.value || job.value?.id !== id) return
    clearAuditionTimers()
    auditionPending.value = false
    error.value = speechError(e)
  }
}
</script>
<template>
  <NButton v-if="speech.quota?.enabled" text size="small" @click="open">音色</NButton>
  <NModal v-model:show="visible" :auto-focus="false" :trap-focus="false">
    <section class="rv-shell" role="dialog" aria-modal="true" :aria-labelledby="titleId">
      <header class="rv-head">
        <div class="rv-head__text">
          <h2 :id="titleId">选择角色音色</h2>
          <p>{{ identityName ? `角色：${identityName}` : '为当前角色绑定朗读音色' }}</p>
        </div>
        <button type="button" class="rv-close" aria-label="关闭" @click="visible = false">✕</button>
      </header>
      <NAlert v-if="error" type="error" class="rv-alert">{{ error }}</NAlert>
      <div v-if="role" class="rv-body">
        <VoicePicker v-model="selection" mode="select" :voice-context="voiceContext" :voice-contexts="voiceContexts" :default-voice="speech.quota?.defaultVoice" class="rv-picker" @speech-languages="speechLanguages = $event" @system-model="systemModel = $event" />
        <aside class="rv-side">
          <section class="rv-section">
            <h3>角色语音参数</h3>
            <label class="rv-field">
              <span>朗读语言</span>
              <NSelect v-model:value="role.speechLanguage" :options="languageOptions" />
            </label>
            <p class="rv-hint">指定语言后，使用平台 AI 转换全文并计入文本额度；跟随原文直接朗读。</p>
            <label class="rv-field">
              <span>朗读指令</span>
              <NInput v-model:value="role.instruction" :disabled="instructionDisabled" placeholder="朗读指令（不调用文本模型）" />
            </label>
            <p v-if="tencentTraditional" class="rv-hint">腾讯传统 TTS 不支持朗读指令或音调调整。</p>
            <p v-if="tencentMPS" class="rv-hint">腾讯 MPS MiniMax 当前不支持自由朗读指令；情绪参数将在后续独立接入。</p>
            <NAlert v-if="incompatibleMPSInstruction" type="warning">
              当前参数包含腾讯 MPS 不支持的自由朗读指令，请确认后清除。
              <NButton size="small" @click="clearMPSInstruction">清除指令</NButton>
            </NAlert>
            <NAlert v-if="incompatibleTencentParameters" type="warning">
              当前参数包含腾讯不支持的朗读指令或音调，请确认后恢复默认参数。
              <NButton size="small" @click="resetTencentParameters">清除指令并将音调恢复为 1</NButton>
            </NAlert>
            <div class="rv-numbers">
              <label class="rv-field"><span>语速</span><NInputNumber v-model:value="role.rate" :min="0.5" :max="2" :step="0.1" /></label>
              <label class="rv-field"><span>音调</span><NInputNumber :value="tencentTraditional ? 1 : role.pitch" :disabled="tencentTraditional" :min="0.5" :max="2" :step="0.1" @update:value="role.pitch = $event ?? 1" /></label>
              <label class="rv-field"><span>音量</span><NInputNumber v-model:value="role.volume" :min="0" :max="100" /></label>
            </div>
          </section>
          <section class="rv-section">
            <h3>试听</h3>
            <NInput v-model:value="text" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="试听文字" />
            <p class="rv-hint">按当前选择与参数合成新音频；已有结果可重放。</p>
            <NButton :disabled="busy || !auditionDraftReady || auditionPending || !speech.canSynthesize || incompatibleParameters" @click="audition">确认试听</NButton>
            <div v-if="auditionPending || auditionReplayAvailable" class="rv-audition" role="status" aria-live="polite" :aria-busy="auditionPending">
              <NIcon v-if="auditionPending" :component="Volume2" size="22" class="rv-audition__generating" aria-hidden="true" />
              <NButton v-else text :class="{ 'rv-audition__playing': auditionActive && speechPlayer.state.playing }" :aria-label="auditionActive ? '停止试听' : '重播试听'" :aria-pressed="auditionActive" @click="replayAudition">
                <NIcon :component="auditionActive ? Volume : Volume2" size="22" />
              </NButton>
              <span>{{ auditionPending ? '正在生成试听…' : auditionActive ? (speechPlayer.state.loading ? '正在加载试听…' : '正在播放试听，点击停止') : '点击声音图标重播' }}</span>
            </div>
          </section>
        </aside>
      </div>
      <p v-else class="rv-empty">{{ busy ? '正在读取角色音色…' : '' }}</p>
      <footer class="rv-foot">
        <span class="rv-foot__hint">选择“跟随平台默认音色”并保存即可清除绑定。</span>
        <div class="rv-foot__actions">
          <NButton @click="visible = false">取消</NButton>
          <NButton type="primary" :loading="busy" :disabled="!role || incompatibleParameters" @click="save">保存绑定</NButton>
        </div>
      </footer>
    </section>
  </NModal>
</template>

<style scoped>
/* NModal adds `.n-modal` to this root, which custom themes paint with
   --sc-bg-elevated; every palette uses the same surface here. */
.rv-shell {
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
.rv-head { display: flex; flex: none; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 18px 22px 12px; }
.rv-head h2 { margin: 0; font-size: 18px; font-weight: 600; }
.rv-head p { margin: 4px 0 0; font-size: 13px; color: var(--sc-text-secondary); }
.rv-head__text { min-width: 0; }
.rv-close {
  flex: none;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--sc-text-secondary);
  font-size: 16px;
  cursor: pointer;
}
.rv-close:hover { background: color-mix(in srgb, var(--sc-text-primary) 8%, transparent); color: var(--sc-text-primary); }
.rv-alert { flex: none; margin: 0 22px 12px; }
.rv-body {
  display: grid;
  flex: 1 1 auto;
  grid-template-columns: minmax(0, 1fr) 280px;
  grid-template-rows: minmax(0, 1fr);
  gap: 22px;
  min-height: 0;
  padding: 0 22px 16px;
}
.rv-picker { min-height: 0; }
.rv-side {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-height: 0;
  padding-left: 22px;
  overflow-y: auto;
  border-left: 1px solid var(--sc-border-mute);
}
.rv-section { display: flex; flex-direction: column; gap: 10px; }
.rv-section h3 { margin: 0; font-size: 14px; font-weight: 600; }
.rv-field { display: flex; flex-direction: column; gap: 4px; min-width: 0; font-size: 12px; color: var(--sc-text-secondary); }
.rv-numbers { display: grid; grid-template-columns: minmax(0, 1fr); gap: 8px; }
.rv-numbers .rv-field { display: grid; grid-template-columns: 3em minmax(0, 1fr); align-items: center; }
.rv-hint { margin: 0; font-size: 12px; color: var(--sc-text-secondary); }
.rv-audition { display: flex; align-items: center; gap: 8px; min-width: 0; font-size: 13px; color: var(--sc-text-secondary); }
.rv-audition > span { min-width: 0; overflow-wrap: anywhere; }
.rv-audition :deep(.n-icon), .rv-audition :deep(.n-button) { flex: none; }
.rv-audition__generating { animation: rv-audition-rotate 1s linear infinite; }
.rv-audition__playing :deep(.n-icon) { animation: rv-audition-pulse 1.2s ease-in-out infinite; }
.rv-audition__playing { color: var(--sc-text-primary); }
@keyframes rv-audition-rotate { to { transform: rotate(360deg); } }
@keyframes rv-audition-pulse { 50% { opacity: .45; transform: scale(.9); } }
.rv-empty { flex: 1 1 auto; margin: 0; padding: 24px; text-align: center; color: var(--sc-text-secondary); }
.rv-foot {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 22px;
  border-top: 1px solid var(--sc-border-mute);
}
.rv-foot__hint { min-width: 0; font-size: 12px; color: var(--sc-text-secondary); }
.rv-foot__actions { display: flex; flex: none; gap: 8px; }

@media (max-width: 720px) {
  .rv-shell { width: 100vw; max-width: 100%; height: 100vh; height: 100dvh; border: 0; border-radius: 0; }
  .rv-head { padding: 14px 16px 10px; }
  .rv-alert { margin: 0 16px 10px; }
  .rv-body { display: block; padding: 0 16px 16px; overflow-y: auto; }
  .rv-side { margin-top: 20px; padding: 16px 0 0; overflow: visible; border-top: 1px solid var(--sc-border-mute); border-left: 0; }
  .rv-foot { padding: 10px 16px calc(10px + env(safe-area-inset-bottom)); }
  .rv-foot__hint { display: none; }
  .rv-foot__actions { flex: 1; justify-content: flex-end; }
}
</style>
