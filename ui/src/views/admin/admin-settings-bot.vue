<script setup lang="tsx">
import AvatarEditor from '@/components/AvatarEditor.vue';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { useChatStore, chatEvent } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useDialog, useMessage } from 'naive-ui';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n()

const emit = defineEmits(['close']);

const cancel = () => {
  emit('close');
}

const showModal = ref(false);
const editingToken = ref<any | null>(null);
const newTokenName = ref('bot')
const newTokenAvatar = ref('')
const newTokenColor = ref('#2563eb')
const avatarFileInputRef = ref<HTMLInputElement | null>(null)
const avatarEditorVisible = ref(false)
const avatarEditorFile = ref<File | null>(null)
const avatarPreview = ref('')
let avatarPreviewObjectUrl: string | null = null
const uploadingAvatar = ref(false)
const avatarVersion = ref(0)

const appendAvatarVersion = (url: string, version?: number | string) => {
  if (!url || !version) {
    return url
  }
  const mark = url.includes('?') ? '&' : '?'
  return `${url}${mark}v=${encodeURIComponent(String(version))}`
}

const botAvatarDisplay = computed(() => {
  const base = avatarPreview.value || resolveAttachmentUrl(newTokenAvatar.value)
  return appendAvatarVersion(base, avatarPreview.value ? undefined : avatarVersion.value)
})

const clearAvatarPreview = () => {
  if (avatarPreviewObjectUrl) {
    URL.revokeObjectURL(avatarPreviewObjectUrl)
    avatarPreviewObjectUrl = null
  }
  avatarPreview.value = ''
}

const setAvatarPreview = (file: File) => {
  clearAvatarPreview()
  avatarPreviewObjectUrl = URL.createObjectURL(file)
  avatarPreview.value = avatarPreviewObjectUrl
}
// const newChannel = async () => {
//   if (!newChannelName.value.trim()) {
//     message.error(t('dialoChannelgNew.channelNameHint'));
//     return;
//   }
//   await chat.channelCreate(newChannelName.value);
//   await chat.channelList();
// }

const resetForm = () => {
  newTokenName.value = 'bot';
  newTokenAvatar.value = '';
  newTokenColor.value = '#2563eb';
  clearAvatarPreview();
};

const openCreateModal = () => {
  editingToken.value = null;
  resetForm();
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
  showModal.value = true;
};

const resolveBotAvatarValue = (token?: any) => {
  if (!token) return '';
  return token.avatar || token.avatarAttachmentId || token.avatar_id || token.avatarId || token.avatar_attachment_id || '';
};

const openEditModal = (token: any) => {
  editingToken.value = token;
  newTokenName.value = token.name || 'bot';
  newTokenAvatar.value = resolveBotAvatarValue(token);
  newTokenColor.value = token.nickColor || '#2563eb';
  clearAvatarPreview();
  avatarEditorVisible.value = false;
  avatarEditorFile.value = null;
  showModal.value = true;
};

const submitToken = async () => {
  const payload = {
    name: newTokenName.value.trim() || 'bot',
    avatar: newTokenAvatar.value.trim(),
    nickColor: newTokenColor.value,
  };
  try {
    if (editingToken.value) {
      await utils.botTokenUpdate({
        id: editingToken.value.id,
        ...payload,
      });
      message.success('更新成功');
    } else {
      await utils.botTokenAdd(payload);
      message.success('添加成功');
    }
    refresh();
    chat.invalidateBotListCache();
    chatEvent.emit('bot-list-updated');
    showModal.value = false;
    if (!editingToken.value) {
      resetForm();
    }
  } catch (error) {
    message.error((editingToken.value ? '更新失败: ' : '添加失败: ') + ((error as any).response?.data?.message || '未知错误'));
  }
};

// const tokens = ref([
//   { name: '海豹', value: 'KHhD0rCfVnXVQEBybZIBm5FND10s0EQE', expireAt: 123 }
// ])
const tokens = ref({
  total: 0,
  items: [] as any[]
})

const utils = useUtilsStore();
const chat = useChatStore();
const message = useMessage()
const dialog = useDialog()

const refresh = async () => {
  const resp = await utils.botTokenList();
  tokens.value = resp.data;
}

const deleteItem = async (i: any) => {
  dialog.warning({
    title: t('dialogLogOut.title'),
    content: '确定要删除吗？',
    positiveText: t('dialogLogOut.positiveText'),
    negativeText: t('dialogLogOut.negativeText'),
    onPositiveClick: async () => {
      try {
        await utils.botTokenDelete(i.id);
        message.success('删除成功');
        refresh();
        chat.invalidateBotListCache();
        chatEvent.emit('bot-list-updated');
      } catch (error) {
        message.error('删除失败: ' + (error as any).response?.data?.message || '未知错误');
      }
    },
    onNegativeClick: () => {
    }
  })
}

const resolveAvatar = (value?: string, version?: number | string) => {
  if (!value) {
    return ''
  }
  const resolved = resolveAttachmentUrl(value)
  return appendAvatarVersion(resolved, version)
}

const triggerAvatarUpload = () => {
  avatarFileInputRef.value?.click()
}

const handleAvatarFileChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input?.files?.[0]
  if (!file) {
    return
  }
  // Check file size before uploading
  const sizeLimit = utils.fileSizeLimit
  if (file.size > sizeLimit) {
    const limitMB = (sizeLimit / 1024 / 1024).toFixed(1)
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`)
    if (input) {
      input.value = ''
    }
    return
  }
  avatarEditorFile.value = file
  avatarEditorVisible.value = true
  if (input) {
    input.value = ''
  }
}

const handleAvatarEditorSave = async (file: File) => {
  uploadingAvatar.value = true
  avatarEditorVisible.value = false
  avatarEditorFile.value = null
  setAvatarPreview(file)
  try {
    const result = await uploadImageAttachment(file, { channelId: 'bot-avatar', skipCompression: true })
    if (!result.attachmentId) {
      throw new Error('上传失败')
    }
    newTokenAvatar.value = result.attachmentId
    avatarVersion.value = Date.now()
    message.success('头像上传成功')
  } catch (error: any) {
    message.error(error?.message || '头像上传失败')
  } finally {
    uploadingAvatar.value = false
  }
}

const handleAvatarEditorCancel = () => {
  avatarEditorVisible.value = false
  avatarEditorFile.value = null
}

const clearBotAvatar = () => {
  newTokenAvatar.value = ''
  clearAvatarPreview()
}

onUnmounted(() => {
  clearAvatarPreview()
})

watch(showModal, (visible) => {
  if (visible) {
    return
  }
  avatarEditorVisible.value = false
  avatarEditorFile.value = null
  clearAvatarPreview()
})

onMounted(async () => {
  refresh()
})

watch(newTokenAvatar, (value, oldValue) => {
  if (!value || value === oldValue) {
    return
  }
  avatarVersion.value = Date.now()
})
</script>

<template>
  <div class="overflow-y-auto pr-2" style="max-height: 61vh;  margin-top: 0;">
    <n-list>
      <template #header>
        <div>当前token列表</div>
        <p class="bot-list-hint">创建机器人后，可在频道的掷骰面板点击设置齿轮，启用"机器人骰点"并选择对应机器人。</p>
      </template>

      <n-list-item v-for="i in tokens.items" :key="i.id">
        <template #suffix>
          <div class="flex items-center space-x-2">
            <div style="width: 9rem;">
              <span>到期时间</span>
              <n-date-picker v-model:value="i.expiresAt" type="date" />
              <!-- <div v-else>无期限</div> -->
            </div>
            <div class="flex flex-col space-y-1">
              <span>操作</span>
              <div class="space-x-2">
                <n-button size="small" @click="openEditModal(i)">编辑</n-button>
                <n-button size="small" @click="deleteItem(i)">删除</n-button>
              </div>
            </div>
          </div>
        </template>
        <n-thing :title="i.name" :description="i.token">
          <template #avatar>
            <img
              v-if="resolveBotAvatarValue(i)"
              :src="resolveAvatar(resolveBotAvatarValue(i), i.updatedAt)"
              style="width: 28px; height: 28px; min-width: 28px; min-height: 28px; border-radius: 3px; object-fit: cover;"
            />
            <n-avatar v-else size="small">
              {{ i.name?.slice(0, 1) || 'B' }}
            </n-avatar>
          </template>
          <template #header-extra>
            <div class="flex items-center space-x-1 text-xs text-gray-500">
              <span>昵称色彩</span>
              <span class="bot-color-chip" :style="i.nickColor ? { backgroundColor: i.nickColor } : undefined"></span>
              <span>{{ i.nickColor || '默认' }}</span>
            </div>
          </template>
        </n-thing>
      </n-list-item>

      <template #footer>
        <n-button @click="openCreateModal">添加</n-button>
      </template>
    </n-list>
  </div>
  <div class="space-x-2 float-right">
    <n-button @click="cancel">关闭</n-button>
    <!-- <n-button type="primary" :disabled="!modified" @click="save">保存</n-button> -->
  </div>
  <n-modal v-model:show="showModal" preset="dialog" :title="editingToken ? '编辑机器人' : '配置机器人外观'" :positive-text="editingToken ? '保存' : $t('dialoChannelgNew.positiveText')"
    :negative-text="$t('dialoChannelgNew.negativeText')" @positive-click="submitToken">
    <n-form label-placement="top">
      <n-form-item label="机器人名称">
        <n-input v-model:value="newTokenName" placeholder="机器人名称" />
      </n-form-item>
      <n-form-item label="机器人头像">
        <input ref="avatarFileInputRef" type="file" accept="image/*" class="hidden" @change="handleAvatarFileChange">
        <div class="bot-avatar-uploader">
          <img
            v-if="botAvatarDisplay"
            :src="botAvatarDisplay"
            class="bot-avatar-uploader__preview"
          />
          <n-avatar v-else size="large">
            {{ newTokenName.slice(0, 1) || 'B' }}
          </n-avatar>
          <div class="bot-avatar-uploader__actions">
            <n-space>
              <n-button size="tiny" :loading="uploadingAvatar" @click="triggerAvatarUpload">上传头像</n-button>
              <n-button size="tiny" quaternary :disabled="!newTokenAvatar" @click="clearBotAvatar">清除</n-button>
            </n-space>
            <n-input v-model:value="newTokenAvatar" size="small" placeholder="也可粘贴图片地址或附件ID" @update:value="clearAvatarPreview" />
            <p class="bot-avatar-uploader__hint">支持本地上传，系统会返回附件ID，以 <code>id:xxxxx</code> 开头。</p>
          </div>
        </div>
      </n-form-item>
      <n-form-item label="昵称色彩">
        <div class="flex items-center space-x-3 w-full">
          <n-color-picker v-model:value="newTokenColor" :modes="['hex']" :show-alpha="false" size="small" />
          <span class="text-xs text-gray-500">用于频道中展示机器人昵称颜色</span>
        </div>
      </n-form-item>
    </n-form>
  </n-modal>
  <n-modal
    v-model:show="avatarEditorVisible"
    preset="card"
    title="编辑头像"
    style="max-width: 450px;"
    :mask-closable="false"
  >
    <AvatarEditor
      :file="avatarEditorFile"
      @save="handleAvatarEditorSave"
      @cancel="handleAvatarEditorCancel"
    />
  </n-modal>
</template>

<style scoped>
.bot-color-chip {
  width: 0.85rem;
  height: 0.85rem;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.4);
  display: inline-block;
}

.bot-avatar-uploader {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.bot-avatar-uploader__preview {
  width: 40px;
  height: 40px;
  min-width: 40px;
  min-height: 40px;
  border-radius: 3px;
  object-fit: cover;
}

.bot-avatar-uploader__actions {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bot-avatar-uploader__hint {
  font-size: 12px;
  color: #94a3b8;
  margin: 0;
}

.bot-list-hint {
  font-size: 12px;
  color: #94a3b8;
  margin: 0.25rem 0 0;
}
</style>
