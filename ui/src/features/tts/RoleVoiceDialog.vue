<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NAlert, NButton, NInput, NInputNumber, NModal, NSelect, NSpace, NPagination } from 'naive-ui'
import { speechAPI, speechError } from './api'
import { useSpeechStore } from './store'
import type { RoleSpeechConfig, SpeechJob } from './types'
import { speechPlayer } from './player'
import { useUserStore } from '@/stores/user'
const props = defineProps<{ identityId: string }>()
const speech = useSpeechStore()
const user = useUserStore()
let generation = 0
const visible = ref(false)
const error = ref('')
const busy = ref(false)
const text = ref('你好，这是我的角色语音试听。')
const job = ref<SpeechJob | null>(null)
const role = ref<RoleSpeechConfig | null>(null)
watch([() => user.info.id, () => props.identityId], () => { generation++; visible.value = false; role.value = null; job.value = null; busy.value = false }, { flush: 'sync' })
watch(visible, () => { generation++; busy.value = false }, { flush: 'sync' })
onBeforeUnmount(() => { generation++ })
const options = ref<{ label: string; value: string }[]>([])
const search = ref('')
const page = ref(1)
const total = ref(0)
const unavailable = computed(() => !!voice.value && !options.value.some(option => option.value === voice.value))
const voice = computed({
  get: () => role.value?.voiceId ? `voice:${role.value.voiceId}` : role.value?.systemVoice ? `system:${role.value.systemVoice}` : '',
  set: (value: string) => {
    if (!role.value) return
    role.value.voiceId = value.startsWith('voice:') ? value.slice(6) : ''
    role.value.systemVoice = value.startsWith('system:') ? value.slice(7) : ''
  },
})
async function open() {
  if (visible.value) return
  visible.value = true
  busy.value = true
  error.value = ''
  role.value = null
  job.value = null
  const identityId = props.identityId
  const current = generation
  try {
    const [value, directory] = await Promise.all([speechAPI.role(identityId), speechAPI.voices({ page: 1 })])
    if (current !== generation || !visible.value || identityId !== props.identityId) return
    role.value = value
    page.value = 1
    search.value = ''
    total.value = directory.total
    options.value = [{ label: '继承平台默认音色', value: '' },
      ...directory.system.filter(v => v.targetModel === speech.quota?.defaultModel).map(v => ({ label: v.name, value: `system:${v.id}` })),
      ...directory.items.filter(v => v.lifecycle === 'saved' && v.providerStatus === 'OK').map(v => ({ label: v.name, value: `voice:${v.id}` })),
    ]
    if (value.voiceId && !options.value.some(option => option.value === `voice:${value.voiceId}`)) {
      try {
        const selected = await speechAPI.voice(value.voiceId)
        if (current === generation && visible.value && identityId === props.identityId) options.value.push({ label: selected.name, value: `voice:${selected.id}` })
      } catch { /* Keep an explicit unavailable binding; never choose a fallback. */ }
    }
  } catch (e) { if (current === generation) error.value = speechError(e) }
  finally { if (current === generation) busy.value = false }
}
async function browse(nextPage = 1) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  const current = generation
  const selected = options.value.find(option => option.value === voice.value)
  try {
    const directory = await speechAPI.voices({ page: nextPage, search: search.value })
    if (current !== generation) return
    const values = [
      { label: '继承平台默认音色', value: '' },
      ...directory.system.filter(v => v.targetModel === speech.quota?.defaultModel).map(v => ({ label: v.name, value: `system:${v.id}` })),
      ...directory.items.filter(v => v.lifecycle === 'saved' && v.providerStatus === 'OK').map(v => ({ label: v.name, value: `voice:${v.id}` })),
    ]
    if (selected && !values.some(option => option.value === selected.value)) values.push(selected)
    options.value = values
    page.value = nextPage
    total.value = directory.total
  } catch (e) { if (current === generation) error.value = speechError(e) }
  finally { if (current === generation) busy.value = false }
}
async function save() {
  if (!role.value || busy.value) return
  busy.value = true
  error.value = ''
  const current = generation
  try {
    await speechAPI.saveRole(props.identityId, { ...role.value })
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
    const value = await speechAPI.submit('audition', { ...role.value, text: text.value, requestKey: crypto.randomUUID() })
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
  <NModal v-model:show="visible" preset="card" title="角色音色" style="width: min(520px, 94vw)">
    <NSpace vertical>
      <NAlert v-if="error" type="error">{{ error }}</NAlert>
      <template v-if="role">
        <NSpace>
          <NInput v-model:value="search" placeholder="搜索个人或公开音色名称、标签" @keyup.enter="browse(1)" />
          <NButton :loading="busy" @click="browse(1)">搜索音色</NButton>
        </NSpace>
        <NSelect v-model:value="voice" :options="options" filterable />
        <NPagination :page="page" :item-count="total" :page-size="20" :disabled="busy" @update:page="browse" />
        <span>{{ voice ? '当前使用显式音色绑定' : '当前继承平台默认音色' }}；选择“继承平台默认音色”并保存可清除绑定。</span>
        <NAlert v-if="unavailable" type="warning">当前绑定已不可用或不再公开；不会自动改用其他声音。请重新选择或清除绑定。</NAlert>
        <NInput v-model:value="role.instruction" placeholder="朗读指令（不调用文本模型）" />
        <label>语速<NInputNumber v-model:value="role.rate" :min="0.5" :max="2" :step="0.1" /></label>
        <label>音调<NInputNumber v-model:value="role.pitch" :min="0.5" :max="2" :step="0.1" /></label>
        <label>音量<NInputNumber v-model:value="role.volume" :min="0" :max="100" /></label>
        <NButton :loading="busy" @click="save">保存绑定</NButton>
        <NInput v-model:value="text" placeholder="试听文字" />
        <NButton :loading="busy" :disabled="!speech.quota?.enabled || speech.quota.characterPrice == null" @click="audition">确认付费试听（{{ speech.quota?.characterPrice ?? '未定价' }} / 字）</NButton>
        <template v-if="job">
          <span>{{ job.status }} {{ job.errorCode }}</span>
          <NButton @click="query">查询试听状态</NButton>
          <NButton v-if="job.audioResourceId" @click="speechPlayer.play('resources', job.audioResourceId)">免费重放 / 停止</NButton>
        </template>
      </template>
    </NSpace>
  </NModal>
</template>
