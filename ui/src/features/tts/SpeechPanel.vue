<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NAlert, NButton, NInput, NModal, NSpace, NSwitch, NTabPane, NTabs, NSelect, NPagination } from 'naive-ui'
import { speechAPI, speechError } from './api'
import { useSpeechStore } from './store'
import { speechPlayer } from './player'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import type { SpeechJob, SpeechVoice, VoiceDirectory } from './types'
import SpeechQuotaSummary from './SpeechQuotaSummary.vue'

const speech = useSpeechStore()
const chat = useChatStore()
const user = useUserStore()
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
const tab = ref('browse')
const error = ref('')
const busy = ref(false)
const text = ref('你好，欢迎来到我们的冒险故事。')
const search = ref('')
const language = ref('')
const page = ref(1)
const directory = ref<VoiceDirectory>({ items: [], system: [], total: 0, catalogVersion: '' })
const selected = ref('')
const name = ref('')
const description = ref('')
const operation = ref<'design' | 'clone'>('design')
const source = ref<File | null>(null)
let uploadedSource: { file: File; id: string } | null = null
const authorized = ref(false)
const replaceId = ref('')
const job = ref<SpeechJob | null>(null)
let alive = true
let serial = 0
let timer: ReturnType<typeof setTimeout> | undefined
const options = computed(() => [
  ...directory.value.system.filter(v => v.targetModel === speech.quota?.defaultModel && (!language.value || v.languages.includes(language.value)) && (!search.value || `${v.name} ${v.tags || ''}`.includes(search.value))).map(v => ({ label: `${v.name} · ${v.kind === 'basic' ? '基础' : '系统'} · ${v.tags || ''}`, value: `system:${v.id}` })),
  ...directory.value.items.filter(v => v.lifecycle === 'saved' && v.providerStatus === 'OK').map(v => ({ label: v.name, value: `voice:${v.id}` })),
])
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try { await action() } catch (e) { if (alive) error.value = speechError(e) }
  finally { if (alive) busy.value = false }
}
async function load() {
  const current = ++serial
  const value = await speechAPI.voices({ mine: tab.value !== 'browse', search: search.value, page: page.value })
  if (alive && current === serial) directory.value = value
}
async function poll(id: string) {
  try {
    const value = await speechAPI.job(id)
    if (!alive || job.value?.id !== id) return
    job.value = value
    if (['queued', 'running', 'storage_pending'].includes(value.status)) {
      timer = setTimeout(() => void poll(id), 1500)
    } else { await load(); await speech.refresh() }
  } catch (e) { if (alive) error.value = speechError(e) }
}
async function submit(kind: 'audition' | 'design' | 'clone') {
  const userId = user.info.id
  const [type, voice] = selected.value.split(':')
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
    ...(type === 'voice' ? { voiceId: voice } : { systemVoice: voice || speech.quota?.defaultVoice }),
  })
  if (!alive || userId !== user.info.id) return
  job.value = value
  void poll(value.id)
}
async function save(voice: SpeechVoice) { await speechAPI.save(voice.id, replaceId.value); await load(); await speech.refresh() }
async function remove(voice: SpeechVoice) { await speechAPI.remove(voice.id); await load(); await speech.refresh() }
onMounted(() => void run(async () => { await speech.refresh(); await load() }))
onBeforeUnmount(() => { alive = false; serial++; clearTimeout(timer) })
</script>

<template>
  <NModal v-model:show="speech.visible" preset="card" title="语音朗读" style="width: min(860px, 94vw)" :bordered="false">
    <NSpace vertical size="large">
      <NAlert v-if="error || speechPlayer.state.error" type="error">{{ error || speechPlayer.state.error }}</NAlert>
      <NAlert v-if="speech.quota && !speech.quota.enabled" type="warning">平台未启用新的语音收费操作；已有语音仍可播放。</NAlert>
      <NSpace align="center">
        <span>自动合成</span>
        <NSwitch :value="speech.quota?.autoSynthesis ?? false" :loading="busy" @update:value="value => run(async () => { await speechAPI.settings(value); await speech.refresh() })" />
        <span>已保存音色 {{ speech.quota?.saved ?? 0 }} / {{ speech.quota?.slots ?? 0 }}</span>
        <span>当前浏览器自动播放</span>
        <NSwitch :value="speechPlayer.state.preferred" @update:value="value => run(() => speechPlayer.setAutomatic(value))" />
        <span v-if="speechPlayer.state.preferred && !speechPlayer.state.automatic" class="autoplay-pending">等待页面交互后自动启用</span>
      </NSpace>
      <SpeechQuotaSummary v-if="speech.quota" :quota="speech.quota" />
      <NAlert type="info">PCM16 WAV 支持增量播放（约 300ms 预缓冲）；MP3 使用完整文件兼容模式。当前生产格式仅支持 WAV / MP3。自动播放不触发收费合成；手动重放或试听会暂停本地自动播放。</NAlert>
      <NSpace v-if="currentChannel">
        <NButton @click="run(loadQueue)">查询频道朗读队列</NButton>
        <span v-if="queue">可见待处理任务 {{ queue.items.length }}</span>
        <template v-if="queue?.canControl">
          <NButton @click="run(async () => { await speechAPI.control(currentChannel!, 'skip'); await loadQueue() })">GM 跳过当前</NButton>
          <NButton @click="run(async () => { await speechAPI.control(currentChannel!, 'clear'); await loadQueue() })">GM 清空队列</NButton>
        </template>
      </NSpace>
      <NTabs v-model:value="tab" type="line" @update:value="() => { page = 1; run(load) }">
        <NTabPane name="browse" tab="音色试听与浏览">
          <NSpace vertical>
            <NInput v-model:value="search" placeholder="搜索音色名称或标签" @keyup.enter="run(load)" />
            <NButton :loading="busy" @click="run(load)">搜索</NButton>
            <NSelect v-model:value="language" :options="[{ label: '全部语言', value: '' }, { label: '中文', value: 'zh' }, { label: '英文', value: 'en' }]" />
            <NSelect v-model:value="selected" :options="options" placeholder="选择系统、公开或自己的音色" filterable />
            <NInput v-model:value="text" type="textarea" placeholder="输入新的试听文字（最多 500 字）" />
            <NAlert type="info">新文本试听会扣发起人的语音额度，单价 {{ speech.quota?.characterPrice ?? '尚未确认' }} / 字；重放已有文件不收费。</NAlert>
            <NButton :disabled="!speech.quota?.enabled || speech.quota.characterPrice == null" :loading="busy" @click="run(() => submit('audition'))">确认付费合成试听</NButton>
          </NSpace>
        </NTabPane>
        <NTabPane name="workshop" tab="音色工作台">
          <NSpace vertical>
            <NSelect v-model:value="operation" :options="[{ label: '声音设计', value: 'design' }, { label: '样本复刻', value: 'clone' }]" />
            <NInput v-model:value="name" placeholder="音色名称" />
            <NInput v-model:value="description" type="textarea" placeholder="描述希望设计的声音" />
            <NInput v-model:value="text" type="textarea" placeholder="15–200 字预览文本" />
            <NButton v-if="operation === 'design'" :disabled="!speech.quota?.enabled || speech.quota.designPrice == null" :loading="busy" @click="run(() => submit('design'))">确认付费设计（{{ speech.quota?.designPrice ?? '未定价' }} / 次）</NButton>
            <template v-else>
              <input type="file" accept="audio/wav,audio/mpeg" @change="event => { source = (event.target as HTMLInputElement).files?.[0] ?? null }" />
              <label><input v-model="authorized" type="checkbox" />我确认拥有此样本的复刻授权（10–60 秒 WAV/MP3）</label>
              <NButton :disabled="!authorized || !source || !speech.quota?.enabled || speech.quota.clonePrice == null" :loading="busy" @click="run(() => submit('clone'))">确认付费复刻（{{ speech.quota?.clonePrice ?? '未定价' }} / 次）</NButton>
            </template>
          </NSpace>
        </NTabPane>
        <NTabPane name="personal" tab="个人音色管理">
          <NAlert type="info">公开自己的音色仍占槽位。删除音色不影响已经生成的消息文件。</NAlert>
        </NTabPane>
      </NTabs>
      <NSpace v-if="job" align="center">
        <span>任务：{{ job.status }} {{ job.errorCode }}</span>
        <NButton v-if="job.audioResourceId" @click="speechPlayer.play('resources', job.audioResourceId)">播放 / 停止已有试听</NButton>
        <NButton @click="run(() => poll(job!.id))">查询状态（免费）</NButton>
      </NSpace>
      <NSpace v-for="voice in directory.items" :key="voice.id" vertical class="voice-row">
        <strong>{{ voice.name }}</strong>
        <span>{{ voice.targetModel }} · {{ voice.lifecycle }} · {{ voice.providerStatus }} · {{ voice.tags }}</span>
        <span v-if="voice.previewExpiresAt">预览到期：{{ voice.previewExpiresAt }}</span>
        <template v-if="tab === 'personal' && voice.lifecycle === 'saved'">
          <NInput v-model:value="voice.name" placeholder="音色名称" />
          <NInput v-model:value="voice.tags" placeholder="标签" />
          <NInput v-model:value="voice.description" placeholder="描述" />
          <NInput v-model:value="voice.parameters" type="textarea" placeholder='默认参数 JSON，例如 {"rate":1,"pitch":1,"volume":50}' />
          <label>公开音色 <NSwitch v-model:value="voice.isPublic" /></label>
          <NButton :loading="busy" @click="run(async () => { if (voice.parameters) JSON.parse(voice.parameters); await speechAPI.update(voice); await load() })">保存本地名称、标签、公开状态与参数</NButton>
        </template>
        <template v-if="tab !== 'browse' && voice.lifecycle === 'preview'">
          <NSelect v-model:value="replaceId" :options="[{ label: '保存为新音色', value: '' }, ...directory.items.filter(v => v.lifecycle === 'saved').map(v => ({ label: `替换 ${v.name}（不额外占永久槽位）`, value: v.id }))]" />
          <small v-if="replaceId">替换将保留正在执行任务的旧版本；原角色绑定失效后需重新选择新版本，不自动换声。</small>
        </template>
        <NSpace>
          <NButton v-if="voice.previewResourceId" @click="speechPlayer.play('resources', voice.previewResourceId)">免费重放 / 停止</NButton>
          <template v-if="tab !== 'browse'">
            <NButton v-if="voice.lifecycle === 'preview'" :loading="busy" @click="run(() => save(voice))">保存到个人音色</NButton>
            <NButton :loading="busy" @click="run(() => remove(voice))">删除 / 丢弃</NButton>
          </template>
        </NSpace>
      </NSpace>
      <NPagination v-model:page="page" :item-count="directory.total" :page-size="20" @update:page="run(load)" />
    </NSpace>
  </NModal>
</template>

<style scoped>
.voice-row { border-top: 1px solid var(--n-border-color); padding-top: 12px; }
.autoplay-pending { font-size: 12px; opacity: .6; }
</style>
