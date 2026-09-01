<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue';
import type { ChannelIForm } from '@/types/iform';
import { useIFormStore } from '@/stores/iform';
import IFormEmbedFrame from '@/components/iform/IFormEmbedFrame.vue';

const props = defineProps<{
  resourceId: string;
  worldId: string;
  channelId: string;
}>();

const emit = defineEmits<{
  ready: [];
  unavailable: [message: string];
  error: [message: string];
}>();

const iformStore = useIFormStore();
const form = computed<ChannelIForm | null>(() => (
  (iformStore.formsByChannel[props.channelId] || []).find(item => item.id === props.resourceId) || null
));
let taskEpoch = 0;
let resourceReady = false;

iformStore.bootstrap();

watch(
  () => [props.resourceId, props.channelId] as const,
  async ([resourceId, channelId]) => {
    const epoch = ++taskEpoch;
    resourceReady = false;
    try {
      await iformStore.ensureForms(channelId);
      if (epoch !== taskEpoch) return;
      const matched = form.value;
      if (!matched) {
        emit('unavailable', 'IForm 不存在或当前用户不可见');
        return;
      }
      resourceReady = true;
      emit('ready');
    } catch (error: any) {
      if (epoch !== taskEpoch) return;
      emit('error', error?.response?.data?.error || error?.message || 'IForm 加载失败');
    }
  },
  { immediate: true },
);

watch(form, (nextForm, previousForm) => {
  if (!resourceReady || !previousForm || nextForm) return;
  resourceReady = false;
  emit('unavailable', 'IForm 已删除或当前用户不可见');
});

onBeforeUnmount(() => { taskEpoch += 1; });
</script>

<template>
  <IFormEmbedFrame
    v-if="form"
    class="iform-internal-surface"
    :form="form"
    :channel-id="channelId"
    :enable-channel-embed="true"
  />
</template>

<style scoped>
.iform-internal-surface {
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 0;
}
</style>
