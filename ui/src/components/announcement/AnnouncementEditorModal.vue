<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useWindowSize } from '@vueuse/core'
import { useMessage } from 'naive-ui'
import KeywordRichEditor from '@/views/world/KeywordRichEditor.vue'
import type { AnnouncementItem, AnnouncementPayload, AnnouncementScopeType } from '@/models/announcement'
import { useAnnouncementStore } from '@/stores/announcement'

const props = defineProps<{
  visible: boolean
  scopeType: AnnouncementScopeType
  scopeId?: string
  item?: AnnouncementItem | null
}>()

const emit = defineEmits<{
  (event: 'update:visible', value: boolean): void
  (event: 'saved'): void
}>()

const message = useMessage()
const announcementStore = useAnnouncementStore()
const { width: viewportWidth } = useWindowSize()

const saving = ref(false)
const showOptions = ref(false)
const form = ref<AnnouncementPayload>({
  title: '',
  content: '',
  contentFormat: 'rich',
  status: 'published',
  isPinned: false,
  pinOrder: 0,
  popupMode: 'none',
  requireAck: false,
})

const isEdit = computed(() => !!props.item?.id)
const titleText = computed(() => isEdit.value ? '编辑公告' : '新建公告')
const canRequireAck = computed(() => props.scopeType === 'world')
const handleVisibleUpdate = (value: boolean) => emit('update:visible', value)
const modalStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'width: calc(100vw - 12px); max-width: calc(100vw - 12px);'
    : 'width: min(60vw, 1240px); max-width: calc(100vw - 32px);'
))
const headerStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 12px 14px 8px;'
    : 'padding: 14px 18px 10px;'
))
const contentStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 10px 12px 12px;'
    : 'padding: 10px 18px 14px;'
))
const footerStyle = computed(() => (
  viewportWidth.value <= 640
    ? 'padding: 0 12px 12px;'
    : 'padding: 0 18px 14px;'
))
const statusLabel = computed(() => form.value.status === 'draft' ? '草稿' : '发布')
const popupLabel = computed(() => {
  switch (form.value.popupMode) {
    case 'once_per_version':
      return '每次编辑'
    case 'every_entry':
      return '每次进入'
    default:
      return '不弹出'
  }
})
const optionSummary = computed(() => {
  const parts = [
    `状态 ${statusLabel.value}`,
    `弹窗 ${popupLabel.value}`,
    form.value.isPinned ? `置顶 #${form.value.pinOrder}` : '未置顶',
  ]
  if (canRequireAck.value) {
    parts.push(form.value.requireAck ? '需成员确认' : '无需确认')
  }
  return parts.join(' · ')
})

watch(
  () => [props.visible, props.item?.id],
  () => {
    if (!props.visible) return
    showOptions.value = false
    const item = props.item
    if (item) {
      form.value = {
        title: item.title || '',
        content: item.content || '',
        contentFormat: item.contentFormat || 'rich',
        status: item.status === 'archived' ? 'draft' : item.status,
        isPinned: item.isPinned,
        pinOrder: item.pinOrder || 0,
        popupMode: item.popupMode || 'none',
        requireAck: item.requireAck,
      }
      return
    }
    form.value = {
      title: '',
      content: '',
      contentFormat: 'rich',
      status: 'published',
      isPinned: false,
      pinOrder: 0,
      popupMode: 'none',
      requireAck: false,
    }
  },
  { immediate: true },
)

const close = () => emit('update:visible', false)

const handleSave = async () => {
  if (!form.value.title.trim()) {
    message.warning('请填写公告标题')
    return
  }
  if (!form.value.content.trim()) {
    message.warning('请填写公告内容')
    return
  }
  saving.value = true
  try {
    const payload: AnnouncementPayload = {
      ...form.value,
      requireAck: canRequireAck.value ? form.value.requireAck : false,
    }
    if (props.scopeType === 'world') {
      const worldId = String(props.scopeId || '').trim()
      if (!worldId) throw new Error('world id required')
      if (props.item?.id) {
        await announcementStore.updateWorld(worldId, props.item.id, payload)
      } else {
        await announcementStore.createWorld(worldId, payload)
      }
    } else if (props.item?.id) {
      await announcementStore.updateLobby(props.item.id, payload)
    } else {
      await announcementStore.createLobby(payload)
    }
    message.success('公告已保存')
    emit('saved')
    close()
  } catch (err: any) {
    message.error(err?.response?.data?.message || err?.message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    closable
    :title="titleText"
    :mask-closable="false"
    :style="modalStyle"
    :header-style="headerStyle"
    :content-style="contentStyle"
    :footer-style="footerStyle"
    @update:show="handleVisibleUpdate"
  >
    <div class="announcement-editor">
      <div class="announcement-editor__header">
        <n-input
          v-model:value="form.title"
          class="announcement-editor__title-input"
          maxlength="120"
          show-count
          placeholder="输入公告标题"
        />
      </div>
      <div class="announcement-editor__summary-bar">
        <div class="announcement-editor__summary-text">{{ optionSummary }}</div>
        <n-button size="small" quaternary @click="showOptions = !showOptions">
          {{ showOptions ? '收起选项' : '展开选项' }}
        </n-button>
      </div>
      <n-collapse-transition :show="showOptions">
        <div class="announcement-editor__toolbar">
          <div class="announcement-editor__tool">
            <span class="announcement-editor__tool-label">状态</span>
            <n-radio-group v-model:value="form.status" size="small">
              <n-radio-button value="published">发布</n-radio-button>
              <n-radio-button value="draft">草稿</n-radio-button>
            </n-radio-group>
          </div>
          <div class="announcement-editor__tool announcement-editor__tool--wide">
            <span class="announcement-editor__tool-label">弹窗</span>
            <n-radio-group v-model:value="form.popupMode" size="small">
              <n-radio-button value="none">不弹出</n-radio-button>
              <n-radio-button value="once_per_version">每次编辑</n-radio-button>
              <n-radio-button value="every_entry">每次进入</n-radio-button>
            </n-radio-group>
          </div>
          <div class="announcement-editor__tool announcement-editor__tool--switch">
            <span class="announcement-editor__tool-label">置顶</span>
            <n-switch v-model:value="form.isPinned" size="small" />
          </div>
          <div class="announcement-editor__tool announcement-editor__tool--number">
            <span class="announcement-editor__tool-label">置顶顺序</span>
            <n-input-number v-model:value="form.pinOrder" size="small" :min="0" />
          </div>
          <div v-if="canRequireAck" class="announcement-editor__tool announcement-editor__tool--switch">
            <span class="announcement-editor__tool-label">成员确认</span>
            <n-switch v-model:value="form.requireAck" size="small" />
          </div>
        </div>
      </n-collapse-transition>
      <div class="announcement-editor__body">
        <KeywordRichEditor v-model:model-value="form.content" :maxlength="12000" placeholder="填写公告正文" />
      </div>
    </div>
    <template #action>
      <n-space justify="end">
        <n-button quaternary @click="close">取消</n-button>
        <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.announcement-editor {
  max-height: 78vh;
  display: flex;
  flex-direction: column;
  gap: 10px;
  overflow: hidden;
}

.announcement-editor__header {
  flex: 0 0 auto;
}

.announcement-editor__title-input {
  width: 100%;
}

.announcement-editor__summary-bar {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border: 1px solid var(--sc-border-color, rgba(0, 0, 0, 0.08));
  border-radius: 12px;
  background: var(--sc-bg-secondary, rgba(0, 0, 0, 0.02));
}

.announcement-editor__summary-text {
  min-width: 0;
  font-size: 12px;
  color: var(--sc-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.announcement-editor__toolbar {
  flex: 0 0 auto;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--sc-border-color, rgba(0, 0, 0, 0.08));
  border-radius: 12px;
  background: var(--sc-bg-secondary, rgba(0, 0, 0, 0.02));
}

.announcement-editor__tool {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.announcement-editor__tool--wide {
  flex: 1 1 360px;
}

.announcement-editor__tool--switch,
.announcement-editor__tool--number {
  flex: 0 0 auto;
}

.announcement-editor__tool-label {
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--sc-text-secondary);
  white-space: nowrap;
}

.announcement-editor__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}

@media (max-width: 640px) {
  .announcement-editor {
    max-height: 84vh;
    gap: 8px;
  }

  .announcement-editor__summary-bar {
    align-items: flex-start;
    flex-direction: column;
    padding: 10px;
  }

  .announcement-editor__summary-text {
    white-space: normal;
  }

  .announcement-editor__toolbar {
    gap: 10px;
    padding: 10px;
  }

  .announcement-editor__tool,
  .announcement-editor__tool--wide,
  .announcement-editor__tool--number,
  .announcement-editor__tool--switch {
    width: 100%;
    justify-content: space-between;
  }

  .announcement-editor__tool :deep(.n-radio-group),
  .announcement-editor__tool :deep(.n-input-number) {
    max-width: 100%;
  }
}
</style>
