<script setup lang="ts">
import router from '@/router';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { NButton, NForm, NFormItem, NInput, NInputGroup, NSteps, NStep, useMessage } from 'naive-ui';
import { useUserStore } from '@/stores/user';
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

const CAPTCHA_SCENE = 'password_reset';
let turnstileScriptPromise: Promise<void> | null = null;

const message = useMessage();
const userStore = useUserStore();
const utils = useUtilsStore();
const config = ref<ServerConfig | null>(null);

// 步骤状态：1=验证身份 2=重置密码 3=完成
const currentStep = ref(1);

// 步骤1的表单
const form = ref({
  account: '',
});

// 步骤2的数据
const verifiedUser = ref({
  username: '',
  email: '',
  maskedEmail: '',
  needEmailConfirm: false, // 是否需要补全邮箱
});

// 邮箱补全表单
const emailConfirmInput = ref('');
const emailConfirmError = ref('');

// 步骤2的表单
const resetForm = ref({
  code: '',
  newPassword: '',
  confirmPassword: '',
});

// Captcha
const captchaId = ref('');
const captchaInput = ref('');
const captchaImageSeed = ref(0);
const captchaLoading = ref(false);
const captchaError = ref('');

// Turnstile
const turnstileContainer = ref<HTMLDivElement | null>(null);
const turnstileToken = ref('');
const turnstileWidgetId = ref<string | null>(null);
const turnstileError = ref('');
const turnstileLoading = ref(false);

// Email code
const emailCodeSending = ref(false);
const emailCodeCountdown = ref(0);
let emailCodeTimer: ReturnType<typeof setInterval> | null = null;

const verifying = ref(false);
const submitting = ref(false);

const emailAuthEnabled = computed(() => config.value?.emailAuth?.enabled ?? false);
const captchaMode = computed(() => config.value?.captcha?.passwordReset?.mode ?? config.value?.captcha?.mode ?? 'off');

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
  if (!captchaId.value) return '';
  return `${urlBase}/api/v1/captcha/${captchaId.value}.png?scene=${CAPTCHA_SCENE}&ts=${captchaImageSeed.value}`;
});

const ensureTurnstileScript = async () => {
  if (typeof window === 'undefined' || typeof document === 'undefined') return;
  if (window.turnstile) return;
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
  if (captchaMode.value !== 'local') return;
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
    console.error(err);
    captchaError.value = '验证码刷新失败，已重新生成';
    await fetchCaptcha();
  } finally {
    captchaLoading.value = false;
  }
};

const destroyTurnstile = () => {
  if (typeof window === 'undefined') return;
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
  if (typeof window === 'undefined') return;
  turnstileError.value = '';
  turnstileLoading.value = true;
  try {
    await ensureTurnstileScript();
    await nextTick();
    const siteKey = config.value?.captcha?.passwordReset?.turnstile?.siteKey?.trim()
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

// 步骤1：验证身份
const verifyIdentity = async () => {
  if (verifying.value) return;

  const account = form.value.account.trim();
  if (!account) {
    message.error('请输入用户名或邮箱');
    return;
  }

  if (captchaMode.value === 'local' && (!captchaId.value || !captchaInput.value.trim())) {
    message.error('请先填写验证码');
    return;
  }
  if (captchaMode.value === 'turnstile' && !turnstileToken.value) {
    message.error('请先完成人机验证');
    return;
  }

  verifying.value = true;
  try {
    const resp = await userStore.verifyPasswordResetIdentity({
      account,
      captchaId: captchaId.value,
      captchaValue: captchaInput.value.trim(),
      turnstileToken: turnstileToken.value,
    });

    verifiedUser.value = {
      username: resp.data.username,
      email: resp.data.email,
      maskedEmail: resp.data.maskedEmail,
      needEmailConfirm: resp.data.needEmailConfirm ?? false,
    };

    // 重置邮箱补全状态
    emailConfirmInput.value = '';
    emailConfirmError.value = '';

    currentStep.value = 2;
    message.success('身份验证成功');
  } catch (e: any) {
    message.error(e?.response?.data?.error || '验证失败');
    if (captchaMode.value === 'local') {
      fetchCaptcha();
    } else if (captchaMode.value === 'turnstile' && turnstileWidgetId.value && window.turnstile?.reset) {
      window.turnstile.reset(turnstileWidgetId.value);
      turnstileToken.value = '';
    }
  } finally {
    verifying.value = false;
  }
};

// 验证邮箱补全是否正确
const isEmailConfirmed = computed(() => {
  if (!verifiedUser.value.needEmailConfirm) return true;
  return emailConfirmInput.value.trim().toLowerCase() === verifiedUser.value.email.toLowerCase();
});

// 步骤2：发送验证码
const sendResetCode = async () => {
  if (emailCodeSending.value || emailCodeCountdown.value > 0) return;

  // 如果需要补全邮箱，先验证
  if (verifiedUser.value.needEmailConfirm) {
    const inputEmail = emailConfirmInput.value.trim().toLowerCase();
    if (!inputEmail) {
      emailConfirmError.value = '请输入完整邮箱地址';
      return;
    }
    if (inputEmail !== verifiedUser.value.email.toLowerCase()) {
      emailConfirmError.value = '邮箱地址不正确';
      return;
    }
    emailConfirmError.value = '';
  }

  emailCodeSending.value = true;
  try {
    await userStore.sendPasswordResetCode({
      account: verifiedUser.value.email,
      verified: true,
    });
    message.success('验证码已发送到您的邮箱');
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
  } finally {
    emailCodeSending.value = false;
  }
};

// 步骤2：确认重置
const confirmReset = async () => {
  const code = resetForm.value.code.trim();
  const newPassword = resetForm.value.newPassword;
  const confirmPassword = resetForm.value.confirmPassword;

  if (!code) {
    message.error('请输入验证码');
    return;
  }
  if (!newPassword) {
    message.error('请输入新密码');
    return;
  }
  if (newPassword.length < 6) {
    message.error('密码至少6位');
    return;
  }
  if (newPassword !== confirmPassword) {
    message.error('两次输入的密码不一致');
    return;
  }

  submitting.value = true;
  try {
    await userStore.confirmPasswordReset({
      account: verifiedUser.value.email,
      code,
      newPassword,
    });
    message.success('密码重置成功');
    currentStep.value = 3;
  } catch (e: any) {
    message.error(e?.response?.data?.error || '重置失败');
  } finally {
    submitting.value = false;
  }
};

const goBack = () => {
  currentStep.value = 1;
  resetForm.value = { code: '', newPassword: '', confirmPassword: '' };
  verifiedUser.value = { username: '', email: '', maskedEmail: '', needEmailConfirm: false };
  emailConfirmInput.value = '';
  emailConfirmError.value = '';
  if (emailCodeTimer) {
    clearInterval(emailCodeTimer);
    emailCodeTimer = null;
  }
  emailCodeCountdown.value = 0;
};

const goToLogin = () => {
  router.push({ name: 'user-signin' });
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
  }
});
</script>

<template>
  <div class="password-recovery-root">
    <!-- Background layers -->
    <div v-if="hasLoginBg" class="login-bg-layer" :style="loginBgStyle"></div>
    <div v-if="hasLoginBg && loginOverlayStyle" class="login-overlay-layer" :style="loginOverlayStyle"></div>

    <div class="recovery-content sc-form-scroll" :class="{ 'has-bg': hasLoginBg }">
      <h2 class="font-bold text-xl mb-6">找回密码</h2>

      <!-- 未启用邮箱认证 -->
      <div v-if="!emailAuthEnabled" class="text-center text-gray-500 dark:text-gray-400">
        <p class="mb-4">当前未启用邮箱认证功能</p>
        <p class="mb-4 text-sm">请联系管理员重置密码</p>
        <NButton type="primary" @click="goToLogin">返回登录</NButton>
      </div>

      <!-- 已启用邮箱认证 -->
      <template v-else>
        <NSteps :current="currentStep" class="mb-6 w-full max-w-md">
          <NStep title="验证身份" />
          <NStep title="重置密码" />
          <NStep title="完成" />
        </NSteps>

        <div class="w-full max-w-md px-4">
          <!-- Step 1: 验证身份 -->
          <template v-if="currentStep === 1">
            <NForm class="w-full">
              <NFormItem label="用户名或邮箱">
                <NInput v-model:value="form.account" placeholder="请输入用户名或邮箱" />
              </NFormItem>

              <!-- 本地验证码 -->
              <NFormItem v-if="captchaMode === 'local'" label="图形验证码">
                <div class="flex w-full items-center gap-3">
                  <NInput v-model:value="captchaInput" placeholder="请输入验证码" />
                  <div class="sc-captcha-box rounded bg-gray-100 dark:bg-gray-700 flex items-center justify-center cursor-pointer"
                    title="点击刷新" @click.prevent="reloadCaptchaImage">
                    <img v-if="captchaImageUrl" :src="captchaImageUrl" alt="captcha" class="sc-captcha-img" />
                    <span v-else class="text-xs text-gray-500">加载中</span>
                  </div>
                  <NButton text size="tiny" :loading="captchaLoading" @click.prevent="reloadCaptchaImage">刷新</NButton>
                </div>
                <div v-if="captchaError" class="text-xs text-red-500 mt-1">{{ captchaError }}</div>
              </NFormItem>

              <!-- Turnstile -->
              <NFormItem v-else-if="captchaMode === 'turnstile'" label="人机验证">
                <div class="w-full rounded border border-gray-200 dark:border-gray-600 py-2 flex items-center justify-center min-h-[90px]">
                  <div ref="turnstileContainer" class="w-full flex items-center justify-center"></div>
                </div>
                <div class="flex justify-end mt-1">
                  <NButton text size="tiny" :loading="turnstileLoading" @click.prevent="renderTurnstileWidget">刷新</NButton>
                </div>
                <div v-if="turnstileError" class="text-xs text-red-500 mt-1">{{ turnstileError }}</div>
              </NFormItem>

              <div class="flex justify-between mt-4">
                <NButton text @click="goToLogin">返回登录</NButton>
                <NButton type="primary" :loading="verifying" @click="verifyIdentity">
                  下一步
                </NButton>
              </div>
            </NForm>
          </template>

          <!-- Step 2: 重置密码 -->
          <template v-else-if="currentStep === 2">
            <NForm class="w-full">
              <!-- 用户信息显示 -->
              <div class="mb-4 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <div class="text-sm text-gray-600 dark:text-gray-400 mb-1">用户名</div>
                <div class="font-medium">{{ verifiedUser.username }}</div>
                <div class="text-sm text-gray-600 dark:text-gray-400 mt-2 mb-1">邮箱</div>
                <div class="font-medium">{{ verifiedUser.maskedEmail }}</div>
              </div>

              <!-- 邮箱补全（仅当需要时显示） -->
              <NFormItem v-if="verifiedUser.needEmailConfirm" label="确认邮箱">
                <NInput
                  v-model:value="emailConfirmInput"
                  placeholder="请输入完整邮箱地址以确认身份"
                  :status="emailConfirmError ? 'error' : undefined"
                />
                <template #feedback>
                  <div v-if="emailConfirmError" class="text-red-500 text-xs">{{ emailConfirmError }}</div>
                  <div v-else class="text-gray-500 text-xs">请输入您的完整邮箱地址，与上方脱敏邮箱匹配</div>
                </template>
              </NFormItem>

              <NFormItem label="邮箱验证码">
                <NInputGroup>
                  <NInput v-model:value="resetForm.code" placeholder="请输入验证码" maxlength="6" style="flex: 1" />
                  <NButton
                    type="primary"
                    :disabled="emailCodeSending || emailCodeCountdown > 0 || (verifiedUser.needEmailConfirm && !isEmailConfirmed)"
                    :loading="emailCodeSending"
                    @click="sendResetCode"
                    style="width: 110px"
                  >
                    {{ emailCodeSending ? '发送中...' : (emailCodeCountdown > 0 ? `${emailCodeCountdown}s` : '发送验证码') }}
                  </NButton>
                </NInputGroup>
              </NFormItem>

              <NFormItem label="新密码">
                <NInput v-model:value="resetForm.newPassword" type="password" placeholder="请输入新密码（至少6位）" show-password-on="click" />
              </NFormItem>

              <NFormItem label="确认密码">
                <NInput v-model:value="resetForm.confirmPassword" type="password" placeholder="请再次输入新密码" show-password-on="click" />
              </NFormItem>

              <div class="flex justify-between mt-4">
                <NButton text @click="goBack">上一步</NButton>
                <NButton type="primary" :loading="submitting" @click="confirmReset">
                  重置密码
                </NButton>
              </div>
            </NForm>
          </template>

          <!-- Step 3: 完成 -->
          <template v-else-if="currentStep === 3">
            <div class="text-center">
              <div class="text-green-500 text-5xl mb-4">✓</div>
              <p class="text-lg mb-4">密码重置成功</p>
              <NButton type="primary" @click="goToLogin">前往登录</NButton>
            </div>
          </template>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.password-recovery-root {
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

.recovery-content {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-start;
  width: 50%;
  min-width: 20rem;
  max-width: 32rem;
  max-height: 100%;
  padding: 2rem;
  transition: all 0.3s;
}

.recovery-content.has-bg {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(8px);
  border-radius: 12px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.1);
}

:global(.dark) .recovery-content.has-bg {
  background: rgba(31, 41, 55, 0.85);
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.3);
}
</style>
