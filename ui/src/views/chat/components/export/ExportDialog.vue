<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useUtilsStore } from '@/stores/utils'

interface ExportParams {
  format: string
  timeRange: [number, number] | null
  includeOoc: boolean
  includeArchived: boolean
  withoutTimestamp: boolean
  mergeMessages: boolean
  autoUpload: boolean
}

interface Props {
  visible: boolean
  channelId?: string
}

interface Emits {
  (e: 'update:visible', visible: boolean): void
  (e: 'export', params: ExportParams): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const message = useMessage()
const utils = useUtilsStore()
const loading = ref(false)

const timePreset = ref<'none' | '1d' | '7d' | '30d' | 'custom'>('none')
const isApplyingPreset = ref(false)
const form = reactive<ExportParams>({
  format: 'txt',
  timeRange: null,
  includeOoc: true,
  includeArchived: false,
  withoutTimestamp: false,
  mergeMessages: true,
  autoUpload: false,
})

const logUploadConfig = computed(() => utils.config?.logUpload)
const cloudUploadEnabled = computed(() => !!logUploadConfig.value?.endpoint && logUploadConfig.value?.enabled !== false)
const cloudUploadHint = computed(() => logUploadConfig.value?.note || '可上传到 DicePP 云端，获得海豹染色器 BBcode/Docx 文件。')
const showCloudUploadOption = computed(() => cloudUploadEnabled.value && form.format === 'json')
const cloudUploadDefaultName = '频道名_时间范围（例如：新的_20251107-20251108）'
const isSealFormatter = computed(() => form.format === 'json')

watch(
  () => form.format,
  (newFormat) => {
    if (newFormat === 'json' && cloudUploadEnabled.value) {
      form.autoUpload = true
    } else if (newFormat !== 'json') {
      form.autoUpload = false
    }
  },
  { immediate: true }
)

const formatOptions = [
  { label: '纯文本 (.txt)', value: 'txt' },
  { label: 'HTML (.html)', value: 'html' },
  { label: '海豹染色器 (BBcode/Docx)', value: 'json' },
]

const timePresets = [
  { label: '一天内', value: '1d' },
  { label: '一周内', value: '7d' },
  { label: '一月内', value: '30d' },
]

type PresetValue = '1d' | '7d' | '30d'

const applyPresetRange = (preset: PresetValue) => {
  isApplyingPreset.value = true
  const end = Date.now()
  let start = end
  switch (preset) {
    case '1d':
      start = end - 24 * 60 * 60 * 1000
      break
    case '7d':
      start = end - 7 * 24 * 60 * 60 * 1000
      break
    case '30d':
      start = end - 30 * 24 * 60 * 60 * 1000
      break
  }
  form.timeRange = [start, end]
  timePreset.value = preset
  void nextTick(() => {
    isApplyingPreset.value = false
  })
}

const handlePresetClick = (preset: PresetValue) => {
  applyPresetRange(preset)
}

const handleClearPreset = () => {
  form.timeRange = null
  timePreset.value = 'none'
}

watch(
  () => form.timeRange,
  (newVal, oldVal) => {
    if (isApplyingPreset.value) {
      return
    }
    if (!newVal && oldVal) {
      timePreset.value = 'none'
      return
    }
    if (newVal && timePreset.value !== 'custom') {
      timePreset.value = 'custom'
    }
  }
)

const handleExport = async () => {
  if (!props.channelId) {
    message.error('未选择频道')
    return
  }

  loading.value = true
  try {
    emit('export', { ...form })
  } catch (error) {
    message.error('导出失败')
  } finally {
    loading.value = false
  }
}

const handleClose = () => {
  emit('update:visible', false)
  // 重置表单
  form.format = 'txt'
  form.timeRange = null
  form.includeOoc = true
  form.includeArchived = false
  form.withoutTimestamp = false
  form.mergeMessages = true
  form.autoUpload = false
  timePreset.value = 'none'
}

const shortcuts = {
  '最近7天': () => {
    const end = new Date()
    const start = new Date()
    start.setDate(start.getDate() - 7)
    return [start.getTime(), end.getTime()]
  },
  '最近30天': () => {
    const end = new Date()
    const start = new Date()
    start.setDate(start.getDate() - 30)
    return [start.getTime(), end.getTime()]
  },
  '最近3个月': () => {
    const end = new Date()
    const start = new Date()
    start.setMonth(start.getMonth() - 3)
    return [start.getTime(), end.getTime()]
  },
}
</script>

<template>
  <n-modal
    :show="visible"
    @update:show="emit('update:visible', $event)"
    preset="card"
    title="导出聊天记录"
    class="export-dialog"
    :auto-focus="false"
  >
    <div class="export-notice">
      <n-alert type="info" :show-icon="false">
        <template #header>
          导出说明
        </template>
        <p>提交后系统会在后台生成文件，完成后自动下载。范围越大耗时越久，请耐心等待。</p>
        <p v-if="cloudUploadEnabled" class="cloud-tip">
          云端染色已开放：JSON 导出可一键上传到 SealDice 云端，生成 docx/BBcode 渲染结果。
        </p>
      </n-alert>
    </div>

    <n-form :model="form" label-width="100px" label-placement="left">
      <n-form-item label="导出格式">
        <n-select
          v-model:value="form.format"
          :options="formatOptions"
          placeholder="选择导出格式"
        />
        <template #feedback>
          <div v-if="isSealFormatter" class="seal-tip">
            JSON 导出会生成海豹染色器专用格式，可在云端转换为 BBcode 或 Docx。
          </div>
        </template>
      </n-form-item>

      <n-form-item label="时间范围">
        <div class="time-range">
          <n-date-picker
            v-model:value="form.timeRange"
            type="datetimerange"
            clearable
            :shortcuts="shortcuts"
            format="yyyy-MM-dd HH:mm:ss"
            placeholder="选择时间范围，留空表示全部"
            style="flex: 1"
          />
          <div class="preset-group">
            <n-button-group size="small">
              <n-button
                v-for="item in timePresets"
                :key="item.value"
                :type="timePreset === item.value ? 'primary' : 'default'"
                @click="handlePresetClick(item.value as PresetValue)"
              >
                {{ item.label }}
              </n-button>
            </n-button-group>
            <n-button text size="small" @click="handleClearPreset" v-if="timePreset !== 'none'">
              清除
            </n-button>
          </div>
        </div>
      </n-form-item>

      <n-form-item label="包含内容">
        <n-space vertical>
          <n-checkbox v-model:checked="form.includeOoc">
            包含场外 (OOC) 消息
          </n-checkbox>
          <n-checkbox v-model:checked="form.includeArchived">
            包含已归档消息
          </n-checkbox>
        </n-space>
      </n-form-item>

      <n-form-item label="格式选项">
        <n-space vertical>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-checkbox v-model:checked="form.mergeMessages">
                合并连续消息
              </n-checkbox>
            </template>
            同一角色在短时间内连续发送的消息会拼成一条，仅首条显示时间。
          </n-tooltip>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-checkbox v-model:checked="form.withoutTimestamp">
                不带时间戳
              </n-checkbox>
            </template>
            导出的文本中移除每条消息的时间前缀，适合整理剧本或公开内容。
          </n-tooltip>
        </n-space>
      </n-form-item>

      <n-form-item v-if="showCloudUploadOption" label="云端染色">
        <n-space vertical>
          <n-checkbox v-model:checked="form.autoUpload">
            导出完成后自动上传到云端染色服务
          </n-checkbox>
          <n-text depth="3">{{ cloudUploadHint }}</n-text>
          <n-text depth="3">默认名称：{{ cloudUploadDefaultName }}</n-text>
        </n-space>
      </n-form-item>
    </n-form>

    <template #footer>
      <n-space justify="end">
        <n-button @click="handleClose">取消</n-button>
        <n-button
          type="primary"
          :loading="loading"
          @click="handleExport"
        >
          开始导出
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style lang="scss" scoped>
.export-dialog {
  width: 500px;
  max-width: 90vw;
}

.export-notice {
  margin-bottom: 1.5rem;
}

:deep(.n-alert) {
  .n-alert__header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
}

.time-range {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.preset-group {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.cloud-tip {
  margin-top: 0.5rem;
  line-height: 1.4;
}

.seal-tip {
  margin-top: 0.5rem;
  font-size: 12px;
  color: var(--primary-color);
}
</style>
