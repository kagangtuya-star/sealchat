<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { NResult } from 'naive-ui'
import WorldClueBoardApp from '@/components/world-clue-board/WorldClueBoardApp.vue'

const props = defineProps<{
  resourceId: string
  worldId: string
  channelId: string
}>()

const emit = defineEmits<{
  ready: []
  unavailable: [message: string]
  error: [message: string]
}>()

const isMainResource = computed(() => props.resourceId === 'main')

onMounted(() => {
  if (!isMainResource.value) emit('unavailable', '画板验证页仅接受 resourceId=main')
})
</script>

<template>
  <WorldClueBoardApp
    v-if="isMainResource"
    :resource-id="resourceId"
    :world-id="worldId"
    :channel-id="channelId"
    @ready="emit('ready')"
    @unavailable="emit('unavailable', $event)"
    @error="emit('error', $event)"
  />
  <n-result
    v-else
    status="404"
    title="未知画板资源"
    description="本阶段仅支持 resourceId=main"
  />
</template>

<style scoped>
:deep(.n-result) {
  display: flex;
  width: 100%;
  height: 100%;
  align-items: center;
  justify-content: center;
}
</style>
