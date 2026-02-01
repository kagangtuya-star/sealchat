<script setup lang="ts">
import router from '@/router';
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useUserStore } from '@/stores/user';
import { useMessage } from 'naive-ui';
import { useUtilsStore } from '@/stores/utils';
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
const config = ref<ServerConfig | null>(null);
const captchaMode = computed(() => config.value?.captcha?.signup?.mode ?? config.value?.captcha?.mode ?? 'off');
const emailAuthEnabled = computed(() => config.value?.emailAuth?.enabled ?? false);

const emailCodeSending = ref(false);
const emailCodeCountdown = ref(0);
let emailCodeTimer: ReturnType<typeof setInterval> | null = null;
const captchaVerified = ref(false); // 标记验证码已通过验证

const emailPattern = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
const emailError = computed(() => {
  if (!emailAuthEnabled.value || !form.email.trim()) return '';
  return emailPattern.test(form.email.trim()) ? '' : '请输入有效的邮箱地址';
});

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
  }

  emailCodeSending.value = true;
  try {
    await userStore.sendSignupEmailCode({
      email,
      captchaId: captchaVerified.value ? '' : captchaId.value,
      captchaValue: captchaVerified.value ? '' : captchaInput.value.trim(),
      turnstileToken: captchaVerified.value ? '' : turnstileToken.value,
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
    message.error(e?.response?.data?.error || '发送失败');
    // 发送失败时刷新验证码
    if (!captchaVerified.value) {
      if (captchaMode.value === 'local') {
        fetchCaptcha();
      } else if (captchaMode.value === 'turnstile' && turnstileWidgetId.value && window.turnstile?.reset) {
        window.turnstile.reset(turnstileWidgetId.value);
        turnstileToken.value = '';
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

watch(
  () => captchaMode.value,
  (mode) => {
    if (!mode || mode === 'off') {
      resetLocalCaptchaState();
      destroyTurnstile();
      return;
    }
    if (mode === 'local') {
      destroyTurnstile();
      fetchCaptcha();
    } else if (mode === 'turnstile') {
      resetLocalCaptchaState();
      renderTurnstileWidget();
    }
  },
  { immediate: true },
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
  }

  const captchaValue = captchaInput.value.trim();
  const ret = await userStore.signUp({
    username: form.username,
    password: form.password,
    nickname: form.nickname,
    captchaId: captchaId.value,
    captchaValue,
    turnstileToken: turnstileToken.value,
  });

  if (captchaMode.value === 'local') {
    fetchCaptcha();
  } else if (captchaMode.value === 'turnstile' && turnstileWidgetId.value && window.turnstile?.reset) {
    window.turnstile.reset(turnstileWidgetId.value);
    turnstileToken.value = '';
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
      :class="{ 'has-bg': hasLoginBg }"
      v-if="config?.registerOpen">
      <div class="px-6 py-4">
        <div class="flex justify-center mx-auto">
          <!-- <img class="w-auto h-7 sm:h-8" src="https://merakiui.com/images/logo.svg" alt=""> -->
        </div>

        <h3 class="mt-3 text-xl font-medium text-center text-gray-600 dark:text-gray-200">注册</h3>

        <div style="font-size: 12; overflow-y: auto; max-height: 10rem;">
          <!-- {{ authStore.session }} -->
        </div>

        <form class="min-w-20rem">

          <div class="w-full mt-4">
            <div class="relative">
              <input v-model="form.username"
                class="block w-full px-4 py-2 mt-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
                type="username" placeholder="用户名，用于登录和识别，可被其他人看到" aria-label="用户名" />
              <button @click.prevent="randomUsername"
                class="absolute right-0 h-full top-0 px-1 mr-1 text-sm font-medium text-blue-500 capitalize" tabindex="-1">随机
              </button>
            </div>
            <p v-if="usernameError" class="mt-1 text-xs text-red-500">{{ usernameError }}</p>
          </div>

          <div class="w-full mt-4">
            <input v-model="form.nickname"
              class="block w-full px-4 py-2 mt-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
              type="text" placeholder="昵称" aria-label="昵称" />
          </div>

          <div class="w-full mt-4">
            <input v-model="form.password"
              class="block w-full px-4 py-2 mt-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
              type="password" placeholder="密码" aria-label="密码" />
          </div>

          <!-- 邮箱注册区域 -->
          <template v-if="emailAuthEnabled">
            <div class="w-full mt-4">
              <label class="block text-xs text-gray-500 dark:text-gray-300">邮箱地址</label>
              <input v-model="form.email"
                class="block w-full px-4 py-2 mt-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
                type="email" placeholder="邮箱地址" aria-label="邮箱地址" />
              <p v-if="emailError" class="mt-1 text-xs text-red-500 dark:text-red-400">{{ emailError }}</p>
            </div>

            <!-- 基础验证码（发送邮箱验证码前需要，验证通过后隐藏） -->
            <div class="w-full mt-4" v-if="captchaMode === 'local' && !captchaVerified">
              <label class="block text-xs text-gray-500 dark:text-gray-300">图形验证码</label>
              <div class="flex items-center gap-3 mt-2">
                <input v-model="captchaInput"
                  class="block w-full px-4 py-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
                  type="text" placeholder="请输入图形验证码" aria-label="图形验证码"
                />
                <div class="flex flex-col items-center gap-1">
                  <div class="sc-captcha-box bg-gray-100 dark:bg-gray-700 flex items-center justify-center rounded cursor-pointer"
                    @click.prevent="reloadCaptchaImage" title="点击刷新">
                    <img v-if="captchaImageUrl" :src="captchaImageUrl" alt="captcha" class="sc-captcha-img" />
                    <span v-else class="text-xs text-gray-500">加载中</span>
                  </div>
                  <button type="button" @click.prevent="reloadCaptchaImage"
                    class="text-xs text-blue-500" :disabled="captchaLoading">
                    {{ captchaLoading ? '刷新中' : '刷新' }}
                  </button>
                </div>
              </div>
              <p v-if="captchaError" class="mt-1 text-xs text-red-500">{{ captchaError }}</p>
            </div>

            <div class="w-full mt-4" v-else-if="captchaMode === 'turnstile' && !captchaVerified">
              <label class="block text-xs text-gray-500 dark:text-gray-300">人机验证</label>
              <div class="mt-2 rounded border border-gray-200 dark:border-gray-600 bg-white dark:bg-gray-800">
                <div ref="turnstileContainer" class="flex items-center justify-center min-h-[90px] py-2"></div>
              </div>
              <div class="flex justify-end mt-2">
                <button type="button" class="text-xs text-blue-500" :disabled="turnstileLoading"
                  @click.prevent="renderTurnstileWidget">
                  {{ turnstileLoading ? '加载中' : '刷新' }}
                </button>
              </div>
              <p v-if="turnstileError" class="mt-1 text-xs text-red-500">{{ turnstileError }}</p>
            </div>

            <div class="w-full mt-4">
              <label class="block text-xs text-gray-500 dark:text-gray-300">邮箱验证码</label>
              <div class="flex items-center gap-3 mt-2">
                <input v-model="form.emailCode"
                  class="block w-full px-4 py-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
                  type="text" placeholder="请输入邮箱验证码" maxlength="6" aria-label="邮箱验证码"
                />
                <button type="button" @click.prevent="sendEmailCode"
                  class="shrink-0 px-4 py-2 text-sm font-medium text-white bg-blue-500 rounded-lg hover:bg-blue-400 disabled:opacity-50 disabled:cursor-not-allowed"
                  :disabled="emailCodeSending || emailCodeCountdown > 0">
                  {{ emailCodeSending ? '发送中...' : (emailCodeCountdown > 0 ? `${emailCodeCountdown}s` : '获取验证码') }}
                </button>
              </div>
            </div>
          </template>

          <!-- 原有验证码区域（非邮箱注册模式） -->
          <template v-else>
            <div class="w-full mt-4" v-if="captchaMode === 'local'">
            <label class="block text-xs text-gray-500 dark:text-gray-300">验证码</label>
            <div class="flex items-center gap-3 mt-2">
              <input v-model="captchaInput"
                class="block w-full px-4 py-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
                type="text" placeholder="请输入验证码" aria-label="验证码"
              />
              <div class="flex flex-col items-center gap-1">
                <div class="sc-captcha-box bg-gray-100 dark:bg-gray-700 flex items-center justify-center rounded cursor-pointer"
                  @click.prevent="reloadCaptchaImage" title="点击刷新">
                  <img v-if="captchaImageUrl" :src="captchaImageUrl" alt="captcha" class="sc-captcha-img" />
                  <span v-else class="text-xs text-gray-500">加载中</span>
                </div>
                <button type="button" @click.prevent="reloadCaptchaImage"
                  class="text-xs text-blue-500" :disabled="captchaLoading">
                  {{ captchaLoading ? '刷新中' : '刷新' }}
                </button>
              </div>
            </div>
            <p v-if="captchaError" class="mt-1 text-xs text-red-500">{{ captchaError }}</p>
          </div>

          <div class="w-full mt-4" v-else-if="captchaMode === 'turnstile'">
            <label class="block text-xs text-gray-500 dark:text-gray-300">人机验证</label>
            <div class="mt-2 rounded border border-gray-200 dark:border-gray-600 bg-white dark:bg-gray-800">
              <div ref="turnstileContainer" class="flex items-center justify-center min-h-[90px] py-2"></div>
            </div>
            <div class="flex justify-end mt-2">
              <button type="button" class="text-xs text-blue-500" :disabled="turnstileLoading"
                @click.prevent="renderTurnstileWidget">
                {{ turnstileLoading ? '加载中' : '刷新' }}
              </button>
            </div>
            <p v-if="turnstileError" class="mt-1 text-xs text-red-500">{{ turnstileError }}</p>
          </div>
          </template>

          <!-- <div class="w-full mt-4">
            <input v-model="form.password2"
              class="block w-full px-4 py-2 mt-2 text-gray-700 placeholder-gray-500 bg-white border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:placeholder-gray-400 focus:border-blue-400 dark:focus:border-blue-300 focus:ring-opacity-40 focus:outline-none focus:ring focus:ring-blue-300"
              type="password" placeholder="重复密码" aria-label="重复密码" />
          </div> -->

          <div class="flex items-center justify-between mt-4">
            <div></div>
            <!-- <a href="#" class="text-sm text-gray-600 dark:text-gray-200 hover:text-gray-500">忘记密码</a> -->

            <button @click.prevent="signUp"
              class="px-6 py-2 text-sm font-medium tracking-wide text-white capitalize transition-colors duration-300 transform bg-blue-500 rounded-lg hover:bg-blue-400 focus:outline-none focus:ring focus:ring-blue-300 focus:ring-opacity-50">
              注册
            </button>
          </div>
        </form>
      </div>

      <div class="flex items-center justify-center py-4 text-center bg-gray-50 dark:bg-gray-700">
        <span class="text-sm text-gray-600 dark:text-gray-200">已有账号 ？</span>
        <router-link :to="{ name: 'user-signin' }"
          class="mx-2 text-sm font-bold text-blue-500 dark:text-blue-400 hover:underline">登录</router-link>
      </div>
    </div>
    <div class="w-full max-w-sm mx-auto overflow-hidden rounded-lg shadow-md sign-up-card"
      :class="{ 'has-bg': hasLoginBg }" v-else>
      <div class="p-6">你来晚了，门已经悄然关闭。</div>
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
  background: white;
  max-height: 100%;
}

:global(.dark) .sign-up-card {
  background: #1f2937;
}

.sign-up-card.has-bg {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(8px);
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.1);
}

:global(.dark) .sign-up-card.has-bg {
  background: rgba(31, 41, 55, 0.85);
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.3);
}
</style>
