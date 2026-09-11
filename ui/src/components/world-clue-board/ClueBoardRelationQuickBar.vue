<script setup lang="ts">
import { NButton, NIcon, NTooltip } from 'naive-ui'
import { AlertTriangle, ArrowForwardUp, Check, FileText, Link } from '@vicons/tabler'
import type { WorldClueBoardRelationKind } from '@/stores/worldClueBoard'

const emit = defineEmits<{ start: [kind: WorldClueBoardRelationKind] }>()

const actions: Array<{ kind: WorldClueBoardRelationKind; label: string; icon: typeof Link }> = [
  { kind: 'related', label: '相关', icon: Link },
  { kind: 'supports', label: '支持', icon: Check },
  { kind: 'contradicts', label: '矛盾', icon: AlertTriangle },
  { kind: 'causes', label: '因果', icon: ArrowForwardUp },
  { kind: 'references', label: '引用', icon: FileText },
]
</script>

<template>
  <div class="clue-board-relation-quickbar" role="toolbar" aria-label="关系快捷操作">
    <NTooltip v-for="action in actions" :key="action.kind">
    <template #trigger><NButton
      quaternary
      circle
      size="small"
      :title="action.label"
      :aria-label="action.label"
      @pointerdown.stop
      @click.stop="emit('start', action.kind)"
    >
      <template #icon><NIcon><component :is="action.icon" /></NIcon></template>
    </NButton></template>{{ action.label }}</NTooltip>
  </div>
</template>

<style scoped>
.clue-board-relation-quickbar { display: flex; align-items: center; gap: 2px; padding: 3px 5px; border: 1px solid color-mix(in srgb, var(--primary-color, #3388de) 40%, var(--sc-border-mute)); border-radius: 999px; background: color-mix(in srgb, var(--sc-bg-elevated) 96%, transparent); box-shadow: 0 8px 24px #0003; pointer-events: auto; }
.clue-board-relation-quickbar :deep(.n-button) { color: var(--sc-text-secondary); }
.clue-board-relation-quickbar :deep(.n-button:hover) { color: var(--primary-color, #3388de); background: color-mix(in srgb, var(--primary-color, #3388de) 12%, transparent); }
</style>
