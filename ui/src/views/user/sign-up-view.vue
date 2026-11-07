<script setup lang="ts">
import router from '@/router';
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useUserStore } from '@/stores/user';
import { useMessage } from 'naive-ui';
import { useUtilsStore } from '@/stores/utils';
import type { ServerConfig } from '@/types';
import { api, urlBase } from '@/stores/_config';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { useLoginGlass } from '@/composables/useLoginGlass';
import { useCapWidget } from '@/composables/useCapWidget';

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

const userStore = useUserStore();

const CAPTCHA_SCENE = 'signup';

const form = reactive({
  username: '',
  password: '',
  password2: '',
  nickname: '',
  email: '',
  emailCode: '',
});

const captchaId = ref('');
const captchaInput = ref('');
const captchaImageSeed = ref(0);
const captchaLoading = ref(false);
const captchaError = ref('');

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

const message = useMessage();

const usernamePattern = /^[A-Za-z0-9_.-]+$/;
const usernameError = computed(() => {
  const value = form.username.trim();
  if (!value) {
    return '';
  }
  return usernamePattern.test(value) ? '' : '用户名仅能包含英文、数字、下划线、点或中划线，不能使用汉字';
});

const utils = useUtilsStore();
const config = ref<ServerConfig | null>(utils.config);
const captchaMode = computed(() => config.value?.captcha?.signup?.mode ?? config.value?.captcha?.mode ?? 'off');
const emailAuthEnabled = computed(() => config.value?.emailAuth?.enabled ?? false);
const registerOpen = computed<boolean | null>(() => {
  if (!config.value) return null;
  return config.value.registerOpen ?? false;
});
const registerInviteRequired = computed(() => config.value?.registerInviteRequired ?? false);
const inviteForm = reactive({ code: '' });
const inviteVerified = ref(false);
const inviteVerifying = ref(false);
const inviteError = ref('');
const verifiedInvitationCode = ref('');
const canShowSignupForm = computed(() => registerOpen.value === true && (!registerInviteRequired.value || inviteVerified.value));
const currentInvitationCode = computed(() => registerInviteRequired.value ? verifiedInvitationCode.value : '');

const emailCodeSending = ref(false);
const emailCodeCountdown = ref(0);
let emailCodeTimer: ReturnType<typeof setInterval> | null = null;
const captchaVerified = ref(false); // 标记验证码已通过验证

const emailPattern = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
const emailError = computed(() => {
  if (!emailAuthEnabled.value || !form.email.trim()) return '';
  return emailPattern.test(form.email.trim()) ? '' : '请输入有效的邮箱地址';
});

const shouldForceCaptchaRetry = (errMsg: string) => {
  if (!errMsg) {
    return false;
  }
  return ['请完成验证码验证', '请完成人机验证', '人机验证失败', '验证码错误', '验证码验证失败'].some((keyword) => errMsg.includes(keyword));
};

const verifyInvitationCode = async () => {
  const code = inviteForm.code.trim();
  if (!code) {
    inviteError.value = '请输入邀请码';
    return;
  }
  inviteVerifying.value = true;
  inviteError.value = '';
  try {
    await api.post('api/v1/register-invite/verify', { invitationCode: code });
    verifiedInvitationCode.value = code;
    inviteVerified.value = true;
  } catch (e: any) {
    inviteError.value = e?.response?.data?.message || '邀请码无效';
  } finally {
    inviteVerifying.value = false;
  }
};

const sendEmailCode = async () => {
  if (emailCodeSending.value || emailCodeCountdown.value > 0) return;

  const email = form.email.trim().toLowerCase();
  if (!email || emailError.value) {
    message.error('请输入有效的邮箱地址');
    return;
  }

  // 只有在验证码未验证过时才需要验证
  if (!captchaVerified.value) {
    if (captchaMode.value === 'local' && (!captchaId.value || !captchaInput.value.trim())) {
      message.error('请先填写验证码');
      return;
    }
    if (captchaMode.value === 'turnstile' && !turnstileToken.value) {
      message.error('请先完成人机验证');
      return;
    }
    if (captchaMode.value === 'cap' && !capToken.value) {
      message.error('请先完成验证码验证');
      return;
    }
  }

  emailCodeSending.value = true;
  try {
    await userStore.sendSignupEmailCode({
      email,
      invitationCode: currentInvitationCode.value,
      captchaId: captchaVerified.value ? '' : captchaId.value,
      captchaValue: captchaVerified.value ? '' : captchaInput.value.trim(),
      turnstileToken: captchaVerified.value ? '' : turnstileToken.value,
      capToken: captchaVerified.value ? '' : capToken.value,
    });
    message.success('验证码已发送到您的邮箱');
    captchaVerified.value = true; // 标记验证码已通过
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

    // 发送失败时刷新验证码
    if (!captchaVerified.value) {
      if (captchaMode.value === 'local') {
        fetchCaptcha();
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

const { glassStyle: loginGlassStyle } = useLoginGlass({
  imageUrl: loginBgUrl,
  config: loginBgConfig,
  enabled: hasLoginBg,
  radius: '8px',
});

const captchaImageUrl = computed(() => {
  if (!captchaId.value) {
    return '';
  }
  return `${urlBase}/api/v1/captcha/${captchaId.value}.png?scene=${CAPTCHA_SCENE}&ts=${captchaImageSeed.value}`;
});

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

const resetLocalCaptchaState = () => {
  captchaId.value = '';
  captchaInput.value = '';
  captchaImageSeed.value = Date.now();
  captchaError.value = '';
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
    captchaError.value = '验证码刷新失败，已为你重新生成';
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
    console.error(err);
    turnstileError.value = '无法加载 Turnstile，请稍后重试';
  } finally {
    turnstileLoading.value = false;
  }
};

const renderCaptchaAfterSignupFormVisible = async () => {
  if (!canShowSignupForm.value) {
    return;
  }
  await nextTick();
  if (!canShowSignupForm.value) {
    return;
  }
  if (captchaMode.value === 'local') {
    await fetchCaptcha();
  } else if (captchaMode.value === 'turnstile') {
    await renderTurnstileWidget();
  } else if (captchaMode.value === 'cap') {
    await renderCapWidget();
  }
};

watch(
  () => captchaMode.value,
  (mode) => {
    if (!mode || mode === 'off') {
      resetLocalCaptchaState();
      destroyCapWidget();
      destroyTurnstile();
      return;
    }
    if (mode === 'local') {
      destroyCapWidget();
      destroyTurnstile();
      if (canShowSignupForm.value) {
        fetchCaptcha();
      }
    } else if (mode === 'turnstile') {
      destroyCapWidget();
      resetLocalCaptchaState();
      if (canShowSignupForm.value) {
        renderTurnstileWidget();
      }
    } else if (mode === 'cap') {
      destroyTurnstile();
      resetLocalCaptchaState();
      if (canShowSignupForm.value) {
        renderCapWidget();
      }
    }
  },
  { immediate: true },
);

watch(
  () => canShowSignupForm.value,
  (visible) => {
    if (visible) {
      renderCaptchaAfterSignupFormVisible();
      return;
    }
    resetLocalCaptchaState();
    destroyCapWidget();
    destroyTurnstile();
  },
  { immediate: true },
);

watch(
  () => registerInviteRequired.value,
  (required) => {
    if (!required) {
      inviteForm.code = '';
      inviteVerified.value = false;
      verifiedInvitationCode.value = '';
      inviteError.value = '';
    }
  },
);

const signUp = async () => {
  if (usernameError.value) {
    message.error(usernameError.value);
    return;
  }

  form.username = form.username.trim();

  // 邮箱注册流程
  if (emailAuthEnabled.value) {
    if (emailError.value) {
      message.error(emailError.value);
      return;
    }
    if (!form.email.trim()) {
      message.error('请输入邮箱地址');
      return;
    }
    if (!form.emailCode.trim()) {
      message.error('请输入邮箱验证码');
      return;
    }

    try {
      await userStore.signUpWithEmail({
        username: form.username,
        password: form.password,
        nickname: form.nickname || form.username,
        email: form.email.trim().toLowerCase(),
        code: form.emailCode.trim(),
        invitationCode: currentInvitationCode.value,
      });
      message.success('注册成功，即将前往世界大厅');
      router.replace({ name: 'world-lobby' });
    } catch (e: any) {
      message.error(e?.response?.data?.error || '注册失败');
    }
    return;
  }

  // 原有注册流程
  if (captchaMode.value === 'local') {
    if (!captchaId.value) {
      await fetchCaptcha();
      message.error('验证码加载中，请稍后再试');
      return;
    }
    const value = captchaInput.value.trim();
    if (!value) {
      message.error('请输入验证码');
      return;
    }
  } else if (captchaMode.value === 'turnstile' && !turnstileToken.value) {
    message.error('请完成人机验证');
    return;
  } else if (captchaMode.value === 'cap' && !capToken.value) {
    message.error('请先完成验证码验证');
    return;
  }

  const captchaValue = captchaInput.value.trim();
  const ret = await userStore.signUp({
    username: form.username,
    password: form.password,
    nickname: form.nickname,
    invitationCode: currentInvitationCode.value,
    captchaId: captchaId.value,
    captchaValue,
    turnstileToken: turnstileToken.value,
    capToken: capToken.value,
  });

  if (captchaMode.value === 'local') {
    fetchCaptcha();
  } else if (captchaMode.value === 'turnstile' && turnstileWidgetId.value && window.turnstile?.reset) {
    window.turnstile.reset(turnstileWidgetId.value);
    turnstileToken.value = '';
  } else if (captchaMode.value === 'cap') {
    resetCapWidget();
  }

  if (ret) {
    message.error(ret);
  } else {
    message.success('注册成功，即将前往世界大厅');
    router.replace({ name: 'world-lobby' });
  }
};

const randomUsername = () => {
  const characters = 'abcdefghjkmnpqrstuvwxyz';
  const characters2 = 'abcdefghjkmnpqrstuvwxyz23456789';
  let result = '';
  for (let i = 0; i < 1; i++) {
    result += characters.charAt(Math.floor(Math.random() * characters.length));
  }
  for (let i = 0; i < 4; i++) {
    result += characters2.charAt(Math.floor(Math.random() * characters2.length));
  }
  form.username = result;
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
  destroyCapWidget();
  destroyTurnstile();
  if (emailCodeTimer) {
    clearInterval(emailCodeTimer);
    emailCodeTimer = null;
  }
});
</script>

<template>
  <div class="sign-up-root">
    <!-- Background layers -->
    <div v-if="hasLoginBg" class="login-bg-layer" :style="loginBgStyle"></div>
    <div v-if="hasLoginBg && loginOverlayStyle" class="login-overlay-layer" :style="loginOverlayStyle"></div>

    <div class="w-full max-w-sm mx-auto overflow-hidden rounded-lg shadow-md sign-up-card sc-form-scroll"
      :class="{ 'sc-glass-panel': hasLoginBg, 'sign-up-card--glass': hasLoginBg }"
      :style="hasLoginBg ? loginGlassStyle : undefined"
      v-if="canShowSignupForm">
      <div class="px-8 py-6">
        <h3 class="mb-6 text-xl font-medium text-center auth-title">注册</h3>

        <n-form class="w-full" @submit.prevent="signUp">
          <n-form-item label="用户名">
            <n-input
              v-model:value="form.username"
              placeholder="用户名，用于登录和识别，可被其他人看到"
              @keydown.enter.prevent
            >
              <template #suffix>
                <n-button text size="tiny" tabindex="-1" @click.prevent="randomUsername">随机</n-button>
              </template>
            </n-input>
          </n-form-item>
          <div v-if="usernameError" class="auth-error">{{ usernameError }}</div>

          <n-form-item label="昵称">
            <n-input v-model:value="form.nickname" placeholder="昵称" @keydown.enter.prevent />
          </n-form-item>

          <n-form-item label="密码">
            <n-input v-model:value="form.password" type="password" placeholder="密码" @keydown.enter.prevent />
          </n-form-item>

          <template v-if="emailAuthEnabled">
            <n-form-item label="邮箱地址">
              <n-input v-model:value="form.email" :input-props="{ type: 'email' }" placeholder="邮箱地址" @keydown.enter.prevent />
            </n-form-item>
            <div v-if="emailError" class="auth-error">{{ emailError }}</div>

            <n-form-item v-if="captchaMode === 'local' && !captchaVerified" label="图形验证码">
              <div class="auth-captcha-stack">
                <n-input v-model:value="captchaInput" placeholder="请输入图形验证码" @keydown.enter.prevent />
                <div class="auth-captcha-row">
                  <div class="sc-captcha-box rounded bg-gray-100 dark:bg-gray-700 flex items-center justify-center cursor-pointer"
                    @click.prevent="reloadCaptchaImage" title="点击刷新">
                    <img v-if="captchaImageUrl" :src="captchaImageUrl" alt="captcha" class="sc-captcha-img" />
                    <span v-else class="text-xs text-gray-500">加载中</span>
                  </div>
                  <n-button text size="tiny" :loading="captchaLoading" @click.prevent="reloadCaptchaImage">刷新</n-button>
                </div>
              </div>
            </n-form-item>
            <div v-if="captchaError && captchaMode === 'local' && !captchaVerified" class="auth-error">{{ captchaError }}</div>

            <n-form-item v-else-if="captchaMode === 'turnstile' && !captchaVerified" label="人机验证">
              <div class="auth-verification-box">
                <div ref="turnstileContainer" class="flex items-center justify-center min-h-[90px] py-2"></div>
                <div class="auth-captcha-actions">
                  <n-button text size="tiny" :loading="turnstileLoading" @click.prevent="renderTurnstileWidget">刷新</n-button>
                </div>
              </div>
            </n-form-item>
            <div v-if="turnstileError && captchaMode === 'turnstile' && !captchaVerified" class="auth-error">{{ turnstileError }}</div>

            <n-form-item v-else-if="captchaMode === 'cap' && !captchaVerified" label="验证码验证">
              <div class="auth-verification-box">
                <div ref="capContainer" class="w-full"></div>
                <div class="auth-captcha-actions">
                  <n-button text size="tiny" :loading="capLoading" @click.prevent="resetCapWidget">刷新</n-button>
                </div>
              </div>
            </n-form-item>
            <div v-if="capError && captchaMode === 'cap' && !captchaVerified" class="auth-error">{{ capError }}</div>

            <n-form-item label="邮箱验证码">
              <n-input-group>
                <n-input
                  v-model:value="form.emailCode"
                  placeholder="请输入邮箱验证码"
                  maxlength="6"
                  @keydown.enter.prevent
                />
                <n-button
                  type="primary"
                  :loading="emailCodeSending"
                  :disabled="emailCodeCountdown > 0"
                  @click.prevent="sendEmailCode"
                >
                  {{ emailCodeSending ? '发送中...' : (emailCodeCountdown > 0 ? `${emailCodeCountdown}s` : '获取验证码') }}
                </n-button>
              </n-input-group>
            </n-form-item>
          </template>

          <template v-else>
            <n-form-item v-if="captchaMode === 'local'" label="验证码">
              <div class="auth-captcha-stack">
                <n-input v-model:value="captchaInput" placeholder="请输入验证码" @keydown.enter.prevent />
                <div class="auth-captcha-row">
                  <div class="sc-captcha-box rounded bg-gray-100 dark:bg-gray-700 flex items-center justify-center cursor-pointer"
                    @click.prevent="reloadCaptchaImage" title="点击刷新">
                    <img v-if="captchaImageUrl" :src="captchaImageUrl" alt="captcha" class="sc-captcha-img" />
                    <span v-else class="text-xs text-gray-500">加载中</span>
                  </div>
                  <n-button text size="tiny" :loading="captchaLoading" @click.prevent="reloadCaptchaImage">刷新</n-button>
                </div>
              </div>
            </n-form-item>
            <div v-if="captchaError && captchaMode === 'local'" class="auth-error">{{ captchaError }}</div>

            <n-form-item v-else-if="captchaMode === 'turnstile'" label="人机验证">
              <div class="auth-verification-box">
                <div ref="turnstileContainer" class="flex items-center justify-center min-h-[90px] py-2"></div>
                <div class="auth-captcha-actions">
                  <n-button text size="tiny" :loading="turnstileLoading" @click.prevent="renderTurnstileWidget">刷新</n-button>
                </div>
              </div>
            </n-form-item>
            <div v-if="turnstileError && captchaMode === 'turnstile'" class="auth-error">{{ turnstileError }}</div>

            <n-form-item v-else-if="captchaMode === 'cap'" label="验证码验证">
              <div class="auth-verification-box">
                <div ref="capContainer" class="w-full"></div>
                <div class="auth-captcha-actions">
                  <n-button text size="tiny" :loading="capLoading" @click.prevent="resetCapWidget">刷新</n-button>
                </div>
              </div>
            </n-form-item>
            <div v-if="capError && captchaMode === 'cap'" class="auth-error">{{ capError }}</div>
          </template>

          <div class="auth-actions">
            <n-button type="primary" round @click.prevent="signUp">注册</n-button>
          </div>
        </n-form>
      </div>

      <div class="flex items-center justify-center py-4 text-center sign-up-footer">
        <span class="text-sm sign-up-footer__text">已有账号 ？</span>
        <router-link :to="{ name: 'user-signin' }"
          class="mx-2 text-sm font-bold sign-up-footer__link hover:underline">登录</router-link>
      </div>
    </div>
    <div class="w-full max-w-sm mx-auto overflow-hidden rounded-lg shadow-md sign-up-card"
      :class="{ 'sc-glass-panel': hasLoginBg, 'sign-up-card--glass': hasLoginBg }"
      :style="hasLoginBg ? loginGlassStyle : undefined" v-else-if="registerOpen === false">
      <div class="p-6">你来晚了，门已经悄然关闭。</div>
    </div>
    <div class="w-full max-w-sm mx-auto overflow-hidden rounded-lg shadow-md sign-up-card"
      :class="{ 'sc-glass-panel': hasLoginBg, 'sign-up-card--glass': hasLoginBg }"
      :style="hasLoginBg ? loginGlassStyle : undefined" v-else-if="registerOpen === true && registerInviteRequired">
      <div class="px-8 py-6">
        <h3 class="mb-3 text-xl font-medium text-center auth-title">输入邀请码</h3>
        <p class="invite-hint">此站点需要邀请码后才能注册。</p>
        <n-form class="w-full" @submit.prevent="verifyInvitationCode">
          <n-form-item label="邀请码">
            <n-input
              v-model:value="inviteForm.code"
              type="password"
              show-password-on="click"
              placeholder="请输入邀请码"
              @keydown.enter.prevent="verifyInvitationCode"
            />
          </n-form-item>
          <div v-if="inviteError" class="auth-error">{{ inviteError }}</div>
          <div class="auth-actions">
            <n-button type="primary" round :loading="inviteVerifying" @click.prevent="verifyInvitationCode">继续注册</n-button>
          </div>
        </n-form>
      </div>
      <div class="flex items-center justify-center py-4 text-center sign-up-footer">
        <span class="text-sm sign-up-footer__text">已有账号 ？</span>
        <router-link :to="{ name: 'user-signin' }"
          class="mx-2 text-sm font-bold sign-up-footer__link hover:underline">登录</router-link>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sign-up-root {
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

.sign-up-card {
  position: relative;
  z-index: 2;
  background: var(--sc-bg-elevated, #ffffff);
  max-height: 100%;
}

:global(.dark) .sign-up-card {
  background: #1f2937;
}

.sign-up-card.sc-glass-panel {
  background: var(--sc-glass-bg);
  box-shadow: var(--sc-glass-shadow);
}

.sign-up-footer {
  background: transparent;
  border-top: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.06));
}

:global(.dark) .sign-up-footer {
  background: transparent;
}

.sign-up-card--glass .sign-up-footer {
  background: transparent;
  border-top: 1px solid var(--sc-glass-border);
}

.sign-up-footer__text {
  color: var(--sc-text-secondary, #475569);
}

.sign-up-footer__link {
  color: var(--primary-color, #3388de);
}

.auth-title {
  color: var(--sc-text-primary, #0f172a);
}

.auth-error {
  margin-top: -14px;
  margin-bottom: 12px;
  font-size: 12px;
  color: #ef4444;
}

.invite-hint {
  margin-bottom: 1rem;
  text-align: center;
  font-size: 13px;
  color: var(--sc-text-secondary, #475569);
}

.auth-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 1rem;
}

.auth-captcha-stack,
.auth-verification-box {
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: 0.5rem;
}

.auth-captcha-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.auth-captcha-actions {
  display: flex;
  justify-content: flex-end;
}

.sign-up-card--glass :deep(input) {
  color: rgba(17, 24, 39, 0.96) !important;
}

.sign-up-card--glass :deep(input::placeholder) {
  color: rgba(55, 65, 81, 0.78) !important;
}

.sign-up-card--glass :deep(.text-gray-500),
.sign-up-card--glass :deep(.text-gray-600),
.sign-up-card--glass :deep(.text-gray-700),
.sign-up-card--glass :deep(.dark\:text-gray-200),
.sign-up-card--glass :deep(.dark\:text-gray-300),
.sign-up-card--glass :deep(.dark\:text-gray-400) {
  color: rgba(17, 24, 39, 0.92) !important;
}

:global(.dark) .sign-up-card--glass :deep(input) {
  color: rgba(243, 244, 246, 0.96) !important;
}

:global(.dark) .sign-up-card--glass :deep(input::placeholder) {
  color: rgba(229, 231, 235, 0.8) !important;
}

:global(.dark) .sign-up-card--glass :deep(.text-gray-500),
:global(.dark) .sign-up-card--glass :deep(.text-gray-600),
:global(.dark) .sign-up-card--glass :deep(.text-gray-700),
:global(.dark) .sign-up-card--glass :deep(.dark\:text-gray-200),
:global(.dark) .sign-up-card--glass :deep(.dark\:text-gray-300),
:global(.dark) .sign-up-card--glass :deep(.dark\:text-gray-400) {
  color: rgba(243, 244, 246, 0.93) !important;
}
</style>
