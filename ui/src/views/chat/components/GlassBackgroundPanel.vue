<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useDraggable, useResizeObserver, useWindowSize } from '@vueuse/core';
import { NButton, NColorPicker, NSelect, NSlider, NSwitch, useMessage } from 'naive-ui';
import { useGlassBackground, type GlassBackgroundSettings } from '@/composables/useGlassBackground';
import { buildBackgroundImageStyle, buildBackgroundOverlayStyle } from '@/utils/backgroundPresentation';
import { compressImage } from '@/composables/useImageCompressor';
import { normalizeAttachmentId } from '@/composables/useAttachmentResolver';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useChatStore } from '@/stores/chat';

const emit = defineEmits<{ (event: 'close'): void }>();
const { settings, presentation, update, clear, reset } = useGlassBackground();
const chat = useChatStore();
const message = useMessage();
const panelRef = ref<HTMLElement>();
const handleRef = ref<HTMLElement>();
const fileRef = ref<HTMLInputElement>();
const minimized = ref(false);
const uploading = ref(false);
const { width, height } = useWindowSize();
const key = 'sealchat.glass-background.panel.v1';
let alive = true;
let uploadGeneration = 0;
const { x, y } = useDraggable(panelRef, {
  handle: handleRef, initialValue: { x: 24, y: 80 }, preventDefault: true,
  onStart: (_, event) => {
    if (event.button !== 0 || (event.target as HTMLElement).closest('button')) return false;
  },
  onMove: () => clamp(),
  onEnd: () => { clamp(); persist(); },
});

function clamp() {
  const rect = panelRef.value?.getBoundingClientRect();
  x.value = Math.max(12, Math.min(x.value, width.value - (rect?.width ?? 380) - 12));
  y.value = Math.max(12, Math.min(y.value, height.value - (rect?.height ?? 48) - 12));
}
function persist() {
  try { localStorage.setItem(key, JSON.stringify({ version: 1, x: x.value, y: y.value, minimized: minimized.value })); }
  catch { /* Keep the panel usable without storage. */ }
}
onMounted(() => {
  try {
    const saved = JSON.parse(localStorage.getItem(key) || 'null');
    if (saved?.version === 1) {
      if (typeof saved.x === 'number' && Number.isFinite(saved.x)) x.value = saved.x;
      if (typeof saved.y === 'number' && Number.isFinite(saved.y)) y.value = saved.y;
      minimized.value = saved.minimized === true;
    }
  } catch { /* Use the default position. */ }
  nextTick(clamp);
});
watch([width, height, minimized], () => nextTick(() => { clamp(); persist(); }));
useResizeObserver(panelRef, clamp);
onBeforeUnmount(() => { alive = false; uploadGeneration++; persist(); });

const imageStyle = computed(() => ({
  ...buildBackgroundImageStyle(settings.attachmentId, presentation.value),
  inset: `${-settings.backgroundBlur * 3}px`,
}));
const overlayStyle = computed(() => buildBackgroundOverlayStyle(presentation.value));
const modes = [
  { label: '覆盖', value: 'cover' }, { label: '完整显示', value: 'contain' },
  { label: '居中', value: 'center' }, { label: '平铺', value: 'tile' },
];
type SliderKey = 'backgroundOpacity' | 'backgroundBrightness' | 'backgroundBlur' | 'surfaceOpacity' | 'glassBlur' | 'saturation' | 'overlayOpacity';
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
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  if (!file.type.startsWith('image/')) { message.error('请选择图片文件'); return; }
  const generation = ++uploadGeneration;
  const channelId = chat.curChannel?.id;
  uploading.value = true;
  try {
    const compressed = await compressImage(file, { maxWidth: 1920, maxHeight: 1080 });
    if (!alive || generation !== uploadGeneration) return;
    const result = await uploadImageAttachment(compressed, { channelId, skipCompression: true, confirm: true });
    if (alive && generation === uploadGeneration) update({ attachmentId: normalizeAttachmentId(result.attachmentId) });
  } catch (error) {
    if (alive && generation === uploadGeneration) message.error(error instanceof Error ? error.message : '上传失败');
  } finally {
    if (alive && generation === uploadGeneration) uploading.value = false;
  }
}
function clearImage() {
  uploadGeneration++;
  uploading.value = false;
  clear();
}
</script>

<template>
  <Teleport to="body">
    <section ref="panelRef" class="sc-glass-settings-panel" role="dialog" aria-label="玻璃背景" :style="{ left: `max(${x}px, env(safe-area-inset-left))`, top: `max(${y}px, env(safe-area-inset-top))` }">
      <header ref="handleRef" class="sc-glass-settings-panel__header">
        <strong>玻璃背景</strong>
        <NButton quaternary size="small" :aria-label="minimized ? '展开' : '最小化'" @click="minimized = !minimized">{{ minimized ? '＋' : '—' }}</NButton>
        <NButton quaternary size="small" aria-label="关闭" @click="emit('close')">×</NButton>
      </header>
      <div v-show="!minimized" class="sc-glass-settings-panel__body">
        <div class="sc-glass-settings-panel__row"><span>启用玻璃背景</span><NSwitch :value="settings.enabled" @update:value="update({ enabled: $event })" /></div>
        <div class="sc-glass-settings-panel__preview">
          <div v-if="settings.attachmentId" class="sc-glass-settings-panel__image" :style="imageStyle" />
          <div v-if="settings.attachmentId && overlayStyle" class="sc-glass-settings-panel__image" :style="overlayStyle" />
          <span v-if="!settings.attachmentId">选择一张环境背景图片</span>
        </div>
        <div class="sc-glass-settings-panel__actions">
          <NButton size="small" :loading="uploading" @click="fileRef?.click()">选择图片</NButton>
          <NButton size="small" :disabled="!settings.attachmentId && !uploading" @click="clearImage">清除</NButton>
          <input ref="fileRef" type="file" accept="image/*" hidden @change="selectImage" />
        </div>
        <label>背景布局<NSelect :value="settings.mode" :options="modes" @update:value="update({ mode: $event as GlassBackgroundSettings['mode'] })" /></label>
        <label v-for="slider in sliders" :key="slider.key">
          <span class="sc-glass-settings-panel__row"><span>{{ slider.label }}</span><span>{{ settings[slider.key] }}{{ slider.unit }}</span></span>
          <NSlider :value="settings[slider.key]" :min="slider.min" :max="slider.max" :aria-label="slider.label" @update:value="update({ [slider.key]: $event })" />
        </label>
        <label>遮罩颜色<NColorPicker :value="settings.overlayColor || '#000000'" :show-alpha="false" @update:value="update({ overlayColor: $event })" /></label>
        <label><span class="sc-glass-settings-panel__row"><span>遮罩强度</span><span>{{ settings.overlayOpacity }}%</span></span><NSlider :value="settings.overlayOpacity" :min="0" :max="100" aria-label="遮罩强度" @update:value="update({ overlayOpacity: $event, overlayColor: settings.overlayColor || '#000000' })" /></label>
        <NButton size="small" @click="reset">恢复默认参数</NButton>
      </div>
    </section>
  </Teleport>
</template>

<style scoped>
.sc-glass-settings-panel { position: fixed; z-index: 3200; width: 380px; max-width: calc(100vw - 24px - env(safe-area-inset-left) - env(safe-area-inset-right)); max-height: calc(100dvh - 24px - env(safe-area-inset-top) - env(safe-area-inset-bottom)); display: flex; flex-direction: column; overflow: hidden; border: 1px solid var(--sc-border-strong); border-radius: 12px; background: var(--sc-bg-elevated); color: var(--sc-text-primary); box-shadow: 0 12px 36px #0003; }
.sc-glass-settings-panel__header { display: flex; align-items: center; gap: 4px; padding: 10px 12px; flex-shrink: 0; cursor: grab; touch-action: none; user-select: none; border-bottom: 1px solid var(--sc-border-mute); }
.sc-glass-settings-panel__header strong { flex: 1; }
.sc-glass-settings-panel__body { padding: 16px; overflow-y: auto; min-height: 0; display: flex; flex-direction: column; gap: 14px; }
.sc-glass-settings-panel__row { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.sc-glass-settings-panel__actions { display: flex; gap: 8px; }
.sc-glass-settings-panel__preview { position: relative; min-height: 120px; display: grid; place-items: center; overflow: hidden; border-radius: 8px; background: var(--sc-bg-page); color: var(--sc-text-secondary); }
.sc-glass-settings-panel__image { position: absolute; inset: 0; }
.sc-glass-settings-panel__body label { display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
</style>
