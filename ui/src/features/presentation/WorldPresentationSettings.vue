<script setup lang="ts">
import { ref } from 'vue'
import { NButton, NIcon } from 'naive-ui'
import { Stars } from '@vicons/tabler'
import WorldPresentationConsoleModal from './WorldPresentationConsoleModal.vue'

defineProps<{ worldId: string }>()
const show = ref(false)
</script>

<template>
  <section class="world-presentation-entry" aria-label="演出与特效">
    <span class="world-presentation-entry__icon"><n-icon><Stars /></n-icon></span>
    <div class="world-presentation-entry__text">
      <strong>演出与特效</strong>
      <p>管理世界级特效、消息关键词触发与特效序列，对本世界所有频道生效。聊天页「更多跑团功能 → 演出与特效」可打开同一个控制台并现场播放。</p>
    </div>
    <n-button size="small" secondary type="primary" @click="show = true">打开演出控制台</n-button>
  </section>
  <WorldPresentationConsoleModal v-model:show="show" :world-id="worldId" />
</template>

<style scoped>
.world-presentation-entry { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 12px; margin-block: 12px; padding: 12px 14px; border: 1px solid var(--sc-border-strong, #8885); border-radius: 10px; }
.world-presentation-entry__icon { width: 34px; height: 34px; display: grid; place-items: center; border-radius: 9px; color: var(--sc-text-primary); background: var(--sc-chip-bg, #8881); font-size: 18px; }
.world-presentation-entry__text { min-width: 0; display: grid; gap: 2px; }
.world-presentation-entry__text strong { font-size: 14px; }
.world-presentation-entry__text p { margin: 0; color: var(--sc-text-secondary); font-size: 12px; line-height: 1.6; }
@media (max-width: 560px) {
  .world-presentation-entry { grid-template-columns: auto minmax(0, 1fr); }
  .world-presentation-entry > .n-button { grid-column: 1 / -1; }
}
</style>
