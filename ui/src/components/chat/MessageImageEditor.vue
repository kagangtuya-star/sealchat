<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { useElementSize, useEventListener, useWindowSize } from '@vueuse/core';
import { NButton, NIcon, NSlider, NSpin, NSwitch, NTooltip } from 'naive-ui';
import { Adjustments, ArrowBackUp, ArrowForwardUp, Check, Refresh, X } from '@vicons/tabler';
import 'vue-paint/themes/default.css';
import { VpImage, type SaveParameters, useEditor } from 'vue-paint';
import {
  useMessageImageEditor,
  type MessageImageEditorResult,
  type MessageImageEditorTool,
} from '@/composables/useMessageImageEditor';
import {
  createDefaultMediaFxSpec,
  mediaFxAdvancedHasContent,
  mediaFxHasContent,
  normalizeMediaFxSpec,
  type MediaFxCapabilities,
  type MediaFxSpec,
} from '@/features/media-fx/media-fx';
import { bakeMediaFxToCanvas, MediaFxAdvancedBakeError } from '@/features/media-fx/media-fx-canvas';
import { vMediaFx } from '@/features/media-fx/media-fx-dom';
import { mediaFxGpuSupported, subscribeMediaFxGpuAvailability } from '@/features/media-fx/media-fx-gpu';
import {
  createMediaFxPreviewScheduler,
  mediaFxStaticPreviewSpec,
  mediaFxStaticSignature,
} from '@/features/media-fx/media-fx-preview';
import MediaFxPanel from '@/features/media-fx/MediaFxPanel.vue';

// effectMode:
// - bake (default): filter / advanced are baked into the confirmed file; no motion.
// - preserve: the file only carries drawing edits; the edited MediaFxSpec (including
//   motion) is returned next to it for the caller to persist.
const props = withDefaults(defineProps<{
  show: boolean;
  file: File | null;
  effectMode?: 'bake' | 'preserve';
  // preserve only: the spec the session starts from. Ignored in bake mode.
  initialMediaFx?: MediaFxSpec | null;
  // What the caller's target renderer supports (preserve), or an opt-out (bake).
  mediaFxCapabilities?: Partial<MediaFxCapabilities>;
}>(), {
  effectMode: 'bake',
  initialMediaFx: null,
  mediaFxCapabilities: () => ({}),
});

const emit = defineEmits<{
  (event: 'confirm', result: MessageImageEditorResult): void;
  (event: 'cancel'): void;
  (event: 'update:show', value: boolean): void;
}>();

const isPreserveMode = computed(() => props.effectMode === 'preserve');
const createSessionMediaFx = () => (
  isPreserveMode.value ? normalizeMediaFxSpec(props.initialMediaFx) : createDefaultMediaFxSpec()
);

const {
  activeTool,
  canEdit,
  canRestoreBeforeCrop,
  editorKey,
  errorMessage,
  exportEditedFile,
  exportPreviewSnapshot,
  history,
  imageHeight,
  imageWidth,
  isPreparing,
  isSaving,
  mediaFx,
  restoreLastDrawTool,
  restoreBeforeCrop,
  selectTool,
  setColor,
  setMediaFx,
  setThickness,
  settings,
  tools,
} = useMessageImageEditor(computed(() => props.file), {
  effectMode: () => props.effectMode,
  initialMediaFx: createSessionMediaFx,
});

const { width: viewportWidth } = useWindowSize();
const isMobileLayout = computed(() => (viewportWidth.value || 0) > 0 && viewportWidth.value < 768);

const vpImageRef = ref<any>(null);
const editorViewportRef = ref<HTMLElement | null>(null);
const viewportCenterRef = ref<HTMLElement | null>(null);
const { width: viewportInnerWidth, height: viewportInnerHeight } = useElementSize(editorViewportRef);
const { width: viewportCenterWidth, height: viewportCenterHeight } = useElementSize(viewportCenterRef);
const zoom = ref(1);
const pinchStartDistance = ref(0);
const pinchStartZoom = ref(1);
const isPanning = ref(false);
const panPointerId = ref<number | null>(null);
const panStartX = ref(0);
const panStartY = ref(0);
const panStartScrollLeft = ref(0);
const panStartScrollTop = ref(0);

const colorPresets = ['#e11d48', '#2563eb', '#16a34a', '#f59e0b', '#ffffff', '#0f172a'];
const thicknessValue = computed(() => Number(settings.value?.thickness || 6));
const currentColor = computed(() => String(settings.value?.color || '#e11d48'));
const isMoveTool = computed(() => activeTool.value === 'move');
const fitScale = computed(() => {
  const viewportWidthValue = Math.max(0, (viewportCenterWidth.value || viewportInnerWidth.value) - 8);
  const viewportHeightValue = Math.max(0, (viewportCenterHeight.value || viewportInnerHeight.value) - 8);
  if (!viewportWidthValue || !viewportHeightValue || !imageWidth.value || !imageHeight.value) {
    return 1;
  }
  return Math.min(
    viewportWidthValue / imageWidth.value,
    viewportHeightValue / imageHeight.value,
    1,
  );
});
const renderScale = computed(() => Math.max(0.01, fitScale.value * zoom.value));
const canvasShellStyle = computed(() => ({
  width: `${Math.max(1, Math.round(imageWidth.value * renderScale.value))}px`,
  height: `${Math.max(1, Math.round(imageHeight.value * renderScale.value))}px`,
}));
const imageStyle = computed(() => ({
  width: `${Math.max(1, Math.round(imageWidth.value * renderScale.value))}px`,
  height: `${Math.max(1, Math.round(imageHeight.value * renderScale.value))}px`,
}));
const mediaFxPanelOpen = ref(false);
const mediaFxGpuAvailable = ref(mediaFxGpuSupported());
const unsubscribeMediaFxGpuAvailability = subscribeMediaFxGpuAvailability((available) => {
  mediaFxGpuAvailable.value = available;
});
// bake: a still image has no motion, and advanced needs a working GPU at confirm time
// (existing advanced values stay editable so they can be turned off after a failure).
// preserve: the caller describes what its target renderer supports.
const editorMediaFxCapabilities = computed<MediaFxCapabilities>(() => {
  const caller = props.mediaFxCapabilities || {};
  if (!isPreserveMode.value) {
    return {
      motion: false,
      filters: caller.filters !== false,
      advanced: caller.advanced !== false
        && (mediaFxGpuAvailable.value || mediaFxAdvancedHasContent(mediaFx.value.advanced)),
      animatedMedia: false,
    };
  }
  return {
    motion: caller.motion !== false,
    filters: caller.filters !== false && caller.animatedMedia !== true,
    advanced: caller.advanced === true && caller.animatedMedia !== true,
    animatedMedia: caller.animatedMedia === true,
  };
});
const mediaFxPanelMode = computed(() => (isPreserveMode.value ? 'live' : 'bake'));
const mediaFxActive = computed(() => mediaFxHasContent(mediaFx.value));
const applyEffectPreviewToEditor = ref(false);
const effectPreviewReady = ref(false);
const editorEffectPreviewActive = computed(() => applyEffectPreviewToEditor.value && effectPreviewReady.value);
// CSS preview of the basic filter on the editing canvas. Motion is never applied here,
// so drawing / crop / pan / zoom coordinates stay stable; blur is scaled to match the
// full-resolution result. When the full preview overlay is active, static effects are
// already baked into that overlay and must not be applied to the shell a second time.
const mediaFxPreview = computed(() => ({
  spec: mediaFx.value,
  motion: false,
  filters: editorMediaFxCapabilities.value.filters && !editorEffectPreviewActive.value,
  blurScale: renderScale.value,
}));
const saveErrorMessage = ref('');

const handleMediaFxUpdate = (value: MediaFxSpec) => {
  saveErrorMessage.value = '';
  setMediaFx(value);
};

const handleEditorSave = async (payload: SaveParameters) => {
  saveErrorMessage.value = '';
  try {
    const result = await exportEditedFile(payload);
    emit('confirm', result);
    emit('update:show', false);
  } catch (error) {
    console.error('导出编辑图片失败', error);
    // The editor stays open; nothing is emitted without the requested effects.
    if (error instanceof MediaFxAdvancedBakeError) {
      mediaFxPanelOpen.value = true;
    }
    saveErrorMessage.value = error instanceof Error && error.message ? error.message : '图片导出失败，请重试';
  }
};

// Final-look preview: a downscaled raster of the current composition with the static
// effects baked by the same helper as the export, wrapped in v-media-fx for motion
// only. Only static changes (drawing, filter, advanced) re-rasterize; motion edits
// just update the directive. Stale jobs are dropped by the scheduler's generation.
const EFFECT_PREVIEW_MAX_EDGE = 480;
const EFFECT_PREVIEW_DELAY_MS = 160;
const effectPreviewCanvasRef = ref<HTMLCanvasElement | null>(null);
const editorEffectPreviewCanvasRef = ref<HTMLCanvasElement | null>(null);
const effectPreviewFrameRef = ref<HTMLElement | null>(null);
const { width: effectPreviewFrameWidth, height: effectPreviewFrameHeight } = useElementSize(effectPreviewFrameRef);
const effectPreviewSize = ref({ width: 0, height: 0 });
// The motion wrapper matches the fitted image box, so motion offsets are relative to
// the image like on the real renderers.
const effectPreviewBoxStyle = computed(() => {
  const { width, height } = effectPreviewSize.value;
  if (!width || !height) {
    return {};
  }
  const scale = Math.min(
    (effectPreviewFrameWidth.value || 236) / width,
    (effectPreviewFrameHeight.value || 150) / height,
  );
  return {
    width: `${Math.max(1, Math.floor(width * scale))}px`,
    height: `${Math.max(1, Math.floor(height * scale))}px`,
  };
});
const effectPreviewError = ref('');
const drawingRevision = ref(0);
const effectPreviewStaticSpec = computed(() => mediaFxStaticPreviewSpec(mediaFx.value, {
  filters: editorMediaFxCapabilities.value.filters,
  advanced: editorMediaFxCapabilities.value.advanced,
}));
const effectPreviewKey = computed(() => [
  editorKey.value,
  imageWidth.value,
  imageHeight.value,
  drawingRevision.value,
  mediaFxStaticSignature(effectPreviewStaticSpec.value),
].join(':'));
const effectPreviewMotion = computed(() => ({
  spec: mediaFx.value,
  filters: false,
  motion: editorMediaFxCapabilities.value.motion,
}));
const releaseCanvas = (canvas: HTMLCanvasElement) => {
  canvas.width = 0;
  canvas.height = 0;
};
const paintEffectPreviewCanvas = (target: HTMLCanvasElement | null, source: HTMLCanvasElement) => {
  if (!target) return;
  target.width = source.width;
  target.height = source.height;
  target.getContext('2d')?.drawImage(source, 0, 0);
};

const effectPreviewScheduler = createMediaFxPreviewScheduler<HTMLCanvasElement>({
  delayMs: EFFECT_PREVIEW_DELAY_MS,
  render: async () => {
    const snapshot = await exportPreviewSnapshot(getEditorSvg(), EFFECT_PREVIEW_MAX_EDGE);
    if (!snapshot) {
      return null;
    }
    try {
      return bakeMediaFxToCanvas(
        snapshot.canvas,
        { width: snapshot.canvas.width, height: snapshot.canvas.height },
        effectPreviewStaticSpec.value,
        { pixelRatio: snapshot.scale },
      );
    } finally {
      releaseCanvas(snapshot.canvas);
    }
  },
  commit: (canvas) => {
    paintEffectPreviewCanvas(effectPreviewCanvasRef.value, canvas);
    paintEffectPreviewCanvas(editorEffectPreviewCanvasRef.value, canvas);
    effectPreviewSize.value = { width: canvas.width, height: canvas.height };
    effectPreviewReady.value = true;
    effectPreviewError.value = '';
    releaseCanvas(canvas);
  },
  discard: releaseCanvas,
  onError: (error) => {
    console.warn('生成效果预览失败', error);
    effectPreviewError.value = '预览生成失败';
  },
});

const {
  activeShape,
  redo,
  reset,
  save,
  undo,
} = useEditor({
  vpImage: vpImageRef as any,
  tools: tools.value as any,
  history: history as any,
  settings: settings as any,
  width: imageWidth as any,
  height: imageHeight as any,
  emit: (event: string, payload: SaveParameters) => {
    if (event === 'save') {
      void handleEditorSave(payload);
    }
  },
});

const clampZoom = (value: number) => Math.max(1, Math.min(4, Number(value.toFixed(2))));

const getEditorSvg = () => {
  const exposed = vpImageRef.value?.svg as SVGElement | undefined;
  if (exposed) {
    return exposed;
  }
  return vpImageRef.value?.$refs?.svg as SVGElement | null;
};

const centerViewport = () => {
  const viewport = editorViewportRef.value;
  if (!viewport) {
    return;
  }
  viewport.scrollLeft = Math.max(0, (viewport.scrollWidth - viewport.clientWidth) / 2);
  viewport.scrollTop = Math.max(0, (viewport.scrollHeight - viewport.clientHeight) / 2);
};

const resetZoom = () => {
  zoom.value = 1;
  nextTick(() => {
    centerViewport();
  });
};

const beginPan = (clientX: number, clientY: number) => {
  const viewport = editorViewportRef.value;
  if (!viewport) {
    return false;
  }
  isPanning.value = true;
  panStartX.value = clientX;
  panStartY.value = clientY;
  panStartScrollLeft.value = viewport.scrollLeft;
  panStartScrollTop.value = viewport.scrollTop;
  return true;
};

const updatePan = (clientX: number, clientY: number) => {
  const viewport = editorViewportRef.value;
  if (!viewport || !isPanning.value) {
    return;
  }
  viewport.scrollLeft = panStartScrollLeft.value - (clientX - panStartX.value);
  viewport.scrollTop = panStartScrollTop.value - (clientY - panStartY.value);
};

const endPan = () => {
  isPanning.value = false;
  panPointerId.value = null;
};

const updateZoom = (value: number, anchor?: { x: number; y: number }) => {
  const nextZoom = clampZoom(value);
  const viewport = editorViewportRef.value;
  if (!viewport || nextZoom === zoom.value) {
    zoom.value = nextZoom;
    return;
  }

  const previousScrollWidth = Math.max(viewport.scrollWidth, 1);
  const previousScrollHeight = Math.max(viewport.scrollHeight, 1);
  const anchorX = anchor?.x ?? viewport.clientWidth / 2;
  const anchorY = anchor?.y ?? viewport.clientHeight / 2;
  const scrollRatioX = (viewport.scrollLeft + anchorX) / previousScrollWidth;
  const scrollRatioY = (viewport.scrollTop + anchorY) / previousScrollHeight;

  zoom.value = nextZoom;

  nextTick(() => {
    viewport.scrollLeft = scrollRatioX * viewport.scrollWidth - anchorX;
    viewport.scrollTop = scrollRatioY * viewport.scrollHeight - anchorY;
  });
};

const getTouchDistance = (touches: TouchList) => {
  if (touches.length < 2) {
    return 0;
  }
  const [first, second] = [touches[0], touches[1]];
  return Math.hypot(second.clientX - first.clientX, second.clientY - first.clientY);
};

const getTouchCenter = (touches: TouchList) => {
  const [first, second] = [touches[0], touches[1]];
  return {
    x: (first.clientX + second.clientX) / 2,
    y: (first.clientY + second.clientY) / 2,
  };
};

const handleViewportWheel = (event: WheelEvent) => {
  if (!canEdit.value) {
    return;
  }
  const viewport = editorViewportRef.value;
  if (!viewport) {
    return;
  }
  const rect = viewport.getBoundingClientRect();
  const delta = event.deltaY < 0 ? 0.14 : -0.14;
  updateZoom(zoom.value + delta, {
    x: event.clientX - rect.left,
    y: event.clientY - rect.top,
  });
};

const handleTouchStart = (event: TouchEvent) => {
  if (isMoveTool.value && event.touches.length === 1) {
    beginPan(event.touches[0].clientX, event.touches[0].clientY);
    pinchStartDistance.value = 0;
    return;
  }
  if (event.touches.length !== 2) {
    return;
  }
  isPanning.value = false;
  pinchStartDistance.value = getTouchDistance(event.touches);
  pinchStartZoom.value = zoom.value;
};

const handleTouchMove = (event: TouchEvent) => {
  if (isMoveTool.value && event.touches.length === 1 && pinchStartDistance.value <= 0) {
    updatePan(event.touches[0].clientX, event.touches[0].clientY);
    return;
  }
  if (event.touches.length !== 2 || pinchStartDistance.value <= 0) {
    return;
  }
  const viewport = editorViewportRef.value;
  if (!viewport) {
    return;
  }
  const rect = viewport.getBoundingClientRect();
  const currentDistance = getTouchDistance(event.touches);
  const center = getTouchCenter(event.touches);
  updateZoom(pinchStartZoom.value * (currentDistance / pinchStartDistance.value), {
    x: center.x - rect.left,
    y: center.y - rect.top,
  });
};

const handleTouchEnd = (event: TouchEvent) => {
  if (event.touches.length === 0) {
    endPan();
  }
  if (event.touches.length < 2) {
    pinchStartDistance.value = 0;
  }
};

const handleViewportPointerDown = (event: PointerEvent) => {
  if (!isMoveTool.value || !canEdit.value || event.button !== 0) {
    return;
  }
  if (!beginPan(event.clientX, event.clientY)) {
    return;
  }
  panPointerId.value = event.pointerId;
  editorViewportRef.value?.setPointerCapture?.(event.pointerId);
  event.preventDefault();
  event.stopPropagation();
};

const handleViewportPointerMove = (event: PointerEvent) => {
  if (!isPanning.value || panPointerId.value !== event.pointerId) {
    return;
  }
  updatePan(event.clientX, event.clientY);
};

const handleViewportPointerUp = (event: PointerEvent) => {
  if (panPointerId.value !== null && panPointerId.value !== event.pointerId) {
    return;
  }
  editorViewportRef.value?.releasePointerCapture?.(event.pointerId);
  endPan();
};

const handleCancel = () => {
  emit('update:show', false);
  emit('cancel');
};

const handleColorInput = (event: Event) => {
  const target = event.target as HTMLInputElement | null;
  if (!target) {
    return;
  }
  setColor(target.value);
};

const handleThicknessUpdate = (value: number | [number, number]) => {
  const nextValue = Array.isArray(value) ? value[0] : value;
  setThickness(Number(nextValue));
};

const handleToolSelect = async (tool: MessageImageEditorTool) => {
  await selectTool(tool, getEditorSvg());
};

const handleRestoreDrawTool = async () => {
  await restoreLastDrawTool(getEditorSvg());
};

const handleUndoAction = () => {
  if (canRestoreBeforeCrop.value && history.value.length <= 1) {
    if (restoreBeforeCrop()) {
      resetZoom();
      return;
    }
  }
  undo();
};

const handleResetAction = async () => {
  if (canRestoreBeforeCrop.value) {
    if (restoreBeforeCrop()) {
      resetZoom();
      return;
    }
  }
  await reset();
  handleMediaFxUpdate(createSessionMediaFx());
  resetZoom();
};

watch(
  () => props.show,
  (show) => {
    saveErrorMessage.value = '';
    if (!show) {
      mediaFxPanelOpen.value = false;
    }
    if (!show) {
      applyEffectPreviewToEditor.value = false;
    }
    if (show) {
      // A reopened session starts from its initial effects (default in bake mode, the
      // caller's spec in preserve mode), like the drawing history reset below.
      setMediaFx(createSessionMediaFx());
      nextTick(() => {
        void reset();
        resetZoom();
      });
    }
  },
  { immediate: true },
);

watch(
  () => editorKey.value,
  () => {
    if (!props.show || !tools.value.length) {
      return;
    }
    nextTick(() => {
      void reset();
      resetZoom();
    });
  },
);

watch(
  () => (props.show && mediaFxPanelOpen.value ? history.value : null),
  (value) => {
    if (value) {
      drawingRevision.value += 1;
    }
  },
  { deep: true },
);

watch(
  () => (props.show && mediaFxPanelOpen.value && canEdit.value ? effectPreviewKey.value : null),
  (key) => {
    if (key === null) {
      effectPreviewScheduler.invalidate();
      effectPreviewReady.value = false;
      effectPreviewError.value = '';
      return;
    }
    effectPreviewScheduler.request(key);
  },
  { flush: 'post' },
);

watch(isMoveTool, (enabled) => {
  if (!enabled) {
    endPan();
  }
});

watch(
  () => props.show,
  (show) => {
    if (typeof document === 'undefined') {
      return;
    }
    document.body.style.overflow = show ? 'hidden' : '';
  },
  { immediate: true },
);

useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  if (!props.show) {
    return;
  }
  if (event.key === 'Escape') {
    event.preventDefault();
    handleCancel();
  }
});

onUnmounted(() => {
  effectPreviewScheduler.dispose();
  unsubscribeMediaFxGpuAvailability();
  endPan();
  if (typeof document !== 'undefined') {
    document.body.style.overflow = '';
  }
});
</script>

<template>
  <Teleport to="body">
    <div
      v-if="show"
      class="message-image-editor-overlay"
      :class="{ 'message-image-editor-overlay--mobile': isMobileLayout }"
      @click.self="handleCancel"
    >
      <div class="message-image-editor" :class="{ 'message-image-editor--mobile': isMobileLayout }">
      <header class="message-image-editor__header">
        <h3 class="message-image-editor__title">编辑上传图片</h3>
        <n-button quaternary circle class="message-image-editor__close message-image-editor__icon-button" @click="handleCancel">
          <template #icon>
            <n-icon :component="X" size="18" />
          </template>
        </n-button>
      </header>

      <div v-if="isPreparing" class="message-image-editor__state">
        <n-spin size="large" />
        <p>正在载入图片…</p>
      </div>

      <div v-else-if="errorMessage" class="message-image-editor__state message-image-editor__state--error">
        <p>{{ errorMessage }}</p>
      </div>

      <template v-else>
        <div class="message-image-editor__workspace">
        <div
          ref="editorViewportRef"
          class="message-image-editor__viewport"
          :class="{
            'message-image-editor__viewport--move': isMoveTool,
            'message-image-editor__viewport--panning': isPanning,
          }"
          @pointerdown.capture="handleViewportPointerDown"
          @pointermove="handleViewportPointerMove"
          @pointerup="handleViewportPointerUp"
          @pointercancel="handleViewportPointerUp"
          @wheel.prevent="handleViewportWheel"
          @touchstart.passive="handleTouchStart"
          @touchmove.prevent="handleTouchMove"
          @touchend="handleTouchEnd"
          @touchcancel="handleTouchEnd"
        >
          <div ref="viewportCenterRef" class="message-image-editor__viewport-center">
            <div
              v-media-fx="mediaFxPreview"
              class="message-image-editor__canvas-shell"
              :class="{ 'message-image-editor__canvas-shell--effect-preview': editorEffectPreviewActive }"
              :style="canvasShellStyle"
            >
              <VpImage
                ref="vpImageRef"
                :key="editorKey"
                class="message-image-editor__image"
                :style="imageStyle"
                :tools="tools"
                :active-shape="activeShape"
                :history="history"
                :width="imageWidth"
                :height="imageHeight"
              />
              <div
                v-media-fx="effectPreviewMotion"
                class="message-image-editor__editor-effect-preview"
                :class="{ 'is-active': editorEffectPreviewActive }"
                aria-hidden="true"
              >
                <canvas
                  ref="editorEffectPreviewCanvasRef"
                  class="message-image-editor__editor-effect-preview-canvas"
                />
              </div>
            </div>
          </div>
        </div>
        <aside v-if="mediaFxPanelOpen" class="message-image-editor__fx-panel" aria-label="图像效果">
          <section class="message-image-editor__fx-preview" aria-label="效果预览">
            <header class="message-image-editor__fx-preview-heading">
              <span>效果预览</span>
              <small v-if="effectPreviewError">{{ effectPreviewError }}</small>
              <small v-else-if="!effectPreviewReady">生成中…</small>
            </header>
            <div ref="effectPreviewFrameRef" class="message-image-editor__fx-preview-frame">
              <div v-media-fx="effectPreviewMotion" class="message-image-editor__fx-preview-motion" :style="effectPreviewBoxStyle">
                <canvas
                  ref="effectPreviewCanvasRef"
                  class="message-image-editor__fx-preview-canvas"
                  :class="{ 'is-hidden': !effectPreviewReady }"
                />
              </div>
            </div>
            <label class="message-image-editor__fx-preview-toggle">
              <span>应用到编辑区</span>
              <n-switch v-model:value="applyEffectPreviewToEditor" size="small" />
            </label>
          </section>
          <MediaFxPanel
            :model-value="mediaFx"
            :mode="mediaFxPanelMode"
            :capabilities="editorMediaFxCapabilities"
            :disabled="isSaving"
            @update:model-value="handleMediaFxUpdate"
          />
        </aside>
        </div>

        <div class="message-image-editor__controls">
          <p v-if="saveErrorMessage" class="message-image-editor__save-error" role="alert">{{ saveErrorMessage }}</p>
          <div class="message-image-editor__tool-row">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  circle
                  quaternary
                  class="message-image-editor__tool"
                  :class="{ 'message-image-editor__tool--active': activeTool === 'move' }"
                  @click="handleToolSelect('move')"
                >
                  ✥
                </n-button>
              </template>
              拖拽图片
            </n-tooltip>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  circle
                  quaternary
                  class="message-image-editor__tool"
                  :class="{ 'message-image-editor__tool--active': activeTool === 'freehand' }"
                  @click="handleToolSelect('freehand')"
                >
                  ✎
                </n-button>
              </template>
              自由涂鸦
            </n-tooltip>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  circle
                  quaternary
                  class="message-image-editor__tool"
                  :class="{ 'message-image-editor__tool--active': activeTool === 'rectangle' }"
                  @click="handleToolSelect('rectangle')"
                >
                  ▭
                </n-button>
              </template>
              矩形标注
            </n-tooltip>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  circle
                  quaternary
                  class="message-image-editor__tool"
                  :class="{ 'message-image-editor__tool--active': activeTool === 'crop' }"
                  @click="handleToolSelect('crop')"
                >
                  ✂
                </n-button>
              </template>
              裁剪
            </n-tooltip>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  circle
                  quaternary
                  class="message-image-editor__tool"
                  :class="{
                    'message-image-editor__tool--active': mediaFxPanelOpen,
                    'message-image-editor__tool--marked': mediaFxActive && !mediaFxPanelOpen,
                  }"
                  aria-label="效果"
                  :aria-expanded="mediaFxPanelOpen"
                  @click="mediaFxPanelOpen = !mediaFxPanelOpen"
                >
                  <template #icon>
                    <n-icon :component="Adjustments" size="18" />
                  </template>
                </n-button>
              </template>
              效果
            </n-tooltip>
            <div class="message-image-editor__action-icons">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button circle quaternary size="small" class="message-image-editor__icon-button" :disabled="isSaving" @click="handleUndoAction">
                    <template #icon>
                      <n-icon :component="ArrowBackUp" size="16" />
                    </template>
                  </n-button>
                </template>
                撤销
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button circle quaternary size="small" class="message-image-editor__icon-button" :disabled="isSaving" @click="redo">
                    <template #icon>
                      <n-icon :component="ArrowForwardUp" size="16" />
                    </template>
                  </n-button>
                </template>
                重做
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button circle quaternary size="small" class="message-image-editor__icon-button" :disabled="isSaving || isPreparing" @click="handleResetAction">
                    <template #icon>
                      <n-icon :component="Refresh" size="16" />
                    </template>
                  </n-button>
                </template>
                重置
              </n-tooltip>
              <n-tooltip v-if="activeTool === 'crop'" trigger="hover">
                <template #trigger>
                  <n-button
                    circle
                    quaternary
                    size="small"
                    class="message-image-editor__icon-button"
                    :disabled="isPreparing"
                    @click="handleRestoreDrawTool"
                  >
                    ←
                  </n-button>
                </template>
                应用裁剪并返回
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button circle quaternary size="small" class="message-image-editor__icon-button" :disabled="isSaving" @click="handleCancel">
                    <template #icon>
                      <n-icon :component="X" size="16" />
                    </template>
                  </n-button>
                </template>
                取消
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button circle type="primary" size="small" :disabled="!canEdit" :loading="isSaving" @click="save">
                    <template #icon>
                      <n-icon :component="Check" size="16" />
                    </template>
                  </n-button>
                </template>
                确认插入
              </n-tooltip>
            </div>
            <div class="message-image-editor__spacer" />
          </div>

          <div class="message-image-editor__style-row">
            <div class="message-image-editor__palette">
              <button
                v-for="color in colorPresets"
                :key="color"
                type="button"
                class="message-image-editor__swatch"
                :class="{ 'message-image-editor__swatch--active': currentColor === color }"
                :style="{ backgroundColor: color }"
                @click="setColor(color)"
              />
              <input
                :value="currentColor"
                type="color"
                class="message-image-editor__color-input"
                @input="handleColorInput"
              />
            </div>
            <div class="message-image-editor__zoom">
              <n-button circle quaternary size="small" class="message-image-editor__icon-button" :disabled="zoom <= 1" @click="updateZoom(zoom - 0.2)">
                －
              </n-button>
              <n-button quaternary size="small" class="message-image-editor__zoom-reset message-image-editor__icon-button" @click="resetZoom">
                {{ Math.round(zoom * 100) }}%
              </n-button>
              <n-button circle quaternary size="small" class="message-image-editor__icon-button" :disabled="zoom >= 4" @click="updateZoom(zoom + 0.2)">
                ＋
              </n-button>
            </div>
          </div>

          <div class="message-image-editor__thickness-row">
            <div class="message-image-editor__thickness">
              <span class="message-image-editor__thickness-label">粗细</span>
              <n-slider
                :value="thicknessValue"
                :min="1"
                :max="24"
                :step="1"
                @update:value="handleThicknessUpdate"
              />
            </div>
          </div>
        </div>
      </template>
      </div>
    </div>
  </Teleport>
</template>

<style scoped lang="scss">
.message-image-editor-overlay {
  --editor-overlay-bg: color-mix(in srgb, var(--sc-bg-page, #f5f5f7) 72%, transparent);
  --editor-shell-bg: color-mix(in srgb, var(--sc-bg-elevated, #ffffff) 94%, var(--sc-bg-surface, #ffffff) 6%);
  --editor-shell-border: color-mix(in srgb, var(--sc-border-strong, rgba(148, 163, 184, 0.24)) 88%, transparent);
  --editor-shell-shadow: 0 24px 70px rgba(15, 23, 42, 0.16);
  --editor-fg: var(--sc-text-primary, #1f2937);
  --editor-muted: var(--sc-text-secondary, #6b7280);
  --editor-btn-bg: color-mix(in srgb, var(--sc-bg-layer, #f5f5f7) 88%, var(--primary-color, #3388de) 12%);
  --editor-btn-bg-hover: color-mix(in srgb, var(--sc-bg-layer, #f5f5f7) 74%, var(--primary-color, #3388de) 20%);
  --editor-btn-bg-pressed: color-mix(in srgb, var(--sc-bg-layer, #f5f5f7) 66%, var(--primary-color, #3388de) 26%);
  --editor-btn-border: color-mix(in srgb, var(--sc-border-strong, rgba(148, 163, 184, 0.3)) 82%, transparent);
  --editor-btn-text: var(--sc-text-primary, #1f2937);
  --editor-panel-bg:
    linear-gradient(135deg, color-mix(in srgb, var(--primary-color, #3388de) 4%, transparent), transparent 35%),
    color-mix(in srgb, var(--sc-bg-layer, #f5f5f7) 94%, var(--sc-bg-surface, #ffffff) 6%);
  --editor-state-bg:
    radial-gradient(circle at top, color-mix(in srgb, var(--primary-color, #3388de) 10%, transparent), transparent 52%),
    color-mix(in srgb, var(--sc-bg-layer, #f5f5f7) 90%, var(--sc-bg-surface, #ffffff) 10%);
  --editor-image-bg:
    linear-gradient(135deg, color-mix(in srgb, var(--primary-color, #3388de) 5%, transparent), transparent 36%),
    color-mix(in srgb, var(--sc-bg-surface, #ffffff) 96%, var(--sc-bg-layer, #f5f5f7) 4%);
  position: fixed;
  inset: 0;
  z-index: 2400;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: var(--editor-overlay-bg);
  backdrop-filter: blur(4px);
}

.message-image-editor-overlay--mobile {
  padding: 0;
}

.message-image-editor {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  width: min(1040px, 96vw);
  height: min(92vh, 920px);
  min-height: min(92vh, 920px);
  padding: 18px;
  border-radius: 24px;
  border: 1px solid var(--editor-shell-border);
  background: var(--editor-shell-bg);
  background-color: var(--editor-shell-bg);
  box-shadow: var(--editor-shell-shadow);
  overflow: hidden;
  color: var(--editor-fg);
}

.message-image-editor--mobile {
  gap: 0.75rem;
  width: 100vw;
  height: 100vh;
  min-height: 100vh;
  border-radius: 0;
  padding: 16px;
}

.message-image-editor__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.message-image-editor__title {
  margin: 0;
  font-size: 1rem;
  line-height: 1.35;
  font-weight: 600;
  color: var(--editor-fg);
}

.message-image-editor__close {
  flex: 0 0 auto;
}

.message-image-editor__state {
  flex: 1 1 auto;
  min-height: 320px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  border-radius: 18px;
  border: 1px dashed var(--editor-shell-border);
  background: var(--editor-state-bg);
  background-color: var(--sc-bg-layer, #f5f5f7);
  color: var(--editor-muted);
}

.message-image-editor__state--error {
  color: #ef4444;
}

.message-image-editor__workspace {
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
  display: flex;
  gap: 0.75rem;
}

.message-image-editor__fx-panel {
  flex: 0 0 260px;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 0.75rem;
  border-radius: 18px;
  border: 1px solid var(--editor-shell-border);
  background: var(--editor-panel-bg);
  background-color: var(--sc-bg-layer, #f5f5f7);
  color: var(--editor-fg);
}

.message-image-editor__fx-preview {
  display: grid;
  gap: 6px;
  margin-bottom: 10px;
  font-size: 12px;
}

.message-image-editor__fx-preview-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-weight: 600;
  opacity: 0.85;
}

.message-image-editor__fx-preview-heading small {
  font-weight: 400;
  color: var(--editor-muted);
}

.message-image-editor__fx-preview-frame {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 150px;
  overflow: hidden;
  border-radius: 12px;
  border: 1px solid var(--editor-shell-border);
  background: var(--editor-image-bg);
  background-color: var(--sc-bg-surface, #ffffff);
}

.message-image-editor__fx-preview-motion {
  flex: 0 0 auto;
}

.message-image-editor__fx-preview-canvas {
  display: block;
  width: 100%;
  height: 100%;
}

.message-image-editor__fx-preview-canvas.is-hidden {
  visibility: hidden;
}

.message-image-editor__fx-preview-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-height: 24px;
  padding: 1px 2px 0;
  color: var(--editor-muted);
  font-size: 12px;
  cursor: pointer;
}

.message-image-editor__save-error {
  margin: 0;
  font-size: 0.82rem;
  color: #ef4444;
}

.message-image-editor__viewport {
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
  overflow: auto;
  overscroll-behavior: contain;
  border-radius: 18px;
  padding: 0.35rem;
  background: var(--editor-panel-bg);
  background-color: var(--sc-bg-layer, #f5f5f7);
}

.message-image-editor__viewport--move {
  cursor: grab;
  touch-action: none;
}

.message-image-editor__viewport--panning {
  cursor: grabbing;
}

.message-image-editor__viewport-center {
  min-width: 100%;
  min-height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.message-image-editor__canvas-shell {
  position: relative;
  flex: 0 0 auto;
}

.message-image-editor__canvas-shell--effect-preview .message-image-editor__image {
  opacity: 0;
}

.message-image-editor__editor-effect-preview {
  position: absolute;
  inset: 0;
  z-index: 2;
  pointer-events: none;
  visibility: hidden;
  border-radius: 18px;
  border: 1px solid var(--editor-shell-border);
  box-sizing: border-box;
  transform-origin: center;
}

.message-image-editor__editor-effect-preview.is-active {
  visibility: visible;
}

.message-image-editor__editor-effect-preview-canvas {
  display: block;
  width: 100%;
  height: 100%;
  border-radius: inherit;
}

.message-image-editor__image {
  display: block;
  border-radius: 18px;
  overflow: hidden;
  border: 1px solid var(--editor-shell-border);
  background: var(--editor-image-bg);
  background-color: var(--sc-bg-surface, #ffffff);
}

.message-image-editor__image :deep(.vp-image) {
  display: block;
  width: 100%;
  height: 100%;
  background: var(--editor-image-bg);
  background-color: var(--sc-bg-surface, #ffffff);
  color: var(--editor-fg);
}

.message-image-editor__controls {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.1rem 0 0;
}

.message-image-editor__tool-row,
.message-image-editor__style-row,
.message-image-editor__thickness-row {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  flex-wrap: wrap;
}

.message-image-editor__tool {
  font-size: 1rem;
}

.message-image-editor__tool,
.message-image-editor__icon-button {
  --n-color: var(--editor-btn-bg) !important;
  --n-color-hover: var(--editor-btn-bg-hover) !important;
  --n-color-pressed: var(--editor-btn-bg-pressed) !important;
  --n-color-focus: var(--editor-btn-bg-hover) !important;
  --n-text-color: var(--editor-btn-text) !important;
  --n-text-color-hover: var(--editor-btn-text) !important;
  --n-text-color-pressed: var(--editor-btn-text) !important;
  --n-text-color-focus: var(--editor-btn-text) !important;
  --n-border: 1px solid var(--editor-btn-border) !important;
  --n-border-hover: 1px solid var(--editor-btn-border) !important;
  --n-border-pressed: 1px solid var(--editor-btn-border) !important;
  --n-border-focus: 1px solid var(--editor-btn-border) !important;
  --n-ripple-color: color-mix(in srgb, var(--primary-color, #3388de) 22%, transparent) !important;
}

.message-image-editor__tool :deep(.n-button__content),
.message-image-editor__icon-button :deep(.n-button__content) {
  color: var(--editor-btn-text);
}

.message-image-editor__tool--active {
  background: color-mix(in srgb, var(--primary-color, #3388de) 14%, transparent);
  color: var(--primary-color, #3388de);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--primary-color, #3388de) 28%, transparent);
}

.message-image-editor__tool--marked {
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--primary-color, #3388de) 45%, transparent);
}

.message-image-editor__spacer {
  flex: 1 1 auto;
}

.message-image-editor__zoom {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  flex: 0 0 auto;
}

.message-image-editor__zoom-reset {
  min-width: 4.6rem;
}

.message-image-editor__palette {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  flex: 1 1 auto;
  min-width: 0;
  flex-wrap: nowrap;
  overflow-x: auto;
  overflow-y: hidden;
}

.message-image-editor__swatch {
  width: 1.65rem;
  height: 1.65rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--sc-border-strong, rgba(148, 163, 184, 0.32)) 92%, transparent);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.1) inset;
  cursor: pointer;
}

.message-image-editor__swatch--active {
  transform: scale(1.08);
  box-shadow:
    0 0 0 2px color-mix(in srgb, var(--primary-color, #3388de) 35%, transparent),
    0 0 0 1px rgba(255, 255, 255, 0.2) inset;
}

.message-image-editor__color-input {
  width: 2rem;
  height: 2rem;
  padding: 0;
  border: none;
  background: transparent;
  cursor: pointer;
}

.message-image-editor__color-input::-webkit-color-swatch-wrapper {
  padding: 0;
}

.message-image-editor__color-input::-webkit-color-swatch {
  border: 1px solid var(--editor-shell-border);
  border-radius: 999px;
}

.message-image-editor__thickness {
  min-width: min(240px, 100%);
  flex: 1 1 220px;
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.message-image-editor__thickness-label {
  flex: 0 0 auto;
  color: var(--editor-muted);
  font-size: 0.82rem;
}

.message-image-editor__thickness :deep(.n-slider) {
  flex: 1 1 auto;
  --n-rail-color: color-mix(in srgb, var(--sc-bg-layer, #f5f5f7) 82%, var(--editor-shell-border) 18%);
  --n-fill-color: var(--primary-color, #3388de);
  --n-fill-color-hover: var(--primary-color, #3388de);
  --n-handle-color: var(--primary-color, #3388de);
  --n-handle-color-hover: var(--primary-color, #3388de);
  --n-dot-color: var(--primary-color, #3388de);
}

.message-image-editor__action-icons {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  flex-wrap: wrap;
}

@media (max-width: 767px) {
  .message-image-editor__workspace {
    flex-direction: column;
  }

  .message-image-editor__fx-panel {
    flex: 0 1 auto;
    max-height: 38vh;
    padding: 0.6rem 0.75rem;
  }

  .message-image-editor__style-row,
  .message-image-editor__tool-row,
  .message-image-editor__thickness-row {
    align-items: stretch;
  }

  .message-image-editor__spacer {
    display: none;
  }

  .message-image-editor__tool-row {
    justify-content: flex-start;
  }

  .message-image-editor__style-row {
    flex-wrap: nowrap;
    align-items: center;
  }

  .message-image-editor__palette {
    flex: 1 1 auto;
  }

  .message-image-editor__zoom {
    flex: 0 0 auto;
  }

  .message-image-editor__thickness {
    min-width: 100%;
  }
}

:global(html[data-display-palette='night']) .message-image-editor-overlay,
:global(:root[data-display-palette='night']) .message-image-editor-overlay,
:global([data-display-palette='night']) .message-image-editor-overlay {
  --editor-overlay-bg: color-mix(in srgb, #020617 72%, transparent);
  --editor-shell-shadow: 0 26px 72px rgba(0, 0, 0, 0.45);
  --editor-btn-bg: color-mix(in srgb, var(--sc-bg-layer, #2f2f34) 78%, var(--primary-color, #60a5fa) 22%);
  --editor-btn-bg-hover: color-mix(in srgb, var(--sc-bg-layer, #2f2f34) 64%, var(--primary-color, #60a5fa) 28%);
  --editor-btn-bg-pressed: color-mix(in srgb, var(--sc-bg-layer, #2f2f34) 58%, var(--primary-color, #60a5fa) 32%);
  --editor-panel-bg:
    linear-gradient(135deg, color-mix(in srgb, var(--primary-color, #60a5fa) 14%, transparent), transparent 42%),
    color-mix(in srgb, var(--sc-bg-layer, #2f2f34) 92%, var(--sc-bg-elevated, #26262c) 8%);
  --editor-state-bg:
    radial-gradient(circle at top, color-mix(in srgb, var(--primary-color, #60a5fa) 18%, transparent), transparent 52%),
    color-mix(in srgb, var(--sc-bg-layer, #2f2f34) 88%, var(--sc-bg-elevated, #26262c) 12%);
  --editor-image-bg:
    linear-gradient(135deg, color-mix(in srgb, var(--primary-color, #60a5fa) 14%, transparent), transparent 42%),
    color-mix(in srgb, var(--sc-bg-elevated, #26262c) 94%, var(--sc-bg-layer, #2f2f34) 6%);
}

:global(html[data-custom-theme='true']) .message-image-editor-overlay,
:global(:root[data-custom-theme='true']) .message-image-editor-overlay,
:global([data-custom-theme='true']) .message-image-editor-overlay {
  --editor-overlay-bg: color-mix(in srgb, var(--sc-bg-page, #f5f5f7) 66%, transparent);
  --editor-shell-shadow:
    0 24px 70px color-mix(in srgb, var(--sc-text-primary, #0f172a) 14%, transparent);
}

:global(html[data-custom-theme='true'][data-display-palette='night']) .message-image-editor-overlay,
:global(:root[data-custom-theme='true'][data-display-palette='night']) .message-image-editor-overlay,
:global([data-custom-theme='true'][data-display-palette='night']) .message-image-editor-overlay {
  --editor-overlay-bg: color-mix(in srgb, #020617 74%, transparent);
}
</style>
