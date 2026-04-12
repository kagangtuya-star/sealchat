<script setup lang="tsx">
import { useUtilsStore } from '@/stores/utils';
import type { ServerConfig } from '@/types';
import { cloneDeep } from 'lodash-es';
import { useMessage } from 'naive-ui';
import { computed, nextTick } from 'vue';
import { onMounted, ref, watch } from 'vue';
import { api } from '@/stores/_config';
import dayjs from 'dayjs';

const model = ref<ServerConfig>({
  serveAt: ':3212',
  domain: '127.0.0.1:3212',
  registerOpen: true,
  // VisitorOpen: true,
  webUrl: '/',
  pageTitle: '海豹尬聊 SealChat',
  chatHistoryPersistentDays: 0,
  messageSortBasis: 'typing_start',
  imageSizeLimit: 2 * 1024,
  imageCompress: true,
  imageCompressQuality: 85,
  builtInSealBotEnable: true,
  emailNotification: { enabled: false },
  audio: { allowWorldAudioWorkbench: false, allowNonAdminCreateWorld: true },
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
const serveAtHelp = '选择监听地址并设置端口，保存后需重启；0.0.0.0 对外开放，127.0.0.1 仅本机；IPv6 可填 :: 或 ::1，保存时自动补全中括号。';
const baseServeAtHostOptions = [
  { label: '仅本机 (127.0.0.1)', value: '127.0.0.1' },
  { label: '所有网卡 (0.0.0.0)', value: '0.0.0.0' },
  { label: '仅本机 (::1)', value: '::1' },
  { label: '所有网卡 (::)', value: '::' },
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

const stripHostBrackets = (value: string) => {
  const trimmed = value.trim();
  if (trimmed.startsWith('[')) {
    const end = trimmed.indexOf(']');
    if (end >= 0) {
      return trimmed.slice(1, end);
    }
  }
  return trimmed;
};

const normalizeHostForServeAt = (value: string) => {
  const trimmed = stripHostBrackets(value);
  if (!trimmed) return '';
  if (trimmed.includes(':')) {
    return `[${trimmed}]`;
  }
  return trimmed;
};

const parseServeAt = (value: string) => {
  const trimmed = (value || '').trim();
  let host = '0.0.0.0';
  let port = 3212;
  if (!trimmed) return { host, port };
  if (trimmed.startsWith('[')) {
    const end = trimmed.indexOf(']');
    if (end >= 0) {
      const hostPart = trimmed.slice(1, end).trim();
      if (hostPart) host = hostPart;
      const rest = trimmed.slice(end + 1).trim();
      if (rest.startsWith(':')) {
        const parsedPort = Number.parseInt(rest.slice(1), 10);
        if (!Number.isNaN(parsedPort)) {
          port = parsedPort;
        }
      }
      return { host, port };
    }
  }
  if (trimmed.startsWith(':') && trimmed.indexOf(':', 1) === -1) {
    const parsedPort = Number.parseInt(trimmed.slice(1), 10);
    if (!Number.isNaN(parsedPort)) {
      port = parsedPort;
    }
    return { host, port };
  }
  const colonCount = (trimmed.match(/:/g) || []).length;
  if (colonCount >= 2) {
    const lastColonIndex = trimmed.lastIndexOf(':');
    const hostPart = trimmed.slice(0, lastColonIndex).trim();
    const portPart = trimmed.slice(lastColonIndex + 1).trim();
    if (hostPart && !hostPart.endsWith(':') && /^\d+$/.test(portPart)) {
      const parsedPort = Number.parseInt(portPart, 10);
      if (!Number.isNaN(parsedPort)) {
        port = parsedPort;
        host = hostPart;
        return { host, port };
      }
    }
    return { host: trimmed, port };
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
  const normalizedHost = normalizeHostForServeAt(host || '0.0.0.0');
  const next = normalizedHost ? `${normalizedHost}:${normalizedPort}` : `:${normalizedPort}`;
  if (next !== model.value.serveAt) {
    model.value.serveAt = next;
  }
});

onMounted(async () => {
  const resp = await utils.configGet();
  model.value = cloneDeep(resp.data);
  if (!model.value.audio) {
    model.value.audio = { allowWorldAudioWorkbench: false, allowNonAdminCreateWorld: true };
  }
  if (model.value.messageSortBasis !== 'send_time' && model.value.messageSortBasis !== 'typing_start') {
    model.value.messageSortBasis = 'typing_start';
  }
  if (model.value.audio.allowNonAdminCreateWorld === undefined) {
    model.value.audio.allowNonAdminCreateWorld = true;
  }
  nextTick(() => {
    modified.value = false;
  })
  await fetchUpdateStatus();
})

watch(model, (v) => {
  modified.value = true;
}, { deep: true })

const applyBasicSettingsToPayload = (payload: ServerConfig) => {
  payload.serveAt = model.value.serveAt;
  payload.domain = model.value.domain;
  payload.registerOpen = model.value.registerOpen;
  payload.webUrl = model.value.webUrl;
  payload.pageTitle = model.value.pageTitle;
  payload.chatHistoryPersistentDays = model.value.chatHistoryPersistentDays;
  payload.messageSortBasis = model.value.messageSortBasis;
  payload.imageSizeLimit = model.value.imageSizeLimit;
  payload.imageCompress = model.value.imageCompress;
  payload.imageCompressQuality = model.value.imageCompressQuality;
  payload.builtInSealBotEnable = model.value.builtInSealBotEnable;
  payload.keywordMaxLength = model.value.keywordMaxLength;
  payload.emailNotification = {
    ...(payload.emailNotification || {}),
    ...(model.value.emailNotification || {}),
    enabled: model.value.emailNotification?.enabled ?? false,
  };
  payload.audio = {
    ...(payload.audio || {}),
    ...(model.value.audio || {}),
    allowWorldAudioWorkbench: model.value.audio?.allowWorldAudioWorkbench ?? false,
    allowNonAdminCreateWorld: model.value.audio?.allowNonAdminCreateWorld ?? true,
  };
}

const save = async () => {
  try {
    const resp = await utils.configGet();
    const payload = cloneDeep(resp.data as ServerConfig);
    applyBasicSettingsToPayload(payload);
    await utils.configSet(payload);
    model.value = cloneDeep(payload);
    if (model.value.messageSortBasis !== 'send_time' && model.value.messageSortBasis !== 'typing_start') {
      model.value.messageSortBasis = 'typing_start';
    }
    if (!model.value.audio) {
      model.value.audio = { allowWorldAudioWorkbench: false, allowNonAdminCreateWorld: true };
    }
    if (model.value.audio.allowNonAdminCreateWorld === undefined) {
      model.value.audio.allowNonAdminCreateWorld = true;
    }
    modified.value = false;
    message.success('保存成功');
  } catch (error) {
    message.error('失败:' + (error as any)?.response?.data?.message || '未知原因')
  }
}

defineExpose({
  save,
  isModified: () => modified.value,
})

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
  <div class="admin-settings-scroll overflow-y-auto pr-2" style="max-height: 61vh;  margin-top: 0;">
    <n-form label-placement="left" label-width="120">
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
      <n-form-item label="消息排序方式" feedback="仅影响新发送消息的默认排序依据；手动拖拽预览和插入定位优先级更高。">
        <n-radio-group v-model:value="model.messageSortBasis">
          <n-space>
            <n-radio-button value="typing_start">开始输入时间戳</n-radio-button>
            <n-radio-button value="send_time">发送时间戳</n-radio-button>
          </n-space>
        </n-radio-group>
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
      <n-form-item v-if="model.audio" label="允许世界管理员使用音频工作台" feedback="开启后世界主/管理员可上传和管理世界级音频">
        <n-switch v-model:value="model.audio.allowWorldAudioWorkbench" />
      </n-form-item>
      <n-form-item v-if="model.audio" label="允许非平台管理员创建新世界" feedback="关闭后仅平台管理员可创建世界">
        <n-switch v-model:value="model.audio.allowNonAdminCreateWorld" />
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

    </n-form>
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

.admin-settings-scroll {
  overflow-x: hidden;
  overflow-y: scroll;
  scrollbar-gutter: stable;
}
</style>
