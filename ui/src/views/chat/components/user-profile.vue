<script lang="tsx" setup>
import { useUserStore } from '@/stores/user';
import { useUtilsStore } from '@/stores/utils';
import { onMounted, ref } from 'vue';
import Avatar from '@/components/avatar.vue'
import AvatarEditor from '@/components/AvatarEditor.vue'
import { api } from '@/stores/_config';
import { useMessage } from 'naive-ui';
import { useI18n } from 'vue-i18n'
import router from '@/router';

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

onMounted(async () => {
  await user.infoUpdate();
  model.value.nickname = user.info.nick;
  model.value.brief = user.info.brief;
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
    }
  } catch (error) {
    message.error('出错了，请刷新重试或联系管理员: ' + (error as any).toString())
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
</script>

<template>
  <div class="pointer-events-auto relative border px-4 py-2 rounded-md" style="min-width: 20rem;">
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
        <n-button @click="passwordChange">修改密码</n-button>
      </n-form-item>
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
