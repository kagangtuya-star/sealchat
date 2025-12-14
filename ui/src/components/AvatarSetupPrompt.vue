<script setup lang="ts">
import { computed } from 'vue';
import { NModal, NButton, NIcon } from 'naive-ui';
import { Camera } from '@vicons/tabler';
import Avatar from '@/components/avatar.vue';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';

const props = withDefaults(defineProps<{
  show: boolean;
}>(), {
  show: false,
});

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void;
  (event: 'setup'): void;
  (event: 'skip'): void;
}>();

const { t } = useI18n();
const user = useUserStore();

const showModal = computed({
  get: () => props.show,
  set: (val: boolean) => emit('update:show', val),
});

const handleSetup = () => {
  emit('setup');
  showModal.value = false;
};

const handleSkip = () => {
  emit('skip');
  showModal.value = false;
};
</script>

<template>
  <n-modal
    v-model:show="showModal"
    preset="card"
    :bordered="false"
    :closable="true"
    :mask-closable="true"
    class="avatar-setup-modal"
    style="width: 360px; max-width: 90vw;"
    @close="handleSkip"
  >
    <div class="avatar-prompt">
      <div class="avatar-prompt__icon-wrap">
        <Avatar :src="user.info.avatar" :size="80" />
        <div class="avatar-prompt__camera-badge">
          <n-icon :component="Camera" size="18" />
        </div>
      </div>

      <h3 class="avatar-prompt__title">{{ t('avatarPrompt.title') }}</h3>
      <p class="avatar-prompt__desc">{{ t('avatarPrompt.description') }}</p>

      <div class="avatar-prompt__actions">
        <n-button type="primary" @click="handleSetup">
          {{ t('avatarPrompt.action') }}
        </n-button>
        <n-button quaternary @click="handleSkip">
          {{ t('avatarPrompt.skip') }}
        </n-button>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.avatar-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 1rem 0.5rem;
}

.avatar-prompt__icon-wrap {
  position: relative;
  margin-bottom: 1.25rem;
}

.avatar-prompt__camera-badge {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--sc-primary, #3b82f6);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.4);
}

.avatar-prompt__title {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0 0 0.5rem;
  color: var(--sc-text-primary, #1f2937);
}

.avatar-prompt__desc {
  font-size: 0.875rem;
  color: var(--sc-text-secondary, #6b7280);
  margin: 0 0 1.5rem;
  line-height: 1.5;
}

.avatar-prompt__actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  width: 100%;
}

.avatar-prompt__actions .n-button {
  width: 100%;
}
</style>
