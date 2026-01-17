<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useDisplayStore, AVATAR_SIZE_LIMITS, AVATAR_BORDER_RADIUS_LIMITS } from '@/stores/display'
import Avatar from '@/components/avatar.vue'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const display = useDisplayStore()

const draftSize = ref(display.settings.avatarSize)
const draftBorderRadius = ref(display.settings.avatarBorderRadius)

watch(() => props.show, (visible) => {
  if (visible) {
    draftSize.value = display.settings.avatarSize
    draftBorderRadius.value = display.settings.avatarBorderRadius
  }
})

const previewRadius = computed(() => {
  return draftBorderRadius.value >= 50 ? '50%' : `${draftBorderRadius.value}%`
})

const radiusLabel = computed(() => {
  if (draftBorderRadius.value >= 50) return '圆形'
  if (draftBorderRadius.value === 0) return '直角'
  return `${draftBorderRadius.value}%`
})

const formatPxTooltip = (value: number) => `${Math.round(value)}px`
const formatRadiusTooltip = (value: number) => value >= 50 ? '圆形' : `${value}%`

const handleConfirm = () => {
  display.updateSettings({
    avatarSize: draftSize.value,
    avatarBorderRadius: draftBorderRadius.value,
  })
  emit('update:show', false)
}

const handleCancel = () => {
  emit('update:show', false)
}

const handleReset = () => {
  draftSize.value = AVATAR_SIZE_LIMITS.DEFAULT
  draftBorderRadius.value = AVATAR_BORDER_RADIUS_LIMITS.DEFAULT
}

// Quick preset buttons for border radius
const radiusPresets = [
  { label: '直角', value: 0 },
  { label: '小圆角', value: 10 },
  { label: '中圆角', value: 20 },
  { label: '大圆角', value: 35 },
  { label: '圆形', value: 50 },
]
</script>

<template>
  <n-modal
    class="avatar-style-panel"
    preset="card"
    :show="props.show"
    title="头像样式"
    :style="{ width: 'min(420px, 92vw)' }"
    @update:show="emit('update:show', $event)"
  >
    <div class="avatar-style-content">
      <section class="avatar-style-section">
        <header>
          <p class="section-title">头像大小</p>
          <p class="section-desc">调整聊天消息中头像的显示尺寸</p>
        </header>
        <div class="size-control">
          <n-slider
            v-model:value="draftSize"
            :min="AVATAR_SIZE_LIMITS.MIN"
            :max="AVATAR_SIZE_LIMITS.MAX"
            :step="2"
            :format-tooltip="formatPxTooltip"
          />
          <n-input-number
            v-model:value="draftSize"
            size="small"
            :min="AVATAR_SIZE_LIMITS.MIN"
            :max="AVATAR_SIZE_LIMITS.MAX"
            :step="2"
            style="width: 90px"
          />
        </div>
      </section>

      <section class="avatar-style-section">
        <header>
          <p class="section-title">圆角强度</p>
          <p class="section-desc">调整头像边角的圆滑程度，50% 为完全圆形</p>
        </header>
        <div class="radius-presets">
          <n-button
            v-for="preset in radiusPresets"
            :key="preset.value"
            size="tiny"
            :type="draftBorderRadius === preset.value ? 'primary' : 'default'"
            :secondary="draftBorderRadius !== preset.value"
            @click="draftBorderRadius = preset.value"
          >
            {{ preset.label }}
          </n-button>
        </div>
        <div class="size-control">
          <n-slider
            v-model:value="draftBorderRadius"
            :min="AVATAR_BORDER_RADIUS_LIMITS.MIN"
            :max="AVATAR_BORDER_RADIUS_LIMITS.MAX"
            :step="1"
            :format-tooltip="formatRadiusTooltip"
          />
          <span class="radius-value">{{ radiusLabel }}</span>
        </div>
      </section>

      <section class="avatar-style-section">
        <header>
          <p class="section-title">预览</p>
        </header>
        <div class="avatar-preview">
          <div
            class="avatar-preview__item"
            :style="{
              width: `${draftSize}px`,
              height: `${draftSize}px`,
              borderRadius: previewRadius,
            }"
          >
            <Avatar :size="draftSize" :border="false" />
          </div>
          <span class="avatar-preview__label">{{ draftSize }}px · {{ radiusLabel }}</span>
        </div>
      </section>

      <n-space justify="space-between" align="center" class="avatar-style-footer">
        <n-button quaternary size="small" @click="handleReset">恢复默认</n-button>
        <n-space size="small">
          <n-button quaternary size="small" @click="handleCancel">取消</n-button>
          <n-button type="primary" size="small" @click="handleConfirm">确定</n-button>
        </n-space>
      </n-space>
    </div>
  </n-modal>
</template>

<style scoped lang="scss">
.avatar-style-panel :deep(.n-card) {
  background-color: var(--sc-bg-elevated);
  border: 1px solid var(--sc-border-strong);
  color: var(--sc-text-primary);
}

.avatar-style-content {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.avatar-style-section header {
  margin-bottom: 0.5rem;
}

.section-title {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.section-desc {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  margin-top: 0.1rem;
}

.size-control {
  display: flex;
  gap: 0.75rem;
  align-items: center;
}

.size-control :deep(.n-slider) {
  flex: 1;
}

.radius-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.radius-value {
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
  min-width: 50px;
  text-align: right;
}

.avatar-preview {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  border-radius: 0.75rem;
  background: var(--sc-bg-surface);
  border: 1px dashed var(--sc-border-mute);
}

.avatar-preview__item {
  overflow: hidden;
  flex-shrink: 0;
  background: linear-gradient(135deg, #f87171, #fbbf24);
  border: 1px solid var(--sc-border-mute);
}

.avatar-preview__label {
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
}

.avatar-style-footer {
  margin-top: 0.5rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--sc-border-mute);
}
</style>
