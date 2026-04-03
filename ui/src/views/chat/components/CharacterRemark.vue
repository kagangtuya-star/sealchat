<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useChatStore } from '@/stores/chat'
import { useCharacterRemarkStore } from '@/stores/characterRemark'

const props = defineProps<{
  identityId?: string
  identityColor?: string
  channelId?: string
}>()

const chatStore = useChatStore()
const remarkStore = useCharacterRemarkStore()
const message = useMessage()

const editing = ref(false)
const draft = ref('')
const inputRef = ref<any>(null)
const lastTouchTapAt = ref(0)
const lastTouchTapX = ref(0)
const lastTouchTapY = ref(0)

const DOUBLE_TAP_MAX_DELAY_MS = 320
const DOUBLE_TAP_MAX_MOVE_PX = 24

const resolvedChannelId = computed(() => props.channelId || chatStore.curChannel?.id || '')

const remarkEntry = computed(() => {
  if (!props.identityId) return null
  return remarkStore.getRemarkByIdentity(props.identityId, resolvedChannelId.value)
})

const isEditable = computed(() => (
  !!props.identityId
  && !!resolvedChannelId.value
  && remarkStore.isOwnedByCurrentUser(resolvedChannelId.value, props.identityId)
))

const isVisible = computed(() => remarkStore.shouldShowRemark(remarkEntry.value))

const displayText = computed(() => remarkEntry.value?.content || '')

const remarkStyle = computed(() => {
  if (!props.identityColor) return {}
  return {
    backgroundColor: `${props.identityColor}0d`,
    color: props.identityColor,
    borderColor: `${props.identityColor}2b`,
  }
})

const beginEdit = async () => {
  if (!isEditable.value || !isVisible.value) {
    return
  }
  draft.value = displayText.value
  editing.value = true
  await nextTick()
  inputRef.value?.focus?.()
}

const resetTouchTapState = () => {
  lastTouchTapAt.value = 0
  lastTouchTapX.value = 0
  lastTouchTapY.value = 0
}

const handleTouchEnd = async (event: TouchEvent) => {
  if (editing.value || !isEditable.value || !isVisible.value) {
    resetTouchTapState()
    return
  }
  const touch = event.changedTouches?.[0]
  if (!touch) {
    resetTouchTapState()
    return
  }
  const now = Date.now()
  const delta = now - lastTouchTapAt.value
  const moveX = Math.abs(touch.clientX - lastTouchTapX.value)
  const moveY = Math.abs(touch.clientY - lastTouchTapY.value)
  if (
    lastTouchTapAt.value > 0
    && delta <= DOUBLE_TAP_MAX_DELAY_MS
    && moveX <= DOUBLE_TAP_MAX_MOVE_PX
    && moveY <= DOUBLE_TAP_MAX_MOVE_PX
  ) {
    resetTouchTapState()
    event.preventDefault()
    await beginEdit()
    return
  }
  lastTouchTapAt.value = now
  lastTouchTapX.value = touch.clientX
  lastTouchTapY.value = touch.clientY
}

const handleTouchCancel = () => {
  resetTouchTapState()
}

const cancelEdit = () => {
  editing.value = false
  draft.value = displayText.value
}

const commitEdit = async () => {
  if (!isEditable.value || !props.identityId || !resolvedChannelId.value) {
    return
  }
  const result = await remarkStore.saveRemark(resolvedChannelId.value, props.identityId, draft.value)
  if (!result.ok) {
    message.error(result.error)
    return
  }
  editing.value = false
}

watch(
  () => displayText.value,
  (value) => {
    if (!editing.value) {
      draft.value = value
    }
  },
  { immediate: true },
)
</script>

<template>
  <span
    v-if="isVisible"
    class="character-remark"
    :class="{ 'character-remark--editable': isEditable }"
    :style="remarkStyle"
    :title="isEditable ? '双击或双触编辑角色备注' : displayText"
    @dblclick.stop="beginEdit"
    @touchend.stop="handleTouchEnd"
    @touchcancel="handleTouchCancel"
  >
    <n-input
      v-if="editing"
      ref="inputRef"
      v-model:value="draft"
      size="tiny"
      class="character-remark__input"
      :maxlength="remarkStore.maxLength"
      placeholder="输入角色备注"
      @click.stop
      @dblclick.stop
      @keyup.enter.stop="commitEdit"
      @keyup.escape.stop="cancelEdit"
      @blur="commitEdit"
    />
    <span v-else class="character-remark__text">{{ displayText }}</span>
  </span>
</template>

<style scoped>
.character-remark {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  max-width: min(18em, 36vw);
  font-size: 0.68em;
  line-height: 1.2;
  padding: 0.08em 0.36em;
  border-radius: 6px;
  border: 1px solid rgba(128, 128, 128, 0.16);
  margin-left: 0.5em;
  vertical-align: middle;
  background: rgba(148, 163, 184, 0.08);
  color: rgba(51, 65, 85, 0.88);
}

.character-remark--editable {
  cursor: text;
  touch-action: manipulation;
  -webkit-tap-highlight-color: transparent;
}

.character-remark__text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.character-remark__input {
  width: min(20em, 42vw);
}
</style>
