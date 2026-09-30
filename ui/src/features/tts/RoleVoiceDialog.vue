<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NAlert, NButton, NInput, NInputNumber, NModal, NSelect } from 'naive-ui'
import { speechAPI, speechError } from './api'
import { useSpeechStore } from './store'
import type { RoleSpeechConfig, SpeechJob } from './types'
import { speechPlayer } from './player'
import { useUserStore } from '@/stores/user'
import VoicePicker from './VoicePicker.vue'
import { compatibleSpeechLanguage, roleVoiceFields, roleVoiceSelection, speechLanguageOptions, type VoiceSelection } from './voice-catalog'
const props = defineProps<{ identityId: string; identityName?: string }>()
const speech = useSpeechStore()
const user = useUserStore()
let generation = 0
const visible = ref(false)
const error = ref('')
const busy = ref(false)
const text = ref('你好，这是我的角色语音试听。')
const job = ref<SpeechJob | null>(null)
const role = ref<RoleSpeechConfig | null>(null)
const selection = ref<VoiceSelection>({ type: 'inherit' })
const speechLanguages = ref<string[] | null>(null)
const languageOptions = computed(() => speechLanguageOptions(speechLanguages.value ?? []))
watch([speechLanguages, () => role.value?.speechLanguage], () => {
  if (role.value && speechLanguages.value !== null) {
    role.value.speechLanguage = compatibleSpeechLanguage(role.value.speechLanguage ?? '', speechLanguages.value)
  }
}, { flush: 'sync' })
watch([() => user.info.id, () => props.identityId], () => { generation++; visible.value = false; role.value = null; selection.value = { type: 'inherit' }; job.value = null; busy.value = false }, { flush: 'sync' })
watch(visible, () => { generation++; busy.value = false }, { flush: 'sync' })
onBeforeUnmount(() => { generation++ })
const titleId = computed(() => `role-voice-title-${props.identityId}`)
// Keep the picker-side selection separate from the persisted role fields so a
// system voice can retain its model/provider-qualified identity while editing.
const voiceContext = computed(() => speech.quota?.voiceContext ?? null)
const voiceContexts = computed(() => speech.quota?.voiceContexts?.length ? speech.quota.voiceContexts : voiceContext.value ? [voiceContext.value] : [])
async function open() {
  if (visible.value) return
  visible.value = true
  busy.value = true
  error.value = ''
  role.value = null
  speechLanguages.value = null
  job.value = null
  const identityId = props.identityId
  const current = generation
  try {
    await speech.refresh()
    if (current !== generation || !visible.value || identityId !== props.identityId) return
    const value = await speechAPI.role(identityId)
    if (current !== generation || !visible.value || identityId !== props.identityId) return
    selection.value = roleVoiceSelection(value)
    role.value = { ...value, speechLanguage: value.speechLanguage ?? '' }
  } catch (e) { if (current === generation) error.value = speechError(e) }
  finally { if (current === generation) busy.value = false }
}
async function save() {
  if (!role.value || busy.value) return
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
  if (!role.value || busy.value) return
  busy.value = true
  error.value = ''
  const current = generation
  try {
    const value = await speechAPI.submit('audition', { ...role.value, ...roleVoiceFields(selection.value), speechLanguage: role.value.speechLanguage, text: text.value, requestKey: crypto.randomUUID() })
    if (current === generation) job.value = value
  }
  catch (e) { if (current === generation) error.value = speechError(e) }
  finally { if (current === generation) busy.value = false }
}
async function query() {
  if (!job.value || busy.value) return
  const id = job.value.id
  const current = generation
  busy.value = true
  try {
    const value = await speechAPI.job(id)
    if (current === generation && job.value?.id === id) job.value = value
  } catch (e) { if (current === generation) error.value = speechError(e) }
  finally { if (current === generation) busy.value = false }
}
</script>
<template>
  <NButton text size="small" @click="open">音色</NButton>
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
        <VoicePicker v-model="selection" mode="select" :voice-context="voiceContext" :voice-contexts="voiceContexts" :default-voice="speech.quota?.defaultVoice" class="rv-picker" @speech-languages="speechLanguages = $event" />
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
              <NInput v-model:value="role.instruction" placeholder="朗读指令（不调用文本模型）" />
            </label>
            <div class="rv-numbers">
              <label class="rv-field"><span>语速</span><NInputNumber v-model:value="role.rate" :min="0.5" :max="2" :step="0.1" /></label>
              <label class="rv-field"><span>音调</span><NInputNumber v-model:value="role.pitch" :min="0.5" :max="2" :step="0.1" /></label>
              <label class="rv-field"><span>音量</span><NInputNumber v-model:value="role.volume" :min="0" :max="100" /></label>
            </div>
          </section>
          <section class="rv-section">
            <h3>试听</h3>
            <NInput v-model:value="text" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="试听文字" />
            <p class="rv-hint">按当前选择与参数合成新音频；已有结果可重放。</p>
            <NButton :loading="busy" :disabled="!speech.canSynthesize" @click="audition">确认试听</NButton>
            <div v-if="job" class="rv-job">
              <span>试听任务：{{ job.status }} {{ job.errorCode }}</span>
              <div class="rv-job__actions">
                <NButton size="small" @click="query">查询试听状态</NButton>
                <NButton v-if="job.audioResourceId" size="small" @click="speechPlayer.play('resources', job.audioResourceId)">重放 / 停止</NButton>
              </div>
            </div>
          </section>
        </aside>
      </div>
      <p v-else class="rv-empty">{{ busy ? '正在读取角色音色…' : '' }}</p>
      <footer class="rv-foot">
        <span class="rv-foot__hint">选择“跟随平台默认音色”并保存即可清除绑定。</span>
        <div class="rv-foot__actions">
          <NButton @click="visible = false">取消</NButton>
          <NButton type="primary" :loading="busy" :disabled="!role" @click="save">保存绑定</NButton>
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
.rv-job { display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
.rv-job__actions { display: flex; flex-wrap: wrap; gap: 6px; }
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
