<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { NAlert, NButton, NCard, NCollapse, NCollapseItem, NInput, NInputNumber, NSlider, NSpace, NSwitch, useMessage } from 'naive-ui';
import { api } from '@/stores/_config';

const SMTP_STORAGE_KEY = 'sealchat_email_smtp_config';

interface SmtpConfig {
  smtpHost: string;
  smtpPort: number;
  smtpUsername: string;
  smtpFromAddress: string;
  smtpFromName: string;
  smtpUseTls: boolean;
}

interface EmailNotificationSettings {
  enabled: boolean;
  email: string;
  delayMinutes: number;
  minDelay?: number;
  maxDelay?: number;
  featureDisabled?: boolean;
  message?: string;
  useCustomSmtp?: boolean;
  smtpHost?: string;
  smtpPort?: number;
  smtpUsername?: string;
  smtpFromAddress?: string;
  smtpFromName?: string;
  smtpUseTls?: boolean;
  hasPassword?: boolean;
}

const props = defineProps<{
  channelId: string;
}>();

const message = useMessage();
const loading = ref(false);
const errorText = ref('');
const featureDisabled = ref(false);
const smtpPassword = ref('');

const settings = ref<EmailNotificationSettings>({
  enabled: false,
  email: '',
  delayMinutes: 10,
  minDelay: 10,
  maxDelay: 30,
  useCustomSmtp: false,
  smtpHost: '',
  smtpPort: 587,
  smtpUsername: '',
  smtpFromAddress: '',
  smtpFromName: '',
  smtpUseTls: true,
  hasPassword: false,
});

const hasChannel = computed(() => !!props.channelId && props.channelId.trim().length > 0);

// localStorage 保存/读取 SMTP 配置（跨频道共享）
const saveSmtpToLocalStorage = () => {
  try {
    const smtpConfig: SmtpConfig = {
      smtpHost: settings.value.smtpHost || '',
      smtpPort: settings.value.smtpPort || 587,
      smtpUsername: settings.value.smtpUsername || '',
      smtpFromAddress: settings.value.smtpFromAddress || '',
      smtpFromName: settings.value.smtpFromName || '',
      smtpUseTls: settings.value.smtpUseTls ?? true,
    };
    localStorage.setItem(SMTP_STORAGE_KEY, JSON.stringify(smtpConfig));
  } catch (e) {
    // ignore storage errors
  }
};

const loadSmtpFromLocalStorage = (): SmtpConfig | null => {
  try {
    const stored = localStorage.getItem(SMTP_STORAGE_KEY);
    if (stored) {
      return JSON.parse(stored) as SmtpConfig;
    }
  } catch (e) {
    // ignore parse errors
  }
  return null;
};

const applyLocalStorageSmtp = () => {
  const cached = loadSmtpFromLocalStorage();
  if (cached) {
    settings.value.smtpHost = cached.smtpHost || settings.value.smtpHost;
    settings.value.smtpPort = cached.smtpPort || settings.value.smtpPort;
    settings.value.smtpUsername = cached.smtpUsername || settings.value.smtpUsername;
    settings.value.smtpFromAddress = cached.smtpFromAddress || settings.value.smtpFromAddress;
    settings.value.smtpFromName = cached.smtpFromName || settings.value.smtpFromName;
    settings.value.smtpUseTls = cached.smtpUseTls ?? settings.value.smtpUseTls;
  }
};

const refresh = async () => {
  if (!hasChannel.value) return;
  loading.value = true;
  errorText.value = '';
  smtpPassword.value = '';
  try {
    const resp = await api.get<EmailNotificationSettings>(`/api/v1/channels/${props.channelId}/email-notification`);
    const data = resp.data;
    if (data.featureDisabled) {
      featureDisabled.value = true;
      settings.value.enabled = false;
      errorText.value = data.message || '邮件通知功能未启用';
    } else {
      featureDisabled.value = false;
      settings.value = { ...settings.value, ...data };
      // 如果服务端没有 SMTP 配置，从 localStorage 加载
      if (!data.smtpHost) {
        applyLocalStorageSmtp();
      }
    }
  } catch (e: any) {
    errorText.value = e?.response?.data?.message || e?.message || '加载失败';
  } finally {
    loading.value = false;
  }
};

const saveSettings = async () => {
  if (!hasChannel.value) return;
  if (settings.value.enabled && !settings.value.email.includes('@')) {
    errorText.value = '请填写有效的邮箱地址';
    return;
  }
  if (settings.value.useCustomSmtp && !settings.value.smtpHost) {
    errorText.value = '使用自定义 SMTP 时请填写服务器地址';
    return;
  }
  loading.value = true;
  errorText.value = '';
  try {
    const resp = await api.post<EmailNotificationSettings>(`/api/v1/channels/${props.channelId}/email-notification`, {
      enabled: settings.value.enabled,
      email: settings.value.email,
      delayMinutes: settings.value.delayMinutes,
      useCustomSmtp: settings.value.useCustomSmtp,
      smtpHost: settings.value.smtpHost,
      smtpPort: settings.value.smtpPort || 587,
      smtpUsername: settings.value.smtpUsername,
      smtpPassword: smtpPassword.value,
      smtpFromAddress: settings.value.smtpFromAddress,
      smtpFromName: settings.value.smtpFromName,
      smtpUseTls: settings.value.smtpUseTls,
    });
    settings.value = { ...settings.value, ...resp.data };
    smtpPassword.value = '';
    // 保存 SMTP 配置到 localStorage（跨频道共享）
    if (settings.value.useCustomSmtp && settings.value.smtpHost) {
      saveSmtpToLocalStorage();
    }
    message.success('保存成功');
  } catch (e: any) {
    errorText.value = e?.response?.data?.message || e?.message || '保存失败';
  } finally {
    loading.value = false;
  }
};

const testEmail = async () => {
  if (!hasChannel.value) {
    errorText.value = '缺少频道ID';
    return;
  }
  if (!settings.value.email.includes('@')) {
    errorText.value = '请先填写有效的邮箱地址';
    return;
  }
  if (settings.value.useCustomSmtp && !settings.value.smtpHost) {
    errorText.value = '使用自定义 SMTP 时请填写服务器地址';
    return;
  }
  loading.value = true;
  errorText.value = '';
  try {
    const payload: any = {
      channelId: props.channelId,
    };
    // 如果使用自定义 SMTP，传递配置
    if (settings.value.useCustomSmtp) {
      payload.useCustomSmtp = true;
      payload.smtpHost = settings.value.smtpHost;
      payload.smtpPort = settings.value.smtpPort || 587;
      payload.smtpUsername = settings.value.smtpUsername;
      payload.smtpPassword = smtpPassword.value; // 使用当前输入的密码
      payload.smtpFromAddress = settings.value.smtpFromAddress;
      payload.smtpFromName = settings.value.smtpFromName;
      payload.smtpUseTls = settings.value.smtpUseTls;
    }
    await api.post('/api/v1/email-notification/test', payload);
    message.success('测试邮件已发送，请检查收件箱');
  } catch (e: any) {
    errorText.value = e?.response?.data?.message || e?.message || '发送失败';
  } finally {
    loading.value = false;
  }
};

watch(() => props.channelId, refresh, { immediate: true });
onMounted(refresh);
</script>

<template>
  <div class="p-3">
    <n-alert v-if="featureDisabled" type="warning" :bordered="false" class="mb-3">
      邮件通知功能未由管理员启用。请联系管理员在服务器配置中启用此功能。
    </n-alert>

    <n-alert v-else-if="errorText" type="error" :bordered="false" class="mb-3">
      {{ errorText }}
    </n-alert>

    <n-card v-if="!featureDisabled" title="邮件提醒设置" size="small">
      <n-space vertical size="medium">
        <!-- 启用开关 -->
        <div class="flex items-center justify-between">
          <span class="text-sm">启用邮件提醒</span>
          <n-switch v-model:value="settings.enabled" :disabled="loading" />
        </div>

        <!-- 邮箱地址 -->
        <div>
          <div class="text-sm mb-1">接收邮箱</div>
          <n-input
            v-model:value="settings.email"
            placeholder="your@email.com"
            :disabled="loading"
          />
        </div>

        <!-- 延迟时间 -->
        <div>
          <div class="text-sm mb-1">
            延迟推送时间：{{ settings.delayMinutes }} 分钟
          </div>
          <div class="text-xs text-gray-500 mb-2">
            消息在该时间内未被阅读时，将发送邮件提醒
          </div>
          <n-slider
            v-model:value="settings.delayMinutes"
            :min="settings.minDelay || 10"
            :max="settings.maxDelay || 30"
            :step="1"
            :disabled="loading"
            :marks="{ [settings.minDelay || 10]: `${settings.minDelay || 10}分钟`, [settings.maxDelay || 30]: `${settings.maxDelay || 30}分钟` }"
          />
        </div>

        <!-- 自定义 SMTP 配置 -->
        <n-collapse>
          <n-collapse-item title="自定义 SMTP 服务器（可选）" name="smtp">
            <n-space vertical size="small">
              <div class="flex items-center justify-between">
                <span class="text-sm">使用自定义 SMTP</span>
                <n-switch v-model:value="settings.useCustomSmtp" :disabled="loading" />
              </div>

              <template v-if="settings.useCustomSmtp">
                <div>
                  <div class="text-xs mb-1">SMTP 服务器地址</div>
                  <n-input v-model:value="settings.smtpHost" placeholder="smtp.example.com" :disabled="loading" />
                </div>
                <div>
                  <div class="text-xs mb-1">端口</div>
                  <n-input-number v-model:value="settings.smtpPort" :min="1" :max="65535" placeholder="587" :disabled="loading" style="width: 100%;" />
                </div>
                <div>
                  <div class="text-xs mb-1">用户名</div>
                  <n-input v-model:value="settings.smtpUsername" placeholder="your@email.com" :disabled="loading" />
                </div>
                <div>
                  <div class="text-xs mb-1">密码 {{ settings.hasPassword ? '（已设置，留空保持不变）' : '' }}</div>
                  <n-input v-model:value="smtpPassword" type="password" placeholder="SMTP 密码" :disabled="loading" />
                </div>
                <div>
                  <div class="text-xs mb-1">发件人地址</div>
                  <n-input v-model:value="settings.smtpFromAddress" placeholder="noreply@example.com" :disabled="loading" />
                </div>
                <div>
                  <div class="text-xs mb-1">发件人名称</div>
                  <n-input v-model:value="settings.smtpFromName" placeholder="SealChat" :disabled="loading" />
                </div>
                <div class="flex items-center justify-between">
                  <span class="text-xs">使用 TLS 加密</span>
                  <n-switch v-model:value="settings.smtpUseTls" :disabled="loading" size="small" />
                </div>
              </template>

              <n-alert v-if="!settings.useCustomSmtp" type="info" :bordered="false">
                <div class="text-xs">不填写自定义 SMTP 时，将使用系统管理员配置的默认邮件服务器。</div>
              </n-alert>
            </n-space>
          </n-collapse-item>
        </n-collapse>

        <!-- 操作按钮 -->
        <n-space justify="end" class="mt-3">
          <n-button :loading="loading" :disabled="!settings.email" @click="testEmail">
            发送测试邮件
          </n-button>
          <n-button type="primary" :loading="loading" @click="saveSettings">
            保存设置
          </n-button>
        </n-space>

        <!-- 说明 -->
        <n-alert type="info" :bordered="false" class="mt-3">
          <div class="text-xs">
            <strong>工作原理：</strong>
            <ul class="list-disc ml-4 mt-1">
              <li>当有新消息且您未在线阅读时，系统会在设定的延迟时间后发送邮件提醒</li>
              <li>邮件中包含未读消息的摘要信息</li>
              <li>每小时最多发送一定数量的提醒邮件，避免打扰</li>
            </ul>
          </div>
        </n-alert>
      </n-space>
    </n-card>
  </div>
</template>
