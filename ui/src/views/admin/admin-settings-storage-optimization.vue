<script setup lang="tsx">
import { useUtilsStore } from '@/stores/utils'
import { api } from '@/stores/_config'
import type { AdminStorageStatus, BackupConfig, BackupInfo, S3StorageConfig, SQLiteConfig, ServerConfig, ServerStorageConfig } from '@/types'
import { cloneDeep } from 'lodash-es'
import { NButton, NTag, useMessage } from 'naive-ui'
import dayjs from 'dayjs'
import { computed, h, onMounted, ref, watch } from 'vue'

type AdminStorageOptimizationExpose = {
  save: () => Promise<void>
  isModified: () => boolean
}

type StorageOptimizationModel = {
  backup: BackupConfig
  sqlite: SQLiteConfig
  storage: ServerStorageConfig & { s3: S3StorageConfig }
}

type MessageVisibleCharCountRepairState = {
  status?: string
  mode?: string
  phase?: string
  processed_count?: number
  last_error?: string
  heartbeat_at?: string
  completed_at?: string
  updated_at?: string
}

const utils = useUtilsStore()
const message = useMessage()

const defaultBackupConfig = (): BackupConfig => ({
  enabled: true,
  intervalHours: 12,
  minIntervalMinutes: 10,
  retentionCount: 5,
  path: './backups',
  s3Enabled: false,
  s3Prefix: 'backups',
})

const defaultSQLiteConfig = (): SQLiteConfig => ({
  autoVacuumEnabled: true,
  autoVacuumIntervalHours: 168,
})

const normalizeBackupConfig = (value?: BackupConfig | null): BackupConfig => ({
  enabled: value?.enabled ?? true,
  intervalHours: value?.intervalHours && value.intervalHours > 0 ? value.intervalHours : 12,
  minIntervalMinutes:
    value?.minIntervalMinutes && value.minIntervalMinutes > 0 ? value.minIntervalMinutes : 10,
  retentionCount: value?.retentionCount && value.retentionCount > 0 ? value.retentionCount : 5,
  path: value?.path || './backups',
  s3Enabled: value?.s3Enabled ?? false,
  s3Prefix: value?.s3Prefix?.trim() || 'backups',
})

const normalizeSQLiteConfig = (value?: SQLiteConfig | null): SQLiteConfig => ({
  autoVacuumEnabled: value?.autoVacuumEnabled ?? true,
  autoVacuumIntervalHours:
    value?.autoVacuumIntervalHours && value.autoVacuumIntervalHours > 0
      ? value.autoVacuumIntervalHours
      : 168,
})

const model = ref<StorageOptimizationModel>({
  backup: defaultBackupConfig(),
  sqlite: defaultSQLiteConfig(),
  storage: { s3: {} },
})
const originalSnapshot = ref('')
const isModified = computed(() => JSON.stringify(model.value) !== originalSnapshot.value)
const storageConfig = computed(() => model.value.storage)
const s3Config = computed(() => model.value.storage.s3)

const storageStatus = ref<AdminStorageStatus | null>(null)
const storageStatusLoading = ref(false)
const storageStatusError = ref('')
const s3Testing = ref(false)
const s3TestResult = ref<{ success: boolean; message: string } | null>(null)
const s3Provider = ref('generic')
const s3Presets = [
  { label: '通用 S3', value: 'generic', endpoint: 's3.example.com', region: '按服务商填写', hint: '使用服务商提供的 S3 Endpoint，建议启用 SSL。' },
  { label: '腾讯云 COS', value: 'cos', endpoint: 'cos.ap-guangzhou.myqcloud.com', region: 'ap-guangzhou', hint: 'Bucket 通常包含 APPID；使用所在地域的 COS Endpoint。' },
  { label: '阿里云 OSS', value: 'oss', endpoint: 'oss-cn-hangzhou.aliyuncs.com', region: 'cn-hangzhou', hint: '使用 OSS 的 S3 兼容 Endpoint 与对应地域。' },
  { label: 'AWS S3', value: 'aws', endpoint: 's3.ap-southeast-1.amazonaws.com', region: 'ap-southeast-1', hint: 'Endpoint 与 Bucket 的 Region 保持一致。' },
  { label: 'Cloudflare R2', value: 'r2', endpoint: '<account-id>.r2.cloudflarestorage.com', region: 'auto', hint: 'Region 通常为 auto；凭据使用 R2 S3 API Token。' },
  { label: 'MinIO / 自托管', value: 'minio', endpoint: 'http://localhost:9000', region: 'us-east-1', hint: '填写 S3 API 端口；通常启用 Force Path Style，HTTP 地址使用非 SSL。' },
]
const s3Preset = computed(() => s3Presets.find((preset) => preset.value === s3Provider.value) || s3Presets[0])
const storageModeOptions = [
  { label: 'local · 本地写入', value: 'local' },
  { label: 's3 · 优先 S3，失败回退本地', value: 's3' },
  { label: 'auto · 远端可用时优先 S3', value: 'auto' },
]
const s3ModuleOptions = [
  { key: 'attachmentsEnabled', label: '附件' },
  { key: 'audioEnabled', label: '音频素材' },
  { key: 'ttsEnabled', label: 'TTS 结果' },
  { key: 'theaterEnabled', label: '小剧场资源' },
  { key: 'fontsEnabled', label: '字体资源' },
] as const
type S3ModuleKey = typeof s3ModuleOptions[number]['key']
const moduleChecked = (key: S3ModuleKey) => {
  if (key === 'ttsEnabled') return s3Config.value.ttsEnabled ?? false
  if (key === 'theaterEnabled' && s3Config.value.theaterEnabled == null) {
    return (s3Config.value.attachmentsEnabled ?? true) && (s3Config.value.audioEnabled ?? true)
  }
  return s3Config.value[key] ?? true
}
const moduleIndeterminate = (key: S3ModuleKey) => {
  if (key !== 'theaterEnabled' || s3Config.value.theaterEnabled != null) return false
  return (s3Config.value.attachmentsEnabled ?? true) !== (s3Config.value.audioEnabled ?? true)
}
const backendLabel = (backend?: string) => backend === 's3' ? 'S3' : backend === 'local' ? '本地' : '未知'

const fetchStorageStatus = async () => {
  if (storageStatusLoading.value) return
  storageStatusLoading.value = true
  storageStatusError.value = ''
  try {
    const resp = await api.get<AdminStorageStatus>('/api/v1/admin/storage/status')
    storageStatus.value = resp.data
  } catch {
    storageStatus.value = null
    storageStatusError.value = '获取运行状态失败，请刷新重试'
  } finally {
    storageStatusLoading.value = false
  }
}

const validateS3Credentials = () => {
  if (!!s3Config.value.accessKey?.trim() !== !!s3Config.value.secretKey?.trim()) {
    message.error('Access Key 和 Secret Key 必须成对填写，或同时留空')
    return false
  }
  return true
}

const testS3Connection = async () => {
  if (s3Testing.value || !validateS3Credentials()) return
  s3Testing.value = true
  s3TestResult.value = null
  const testedConfig = cloneDeep(storageConfig.value)
  const testedSnapshot = JSON.stringify(testedConfig)
  try {
    const resp = await api.post<{ success: boolean; message: string }>(
      '/api/v1/admin/storage/s3/test', testedConfig, { timeout: 30000 },
    )
    if (JSON.stringify(storageConfig.value) === testedSnapshot) s3TestResult.value = resp.data
  } catch {
    if (JSON.stringify(storageConfig.value) === testedSnapshot) {
      s3TestResult.value = { success: false, message: '测试请求失败，请检查表单配置或稍后重试' }
    }
  } finally {
    s3Testing.value = false
  }
}

watch(() => JSON.stringify(model.value.storage), () => { s3TestResult.value = null })

const backupConfig = computed({
  get: () => model.value.backup,
  set: (value: BackupConfig) => {
    model.value.backup = normalizeBackupConfig(value)
  },
})

const sqliteMaintenanceConfig = computed({
  get: () => model.value.sqlite,
  set: (value: SQLiteConfig) => {
    model.value.sqlite = normalizeSQLiteConfig(value)
  },
})

const applyConfig = (config?: ServerConfig | null) => {
  const storage = cloneDeep(config?.storage || {})
  model.value = {
    backup: normalizeBackupConfig(config?.backup),
    sqlite: normalizeSQLiteConfig(config?.sqlite),
    storage: {
      ...storage,
      s3: { ...storage.s3, accessKey: '', secretKey: '', sessionToken: '' },
    },
  }
  originalSnapshot.value = JSON.stringify(model.value)
}

const resetFromConfig = async () => {
  const resp = await utils.configGet()
  applyConfig(cloneDeep(resp.data as ServerConfig))
}

const save = async () => {
  if (!validateS3Credentials()) return
  try {
    const resp = await utils.configGet()
    const payload = cloneDeep(resp.data as ServerConfig)
    payload.backup = cloneDeep(model.value.backup)
    payload.sqlite = cloneDeep(model.value.sqlite)
    // Apply only edited storage fields onto the latest complete configuration.
    // Untouched nullable module switches retain their original inheritance.
    const original = JSON.parse(originalSnapshot.value) as StorageOptimizationModel
    payload.storage = cloneDeep(payload.storage || model.value.storage)
    payload.storage.s3 = { ...payload.storage.s3 }
    if (model.value.storage.mode !== original.storage.mode) {
      payload.storage.mode = model.value.storage.mode
    }
    const editableS3Fields = [
      'enabled', 'endpoint', 'region', 'bucket', 'accessKey', 'secretKey', 'sessionToken',
      'publicBaseUrl', 'useSSL', 'forcePathStyle',
      ...s3ModuleOptions.map((option) => option.key),
    ] as const
    for (const key of editableS3Fields) {
      const value = model.value.storage.s3[key]
      if (value !== original.storage.s3[key]) {
        Object.assign(payload.storage.s3, { [key]: value })
      }
    }
    const saveResp = await utils.configSet(payload, { timeout: 120000 })
    await resetFromConfig()
    await fetchStorageStatus()
    if (saveResp.data.storageRestartRequired) {
      message.warning('配置已保存；存储目标发生变化，需重启服务后生效')
    } else {
      message.success('备份与储存优化已保存')
    }
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存失败')
  }
}

defineExpose<AdminStorageOptimizationExpose>({
  save,
  isModified: () => isModified.value,
})

const backupList = ref<BackupInfo[]>([])
const backupListLoading = ref(false)
const backupExecuting = ref(false)
const sqliteVacuumExecuting = ref(false)
const sqliteVacuumStatusLoading = ref(false)
const sqliteDbSizeBytes = ref<number | null>(null)
const sqliteDbSizeError = ref('')
const sqliteLastBeforeSizeBytes = ref<number | null>(null)
const sqliteLastAfterSizeBytes = ref<number | null>(null)
const sqliteLastReclaimedBytes = ref<number | null>(null)
const messageVisibleCharCountRepairLoading = ref(false)
const messageVisibleCharCountRepairExecuting = ref(false)
const messageVisibleCharCountRepairState = ref<MessageVisibleCharCountRepairState | null>(null)

const toNullableNumber = (value: unknown): number | null => {
  const num = Number(value)
  if (!Number.isFinite(num)) {
    return null
  }
  return num
}

const fetchBackupList = async () => {
  backupListLoading.value = true
  try {
    const resp = await utils.adminBackupList()
    backupList.value = resp.data
  } catch {
    message.error('获取备份列表失败')
  } finally {
    backupListLoading.value = false
  }
}

const executeBackup = async () => {
  backupExecuting.value = true
  try {
    await utils.adminBackupExecute()
    message.success('备份任务已提交')
    setTimeout(fetchBackupList, 1000)
  } catch (error: any) {
    message.error('执行备份失败: ' + (error?.response?.data?.message || '未知错误'))
  } finally {
    backupExecuting.value = false
  }
}

const fetchSQLiteVacuumStatus = async () => {
  sqliteVacuumStatusLoading.value = true
  try {
    const resp = await utils.adminSQLiteVacuumStatus()
    sqliteDbSizeBytes.value = toNullableNumber(resp.data?.dbSizeBytes)
    sqliteDbSizeError.value = (resp.data?.dbSizeError || '').toString()
  } catch (error: any) {
    sqliteDbSizeError.value = error?.response?.data?.message || '获取 SQLite 大小失败'
  } finally {
    sqliteVacuumStatusLoading.value = false
  }
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const executeSQLiteVacuum = async () => {
  sqliteVacuumExecuting.value = true
  try {
    const resp = await utils.adminSQLiteVacuumExecute()
    sqliteLastBeforeSizeBytes.value = toNullableNumber(resp.data?.beforeSizeBytes)
    sqliteLastAfterSizeBytes.value = toNullableNumber(resp.data?.afterSizeBytes)
    sqliteLastReclaimedBytes.value = toNullableNumber(resp.data?.reclaimedBytes)
    sqliteDbSizeBytes.value = sqliteLastAfterSizeBytes.value
    sqliteDbSizeError.value = (resp.data?.afterSizeError || '').toString()
    const reclaimed = sqliteLastReclaimedBytes.value
    if (reclaimed !== null) {
      message.success(`数据库空间整理已完成，本次回收 ${formatBytes(Math.max(0, reclaimed))}`)
    } else {
      message.success('数据库空间整理已完成')
    }
  } catch (error: any) {
    message.error('执行空间整理失败: ' + (error?.response?.data?.message || '未知错误'))
  } finally {
    sqliteVacuumExecuting.value = false
  }
}

const formatDateTime = (value?: string | null) => {
  if (!value) return '未知'
  const date = dayjs(value)
  return date.isValid() ? date.format('YYYY-MM-DD HH:mm:ss') : value
}

const messageVisibleCharCountRepairStatusText = computed(() => {
  const status = (messageVisibleCharCountRepairState.value?.status || '').trim()
  switch (status) {
    case 'running':
      return '运行中'
    case 'done':
      return '最近一次完成'
    case 'failed':
      return '最近一次失败'
    default:
      return '未执行'
  }
})

const fetchMessageVisibleCharCountRepairStatus = async () => {
  messageVisibleCharCountRepairLoading.value = true
  try {
    const resp = await utils.adminMessageVisibleCharCountStatus()
    messageVisibleCharCountRepairState.value = (resp.data?.state || null) as MessageVisibleCharCountRepairState | null
  } catch (error: any) {
    message.error('获取消息字数修复状态失败: ' + (error?.response?.data?.message || '未知错误'))
  } finally {
    messageVisibleCharCountRepairLoading.value = false
  }
}

const executeMessageVisibleCharCountRepair = async () => {
  messageVisibleCharCountRepairExecuting.value = true
  try {
    const resp = await utils.adminMessageVisibleCharCountRebuild()
    messageVisibleCharCountRepairState.value = (resp.data?.state || null) as MessageVisibleCharCountRepairState | null
    const processed = Number(resp.data?.state?.processed_count || 0)
    message.success(`消息可见字数重算完成，本次处理 ${processed} 条消息`)
  } catch (error: any) {
    message.error('执行消息字数重算失败: ' + (error?.response?.data?.message || '未知错误'))
    await fetchMessageVisibleCharCountRepairStatus()
  } finally {
    messageVisibleCharCountRepairExecuting.value = false
  }
}

const deleteBackup = async (row: BackupInfo) => {
  try {
    await utils.adminBackupDelete(row.filename, row.storage)
    message.success('删除成功')
    await fetchBackupList()
  } catch (error: any) {
    message.error(error?.response?.data?.error || error?.response?.data?.message || '删除失败')
  }
}

const backupColumns = [
  { title: '文件名', key: 'filename' },
  {
    title: '存储位置',
    key: 'storage',
    render: (row: BackupInfo) => (row.storage === 's3' ? 'S3' : '本地'),
  },
  { title: '大小', key: 'size', render: (row: BackupInfo) => formatBytes(row.size) },
  { title: '创建时间', key: 'createdAt', render: (row: BackupInfo) => dayjs(row.createdAt * 1000).format('YYYY-MM-DD HH:mm:ss') },
  {
    title: '状态',
    key: 'protected',
    render(row: BackupInfo) {
      if (!row.protected) return '普通'
      return h(
        NTag,
        { size: 'small', type: 'warning', round: true },
        { default: () => '历史保护' },
      )
    },
  },
  {
    title: '操作',
    key: 'actions',
    render(row: BackupInfo) {
      return h(
        NButton,
        {
          size: 'tiny',
          type: 'error',
          disabled: row.protected,
          onClick: () => deleteBackup(row),
        },
        { default: () => (row.protected ? '受保护' : '删除') },
      )
    },
  },
]

const migrationStats = ref<{
  total: number
  pending: number
  completed: number
  failed: number
  skipped: number
  spaceSaved: number
} | null>(null)
const migrationLoading = ref(false)
const migrationExecuting = ref(false)
const migrationBatchSize = ref(100)

const fetchMigrationPreview = async () => {
  migrationLoading.value = true
  try {
    const resp = await api.get('/api/v1/admin/image-migration/preview')
    migrationStats.value = resp.data.stats
  } catch (error: any) {
    message.error('获取迁移预览失败: ' + (error?.response?.data?.error || error?.response?.data?.message || '未知错误'))
  } finally {
    migrationLoading.value = false
  }
}

const executeMigration = async (dryRun: boolean = false) => {
  migrationExecuting.value = true
  try {
    const resp = await api.post('/api/v1/admin/image-migration/execute', {
      batchSize: migrationBatchSize.value,
      dryRun,
    })
    const stats = resp.data.stats
    if (dryRun) {
      message.success(`模拟迁移完成: ${stats.completed} 张图片可被迁移，预计节省 ${formatBytes(stats.spaceSaved)}`)
    } else {
      message.success(`迁移完成: ${stats.completed} 成功, ${stats.failed} 失败, ${stats.skipped} 跳过，节省 ${formatBytes(stats.spaceSaved)}`)
    }
    await fetchMigrationPreview()
  } catch (error: any) {
    message.error('执行迁移失败: ' + (error?.response?.data?.message || '未知错误'))
  } finally {
    migrationExecuting.value = false
  }
}

const s3MigrationType = ref<'images' | 'audio' | 'theater'>('images')
const s3MigrationTarget = ref<'s3' | 'local'>('s3')
const s3MigrationStats = ref<{
  total: number
  pending: number
  completed: number
  failed: number
  skipped: number
} | null>(null)
const s3MigrationLoading = ref(false)
const s3MigrationExecuting = ref(false)
const s3MigrationBatchSize = ref(100)
const s3MigrationDeleteSource = ref(true)

watch(s3MigrationType, (value) => {
  s3MigrationDeleteSource.value = value === 'images'
  if (value === 'images') s3MigrationTarget.value = 's3'
  s3MigrationStats.value = null
})

watch(s3MigrationTarget, () => {
  s3MigrationStats.value = null
})

const fetchS3MigrationPreview = async () => {
  s3MigrationLoading.value = true
  try {
    const resp = await api.get('/api/v1/admin/s3-migration/preview', {
      params: { type: s3MigrationType.value, target: s3MigrationTarget.value },
    })
    s3MigrationStats.value = resp.data.stats
  } catch {
    message.error('获取迁移预览失败')
  } finally {
    s3MigrationLoading.value = false
  }
}

const executeS3Migration = async (dryRun: boolean = false) => {
  s3MigrationExecuting.value = true
  try {
    const resp = await api.post('/api/v1/admin/s3-migration/execute', {
      type: s3MigrationType.value,
      target: s3MigrationTarget.value,
      batchSize: s3MigrationBatchSize.value,
      dryRun,
      deleteSource: s3MigrationDeleteSource.value,
    })
    const stats = resp.data.stats
    if (dryRun) {
      message.success(`模拟迁移完成：可迁移 ${stats.completed} 项，跳过 ${stats.skipped} 项`)
    } else {
      message.success(`迁移完成：成功 ${stats.completed} 项，失败 ${stats.failed} 项`)
    }
    await fetchS3MigrationPreview()
  } catch (error: any) {
    message.error('执行迁移失败: ' + (error?.response?.data?.error || error?.response?.data?.message || '未知错误'))
  } finally {
    s3MigrationExecuting.value = false
  }
}

onMounted(async () => {
  await resetFromConfig()
  await Promise.all([fetchBackupList(), fetchSQLiteVacuumStatus(), fetchMessageVisibleCharCountRepairStatus(), fetchStorageStatus()])
})
</script>

<template>
  <div class="admin-settings-scroll overflow-y-auto pr-2" style="max-height: 61vh; margin-top: 0;">
    <n-form label-placement="left" label-width="120">
      <n-collapse class="settings-collapse" :default-expanded-names="[]">
        <n-collapse-item title="数据备份" name="data-backup">
          <n-form-item label="启用自动备份">
            <n-switch v-model:value="backupConfig.enabled" />
          </n-form-item>
          <n-form-item label="备份间隔">
            <n-input-number v-model:value="backupConfig.intervalHours" :min="1">
              <template #suffix>小时</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="最小备份间隔" feedback="仅限制自动备份；手动备份不受影响。">
            <n-input-number v-model:value="backupConfig.minIntervalMinutes" :min="1" :precision="0">
              <template #suffix>分钟</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="保留数量" feedback="超过此数量的旧备份将被自动删除；其中约三分之一历史时间桶会自动保护，超时后才可被轮转。保留数量为 1 时仅保留最新备份，历史保护不会生效。">
            <n-input-number v-model:value="backupConfig.retentionCount" :min="1" />
          </n-form-item>
          <n-form-item label="备份路径" feedback="服务端存储备份文件的绝对路径">
            <n-input v-model:value="backupConfig.path" placeholder="./backups" />
          </n-form-item>
          <n-form-item
            label="备份至 S3"
            feedback="复用平台 S3 配置。备份先在本地生成，确认上传成功后删除本地文件；上传失败时保留本地备份。"
          >
            <n-switch v-model:value="backupConfig.s3Enabled" />
          </n-form-item>
          <n-form-item label="S3 目录">
            <n-input v-model:value="backupConfig.s3Prefix" placeholder="backups" :disabled="!backupConfig.s3Enabled" />
          </n-form-item>
          <n-form-item label="手动备份">
            <div class="flex flex-col gap-2 w-full">
              <div class="flex gap-2">
                <n-button size="small" @click="executeBackup" :loading="backupExecuting">立即备份</n-button>
                <n-button size="small" @click="fetchBackupList" :loading="backupListLoading">刷新列表</n-button>
              </div>
              <n-data-table
                :columns="backupColumns"
                :data="backupList"
                :loading="backupListLoading"
                size="small"
                :max-height="250"
              />
            </div>
          </n-form-item>
        </n-collapse-item>

        <n-collapse-item title="SQLite空间压缩" name="sqlite-space-compress">
          <n-form-item label="当前数据库大小">
            <div class="flex flex-col gap-1">
              <span v-if="sqliteDbSizeBytes !== null">{{ formatBytes(sqliteDbSizeBytes) }}</span>
              <span v-else-if="sqliteVacuumStatusLoading">读取中...</span>
              <span v-else>未知</span>
              <span v-if="sqliteDbSizeError" class="text-xs text-orange-500">{{ sqliteDbSizeError }}</span>
            </div>
          </n-form-item>
          <n-form-item label="启用自动整理" feedback="仅 SQLite 生效：空闲时按周期自动执行 VACUUM">
            <n-switch v-model:value="sqliteMaintenanceConfig.autoVacuumEnabled" />
          </n-form-item>
          <n-form-item label="整理周期">
            <n-input-number v-model:value="sqliteMaintenanceConfig.autoVacuumIntervalHours" :min="1">
              <template #suffix>小时</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="手动整理" feedback="立即触发一次 VACUUM 空间整理">
            <div class="flex flex-col gap-1">
              <n-button size="small" @click="executeSQLiteVacuum" :loading="sqliteVacuumExecuting">立即整理</n-button>
              <span
                v-if="sqliteLastBeforeSizeBytes !== null && sqliteLastAfterSizeBytes !== null"
                class="text-xs text-gray-600 dark:text-gray-400"
              >
                整理前 {{ formatBytes(sqliteLastBeforeSizeBytes) }}，整理后 {{ formatBytes(sqliteLastAfterSizeBytes) }}，
                回收 {{ formatBytes(Math.max(0, sqliteLastReclaimedBytes ?? 0)) }}
              </span>
            </div>
          </n-form-item>
        </n-collapse-item>

        <n-collapse-item title="输入统计字数修复" name="message-visible-char-count-repair">
          <n-form-item label="最近状态">
            <div class="flex flex-col gap-1">
              <span v-if="messageVisibleCharCountRepairLoading">读取中...</span>
              <template v-else>
                <span>{{ messageVisibleCharCountRepairStatusText }}</span>
                <span class="text-xs text-gray-600 dark:text-gray-400">
                  模式: {{ messageVisibleCharCountRepairState?.mode || '未知' }}，
                  已处理 {{ Number(messageVisibleCharCountRepairState?.processed_count || 0) }} 条
                </span>
                <span v-if="messageVisibleCharCountRepairState?.completed_at" class="text-xs text-gray-600 dark:text-gray-400">
                  完成时间：{{ formatDateTime(messageVisibleCharCountRepairState?.completed_at) }}
                </span>
                <span v-if="messageVisibleCharCountRepairState?.last_error" class="text-xs text-orange-500">
                  最近错误：{{ messageVisibleCharCountRepairState?.last_error }}
                </span>
              </template>
            </div>
          </n-form-item>
          <n-form-item
            label="强制重算"
            feedback="全库重算 messages.visible_char_count，仅用于修复消息总字数明显偏低。纯图片消息仍会是 0。"
          >
            <div class="flex flex-col gap-1">
              <div class="flex gap-2">
                <n-button size="small" @click="fetchMessageVisibleCharCountRepairStatus" :loading="messageVisibleCharCountRepairLoading">
                  刷新状态
                </n-button>
                <n-popconfirm @positive-click="executeMessageVisibleCharCountRepair">
                  <template #trigger>
                    <n-button size="small" type="warning" :loading="messageVisibleCharCountRepairExecuting">
                      立即重算
                    </n-button>
                  </template>
                  确定要全库重算消息可见字数吗？数据量较大时可能耗时较久。
                </n-popconfirm>
              </div>
              <span v-if="messageVisibleCharCountRepairState?.updated_at" class="text-xs text-gray-600 dark:text-gray-400">
                最近心跳：{{ formatDateTime(messageVisibleCharCountRepairState?.updated_at) }}
              </span>
            </div>
          </n-form-item>
        </n-collapse-item>

        <n-collapse-item title="对象存储（S3）" name="object-storage-s3">
          <div class="s3-status-strip">
            <div class="s3-status-summary">
              <span class="font-medium">当前运行状态</span>
              <div v-if="storageStatus" class="s3-status-tags">
                <n-tag size="small" :type="storageStatus.enabled ? 'success' : 'default'">
                  S3 {{ storageStatus.enabled ? '已启用' : '未启用' }}
                </n-tag>
                <n-tag size="small" :type="storageStatus.remoteReady ? 'success' : storageStatus.lastError ? 'error' : 'default'">
                  远端{{ storageStatus.remoteReady ? '已初始化' : '未就绪' }}
                </n-tag>
                <n-tag size="small">ActiveBackend：{{ backendLabel(storageStatus.activeBackend) }}</n-tag>
              </div>
              <n-text v-else :type="storageStatusError ? 'error' : 'default'">
                {{ storageStatusError || '读取中…' }}
              </n-text>
            </div>

            <template v-if="storageStatus">
              <dl class="s3-status-details">
                <div><dt>Endpoint</dt><dd>{{ storageStatus.endpoint || '未配置' }}</dd></div>
                <div><dt>Region</dt><dd>{{ storageStatus.region || '默认' }}</dd></div>
                <div><dt>Bucket</dt><dd>{{ storageStatus.bucket || '未配置' }}</dd></div>
              </dl>

              <div class="s3-status-modules">
                <span class="s3-status-section-label">实际写入位置</span>
                <div class="s3-status-module-values">
                  <span>附件 {{ backendLabel(storageStatus.modules.attachments) }}</span>
                  <span>音频 {{ backendLabel(storageStatus.modules.audio) }}</span>
                  <span>TTS {{ backendLabel(storageStatus.modules.tts) }}</span>
                  <span v-if="storageStatus.modules.theaterAttachments === storageStatus.modules.theaterAudio">
                    小剧场 {{ backendLabel(storageStatus.modules.theaterAttachments) }}
                  </span>
                  <span v-else>
                    小剧场附件 {{ backendLabel(storageStatus.modules.theaterAttachments) }} / 音频 {{ backendLabel(storageStatus.modules.theaterAudio) }}
                  </span>
                  <span>字体 {{ backendLabel(storageStatus.modules.fonts) }}</span>
                </div>
              </div>
            </template>

            <div class="s3-status-actions">
              <n-button size="small" :loading="storageStatusLoading" @click="fetchStorageStatus">刷新状态</n-button>
            </div>

            <div class="s3-status-footer">
              <n-text v-if="storageStatus?.lastError" type="error">{{ storageStatus.lastError }}</n-text>
              <span class="s3-hint">模块开关与同一存储目标内的配置可即时生效；Endpoint、Bucket 或本地存储目录变更需重启。</span>
            </div>
          </div>

          <div class="s3-config-heading">快速配置</div>
          <div class="s3-config-grid">
            <n-form-item label="启用 S3" label-placement="top" :show-feedback="false">
              <n-switch :value="s3Config.enabled ?? false" @update:value="s3Config.enabled = $event" />
            </n-form-item>
            <n-form-item label="存储模式" label-placement="top" :show-feedback="false">
              <n-select :value="storageConfig.mode || 'local'" :options="storageModeOptions" @update:value="storageConfig.mode = $event" />
            </n-form-item>
            <n-form-item label="服务商预设" label-placement="top" :show-feedback="false">
              <n-select v-model:value="s3Provider" :options="s3Presets" />
            </n-form-item>
          </div>
          <div class="s3-hint">{{ s3Preset.hint }} 预设仅提供填写提示。</div>
          <div v-if="!s3Config.enabled && s3Config.bucket?.trim()" class="s3-hint">
            关闭 S3 不会清除现有 Endpoint、Bucket 与凭据；需重启服务后生效。
          </div>
          <div class="s3-config-grid">
            <n-form-item label="Endpoint" label-placement="top" :show-feedback="false">
              <n-input v-model:value="s3Config.endpoint" :placeholder="s3Preset.endpoint" />
            </n-form-item>
            <n-form-item label="Region" label-placement="top" :show-feedback="false">
              <n-input v-model:value="s3Config.region" :placeholder="s3Preset.region" />
            </n-form-item>
            <n-form-item label="Bucket" label-placement="top" :show-feedback="false">
              <n-input v-model:value="s3Config.bucket" placeholder="Bucket 名称" />
            </n-form-item>
            <n-form-item label="Access Key" label-placement="top" :show-feedback="false">
              <n-input v-model:value="s3Config.accessKey" type="password" show-password-on="click" placeholder="留空保持当前凭据" autocomplete="off" />
            </n-form-item>
            <n-form-item label="Secret Key" label-placement="top" :show-feedback="false">
              <n-input v-model:value="s3Config.secretKey" type="password" show-password-on="click" placeholder="留空保持当前凭据" autocomplete="new-password" />
            </n-form-item>
            <n-form-item label="Session Token（可选）" label-placement="top" :show-feedback="false">
              <n-input v-model:value="s3Config.sessionToken" type="password" show-password-on="click" placeholder="留空沿用当前 Token；填写则替换" autocomplete="off" />
            </n-form-item>
            <n-form-item label="Public Base URL" label-placement="top" :show-feedback="false">
              <n-input v-model:value="s3Config.publicBaseUrl" placeholder="可选：公开访问或 CDN 地址" />
            </n-form-item>
          </div>
          <div class="s3-module-options">
            <n-checkbox :checked="s3Config.useSSL ?? true" @update:checked="s3Config.useSSL = $event">Use SSL</n-checkbox>
            <n-checkbox :checked="s3Config.forcePathStyle ?? false" @update:checked="s3Config.forcePathStyle = $event">Force Path Style</n-checkbox>
          </div>

          <div class="s3-config-heading">使用对象存储的模块</div>
          <div class="s3-module-options">
            <n-checkbox
              v-for="option in s3ModuleOptions"
              :key="option.key"
              :checked="moduleChecked(option.key)"
              :indeterminate="moduleIndeterminate(option.key)"
              @update:checked="s3Config[option.key] = $event"
            >
              {{ option.label }}
              <span v-if="option.key === 'theaterEnabled' && s3Config.theaterEnabled == null" class="s3-inherit-label">（继承附件/音频）</span>
            </n-checkbox>
          </div>
          <div class="s3-hint">TTS 结果仅包含合成后的语音，不包含音色复刻源样本；仅影响新写入资源。</div>
          <div class="s3-hint">模块开关仅影响新写入资源；当前存储迁移仅支持下方已有类型，历史 TTS 暂不自动迁移。</div>
          <div class="s3-test-row">
            <n-button size="small" :loading="s3Testing" @click="testS3Connection">测试连接</n-button>
            <n-text v-if="s3TestResult" :type="s3TestResult.success ? 'success' : 'error'">{{ s3TestResult.message }}</n-text>
            <span v-else class="s3-hint">验证当前表单的 Bucket 读写能力，不保存配置。</span>
          </div>
        </n-collapse-item>

        <n-collapse-item title="存储迁移" name="storage-migration">
          <n-form-item label="迁移类型">
            <n-select
              v-model:value="s3MigrationType"
              :options="[
                { label: '图片附件', value: 'images' },
                { label: '音频', value: 'audio' },
                { label: '小剧场资源', value: 'theater' },
              ]"
              class="w-52"
            />
          </n-form-item>
          <n-form-item label="目标存储">
            <n-select
              v-model:value="s3MigrationTarget"
              :options="[
                { label: 'S3', value: 's3' },
                { label: '本地', value: 'local', disabled: s3MigrationType === 'images' },
              ]"
              class="w-52"
            />
          </n-form-item>
          <n-form-item label="迁移状态">
            <div class="flex flex-col gap-2 w-full">
              <div v-if="s3MigrationStats" class="text-sm text-gray-600 dark:text-gray-400">
                待迁移: {{ s3MigrationStats.pending }} 项
              </div>
              <div class="flex gap-2 items-center">
                <n-button size="small" @click="fetchS3MigrationPreview" :loading="s3MigrationLoading">刷新预览</n-button>
              </div>
            </div>
          </n-form-item>
          <n-form-item label="批量大小">
            <n-input-number v-model:value="s3MigrationBatchSize" :min="1" :max="1000" />
          </n-form-item>
          <n-form-item label="删除源文件" feedback="仅在目标文件写入并校验成功后删除源文件">
            <n-switch v-model:value="s3MigrationDeleteSource" />
          </n-form-item>
          <n-form-item label="执行迁移">
            <div class="flex gap-2">
              <n-button
                size="small"
                @click="executeS3Migration(true)"
                :loading="s3MigrationExecuting"
                :disabled="!s3MigrationStats || s3MigrationStats.pending === 0"
              >
                模拟运行
              </n-button>
              <n-popconfirm @positive-click="executeS3Migration(false)">
                <template #trigger>
                  <n-button
                    size="small"
                    type="warning"
                    :loading="s3MigrationExecuting"
                    :disabled="!s3MigrationStats || s3MigrationStats.pending === 0"
                  >
                    执行迁移
                  </n-button>
                </template>
                确定要执行迁移吗？资源将迁移到 {{ s3MigrationTarget === 's3' ? 'S3' : '本地存储' }}。
                <span v-if="s3MigrationDeleteSource">目标文件校验成功后将删除源文件。</span>
              </n-popconfirm>
            </div>
          </n-form-item>
        </n-collapse-item>

        <n-collapse-item title="图片压缩" name="image-migrate-webp">
          <n-form-item label="迁移状态">
            <div class="flex flex-col gap-2 w-full">
              <div v-if="migrationStats" class="text-sm text-gray-600 dark:text-gray-400">
                待迁移（非Webp的图片）: {{ migrationStats.pending }} 张 (不含 GIF 和 S3 图片)
              </div>
              <div class="flex gap-2 items-center">
                <n-button size="small" @click="fetchMigrationPreview" :loading="migrationLoading">刷新预览</n-button>
              </div>
            </div>
          </n-form-item>
          <n-form-item label="批量大小">
            <n-input-number v-model:value="migrationBatchSize" :min="1" :max="1000" />
          </n-form-item>
          <n-form-item label="执行迁移">
            <div class="flex gap-2">
              <n-button
                size="small"
                @click="executeMigration(true)"
                :loading="migrationExecuting"
                :disabled="!migrationStats || migrationStats.pending === 0"
              >
                模拟运行
              </n-button>
              <n-popconfirm @positive-click="executeMigration(false)">
                <template #trigger>
                  <n-button
                    size="small"
                    type="warning"
                    :loading="migrationExecuting"
                    :disabled="!migrationStats || migrationStats.pending === 0"
                  >
                    执行迁移
                  </n-button>
                </template>
                确定要执行迁移吗？此操作会将 {{ migrationBatchSize }} 张图片转换为 WebP 格式，原文件将被删除。
              </n-popconfirm>
            </div>
          </n-form-item>
        </n-collapse-item>
      </n-collapse>
    </n-form>
  </div>
</template>

<style scoped>
.admin-settings-scroll {
  overflow-x: hidden;
  overflow-y: scroll;
  scrollbar-gutter: stable;
}

.settings-collapse {
  width: 100%;
}

.s3-status-strip {
  display: grid;
  grid-template-columns: minmax(250px, 0.9fr) minmax(420px, 1.6fr) minmax(320px, 1.2fr) auto;
  gap: 14px 24px;
  align-items: center;
  width: 100%;
  margin-bottom: 16px;
  padding: 12px 0 10px;
  border-top: 1px solid rgba(128, 128, 128, 0.2);
  border-bottom: 1px solid rgba(128, 128, 128, 0.2);
}

.s3-status-summary,
.s3-status-modules {
  min-width: 0;
}

.s3-status-summary {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.s3-status-tags,
.s3-status-module-values,
.s3-module-options,
.s3-test-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 12px;
}

.s3-status-details {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(110px, 0.7fr) minmax(140px, 0.9fr);
  gap: 12px 20px;
  margin: 0;
}

.s3-status-details > div,
.s3-config-grid > * {
  min-width: 0;
}

.s3-status-details dt,
.s3-status-section-label,
.s3-hint,
.s3-inherit-label {
  font-size: 12px;
  opacity: 0.72;
}

.s3-status-details dd {
  margin: 2px 0 0;
  overflow-wrap: anywhere;
}

.s3-status-modules {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}

.s3-status-actions {
  align-self: start;
}

.s3-status-footer {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 16px;
  min-width: 0;
}

.s3-status-footer .s3-hint {
  margin: 0;
}

.s3-config-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
  gap: 12px;
}

.s3-hint {
  margin: 8px 0 12px;
}

@media (max-width: 1180px) {
  .s3-status-strip {
    grid-template-columns: minmax(240px, 0.9fr) minmax(0, 1.6fr) auto;
  }

  .s3-status-modules {
    grid-column: 1 / -2;
  }
}

@media (max-width: 760px) {
  .s3-status-strip {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .s3-status-details {
    grid-template-columns: 1fr;
  }

  .s3-status-modules,
  .s3-status-actions,
  .s3-status-footer {
    grid-column: 1;
  }

  .s3-status-actions {
    justify-self: start;
  }
}

.s3-config-heading {
  font-weight: 500;
  margin: 16px 0 10px;
}

.s3-test-row {
  margin-top: 16px;
  overflow-wrap: anywhere;
}

.s3-test-row .s3-hint {
  margin: 0;
}
</style>
