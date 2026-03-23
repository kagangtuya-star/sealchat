<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NCode,
  NInput,
  NInputNumber,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSpace,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { api, urlBase } from '@/stores/_config'

const WEBHOOK_TOKEN_STORAGE_KEY = 'sealchat_webhook_token_cache'
const WEBHOOK_INTEGRATION_STORAGE_KEY = 'sealchat_webhook_integration_cache'
const DEFAULT_CHANNEL_DIGEST_TEXT_TEMPLATE = '在 {{window_label}}，{{speaker_names}} 在 {{channel_name}} 频道发送了 {{message_count}} 条消息。'
const DEFAULT_CHANNEL_DIGEST_JSON_TEMPLATE = `{
  "scopeType": {{scope_type}},
  "scopeId": {{scope_id}},
  "window": {
    "start": {{window_start_ts}},
    "end": {{window_end_ts}},
    "label": {{window_label}},
    "seconds": {{window_seconds}}
  },
  "channel": {
    "id": {{channel_id}},
    "name": {{channel_name}}
  },
  "world": {
    "id": {{world_id}},
    "name": {{world_name}}
  },
  "messageCount": {{message_count}},
  "activeUserCount": {{active_user_count}},
  "speakerNames": {{speaker_names_array}},
  "speakerSummary": {{speaker_summary}},
  "speakers": {{speakers}},
  "text": {{rendered_text}}
}`
const DEFAULT_WORLD_DIGEST_TEXT_TEMPLATE = '在 {{window_label}}，{{scope_name}} 有 {{channel_count}} 个频道出现新消息：\n{{channel_digest_lines}}'
const DEFAULT_WORLD_DIGEST_JSON_TEMPLATE = `{
  "scopeType": {{scope_type}},
  "scopeId": {{scope_id}},
  "window": {
    "start": {{window_start_ts}},
    "end": {{window_end_ts}},
    "label": {{window_label}},
    "seconds": {{window_seconds}}
  },
  "world": {
    "id": {{world_id}},
    "name": {{world_name}}
  },
  "channelCount": {{channel_count}},
  "targetChannelIds": {{target_channel_ids}},
  "targetChannelNames": {{target_channel_names_array}},
  "channels": {{channels}},
  "messageCount": {{message_count}},
  "activeUserCount": {{active_user_count}},
  "speakerNames": {{speaker_names_array}},
  "speakerSummary": {{speaker_summary}},
  "speakers": {{speakers}},
  "text": {{rendered_text}}
}`

interface WebhookIntegrationItem {
  id: string
  channelId: string
  name: string
  source: string
  botUserId: string
  status: 'active' | 'revoked' | string
  createdAt: number
  createdBy: string
  lastUsedAt: number
  tokenTailFragment: string
  capabilities: string[]
}

interface DigestPushSettings {
  enabled: boolean
  scopeType: string
  scopeId: string
  windowSeconds: number
  supportedWindowSeconds: number[]
  activeUserThresholdMode: string
  activeUserThresholdValue: number
  effectiveActiveUserThreshold: number
  pushMode: string
  selectedChannelIds: string[]
  textTemplate: string
  jsonTemplate: string
  activeWebhookUrl: string
  activeWebhookMethod: string
  activeWebhookHeaders: string
  hasSigningSecret: boolean
  passivePullPath: string
  passiveLatestPath: string
  availableChannels: Array<{ id: string; name: string }>
}

interface DigestPreview {
  windowLabel: string
  messageCount: number
  activeUserCount: number
  thresholdValue: number
  thresholdSatisfied: boolean
  channelCount?: number
  renderedText: string
  renderedJson: string
}

interface DigestRecordItem {
  id: string
  ruleId: string
  scopeType: string
  scopeId: string
  windowSeconds: number
  windowStart: number
  windowEnd: number
  messageCount: number
  activeUserCount: number
  speakerNames: string[]
  speakerSummary: string
  renderedText: string
  renderedJson: string
  status: string
  generatedAt: number
  triggeredBy: string
  deliveryAttempts: number
}

const props = withDefaults(defineProps<{
  scopeId: string
  scopeType?: 'channel' | 'world'
}>(), {
  scopeType: 'channel',
})

const message = useMessage()
const loading = ref(false)
const testing = ref(false)
const errorText = ref('')
const saveErrorText = ref('')
const testErrorText = ref('')
const signingSecret = ref('')
const clearSigningSecret = ref(false)
const passiveToken = ref('')
const passiveIntegrationId = ref('')
const passiveTokenLoading = ref(false)
const passiveTokenError = ref('')
const testWindowSeconds = ref(3600)
const testFromTime = ref('')
const testToTime = ref('')
const testDeliverActive = ref(false)
const testPreview = ref<DigestPreview | null>(null)
const testRecord = ref<DigestRecordItem | null>(null)
const testDelivery = ref<any>(null)

const isWorldScope = computed(() => props.scopeType === 'world')
const scopeLabel = computed(() => isWorldScope.value ? '世界' : '频道')
const scopeKey = computed(() => `${props.scopeType}:${(props.scopeId || '').trim()}`)
const defaultTextTemplate = computed(() => isWorldScope.value ? DEFAULT_WORLD_DIGEST_TEXT_TEMPLATE : DEFAULT_CHANNEL_DIGEST_TEXT_TEMPLATE)
const defaultJsonTemplate = computed(() => isWorldScope.value ? DEFAULT_WORLD_DIGEST_JSON_TEMPLATE : DEFAULT_CHANNEL_DIGEST_JSON_TEMPLATE)

const settings = ref<DigestPushSettings>({
  enabled: false,
  scopeType: 'channel',
  scopeId: '',
  windowSeconds: 3600,
  supportedWindowSeconds: [300, 900, 1800, 3600, 7200, 21600, 86400],
  activeUserThresholdMode: 'channel_member_count',
  activeUserThresholdValue: 0,
  effectiveActiveUserThreshold: 1,
  pushMode: 'passive',
  selectedChannelIds: [],
  textTemplate: DEFAULT_CHANNEL_DIGEST_TEXT_TEMPLATE,
  jsonTemplate: DEFAULT_CHANNEL_DIGEST_JSON_TEMPLATE,
  activeWebhookUrl: '',
  activeWebhookMethod: 'POST',
  activeWebhookHeaders: '{}',
  hasSigningSecret: false,
  passivePullPath: '',
  passiveLatestPath: '',
  availableChannels: [],
})

const hasScope = computed(() => !!props.scopeId && props.scopeId.trim().length > 0)
const accessBaseUrl = computed(() => {
  if (/^https?:\/\//i.test(urlBase)) {
    return urlBase
  }
  if (urlBase.startsWith('//')) {
    return `${window.location.protocol}${urlBase}`
  }
  return `${window.location.origin}${urlBase.startsWith('/') ? '' : '/'}${urlBase}`
})
const readWebhookTokenCache = () => {
  try {
    const raw = localStorage.getItem(WEBHOOK_TOKEN_STORAGE_KEY) || '{}'
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') {
      return String((parsed as Record<string, string>)[scopeKey.value] || '').trim()
    }
  } catch {
    // ignore
  }
  return ''
}
const readWebhookIntegrationCache = () => {
  try {
    const raw = localStorage.getItem(WEBHOOK_INTEGRATION_STORAGE_KEY) || '{}'
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') {
      return String((parsed as Record<string, string>)[scopeKey.value] || '').trim()
    }
  } catch {
    // ignore
  }
  return ''
}
const writeWebhookTokenCache = (value: string) => {
  const key = scopeKey.value
  if (!key) return
  try {
    const raw = localStorage.getItem(WEBHOOK_TOKEN_STORAGE_KEY) || '{}'
    const parsed = JSON.parse(raw)
    const next = parsed && typeof parsed === 'object' ? parsed as Record<string, string> : {}
    if ((value || '').trim()) {
      next[key] = value.trim()
    } else {
      delete next[key]
    }
    localStorage.setItem(WEBHOOK_TOKEN_STORAGE_KEY, JSON.stringify(next))
  } catch {
    // ignore
  }
}
const writeWebhookIntegrationCache = (value: string) => {
  const key = scopeKey.value
  if (!key) return
  try {
    const raw = localStorage.getItem(WEBHOOK_INTEGRATION_STORAGE_KEY) || '{}'
    const parsed = JSON.parse(raw)
    const next = parsed && typeof parsed === 'object' ? parsed as Record<string, string> : {}
    if ((value || '').trim()) {
      next[key] = value.trim()
    } else {
      delete next[key]
    }
    localStorage.setItem(WEBHOOK_INTEGRATION_STORAGE_KEY, JSON.stringify(next))
  } catch {
    // ignore
  }
}
const buildPassiveUrl = (path: string, extraQuery = '') => {
  const normalizedPath = (path || '').trim()
  if (!normalizedPath) return ''
  const queryParts: string[] = []
  if (passiveToken.value) {
    queryParts.push(`token=${encodeURIComponent(passiveToken.value)}`)
  }
  if (extraQuery) {
    queryParts.push(extraQuery)
  }
  const query = queryParts.length > 0 ? `?${queryParts.join('&')}` : ''
  return `${accessBaseUrl.value}${normalizedPath}${query}`
}
const passivePullUrl = computed(() => buildPassiveUrl(settings.value.passivePullPath, 'limit=30'))
const passiveLatestUrl = computed(() => buildPassiveUrl(settings.value.passiveLatestPath))
const passiveTokenReady = computed(() => !!passiveToken.value.trim())
const showFixedThreshold = computed(() => settings.value.activeUserThresholdMode === 'fixed')
const showActivePush = computed(() => settings.value.pushMode === 'active' || settings.value.pushMode === 'both')
const showWorldChannelPicker = computed(() => isWorldScope.value && settings.value.availableChannels.length > 0)

const windowOptions = computed(() => {
  const labels: Record<number, string> = {
    300: '5 分钟',
    900: '15 分钟',
    1800: '30 分钟',
    3600: '1 小时',
    7200: '2 小时',
    21600: '6 小时',
    86400: '24 小时',
  }
  return (settings.value.supportedWindowSeconds || [300, 900, 1800, 3600, 7200, 21600, 86400]).map((value) => ({
    label: labels[value] || `${Math.round(value / 60)} 分钟`,
    value,
  }))
})

const pushModeOptions = [
  { label: '被动拉取', value: 'passive' },
  { label: '主动推送', value: 'active' },
  { label: '主动 + 被动', value: 'both' },
]

const thresholdModeOptions = computed(() => [
  { label: isWorldScope.value ? '覆盖频道成员数' : '频道成员数', value: 'channel_member_count' },
  { label: '固定阈值', value: 'fixed' },
])

const methodOptions = [
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' },
  { label: 'PATCH', value: 'PATCH' },
]

const normalizeSettingsValue = (input?: Partial<DigestPushSettings>) => {
  return {
    ...settings.value,
    ...(input || {}),
    scopeType: props.scopeType,
    scopeId: props.scopeId,
    selectedChannelIds: Array.isArray(input?.selectedChannelIds ?? settings.value.selectedChannelIds)
      ? [...(input?.selectedChannelIds ?? settings.value.selectedChannelIds ?? [])]
      : [],
    availableChannels: Array.isArray(input?.availableChannels ?? settings.value.availableChannels)
      ? [...(input?.availableChannels ?? settings.value.availableChannels ?? [])]
      : [],
    textTemplate: String(input?.textTemplate ?? settings.value.textTemplate ?? '').trim() || defaultTextTemplate.value,
    jsonTemplate: String(input?.jsonTemplate ?? settings.value.jsonTemplate ?? '').trim() || defaultJsonTemplate.value,
    activeWebhookHeaders: '{}',
  }
}

const resetTestResult = () => {
  testErrorText.value = ''
  testPreview.value = null
  testRecord.value = null
  testDelivery.value = null
}

const loadLatestRecord = async () => {
  if (!hasScope.value || !passiveToken.value.trim() || !settings.value.passiveLatestPath.trim()) {
    testRecord.value = null
    return
  }
  try {
    const resp = await api.get<{ item?: DigestRecordItem | null }>(settings.value.passiveLatestPath, {
      params: {
        token: passiveToken.value.trim(),
      },
    })
    testRecord.value = resp.data?.item || null
  } catch {
    // 忽略读取失败，避免覆盖当前测试流程中的其他提示
  }
}

const loadPassivePullCache = () => {
  passiveToken.value = readWebhookTokenCache()
  passiveIntegrationId.value = readWebhookIntegrationCache()
}

const persistPassivePullCache = () => {
  writeWebhookTokenCache(passiveToken.value)
  writeWebhookIntegrationCache(passiveIntegrationId.value)
}

const listWebhookIntegrations = async () => {
  const path = isWorldScope.value
    ? `/api/v1/worlds/${props.scopeId}/digest-integrations`
    : `/api/v1/channels/${props.scopeId}/webhook-integrations`
  const resp = await api.get<{ items: WebhookIntegrationItem[] }>(path)
  return resp.data?.items || []
}

const createPassivePullToken = async () => {
  const path = isWorldScope.value
    ? `/api/v1/worlds/${props.scopeId}/digest-integrations`
    : `/api/v1/channels/${props.scopeId}/webhook-integrations`
  const payload = isWorldScope.value
    ? { name: '世界摘要拉取' }
    : { name: '摘要拉取', source: 'digest-pull', capabilities: ['read_digest'] }
  const resp = await api.post<{ item: WebhookIntegrationItem, token: string }>(path, payload)
  passiveIntegrationId.value = resp.data?.item?.id || ''
  passiveToken.value = resp.data?.token || ''
  persistPassivePullCache()
}

const rotatePassivePullToken = async () => {
  if (!passiveIntegrationId.value) {
    await createPassivePullToken()
    return
  }
  const path = isWorldScope.value
    ? `/api/v1/worlds/${props.scopeId}/digest-integrations/${passiveIntegrationId.value}/rotate`
    : `/api/v1/channels/${props.scopeId}/webhook-integrations/${passiveIntegrationId.value}/rotate`
  const resp = await api.post<{ token: string }>(path, {})
  passiveToken.value = resp.data?.token || ''
  persistPassivePullCache()
}

const ensurePassivePullToken = async (forceRotate = false) => {
  if (!hasScope.value) return
  if (!forceRotate && passiveTokenReady.value) return
  passiveTokenLoading.value = true
  passiveTokenError.value = ''
  try {
    const items = await listWebhookIntegrations()
    const dedicated = items.find(item =>
      item.status === 'active'
      && item.source === 'digest-pull'
      && (isWorldScope.value || (item.capabilities || []).includes('read_digest')),
    )
    if (dedicated) {
      passiveIntegrationId.value = dedicated.id
      if (!passiveTokenReady.value || forceRotate) {
        await rotatePassivePullToken()
      } else {
        persistPassivePullCache()
      }
      return
    }
    await createPassivePullToken()
  } catch (e: any) {
    passiveTokenError.value = e?.response?.data?.message || e?.message || '自动生成 token 失败'
  } finally {
    passiveTokenLoading.value = false
  }
}

const refresh = async () => {
  if (!hasScope.value) return
  loading.value = true
  errorText.value = ''
  saveErrorText.value = ''
  passiveTokenError.value = ''
  signingSecret.value = ''
  clearSigningSecret.value = false
  loadPassivePullCache()
  resetTestResult()
  try {
    const path = isWorldScope.value
      ? `/api/v1/worlds/${props.scopeId}/digest-push`
      : `/api/v1/channels/${props.scopeId}/digest-push`
    const resp = await api.get<DigestPushSettings>(path)
    settings.value = normalizeSettingsValue(resp.data)
    testWindowSeconds.value = settings.value.windowSeconds || 3600
  } catch (e: any) {
    errorText.value = e?.response?.data?.message || e?.message || '加载失败'
  } finally {
    loading.value = false
  }
  await ensurePassivePullToken()
  await loadLatestRecord()
}

const validateBeforeSave = () => {
  saveErrorText.value = ''
  settings.value = normalizeSettingsValue()
  if (showFixedThreshold.value && (!settings.value.activeUserThresholdValue || settings.value.activeUserThresholdValue <= 0)) {
    saveErrorText.value = '固定阈值必须大于 0'
    return false
  }
  if (showActivePush.value && !settings.value.activeWebhookUrl.trim()) {
    saveErrorText.value = '主动推送模式需要填写推送地址'
    return false
  }
  return true
}

const validateBeforeTest = () => {
  testErrorText.value = ''
  settings.value = normalizeSettingsValue()
  if (showFixedThreshold.value && (!settings.value.activeUserThresholdValue || settings.value.activeUserThresholdValue <= 0)) {
    testErrorText.value = '固定阈值必须大于 0'
    return false
  }
  if (showActivePush.value && testDeliverActive.value && !settings.value.activeWebhookUrl.trim()) {
    testErrorText.value = '勾选主动推送测试时，必须先填写主动推送地址'
    return false
  }
  return true
}

const saveSettings = async () => {
  if (!hasScope.value || !validateBeforeSave()) return
  loading.value = true
  errorText.value = ''
  saveErrorText.value = ''
  try {
    const path = isWorldScope.value
      ? `/api/v1/worlds/${props.scopeId}/digest-push`
      : `/api/v1/channels/${props.scopeId}/digest-push`
    const resp = await api.post<DigestPushSettings>(path, {
      enabled: settings.value.enabled,
      windowSeconds: settings.value.windowSeconds,
      activeUserThresholdMode: settings.value.activeUserThresholdMode,
      activeUserThresholdValue: showFixedThreshold.value ? settings.value.activeUserThresholdValue : 0,
      pushMode: settings.value.pushMode,
      selectedChannelIds: isWorldScope.value ? settings.value.selectedChannelIds : [],
      textTemplate: settings.value.textTemplate,
      jsonTemplate: settings.value.jsonTemplate,
      activeWebhookUrl: settings.value.activeWebhookUrl,
      activeWebhookMethod: settings.value.activeWebhookMethod,
      activeWebhookHeaders: settings.value.activeWebhookHeaders,
      signingSecret: signingSecret.value,
      clearSigningSecret: clearSigningSecret.value,
    })
    settings.value = normalizeSettingsValue(resp.data)
    signingSecret.value = ''
    clearSigningSecret.value = false
    message.success(`${scopeLabel.value}未读提醒配置已保存`)
  } catch (e: any) {
    saveErrorText.value = e?.response?.data?.message || e?.message || '保存失败'
  } finally {
    loading.value = false
  }
}

const removeSettings = async () => {
  if (!hasScope.value) return
  loading.value = true
  errorText.value = ''
  saveErrorText.value = ''
  try {
    const path = isWorldScope.value
      ? `/api/v1/worlds/${props.scopeId}/digest-push`
      : `/api/v1/channels/${props.scopeId}/digest-push`
    await api.delete(path)
    await refresh()
    message.success(`${scopeLabel.value}未读提醒配置已删除`)
  } catch (e: any) {
    saveErrorText.value = e?.response?.data?.message || e?.message || '删除失败'
  } finally {
    loading.value = false
  }
}

const toTimestamp = (value: string) => {
  if (!value) return 0
  const ts = new Date(value).getTime()
  return Number.isFinite(ts) ? ts : 0
}

const runTest = async () => {
  if (!hasScope.value || !validateBeforeTest()) return
  testing.value = true
  errorText.value = ''
  testErrorText.value = ''
  resetTestResult()
  try {
    const payload: Record<string, any> = {
      enabled: settings.value.enabled,
      windowSeconds: testWindowSeconds.value || settings.value.windowSeconds,
      activeUserThresholdMode: settings.value.activeUserThresholdMode,
      activeUserThresholdValue: showFixedThreshold.value ? settings.value.activeUserThresholdValue : 0,
      pushMode: settings.value.pushMode,
      selectedChannelIds: isWorldScope.value ? settings.value.selectedChannelIds : [],
      textTemplate: settings.value.textTemplate,
      jsonTemplate: settings.value.jsonTemplate,
      activeWebhookUrl: settings.value.activeWebhookUrl,
      activeWebhookMethod: settings.value.activeWebhookMethod,
      activeWebhookHeaders: settings.value.activeWebhookHeaders,
      signingSecret: signingSecret.value,
      clearSigningSecret: clearSigningSecret.value,
      deliverActive: testDeliverActive.value,
    }
    const fromTs = toTimestamp(testFromTime.value)
    const toTs = toTimestamp(testToTime.value)
    if (fromTs > 0 || toTs > 0) {
      payload.fromTime = fromTs
      payload.toTime = toTs
    }
    const path = isWorldScope.value
      ? `/api/v1/worlds/${props.scopeId}/digest-push/test`
      : `/api/v1/channels/${props.scopeId}/digest-push/test`
    const resp = await api.post(path, payload)
    testPreview.value = resp.data?.preview || null
    testRecord.value = resp.data?.item || null
    testDelivery.value = resp.data?.delivery || null
    await loadLatestRecord()
    message.success(testDeliverActive.value ? '测试推送已执行并落库' : '测试摘要已生成并落库')
  } catch (e: any) {
    testPreview.value = e?.response?.data?.preview || null
    testRecord.value = e?.response?.data?.item || null
    testDelivery.value = e?.response?.data?.delivery || null
    testErrorText.value = e?.response?.data?.message || e?.message || '测试失败'
    await loadLatestRecord()
  } finally {
    testing.value = false
  }
}

const formatRecordTime = (value?: number) => {
  if (!value || value <= 0) return '-'
  try {
    return new Date(value).toLocaleString()
  } catch {
    return String(value)
  }
}

watch(() => [props.scopeId, props.scopeType], refresh, { immediate: true })
watch(() => settings.value.pushMode, () => {
  if (!showActivePush.value) {
    testDeliverActive.value = false
  }
})
watch(passiveToken, () => {
  persistPassivePullCache()
})
onMounted(refresh)
</script>

<template>
  <div class="p-3">
    <n-alert v-if="errorText" type="error" :bordered="false" class="mb-3">
      {{ errorText }}
    </n-alert>

    <n-alert v-if="saveErrorText" type="error" :bordered="false" class="mb-3">
      {{ saveErrorText }}
    </n-alert>

    <n-card :title="`${scopeLabel}未读提醒`" size="small" class="mb-3">
      <n-space vertical size="large">
        <div class="flex items-center justify-between">
          <div>
            <div class="font-medium">启用规则</div>
            <div class="text-xs text-gray-500">窗口结束后按访问人数阈值判断是否生成摘要</div>
          </div>
          <n-switch v-model:value="settings.enabled" :disabled="loading" />
        </div>

        <div>
          <div class="text-sm mb-2">事件周期</div>
          <n-select
            v-model:value="settings.windowSeconds"
            :options="windowOptions"
            :disabled="loading"
          />
        </div>

        <div>
          <div class="text-sm mb-2">登录用户阈值</div>
          <n-radio-group v-model:value="settings.activeUserThresholdMode" name="threshold-mode">
            <n-space>
              <n-radio-button
                v-for="item in thresholdModeOptions"
                :key="item.value"
                :value="item.value"
              >
                {{ item.label }}
              </n-radio-button>
            </n-space>
          </n-radio-group>
          <div class="text-xs text-gray-500 mt-2">
            当前生效阈值：{{ settings.effectiveActiveUserThreshold }}
          </div>
          <n-input-number
            v-if="showFixedThreshold"
            v-model:value="settings.activeUserThresholdValue"
            class="mt-2 w-full"
            :min="1"
            :disabled="loading"
          />
        </div>

        <div v-if="showWorldChannelPicker">
          <div class="text-sm mb-2">合并频道范围</div>
          <n-select
            v-model:value="settings.selectedChannelIds"
            multiple
            clearable
            filterable
            :options="settings.availableChannels.map(item => ({ label: item.name, value: item.id }))"
            :disabled="loading"
            placeholder="留空表示当前世界全部可用频道；选择后只合并这些频道"
          />
          <div class="text-xs text-gray-500 mt-2">
            选中的频道会合并为一条世界级摘要，并写入 webhook JSON 的 <n-code>text</n-code> 字段。
          </div>
        </div>

        <div>
          <div class="text-sm mb-2">推送方式</div>
          <n-radio-group v-model:value="settings.pushMode" name="push-mode">
            <n-space>
              <n-radio-button
                v-for="item in pushModeOptions"
                :key="item.value"
                :value="item.value"
              >
                {{ item.label }}
              </n-radio-button>
            </n-space>
          </n-radio-group>
        </div>

        <div>
          <div class="text-sm mb-2">文本模板</div>
          <n-input
            v-model:value="settings.textTemplate"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 6 }"
            :disabled="loading"
          />
          <div class="text-xs text-gray-500 mt-2">
            可用占位符：<n-code v-pre>{{window_label}}</n-code>、
            <n-code v-pre>{{scope_name}}</n-code>、
            <n-code v-pre>{{channel_name}}</n-code>、
            <n-code v-pre>{{channel_count}}</n-code>、
            <n-code v-pre>{{message_count}}</n-code>、
            <n-code v-pre>{{active_user_count}}</n-code>、
            <n-code v-pre>{{speaker_names}}</n-code>、
            <n-code v-pre>{{speaker_summary}}</n-code>、
            <n-code v-pre>{{channel_digest_lines}}</n-code>
          </div>
        </div>

        <div>
          <div class="text-sm mb-2">JSON 模板</div>
          <n-input
            v-model:value="settings.jsonTemplate"
            type="textarea"
            :autosize="{ minRows: 8, maxRows: 16 }"
            :disabled="loading"
          />
          <div class="text-xs text-gray-500 mt-2">
            JSON 模板中的字符串占位符不要手动加引号，例如 <n-code v-pre>"text": {{rendered_text}}</n-code>
          </div>
        </div>

        <div v-if="showActivePush">
          <div class="text-sm mb-2">主动推送配置</div>
          <n-space vertical size="small">
            <n-input v-model:value="settings.activeWebhookUrl" placeholder="https://example.com/webhook" :disabled="loading" />
            <n-select v-model:value="settings.activeWebhookMethod" :options="methodOptions" :disabled="loading" />
            <div class="text-xs text-gray-500">
              额外请求头默认使用空对象 <n-code>{}</n-code>，当前界面不再单独编辑；如需扩展，请直接在代码或接口层配置。
            </div>
            <n-input
              v-model:value="signingSecret"
              type="password"
              show-password-on="click"
              :disabled="loading || clearSigningSecret"
              placeholder="留空则保持现有签名密钥"
            />
            <div class="flex items-center justify-between text-xs text-gray-500">
              <span>当前签名密钥：{{ settings.hasSigningSecret ? '已配置' : '未配置' }}</span>
              <label class="flex items-center gap-2">
                <input v-model="clearSigningSecret" type="checkbox" />
                清空签名密钥
              </label>
            </div>
          </n-space>
        </div>

        <n-space justify="end">
          <n-button :disabled="loading" @click="removeSettings">删除规则</n-button>
          <n-button type="primary" :loading="loading" @click="saveSettings">保存配置</n-button>
        </n-space>
      </n-space>
    </n-card>

    <n-card title="被动拉取" size="small" class="mb-3">
      <n-space vertical size="small">
        <div class="text-sm">系统会自动创建摘要拉取 token，并拼成可直接访问的完整链接：</div>
        <n-alert v-if="passiveTokenError" type="warning" :bordered="false">
          {{ passiveTokenError }}
        </n-alert>
        <div>
          <div class="text-sm mb-2">被动拉取 Token</div>
          <n-input
            v-model:value="passiveToken"
            type="password"
            show-password-on="click"
            placeholder="可在此直接修改 token"
            :disabled="passiveTokenLoading"
          />
          <n-space class="mt-2" justify="end">
            <n-button size="small" :loading="passiveTokenLoading" @click="ensurePassivePullToken()">自动生成</n-button>
            <n-button size="small" :loading="passiveTokenLoading" @click="ensurePassivePullToken(true)">重新生成</n-button>
          </n-space>
        </div>
        <div class="text-xs break-all">
          列表：<n-code>{{ passivePullUrl }}</n-code>
        </div>
        <div class="text-xs break-all">
          最新：<n-code>{{ passiveLatestUrl }}</n-code>
        </div>
      </n-space>
    </n-card>

    <n-card title="测试推送" size="small">
      <n-space vertical size="large">
        <div>
          <div class="text-sm mb-2">测试周期</div>
          <n-select v-model:value="testWindowSeconds" :options="windowOptions" :disabled="testing" />
        </div>

        <div>
          <div class="text-sm mb-2">指定时间范围（可选）</div>
          <div class="grid grid-cols-1 gap-2">
            <n-input v-model:value="testFromTime" type="datetime-local" :disabled="testing" placeholder="开始时间" />
            <n-input v-model:value="testToTime" type="datetime-local" :disabled="testing" placeholder="结束时间" />
          </div>
          <div class="text-xs text-gray-500 mt-2">
            若不填写，则默认测试最近一个已结束的周期窗口。
          </div>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm">同时触发主动推送</div>
            <div class="text-xs text-gray-500">仅在已配置主动推送地址时有效</div>
          </div>
          <n-switch v-model:value="testDeliverActive" :disabled="testing || !showActivePush" />
        </div>

        <n-space justify="end">
          <n-button type="primary" :loading="testing" @click="runTest">执行测试</n-button>
        </n-space>

        <n-alert v-if="testErrorText" type="error" :bordered="false">
          {{ testErrorText }}
        </n-alert>

        <n-alert v-if="testRecord" type="success" :bordered="false">
          最近落库摘要：
          <div class="text-sm leading-6 mt-1">
            记录 ID：{{ testRecord.id }}<br />
            状态：{{ testRecord.status }}<br />
            时间窗口：{{ formatRecordTime(testRecord.windowStart) }} ~ {{ formatRecordTime(testRecord.windowEnd) }}<br />
            写入时间：{{ formatRecordTime(testRecord.generatedAt) }}<br />
            消息数：{{ testRecord.messageCount }}<br />
            访问人数：{{ testRecord.activeUserCount }}
          </div>
        </n-alert>

        <n-alert v-else type="warning" :bordered="false">
          最近落库摘要为空。请先执行测试，或等待周期任务生成摘要记录。
        </n-alert>

        <n-alert v-if="testPreview" type="info" :bordered="false">
          <div class="text-sm leading-6">
            时间窗口：{{ testPreview.windowLabel }}<br />
            消息数：{{ testPreview.messageCount }}<br />
            <template v-if="isWorldScope && testPreview.channelCount">
              命中频道数：{{ testPreview.channelCount }}<br />
            </template>
            访问人数：{{ testPreview.activeUserCount }} / 阈值 {{ testPreview.thresholdValue }}<br />
            规则命中：{{ testPreview.thresholdSatisfied ? '是' : '否' }}
          </div>
        </n-alert>

        <div v-if="testRecord">
          <div class="text-sm mb-2">最近落库文本</div>
          <n-input :value="testRecord.renderedText" type="textarea" readonly :autosize="{ minRows: 3, maxRows: 6 }" />
        </div>

        <div v-if="testRecord">
          <div class="text-sm mb-2">最近落库 JSON</div>
          <n-input :value="testRecord.renderedJson" type="textarea" readonly :autosize="{ minRows: 8, maxRows: 16 }" />
        </div>

        <n-alert v-if="testDelivery" :type="testDelivery.success ? 'success' : 'warning'" :bordered="false">
          主动推送结果：{{ testDelivery.success ? '成功' : '失败' }}，
          状态码 {{ testDelivery.statusCode || 0 }}，
          耗时 {{ testDelivery.responseTimeMs || 0 }} ms
          <div v-if="testDelivery.errorText" class="mt-1 text-xs">{{ testDelivery.errorText }}</div>
        </n-alert>
      </n-space>
    </n-card>
  </div>
</template>
