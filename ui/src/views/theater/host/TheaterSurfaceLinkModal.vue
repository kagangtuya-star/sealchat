<script setup lang="ts">
import { computed, ref } from 'vue'
import { NButton, NCheckbox, NInput, NModal, NSpace, useMessage } from 'naive-ui'
import { useUtilsStore } from '@/stores/utils'
import { copyTextWithFallback } from '@/utils/clipboard'
import { generateInternalSurfaceLink, resolveInternalSurfaceLinkBase } from '@/utils/internalSurfaceLink'

const props = defineProps<{ show: boolean; worldId: string; channelId: string }>()
const emit = defineEmits<{ 'update:show': [show: boolean] }>()
const utils = useUtilsStore()
const message = useMessage()
const transparent = ref(false)
const full = ref(false)
const link = computed(() => generateInternalSurfaceLink({
  type: 'theater',
  id: props.worldId,
  worldId: props.worldId,
  channelId: props.channelId,
  transparent: transparent.value,
  full: full.value,
}, { base: resolveInternalSurfaceLinkBase(utils.config) }))
const copyLink = async () => {
  if (await copyTextWithFallback(link.value)) message.success('已复制小剧场浮层链接')
  else message.warning('复制失败，请手动复制链接')
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    title="小剧场浮层"
    :style="{ width: 'min(520px, calc(100vw - 32px))' }"
    @update:show="emit('update:show', $event)"
  >
    <NSpace vertical :size="16">
      <NCheckbox v-model:checked="transparent">是否透明背景</NCheckbox>
      <NCheckbox v-model:checked="full">完整模式</NCheckbox>
      <p class="theater-surface-link-hint">默认隐藏聊天区，可缩放、平移舞台并跳过或关闭对白。完整模式提供标准工具栏，可按账号权限操作舞台、展开聊天区。透明背景隐藏底色和场景背景层，保留前景、立绘、对白等内容。</p>
      <NInput :value="link" readonly type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" aria-label="小剧场浮层链接" />
      <NSpace justify="end">
        <NButton type="primary" @click="copyLink">复制链接</NButton>
      </NSpace>
    </NSpace>
  </NModal>
</template>

<style scoped>
.theater-surface-link-hint { margin: 0; color: var(--sc-text-secondary, #71717a); font-size: 13px; line-height: 1.6; }
</style>
