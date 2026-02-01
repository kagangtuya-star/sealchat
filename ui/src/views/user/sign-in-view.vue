<script setup lang="ts">
import router from '@/router';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import type { FormInst, FormRules } from 'naive-ui';
import { useMessage } from 'naive-ui';
import { useUserStore } from '@/stores/user';
import { DEFAULT_PAGE_TITLE, useUtilsStore } from '@/stores/utils';
import type { ServerConfig } from '@/types';
import { api, urlBase } from '@/stores/_config';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

declare global {
  interface Window {
    turnstile?: {
      render: (container: HTMLElement | string, options: Record<string, any>) => string;
      reset: (widgetId?: string) => void;
      remove: (widgetId?: string) => void;
    };
  }
}

const CAPTCHA_SCENE = 'signin';
let turnstileScriptPromise: Promise<void> | null = null;

const message = useMessage();
const formRef = ref<FormInst | null>(null);

const model = ref({
  account: '',
  password: '',
});

const captchaId = ref('');
const captchaInput = ref('');
const captchaImageSeed = ref(0);
const captchaLoading = ref(false);
const captchaError = ref('');

const turnstileContainer = ref<HTMLDivElement | null>(null);
const turnstileToken = ref('');
const turnstileWidgetId = ref<string | null>(null);
const turnstileError = ref('');
const turnstileLoading = ref(false);

const userStore = useUserStore();
const utils = useUtilsStore();
const config = ref<ServerConfig | null>(null);

const signInTitle = computed(() => {
  const title = (config.value?.pageTitle ?? utils.config?.pageTitle)?.trim();
  return title && title.length > 0 ? title : DEFAULT_PAGE_TITLE;
});

// Login background
const loginBgConfig = computed(() => config.value?.loginBackground);
const loginBgUrl = computed(() => {
  const id = loginBgConfig.value?.attachmentId;
  if (!id) return '';
  return resolveAttachmentUrl(id.startsWith('id:') ? id : `id:${id}`);
});
const hasLoginBg = computed(() => !!loginBgUrl.value);
const loginBgStyle = computed(() => {
  if (!loginBgUrl.value) return {};
  const cfg = loginBgConfig.value;
  const mode = cfg?.mode || 'cover';
  let bgSize = 'cover';
  let bgRepeat = 'no-repeat';
  let bgPosition = 'center';
  switch (mode) {
    case 'contain': bgSize = 'contain'; break;
    case 'tile': bgSize = 'auto'; bgRepeat = 'repeat'; break;
    case 'center': bgSize = 'auto'; bgPosition = 'center'; break;
  }
  return {
    backgroundImage: `url(${loginBgUrl.value})`,
    backgroundSize: bgSize,
    backgroundRepeat: bgRepeat,
    backgroundPosition: bgPosition,
    opacity: (cfg?.opacity ?? 30) / 100,
    filter: `blur(${cfg?.blur ?? 0}px) brightness(${cfg?.brightness ?? 100}%)`,
  };
});
const loginOverlayStyle = computed(() => {
  const cfg = loginBgConfig.value;
  if (!cfg?.overlayColor || !cfg?.overlayOpacity) return null;
  return {
    backgroundColor: cfg.overlayColor,
    opacity: cfg.overlayOpacity / 100,
  };
});

const captchaMode = computed(() => config.value?.captcha?.signin?.mode ?? config.value?.captcha?.mode ?? 'off');
const emailAuthEnabled = computed(() => config.value?.emailAuth?.enabled ?? false);
const captchaImageUrl = computed(() => {
  if (!captchaId.value) {
    return '';
  }
  return `${urlBase}/api/v1/captcha/${captchaId.value}.png?scene=${CAPTCHA_SCENE}&ts=${captchaImageSeed.value}`;
});

const rules: FormRules = {
  account: [{ required: true, message: '请输入用户名/昵称/邮箱' }],
  password: [{ required: true, message: '请输入密码' }],
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
      const existing = document.getElementById('cf-turnstile-script');
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

const fetchCaptcha = async () => {
  if (captchaMode.value !== 'local') {
    return;
  }
  captchaLoading.value = true;
  captchaError.value = '';
  try {
    const resp = await api.get<{ id: string }>('api/v1/captcha/new', { params: { scene: CAPTCHA_SCENE } });
    captchaId.value = resp.data.id;
    captchaInput.value = '';
    captchaImageSeed.value = Date.now();
  } catch (err) {
    console.error(err);
    captchaError.value = '验证码加载失败，请稍后重试';
  } finally {
    captchaLoading.value = false;
  }
};

const reloadCaptchaImage = async () => {
  if (captchaMode.value !== 'local') {
    return;
  }
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
    console.error(err);
    captchaError.value = '验证码刷新失败，已重新生成';
    await fetchCaptcha();
  } finally {
    captchaLoading.value = false;
  }
};

const destroyTurnstile = () => {
  if (typeof window === 'undefined') {
    return;
  }
  if (turnstileWidgetId.value && window.turnstile?.remove) {
    window.turnstile.remove(turnstileWidgetId.value);
  }
  turnstileWidgetId.value = null;
  turnstileToken.value = '';
  turnstileError.value = '';
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
    const siteKey = config.value?.captcha?.signin?.turnstile?.siteKey?.trim() || config.value?.captcha?.turnstile?.siteKey?.trim();
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
    console.error(err);
    turnstileError.value = '无法加载 Turnstile，请稍后再试';
  } finally {
    turnstileLoading.value = false;
  }
};

watch(
  () => captchaMode.value,
  (mode) => {
    if (mode === 'local') {
      destroyTurnstile();
      fetchCaptcha();
    } else if (mode === 'turnstile') {
      captchaId.value = '';
      captchaInput.value = '';
      renderTurnstileWidget();
    } else {
      destroyTurnstile();
      captchaId.value = '';
      captchaInput.value = '';
    }
  },
  { immediate: true },
);

const handleValidateButtonClick = async (e: MouseEvent) => {
  e.preventDefault();
  formRef.value?.validate(async (errors) => {
    if (errors) {
      message.error('验证失败');
      return;
    }

    const account = model.value.account.trim();
    if (!account) {
      message.error('请输入用户名/昵称/邮箱');
      return;
    }

    if (captchaMode.value === 'local') {
      if (!captchaId.value) {
        await fetchCaptcha();
        message.error('验证码加载中，请稍后再试');
        return;
      }
      if (!captchaInput.value.trim()) {
        message.error('请输入验证码');
        return;
      }
    } else if (captchaMode.value === 'turnstile' && !turnstileToken.value) {
      message.error('请完成人机验证');
      return;
    }

    try {
      const resp = await userStore.signIn({
        username: account,
        password: model.value.password || '',
        captchaId: captchaId.value,
        captchaValue: captchaInput.value.trim(),
        turnstileToken: turnstileToken.value,
      });
      const ret = resp.data;
      if (captchaMode.value === 'local') {
        fetchCaptcha();
      } else if (captchaMode.value === 'turnstile' && turnstileWidgetId.value && window.turnstile?.reset) {
        window.turnstile.reset(turnstileWidgetId.value);
        turnstileToken.value = '';
      }
      message.success('验证成功，即将返回首页');
      if (ret.token) {
        router.replace({ name: 'home' });
      }
    } catch (err) {
      message.error('登录失败: ' + ((err as any)?.response?.data?.message || '账号或密码错误/连接服务器失败'));
      if (captchaMode.value === 'local') {
        fetchCaptcha();
      } else if (captchaMode.value === 'turnstile' && turnstileWidgetId.value && window.turnstile?.reset) {
        window.turnstile.reset(turnstileWidgetId.value);
        turnstileToken.value = '';
      }
    }
  });
};

onMounted(async () => {
  try {
    const resp = await utils.configGet();
    config.value = resp.data;
  } catch (err) {
    console.error('Failed to load config:', err);
  }
});

onBeforeUnmount(() => {
  destroyTurnstile();
});
</script>

<template>
  <div class="sign-in-root">
    <!-- Background layers -->
    <div v-if="hasLoginBg" class="login-bg-layer" :style="loginBgStyle"></div>
    <div v-if="hasLoginBg && loginOverlayStyle" class="login-overlay-layer" :style="loginOverlayStyle"></div>

    <div class="sign-in-content sc-form-scroll" :class="{ 'has-bg': hasLoginBg }">
      <h2 class="font-bold text-xl mb-8">{{ signInTitle }}</h2>

      <n-form ref="formRef" :model="model" :rules="rules" class="w-full px-8 max-w-md">
      <n-form-item path="account" label="用户名/昵称/邮箱">
        <n-input v-model:value="model.account" placeholder="用户名/昵称/邮箱" @keydown.enter.prevent />
      </n-form-item>

        <n-form-item path="password" label="密码">
          <n-input v-model:value="model.password" type="password" @keydown.enter.prevent />
        </n-form-item>

        <n-form-item v-if="captchaMode === 'local'" label="验证码">
          <div class="flex w-full items-center gap-3">
            <n-input v-model:value="captchaInput" placeholder="请输入验证码" />
            <div class="sc-captcha-box rounded bg-gray-100 dark:bg-gray-700 flex items-center justify-center cursor-pointer"
              title="点击刷新" @click.prevent="reloadCaptchaImage">
              <img v-if="captchaImageUrl" :src="captchaImageUrl" alt="captcha" class="sc-captcha-img" />
              <span v-else class="text-xs text-gray-500">加载中</span>
            </div>
            <n-button text size="tiny" :loading="captchaLoading" @click.prevent="reloadCaptchaImage">刷新</n-button>
          </div>
          <div v-if="captchaError" class="text-xs text-red-500 mt-1">{{ captchaError }}</div>
        </n-form-item>

        <n-form-item v-else-if="captchaMode === 'turnstile'" label="人机验证">
          <div class="w-full rounded border border-gray-200 dark:border-gray-600 py-2 flex items-center justify-center min-h-[90px]">
            <div ref="turnstileContainer" class="w-full flex items-center justify-center"></div>
          </div>
          <div class="flex justify-end mt-1">
            <n-button text size="tiny" :loading="turnstileLoading" @click.prevent="renderTurnstileWidget">刷新</n-button>
          </div>
          <div v-if="turnstileError" class="text-xs text-red-500 mt-1">{{ turnstileError }}</div>
        </n-form-item>

        <n-row :gutter="[0, 24]">
          <n-col :span="24">
            <div class=" flex justify-between">
              <div class="flex items-center gap-2">
                <router-link :to="{ name: 'user-signup' }">
                  <n-button type="text" v-if="config?.registerOpen">注册</n-button>
                </router-link>
                <n-button v-if="emailAuthEnabled" type="text" @click="router.push({ name: 'password-recovery' })">忘记密码</n-button>
              </div>

              <n-button :disabled="model.account === ''" round type="primary" @click="handleValidateButtonClick">
                登录
              </n-button>
            </div>
          </n-col>
        </n-row>
      </n-form>

    </div>
  </div>
</template>
  
<style scoped>
.sign-in-root {
  position: relative;
  display: flex;
  height: 100%;
  width: 100%;
  justify-content: center;
  align-items: center;
  overflow: hidden;
  padding: 1rem;
  box-sizing: border-box;
}

.login-bg-layer {
  position: fixed;
  inset: 0;
  z-index: 0;
  pointer-events: none;
}

.login-overlay-layer {
  position: fixed;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}

.sign-in-content {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-start;
  width: 50%;
  min-width: 20rem;
  max-height: 100%;
  padding: 2rem;
  transition: all 0.3s;
}

.sign-in-content.has-bg {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(8px);
  border-radius: 12px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.1);
}

:global(.dark) .sign-in-content.has-bg {
  background: rgba(31, 41, 55, 0.85);
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.3);
}
</style>
