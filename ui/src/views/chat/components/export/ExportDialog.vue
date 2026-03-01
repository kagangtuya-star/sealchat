<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { Settings } from '@vicons/ionicons5'
import { useUtilsStore } from '@/stores/utils'
import { useDisplayStore } from '@/stores/display'
import { useChatStore } from '@/stores/chat'

interface ExportParams {
  format: string
  displayName?: string
  timeRange: [number, number] | null
  includeOoc: boolean
  includeArchived: boolean
  includeImages: boolean
  removeDiceCommands: boolean
  withoutTimestamp: boolean
  mergeMessages: boolean
  textColorizeBBCode: boolean
  textColorizeBBCodeMap?: Record<string, string>
  autoUpload: boolean
  maxExportMessages: number
  maxExportConcurrency: number
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

const SLICE_LIMIT_MIN = 1000
const SLICE_LIMIT_MAX = 20000
const SLICE_LIMIT_DEFAULT = 5000
const CONCURRENCY_MIN = 1
const CONCURRENCY_MAX = 8
const CONCURRENCY_DEFAULT = 2
const HTML_SLICE_LIMIT_DEFAULT = 100
const HTML_SLICE_LIMIT_MAX = 500
const HTML_CONCURRENCY_MAX = 2

const clampSliceLimit = (value?: number): number => {
  if (!Number.isFinite(value)) return SLICE_LIMIT_DEFAULT
  const n = Math.round(value as number)
  if (n < SLICE_LIMIT_MIN) return SLICE_LIMIT_MIN
  if (n > SLICE_LIMIT_MAX) return SLICE_LIMIT_MAX
  return n
}

const clampConcurrency = (value?: number): number => {
  if (!Number.isFinite(value)) return CONCURRENCY_DEFAULT
  const n = Math.round(value as number)
  if (n < CONCURRENCY_MIN) return CONCURRENCY_MIN
  if (n > CONCURRENCY_MAX) return CONCURRENCY_MAX
  return n
}

const clampHtmlSliceLimit = (value?: number): number => {
  const parsed = Number(value ?? HTML_SLICE_LIMIT_DEFAULT)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return HTML_SLICE_LIMIT_DEFAULT
  }
  if (parsed > HTML_SLICE_LIMIT_MAX) {
    return HTML_SLICE_LIMIT_MAX
  }
  if (parsed < 50) {
    return 50
  }
  return Math.round(parsed)
}

const clampHtmlConcurrency = (value?: number): number => {
  const parsed = Number(value ?? CONCURRENCY_DEFAULT)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 1
  }
  if (parsed > HTML_CONCURRENCY_MAX) {
    return HTML_CONCURRENCY_MAX
  }
  if (parsed < CONCURRENCY_MIN) {
    return CONCURRENCY_MIN
  }
  return Math.round(parsed)
}

const applyFormatSpecificLimits = () => {
  if (form.format === 'html') {
    form.maxExportMessages = clampHtmlSliceLimit(form.maxExportMessages)
    form.maxExportConcurrency = clampHtmlConcurrency(form.maxExportConcurrency)
  } else {
    form.maxExportMessages = clampSliceLimit(form.maxExportMessages)
    form.maxExportConcurrency = clampConcurrency(form.maxExportConcurrency)
  }
}

const message = useMessage()
const utils = useUtilsStore()
const display = useDisplayStore()
const chat = useChatStore()
const loading = ref(false)
const textColorizeBBCodeMap = ref<Record<string, string>>({})
const colorProfileVisible = ref(false)
const colorProfileLoading = ref(false)
const colorProfileSaving = ref(false)
const colorProfileKeyword = ref('')
let colorProfileLoadSeq = 0

interface ColorProfileRow {
  identityId: string
  mapKey: string
  label: string
  defaultColor: string
  customColor: string
}

interface SpeakerOption {
  id?: string
  label?: string
  color?: string
}

const colorProfileRows = ref<ColorProfileRow[]>([])

const timePreset = ref<'none' | '1d' | '7d' | '30d' | 'custom'>('none')
const isApplyingPreset = ref(false)
const form = reactive<ExportParams>({
  format: 'txt',
  displayName: '',
  timeRange: null,
  includeOoc: true,
  includeArchived: false,
  includeImages: false,
  removeDiceCommands: true,
  withoutTimestamp: false,
  mergeMessages: true,
  textColorizeBBCode: false,
  autoUpload: false,
  maxExportMessages: SLICE_LIMIT_DEFAULT,
  maxExportConcurrency: CONCURRENCY_DEFAULT,
})

const logUploadConfig = computed(() => utils.config?.logUpload)
const cloudUploadEnabled = computed(() => !!logUploadConfig.value?.endpoint && logUploadConfig.value?.enabled !== false)
const cloudUploadHint = computed(() => logUploadConfig.value?.note || '可上传到 DicePP 云端，获得海豹染色器 BBcode/Docx 文件。')
const showCloudUploadOption = computed(() => cloudUploadEnabled.value && form.format === 'json')
const cloudUploadDefaultName = '频道名_时间范围（例如：新的_20251107-20251108）'
const isSealFormatter = computed(() => form.format === 'json')
const showZipOptions = computed(() => form.format === 'html')
const showColorProfileTrigger = computed(() => form.format === 'txt')
const colorProfileCount = computed(() => Object.keys(textColorizeBBCodeMap.value).length)
const filteredColorProfileRows = computed(() => {
  const keyword = colorProfileKeyword.value.trim().toLowerCase()
  if (!keyword) {
    return colorProfileRows.value
  }
  return colorProfileRows.value.filter(item => item.label.toLowerCase().includes(keyword))
})

const normalizeHexColor = (value: string): string => {
  let color = value.trim().toLowerCase()
  if (!color) return ''
  if (!color.startsWith('#')) {
    color = `#${color}`
  }
  if (/^#[0-9a-f]{3}$/.test(color)) {
    const [, r, g, b] = color.split('')
    color = `#${r}${r}${g}${g}${b}${b}`
  }
  if (!/^#[0-9a-f]{6}$/.test(color)) {
    return ''
  }
  return color
}

const normalizeColorMap = (input?: Record<string, string>) => {
  const result: Record<string, string> = {}
  if (!input) {
    return result
  }
  Object.entries(input).forEach(([rawKey, rawColor]) => {
    const key = String(rawKey || '').trim()
    if (!key.startsWith('identity:')) {
      return
    }
    const normalized = normalizeHexColor(String(rawColor || ''))
    if (!normalized) {
      return
    }
    result[key] = normalized
  })
  return result
}

const buildColorMapKey = (identityId: string) => {
  const trimmed = String(identityId || '').trim()
  return trimmed ? `identity:${trimmed}` : ''
}

const buildDefaultColorMapFromSpeakerOptions = (items?: SpeakerOption[]) => {
  const defaults: Record<string, string> = {}
  for (const item of items || []) {
    const key = buildColorMapKey(String(item?.id || ''))
    if (!key) {
      continue
    }
    const color = normalizeHexColor(String(item?.color || ''))
    if (!color) {
      continue
    }
    defaults[key] = color
  }
  return defaults
}

const buildColorOverridesFromRows = () => {
  const result: Record<string, string> = {}
  colorProfileRows.value.forEach((item) => {
    const key = item.mapKey
    if (!key) {
      return
    }
    const normalizedCustom = normalizeHexColor(item.customColor || '')
    if (!normalizedCustom) {
      return
    }
    if (item.defaultColor && normalizedCustom === item.defaultColor) {
      return
    }
    result[key] = normalizedCustom
  })
  return result
}

const getRowPreviewColor = (item: ColorProfileRow) => {
  return normalizeHexColor(item.customColor || '') || item.defaultColor || '#111111'
}

const loadSavedColorMap = async (channelId?: string) => {
  if (!channelId) {
    textColorizeBBCodeMap.value = {}
    return
  }
  try {
    const profile = await chat.channelExportColorProfileGet(channelId)
    textColorizeBBCodeMap.value = normalizeColorMap(profile?.colors)
  } catch (error) {
    console.warn('加载导出颜色配置失败', error)
    textColorizeBBCodeMap.value = {}
  }
}

const openColorProfilePanel = async () => {
  if (!props.channelId) {
    message.error('未选择频道')
    return
  }
  colorProfileVisible.value = true
  colorProfileKeyword.value = ''
  const seq = ++colorProfileLoadSeq
  colorProfileLoading.value = true
  try {
    const [speakerResp, profileResp] = await Promise.all([
      chat.channelSpeakerOptions(props.channelId),
      chat.channelExportColorProfileGet(props.channelId),
    ])
    if (seq !== colorProfileLoadSeq) {
      return
    }
    const savedMap = normalizeColorMap(profileResp?.colors)
    textColorizeBBCodeMap.value = savedMap
    const rows = (speakerResp?.items || [])
      .map((item) => {
        const identityId = String(item?.id || '').trim()
        const mapKey = buildColorMapKey(identityId)
        if (!identityId || !mapKey) {
          return null
        }
        return {
          identityId,
          mapKey,
          label: String(item?.label || '').trim() || '未命名角色',
          defaultColor: normalizeHexColor(String(item?.color || '')),
          customColor: savedMap[mapKey] || '',
        } as ColorProfileRow
      })
      .filter((item): item is ColorProfileRow => !!item)
      .sort((a, b) => a.label.localeCompare(b.label, 'zh-Hans-CN'))
    colorProfileRows.value = rows
  } catch (error) {
    if (seq !== colorProfileLoadSeq) {
      return
    }
    message.error('加载角色颜色配置失败')
    colorProfileRows.value = []
  } finally {
    if (seq === colorProfileLoadSeq) {
      colorProfileLoading.value = false
    }
  }
}

const handleColorRowBlur = (item: ColorProfileRow) => {
  if (!item.customColor) {
    return
  }
  const normalized = normalizeHexColor(item.customColor)
  if (!normalized) {
    message.warning('颜色格式应为 #RGB 或 #RRGGBB')
    item.customColor = ''
    return
  }
  item.customColor = normalized
}

const handleColorPickerInput = (item: ColorProfileRow, event: Event) => {
  const target = event.target as HTMLInputElement | null
  if (!target) {
    return
  }
  const normalized = normalizeHexColor(target.value || '')
  if (!normalized) {
    return
  }
  item.customColor = normalized
}

const resetColorRow = (item: ColorProfileRow) => {
  item.customColor = ''
}

const resetAllColorRows = () => {
  colorProfileRows.value.forEach((item) => {
    item.customColor = ''
  })
}

const saveColorProfile = async () => {
  if (!props.channelId) {
    message.error('未选择频道')
    return
  }
  const colorMap = buildColorOverridesFromRows()
  colorProfileSaving.value = true
  try {
    if (Object.keys(colorMap).length === 0) {
      await chat.channelExportColorProfileDelete(props.channelId)
    } else {
      await chat.channelExportColorProfileUpsert(props.channelId, colorMap)
    }
    textColorizeBBCodeMap.value = colorMap
    message.success('导出颜色配置已保存')
    colorProfileVisible.value = false
  } catch (error: any) {
    const errMsg = error?.response?.data?.message || error?.response?.data?.error || (error as Error)?.message || '保存失败'
    message.error(`保存失败：${errMsg}`)
  } finally {
    colorProfileSaving.value = false
  }
}

watch(
  () => form.format,
  (newFormat) => {
    if (newFormat === 'json' && cloudUploadEnabled.value) {
      form.autoUpload = true
    } else if (newFormat !== 'json') {
      form.autoUpload = false
    }
    if (newFormat !== 'txt') {
      form.textColorizeBBCode = false
    }
    applyFormatSpecificLimits()
  },
  { immediate: true }
)

const syncExportSettingsFromStore = () => {
  const settings = display.settings
  if (!settings) {
    form.maxExportMessages = SLICE_LIMIT_DEFAULT
    form.maxExportConcurrency = CONCURRENCY_DEFAULT
    applyFormatSpecificLimits()
    return
  }
  form.maxExportMessages = clampSliceLimit(settings.maxExportMessages)
  form.maxExportConcurrency = clampConcurrency(settings.maxExportConcurrency)
  applyFormatSpecificLimits()
}

syncExportSettingsFromStore()

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      syncExportSettingsFromStore()
      void loadSavedColorMap(props.channelId)
    } else {
      colorProfileVisible.value = false
    }
  },
)

watch(
  () => props.channelId,
  (channelId) => {
    if (props.visible) {
      void loadSavedColorMap(channelId)
    }
  },
)

watch(
  () => display.settings,
  () => {
    if (props.visible) {
      syncExportSettingsFromStore()
    }
  },
  { deep: true }
)

watch(showColorProfileTrigger, (enabled) => {
  if (!enabled) {
    colorProfileVisible.value = false
  }
})

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

  const isHtmlExport = showZipOptions.value
  const normalizedSliceLimit = isHtmlExport
    ? clampHtmlSliceLimit(form.maxExportMessages)
    : clampSliceLimit(form.maxExportMessages)
  const normalizedConcurrency = isHtmlExport
    ? clampHtmlConcurrency(form.maxExportConcurrency)
    : clampConcurrency(form.maxExportConcurrency)
  form.maxExportMessages = normalizedSliceLimit
  form.maxExportConcurrency = normalizedConcurrency
  display.updateSettings({
    maxExportMessages: normalizedSliceLimit,
    maxExportConcurrency: normalizedConcurrency,
  })

  loading.value = true
  try {
    let colorMap: Record<string, string> | undefined
    if (form.textColorizeBBCode && form.format === 'txt') {
      try {
        const [speakerResp, profileResp] = await Promise.all([
          chat.channelSpeakerOptions(props.channelId),
          chat.channelExportColorProfileGet(props.channelId),
        ])
        const defaultMap = buildDefaultColorMapFromSpeakerOptions(speakerResp?.items as SpeakerOption[] | undefined)
        const savedMap = normalizeColorMap(profileResp?.colors)
        textColorizeBBCodeMap.value = savedMap
        colorMap = Object.keys(defaultMap).length > 0 || Object.keys(savedMap).length > 0
          ? { ...defaultMap, ...savedMap }
          : undefined
      } catch (error) {
        colorMap = normalizeColorMap(textColorizeBBCodeMap.value)
      }
    }
    emit('export', {
      ...form,
      textColorizeBBCodeMap: colorMap,
      displayName: form.displayName?.trim() || undefined,
    })
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
  form.includeImages = false
  form.removeDiceCommands = true
  form.withoutTimestamp = false
  form.mergeMessages = true
  form.textColorizeBBCode = false
  form.autoUpload = false
  form.displayName = ''
  textColorizeBBCodeMap.value = {}
  colorProfileRows.value = []
  colorProfileKeyword.value = ''
  colorProfileVisible.value = false
  syncExportSettingsFromStore()
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

      <n-form-item label="文件名（可选）">
        <n-input
          v-model:value="form.displayName"
          maxlength="120"
          show-count
          placeholder="留空则自动生成，例如：频道记录或 11 月导出"
        />
        <template #feedback>
          若未填写将自动以频道与时间命名；若不带扩展名会自动补齐当前格式的扩展名。
        </template>
      </n-form-item>

      <n-form-item v-if="showZipOptions" label="ZIP 分片">
        <div class="export-slice-settings">
          <div class="export-slice-settings__row">
            <div>
              <p class="row-title">单个文件消息上限</p>
              <p class="row-desc">超过阈值会自动拆分为下一个 HTML 分片</p>
            </div>
            <n-input-number
              v-model:value="form.maxExportMessages"
              :min="50"
              :max="HTML_SLICE_LIMIT_MAX"
              :step="50"
              :show-button="false"
              size="small"
            />
          </div>
          <div class="export-slice-settings__row">
            <div>
              <p class="row-title">最大并发渲染数</p>
              <p class="row-desc">避免并发过大占满 CPU，建议 1-2</p>
            </div>
            <n-input-number
              v-model:value="form.maxExportConcurrency"
              :min="CONCURRENCY_MIN"
              :max="Math.min(CONCURRENCY_MAX, HTML_CONCURRENCY_MAX)"
              size="small"
            />
          </div>
          <p class="row-hint">
            HTML 导出默认分页 {{ HTML_SLICE_LIMIT_DEFAULT }} 条，最多 {{ HTML_SLICE_LIMIT_MAX }} 条；超出限制会自动截断并拆分。
            并发渲染上限 {{ HTML_CONCURRENCY_MAX }}，以降低内存占用。
          </p>
        </div>
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

      <n-form-item label="导出过滤">
        <n-space vertical>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-checkbox v-model:checked="form.includeImages">
                包含图片
              </n-checkbox>
            </template>
            开启后，图片与表情内容会被导出；关闭后将过滤图片内容。
          </n-tooltip>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-checkbox v-model:checked="form.removeDiceCommands">
                移除掷骰指令
              </n-checkbox>
            </template>
            开启后会移除单行命令（如 .ra /ra !ra），但保留指令结果消息。
          </n-tooltip>
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
          <n-tooltip trigger="hover" v-if="form.format === 'txt'">
            <template #trigger>
              <n-space align="center" :wrap-item="false">
                <n-checkbox v-model:checked="form.textColorizeBBCode">
                  使用 BBCode 染色（昵称颜色）
                </n-checkbox>
                <n-button
                  v-if="showColorProfileTrigger"
                  tertiary
                  circle
                  size="tiny"
                  :disabled="!props.channelId"
                  title="配置角色颜色"
                  @click.stop="openColorProfilePanel"
                >
                  <n-icon :component="Settings" />
                </n-button>
              </n-space>
            </template>
            仅对纯文本导出生效，会使用 [color] 标签包裹角色名与内容，并引用频道内的昵称颜色。
          </n-tooltip>
          <n-text depth="3" v-if="showColorProfileTrigger">
            已保存 {{ colorProfileCount }} 个角色颜色覆盖。
          </n-text>
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

  <n-modal
    :show="colorProfileVisible"
    preset="card"
    title="BBCode 染色配置"
    class="export-color-profile-modal"
    :auto-focus="false"
    @update:show="colorProfileVisible = $event"
  >
    <n-spin :show="colorProfileLoading">
      <div class="color-profile-toolbar">
        <n-input
          v-model:value="colorProfileKeyword"
          clearable
          size="small"
          placeholder="搜索角色名称"
        />
        <n-button size="small" tertiary @click="resetAllColorRows" :disabled="!colorProfileRows.length">
          恢复默认
        </n-button>
      </div>
      <div class="color-profile-list" v-if="filteredColorProfileRows.length">
        <div
          v-for="item in filteredColorProfileRows"
          :key="item.identityId"
          class="color-profile-item"
        >
          <div class="color-profile-item__meta">
            <p class="color-profile-item__name">{{ item.label }}</p>
            <p class="color-profile-item__desc">
              默认颜色：{{ item.defaultColor || '无（将使用系统回退色）' }}
            </p>
          </div>
          <div class="color-profile-item__editor">
            <label class="color-profile-item__picker" title="点击选择颜色">
              <input
                class="color-profile-item__picker-input"
                type="color"
                :value="getRowPreviewColor(item)"
                @input="handleColorPickerInput(item, $event)"
              />
            </label>
            <n-input
              v-model:value="item.customColor"
              size="small"
              placeholder="留空使用默认色"
              @blur="handleColorRowBlur(item)"
            />
            <n-button size="small" tertiary @click="resetColorRow(item)">重置</n-button>
          </div>
        </div>
      </div>
      <n-empty v-else description="当前频道暂无可配置角色" />
    </n-spin>
    <template #footer>
      <n-space justify="end">
        <n-button @click="colorProfileVisible = false">取消</n-button>
        <n-button type="primary" :loading="colorProfileSaving" @click="saveColorProfile">
          保存到云端
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

.export-color-profile-modal {
  width: min(760px, 92vw);
}

.export-dialog :deep(.n-input),
.export-dialog :deep(.n-input-wrapper),
.export-dialog :deep(.n-select),
.export-dialog :deep(.n-date-picker),
.export-dialog :deep(.n-base-selection),
.export-dialog :deep(.n-input__content) {
  background-color: var(--sc-bg-input, #ffffff);
  color: var(--sc-text-primary, #0f172a);
}

.export-dialog :deep(.n-input__state-border),
.export-dialog :deep(.n-input),
.export-dialog :deep(.n-base-selection),
.export-dialog :deep(.n-date-picker),
.export-dialog :deep(.n-select) {
  border-color: var(--sc-border-mute, rgba(15, 23, 42, 0.1));
}

.export-dialog :deep(.n-select .n-base-selection-label),
.export-dialog :deep(.n-input__placeholder),
.export-dialog :deep(.n-date-picker .n-input__input-el) {
  color: var(--sc-text-primary, #0f172a);
}

.export-notice {
  margin-bottom: 1.5rem;
}

:deep(.n-modal.export-dialog .n-card),
.export-dialog :deep(.n-card) {
  background-color: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  border: 1px solid var(--sc-border-strong, rgba(15, 23, 42, 0.12));
}

:deep(.n-modal.export-dialog .n-card__segmented),
.export-dialog :deep(.n-card__segmented) {
  background-color: transparent;
}

:deep(.n-alert) {
  .n-alert__header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
}

.export-slice-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.export-slice-settings__row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.row-title {
  font-weight: 600;
  font-size: 0.9rem;
}

.row-desc {
  font-size: 0.78rem;
  color: var(--sc-text-secondary);
  margin-top: 0.15rem;
}

.row-hint {
  font-size: 0.78rem;
  color: var(--sc-text-tertiary, #6b7280);
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

.color-profile-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.color-profile-list {
  max-height: 56vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.color-profile-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.08));
  border-radius: 8px;
  padding: 0.6rem 0.75rem;
}

.color-profile-item__meta {
  min-width: 0;
}

.color-profile-item__name {
  font-weight: 600;
  margin: 0;
}

.color-profile-item__desc {
  margin: 0.2rem 0 0;
  font-size: 12px;
  color: var(--sc-text-tertiary, #6b7280);
}

.color-profile-item__editor {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  flex: 1;
  max-width: 360px;
}

.color-profile-item__picker {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.16));
  overflow: hidden;
  cursor: pointer;
  display: inline-flex;
  padding: 0;
  flex-shrink: 0;
}

.color-profile-item__picker-input {
  width: 100%;
  height: 100%;
  border: none;
  padding: 0;
  background: transparent;
  cursor: pointer;
}

.color-profile-item__picker-input::-webkit-color-swatch-wrapper {
  padding: 0;
}

.color-profile-item__picker-input::-webkit-color-swatch {
  border: none;
}
</style>
