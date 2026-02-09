<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { CustomThemeColors } from '@/stores/display'

interface ColorField {
  key: keyof CustomThemeColors
  label: string
  group: string
}

interface Props {
  show: boolean
  colorFields: ColorField[]
  themeColors: CustomThemeColors
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'update:theme-color', payload: { key: keyof CustomThemeColors; value: string | null }): void
  (e: 'save-theme'): void
}>()

const zIndex = ref(2147483000)
const pickerModeZIndex = 1900
const floatingX = ref(16)
const floatingY = ref(64)
const initialized = ref(false)
const openedPickerKeys = ref<Set<keyof CustomThemeColors>>(new Set())

const dragging = ref(false)
const dragOffsetX = ref(0)
const dragOffsetY = ref(0)

const close = () => emit('update:show', false)

const colorGroups = computed(() => {
  const groups: Record<string, ColorField[]> = {}
  props.colorFields.forEach((field) => {
    if (!groups[field.group]) groups[field.group] = []
    groups[field.group].push(field)
  })
  return groups
})

const floatingStyle = computed(() => ({
  left: `${floatingX.value}px`,
  top: `${floatingY.value}px`,
  zIndex: `${openedPickerKeys.value.size > 0 ? pickerModeZIndex : zIndex.value}`,
}))

const viewportWidth = () => (typeof window === 'undefined' ? 1280 : window.innerWidth)
const viewportHeight = () => (typeof window === 'undefined' ? 720 : window.innerHeight)

const clampPosition = () => {
  const maxX = Math.max(16, viewportWidth() - 420)
  const maxY = Math.max(16, viewportHeight() - 120)
  floatingX.value = Math.min(Math.max(16, floatingX.value), maxX)
  floatingY.value = Math.min(Math.max(16, floatingY.value), maxY)
}

const ensureInitialPosition = () => {
  if (initialized.value) {
    clampPosition()
    return
  }
  floatingX.value = Math.max(16, viewportWidth() - 420)
  floatingY.value = Math.max(16, Math.min(96, viewportHeight() - 140))
  initialized.value = true
}

const stopDragging = () => {
  if (!dragging.value) return
  dragging.value = false
  window.removeEventListener('mousemove', onDragMove)
  window.removeEventListener('mouseup', stopDragging)
}

function onDragMove(event: MouseEvent) {
  if (!dragging.value) return
  floatingX.value = event.clientX - dragOffsetX.value
  floatingY.value = event.clientY - dragOffsetY.value
  clampPosition()
}

const handleHeaderMouseDown = (event: MouseEvent) => {
  const target = event.target as HTMLElement | null
  if (target?.closest('.theme-live-preview-floating__actions')) return
  dragging.value = true
  dragOffsetX.value = event.clientX - floatingX.value
  dragOffsetY.value = event.clientY - floatingY.value
  window.addEventListener('mousemove', onDragMove)
  window.addEventListener('mouseup', stopDragging)
}

const bringToFront = () => {
  zIndex.value = 2147483000
}

const handleUpdateColor = (key: keyof CustomThemeColors, value: string | null) => {
  emit('update:theme-color', { key, value })
}

const handleSaveTheme = () => {
  emit('save-theme')
}

const handlePickerShowUpdate = (key: keyof CustomThemeColors, visible: boolean) => {
  const next = new Set(openedPickerKeys.value)
  if (visible) {
    next.add(key)
  } else {
    next.delete(key)
  }
  openedPickerKeys.value = next
}

watch(
  () => props.show,
  (visible) => {
    if (!visible) {
      openedPickerKeys.value = new Set()
      return
    }
    ensureInitialPosition()
    bringToFront()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  stopDragging()
})
</script>

<template>
  <teleport to="body">
    <div
      v-if="props.show"
      class="theme-live-preview-floating"
      :style="floatingStyle"
      role="dialog"
      aria-label="主题实时预览浮窗"
      @mousedown="bringToFront"
    >
      <div class="theme-live-preview-floating__header" @mousedown.prevent="handleHeaderMouseDown">
        <div class="theme-live-preview-floating__title-group">
          <p class="theme-live-preview-floating__title">实时预览已开启（可拖动）</p>
          <p class="theme-live-preview-floating__subtitle">可关闭设置面板，继续在此编辑全部主题颜色</p>
        </div>
        <div class="theme-live-preview-floating__actions">
          <button type="button" class="theme-live-preview-floating__save" @click="handleSaveTheme">保存</button>
          <button type="button" class="theme-live-preview-floating__close" @click="close">关闭</button>
        </div>
      </div>

      <div class="theme-live-preview-floating__body">
        <div class="theme-live-preview-floating__tips">
          <span>提示：仅保存后会写入主题模板。</span>
        </div>

        <div class="theme-live-preview-floating__groups">
          <div v-for="(fields, groupName) in colorGroups" :key="groupName" class="theme-live-preview-floating__group">
            <p class="theme-live-preview-floating__group-title">{{ groupName }}</p>
            <div v-for="field in fields" :key="field.key" class="theme-live-preview-floating__item">
              <span class="theme-live-preview-floating__item-label">{{ field.label }}</span>
              <div class="theme-live-preview-floating__item-picker">
                <n-color-picker
                  :value="props.themeColors[field.key] || undefined"
                  :show-alpha="true"
                  size="small"
                  :modes="['hex', 'rgb', 'hsl']"
                  :show-preview="true"
                  :actions="['confirm']"
                  to="body"
                  @update:show="(visible: boolean) => handlePickerShowUpdate(field.key, visible)"
                  @update:value="(v: string | null) => handleUpdateColor(field.key, v)"
                >
                  <template #label>
                    <div
                      class="theme-live-preview-floating__swatch"
                      :class="{ 'theme-live-preview-floating__swatch--empty': !props.themeColors[field.key] }"
                      :style="{ backgroundColor: props.themeColors[field.key] || 'transparent' }"
                    />
                  </template>
                </n-color-picker>
                <n-button
                  v-if="props.themeColors[field.key]"
                  text
                  size="tiny"
                  @click="handleUpdateColor(field.key, null)"
                >
                  清除
                </n-button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </teleport>
</template>

<style scoped lang="scss">
.theme-live-preview-floating {
  position: fixed;
  width: min(380px, calc(100vw - 1.5rem));
  max-height: min(78vh, 760px);
  border-radius: 12px;
  border: 1px solid var(--sc-border-strong);
  background: color-mix(in srgb, var(--sc-bg-elevated) 94%, transparent);
  color: var(--sc-text-primary);
  box-shadow: 0 18px 40px color-mix(in srgb, #000 28%, transparent);
  backdrop-filter: blur(10px);
  overflow: hidden;
  user-select: none;
}

.theme-live-preview-floating__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.8rem 0.9rem;
  border-bottom: 1px solid var(--sc-border-mute);
  background: color-mix(in srgb, var(--sc-bg-surface) 90%, transparent);
  cursor: move;
}

.theme-live-preview-floating__title-group {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.theme-live-preview-floating__title {
  margin: 0;
  font-size: 0.88rem;
  font-weight: 600;
}

.theme-live-preview-floating__subtitle {
  margin: 0;
  font-size: 0.72rem;
  color: var(--sc-text-secondary);
}

.theme-live-preview-floating__close {
  border: 1px solid var(--sc-border-mute);
  background: var(--sc-bg-surface);
  color: var(--sc-text-primary);
  border-radius: 8px;
  font-size: 0.75rem;
  line-height: 1;
  padding: 0.35rem 0.6rem;
  cursor: pointer;
}

.theme-live-preview-floating__actions {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.theme-live-preview-floating__save {
  border: 1px solid color-mix(in srgb, var(--primary-color, #3388de) 68%, transparent);
  background: color-mix(in srgb, var(--primary-color, #3388de) 16%, var(--sc-bg-surface));
  color: var(--primary-color, #3388de);
  border-radius: 8px;
  font-size: 0.75rem;
  line-height: 1;
  padding: 0.35rem 0.6rem;
  cursor: pointer;
}

.theme-live-preview-floating__save:hover {
  border-color: var(--primary-color, #3388de);
  background: color-mix(in srgb, var(--primary-color, #3388de) 24%, var(--sc-bg-surface));
}

.theme-live-preview-floating__close:hover {
  border-color: var(--primary-color, #3388de);
  color: var(--primary-color, #3388de);
}

.theme-live-preview-floating__body {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
  padding: 0.75rem 0.85rem 0.9rem;
  overflow: auto;
}

.theme-live-preview-floating__tips {
  font-size: 0.74rem;
  color: var(--sc-text-secondary);
  border: 1px solid var(--sc-border-mute);
  border-radius: 8px;
  padding: 0.45rem 0.55rem;
}

.theme-live-preview-floating__groups {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.theme-live-preview-floating__group {
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  padding: 0.55rem 0.6rem;
  background: color-mix(in srgb, var(--sc-bg-surface) 90%, transparent);
}

.theme-live-preview-floating__group-title {
  margin: 0 0 0.35rem;
  font-size: 0.76rem;
  font-weight: 600;
  color: var(--sc-text-secondary);
}

.theme-live-preview-floating__item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.65rem;
  padding: 0.28rem 0;
}

.theme-live-preview-floating__item-label {
  font-size: 0.8rem;
  color: var(--sc-text-primary);
}

.theme-live-preview-floating__item-picker {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.theme-live-preview-floating__swatch {
  width: 36px;
  height: 22px;
  border-radius: 4px;
  border: 1px solid var(--sc-border-mute, rgba(0, 0, 0, 0.15));
}

.theme-live-preview-floating__swatch--empty {
  border-style: dashed;
  background: repeating-linear-gradient(
    45deg,
    transparent,
    transparent 3px,
    rgba(128, 128, 128, 0.12) 3px,
    rgba(128, 128, 128, 0.12) 6px
  ) !important;
}

@media (max-width: 600px) {
  .theme-live-preview-floating {
    width: calc(100vw - 1rem);
    max-height: 70vh;
  }
}
</style>
