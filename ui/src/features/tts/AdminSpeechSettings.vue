<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { NAlert, NButton, NCollapse, NCollapseItem, NForm, NFormItem, NGi, NGrid, NInput, NInputNumber, NSelect, NSpace, NSwitch, NTag, NText } from 'naive-ui'
import { speechAPI, speechError } from './api'
import type { ResolvedSpeechProvider, SpeechConfig, SpeechJob, SpeechProvider, SystemVoice } from './types'
import { speechPlayer } from './player'
import AdminSpeechUsage from './AdminSpeechUsage.vue'
const newProviderTarget = '__new_provider__'
const config = ref<SpeechConfig>({ enabled: false, providers: [], defaultProvider: '', defaultVoice: 'longanhuan_v3.6', format: 'wav', quotaDefault: { dailyLimit: 0, monthlyLimit: 0, lifetimeLimit: 0 }, defaultSlots: 0, previewTTLMinutes: 30, previewLimit: 2, requestTimeoutSeconds: 90, maxConcurrent: 2, channelQueueLimit: 8 })
const error = ref('')
const notice = ref('')
const busy = ref(false)
const testJob = ref<SpeechJob | null>(null)
const testError = ref('')
const testNotice = ref('')
const testBusy = ref(false)
const quickBaseUrl = ref('')
const quickApiKey = ref('')
const quickProviderId = ref(newProviderTarget)
const resolvedProvider = ref<ResolvedSpeechProvider | null>(null)
const resolvedModelId = ref('')
const systemVoices = ref<SystemVoice[]>([])
const localProviderIds = new Set<string>()
let alive = true
let testGeneration = 0
let pollTimer: number | undefined
let finishPollDelay: (() => void) | undefined
const quickProviderOptions = computed(() => [
  { label: '新建 Provider', value: newProviderTarget },
  ...config.value.providers.map(provider => ({ label: provider.id, value: provider.id })),
])
const resolvedModelOptions = computed(() => (resolvedProvider.value?.models ?? []).map(model => ({
  label: model.name,
  value: model.id,
})))
const quickProvider = computed(() => config.value.providers.find(provider => provider.id === quickProviderId.value))
async function run(fn: () => Promise<void>) {
  busy.value = true; error.value = ''; notice.value = ''
  try { await fn() } catch (e) { error.value = speechError(e) } finally { busy.value = false }
}
function blankProvider(id: string, enabled = false): SpeechProvider {
  return { id, enabled, credentialScope: '', region: 'cn-beijing', workspace: '', apiKey: '', synthesisEndpoint: '', voiceEndpoint: '', model: 'qwen-audio-3.0-tts-flash', characterPrice: null, designPrice: null, clonePrice: null, accountVoiceLimit: null, revision: 1 }
}
function add() {
  const id = crypto.randomUUID()
  config.value.providers.push(blankProvider(id))
  localProviderIds.add(id)
}
function removeProvider(index: number) {
  const provider = config.value.providers[index]
  if (!provider || !localProviderIds.has(provider.id)) return

  config.value.providers.splice(index, 1)
  localProviderIds.delete(provider.id)

  if (config.value.defaultProvider === provider.id) {
    config.value.defaultProvider = ''
  }
}
async function save() {
  const saved = await speechAPI.saveAdminConfig(config.value)
  config.value = saved
  quickApiKey.value = ''
  localProviderIds.clear()
  notice.value = '语音配置已保存。'
}
function baseUrlFor(provider?: SpeechProvider) {
  return provider?.workspace ? `https://${provider.workspace}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` : ''
}
function selectQuickProvider(providerId: string) {
  const provider = config.value.providers.find(item => item.id === providerId)
  quickProviderId.value = providerId
  quickBaseUrl.value = providerId === newProviderTarget ? '' : baseUrlFor(provider)
  quickApiKey.value = provider && !provider.hasApiKey ? provider.apiKey : ''
  resolvedProvider.value = null
  resolvedModelId.value = ''
}
function ensureDefaultVoice(model: string) {
  if (systemVoices.value.some(voice => voice.id === config.value.defaultVoice && voice.targetModel === model)) return
  const fallback = systemVoices.value.find(voice => voice.targetModel === model)
  if (fallback) config.value.defaultVoice = fallback.id
}
function applyResolvedModel(modelId: string) {
  resolvedModelId.value = modelId
  const resolved = resolvedProvider.value
  const model = resolved?.models.find(item => item.id === modelId)
  if (!resolved || !model) return
  let provider = config.value.providers.find(item => item.id === resolved.providerId)
  const firstProvider = config.value.providers.length === 0
  if (!provider) {
    provider = blankProvider(resolved.providerId, firstProvider)
    config.value.providers.push(provider)
    localProviderIds.add(provider.id)
  }
  Object.assign(provider, {
    credentialScope: resolved.credentialScope,
    region: resolved.region,
    workspace: resolved.workspace,
    apiKey: quickApiKey.value.trim(),
    synthesisEndpoint: resolved.synthesisEndpoint,
    voiceEndpoint: resolved.voiceEndpoint,
    model: model.id,
    characterPrice: model.characterPrice,
    designPrice: model.designPrice,
    clonePrice: model.clonePrice,
  })
  if (firstProvider) {
    provider.enabled = true
    config.value.defaultProvider = provider.id
  }
  if (config.value.defaultProvider === provider.id) ensureDefaultVoice(model.id)
  quickProviderId.value = provider.id
}
async function resolveProvider() {
  const providerId = quickProviderId.value === newProviderTarget ? '' : quickProviderId.value
  const resolved = await speechAPI.resolveProvider(quickBaseUrl.value, quickApiKey.value, providerId)
  resolvedProvider.value = resolved
  const currentModel = config.value.providers.find(provider => provider.id === resolved.providerId)?.model
  const selected = resolved.models.find(model => model.id === currentModel) ?? resolved.models[0]
  if (selected) applyResolvedModel(selected.id)
  notice.value = 'Base URL 与 API Key 已验证，模型信息已从百炼模型目录导入；保存配置后生效。'
}
const terminalTestStates = new Set(['succeeded', 'failed', 'usage_unknown', 'cancelled'])
const waitOneSecond = () => new Promise<void>((resolve) => {
  finishPollDelay = resolve
  pollTimer = window.setTimeout(() => {
    pollTimer = undefined
    finishPollDelay = undefined
    resolve()
  }, 1000)
})
async function testProvider() {
  const generation = ++testGeneration
  const job = await speechAPI.submit('audition', { requestKey: crypto.randomUUID(), text: '这是一次语音测试。' })
  testJob.value = job
  testNotice.value = `已提交合成测试 ${job.id}，正在查询结果。`
  for (let attempt = 0; attempt < 30 && !terminalTestStates.has(testJob.value.status); attempt++) {
    await waitOneSecond()
    if (!alive || generation !== testGeneration) return
    testJob.value = await speechAPI.job(job.id)
  }
  showTestResult(testJob.value)
}
function showTestResult(job: SpeechJob) {
  testError.value = ''
  switch (job.status) {
    case 'succeeded':
      testNotice.value = '合成测试成功。'
      break
    case 'failed':
      testNotice.value = ''
      testError.value = `合成测试失败：${job.message || job.errorCode || '供应商未返回错误信息'}`
      break
    case 'usage_unknown':
      testNotice.value = '供应商用量状态待核对，请勿重新提交测试。'
      break
    case 'cancelled':
      testNotice.value = '合成测试已取消。'
      break
    default:
      testNotice.value = '测试任务仍在处理中，可稍后查询当前任务。'
  }
}
async function runTest(fn: () => Promise<void>) {
  if (testBusy.value) return
  testBusy.value = true
  testError.value = ''
  testNotice.value = ''
  try {
    await fn()
  } catch (e) {
    testError.value = speechError(e)
  } finally {
    testBusy.value = false
  }
}
function runTestProvider() {
  return runTest(testProvider)
}
function refreshTestJob() {
  return runTest(async () => {
    const id = testJob.value?.id
    if (!id) return
    const job = await speechAPI.job(id)
    if (!alive) return
    testJob.value = job
    showTestResult(job)
  })
}
onMounted(() => void run(async () => {
  const [value, directory] = await Promise.all([speechAPI.adminConfig(), speechAPI.voices({ page: 1 })])
  if (!alive) return
  if (value) config.value = value
  systemVoices.value = directory.system
  selectQuickProvider(config.value.defaultProvider || config.value.providers[0]?.id || newProviderTarget)
}))
onBeforeUnmount(() => {
  alive = false
  testGeneration++
  if (pollTimer !== undefined) window.clearTimeout(pollTimer)
  pollTimer = undefined
  finishPollDelay?.()
  finishPollDelay = undefined
})
</script>
<template>
  <div class="admin-settings-scroll speech-settings">
    <div class="speech-settings__header">
      <div class="speech-settings__heading">
        <div>
          <h3>AI 语音服务</h3>
          <p>配置平台语音合成、用户用量限制、音色槽位与服务商参数。</p>
        </div>
        <div class="speech-settings__switch">
          <NText>启用语音</NText>
          <NSwitch v-model:value="config.enabled" />
        </div>
      </div>
      <NButton
        type="primary"
        :loading="busy"
        @click="run(save)"
      >
        保存配置
      </NButton>
    </div>

    <NSpace vertical :size="12">
      <NAlert v-if="error" type="error">{{ error }}</NAlert>
      <NAlert v-if="notice" type="info">{{ notice }}</NAlert>
      <NAlert type="info">
        语音与文本数据相互隔离；新的语音操作仍受平台 AI 总开关约束。
      </NAlert>
      <div class="speech-settings__docs">
        <NText depth="3">相关文档</NText>
        <NSpace :size="12">
          <a href="https://help.aliyun.com/zh/model-studio/get-api-key" target="_blank" rel="noopener noreferrer">获取 API Key</a>
          <a href="https://help.aliyun.com/zh/model-studio/obtain-the-app-id-and-workspace-id#732535cfc959h" target="_blank" rel="noopener noreferrer">业务空间/地域说明</a>
          <a href="https://help.aliyun.com/zh/model-studio/cosyvoice-tts-http-api" target="_blank" rel="noopener noreferrer">合成接口文档</a>
          <a href="https://help.aliyun.com/zh/model-studio/voice-design-api-references" target="_blank" rel="noopener noreferrer">音色接口文档</a>
        </NSpace>
      </div>

      <NForm label-placement="left" label-width="120">
        <NCollapse class="settings-collapse" :default-expanded-names="['basic', 'quota', 'providers']">
          <NCollapseItem title="基础配置" name="basic">
            <NFormItem label="默认 Provider">
              <NSelect
                v-model:value="config.defaultProvider"
                :options="config.providers.map(p => ({
                  label: p.enabled ? p.id : `${p.id}（已停用）`,
                  value: p.id,
                  disabled: !p.enabled,
                }))"
              />
            </NFormItem>
            <NFormItem label="默认系统音色 ID">
              <NInput v-model:value="config.defaultVoice" />
            </NFormItem>
            <NFormItem label="合成格式">
              <NSelect v-model:value="config.format" :options="[{ label: 'WAV（PCM16 增量播放）', value: 'wav' }, { label: 'MP3（兼容文件播放）', value: 'mp3' }]" />
              <template #feedback>
                当前生产格式仅支持 WAV / MP3；不会自动换格式重试调用。私有 TTS 文件存放于本地受保护目录。
              </template>
            </NFormItem>
          </NCollapseItem>

          <NCollapseItem title="用户限制与预览" name="quota">
            <NGrid cols="1 s:2 l:3" :x-gap="16" responsive="screen">
              <NGi>
                <NFormItem label="默认日限制"><NInputNumber v-model:value="config.quotaDefault.dailyLimit" :min="0" /></NFormItem>
              </NGi>
              <NGi>
                <NFormItem label="默认月限制"><NInputNumber v-model:value="config.quotaDefault.monthlyLimit" :min="0" /></NFormItem>
              </NGi>
              <NGi>
                <NFormItem label="默认累计限制"><NInputNumber v-model:value="config.quotaDefault.lifetimeLimit" :min="0" /></NFormItem>
              </NGi>
              <NGi>
                <NFormItem label="个人保存槽位"><NInputNumber v-model:value="config.defaultSlots" :min="0" /></NFormItem>
              </NGi>
              <NGi>
                <NFormItem label="同时预览上限"><NInputNumber v-model:value="config.previewLimit" :min="1" :max="10" /></NFormItem>
              </NGi>
              <NGi>
                <NFormItem label="预览有效分钟"><NInputNumber v-model:value="config.previewTTLMinutes" :min="1" :max="1440" /></NFormItem>
              </NGi>
            </NGrid>
          </NCollapseItem>

          <NCollapseItem name="providers">
            <template #header>
              <div class="speech-settings__collapse-header">
                <span>Provider 配置</span>
              </div>
            </template>
            <NCollapse class="provider-mode-collapse" :default-expanded-names="['quick']">
              <NCollapseItem title="快速接入" name="quick">
                <div class="speech-provider-card speech-provider-card--quick">
                  <NFormItem label="导入到">
                    <NSelect :value="quickProviderId" :options="quickProviderOptions" @update:value="selectQuickProvider" />
                  </NFormItem>
                  <NFormItem label="百炼 Base URL">
                    <NInput v-model:value="quickBaseUrl" placeholder="https://llm-xxx.cn-beijing.maas.aliyuncs.com/compatible-mode/v1" />
                  </NFormItem>
                  <NFormItem label="API Key">
                    <NInput v-model:value="quickApiKey" type="password" :placeholder="quickProvider?.hasApiKey ? '已配置，留空使用已保存密钥' : 'sk-xxx'" />
                  </NFormItem>
                  <NButton type="primary" secondary :loading="busy" :disabled="!quickBaseUrl || (!quickApiKey && !quickProvider?.hasApiKey)" @click="run(resolveProvider)">
                    解析并导入
                  </NButton>

                  <div v-if="resolvedProvider" class="speech-provider-summary">
                    <NGrid cols="1 s:2 l:3" :x-gap="16" :y-gap="8" responsive="screen">
                      <NGi><NText depth="3">业务空间：</NText>{{ resolvedProvider.workspace }}</NGi>
                      <NGi><NText depth="3">地域：</NText>华北2（北京）</NGi>
                      <NGi><NText depth="3">连接：</NText><NTag type="success" size="small">已验证</NTag></NGi>
                      <NGi><NText depth="3">可用模型：</NText>{{ resolvedProvider.models.length }}</NGi>
                      <NGi><NText depth="3">信息来源：</NText>百炼模型目录</NGi>
                    </NGrid>
                    <NFormItem label="模型" class="speech-provider-summary__model">
                      <NSelect :value="resolvedModelId" :options="resolvedModelOptions" @update:value="applyResolvedModel" />
                    </NFormItem>
                    <NAlert v-if="resolvedProvider.models.find(model => model.id === resolvedModelId)?.designPrice == null" type="warning">
                      供应商未返回可确认的声音设计信息，声音设计暂不可提交；普通 TTS 合成不受影响。
                    </NAlert>
                  </div>
                </div>
              </NCollapseItem>

              <NCollapseItem title="高级配置" name="advanced">
                <div class="speech-settings__advanced-actions">
                  <NButton size="small" tertiary @click="add">手工添加 Provider</NButton>
                </div>
                <div
                  v-for="(provider, index) in config.providers"
                  :key="provider.id"
                  class="speech-provider-card"
                >
                  <div class="speech-provider-card__header">
                    <div class="speech-provider-card__identity">
                      <strong>{{ provider.id || '未命名 Provider' }}</strong>
                      <NTag :type="provider.enabled ? 'success' : 'default'" size="small">{{ provider.enabled ? '已启用' : '已停用' }}</NTag>
                    </div>
                    <div class="speech-provider-card__switch">
                      <NText depth="3">启用 Provider</NText>
                      <NSwitch v-model:value="provider.enabled" />
                    </div>
                  </div>

                  <NGrid cols="1 l:2" :x-gap="24" responsive="screen">
                    <NGi>
                      <NFormItem label="Provider ID"><NInput :value="provider.id" readonly /></NFormItem>
                      <NFormItem label="credentialScope"><NInput v-model:value="provider.credentialScope" /></NFormItem>
                      <NFormItem label="Workspace"><NInput v-model:value="provider.workspace" /></NFormItem>
                      <NFormItem label="Region"><NInput v-model:value="provider.region" /></NFormItem>
                      <NFormItem label="API Key"><NInput v-model:value="provider.apiKey" type="password" :placeholder="provider.hasApiKey ? '已配置，留空保留' : '尚未配置'" /></NFormItem>
                      <NFormItem label="Synthesis Endpoint"><NInput v-model:value="provider.synthesisEndpoint" /></NFormItem>
                      <NFormItem label="Voice Endpoint"><NInput v-model:value="provider.voiceEndpoint" /></NFormItem>
                    </NGi>
                    <NGi>
                      <NFormItem label="模型"><NSelect v-model:value="provider.model" :options="['qwen-audio-3.0-tts-flash', 'qwen-audio-3.0-tts-plus'].map(value => ({ label: value, value }))" /></NFormItem>
                      <NFormItem label="字符单位值"><NInputNumber v-model:value="provider.characterPrice" :min="0" placeholder="单字符配置值" /></NFormItem>
                      <NFormItem label="设计单位值"><NInputNumber v-model:value="provider.designPrice" :min="0" /></NFormItem>
                      <NFormItem label="复刻单位值"><NInputNumber v-model:value="provider.clonePrice" :min="0" /></NFormItem>
                      <NFormItem label="账号音色上限"><NInputNumber v-model:value="provider.accountVoiceLimit" :min="0" /></NFormItem>
                      <NFormItem label="Revision"><NInputNumber v-model:value="provider.revision" :min="1" /></NFormItem>
                    </NGi>
                  </NGrid>
                  <div v-if="localProviderIds.has(provider.id)" class="speech-provider-card__actions">
                    <NButton size="small" tertiary type="error" @click="removeProvider(index)">删除 Provider</NButton>
                  </div>
                </div>
              </NCollapseItem>
            </NCollapse>
          </NCollapseItem>

          <NCollapseItem title="运行测试与用量" name="testing">
            <div class="speech-settings__test">
              <NSpace align="center">
                <NButton :loading="testBusy" :disabled="!config.enabled" @click="runTestProvider">
                  合成测试
                </NButton>
                <NText depth="3">只提交一次真实合成；后续每秒查询当前任务，最多约 30 秒。</NText>
              </NSpace>
              <NAlert v-if="testError" type="error" class="speech-settings__test-alert">{{ testError }}</NAlert>
              <NAlert v-if="testNotice" type="info" class="speech-settings__test-alert">{{ testNotice }}</NAlert>

              <div v-if="testJob" class="speech-settings__test-result">
                <div>
                  <NText>状态：{{ testJob.status }}<span v-if="testJob.message"> · {{ testJob.message }}</span></NText>
                  <div class="speech-settings__test-detail">
                    <span v-if="testJob.model">模型：{{ testJob.model }}</span>
                    <span v-if="testJob.media">媒体：{{ testJob.media.container }} / {{ testJob.media.codec }} / {{ testJob.media.sampleRate }} Hz</span>
                    <span v-if="testJob.actualUnits != null">实际字符数：{{ testJob.actualUnits }}</span>
                    <span v-if="testJob.errorCode">错误码：{{ testJob.errorCode }}</span>
                  </div>
                </div>
                <NSpace>
                  <NButton size="small" :disabled="testBusy" @click="refreshTestJob">查询测试结果</NButton>
                  <NButton v-if="testJob.audioResourceId" size="small" @click="speechPlayer.play('resources', testJob.audioResourceId)">播放测试音频</NButton>
                </NSpace>
              </div>
            </div>
            <AdminSpeechUsage />
          </NCollapseItem>
        </NCollapse>
      </NForm>
    </NSpace>
  </div>
</template>

<style scoped>
.admin-settings-scroll {
  max-height: 61vh;
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-gutter: stable;
}

.speech-settings {
  padding-right: 8px;
}

.speech-settings__header,
.speech-settings__heading,
.speech-settings__switch,
.speech-settings__docs,
.speech-settings__collapse-header,
.speech-provider-card__header,
.speech-provider-card__identity,
.speech-provider-card__switch,
.speech-settings__test-result {
  display: flex;
  align-items: center;
}

.speech-settings__header,
.speech-provider-card__header,
.speech-settings__test-result {
  justify-content: space-between;
}

.speech-settings__header {
  gap: 16px;
  margin-bottom: 12px;
}

.speech-settings__heading {
  min-width: 0;
  gap: 20px;
}

.speech-settings__heading h3,
.speech-provider-card__section h4 {
  margin: 0;
}

.speech-settings__heading p {
  margin: 4px 0 0;
  color: var(--n-text-color-3);
}

.speech-settings__switch,
.speech-provider-card__switch,
.speech-provider-card__identity {
  gap: 8px;
  white-space: nowrap;
}

.speech-settings__docs {
  gap: 12px;
  flex-wrap: wrap;
  font-size: 13px;
}

.settings-collapse,
.provider-mode-collapse,
.speech-settings__collapse-header {
  width: 100%;
}

.speech-settings__collapse-header {
  justify-content: space-between;
  gap: 12px;
  padding-right: 8px;
}

.speech-provider-card {
  margin-bottom: 12px;
  padding: 14px 16px 4px;
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  background: var(--n-card-color);
}

.speech-provider-card--quick {
  padding-bottom: 16px;
}

.speech-provider-summary {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--n-border-color);
}

.speech-provider-summary__model {
  margin-top: 14px;
}

.speech-settings__advanced-actions {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 10px;
}

.speech-provider-card:last-child {
  margin-bottom: 0;
}

.speech-provider-card__header {
  gap: 12px;
  margin-bottom: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--n-border-color);
}

.speech-provider-card__section h4 {
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 600;
}

.speech-provider-card__actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 4px;
}

.speech-settings__test {
  margin-bottom: 16px;
}

.speech-settings__test-alert {
  margin-top: 12px;
}

.speech-settings__test-result {
  gap: 12px;
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
}

.speech-settings__test-detail {
  display: flex;
  gap: 6px 14px;
  flex-wrap: wrap;
  margin-top: 4px;
  color: var(--n-text-color-3);
  font-size: 13px;
}

@media (max-width: 768px) {
  .speech-settings__header,
  .speech-settings__heading,
  .speech-settings__test-result {
    align-items: flex-start;
    flex-direction: column;
  }

  .speech-settings__header > .n-button {
    align-self: stretch;
  }

  .speech-provider-card {
    padding-inline: 12px;
  }

  .speech-provider-card__header {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
