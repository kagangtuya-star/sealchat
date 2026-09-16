<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import TheaterPresentationEditorModal from '@/components/theater-presentation/TheaterPresentationEditorModal.vue'
import { dialogueControllerStates, readDialogueController, writeDialogueController, publishDialogueController, type DialogueControllerTemplate } from '@/views/theater/dialogue/theater-dialogue-controller'
import { chatEvent } from '@/stores/chat'

const props = defineProps<{ worldId: string; channelId: string; visible: boolean; theaterMode: boolean }>()
const emit = defineEmits<{ requestEnterTheater: [identityId: string] }>()
const message = useMessage()
const show = ref(false)
const saving = ref(false)
const failed = ref(false)
const state = computed(() => dialogueControllerStates.value[props.worldId])
let generation = 0
const load = async () => {
  if (!props.worldId || !props.visible) return
  const epoch = ++generation
  failed.value = false
  try { await readDialogueController(props.worldId) }
  catch { if (epoch === generation) failed.value = true }
}
const open = async () => {
  const world = props.worldId, channel = props.channelId
  await load()
  if (world !== props.worldId || channel !== props.channelId || !props.visible || failed.value || !state.value?.controller.enabled || !state.value.canManage || !state.value.template) return
  if (!props.theaterMode) {
    emit('requestEnterTheater', state.value.controller.sharedIdentityId)
    return
  }
  show.value = true
}
const save = async (template: DialogueControllerTemplate) => {
  if (!show.value || saving.value) return
  const world = props.worldId
  saving.value = true
  try { await writeDialogueController(world, { template }); if (world === props.worldId) show.value = false }
  catch { if (world === props.worldId) message.error('公共演出设定保存失败，草稿已保留') }
  finally { if (world === props.worldId) saving.value = false }
}
const updated = () => { if (props.visible) void load() }
const fromHost = (event: MessageEvent) => {
  if (event.origin !== window.location.origin || event.source !== window.parent || event.data?.type !== 'sealchat.theater.dialogue-controller.state' || event.data.worldId !== props.worldId) return
  publishDialogueController(props.worldId, event.data.state)
}
watch(() => [props.worldId, props.channelId, props.visible], () => { generation++; show.value = false; saving.value = false; if (props.visible) void load() }, { immediate: true, flush: 'sync' })
watch(() => state.value?.controller.enabled, enabled => { if (!enabled) show.value = false })
onMounted(() => { chatEvent.on('theater.mutation.applied' as never, updated); window.addEventListener('message', fromHost) })
onBeforeUnmount(() => { generation++; chatEvent.off('theater.mutation.applied' as never, updated); window.removeEventListener('message', fromHost) })
defineExpose({ open })
</script>
<template>
  <div v-if="state?.controller.enabled && state.controller.sharedIdentityId" class="controller-identity">
    <div><strong>全局对话框</strong><small>世界级系统角色</small></div>
    <n-button size="small" :disabled="!state.canManage" @click="open">小剧场演出设定</n-button>
    <TheaterPresentationEditorModal v-if="state.template" v-model:show="show" mode="controller" :presentation="state.template.presentation" :controller-template="state.template" :world-template="null" :channel-id="channelId" :identity-id="state.controller.sharedIdentityId" preview-name="实际发言角色" :applying="saving" @apply-controller="save" />
  </div>
  <div v-if="failed" role="alert">全局对话框读取失败 <n-button text @click="load">重试</n-button></div>
</template>
<style scoped>
.controller-identity { display: flex; justify-content: space-between; align-items: center; padding: 12px; margin-bottom: 12px; border-bottom: 1px solid var(--sc-border-mute, #8884); }
small { display: block; opacity: .6; margin-top: 4px; }
</style>
