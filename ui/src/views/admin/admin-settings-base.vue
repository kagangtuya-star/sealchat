<script setup lang="tsx">
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import type { ServerConfig } from '@/types';
import { Message } from '@vicons/tabler';
import { cloneDeep } from 'lodash-es';
import { useMessage } from 'naive-ui';
import { computed, nextTick } from 'vue';
import { onMounted, ref, watch } from 'vue';
import { api } from '@/stores/_config';

const chat = useChatStore();

const model = ref<ServerConfig>({
  serveAt: ':3212',
  domain: '127.0.0.1:3212',
  registerOpen: true,
  // VisitorOpen: true,
  webUrl: '/',
  pageTitle: '海豹尬聊 SealChat',
  chatHistoryPersistentDays: 0,
  imageSizeLimit: 2 * 1024,
  imageCompress: true,
  imageCompressQuality: 85,
  builtInSealBotEnable: true,
  emailNotification: { enabled: false },
})

const utils = useUtilsStore();
const message = useMessage()
const modified = ref(false);

onMounted(async () => {
  const resp = await utils.configGet();
  model.value = cloneDeep(resp.data);
  nextTick(() => {
    modified.value = false;
  })
})

watch(model, (v) => {
  modified.value = true;
}, { deep: true })

const reset = async () => {
  // 重置
  // model.value = {
  //   serveAt: ':3212',
  //   domain: '127.0.0.1:3212',
  //   registerOpen: true,
  //   webUrl: '/test',
  //   chatHistoryPersistentDays: 60,
  //   imageSizeLimit: 2048,
  //   imageCompress: true,
  // }
  // modified.value = true;
}

const emit = defineEmits(['close']);

const cancel = () => {
  emit('close');
}

const save = async () => {
  try {
    await utils.configSet(model.value);
    modified.value = false;
    message.success('保存成功');
  } catch (error) {
    message.error('失败:' + (error as any)?.response?.data?.message || '未知原因')
  }
}

const link = computed(() => {
  return <span class="text-sm font-bold">
    <span>地址 </span>
    <a target="_blank" href={`//${model.value.domain}${model.value.webUrl}`} class="text-blue-500 dark:text-blue-400 hover:underline">{`${model.value.domain}${model.value.webUrl}`}</a>
  </span>
})

const feedbackServeAtShow = ref(false)
const feedbackAdminShow = ref(false)
const feedbackWeburlShow = ref(false)

// Image migration state
const migrationStats = ref<{
  total: number;
  pending: number;
  completed: number;
  failed: number;
  skipped: number;
  spaceSaved: number;
} | null>(null)
const migrationLoading = ref(false)
const migrationExecuting = ref(false)
const migrationBatchSize = ref(100)

const fetchMigrationPreview = async () => {
  migrationLoading.value = true
  try {
    const resp = await api.get('/api/v1/admin/image-migration/preview')
    migrationStats.value = resp.data.stats
  } catch (error) {
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
      dryRun: dryRun
    })
    const stats = resp.data.stats
    if (dryRun) {
      message.success(`模拟迁移完成: ${stats.completed} 张图片可被迁移，预计节省 ${formatBytes(stats.spaceSaved)}`)
    } else {
      message.success(`迁移完成: ${stats.completed} 成功, ${stats.failed} 失败, ${stats.skipped} 跳过，节省 ${formatBytes(stats.spaceSaved)}`)
    }
    // Refresh preview
    await fetchMigrationPreview()
  } catch (error) {
    message.error('执行迁移失败: ' + ((error as any)?.response?.data?.message || '未知错误'))
  } finally {
    migrationExecuting.value = false
  }
}

// S3 migration state
const s3MigrationType = ref<'images' | 'audio'>('images')
const s3MigrationStats = ref<{
  total: number;
  pending: number;
  completed: number;
  failed: number;
  skipped: number;
} | null>(null)
const s3MigrationLoading = ref(false)
const s3MigrationExecuting = ref(false)
const s3MigrationBatchSize = ref(100)
const s3MigrationDeleteSource = ref(true)

watch(s3MigrationType, (v) => {
  s3MigrationDeleteSource.value = v === 'images'
  s3MigrationStats.value = null
})

const fetchS3MigrationPreview = async () => {
  s3MigrationLoading.value = true
  try {
    const resp = await api.get('/api/v1/admin/s3-migration/preview', {
      params: { type: s3MigrationType.value }
    })
    s3MigrationStats.value = resp.data.stats
  } catch (error) {
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
  } catch (error) {
    message.error('执行迁移失败: ' + ((error as any)?.response?.data?.message || '未知错误'))
  } finally {
    s3MigrationExecuting.value = false
  }
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// SMTP test state
const smtpTestEmail = ref('')
const smtpTestLoading = ref(false)
const sendSmtpTestEmail = async () => {
  if (!smtpTestEmail.value || !smtpTestEmail.value.includes('@')) {
    message.error('请填写有效的邮箱地址')
    return
  }
  smtpTestLoading.value = true
  try {
    const resp = await api.post('/api/v1/admin/email-test', { email: smtpTestEmail.value })
    message.success(resp.data?.message || '测试邮件已发送')
  } catch (error: any) {
    message.error(error?.response?.data?.message || '发送失败')
  } finally {
    smtpTestLoading.value = false
  }
}
</script>

<template>
  <div class="overflow-y-auto pr-2" style="max-height: 61vh;  margin-top: 0;">
    <n-form label-placement="left" label-width="auto">
      <n-form-item label="服务地址" :feedback="feedbackServeAtShow ? '慎重填写，重启后生效' : ''">
        <n-input v-model:value="model.serveAt" @focus="feedbackServeAtShow = true" @blur="feedbackServeAtShow = false" />
      </n-form-item>
      <n-form-item label="可访问地址" :feedback="feedbackAdminShow ? link : ''">
        <n-input v-model:value="model.domain" @focus="feedbackAdminShow = true" @blur="feedbackAdminShow = false" />
      </n-form-item>
      <n-form-item label="开放注册">
        <n-switch v-model:value="model.registerOpen" />
      </n-form-item>
      <!-- <n-form-item label="开放游客">
              <n-switch v-model:value="model.VisitorOpen" disabled />
            </n-form-item> -->
      <n-form-item label="子路径设置" :feedback="feedbackWeburlShow ? '慎重填写，重启后生效' : ''">
        <n-input v-model:value="model.webUrl" @focus="feedbackWeburlShow = true" @blur="feedbackWeburlShow = false" />
      </n-form-item>
      <n-form-item label="网页标题" feedback="留空将回退至「海豹尬聊 SealChat」">
        <n-input v-model:value="model.pageTitle" />
      </n-form-item>
      <n-form-item label="可翻阅聊天记录">
        <n-input-number v-model:value="model.chatHistoryPersistentDays" type="number">
          <template #suffix>天</template>
        </n-input-number>
      </n-form-item>
      <n-form-item label="图片大小上限">
        <n-input-number v-model:value="model.imageSizeLimit" type="number">
          <template #suffix>KB</template>
        </n-input-number>
      </n-form-item>
      <n-form-item label="图片上传前压缩">
        <n-switch v-model:value="model.imageCompress" />
      </n-form-item>
      <n-form-item label="压缩质量 (1-100)">
        <n-input-number v-model:value="model.imageCompressQuality" :min="1" :max="100"
          :disabled="!model.imageCompress" />
      </n-form-item>
      <n-form-item label="启用内置小海豹">
        <n-switch v-model:value="model.builtInSealBotEnable" />
      </n-form-item>
      <n-form-item v-if="model.emailNotification" label="启用邮件提醒" feedback="允许用户配置未读消息邮件提醒（需配置 SMTP）">
        <n-switch v-model:value="model.emailNotification.enabled" />
      </n-form-item>
      <n-form-item label="测试 SMTP" feedback="发送测试邮件以验证 SMTP 配置是否正确">
        <div class="flex gap-2 items-center w-full">
          <n-input v-model:value="smtpTestEmail" placeholder="输入测试邮箱" style="max-width: 240px;" />
          <n-button :loading="smtpTestLoading" @click="sendSmtpTestEmail">发送测试</n-button>
        </div>
      </n-form-item>
      <n-form-item label="术语最大字数" feedback="单条术语内容的最大字符数（100-10000）">
        <n-input-number v-model:value="model.keywordMaxLength" :min="100" :max="10000" />
      </n-form-item>

      <!-- Image Migration Section -->
      <n-divider>图片迁移 (WebP)</n-divider>
      <n-form-item label="迁移状态">
        <div class="flex flex-col gap-2 w-full">
          <div v-if="migrationStats" class="text-sm text-gray-600 dark:text-gray-400">
            待迁移: {{ migrationStats.pending }} 张 (不含 GIF 和 S3 图片)
          </div>
          <div class="flex gap-2 items-center">
            <n-button size="small" @click="fetchMigrationPreview" :loading="migrationLoading">
              刷新预览
            </n-button>
          </div>
        </div>
      </n-form-item>
      <n-form-item label="批量大小">
        <n-input-number v-model:value="migrationBatchSize" :min="1" :max="1000" />
      </n-form-item>
      <n-form-item label="执行迁移">
        <div class="flex gap-2">
          <n-button size="small" @click="executeMigration(true)" :loading="migrationExecuting" :disabled="!migrationStats || migrationStats.pending === 0">
            模拟运行
          </n-button>
          <n-popconfirm @positive-click="executeMigration(false)">
            <template #trigger>
              <n-button size="small" type="warning" :loading="migrationExecuting" :disabled="!migrationStats || migrationStats.pending === 0">
                执行迁移
              </n-button>
            </template>
            确定要执行迁移吗？此操作会将 {{ migrationBatchSize }} 张图片转换为 WebP 格式，原文件将被删除。
          </n-popconfirm>
        </div>
      </n-form-item>

      <!-- S3 Migration Section -->
      <n-divider>迁移到 S3</n-divider>
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
            <n-button size="small" @click="fetchS3MigrationPreview" :loading="s3MigrationLoading">
              刷新预览
            </n-button>
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
          <n-button size="small" @click="executeS3Migration(true)" :loading="s3MigrationExecuting" :disabled="!s3MigrationStats || s3MigrationStats.pending === 0">
            模拟运行
          </n-button>
          <n-popconfirm @positive-click="executeS3Migration(false)">
            <template #trigger>
              <n-button size="small" type="warning" :loading="s3MigrationExecuting" :disabled="!s3MigrationStats || s3MigrationStats.pending === 0">
                执行迁移
              </n-button>
            </template>
            确定要执行迁移吗？此操作会将当前类型的本地资源迁移到 S3。
            <span v-if="s3MigrationDeleteSource">迁移成功且可访问后将删除本地源文件。</span>
          </n-popconfirm>
        </div>
      </n-form-item>
    </n-form>
  </div>
  <div class="space-x-2 float-right">
    <n-button @click="cancel">关闭</n-button>
    <n-button type="primary" :disabled="!modified" @click="save">保存</n-button>
  </div>
</template>
