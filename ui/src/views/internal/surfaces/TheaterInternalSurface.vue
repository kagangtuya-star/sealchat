<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import TheaterView from '@/views/theater/host/TheaterView.vue'

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
const route = useRoute()
const validResource = computed(() => props.resourceId === props.worldId)
const context = computed(() => ({ worldId: props.worldId, channelId: props.channelId }))

onMounted(() => {
  if (!validResource.value) emit('unavailable', '小剧场资源与世界不匹配')
})
</script>

<template>
  <TheaterView
    v-if="validResource"
    :key="route.query.full === '1' ? 'full' : 'presentation'"
    :surface-context="context"
    :transparent-background="route.query.transparent === '1'"
    :full-mode="route.query.full === '1'"
    @ready="emit('ready')"
    @error="emit('error', $event)"
  />
</template>
