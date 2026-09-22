<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { NButton, NColorPicker, NSelect, NSlider, useMessage } from 'naive-ui';
import type { GlassBackgroundSettings } from '@/composables/useGlassBackground';
import { buildBackgroundImageStyle, buildBackgroundOverlayStyle } from '@/utils/backgroundPresentation';
import { compressImage } from '@/composables/useImageCompressor';
import { normalizeAttachmentId } from '@/composables/useAttachmentResolver';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useChatStore } from '@/stores/chat';

const props = defineProps<{
  settings: Readonly<GlassBackgroundSettings>;
  disabled?: boolean;
  uploadChannelId?: string | null;
}>();
const emit = defineEmits<{
  (event: 'update', patch: Partial<GlassBackgroundSettings>): void;
  (event: 'uploading', value: boolean): void;
}>();
const chat = useChatStore();
const message = useMessage();
const GLASS_POPUP_Z_INDEX = 3300;
const glassSelectMenuProps = {
  class: 'sc-glass-popup-menu',
  style: { zIndex: GLASS_POPUP_Z_INDEX },
};
const fileRef = ref<HTMLInputElement>();
const colorPickerHost = ref<HTMLElement | null>(null);
const uploading = ref(false);
let generation = 0;
onMounted(() => {
  // Naive UI 2.40.1 has no ColorPicker follower z-index prop; keep this editor's portal local.
  const host = document.createElement('div');
  host.className = 'sc-glass-color-picker-host';
  Object.assign(host.style, {
    position: 'fixed',
    inset: '0',
    zIndex: String(GLASS_POPUP_Z_INDEX),
    pointerEvents: 'none',
  });
  document.body.appendChild(host);
  colorPickerHost.value = host;
});
onBeforeUnmount(() => {
  generation++;
  emit('uploading', false);
  colorPickerHost.value?.remove();
  colorPickerHost.value = null;
});
const presentation = computed(() => ({ mode: props.settings.mode, opacity: props.settings.backgroundOpacity,
  blur: props.settings.backgroundBlur, brightness: props.settings.backgroundBrightness,
  overlayColor: props.settings.overlayColor, overlayOpacity: props.settings.overlayOpacity }));
const imageStyle = computed(() => ({
  ...buildBackgroundImageStyle(props.settings.attachmentId, { ...presentation.value, blur: 0 }),
  inset: 0,
}));
const blurStyle = computed(() => ({
  backdropFilter: `blur(${props.settings.backgroundBlur}px)`,
  WebkitBackdropFilter: `blur(${props.settings.backgroundBlur}px)`,
}));
const overlayStyle = computed(() => buildBackgroundOverlayStyle(presentation.value));
const glassSurfaceStyle = computed(() => {
  const opacity = props.settings.surfaceOpacity;
  const blur = props.settings.glassBlur;
  const saturation = props.settings.saturation;
  return {
    background: `color-mix(in srgb, var(--sc-bg-surface) ${opacity}%, transparent)`,
    backdropFilter: `blur(${blur}px) saturate(${saturation}%)`,
    WebkitBackdropFilter: `blur(${blur}px) saturate(${saturation}%)`,
  };
});
const modes = [{ label: '覆盖', value: 'cover' }, { label: '完整显示', value: 'contain' }, { label: '居中', value: 'center' }, { label: '平铺', value: 'tile' }];
type SliderKey = 'backgroundOpacity' | 'backgroundBrightness' | 'backgroundBlur' | 'surfaceOpacity' | 'glassBlur' | 'saturation';
const sliders: { key: SliderKey; label: string; min: number; max: number; unit: string }[] = [
  { key: 'backgroundOpacity', label: '背景不透明度', min: 0, max: 100, unit: '%' },
  { key: 'backgroundBrightness', label: '背景亮度', min: 20, max: 150, unit: '%' },
  { key: 'backgroundBlur', label: '背景模糊', min: 0, max: 40, unit: 'px' },
  { key: 'surfaceOpacity', label: '玻璃不透明度', min: 35, max: 95, unit: '%' },
  { key: 'glassBlur', label: '玻璃模糊', min: 0, max: 32, unit: 'px' },
  { key: 'saturation', label: '色彩饱和度', min: 50, max: 180, unit: '%' },
];
async function selectImage(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0]; input.value = '';
  if (!file || props.disabled) return;
  if (!file.type.startsWith('image/')) { message.error('请选择图片文件'); return; }
  const token = ++generation;
  const channelId = props.uploadChannelId !== undefined
    ? props.uploadChannelId
    : chat.curChannel?.id;
  uploading.value = true;
  emit('uploading', true);
  try {
    const compressed = await compressImage(file, { maxWidth: 1920, maxHeight: 1080 });
    if (token !== generation) return;
    const result = await uploadImageAttachment(compressed, { channelId, skipCompression: true, confirm: true });
    if (token === generation) emit('update', { attachmentId: normalizeAttachmentId(result.attachmentId) });
  } catch (error) {
    if (token === generation) message.error(error instanceof Error ? error.message : '上传失败');
  } finally { if (token === generation) { uploading.value = false; emit('uploading', false); } }
}
function clearImage() { generation++; uploading.value = false; emit('uploading', false); emit('update', { attachmentId: '' }); }
</script>

<template>
  <div class="glass-editor">
    <div class="glass-editor__preview">
      <div v-if="settings.attachmentId" class="glass-editor__image" :style="imageStyle" />
      <div v-if="settings.attachmentId && settings.backgroundBlur > 0" class="glass-editor__blur" :style="blurStyle" />
      <div v-if="settings.attachmentId && overlayStyle" class="glass-editor__overlay" :style="overlayStyle" />
      <div class="glass-editor__surface" :style="glassSurfaceStyle">
        <div class="glass-editor__surface-title">玻璃材质预览</div>
        <div class="glass-editor__surface-line" />
        <div class="glass-editor__surface-line glass-editor__surface-line--short" />
      </div>
      <span v-if="!settings.attachmentId" class="glass-editor__empty">选择一张环境背景图片</span>
    </div>
    <div class="glass-editor__actions">
      <NButton size="small" :disabled="disabled" :loading="uploading" @click="fileRef?.click()">选择图片</NButton>
      <NButton size="small" :disabled="disabled || (!settings.attachmentId && !uploading)" @click="clearImage">清除</NButton>
      <input ref="fileRef" type="file" accept="image/*" hidden @change="selectImage" />
    </div>
    <label>背景布局<NSelect :disabled="disabled" :value="settings.mode" :options="modes" :menu-props="glassSelectMenuProps" @update:value="emit('update', { mode: $event as GlassBackgroundSettings['mode'] })" /></label>
    <label v-for="slider in sliders" :key="slider.key">
      <span class="glass-editor__row"><span>{{ slider.label }}</span><span>{{ settings[slider.key] }}{{ slider.unit }}</span></span>
      <NSlider :disabled="disabled" :value="settings[slider.key]" :min="slider.min" :max="slider.max" :aria-label="slider.label" @update:value="emit('update', { [slider.key]: $event })" />
    </label>
    <label>遮罩颜色<NColorPicker :disabled="disabled" :value="settings.overlayColor || '#000000'" :modes="['hex']" :show-alpha="false" :to="colorPickerHost || undefined" @update:value="emit('update', { overlayColor: $event })" /></label>
    <label><span class="glass-editor__row"><span>遮罩强度</span><span>{{ settings.overlayOpacity }}%</span></span><NSlider :disabled="disabled" :value="settings.overlayOpacity" :min="0" :max="100" aria-label="遮罩强度" @update:value="emit('update', { overlayOpacity: $event, overlayColor: settings.overlayColor || '#000000' })" /></label>
  </div>
</template>

<style scoped>
.glass-editor { display: flex; flex-direction: column; gap: 14px; }
.glass-editor__preview { position: relative; aspect-ratio: 16 / 9; display: grid; place-items: center; overflow: hidden; border-radius: 8px; background: var(--sc-bg-page); color: var(--sc-text-secondary); }
.glass-editor__image { position: absolute; inset: 0; z-index: 0; }
.glass-editor__blur { position: absolute; inset: 0; z-index: 1; pointer-events: none; }
.glass-editor__overlay { position: absolute; inset: 0; z-index: 2; }
.glass-editor__surface { position: absolute; left: 16%; right: 16%; bottom: 14%; min-height: 42%; padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--sc-border-strong) 70%, transparent); border-radius: 8px; box-shadow: 0 8px 24px rgba(0, 0, 0, .14); display: flex; flex-direction: column; gap: 7px; justify-content: center; z-index: 3; box-sizing: border-box; color: var(--sc-text-primary); }
.glass-editor__surface-title { font-size: 12px; font-weight: 600; }
.glass-editor__surface-line { width: 82%; height: 4px; border-radius: 999px; background: color-mix(in srgb, var(--sc-text-primary) 30%, transparent); }
.glass-editor__surface-line--short { width: 56%; }
.glass-editor__empty { position: absolute; top: 12px; left: 12px; right: 12px; z-index: 4; text-align: center; }
.glass-editor__actions { display: flex; gap: 8px; }
.glass-editor__row { display: flex; justify-content: space-between; gap: 12px; }
label { display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
:global(.v-binder-follower-container:has(.sc-glass-popup-menu)) { z-index: 3300 !important; }
</style>
