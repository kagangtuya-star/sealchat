<script setup lang="ts">
import { computed } from 'vue';
import { useChatStore } from '@/stores/chat';

const props = defineProps<{
  visible: boolean;
  worldId: string;
  ownerNickname: string;
}>();
const emit = defineEmits(['update:visible']);

const chat = useChatStore();

const close = () => emit('update:visible', false);

const confirmButtonText = computed(() => {
  return `我已知晓GM「${props.ownerNickname || '管理员'}」开启了此功能`;
});

const handleConfirm = async () => {
  try {
    await chat.worldAckEditNotice(props.worldId);
    close();
  } catch (e) {
    console.error('确认编辑通知失败', e);
  }
};
</script>

<template>
  <n-modal
    :show="props.visible"
    preset="dialog"
    title="管理员编辑权限提示"
    :mask-closable="false"
    :closable="true"
    @update:show="close"
  >
    <div class="notice-content">
      <p>该世界已开启"允许管理员编辑其他成员发言"功能。</p>
      <p class="notice-sub">这意味着世界管理员可以编辑您发送的消息内容。</p>
    </div>
    <template #action>
      <n-button type="primary" @click="handleConfirm">
        {{ confirmButtonText }}
      </n-button>
    </template>
  </n-modal>
</template>

<style scoped>
.notice-content {
  padding: 8px 0 16px;
}

.notice-content p {
  margin: 0 0 8px;
  color: var(--sc-text-primary);
}

.notice-sub {
  font-size: 13px;
  color: var(--sc-text-secondary);
}
</style>
