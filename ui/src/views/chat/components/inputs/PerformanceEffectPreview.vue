<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import TwinLayerMessage from '@/components/chat/TwinLayerMessage.vue';
import type { PerformanceMarkAttrs } from '@/utils/tiptap-performance-mark';

const props = withDefaults(defineProps<{
  attrs: PerformanceMarkAttrs
  text?: string
}>(), {
  text: '效果预览',
});

const previewRef = ref<InstanceType<typeof TwinLayerMessage> | null>(null);
let replayTimer: number | null = null;

const content = computed(() => JSON.stringify({
  type: 'doc',
  content: [
    {
      type: 'paragraph',
      content: [
        {
          type: 'text',
          text: props.text,
          marks: [
            {
              type: 'performance',
              attrs: props.attrs,
            },
          ],
        },
      ],
    },
  ],
}));

const replay = () => {
  void previewRef.value?.replay?.();
};

onMounted(() => {
  replayTimer = window.setInterval(replay, 3200);
});

onBeforeUnmount(() => {
  if (replayTimer != null) {
    window.clearInterval(replayTimer);
  }
});
</script>

<template>
  <TwinLayerMessage
    ref="previewRef"
    class="performance-effect-preview__message"
    :content="content"
    :autoplay="true"
  />
</template>

<style scoped>
.performance-effect-preview__message {
  min-height: 1.45rem;
  font-size: 0.8rem;
  line-height: 1.45rem;
  text-align: center;
  white-space: nowrap;
}
</style>
