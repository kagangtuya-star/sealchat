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
import dayjs from 'dayjs';

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
const updateStatus = ref<any>(null);
const updateVersionInput = ref('');
const updateLoading = ref(false);
const updateVersionSaving = ref(false);
const updateError = ref('');
const updateBodyExpanded = ref(false);
const serveAtHelp = '选择监听地址并设置端口，保存后需重启；0.0.0.0 对外开放，127.0.0.1 仅本机。';
const baseServeAtHostOptions = [
  { label: '仅本机 (127.0.0.1)', value: '127.0.0.1' },
  { label: '所有网卡 (0.0.0.0)', value: '0.0.0.0' },
];
const serveAtHost = ref('0.0.0.0');
const serveAtPort = ref<number | null>(3212);
const serveAtSyncing = ref(false);
const serveAtHostOptions = computed(() => {
  const options = [...baseServeAtHostOptions];
  if (!options.some((item) => item.value === serveAtHost.value)) {
    options.push({
      label: `当前配置 (${serveAtHost.value})`,
      value: serveAtHost.value,
    });
  }
  return options;
});

const normalizePort = (value: number | null) => {
  if (value === null || Number.isNaN(value)) return null;
  return Math.min(65535, Math.max(1, Math.trunc(value)));
};

const parseServeAt = (value: string) => {
  const trimmed = (value || '').trim();
  let host = '0.0.0.0';
  let port = 3212;
  if (!trimmed) return { host, port };
  if (trimmed.startsWith(':')) {
    const parsedPort = Number.parseInt(trimmed.slice(1), 10);
    if (!Number.isNaN(parsedPort)) {
      port = parsedPort;
    }
    return { host, port };
  }
  const lastColonIndex = trimmed.lastIndexOf(':');
  if (lastColonIndex >= 0) {
    const hostPart = trimmed.slice(0, lastColonIndex).trim();
    const portPart = trimmed.slice(lastColonIndex + 1).trim();
    if (hostPart) host = hostPart;
    const parsedPort = Number.parseInt(portPart, 10);
    if (!Number.isNaN(parsedPort)) {
      port = parsedPort;
    }
    return { host, port };
  }
  return { host: trimmed, port };
};

watch(
  () => model.value.serveAt,
  (value) => {
    const parsed = parseServeAt(value);
    serveAtSyncing.value = true;
    serveAtHost.value = parsed.host;
    serveAtPort.value = parsed.port;
    nextTick(() => {
      serveAtSyncing.value = false;
    });
  },
  { immediate: true },
);

watch([serveAtHost, serveAtPort], ([host, port]) => {
  if (serveAtSyncing.value) return;
  const normalizedPort = normalizePort(port);
  if (!normalizedPort) return;
  const normalizedHost = host || '0.0.0.0';
  const next = `${normalizedHost}:${normalizedPort}`;
  if (next !== model.value.serveAt) {
    model.value.serveAt = next;
  }
});

onMounted(async () => {
  const resp = await utils.configGet();
  model.value = cloneDeep(resp.data);
  nextTick(() => {
    modified.value = false;
  })
  await fetchUpdateStatus();
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

const fetchUpdateStatus = async () => {
  updateLoading.value = true;
  updateError.value = '';
  try {
    const resp = await utils.adminUpdateStatus();
    updateStatus.value = resp.data;
    updateVersionInput.value = updateStatus.value?.currentVersion || '';
  } catch (error) {
    updateError.value = '获取更新状态失败';
  } finally {
    updateLoading.value = false;
  }
};

const triggerUpdateCheck = async () => {
  updateLoading.value = true;
  updateError.value = '';
  try {
    const resp = await utils.adminUpdateCheck();
    updateStatus.value = resp.data;
    updateVersionInput.value = updateStatus.value?.currentVersion || '';
  } catch (error) {
    updateError.value = '检查更新失败';
  } finally {
    updateLoading.value = false;
  }
};

const saveCurrentVersion = async () => {
  const current = (updateVersionInput.value || '').trim();
  if (!current) {
    message.error('请输入当前版本');
    return;
  }
  updateVersionSaving.value = true;
  updateError.value = '';
  try {
    const resp = await utils.adminUpdateVersion(current);
    updateStatus.value = resp.data;
    updateVersionInput.value = updateStatus.value?.currentVersion || current;
    message.success('已更新当前版本');
  } catch (error) {
    updateError.value = '保存当前版本失败';
  } finally {
    updateVersionSaving.value = false;
  }
};

const escapeHtml = (text: string) => {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
};

const formatInline = (text: string) => {
  let result = escapeHtml(text);
  result = result.replace(/`([^`]+)`/g, '<code>$1</code>');
  result = result.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  result = result.replace(/\*([^*]+)\*/g, '<em>$1</em>');
  result = result.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  result = result.replace(/!\[([^\]]*)\]\((https?:\/\/[^\s)]+)\)/g, '<img src="$2" alt="$1" />');
  return result;
};

const renderMarkdown = (text: string) => {
  const lines = (text || '').split(/\r?\n/);
  let html = '';
  let inList = false;
  lines.forEach((raw) => {
    const line = raw.trimEnd();
    if (line.startsWith('- ') || line.startsWith('* ')) {
      if (!inList) {
        html += '<ul>';
        inList = true;
      }
      html += `<li>${formatInline(line.slice(2).trim())}</li>`;
      return;
    }
    if (inList) {
      html += '</ul>';
      inList = false;
    }
    if (line.startsWith('### ')) {
      html += `<h3>${formatInline(line.slice(4).trim())}</h3>`;
      return;
    }
    if (line.startsWith('## ')) {
      html += `<h2>${formatInline(line.slice(3).trim())}</h2>`;
      return;
    }
    if (line.startsWith('# ')) {
      html += `<h1>${formatInline(line.slice(2).trim())}</h1>`;
      return;
    }
    if (line === '') {
      html += '<br />';
      return;
    }
    html += `<p>${formatInline(line)}</p>`;
  });
  if (inList) {
    html += '</ul>';
  }
  return html;
};

const updateBodyRaw = computed(() => (updateStatus.value?.latestBody || '').trim());
const updateBodyHtml = computed(() => renderMarkdown(updateBodyRaw.value));
const toggleUpdateBody = () => {
  updateBodyExpanded.value = !updateBodyExpanded.value;
};
const updatePublishedAtText = computed(() => {
  const ts = updateStatus.value?.latestPublishedAt;
  if (!ts) return '未知';
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss');
});
const updateCheckedAtText = computed(() => {
  const ts = updateStatus.value?.lastCheckedAt;
  if (!ts) return '尚未检查';
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss');
});

watch(updateBodyRaw, (next, prev) => {
  if (next && next !== prev) {
    updateBodyExpanded.value = false;
  }
});

const link = computed(() => {
  return <span class="text-sm font-bold">
    <span>地址 </span>
    <a target="_blank" href={`//${model.value.domain}${model.value.webUrl}`} class="text-blue-500 dark:text-blue-400 hover:underline">{`${model.value.domain}${model.value.webUrl}`}</a>
  </span>
})

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
      <n-form-item label="服务地址" :feedback="serveAtHelp">
        <div class="flex gap-2 items-center w-full">
          <n-select
            v-model:value="serveAtHost"
            :options="serveAtHostOptions"
            placeholder="选择监听地址"
            style="max-width: 240px;"
          />
          <span class="text-gray-500">:</span>
          <n-input-number
            v-model:value="serveAtPort"
            :min="1"
            :max="65535"
            :precision="0"
            placeholder="端口"
            style="max-width: 140px;"
          />
        </div>
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

      <n-divider>版本检测</n-divider>
      <n-form-item label="更新状态">
        <div class="flex flex-col gap-2 w-full">
          <div v-if="updateError" class="text-sm text-red-500">{{ updateError }}</div>
          <div v-else class="text-sm text-gray-600 dark:text-gray-400">
            上次检查：{{ updateCheckedAtText }}
          </div>
          <div class="text-sm text-gray-600 dark:text-gray-400">
            当前版本：{{ updateStatus?.currentVersion || '未知' }}
          </div>
          <div class="flex gap-2 items-center">
            <n-input
              v-model:value="updateVersionInput"
              size="small"
              placeholder="例如 20260102-0362e01"
              style="max-width: 220px;"
            />
            <n-button size="small" @click="saveCurrentVersion" :loading="updateVersionSaving">保存版本</n-button>
            <span class="text-xs text-gray-500">用于已部署实例手动设置当前版本（重启后会被构建版本覆盖）</span>
          </div>
          <div v-if="updateStatus?.latestTag" class="text-sm text-gray-600 dark:text-gray-400">
            最新版本：{{ updateStatus.latestTag }}
          </div>
          <div v-if="updateStatus?.latestName" class="text-sm text-gray-600 dark:text-gray-400">
            版本名称：{{ updateStatus.latestName }}
          </div>
          <div v-if="updateStatus?.latestTag" class="text-sm text-gray-600 dark:text-gray-400">
            发布时间：{{ updatePublishedAtText }}
          </div>
          <div v-if="updateStatus?.latestHtmlUrl" class="text-sm">
            <a :href="updateStatus.latestHtmlUrl" target="_blank" rel="noreferrer">打开发布页</a>
          </div>
          <div class="flex gap-2 items-center">
            <span v-if="updateStatus?.hasUpdate" class="text-xs text-orange-500">有新版本</span>
            <span v-else class="text-xs text-emerald-500">已是最新</span>
            <n-button size="small" @click="triggerUpdateCheck" :loading="updateLoading">检查更新</n-button>
          </div>
          <div v-if="updateBodyRaw" class="flex flex-col gap-2">
            <button
              type="button"
              class="text-xs text-blue-600 dark:text-blue-400 hover:underline self-start"
              @click="toggleUpdateBody"
            >
              {{ updateBodyExpanded ? '收起更新内容' : '展开更新内容' }}
            </button>
            <div
              class="text-sm update-check-body"
              :class="{ 'is-collapsed': !updateBodyExpanded }"
              v-html="updateBodyHtml"
            ></div>
          </div>
        </div>
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

<style scoped>
.update-check-body.is-collapsed {
  max-height: 8rem;
  overflow: hidden;
}

.update-check-body :deep(img) {
  max-width: 100%;
  border-radius: 6px;
  margin-top: 6px;
}

.update-check-body :deep(h1),
.update-check-body :deep(h2),
.update-check-body :deep(h3) {
  margin: 0.5rem 0 0.25rem;
}

.update-check-body :deep(ul) {
  padding-left: 1.1rem;
  margin: 0.35rem 0;
}
</style>
