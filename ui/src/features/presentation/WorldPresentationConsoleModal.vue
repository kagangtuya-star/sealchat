<script setup lang="ts">
import { ref } from 'vue'
import { NButton, NIcon, NModal, useDialog } from 'naive-ui'
import { X } from '@vicons/tabler'
import type { WorldPresentationLiveControl } from './world-presentation-runtime'
import WorldPresentationWorkspace from './WorldPresentationWorkspace.vue'

defineProps<{ worldId: string, live?: WorldPresentationLiveControl }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ saved: [revision: string] }>()
const dialog = useDialog()
const workspace = ref<InstanceType<typeof WorldPresentationWorkspace> | null>(null)

const requestClose = () => {
  if (workspace.value?.isSaving()) return
  if (!workspace.value?.hasUnsavedChanges()) {
    show.value = false
    return
  }
  dialog.warning({
    title: '关闭演出控制台',
    content: '还有未保存的修改，关闭后将会丢失。',
    positiveText: '放弃修改并关闭',
    negativeText: '继续编辑',
    onPositiveClick: () => { show.value = false },
  })
}

// Esc inside a field or picker belongs to that control (closing its menu), not the console.
const handleEscape = (event: KeyboardEvent) => {
  const target = event.target instanceof Element ? event.target : null
  if (event.defaultPrevented || target?.closest('input, textarea, [contenteditable], .n-base-selection, .n-color-picker')) return
  requestClose()
}
</script>

<template>
  <!--
    The effect / sequence editors open nested modals that teleport outside this modal;
    the sequence editor runs without a focus trap, so this console relaxes its own trap.
    Its Select/ColorPicker followers still mount inside the root div.
  -->
  <n-modal
    :show="show"
    :mask-closable="false"
    :auto-focus="false"
    :trap-focus="false"
    :close-on-esc="false"
    @update:show="value => { if (!value) requestClose() }"
  >
    <div class="wp-console-shell" role="dialog" aria-modal="true" aria-label="演出与特效" @keydown.esc="handleEscape">
      <WorldPresentationWorkspace ref="workspace" :world-id="worldId" :live="live" @saved="emit('saved', $event)">
        <template #header-extra>
          <n-button quaternary circle size="small" :disabled="workspace?.isSaving()" aria-label="关闭演出控制台" title="关闭" @click="requestClose">
            <template #icon><n-icon><X /></n-icon></template>
          </n-button>
        </template>
      </WorldPresentationWorkspace>
    </div>
  </n-modal>
</template>

<style scoped>
.wp-console-shell {
  width: min(1240px, calc(100vw - 48px));
  height: min(820px, calc(100dvh - 48px));
  border: 1px solid var(--sc-border-strong);
  border-radius: 14px;
  background: var(--sc-bg-surface);
  box-shadow: 0 24px 80px rgba(0, 0, 0, .28);
}

@media (max-width: 640px) {
  .wp-console-shell {
    width: 100vw;
    height: 100dvh;
    border: 0;
    border-radius: 0;
  }
}
</style>
