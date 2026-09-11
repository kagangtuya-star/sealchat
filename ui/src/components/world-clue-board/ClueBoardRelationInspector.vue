<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NCard, NInput, NSelect, NSpace } from 'naive-ui'
import type { WorldClueSummary } from '@/stores/worldClue'
import type { WorldClueBoardRelation, WorldClueBoardRelationEndpointRef, WorldClueBoardRelationKind } from '@/stores/worldClueBoard'
import { WORLD_CLUE_BOARD_RELATION_KIND_LABELS as relationKindLabels } from '@/stores/worldClueBoard'
import type { BoardDrawingEndpoint } from './boardTypes'

const props = defineProps<{
  relation: WorldClueBoardRelation
  summaries: WorldClueSummary[]
  drawingEndpoints?: BoardDrawingEndpoint[]
}>()
const emit = defineEmits<{
  update: [value: { id: string; kind?: WorldClueBoardRelationKind; label?: string }]
  remove: [id: string]
  reverse: [id: string]
  close: []
}>()

const kind = ref<WorldClueBoardRelationKind>('related')
const label = ref('')
const kindOptions = Object.entries(relationKindLabels).map(([value, label]) => ({ label, value }))

function endpointName(endpoint: WorldClueBoardRelationEndpointRef | undefined) {
  if (!endpoint) return '缺失的端点'
  if (endpoint.kind === 'quickdraw') return props.drawingEndpoints?.find(item => item.id === endpoint.id)?.name || '不可用的便签/文本'
  const item = props.summaries.find(summary => summary.id === endpoint.id)
  return item ? (item.title || '未命名线索') : '缺失的线索'
}

const sourceName = computed(() => endpointName(props.relation.sourceRef))
const targetName = computed(() => endpointName(props.relation.targetRef))

watch(() => props.relation, (relation) => {
  kind.value = relation.kind
  label.value = relation.label || ''
}, { immediate: true })

function commitLabel() {
  emit('update', { id: props.relation.id, label: label.value.trim() })
}

function changeKind(value: WorldClueBoardRelationKind) {
  kind.value = value
  emit('update', { id: props.relation.id, kind: value })
}
</script>

<template>
  <NCard size="small" class="clue-board-inspector" title="关系">
    <template #header-extra><NButton text size="tiny" @click="emit('close')">关闭</NButton></template>
    <div class="clue-board-inspector__endpoints">
      <span :title="sourceName">{{ sourceName }}</span><span class="clue-board-inspector__arrow">{{ kind === 'related' || kind === 'contradicts' ? '—' : '→' }}</span><span :title="targetName">{{ targetName }}</span>
    </div>
    <NSpace vertical size="small">
      <NSelect :value="kind" :options="kindOptions" @update:value="changeKind" />
      <NInput v-model:value="label" maxlength="500" placeholder="关系标签（可选）" @blur="commitLabel" @keyup.enter="commitLabel" />
      <NButton type="error" secondary size="small" @click="emit('remove', relation.id)">删除关系</NButton>
      <NButton v-if="kind !== 'related' && kind !== 'contradicts'" size="small" @click="emit('reverse', relation.id)">翻转方向</NButton>
    </NSpace>
  </NCard>
</template>

<style scoped>
.clue-board-inspector { width: 270px; max-width: calc(100vw - 28px); }
.clue-board-inspector__endpoints { display: flex; align-items: center; gap: 7px; margin-bottom: 10px; color: var(--sc-text-primary); font-size: 12px; }
.clue-board-inspector__endpoints span:first-child, .clue-board-inspector__endpoints span:last-child { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.clue-board-inspector__arrow { color: var(--sc-text-secondary); }
</style>
