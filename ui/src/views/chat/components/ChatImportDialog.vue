<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import { api, urlBase } from '@/stores/_config'
import { uploadImageAttachment } from '../composables/useAttachmentUploader'

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
const reusableIdentities = ref<Record<string, ReusableIdentity[]>>({}) // userId => identities

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

// 预览配置
const previewLimit = ref(20)

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

// 加载指定用户的可复用身份
const loadReusableIdentities = async (userId: string) => {
  if (!props.channelId || !userId) return
  if (reusableIdentities.value[userId]) return // 已加载
  try {
    const res = await api.get<{ identities: ReusableIdentity[] }>(
      `/api/v1/channels/${props.channelId}/import/reusable-identities`,
      { params: { userId } }
    )
    reusableIdentities.value[userId] = res.data.identities || []
  } catch (e) {
    console.error('加载可复用身份失败:', e)
  }
}

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

// 获取指定用户的可复用身份选项
const getIdentityOptions = (userId: string) => {
  const identities = reusableIdentities.value[userId] || []
  return [
    { label: '创建新身份', value: '' },
    ...identities.map(i => ({
      label: i.displayName || '未命名',
      value: i.id,
    }))
  ]
}

// 当用户变化时加载其可复用身份
const onUserChange = async (role: string, userId: string) => {
  form.roleMapping[role].bindToUserId = userId
  form.roleMapping[role].reuseIdentityId = '' // 重置身份选择
  await loadReusableIdentities(userId)
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
            await loadReusableIdentities(mapping.bindToUserId)
            const identities = reusableIdentities.value[mapping.bindToUserId] || []
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

        <n-form-item label="自定义正则">
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
            <n-form-item v-if="getIdentityOptions(form.roleMapping[role].bindToUserId).length > 1" label="复用身份">
              <n-select
                v-model:value="form.roleMapping[role].reuseIdentityId"
                :options="getIdentityOptions(form.roleMapping[role].bindToUserId)"
                placeholder="选择复用已有身份"
              />
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
</style>
