<template>
  <teleport to="body">
    <div
      v-if="available && state.mode !== 'hidden'"
      ref="windowRef"
      class="dice-tray-floating-window"
      :class="{
        'is-mobile': isMobile,
        'is-scaled': scaledLayout,
        'is-minimized': state.mode === 'minimized',
        'is-resizing': resizing,
      }"
      :style="windowStyle"
      @pointerdown="handlePointerDown"
    >
      <div
        v-if="state.mode === 'expanded'"
        ref="contentRef"
        class="dice-tray-floating-window__content"
        :style="contentStyle"
      >
        <DiceTray
          floating
          :default-dice="defaultDice"
          :can-edit-default="canEditDefault"
          :built-in-dice-enabled="builtInDiceEnabled"
          :bot-feature-enabled="botFeatureEnabled"
          @insert="emit('insert', $event)"
          @roll="emit('roll', $event)"
          @update-default="emit('update-default', $event)"
          @minimize="minimize"
          @close="hide"
        >
          <template #header-actions>
            <slot name="header-actions" :is-mobile="isMobile" />
          </template>
        </DiceTray>
      </div>

      <button
        v-else
        type="button"
        class="dice-tray-floating-window__badge"
        title="恢复掷骰面板"
        aria-label="恢复掷骰面板"
      >
        <svg viewBox="0 0 48 48" aria-hidden="true">
          <path d="M10 16h28v22H10z" />
          <path d="M14 11h20l4 5H10z" />
          <circle cx="18" cy="25" r="2" />
          <circle cx="30" cy="25" r="2" />
          <circle cx="24" cy="32" r="2" />
        </svg>
      </button>
    </div>
  </teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import DiceTray from './DiceTray.vue';

type WindowMode = 'hidden' | 'expanded' | 'minimized';

interface PersistedState {
  version: 1;
  mode: WindowMode;
  window: {
    x: number;
    y: number;
    width: number;
    height?: number;
  };
  badge: {
    x: number;
    y: number;
  };
}

const props = withDefaults(defineProps<{
  available?: boolean;
  storageScope: string;
  defaultDice?: string;
  canEditDefault?: boolean;
  builtInDiceEnabled?: boolean;
  botFeatureEnabled?: boolean;
}>(), {
  available: true,
  defaultDice: 'd20',
  canEditDefault: false,
  builtInDiceEnabled: true,
  botFeatureEnabled: false,
});

const emit = defineEmits<{
  (event: 'insert', expr: string): void;
  (event: 'roll', expr: string): void;
  (event: 'update-default', expr: string): void;
  (event: 'mode-change', mode: WindowMode): void;
}>();

const STORAGE_PREFIX = 'sealchat:dice-tray-window:v1';
const VIEWPORT_PADDING = 12;
const BADGE_SIZE = 52;
const DEFAULT_WIDTH = 420;
const MIN_WIDTH = 320;
const MIN_HEIGHT = 280;
const MOBILE_BREAKPOINT = 768;
const DRAG_THRESHOLD = 6;

const windowRef = ref<HTMLElement | null>(null);
const contentRef = ref<HTMLElement | null>(null);
const viewportWidth = ref(typeof window === 'undefined' ? 1200 : Math.max(window.innerWidth, 1));
const viewportHeight = ref(typeof window === 'undefined' ? 800 : Math.max(window.innerHeight, 1));
const isMobile = ref(viewportWidth.value < MOBILE_BREAKPOINT);
const contentHeight = ref(620);
const resizing = ref(false);
const resizePointerId = ref<number | null>(null);
const drag = ref<{
  kind: 'window' | 'badge';
  pointerId: number;
  startClientX: number;
  startClientY: number;
  startX: number;
  startY: number;
  moved: boolean;
} | null>(null);

const state = reactive<PersistedState>(createDefaultState());
let resizeObserver: ResizeObserver | null = null;
let contentResizeObserver: ResizeObserver | null = null;
let observedSize = { width: DEFAULT_WIDTH, height: 0 };

const storageKey = computed(() => `${STORAGE_PREFIX}:${props.storageScope || 'main'}:${isMobile.value ? 'mobile' : 'desktop'}`);
const proportionalLayout = computed(() => isMobile.value || props.storageScope === 'embed:theater');
const panelScale = computed(() => {
  if (!proportionalLayout.value) return 1;
  const viewport = viewportSize();
  const widthScale = (viewport.width * 0.95) / DEFAULT_WIDTH;
  const heightScale = (viewport.height - VIEWPORT_PADDING * 2) / Math.max(contentHeight.value, MIN_HEIGHT);
  return Math.min(1, widthScale, heightScale);
});
const scaledLayout = computed(() => proportionalLayout.value);
const scaledWidth = computed(() => DEFAULT_WIDTH * panelScale.value);
const scaledHeight = computed(() => contentHeight.value * panelScale.value);

function viewportSize() {
  return {
    width: viewportWidth.value,
    height: viewportHeight.value,
  };
}

function createDefaultState(): PersistedState {
  const viewport = viewportSize();
  const width = Math.min(DEFAULT_WIDTH, Math.max(MIN_WIDTH, viewport.width - VIEWPORT_PADDING * 2));
  const renderedWidth = isMobile.value ? Math.min(DEFAULT_WIDTH, viewport.width * 0.95) : width;
  return {
    version: 1,
    mode: 'hidden',
    window: {
      x: Math.max(VIEWPORT_PADDING, Math.round((viewport.width - renderedWidth) / 2)),
      y: Math.max(VIEWPORT_PADDING, Math.round((viewport.height - 620) / 2)),
      width,
    },
    badge: {
      x: Math.max(VIEWPORT_PADDING, viewport.width - BADGE_SIZE - 24),
      y: Math.max(VIEWPORT_PADDING, viewport.height - BADGE_SIZE - 120),
    },
  };
}

function finiteNumber(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function loadState() {
  const fallback = createDefaultState();
  let loaded = fallback;
  try {
    const raw = localStorage.getItem(storageKey.value);
    const parsed = raw ? JSON.parse(raw) as Partial<PersistedState> : null;
    if (parsed?.version === 1) {
      const mode: WindowMode = parsed.mode === 'expanded' || parsed.mode === 'minimized' ? parsed.mode : 'hidden';
      loaded = {
        version: 1,
        mode,
        window: {
          x: finiteNumber(parsed.window?.x, fallback.window.x),
          y: finiteNumber(parsed.window?.y, fallback.window.y),
          width: finiteNumber(parsed.window?.width, fallback.window.width),
          height: parsed.window?.height === undefined ? undefined : finiteNumber(parsed.window.height, MIN_HEIGHT),
        },
        badge: {
          x: finiteNumber(parsed.badge?.x, fallback.badge.x),
          y: finiteNumber(parsed.badge?.y, fallback.badge.y),
        },
      };
    }
  } catch {
    loaded = fallback;
  }
  Object.assign(state, loaded);
  clampState();
  emit('mode-change', state.mode);
  nextTick(clampState);
}

function persistState() {
  try {
    localStorage.setItem(storageKey.value, JSON.stringify(state));
  } catch {
    // ignore unavailable storage
  }
}

function clampPoint(x: number, y: number, width: number, height: number) {
  const viewport = viewportSize();
  return {
    x: Math.min(Math.max(VIEWPORT_PADDING, Math.round(x)), Math.max(VIEWPORT_PADDING, viewport.width - width - VIEWPORT_PADDING)),
    y: Math.min(Math.max(VIEWPORT_PADDING, Math.round(y)), Math.max(VIEWPORT_PADDING, viewport.height - height - VIEWPORT_PADDING)),
  };
}

function clampState() {
  const viewport = viewportSize();
  state.window.width = Math.min(
    Math.max(MIN_WIDTH, state.window.width || DEFAULT_WIDTH),
    Math.max(MIN_WIDTH, viewport.width - VIEWPORT_PADDING * 2),
  );
  if (state.window.height !== undefined) {
    state.window.height = Math.min(
      Math.max(MIN_HEIGHT, state.window.height),
      Math.max(MIN_HEIGHT, viewport.height - VIEWPORT_PADDING * 2),
    );
  }
  const windowWidth = scaledLayout.value ? scaledWidth.value : state.window.width;
  const windowHeight = scaledLayout.value
    ? scaledHeight.value
    : (windowRef.value?.getBoundingClientRect().height || state.window.height || Math.min(620, viewport.height - VIEWPORT_PADDING * 2));
  const windowPoint = clampPoint(state.window.x, state.window.y, windowWidth, windowHeight);
  state.window.x = windowPoint.x;
  state.window.y = windowPoint.y;
  const badgePoint = clampPoint(state.badge.x, state.badge.y, BADGE_SIZE, BADGE_SIZE);
  state.badge.x = badgePoint.x;
  state.badge.y = badgePoint.y;
}

const windowStyle = computed(() => {
  if (state.mode === 'minimized') {
    return {
      left: `${state.badge.x}px`,
      top: `${state.badge.y}px`,
    };
  }
  if (scaledLayout.value) {
    return {
      left: `${state.window.x}px`,
      top: `${state.window.y}px`,
      width: `${scaledWidth.value}px`,
      height: `${scaledHeight.value}px`,
    };
  }
  return {
    left: `${state.window.x}px`,
    top: `${state.window.y}px`,
    width: `${state.window.width}px`,
    height: state.window.height ? `${state.window.height}px` : undefined,
  };
});

const contentStyle = computed(() => {
  if (!scaledLayout.value) {
    return {
      width: '100%',
      height: '100%',
    };
  }
  return {
    width: `${DEFAULT_WIDTH}px`,
    transform: `scale(${panelScale.value})`,
  };
});

function setMode(mode: WindowMode) {
  state.mode = mode;
  if (mode === 'expanded') {
    nextTick(clampState);
  }
  persistState();
  emit('mode-change', mode);
}

function toggle() {
  setMode(state.mode === 'expanded' ? 'hidden' : 'expanded');
}

function hide() {
  setMode('hidden');
}

function minimize() {
  setMode('minimized');
}

function restore() {
  setMode('expanded');
}

function beginDrag(kind: 'window' | 'badge', event: PointerEvent) {
  if (event.pointerType === 'mouse' && event.button !== 0) return;
  const point = kind === 'window' ? state.window : state.badge;
  drag.value = {
    kind,
    pointerId: event.pointerId,
    startClientX: event.clientX,
    startClientY: event.clientY,
    startX: point.x,
    startY: point.y,
    moved: false,
  };
  windowRef.value?.setPointerCapture?.(event.pointerId);
  event.preventDefault();
}

function handlePointerDown(event: PointerEvent) {
  const target = event.target as HTMLElement | null;
  if (state.mode === 'minimized') {
    beginDrag('badge', event);
    return;
  }

  if (!scaledLayout.value && windowRef.value) {
    const rect = windowRef.value.getBoundingClientRect();
    if (event.clientX >= rect.right - 20 && event.clientY >= rect.bottom - 20) {
      resizing.value = true;
      resizePointerId.value = event.pointerId;
      return;
    }
  }

  if (!target?.closest('.dice-tray__header')) return;
  if (target.closest('button, input, textarea, select, [role="button"]')) return;
  beginDrag('window', event);
}

function handlePointerMove(event: PointerEvent) {
  const current = drag.value;
  if (!current || current.pointerId !== event.pointerId) return;
  const dx = event.clientX - current.startClientX;
  const dy = event.clientY - current.startClientY;
  if (!current.moved && Math.hypot(dx, dy) >= DRAG_THRESHOLD) {
    current.moved = true;
  }
  if (!current.moved) return;
  event.preventDefault();
  const rect = windowRef.value?.getBoundingClientRect();
  const width = current.kind === 'badge' ? BADGE_SIZE : (rect?.width || state.window.width);
  const height = current.kind === 'badge' ? BADGE_SIZE : (rect?.height || state.window.height || MIN_HEIGHT);
  const point = clampPoint(current.startX + dx, current.startY + dy, width, height);
  if (current.kind === 'badge') {
    state.badge.x = point.x;
    state.badge.y = point.y;
  } else {
    state.window.x = point.x;
    state.window.y = point.y;
  }
}

function finishPointer(event: PointerEvent) {
  if (resizePointerId.value === event.pointerId) {
    resizing.value = false;
    resizePointerId.value = null;
    if (observedSize.width > 0 && observedSize.height > 0) {
      state.window.width = observedSize.width;
      state.window.height = observedSize.height;
      clampState();
      persistState();
    }
  }

  const current = drag.value;
  if (!current || current.pointerId !== event.pointerId) return;
  windowRef.value?.releasePointerCapture?.(event.pointerId);
  drag.value = null;
  if (current.kind === 'badge' && !current.moved) {
    restore();
    return;
  }
  if (current.moved) persistState();
}

function handleViewportResize() {
  viewportWidth.value = Math.max(window.innerWidth, 1);
  viewportHeight.value = Math.max(window.innerHeight, 1);
  const nextMobile = viewportWidth.value < MOBILE_BREAKPOINT;
  if (nextMobile !== isMobile.value) {
    isMobile.value = nextMobile;
    return;
  }
  clampState();
  persistState();
}

function handleWindowBlur() {
  drag.value = null;
  resizing.value = false;
  resizePointerId.value = null;
}

watch(storageKey, loadState, { immediate: true });

watch(() => props.available, (available) => {
  if (!available) {
    drag.value = null;
    resizing.value = false;
    resizePointerId.value = null;
  }
});

onMounted(() => {
  viewportWidth.value = Math.max(window.innerWidth, 1);
  viewportHeight.value = Math.max(window.innerHeight, 1);
  isMobile.value = viewportWidth.value < MOBILE_BREAKPOINT;
  window.addEventListener('pointermove', handlePointerMove, { passive: false });
  window.addEventListener('pointerup', finishPointer);
  window.addEventListener('pointercancel', finishPointer);
  window.addEventListener('resize', handleViewportResize);
  window.addEventListener('blur', handleWindowBlur);
  resizeObserver = new ResizeObserver((entries) => {
    const rect = entries[0]?.contentRect;
    if (!rect || scaledLayout.value || state.mode !== 'expanded') return;
    observedSize = {
      width: Math.round(rect.width),
      height: Math.round(rect.height),
    };
  });
  if (windowRef.value) resizeObserver.observe(windowRef.value);
  contentResizeObserver = new ResizeObserver((entries) => {
    const element = entries[0]?.target as HTMLElement | undefined;
    if (!element || state.mode !== 'expanded') return;
    const nextHeight = Math.max(MIN_HEIGHT, Math.ceil(element.scrollHeight));
    if (nextHeight !== contentHeight.value) {
      contentHeight.value = nextHeight;
      nextTick(clampState);
    }
  });
  if (contentRef.value) contentResizeObserver.observe(contentRef.value);
  nextTick(clampState);
});

watch(windowRef, (element, previous) => {
  if (previous) resizeObserver?.unobserve(previous);
  if (element) resizeObserver?.observe(element);
});

watch(contentRef, (element, previous) => {
  if (previous) contentResizeObserver?.unobserve(previous);
  if (element) contentResizeObserver?.observe(element);
});

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', handlePointerMove);
  window.removeEventListener('pointerup', finishPointer);
  window.removeEventListener('pointercancel', finishPointer);
  window.removeEventListener('resize', handleViewportResize);
  window.removeEventListener('blur', handleWindowBlur);
  resizeObserver?.disconnect();
  contentResizeObserver?.disconnect();
});

defineExpose({ toggle, hide, minimize, restore });
</script>

<style scoped>
.dice-tray-floating-window {
  position: fixed;
  z-index: 1950;
  box-sizing: border-box;
  min-width: 320px;
  min-height: 280px;
  max-width: calc(100vw - 24px);
  max-height: calc(100dvh - 24px);
  overflow: auto;
  resize: both;
  border: 0;
  border-radius: 12px;
  background: transparent;
  box-shadow: 0 18px 42px rgba(15, 23, 42, 0.2);
}

.dice-tray-floating-window.is-mobile {
  min-width: 0;
  min-height: 0;
  max-height: calc(100dvh - env(safe-area-inset-top) - env(safe-area-inset-bottom) - 24px);
  resize: none;
}

.dice-tray-floating-window.is-scaled {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  resize: none;
}

.dice-tray-floating-window.is-minimized {
  width: 52px !important;
  height: 52px !important;
  min-width: 0;
  min-height: 0;
  overflow: visible;
  resize: none;
  border-radius: 12px;
  box-shadow: none;
}

.dice-tray-floating-window__badge {
  width: 52px;
  height: 52px;
  padding: 5px;
  border: 0;
  border-radius: 12px;
  background: var(--sc-bg-elevated, rgba(255, 255, 255, 0.96));
  color: var(--sc-accent, #2563eb);
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.22);
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.dice-tray-floating-window__badge:active {
  cursor: grabbing;
}

.dice-tray-floating-window__badge svg {
  width: 100%;
  height: 100%;
  fill: none;
  stroke: currentColor;
  stroke-width: 2.3;
  stroke-linecap: round;
  stroke-linejoin: round;
  pointer-events: none;
}

.dice-tray-floating-window__content {
  transform-origin: top left;
}

.dice-tray-floating-window :deep(.dice-tray) {
  width: 100%;
  height: 100%;
}

.dice-tray-floating-window.is-scaled :deep(.dice-tray) {
  width: 420px;
  height: auto;
}

.dice-tray-floating-window.is-scaled :deep(.dice-tray__body) {
  flex-direction: row;
  gap: 4px;
}

:global([data-display-palette='night']) .dice-tray-floating-window__badge {
  background: var(--sc-bg-elevated, rgba(30, 41, 59, 0.96));
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.42);
}
</style>
