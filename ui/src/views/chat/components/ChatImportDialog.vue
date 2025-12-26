<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import { api, urlBase } from '@/stores/_config'
import { uploadImageAttachment } from '../composables/useAttachmentUploader'
import { InfoCircle } from '@vicons/tabler'
import type { SChannel } from '@/types'

interface ParsedEntry {
  rawLine: string
  timestamp?: string
  roleName: string
  content: string
  isOoc: boolean
  lineNumber: number
}

interface PreviewResponse {
  entries: ParsedEntry[]
  totalLines: number
  parsedCount: number
  skippedCount: number
  detectedRoles: string[]
  usedPattern: string
  usedTemplateName: string
}

interface Template {
  id: string
  name: string
  description: string
  pattern: string
  example: string
}

interface RoleMappingConfig {
  displayName: string
  color: string
  avatarAttachmentId: string
  bindToUserId: string
  reuseIdentityId: string
}

interface WorldMember {
  userId: string
  username: string
  nickname: string
  avatar?: string
}

interface ReusableIdentity {
  id: string
  displayName: string
  color: string
  avatarAttachmentId?: string
  channelId: string
}

interface Props {
  visible: boolean
  channelId?: string
  worldId?: string
}

interface Emits {
  (e: 'update:visible', visible: boolean): void
  (e: 'importStarted', jobId: string): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const message = useMessage()
const chat = useChatStore()
const user = useUserStore()

const step = ref(1)
const loading = ref(false)
const templates = ref<Template[]>([])
const previewResult = ref<PreviewResponse | null>(null)
const worldMembers = ref<WorldMember[]>([])
const reusableIdentities = ref<Record<string, ReusableIdentity[]>>({}) // cacheKey => identities

const form = reactive({
  content: '',
  templateId: 'timestamp_angle',
  regexPattern: '',
  mergeUnmatched: true, // 默认合并连续多行，空行分隔
  strictOoc: false,
  baseTime: null as number | null,
  timeIncrement: 1000,
  roleMapping: {} as Record<string, RoleMappingConfig>,
})

const selectedChannelFilters = ref<string[]>([])
const currentWorldId = computed(() => props.worldId || chat.currentWorldId || '')

const flattenChannels = (channels?: SChannel[]): SChannel[] => {
  if (!channels || channels.length === 0) return []
  const stack = [...channels]
  const result: SChannel[] = []
  while (stack.length) {
    const node = stack.shift()
    if (!node) continue
    result.push(node)
    if (node.children && node.children.length > 0) {
      stack.unshift(...node.children)
    }
  }
  return result
}

const getChannelLabel = (channel: SChannel) => {
  if (!channel) return '未命名频道'
  const base = channel.name || '未命名频道'
  return channel.isPrivate ? `${base}（私密）` : base
}

const channelFilterOptions = computed(() => {
  const worldId = currentWorldId.value
  const worldTree =
    (worldId && chat.channelTreeByWorld?.[worldId]) ||
    chat.channelTree ||
    []
  const publicChannels = flattenChannels(worldTree as SChannel[])
  return publicChannels
    .filter(channel => Boolean(channel?.id) && !channel.isPrivate)
    .map(channel => ({
      label: getChannelLabel(channel),
      value: channel.id!,
    }))
})

const channelLabelById = computed(() => {
  const worldId = currentWorldId.value
  const worldTree =
    (worldId && chat.channelTreeByWorld?.[worldId]) ||
    chat.channelTree ||
    []
  const channels = flattenChannels(worldTree as SChannel[])
  const result: Record<string, string> = {}
  for (const channel of channels) {
    if (!channel?.id || channel.isPrivate) {
      continue
    }
    result[channel.id] = getChannelLabel(channel)
  }
  return result
})

watch(channelFilterOptions, (options) => {
  const validIds = new Set(options.map(option => option.value))
  const filtered = selectedChannelFilters.value.filter(id => validIds.has(id))
  if (filtered.length !== selectedChannelFilters.value.length) {
    selectedChannelFilters.value = filtered
  }
})

watch(currentWorldId, () => {
  selectedChannelFilters.value = []
  reusableIdentities.value = {}
})

const clearChannelFilters = () => {
  if (selectedChannelFilters.value.length) {
    selectedChannelFilters.value = []
  }
}

// 预览配置
const previewLimit = ref(20)

// 正则帮助模态框
const regexHelpVisible = ref(false)

// 加载模板列表
const loadTemplates = async () => {
  if (!props.channelId) return
  try {
    const res = await api.get<{ templates: Template[] }>(`/api/v1/channels/${props.channelId}/import/templates`)
    templates.value = res.data.templates || []
  } catch (e) {
    console.error('加载模板失败:', e)
  }
}

// 加载世界成员列表
const loadWorldMembers = async () => {
  if (!props.worldId) return
  try {
    const resp = await chat.worldMemberList(props.worldId, { page: 1, pageSize: 500 })
    const items = resp?.items || []
    worldMembers.value = items.map((item: any) => ({
      userId: item.userId,
      username: item.username,
      nickname: item.nickname,
      avatar: item.avatar,
    }))
  } catch (e) {
    console.error('加载世界成员失败:', e)
  }
}

const normalizeChannelIds = (channelIds?: string[]) => {
  if (!channelIds || channelIds.length === 0) return []
  return Array.from(new Set(channelIds.filter(Boolean))).sort()
}

const resolveIncludeCurrent = (channelIds: string[], includeCurrent?: boolean) => {
  if (includeCurrent !== undefined) return includeCurrent
  if (!props.channelId) return false
  if (channelIds.length === 0) return true
  return channelIds.includes(props.channelId)
}

const buildIdentityCacheKey = (userId: string, channelIds: string[], includeCurrent: boolean) => {
  const worldKey = currentWorldId.value || 'unknown-world'
  const channelKey = props.channelId || 'unknown-channel'
  const channelKeyPart = channelIds.length ? channelIds.join(',') : 'all'
  const includeKey = includeCurrent ? 'inc' : 'exc'
  return `${worldKey}::${channelKey}::${userId}::${channelKeyPart}::${includeKey}`
}

const getReusableIdentitiesFor = (
  userId: string,
  options?: { channelIds?: string[]; includeCurrent?: boolean }
) => {
  const channelIds = normalizeChannelIds(options?.channelIds ?? selectedChannelFilters.value)
  const includeCurrent = resolveIncludeCurrent(channelIds, options?.includeCurrent)
  const cacheKey = buildIdentityCacheKey(userId, channelIds, includeCurrent)
  return reusableIdentities.value[cacheKey] || []
}

const hasIdentityCache = (cacheKey: string) =>
  Object.prototype.hasOwnProperty.call(reusableIdentities.value, cacheKey)

// 加载指定用户的可复用身份
const loadReusableIdentities = async (
  userId: string,
  options?: { channelIds?: string[]; includeCurrent?: boolean; visibleOnly?: boolean }
) => {
  if (!props.channelId || !userId) return
  const channelIds = normalizeChannelIds(options?.channelIds ?? selectedChannelFilters.value)
  const includeCurrent = resolveIncludeCurrent(channelIds, options?.includeCurrent)
  const cacheKey = buildIdentityCacheKey(userId, channelIds, includeCurrent)
  if (hasIdentityCache(cacheKey)) return
  try {
    const params: Record<string, string | boolean> = {
      userId,
      includeCurrent,
      visibleOnly: options?.visibleOnly ?? true,
    }
    if (channelIds.length) {
      params.channelIds = channelIds.join(',')
    }
    const res = await api.get<{ identities: ReusableIdentity[] }>(
      `/api/v1/channels/${props.channelId}/import/reusable-identities`,
      { params }
    )
    reusableIdentities.value[cacheKey] = res.data.identities || []
  } catch (e) {
    console.error('加载可复用身份失败:', e)
  }
}

watch(selectedChannelFilters, async () => {
  const userIds = new Set<string>()
  for (const mapping of Object.values(form.roleMapping)) {
    if (mapping.bindToUserId) {
      userIds.add(mapping.bindToUserId)
    }
  }
  await Promise.all(Array.from(userIds).map(userId => loadReusableIdentities(userId)))
})

// 世界成员选项
const memberOptions = computed(() => {
  const currentUserId = user.info?.id
  const options = [
    { label: '当前用户', value: currentUserId || '' }
  ]
  for (const member of worldMembers.value) {
    if (member.userId !== currentUserId) {
      options.push({
        label: member.nickname || member.username,
        value: member.userId,
      })
    }
  }
  return options
})

const matchIdentityFilter = (identity?: ReusableIdentity) => {
  if (!identity) return false
  if (!selectedChannelFilters.value.length) return true
  if (!identity.channelId) return true
  return selectedChannelFilters.value.includes(identity.channelId)
}

// 获取指定用户的可复用身份选项
const getIdentityOptions = (userId: string) => {
  const identities = getReusableIdentitiesFor(userId)
  const filteredIdentities = selectedChannelFilters.value.length
    ? identities.filter(matchIdentityFilter)
    : identities
  return [
    { label: '创建新身份', value: '' },
    ...filteredIdentities.map(i => {
      const displayName = i.displayName || '未命名'
      const channelLabel = i.channelId ? (channelLabelById.value[i.channelId] || '未知频道') : ''
      return {
        label: channelLabel ? `${displayName} (${channelLabel})` : displayName,
        value: i.id,
      }
    })
  ]
}

const isIdentityFilteredOut = (userId: string) => {
  if (!selectedChannelFilters.value.length) return false
  const channelIds = normalizeChannelIds(selectedChannelFilters.value)
  const includeCurrent = resolveIncludeCurrent(channelIds)
  const cacheKey = buildIdentityCacheKey(userId, channelIds, includeCurrent)
  if (!hasIdentityCache(cacheKey)) return false
  const identities = reusableIdentities.value[cacheKey] || []
  return identities.length === 0
}

// 当用户变化时加载其可复用身份
const onUserChange = async (role: string, userId: string) => {
  form.roleMapping[role].bindToUserId = userId
  form.roleMapping[role].reuseIdentityId = '' // 重置身份选择
  await loadReusableIdentities(userId)
}

// 当选择复用身份时，自动更新显示名称、颜色和头像
const onIdentityChange = (role: string, identityId: string) => {
  form.roleMapping[role].reuseIdentityId = identityId
  
  if (!identityId) {
    // 清空时不做处理
    return
  }
  
  // 查找选中的身份
  const userId = form.roleMapping[role].bindToUserId
  const identities = getReusableIdentitiesFor(userId)
  const selectedIdentity = identities.find(i => i.id === identityId)
  
  if (selectedIdentity) {
    // 自动更新显示名称
    form.roleMapping[role].displayName = selectedIdentity.displayName
    // 自动更新颜色
    form.roleMapping[role].color = selectedIdentity.color || ''
    // 自动更新头像
    form.roleMapping[role].avatarAttachmentId = selectedIdentity.avatarAttachmentId || ''
  }
}

// 头像上传状态
const avatarUploading = ref<Record<string, boolean>>({})

// 处理头像上传
const handleAvatarUpload = async (role: string, event: Event) => {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return

  avatarUploading.value[role] = true
  try {
    const result = await uploadImageAttachment(file, { channelId: props.channelId })
    // 获取实际的 attachment ID（去掉 'id:' 前缀）
    const attachmentId = result.attachmentId.startsWith('id:')
      ? result.attachmentId.slice(3)
      : result.attachmentId
    form.roleMapping[role].avatarAttachmentId = attachmentId
    message.success('头像上传成功')
  } catch (e: any) {
    message.error(e.message || '头像上传失败')
  } finally {
    avatarUploading.value[role] = false
    target.value = '' // 重置 input
  }
}

// 清除头像
const clearAvatar = (role: string) => {
  form.roleMapping[role].avatarAttachmentId = ''
}

// 获取头像预览URL
const getAvatarUrl = (attachmentId: string) => {
  if (!attachmentId) return ''
  return `${urlBase}/api/v1/attachment/${attachmentId}`
}

// 执行预览
const doPreview = async () => {
  if (!props.channelId || !form.content.trim()) {
    message.warning('请输入日志内容')
    return
  }

  loading.value = true
  try {
    const res = await api.post<PreviewResponse>(`/api/v1/channels/${props.channelId}/import/preview`, {
      content: form.content,
      templateId: form.regexPattern ? '' : form.templateId,
      regexPattern: form.regexPattern,
      previewLimit: previewLimit.value,
      mergeUnmatched: form.mergeUnmatched,
    })

    const data = res.data
    previewResult.value = data

    // 加载世界成员
    await loadWorldMembers()

    // 初始化角色映射
    const currentUserId = user.info?.id || ''
    if (data.detectedRoles) {
      for (const role of data.detectedRoles) {
        if (!form.roleMapping[role]) {
          form.roleMapping[role] = {
            displayName: role,
            color: '',
            avatarAttachmentId: '',
            bindToUserId: currentUserId,
            reuseIdentityId: '',
          }
        }
      }
      // 预加载当前用户的可复用身份
      if (currentUserId) {
        await loadReusableIdentities(currentUserId)
      }
    }

    // 进入下一步
    step.value = 2
  } catch (e: any) {
    message.error(e.response?.data?.message || e.response?.data?.error || '预览请求失败')
  } finally {
    loading.value = false
  }
}

// 执行导入
const doImport = async () => {
  if (!props.channelId) return

  loading.value = true
  try {
    const config = {
      version: '1',
      templateId: form.regexPattern ? '' : form.templateId,
      regexPattern: form.regexPattern,
      baseTime: form.baseTime ? new Date(form.baseTime).toISOString() : null,
      timeIncrement: form.timeIncrement,
      mergeUnmatched: form.mergeUnmatched,
      strictOoc: form.strictOoc,
      roleMapping: form.roleMapping,
    }

    const res = await api.post<{ jobId: string }>(`/api/v1/channels/${props.channelId}/import/execute`, {
      content: form.content,
      config,
    })

    message.success('导入任务已创建')
    emit('importStarted', res.data.jobId)
    handleClose()
  } catch (e: any) {
    message.error(e.response?.data?.message || e.response?.data?.error || '导入请求失败')
  } finally {
    loading.value = false
  }
}

const handleClose = () => {
  emit('update:visible', false)
  // 重置表单
  step.value = 1
  form.content = ''
  form.templateId = 'timestamp_angle'
  form.regexPattern = ''
  form.mergeUnmatched = true
  form.strictOoc = false
  form.baseTime = null
  form.timeIncrement = 1000
  form.roleMapping = {}
  previewResult.value = null
  selectedChannelFilters.value = []
}

const goToStep = (s: number) => {
  if (s < step.value) {
    step.value = s
  }
}

const templateOptions = computed(() =>
  templates.value.map(t => ({
    label: t.name,
    value: t.id,
    description: t.description,
  }))
)

const detectedRoles = computed(() => previewResult.value?.detectedRoles || [])

const previewStats = computed(() => {
  if (!previewResult.value) return null
  return {
    total: previewResult.value.totalLines,
    parsed: previewResult.value.parsedCount,
    skipped: previewResult.value.skippedCount,
    roles: previewResult.value.detectedRoles.length,
  }
})

watch(
  () => props.visible,
  (visible) => {
    if (visible && templates.value.length === 0) {
      loadTemplates()
    }
  }
)

// 处理文件上传
const handleFileUpload = (e: Event) => {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return

  const reader = new FileReader()
  reader.onload = () => {
    form.content = reader.result as string
  }
  reader.readAsText(file)
}

// 导出配置
const exportConfig = () => {
  const config = {
    templateId: form.templateId,
    regexPattern: form.regexPattern,
    roleMapping: form.roleMapping,
    mergeUnmatched: form.mergeUnmatched,
    strictOoc: form.strictOoc,
  }
  const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'import-config.json'
  a.click()
  URL.revokeObjectURL(url)
}

// 导入配置
const importConfig = async (e: Event) => {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return

  const reader = new FileReader()
  reader.onload = async () => {
    try {
      const config = JSON.parse(reader.result as string)
      if (config.templateId) form.templateId = config.templateId
      if (config.regexPattern) form.regexPattern = config.regexPattern
      if (config.mergeUnmatched !== undefined) form.mergeUnmatched = config.mergeUnmatched
      if (config.strictOoc !== undefined) form.strictOoc = config.strictOoc

      // 验证并导入角色映射
      if (config.roleMapping && typeof config.roleMapping === 'object') {
        // 确保世界成员列表已加载
        if (worldMembers.value.length === 0) {
          await loadWorldMembers()
        }

        const validMemberIds = new Set(worldMembers.value.map(m => m.userId))
        validMemberIds.add(user.info?.id || '') // 当前用户也有效

        let warnings: string[] = []

        for (const [roleName, mapping] of Object.entries(config.roleMapping as Record<string, RoleMappingConfig>)) {
          // 验证 bindToUserId
          if (mapping.bindToUserId && !validMemberIds.has(mapping.bindToUserId)) {
            warnings.push(`角色 "${roleName}" 的关联用户不在当前世界成员中，已重置为当前用户`)
            mapping.bindToUserId = user.info?.id || ''
          }

          // 验证 reuseIdentityId（需要先加载该用户的可复用身份）
          if (mapping.reuseIdentityId && mapping.bindToUserId) {
            await loadReusableIdentities(mapping.bindToUserId, {
              channelIds: [],
              includeCurrent: true,
            })
            const identities = getReusableIdentitiesFor(mapping.bindToUserId, {
              channelIds: [],
              includeCurrent: true,
            })
            const validIdentityIds = new Set(identities.map(i => i.id))
            if (!validIdentityIds.has(mapping.reuseIdentityId)) {
              warnings.push(`角色 "${roleName}" 的复用身份不存在，已重置`)
              mapping.reuseIdentityId = ''
            }
          }

          form.roleMapping[roleName] = mapping
        }

        if (warnings.length > 0) {
          message.warning(warnings.join('\n'))
        }
      }

      message.success('配置导入成功')
    } catch {
      message.error('配置文件格式错误')
    }
  }
  reader.readAsText(file)
}
</script>

<template>
  <n-modal
    :show="visible"
    @update:show="emit('update:visible', $event)"
    preset="card"
    title="导入聊天记录"
    class="import-dialog"
    :auto-focus="false"
    style="width: 700px; max-width: 95vw;"
  >
    <!-- 步骤条 -->
    <n-steps :current="step" class="import-steps">
      <n-step title="输入与解析" @click="goToStep(1)" />
      <n-step title="角色映射" @click="goToStep(2)" />
      <n-step title="时间与确认" @click="goToStep(3)" />
    </n-steps>

    <!-- 步骤1: 输入与解析 -->
    <div v-show="step === 1" class="step-content">
      <n-form label-width="100px" label-placement="left">
        <n-form-item label="日志内容">
          <div class="content-input">
            <n-input
              v-model:value="form.content"
              type="textarea"
              placeholder="粘贴日志内容，或点击上传文件..."
              :rows="10"
              :maxlength="500000"
              show-count
            />
            <div class="file-upload">
              <input type="file" accept=".txt,.log" @change="handleFileUpload" />
            </div>
          </div>
        </n-form-item>

        <n-form-item label="解析模板">
          <n-select
            v-model:value="form.templateId"
            :options="templateOptions"
            placeholder="选择解析模板"
            :disabled="!!form.regexPattern"
          />
        </n-form-item>

        <n-form-item>
          <template #label>
            <span>自定义正则</span>
            <n-button text size="tiny" class="regex-help-btn" @click="regexHelpVisible = true">
              <n-icon :component="InfoCircle" size="16" />
            </n-button>
          </template>
          <n-input
            v-model:value="form.regexPattern"
            placeholder="留空使用模板，或输入自定义正则表达式"
          />
          <template #feedback>
            可使用 AI 工具生成正则表达式。正则需包含角色名和内容捕获组。
          </template>
        </n-form-item>

        <n-form-item label="解析选项">
          <n-space vertical>
            <n-checkbox v-model:checked="form.mergeUnmatched">
              合并不匹配行到上一条消息
            </n-checkbox>
            <n-checkbox v-model:checked="form.strictOoc">
              严格 OOC 模式（仅看首字符是否为括号）
            </n-checkbox>
          </n-space>
        </n-form-item>
      </n-form>
    </div>

    <!-- 步骤2: 角色映射 -->
    <div v-show="step === 2" class="step-content">
      <n-alert type="info" class="step-alert">
        从日志中识别到 {{ detectedRoles.length }} 个角色。您可以为每个角色配置显示名称、颜色等。
      </n-alert>

      <div class="config-actions">
        <n-button size="small" @click="exportConfig">导出配置</n-button>
        <label class="config-import-btn">
          <input type="file" accept=".json" @change="importConfig" style="display: none" />
          <n-button size="small" tag="span">导入配置</n-button>
        </label>
      </div>

      <div class="channel-filter-bar">
        <div class="channel-filter-label">筛选频道</div>
        <n-select
          v-model:value="selectedChannelFilters"
          multiple
          filterable
          clearable
          :options="channelFilterOptions"
          max-tag-count="responsive"
          placeholder="选择频道以过滤可复用身份"
          class="channel-filter-select"
        />
        <n-button
          v-if="selectedChannelFilters.length"
          size="tiny"
          text
          type="primary"
          class="channel-filter-clear"
          @click="clearChannelFilters"
        >
          清除
        </n-button>
      </div>

      <div class="role-list">
        <div v-for="role in detectedRoles" :key="role" class="role-card">
          <div class="role-header">
            <span class="role-name">{{ role }}</span>
          </div>
          <n-form label-width="80px" label-placement="left" size="small">
            <n-form-item label="显示名称">
              <n-input
                v-model:value="form.roleMapping[role].displayName"
                placeholder="留空使用原名"
              />
            </n-form-item>
            <n-form-item label="颜色">
              <n-color-picker
                v-model:value="form.roleMapping[role].color"
                :show-alpha="false"
                :modes="['hex']"
              />
            </n-form-item>
            <n-form-item label="头像">
              <div class="avatar-upload">
                <n-avatar
                  v-if="form.roleMapping[role].avatarAttachmentId"
                  :src="getAvatarUrl(form.roleMapping[role].avatarAttachmentId)"
                  :size="48"
                  round
                />
                <n-space v-else size="small">
                  <label class="avatar-upload-btn">
                    <input
                      type="file"
                      accept="image/*"
                      style="display: none"
                      @change="handleAvatarUpload(role, $event)"
                    />
                    <n-button
                      size="small"
                      :loading="avatarUploading[role]"
                      tag="span"
                    >
                      上传头像
                    </n-button>
                  </label>
                </n-space>
                <n-button
                  v-if="form.roleMapping[role].avatarAttachmentId"
                  size="tiny"
                  quaternary
                  type="error"
                  @click="clearAvatar(role)"
                >
                  清除
                </n-button>
              </div>
            </n-form-item>
            <n-form-item label="关联用户">
              <n-select
                :value="form.roleMapping[role].bindToUserId"
                :options="memberOptions"
                placeholder="选择关联用户"
                @update:value="onUserChange(role, $event)"
              />
            </n-form-item>
            <n-form-item label="复用身份">
              <n-select
                v-model:value="form.roleMapping[role].reuseIdentityId"
                :options="getIdentityOptions(form.roleMapping[role].bindToUserId)"
                placeholder="选择复用已有身份"
                @update:value="onIdentityChange(role, $event)"
              />
              <template
                v-if="isIdentityFilteredOut(form.roleMapping[role].bindToUserId)"
                #feedback
              >
                当前筛选下没有可复用身份，清除筛选以查看更多频道身份。
              </template>
            </n-form-item>
          </n-form>
        </div>
      </div>
    </div>

    <!-- 步骤3: 时间与确认 -->
    <div v-show="step === 3" class="step-content">
      <n-form label-width="120px" label-placement="left">
        <n-form-item label="基准时间">
          <n-date-picker
            v-model:value="form.baseTime"
            type="datetime"
            clearable
            placeholder="当日志无日期时使用"
          />
          <template #feedback>
            日志中仅有时间无日期时，使用此日期作为基准。
          </template>
        </n-form-item>

        <n-form-item label="时间增量 (毫秒)">
          <n-input-number
            v-model:value="form.timeIncrement"
            :min="100"
            :max="60000"
            :step="100"
          />
          <template #feedback>
            日志中无时间信息时，每条消息递增的时间间隔。
          </template>
        </n-form-item>

        <n-form-item label="多行合并">
          <n-switch v-model:value="form.mergeUnmatched" />
          <template #feedback>
            开启后，不匹配正则的行会追加到上一条消息。关闭则只解析单行完整匹配的内容。
          </template>
        </n-form-item>
      </n-form>

      <n-divider />

      <div v-if="previewStats" class="import-summary">
        <h4>导入概览</h4>
        <n-descriptions :column="2">
          <n-descriptions-item label="总行数">{{ previewStats.total }}</n-descriptions-item>
          <n-descriptions-item label="解析成功">{{ previewStats.parsed }}</n-descriptions-item>
          <n-descriptions-item label="跳过行数">{{ previewStats.skipped }}</n-descriptions-item>
          <n-descriptions-item label="角色数量">{{ previewStats.roles }}</n-descriptions-item>
        </n-descriptions>
      </div>

      <!-- 预览表格 -->
      <div v-if="previewResult?.entries?.length" class="preview-table">
        <h4>预览（前 {{ previewResult.entries.length }} 条）</h4>
        <n-data-table
          :columns="[
            { title: '行号', key: 'lineNumber', width: 60 },
            { title: '角色', key: 'roleName', width: 100 },
            { title: '内容', key: 'content', ellipsis: { tooltip: true } },
            { title: 'OOC', key: 'isOoc', width: 60, render: (row: ParsedEntry) => row.isOoc ? '是' : '否' },
          ]"
          :data="previewResult.entries"
          :max-height="200"
          size="small"
        />
      </div>
    </div>

    <template #footer>
      <n-space justify="space-between">
        <n-button v-if="step > 1" @click="step--">上一步</n-button>
        <span v-else />
        <n-space>
          <n-button @click="handleClose">取消</n-button>
          <n-button
            v-if="step === 1"
            type="primary"
            :loading="loading"
            :disabled="!form.content.trim()"
            @click="doPreview"
          >
            预览解析结果
          </n-button>
          <n-button
            v-else-if="step === 2"
            type="primary"
            @click="step = 3"
          >
            下一步
          </n-button>
          <n-button
            v-else
            type="primary"
            :loading="loading"
            @click="doImport"
          >
            确认导入
          </n-button>
        </n-space>
      </n-space>
    </template>
  </n-modal>

  <!-- 正则表达式帮助模态框 -->
  <n-modal
    v-model:show="regexHelpVisible"
    preset="card"
    title="正则表达式帮助"
    style="width: 600px; max-width: 90vw;"
    :auto-focus="false"
  >
    <n-scrollbar style="max-height: 60vh;">
      <div class="regex-help-content">
        <h4>正则表达式基础</h4>
        <p>正则表达式需要包含<strong>捕获组</strong>来提取角色名和内容。使用圆括号 <code>()</code> 来定义捕获组。</p>

        <n-divider />

        <h4>常用范例</h4>
        <n-table :bordered="false" size="small">
          <thead>
            <tr>
              <th>格式</th>
              <th>正则表达式</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><code>[时间]&lt;角色&gt;内容</code></td>
              <td><code>\[(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\]\s*&lt;([^&gt;]+)&gt;\s*[:：]?\s*(.*)</code></td>
            </tr>
            <tr>
              <td><code>HH:mm:ss&lt;角色&gt;内容</code></td>
              <td><code>(\d{2}:\d{2}:\d{2})\s*&lt;([^&gt;]+)&gt;\s*[:：]?\s*(.*)</code></td>
            </tr>
            <tr>
              <td><code>&lt;角色&gt;内容</code></td>
              <td><code>&lt;([^&gt;]+)&gt;\s*[:：]?\s*(.*)</code></td>
            </tr>
            <tr>
              <td><code>[角色] 内容</code></td>
              <td><code>\[([^\]]+)\]\s*[:：]?\s*(.*)</code></td>
            </tr>
            <tr>
              <td><code>角色：内容</code></td>
              <td><code>^([^:：\s]+)\s*[:：]\s*(.+)</code></td>
            </tr>
            <tr>
              <td><code>【角色】内容</code></td>
              <td><code>【([^】]+)】\s*[:：]?\s*(.*)</code></td>
            </tr>
          </tbody>
        </n-table>

        <n-divider />

        <h4>AI 提示词示例</h4>
        <p>你可以使用以下提示词让 AI 帮你生成正则表达式：</p>
        <n-card size="small" class="ai-prompt-example">
          <pre>请帮我写一个正则表达式，用于解析以下格式的聊天记录：

示例行：
[粘贴你的日志示例]

要求：
1. 提取角色名到第一个捕获组
2. 提取消息内容到第二个捕获组
3. 如果有时间戳，提取到独立的捕获组
4. 返回可直接在 JavaScript 中使用的正则表达式</pre>
        </n-card>

        <n-divider />

        <h4>捕获组顺序</h4>
        <ul>
          <li><strong>有时间戳</strong>：组1=时间，组2=角色名，组3=内容</li>
          <li><strong>无时间戳</strong>：组1=角色名，组2=内容</li>
        </ul>
      </div>
    </n-scrollbar>
  </n-modal>
</template>

<style lang="scss" scoped>
.import-dialog {
  :deep(.n-card__content) {
    padding-top: 1rem;
  }
}

.import-steps {
  margin-bottom: 1.5rem;
}

.step-content {
  min-height: 300px;
}

.step-alert {
  margin-bottom: 1rem;
}

.content-input {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.file-upload {
  display: flex;
  gap: 0.5rem;
}

.config-actions {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.config-import-btn {
  cursor: pointer;
}

.channel-filter-bar {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
  flex-wrap: wrap;
}

.channel-filter-label {
  font-size: 0.9rem;
  color: var(--sc-text-caption, #475467);
}

.channel-filter-select {
  flex: 1 1 240px;
  min-width: 220px;
}

.role-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-height: 400px;
  overflow-y: auto;
}

.role-card {
  padding: 1rem;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.1));
  border-radius: 8px;
  background: var(--sc-bg-input, #ffffff);
}

.role-header {
  margin-bottom: 0.75rem;
}

.role-name {
  font-weight: 600;
  font-size: 1rem;
}

.import-summary {
  margin-bottom: 1rem;

  h4 {
    margin-bottom: 0.5rem;
  }
}

.preview-table {
  h4 {
    margin-bottom: 0.5rem;
  }
}

.avatar-upload {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.avatar-upload-btn {
  cursor: pointer;
}

.regex-help-btn {
  margin-left: 0.5rem;
  vertical-align: middle;
  opacity: 0.7;

  &:hover {
    opacity: 1;
  }
}

.regex-help-content {
  h4 {
    margin: 0.5rem 0;
    font-size: 1rem;
  }

  p {
    margin: 0.5rem 0;
    line-height: 1.6;
  }

  ul {
    margin: 0.5rem 0;
    padding-left: 1.5rem;
  }

  code {
    background: var(--n-action-color);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.85rem;
  }

  .ai-prompt-example {
    pre {
      margin: 0;
      white-space: pre-wrap;
      font-size: 0.85rem;
      line-height: 1.5;
    }
  }
}
</style>
