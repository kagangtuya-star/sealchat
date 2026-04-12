<script setup lang="tsx">
import { useUtilsStore } from '@/stores/utils'
import { api } from '@/stores/_config'
import type { BackupConfig, BackupInfo, SQLiteConfig, ServerConfig } from '@/types'
import { cloneDeep } from 'lodash-es'
import { NButton, useMessage } from 'naive-ui'
import dayjs from 'dayjs'
import { computed, h, onMounted, ref, watch } from 'vue'

type AdminStorageOptimizationExpose = {
  save: () => Promise<void>
  isModified: () => boolean
}

type StorageOptimizationModel = {
  backup: BackupConfig
  sqlite: SQLiteConfig
}

const utils = useUtilsStore()
const message = useMessage()

const defaultBackupConfig = (): BackupConfig => ({
  enabled: true,
  intervalHours: 12,
  retentionCount: 5,
  path: './backups',
})

const defaultSQLiteConfig = (): SQLiteConfig => ({
  autoVacuumEnabled: true,
  autoVacuumIntervalHours: 168,
})

const normalizeBackupConfig = (value?: BackupConfig | null): BackupConfig => ({
  enabled: value?.enabled ?? true,
  intervalHours: value?.intervalHours && value.intervalHours > 0 ? value.intervalHours : 12,
  retentionCount: value?.retentionCount && value.retentionCount > 0 ? value.retentionCount : 5,
  path: value?.path || './backups',
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
})
const originalSnapshot = ref('')
const isModified = computed(
  () =>
    JSON.stringify({
      backup: model.value.backup,
      sqlite: model.value.sqlite,
    }) !== originalSnapshot.value,
)

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
  model.value = {
    backup: normalizeBackupConfig(config?.backup),
    sqlite: normalizeSQLiteConfig(config?.sqlite),
  }
  originalSnapshot.value = JSON.stringify({
    backup: model.value.backup,
    sqlite: model.value.sqlite,
  })
}

const resetFromConfig = async () => {
  const resp = await utils.configGet()
  applyConfig(cloneDeep(resp.data as ServerConfig))
}

const save = async () => {
  try {
    const resp = await utils.configGet()
    const payload = cloneDeep(resp.data as ServerConfig)
    payload.backup = cloneDeep(model.value.backup)
    payload.sqlite = cloneDeep(model.value.sqlite)
    await utils.configSet(payload)
    applyConfig(payload)
    message.success('备份与储存优化已保存')
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

const deleteBackup = async (row: BackupInfo) => {
  try {
    await utils.adminBackupDelete(row.filename)
    message.success('删除成功')
    await fetchBackupList()
  } catch {
    message.error('删除失败')
  }
}

const backupColumns = [
  { title: '文件名', key: 'filename' },
  { title: '大小', key: 'size', render: (row: BackupInfo) => formatBytes(row.size) },
  { title: '创建时间', key: 'createdAt', render: (row: BackupInfo) => dayjs(row.createdAt * 1000).format('YYYY-MM-DD HH:mm:ss') },
  {
    title: '操作',
    key: 'actions',
    render(row: BackupInfo) {
      return h(
        NButton,
        {
          size: 'tiny',
          type: 'error',
          onClick: () => deleteBackup(row),
        },
        { default: () => '删除' },
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
  } catch {
    message.error('获取迁移预览失败')
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

const s3MigrationType = ref<'images' | 'audio'>('images')
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
  s3MigrationStats.value = null
})

const fetchS3MigrationPreview = async () => {
  s3MigrationLoading.value = true
  try {
    const resp = await api.get('/api/v1/admin/s3-migration/preview', {
      params: { type: s3MigrationType.value },
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
    message.error('执行迁移失败: ' + (error?.response?.data?.message || '未知错误'))
  } finally {
    s3MigrationExecuting.value = false
  }
}

onMounted(async () => {
  await resetFromConfig()
  await Promise.all([fetchBackupList(), fetchSQLiteVacuumStatus()])
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
          <n-form-item label="保留数量" feedback="超过此数量的旧备份将被自动删除">
            <n-input-number v-model:value="backupConfig.retentionCount" :min="1" />
          </n-form-item>
          <n-form-item label="备份路径" feedback="服务端存储备份文件的绝对路径">
            <n-input v-model:value="backupConfig.path" placeholder="./backups" />
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

        <n-collapse-item title="迁移到 S3" name="migrate-to-s3">
          <n-form-item label="迁移类型">
            <n-select
              v-model:value="s3MigrationType"
              :options="[
                { label: '图片附件', value: 'images' },
                { label: '音频', value: 'audio' },
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
          <n-form-item label="删除源文件" :feedback="s3MigrationType === 'images' ? '仅在确认上传成功且可访问后删除本地源文件' : ''">
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
                确定要执行迁移吗？此操作会将当前类型的本地资源迁移到 S3。
                <span v-if="s3MigrationDeleteSource">迁移成功且可访问后将删除本地源文件。</span>
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
</style>
