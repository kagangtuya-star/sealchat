<script lang="tsx" setup>
import { useUserStore } from '@/stores/user';
import { useUtilsStore } from '@/stores/utils';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import Avatar from '@/components/avatar.vue'
import AvatarEditor from '@/components/AvatarEditor.vue'
import { api, urlBase } from '@/stores/_config';
import { useMessage } from 'naive-ui';
import { useI18n } from 'vue-i18n'
import router from '@/router';
import type { ServerConfig } from '@/types';

const { t } = useI18n()

const user = useUserStore();
const utils = useUtilsStore();
const message = useMessage()

const model = ref({
  nickname: '',
  brief: ''
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

// Captcha state
const captchaId = ref('');
const captchaInput = ref('');
const captchaImageSeed = ref(0);
const captchaLoading = ref(false);
const captchaError = ref('');
const captchaVerified = ref(false);

const captchaImageUrl = computed(() => {
  if (!captchaId.value) return '';
  return `${urlBase}/api/v1/captcha/${captchaId.value}.png?scene=${CAPTCHA_SCENE}&ts=${captchaImageSeed.value}`;
});

const emailPattern = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;

onMounted(async () => {
  await user.infoUpdate();
  model.value.nickname = user.info.nick;
  model.value.brief = user.info.brief;

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

const resetCaptchaState = () => {
  captchaId.value = '';
  captchaInput.value = '';
  captchaImageSeed.value = 0;
  captchaError.value = '';
  captchaLoading.value = false;
  captchaVerified.value = false;
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
  }

  emailCodeSending.value = true;
  try {
    await user.sendBindEmailCode({
      email,
      captchaId: captchaVerified.value ? '' : captchaId.value,
      captchaValue: captchaVerified.value ? '' : captchaInput.value.trim(),
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
    message.error(e?.response?.data?.error || '发送失败');
    if (!captchaVerified.value && captchaMode.value === 'local') {
      await fetchCaptcha();
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
</style>
