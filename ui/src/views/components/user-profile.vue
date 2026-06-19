<script lang="tsx" setup>
import { useAIStore } from '@/stores/ai';
import { useUserStore } from '@/stores/user';
import { useUtilsStore } from '@/stores/utils';
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import Avatar from '@/components/avatar.vue'
import AvatarEditor from '@/components/AvatarEditor.vue'
import { api, urlBase } from '@/stores/_config';
import { NIcon, useMessage } from 'naive-ui';
import { useI18n } from 'vue-i18n'
import router from '@/router';
import type { AIRunSource, ServerConfig, UserAIProviderProfile } from '@/types';
import { useCapWidget } from '@/composables/useCapWidget';
import { Refresh } from '@vicons/tabler';

declare global {
  interface Window {
    turnstile?: {
      render: (container: HTMLElement | string, options: Record<string, any>) => string;
      reset: (widgetId?: string) => void;
      remove: (widgetId?: string) => void;
    };
  }
}

let turnstileScriptPromise: Promise<void> | null = null;

const { t } = useI18n()

const user = useUserStore();
const utils = useUtilsStore();
const aiStore = useAIStore();
const message = useMessage()

const model = ref({
  nickname: '',
  brief: '',
})

// Avatar editing state
const avatarFile = ref<File | null>(null);
const showEditor = ref(false);
const inputFileRef = ref<HTMLInputElement>()

// Email binding state
const CAPTCHA_SCENE = 'signup';
const config = ref<ServerConfig | null>(null);
const emailAuthEnabled = computed(() => config.value?.emailAuth?.enabled ?? false);
const captchaMode = computed(() => config.value?.captcha?.signup?.mode ?? config.value?.captcha?.mode ?? 'off');
const showEmailBind = ref(false);
const emailBindForm = ref({ email: '', code: '' });
const emailBindSubmitting = ref(false);
const emailCodeSending = ref(false);
const emailCodeCountdown = ref(0);
let emailCodeTimer: ReturnType<typeof setInterval> | null = null;
const lastEmailForCode = ref('');
const aiSettingsVisible = ref(false);
const aiSettingsSaving = ref(false);
const aiSettingsSource = ref<AIRunSource>('platform');
const aiProfileDrafts = ref<UserAIProviderProfile[]>([]);
const aiProfileRefreshing = ref<Record<string, boolean>>({});

// Captcha state
const captchaId = ref('');
const captchaInput = ref('');
const captchaImageSeed = ref(0);
const captchaLoading = ref(false);
const captchaError = ref('');
const captchaVerified = ref(false);
const turnstileToken = ref('');
const turnstileContainer = ref<HTMLDivElement | null>(null);
const turnstileWidgetId = ref<string | null>(null);
const turnstileError = ref('');
const turnstileLoading = ref(false);
const {
  container: capContainer,
  token: capToken,
  error: capError,
  loading: capLoading,
  render: renderCapWidget,
  reset: resetCapWidget,
  destroy: destroyCapWidget,
} = useCapWidget(CAPTCHA_SCENE);

const captchaImageUrl = computed(() => {
  if (!captchaId.value) return '';
  return `${urlBase}/api/v1/captcha/${captchaId.value}.png?scene=${CAPTCHA_SCENE}&ts=${captchaImageSeed.value}`;
});

const emailPattern = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
const shouldForceCaptchaRetry = (errMsg: string) => {
  if (!errMsg) {
    return false;
  }
  return ['请完成验证码验证', '请完成人机验证', '人机验证失败', '验证码错误', '验证码验证失败'].some((keyword) => errMsg.includes(keyword));
};

onMounted(async () => {
  await user.infoUpdate();
  model.value.nickname = user.info.nick;
  model.value.brief = user.info.brief;
  aiSettingsSource.value = aiStore.currentSource;

  try {
    const resp = await utils.configGet();
    config.value = resp.data;
  } catch (err) {
    console.error('Failed to load config:', err);
  }
})

const selectFile = async function () {
  let input = inputFileRef.value
  if (input) {
    input.value = ''
  }
  inputFileRef.value?.click()
}

const onFileChange = async (e: any) => {
  let files = e.target.files || e.dataTransfer.files
  if (!files.length) return
  const file = files[0]
  if (file.size > utils.fileSizeLimit) {
    const limitMB = (utils.fileSizeLimit / 1024 / 1024).toFixed(1)
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`)
    return
  }
  avatarFile.value = file
  showEditor.value = true
}

const handleEditorSave = async (file: File) => {
  try {
    const formData = new FormData();
    formData.append('file', file, file.name);

    const resp = await api.post('/api/v1/upload', formData, {
      headers: {
        Authorization: `${user.token}`,
        ChannelId: 'user-avatar',
      },
    });

    if (resp.status === 200) {
      const attachmentId = resp.data?.ids?.[0];
      if (!attachmentId) {
        message.error('上传失败，未返回附件ID');
        return;
      }
      message.success('头像修改成功!')
      user.info.avatar = `id:${attachmentId}`
    } else {
      message.error('上传失败，请重新尝试')
      console.error('上传失败！', resp);
    }
  } catch (error) {
    message.error('出错了，请刷新重试或联系管理员: ' + (error as any).toString())
    console.error('上传出错！', error);
  } finally {
    showEditor.value = false
    avatarFile.value = null
  }
}

const handleEditorCancel = () => {
  showEditor.value = false
  avatarFile.value = null
}

const emit = defineEmits(['close'])

const save = async () => {
  try {
    if (!model.value.nickname.trim()) {
      message.error('昵称不能为空')
      return
    }
    if (/\s/.test(model.value.nickname)) {
      message.error('昵称中间不能存在空格')
      return
    }

    await user.changeInfo({
      nick: model.value.nickname,
      brief: model.value.brief,
    });
    message.success('修改成功')
    user.info.nick = model.value.nickname
    user.info.brief = model.value.brief
    emit('close')
  } catch (error: any) {
    let msg = error.response?.data?.message;
    if (msg) {
      message.error('出错: ' + msg)
      return
    }
    message.error('修改失败: ' + (error as any).toString())
  }
}

const passwordChange = () => {
  router.push({ name: 'user-password-reset' })
}

const createAIProfileDraft = (): UserAIProviderProfile => ({
  id: `user-ai-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
  name: '',
  enabled: true,
  baseUrl: 'https://api.deepseek.com/v1',
  apiKey: '',
  models: ['deepseek-v4-flash'],
  selectedModel: 'deepseek-v4-flash',
  hasApiKey: false,
})

const cloneAIProfile = (profile: UserAIProviderProfile): UserAIProviderProfile => ({
  id: String(profile.id || '').trim(),
  name: String(profile.name || '').trim(),
  enabled: profile.enabled !== false,
  baseUrl: String(profile.baseUrl || '').trim(),
  apiKey: profile.apiKey || '',
  models: Array.isArray(profile.models) ? profile.models.map((item) => String(item || '').trim()).filter(Boolean) : [],
  selectedModel: String(profile.selectedModel || '').trim(),
  hasApiKey: profile.hasApiKey === true,
})

const formatModelsInput = (profile: UserAIProviderProfile) => (Array.isArray(profile.models) ? profile.models.join(', ') : '')

const profileModelOptions = (profile: UserAIProviderProfile) => {
  const seen = new Set<string>()
  return (Array.isArray(profile.models) ? profile.models : [])
    .map((item) => String(item || '').trim())
    .filter((item) => {
      if (!item || seen.has(item)) return false
      seen.add(item)
      return true
    })
    .map((item) => ({ label: item, value: item }))
}

const updateProfileModels = (profile: UserAIProviderProfile, value: string) => {
  profile.models = value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
  if (!String(profile.selectedModel || '').trim() && profile.models.length > 0) {
    profile.selectedModel = profile.models[0]
  }
}

const updateProfileSelectedModel = (profile: UserAIProviderProfile, value: string) => {
  profile.selectedModel = String(value || '').trim()
  if (profile.selectedModel && !profile.models.includes(profile.selectedModel)) {
    profile.models = [...profile.models, profile.selectedModel]
  }
}

const refreshAIProfileModels = async (profile: UserAIProviderProfile) => {
  const profileId = String(profile.id || '').trim()
  if (!profileId) return
  aiProfileRefreshing.value = {
    ...aiProfileRefreshing.value,
    [profileId]: true,
  }
  try {
    const models = await aiStore.discoverUserProfileModels(profile.baseUrl, profile.apiKey || '')
    profile.models = models
    if (!String(profile.selectedModel || '').trim() && models.length > 0) {
      profile.selectedModel = models[0]
    }
    message.success(`已刷新 ${models.length} 个模型`)
  } catch (error: any) {
    message.error(error?.message || '刷新模型列表失败')
  } finally {
    aiProfileRefreshing.value = {
      ...aiProfileRefreshing.value,
      [profileId]: false,
    }
  }
}

const openAISettings = async () => {
  aiSettingsVisible.value = true
  aiSettingsSource.value = aiStore.currentSource
  try {
    const items = await aiStore.loadUserProfiles()
    aiProfileDrafts.value = items.map(cloneAIProfile)
  } catch (error: any) {
    aiProfileDrafts.value = []
    message.error(error?.response?.data?.message || error?.message || '加载 AI 设置失败')
  }
}

const addAIProfile = () => {
  aiProfileDrafts.value.push(createAIProfileDraft())
}

const removeAIProfile = (profileId: string) => {
  aiProfileDrafts.value = aiProfileDrafts.value.filter((item) => item.id !== profileId)
}

const saveAISettings = async () => {
  aiSettingsSaving.value = true
  try {
    const normalized = aiProfileDrafts.value.map(cloneAIProfile)
    await aiStore.saveUserProfiles(normalized)
    aiStore.setSource(aiSettingsSource.value)
    aiSettingsVisible.value = false
    message.success('AI 设置已保存')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存 AI 设置失败')
  } finally {
    aiSettingsSaving.value = false
  }
}

// Captcha functions
const fetchCaptcha = async () => {
  if (captchaMode.value !== 'local') return;
  captchaLoading.value = true;
  captchaError.value = '';
  try {
    const resp = await api.get<{ id: string }>('api/v1/captcha/new', { params: { scene: CAPTCHA_SCENE } });
    captchaId.value = resp.data.id;
    captchaInput.value = '';
    captchaImageSeed.value = Date.now();
  } catch (err) {
    console.error('Failed to load captcha:', err);
    captchaError.value = '验证码加载失败，请稍后重试';
  } finally {
    captchaLoading.value = false;
  }
};

const reloadCaptchaImage = async () => {
  if (captchaMode.value !== 'local') return;
  if (!captchaId.value) {
    await fetchCaptcha();
    return;
  }
  captchaLoading.value = true;
  captchaError.value = '';
  try {
    await api.get(`api/v1/captcha/${captchaId.value}/reload`, { params: { scene: CAPTCHA_SCENE } });
    captchaImageSeed.value = Date.now();
    captchaInput.value = '';
  } catch (err) {
    console.error('Failed to reload captcha:', err);
    captchaError.value = '验证码刷新失败，已重新生成';
    await fetchCaptcha();
  } finally {
    captchaLoading.value = false;
  }
};

const ensureTurnstileScript = async () => {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return;
  }
  if (window.turnstile) {
    return;
  }
  if (!turnstileScriptPromise) {
    turnstileScriptPromise = new Promise<void>((resolve, reject) => {
      const existing = document.getElementById('cf-turnstile-script') as HTMLScriptElement | null;
      if (existing) {
        existing.addEventListener('load', () => resolve(), { once: true });
        existing.addEventListener('error', () => reject(new Error('Turnstile script load failed')), { once: true });
        return;
      }
      const script = document.createElement('script');
      script.id = 'cf-turnstile-script';
      script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js';
      script.async = true;
      script.defer = true;
      script.onload = () => resolve();
      script.onerror = () => reject(new Error('Turnstile script load failed'));
      document.head.appendChild(script);
    }).catch((err) => {
      turnstileScriptPromise = null;
      throw err;
    });
  }
  await turnstileScriptPromise;
};

const destroyTurnstile = () => {
  if (typeof window !== 'undefined' && turnstileWidgetId.value && window.turnstile?.remove) {
    window.turnstile.remove(turnstileWidgetId.value);
  }
  turnstileWidgetId.value = null;
  turnstileToken.value = '';
  turnstileError.value = '';
  turnstileLoading.value = false;
  if (turnstileContainer.value) {
    turnstileContainer.value.innerHTML = '';
  }
};

const renderTurnstileWidget = async () => {
  if (typeof window === 'undefined') {
    return;
  }
  turnstileError.value = '';
  turnstileLoading.value = true;
  try {
    await ensureTurnstileScript();
    await nextTick();
    const siteKey = config.value?.captcha?.signup?.turnstile?.siteKey?.trim()
      || config.value?.captcha?.turnstile?.siteKey?.trim();
    if (!siteKey) {
      turnstileError.value = '未配置 Turnstile siteKey';
      return;
    }
    if (!turnstileContainer.value || !window.turnstile) {
      turnstileError.value = 'Turnstile 初始化失败';
      return;
    }
    if (turnstileWidgetId.value && window.turnstile.remove) {
      window.turnstile.remove(turnstileWidgetId.value);
    }
    turnstileToken.value = '';
    turnstileWidgetId.value = window.turnstile.render(turnstileContainer.value, {
      sitekey: siteKey,
      callback: (token: string) => {
        turnstileToken.value = token;
        turnstileError.value = '';
      },
      'error-callback': () => {
        turnstileToken.value = '';
        turnstileError.value = '人机验证加载失败，请重试';
      },
      'expired-callback': () => {
        turnstileToken.value = '';
      },
    });
  } catch (err) {
    console.error('Failed to load turnstile:', err);
    turnstileError.value = '无法加载 Turnstile，请稍后重试';
  } finally {
    turnstileLoading.value = false;
  }
};

const resetCaptchaState = () => {
  captchaId.value = '';
  captchaInput.value = '';
  captchaImageSeed.value = 0;
  captchaError.value = '';
  captchaLoading.value = false;
  captchaVerified.value = false;
  destroyCapWidget();
  destroyTurnstile();
};

const resetEmailBindState = () => {
  emailBindForm.value = { email: '', code: '' };
  emailBindSubmitting.value = false;
  if (emailCodeTimer) {
    clearInterval(emailCodeTimer);
    emailCodeTimer = null;
  }
  emailCodeSending.value = false;
  emailCodeCountdown.value = 0;
  lastEmailForCode.value = '';
  resetCaptchaState();
};

const openEmailBind = () => {
  resetEmailBindState();
  showEmailBind.value = true;
  if (captchaMode.value === 'local') {
    fetchCaptcha();
  } else if (captchaMode.value === 'turnstile') {
    nextTick().then(() => renderTurnstileWidget());
  } else if (captchaMode.value === 'cap') {
    nextTick().then(() => renderCapWidget());
  }
};

const sendBindEmailCode = async () => {
  if (emailCodeSending.value || emailCodeCountdown.value > 0) return;

  const email = emailBindForm.value.email.trim().toLowerCase();
  if (!email || !emailPattern.test(email)) {
    message.error('请输入有效的邮箱地址');
    return;
  }

  if (lastEmailForCode.value && lastEmailForCode.value !== email) {
    captchaVerified.value = false;
  }

  if (!captchaVerified.value && captchaMode.value === 'local') {
    if (!captchaId.value) {
      await fetchCaptcha();
      message.error('验证码加载中，请稍后再试');
      return;
    }
    if (!captchaInput.value.trim()) {
      message.error('请输入验证码');
      return;
    }
  } else if (!captchaVerified.value && captchaMode.value === 'turnstile' && !turnstileToken.value) {
    message.error('请先完成人机验证');
    return;
  } else if (!captchaVerified.value && captchaMode.value === 'cap' && !capToken.value) {
    message.error('请先完成验证码验证');
    return;
  }

  emailCodeSending.value = true;
  try {
    await user.sendBindEmailCode({
      email,
      captchaId: captchaVerified.value ? '' : captchaId.value,
      captchaValue: captchaVerified.value ? '' : captchaInput.value.trim(),
      turnstileToken: captchaVerified.value ? '' : turnstileToken.value,
      capToken: captchaVerified.value ? '' : capToken.value,
    });
    message.success('验证码已发送到您的邮箱');
    captchaVerified.value = true;
    lastEmailForCode.value = email;
    emailCodeCountdown.value = 60;
    emailCodeTimer = setInterval(() => {
      emailCodeCountdown.value--;
      if (emailCodeCountdown.value <= 0) {
        clearInterval(emailCodeTimer!);
        emailCodeTimer = null;
      }
    }, 1000);
  } catch (e: any) {
    const errMsg = e?.response?.data?.error || '发送失败';
    message.error(errMsg);

    if (shouldForceCaptchaRetry(errMsg)) {
      captchaVerified.value = false;
      if (captchaMode.value === 'local') {
        captchaInput.value = '';
        await fetchCaptcha();
      } else if (captchaMode.value === 'turnstile') {
        turnstileToken.value = '';
        await nextTick();
        await renderTurnstileWidget();
      } else if (captchaMode.value === 'cap') {
        await resetCapWidget();
      }
      return;
    }

    if (!captchaVerified.value) {
      if (captchaMode.value === 'local') {
        await fetchCaptcha();
      } else if (captchaMode.value === 'turnstile' && turnstileWidgetId.value && window.turnstile?.reset) {
        window.turnstile.reset(turnstileWidgetId.value);
        turnstileToken.value = '';
      } else if (captchaMode.value === 'cap') {
        resetCapWidget();
      }
    }
  } finally {
    emailCodeSending.value = false;
  }
};

const confirmBindEmail = async () => {
  const email = emailBindForm.value.email.trim().toLowerCase();
  if (!email) {
    message.error('请输入邮箱地址');
    return;
  }
  if (!emailPattern.test(email)) {
    message.error('请输入有效的邮箱地址');
    return;
  }

  const code = emailBindForm.value.code.trim();
  if (!code) {
    message.error('请输入验证码');
    return;
  }

  emailBindSubmitting.value = true;
  try {
    await user.confirmBindEmail({ email, code });
    message.success('邮箱绑定成功');
    await user.infoUpdate();
    showEmailBind.value = false;
    resetEmailBindState();
  } catch (e: any) {
    message.error(e?.response?.data?.error || '绑定失败');
  } finally {
    emailBindSubmitting.value = false;
  }
};

const cancelEmailBind = () => {
  showEmailBind.value = false;
  resetEmailBindState();
};

onBeforeUnmount(() => {
  if (emailCodeTimer) {
    clearInterval(emailCodeTimer);
    emailCodeTimer = null;
  }
  destroyCapWidget();
  destroyTurnstile();
});
</script>

<template>
  <div class="pointer-events-auto relative border px-4 py-2 rounded-md sc-form-scroll" style="min-width: 20rem; max-height: 80vh;">
    <div class=" text-lg text-center mb-8">{{ $t('userProfile.title') }}</div>
    <n-form ref="formRef" :model="model" label-placement="left" label-width="64px" require-mark-placement="right-hanging">
      <n-form-item :label="$t('userProfile.nickname')" path="inputValue">
        <n-input v-model:value="model.nickname" placeholder="你的名字" />
      </n-form-item>
      <n-form-item :label="$t('userProfile.avatar')" path="inputValue">
        <input type="file" ref="inputFileRef" @change="onFileChange" accept="image/*" class="input-file" />
        <div v-if="!showEditor" class="avatar-upload-wrapper">
          <Avatar :src="user.info.avatar" @click="selectFile"></Avatar>
          <div class="avatar-upload-hint">点击头像上传</div>
        </div>
        <div v-else class="avatar-editor-container">
          <AvatarEditor
            :file="avatarFile"
            @save="handleEditorSave"
            @cancel="handleEditorCancel"
          />
        </div>
      </n-form-item>
      <n-form-item :label="$t('userProfile.brief')" path="textareaValue">
        <n-input v-model:value="model.brief" :placeholder="$t('userProfile.briefPlaceholder')" type="textarea" :autosize="{
          minRows: 3,
          maxRows: 5
        }" />
      </n-form-item>
      <n-form-item :label="'其他'" path="textareaValue">
        <div class="flex flex-col gap-2 w-full">
          <n-button @click="passwordChange">修改密码</n-button>
          <n-button @click="openAISettings">AI 设置</n-button>

          <!-- 邮箱绑定区域 -->
          <template v-if="emailAuthEnabled">
            <div v-if="user.info.email" class="flex items-center gap-2 text-sm">
              <span class="text-gray-500">已绑定邮箱：</span>
              <span>{{ user.info.email }}</span>
              <n-button size="tiny" quaternary @click="openEmailBind">更换</n-button>
            </div>
            <n-button v-else-if="!showEmailBind" @click="openEmailBind">
              绑定邮箱
            </n-button>
          </template>
        </div>
      </n-form-item>

      <!-- 邮箱绑定表单 -->
      <template v-if="showEmailBind">
        <n-form-item label="邮箱地址">
          <n-input v-model:value="emailBindForm.email" placeholder="请输入邮箱地址" type="email" />
        </n-form-item>
        <n-form-item v-if="captchaMode === 'local' && !captchaVerified" label="图形验证码">
          <div class="flex w-full items-center gap-3">
            <n-input v-model:value="captchaInput" placeholder="请输入验证码" />
            <div class="sc-captcha-box rounded bg-gray-100 dark:bg-gray-700 flex items-center justify-center cursor-pointer"
              title="点击刷新" @click.prevent="reloadCaptchaImage">
              <img v-if="captchaImageUrl" :src="captchaImageUrl" alt="captcha" class="sc-captcha-img" />
              <span v-else class="text-xs text-gray-500">加载中</span>
            </div>
            <n-button text size="tiny" :loading="captchaLoading" @click.prevent="reloadCaptchaImage">刷新</n-button>
          </div>
          <div v-if="captchaError" class="text-xs text-red-500 dark:text-red-400 mt-1">{{ captchaError }}</div>
        </n-form-item>
        <n-form-item v-else-if="captchaMode === 'turnstile' && !captchaVerified" label="人机验证">
          <div class="w-full rounded border border-gray-200 dark:border-gray-600 py-2 flex items-center justify-center min-h-[90px]">
            <div ref="turnstileContainer" class="w-full flex items-center justify-center"></div>
          </div>
          <div class="flex justify-end mt-1">
            <n-button text size="tiny" :loading="turnstileLoading" @click.prevent="renderTurnstileWidget">刷新</n-button>
          </div>
          <div v-if="turnstileError" class="text-xs text-red-500 dark:text-red-400 mt-1">{{ turnstileError }}</div>
        </n-form-item>
        <n-form-item v-else-if="captchaMode === 'cap' && !captchaVerified" label="验证码验证">
          <div class="w-full">
            <div ref="capContainer" class="w-full"></div>
          </div>
          <div class="flex justify-end mt-1">
            <n-button text size="tiny" :loading="capLoading" @click.prevent="resetCapWidget">刷新</n-button>
          </div>
          <div v-if="capError" class="text-xs text-red-500 dark:text-red-400 mt-1">{{ capError }}</div>
        </n-form-item>
        <n-form-item label="邮箱验证码">
          <div class="flex w-full items-center gap-2">
            <n-input v-model:value="emailBindForm.code" placeholder="请输入验证码" maxlength="6" />
            <n-button type="primary" :loading="emailCodeSending"
              :disabled="emailCodeSending || emailCodeCountdown > 0" @click="sendBindEmailCode">
              {{ emailCodeSending ? '发送中...' : (emailCodeCountdown > 0 ? `${emailCodeCountdown}s` : '发送验证码') }}
            </n-button>
          </div>
        </n-form-item>
        <n-form-item label=" ">
          <div class="flex gap-2">
            <n-button @click="cancelEmailBind">取消</n-button>
            <n-button type="primary" :loading="emailBindSubmitting" @click="confirmBindEmail">确认绑定</n-button>
          </div>
        </n-form-item>
      </template>
    </n-form>
    <div class="flex justify-end mb-4 space-x-4">
      <n-button @click="emit('close')">{{ $t('userProfile.cancel') }}</n-button>
      <n-button @click="save" type="primary">{{ $t('userProfile.save') }}</n-button>
    </div>

    <n-modal
      v-model:show="aiSettingsVisible"
      preset="card"
      title="AI 设置"
      class="sc-fluid-modal sc-fluid-modal--xwide"
      :auto-focus="false"
    >
      <n-spin :show="aiStore.profileLoading || aiSettingsSaving">
        <div class="user-profile-ai">
          <n-alert type="info" class="user-profile-ai__notice">
            选择“我的 API”后，配置仅保存在当前浏览器，请求会由浏览器直接发送到你填写的模型接口，不经过 SealChat 后端代理。
          </n-alert>

          <n-form label-placement="top">
            <n-form-item label="AI 来源">
              <n-radio-group v-model:value="aiSettingsSource">
                <n-space>
                  <n-radio-button value="platform">平台 AI</n-radio-button>
                  <n-radio-button value="user">我的 API</n-radio-button>
                </n-space>
              </n-radio-group>
            </n-form-item>
          </n-form>

          <div class="user-profile-ai__header">
            <div class="user-profile-ai__title">自定义 Provider</div>
            <n-button size="small" @click="addAIProfile">新增</n-button>
          </div>

          <div class="user-profile-ai__profiles">
            <n-empty v-if="aiProfileDrafts.length === 0" description="暂无自定义 API 配置" />
            <div v-for="(profile, index) in aiProfileDrafts" :key="profile.id || index" class="user-profile-ai__profile">
              <div class="user-profile-ai__profile-head">
                <span>Provider {{ index + 1 }}</span>
                <n-space align="center" size="small">
                  <n-switch v-model:value="profile.enabled" />
                  <n-button text type="error" @click="removeAIProfile(profile.id)">删除</n-button>
                </n-space>
              </div>
              <div class="user-profile-ai__grid">
                <n-form-item label="名称">
                  <n-input v-model:value="profile.name" placeholder="例如：DeepSeek Personal" />
                </n-form-item>
                <n-form-item label="Base URL">
                  <n-input v-model:value="profile.baseUrl" placeholder="https://api.deepseek.com/v1" />
                </n-form-item>
                <n-form-item label="API Key">
                  <n-input
                    v-model:value="profile.apiKey"
                    type="password"
                    show-password-on="click"
                    placeholder="仅保存在当前浏览器"
                  />
                </n-form-item>
                <n-form-item label="当前模型">
                  <div class="user-profile-ai__model-row">
                    <n-select
                      :value="profile.selectedModel || undefined"
                      :options="profileModelOptions(profile)"
                      filterable
                      tag
                      placeholder="选择或输入模型名"
                      @update:value="updateProfileSelectedModel(profile, String($event || ''))"
                    />
                    <n-button
                      quaternary
                      circle
                      :loading="aiProfileRefreshing[profile.id]"
                      @click="refreshAIProfileModels(profile)"
                    >
                      <template #icon>
                        <n-icon :component="Refresh" />
                      </template>
                    </n-button>
                  </div>
                </n-form-item>
                <n-form-item label="模型列表">
                  <n-input
                    :value="formatModelsInput(profile)"
                    placeholder="多个模型用英文逗号分隔"
                    @update:value="updateProfileModels(profile, $event)"
                  />
                </n-form-item>
              </div>
            </div>
          </div>
        </div>
      </n-spin>

      <template #footer>
        <n-space justify="end">
          <n-button @click="aiSettingsVisible = false">取消</n-button>
          <n-button type="primary" :loading="aiSettingsSaving" @click="saveAISettings">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style lang="scss">
.input-file {
  display: none;
}

.avatar-upload-wrapper {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.35rem;
  cursor: pointer;
}

.avatar-upload-hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #6b7280);
}

.avatar-editor-container {
  width: 100%;
}

.user-profile-ai {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.user-profile-ai__notice {
  margin-bottom: 0.25rem;
}

.user-profile-ai__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.user-profile-ai__title {
  font-size: 0.95rem;
  font-weight: 600;
}

.user-profile-ai__profiles {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.user-profile-ai__profile {
  border: 1px solid var(--n-border-color, rgba(0, 0, 0, 0.12));
  border-radius: 8px;
  padding: 0.9rem;
}

.user-profile-ai__profile-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
}

.user-profile-ai__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 0.75rem;
}

.user-profile-ai__model-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.5rem;
  width: 100%;
}

@media (max-width: 720px) {
  .user-profile-ai__grid {
    grid-template-columns: 1fr;
  }
}
</style>
