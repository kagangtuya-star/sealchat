<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { Minus } from '@vicons/tabler';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { resolveTemplateValue } from '@/utils/characterCardTemplate';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import {
  useChannelCharacterSnapshotStore,
  type CharacterSnapshotNumericSource,
  type ChannelCharacterSnapshotItem,
  type TheaterCharacterStatTemplate,
} from '@/stores/channelCharacterSnapshot';

const props = defineProps<{
  worldId: string;
  channelId: string;
}>();

const emit = defineEmits<{
  openCharacterCard: [identityId: string];
}>();

interface OverlayLayout {
  x: number;
  y: number;
  width: number;
  height: number;
}

type MinimizedEdge = 'left' | 'right' | 'top' | 'bottom';

interface ResolvedStat {
  id: string;
  name: string;
  current: number;
  max: number | null;
  min: number | null;
  barColor: string;
  textColor: string;
  fillLeft: number;
  fillWidth: number;
  zeroLeft: number;
}

interface ResolvedCharacter {
  item: ChannelCharacterSnapshotItem;
  name: string;
  avatarUrl: string;
  preferredColumns: number;
  stats: ResolvedStat[];
}

const rootRef = ref<HTMLElement | null>(null);
const chatStore = useChatStore();
const userStore = useUserStore();
const snapshotStore = useChannelCharacterSnapshotStore();
const layout = reactive<OverlayLayout>({ x: 12, y: 54, width: 300, height: 280 });
const controlsVisible = ref(false);
const minimized = ref(false);
const minimizedEdge = ref<MinimizedEdge>('left');
const slotOrderByUser = ref<Record<string, number>>({});
const stableCharactersByUser = ref<Record<string, ResolvedCharacter>>({});
const overlayReady = ref(false);
let nextSlotOrder = 0;
let resizeObserver: ResizeObserver | null = null;
let interaction: {
  mode: 'drag' | 'resize';
  pointerId: number;
  startX: number;
  startY: number;
  layout: OverlayLayout;
} | null = null;

const viewportClass = () => {
  const width = rootRef.value?.parentElement?.clientWidth || window.innerWidth;
  if (width < 640) return 'narrow';
  if (width < 1100) return 'medium';
  return 'wide';
};

const storageKey = () => [
  'sealchat',
  'theater-character-overlay',
  'v2',
  String(userStore.info?.id || 'guest'),
  props.worldId,
  props.channelId,
  viewportClass(),
].join(':');

const slotStorageKey = () => [
  'sealchat',
  'theater-character-overlay-slots',
  'v1',
  String(userStore.info?.id || 'guest'),
  props.worldId,
  props.channelId,
].join(':');

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

const clampLayout = (candidate: OverlayLayout): OverlayLayout => {
  const parent = rootRef.value?.parentElement;
  const viewportWidth = Math.max(1, parent?.clientWidth || window.innerWidth || 960);
  const viewportHeight = Math.max(1, parent?.clientHeight || window.innerHeight || 640);
  const minimumWidth = Math.min(240, viewportWidth);
  const minimumHeight = Math.min(180, viewportHeight);
  const width = clamp(Number(candidate.width) || 300, minimumWidth, viewportWidth);
  const height = clamp(Number(candidate.height) || 280, minimumHeight, viewportHeight);
  return {
    x: clamp(Number(candidate.x) || 0, 0, Math.max(0, viewportWidth - width)),
    y: clamp(Number(candidate.y) || 0, 0, Math.max(0, viewportHeight - height)),
    width,
    height,
  };
};

const setLayout = (candidate: OverlayLayout) => Object.assign(layout, clampLayout(candidate));

const defaultLayout = (): OverlayLayout => {
  const parent = rootRef.value?.parentElement;
  const viewportWidth = parent?.clientWidth || window.innerWidth || 960;
  const viewportHeight = parent?.clientHeight || window.innerHeight || 640;
  return clampLayout({
    x: 12,
    y: 54,
    width: Math.min(340, Math.max(280, viewportWidth * 0.31)),
    height: Math.min(360, Math.max(220, viewportHeight * 0.4)),
  });
};

const persistLayout = () => {
  const parent = rootRef.value?.parentElement;
  const viewportWidth = Math.max(1, parent?.clientWidth || window.innerWidth || 1);
  const viewportHeight = Math.max(1, parent?.clientHeight || window.innerHeight || 1);
  try {
    localStorage.setItem(storageKey(), JSON.stringify({
      x: layout.x / viewportWidth,
      y: layout.y / viewportHeight,
      width: layout.width / viewportWidth,
      height: layout.height / viewportHeight,
      minimized: minimized.value,
      minimizedEdge: minimizedEdge.value,
    }));
  } catch {
    // Layout remains available for current session.
  }
};

const restoreLayout = () => {
  const parent = rootRef.value?.parentElement;
  const viewportWidth = Math.max(1, parent?.clientWidth || window.innerWidth || 1);
  const viewportHeight = Math.max(1, parent?.clientHeight || window.innerHeight || 1);
  try {
    const value = JSON.parse(localStorage.getItem(storageKey()) || 'null');
    if (value && [value.x, value.y, value.width, value.height].every(Number.isFinite)) {
      setLayout({
        x: value.x * viewportWidth,
        y: value.y * viewportHeight,
        width: value.width * viewportWidth,
        height: value.height * viewportHeight,
      });
      minimized.value = value.minimized === true;
      if (['left', 'right', 'top', 'bottom'].includes(value.minimizedEdge)) {
        minimizedEdge.value = value.minimizedEdge;
      }
      return;
    }
  } catch {
    // Invalid saved layout falls back to default.
  }
  minimized.value = false;
  minimizedEdge.value = 'left';
  setLayout(defaultLayout());
};

const restoreSlotOrder = () => {
  try {
    const value = JSON.parse(localStorage.getItem(slotStorageKey()) || 'null');
    const order = value?.order && typeof value.order === 'object' ? value.order : {};
    const next: Record<string, number> = {};
    Object.entries(order).forEach(([userId, slot]) => {
      if (userId && Number.isFinite(slot) && Number(slot) >= 0) next[userId] = Number(slot);
    });
    slotOrderByUser.value = next;
    nextSlotOrder = Math.max(0, ...Object.values(next).map(slot => slot + 1));
  } catch {
    slotOrderByUser.value = {};
    nextSlotOrder = 0;
  }
};

const persistSlotOrder = () => {
  try {
    localStorage.setItem(slotStorageKey(), JSON.stringify({ order: slotOrderByUser.value }));
  } catch {
    // Slot order remains stable for current session.
  }
};

const ensureSlotOrder = (userId: string) => {
  const current = slotOrderByUser.value[userId];
  if (Number.isFinite(current)) return current;
  const order = nextSlotOrder;
  nextSlotOrder += 1;
  slotOrderByUser.value = { ...slotOrderByUser.value, [userId]: order };
  persistSlotOrder();
  return order;
};

const showControls = () => {
  controlsVisible.value = true;
};

const hideControls = () => {
  if (!interaction) controlsVisible.value = false;
};

const minimize = () => {
  const parent = rootRef.value?.parentElement;
  const viewportWidth = Math.max(1, parent?.clientWidth || window.innerWidth || 1);
  const viewportHeight = Math.max(1, parent?.clientHeight || window.innerHeight || 1);
  const distances: Array<[MinimizedEdge, number]> = [
    ['left', layout.x],
    ['right', viewportWidth - (layout.x + layout.width)],
    ['top', layout.y],
    ['bottom', viewportHeight - (layout.y + layout.height)],
  ];
  minimizedEdge.value = distances.reduce((closest, edge) => edge[1] < closest[1] ? edge : closest)[0];
  minimized.value = true;
  controlsVisible.value = false;
  persistLayout();
};

const restore = () => {
  minimized.value = false;
  controlsVisible.value = true;
  persistLayout();
};

const beginInteraction = (mode: 'drag' | 'resize', event: PointerEvent) => {
  if (event.button !== 0) return;
  interaction = {
    mode,
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    layout: { ...layout },
  };
  showControls();
  (event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId);
  event.preventDefault();
};

const moveInteraction = (event: PointerEvent) => {
  if (!interaction || interaction.pointerId !== event.pointerId) return;
  const dx = event.clientX - interaction.startX;
  const dy = event.clientY - interaction.startY;
  if (interaction.mode === 'drag') {
    setLayout({ ...interaction.layout, x: interaction.layout.x + dx, y: interaction.layout.y + dy });
  } else {
    setLayout({ ...interaction.layout, width: interaction.layout.width + dx, height: interaction.layout.height + dy });
  }
};

const endInteraction = (event: PointerEvent) => {
  if (!interaction || interaction.pointerId !== event.pointerId) return;
  interaction = null;
  (event.currentTarget as HTMLElement).releasePointerCapture?.(event.pointerId);
  persistLayout();
};

const resolveNumericSource = (source: CharacterSnapshotNumericSource | undefined, attrs: Record<string, any>): number | null => {
  if (!source) return null;
  const raw = 'value' in source ? source.value : resolveTemplateValue(attrs, source.path);
  if (raw === null || raw === undefined || raw === '') return null;
  const value = typeof raw === 'number' ? raw : Number(String(raw).trim());
  return Number.isFinite(value) ? value : null;
};

const resolveStat = (template: TheaterCharacterStatTemplate, attrs: Record<string, any>): ResolvedStat | null => {
  const current = resolveNumericSource(template.current, attrs);
  const max = resolveNumericSource(template.max, attrs);
  const min = resolveNumericSource(template.min, attrs);
  if (current === null) return null;
  if (max === null) {
    return {
      id: template.id,
      name: template.name,
      current,
      max: null,
      min,
      barColor: template.barColor || '#ffffff',
      textColor: template.textColor || '#ffffff',
      fillLeft: 0,
      fillWidth: 100,
      zeroLeft: 0,
    };
  }
  const rangeMin = min ?? 0;
  if (max <= rangeMin) {
    return {
      id: template.id,
      name: template.name,
      current,
      max: null,
      min,
      barColor: template.barColor || '#ffffff',
      textColor: template.textColor || '#ffffff',
      fillLeft: 0,
      fillWidth: 100,
      zeroLeft: 0,
    };
  }
  const range = max - rangeMin;
  const currentRatio = clamp((current - rangeMin) / range, 0, 1);
  const zeroRatio = clamp((0 - rangeMin) / range, 0, 1);
  return {
    id: template.id,
    name: template.name,
    current,
    max,
    min,
    barColor: template.barColor || '#ffffff',
    textColor: template.textColor || '#ffffff',
    fillLeft: Math.min(currentRatio, zeroRatio) * 100,
    fillWidth: Math.abs(currentRatio - zeroRatio) * 100,
    zeroLeft: zeroRatio * 100,
  };
};

const snapshotItems = computed(() => snapshotStore.getChannelItems(props.channelId));

const resolveCharacter = (item: ChannelCharacterSnapshotItem): ResolvedCharacter | null => {
  const template = snapshotStore.getOverlayTemplateForSnapshot(item);
  const attrs = item.data.card?.attrs || {};
  if (!template || !item.data.card) return null;
  const stats = template.items.map(stat => resolveStat(stat, attrs)).filter((stat): stat is ResolvedStat => !!stat);
  if (!stats.length) return null;
  return {
    item,
    name: item.data.identity.displayName || item.data.card.name || item.identityId,
    avatarUrl: resolveAttachmentUrl(item.data.card?.avatarAttachmentId || item.data.identity.avatarAttachmentId || ''),
    preferredColumns: clamp(Math.round(template.preferredColumns || 2), 1, 4),
    stats,
  };
};

const resolvedCharacters = computed(() => snapshotItems.value
  .map(resolveCharacter)
  .filter((item): item is ResolvedCharacter => !!item));

const reconcileCharacters = () => {
  if (!overlayReady.value) return;
  const activeUserIds = new Set(snapshotItems.value.map(item => String(item.userId || '').trim()).filter(Boolean));
  const next = { ...stableCharactersByUser.value };
  const latestByUser = new Map<string, ResolvedCharacter>();
  resolvedCharacters.value.forEach((character) => {
    const userId = String(character.item.userId || '').trim();
    const current = latestByUser.get(userId);
    if (userId && (!current || current.item.serverRevision <= character.item.serverRevision)) {
      latestByUser.set(userId, character);
    }
  });
  latestByUser.forEach((character, userId) => {
    ensureSlotOrder(userId);
    next[userId] = character;
  });
  Object.keys(next).forEach((userId) => {
    if (!activeUserIds.has(userId)) delete next[userId];
  });
  stableCharactersByUser.value = next;
};

const characters = computed<ResolvedCharacter[]>(() => Object.values(stableCharactersByUser.value)
  .sort((a, b) => {
    const aAdmin = chatStore.isChannelAdmin(props.channelId, a.item.userId) ? 0 : 1;
    const bAdmin = chatStore.isChannelAdmin(props.channelId, b.item.userId) ? 0 : 1;
    if (aAdmin !== bAdmin) return aAdmin - bAdmin;
    return (slotOrderByUser.value[a.item.userId] ?? Number.MAX_SAFE_INTEGER)
      - (slotOrderByUser.value[b.item.userId] ?? Number.MAX_SAFE_INTEGER);
  }));

const columnCount = computed(() => {
  if (layout.width < 440) return 1;
  if (layout.width < 760) return 2;
  if (layout.width < 1080) return 3;
  return 4;
});

const minimizedLayout = (): OverlayLayout => {
  const parent = rootRef.value?.parentElement;
  const viewportWidth = Math.max(1, parent?.clientWidth || window.innerWidth || 1);
  const viewportHeight = Math.max(1, parent?.clientHeight || window.innerHeight || 1);
  const tabWidth = 16;
  const tabLength = 54;
  if (minimizedEdge.value === 'right') return { x: viewportWidth - tabWidth, y: clamp(layout.y, 0, Math.max(0, viewportHeight - tabLength)), width: tabWidth, height: tabLength };
  if (minimizedEdge.value === 'top') return { x: clamp(layout.x, 0, Math.max(0, viewportWidth - tabLength)), y: 0, width: tabLength, height: tabWidth };
  if (minimizedEdge.value === 'bottom') return { x: clamp(layout.x, 0, Math.max(0, viewportWidth - tabLength)), y: viewportHeight - tabWidth, width: tabLength, height: tabWidth };
  return { x: 0, y: clamp(layout.y, 0, Math.max(0, viewportHeight - tabLength)), width: tabWidth, height: tabLength };
};

const panelStyle = computed(() => {
  const panel = minimized.value ? minimizedLayout() : layout;
  return {
    transform: `translate3d(${panel.x}px, ${panel.y}px, 0)`,
    width: `${panel.width}px`,
    height: `${panel.height}px`,
    '--theater-character-columns': String(columnCount.value),
  };
});

const onViewportResize = () => {
  setLayout({ ...layout });
  persistLayout();
};

watch(() => [props.worldId, props.channelId], async () => {
  overlayReady.value = false;
  stableCharactersByUser.value = {};
  restoreSlotOrder();
  await snapshotStore.initializeChannel(props.channelId);
  try {
    await chatStore.ensureChannelPermissionCache(props.channelId);
  } catch (error) {
    console.warn('[TheaterCharacterOverlay] Failed to load channel admin map', error);
  }
  await nextTick();
  restoreLayout();
  overlayReady.value = true;
  reconcileCharacters();
}, { immediate: true });

watch([snapshotItems, resolvedCharacters], reconcileCharacters, { immediate: true });

onMounted(() => {
  resizeObserver = new ResizeObserver(onViewportResize);
  if (rootRef.value?.parentElement) resizeObserver.observe(rootRef.value.parentElement);
  restoreLayout();
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
});
</script>

<template>
  <div ref="rootRef" class="theater-character-overlay-root">
    <section
      v-if="characters.length"
      class="theater-character-overlay"
      :class="[
        { 'is-chrome-visible': controlsVisible && !minimized, 'is-minimized': minimized },
        minimized ? `is-minimized-${minimizedEdge}` : '',
      ]"
      :style="panelStyle"
      @pointerdown.capture="showControls"
      @pointerleave="hideControls"
    >
      <button
        v-if="minimized"
        type="button"
        class="theater-character-overlay__minimized-tab"
        title="恢复人物数据浮层"
        aria-label="恢复人物数据浮层"
        @click.stop="restore"
      />
      <template v-else>
        <header
          class="theater-character-overlay__handle"
          title="拖动人物数据浮层"
          @pointerdown="beginInteraction('drag', $event)"
          @pointermove="moveInteraction"
          @pointerup="endInteraction"
          @pointercancel="endInteraction"
        >
          <span>人物数据</span>
          <span>{{ characters.length }}</span>
        </header>
        <div class="theater-character-overlay__content">
          <article v-for="character in characters" :key="character.item.userId" class="theater-character-stat-card">
            <button
              type="button"
              class="theater-character-stat-card__avatar"
              :title="`打开 ${character.name} 的人物卡`"
              @click="emit('openCharacterCard', character.item.identityId)"
            >
              <img v-if="character.avatarUrl" :src="character.avatarUrl" :alt="character.name">
              <span v-else>{{ character.name.slice(0, 1) || '?' }}</span>
            </button>
            <div class="theater-character-stat-card__stats">
              <div v-for="stat in character.stats" :key="stat.id" class="theater-character-stat">
                <div class="theater-character-stat__bar" :style="{ color: stat.textColor }">
                  <span
                    class="theater-character-stat__fill"
                    :style="{ left: `${stat.fillLeft}%`, width: `${stat.fillWidth}%`, backgroundColor: stat.barColor }"
                  />
                  <span v-if="stat.min !== null && stat.min < 0 && stat.max !== null && stat.max > 0" class="theater-character-stat__zero" :style="{ left: `${stat.zeroLeft}%` }" />
                  <span class="theater-character-stat__name">{{ stat.name }}</span>
                  <span class="theater-character-stat__value">{{ stat.max === null ? stat.current : `${stat.current}/${stat.max}` }}</span>
                </div>
              </div>
            </div>
          </article>
        </div>
        <button
          type="button"
          class="theater-character-overlay__minimize"
          title="最小化到屏幕边缘"
          aria-label="最小化到屏幕边缘"
          @click.stop="minimize"
        ><n-icon><Minus /></n-icon></button>
        <button
          type="button"
          class="theater-character-overlay__resize"
          aria-label="调整人物数据浮层大小"
          @pointerdown="beginInteraction('resize', $event)"
          @pointermove="moveInteraction"
          @pointerup="endInteraction"
          @pointercancel="endInteraction"
        />
      </template>
    </section>
  </div>
</template>

<style scoped>
.theater-character-overlay-root { position: absolute; inset: 0; z-index: 9500; pointer-events: none; overflow: hidden; }
.theater-character-overlay { position: absolute; left: 0; top: 0; display: flex; flex-direction: column; min-width: 240px; min-height: 180px; overflow: hidden; pointer-events: auto; color: #fff; background: transparent; outline: 1px solid transparent; outline-offset: -1px; transition: outline-color .14s ease; }
.theater-character-overlay.is-chrome-visible, .theater-character-overlay:focus-within { outline-color: rgba(255, 255, 255, .34); }
.theater-character-overlay__handle { flex: 0 0 20px; display: flex; align-items: center; justify-content: space-between; padding: 0 8px; color: transparent; background: transparent; font-size: 10px; letter-spacing: .06em; cursor: move; touch-action: none; user-select: none; transition: color .14s ease, background-color .14s ease; }
.is-chrome-visible .theater-character-overlay__handle, .theater-character-overlay:focus-within .theater-character-overlay__handle { color: rgba(255, 255, 255, .8); background: rgba(12, 14, 18, .42); }
.theater-character-overlay__content { display: grid; align-content: start; grid-template-columns: repeat(var(--theater-character-columns), minmax(0, 1fr)); gap: 9px; min-height: 0; padding: 4px 0 0; overflow: auto; }
.theater-character-stat-card { display: grid; grid-template-columns: 54px minmax(0, 1fr); min-width: 0; min-height: 54px; gap: 8px; overflow: hidden; background: transparent; }
.theater-character-stat-card__avatar { align-self: center; width: 54px; height: 54px; padding: 0; overflow: hidden; color: #fff; background: rgba(255, 255, 255, .08); border: 0; border-radius: 3px; cursor: pointer; }
.theater-character-stat-card__avatar img { width: 100%; height: 100%; display: block; object-fit: cover; }
.theater-character-stat-card__avatar span { width: 100%; height: 100%; display: grid; place-items: center; font-size: 24px; }
.theater-character-stat-card__stats { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-content: start; min-width: 0; gap: 3px 4px; padding: 2px 6px 2px 0; }
.theater-character-stat { min-width: 0; }
.theater-character-stat__name, .theater-character-stat__value { position: absolute; z-index: 1; display: grid; align-items: center; overflow: hidden; font-size: 10px; font-variant-numeric: tabular-nums; font-weight: 650; line-height: 13px; text-shadow: 0 1px 2px #000; white-space: nowrap; }
.theater-character-stat__name { inset: 0 auto 0 4px; max-width: calc(100% - 38px); text-overflow: ellipsis; }
.theater-character-stat__bar { position: relative; height: 16px; overflow: hidden; border: 1px solid transparent; border-radius: 2px; background: rgba(0, 0, 0, .26); }
.theater-character-stat__fill { position: absolute; top: 0; bottom: 0; opacity: .46; }
.theater-character-stat__zero { position: absolute; top: 0; bottom: 0; width: 1px; background: currentColor; opacity: .75; }
.theater-character-stat__value { inset: 0 4px 0 auto; place-items: center end; }
.theater-character-overlay__minimize, .theater-character-overlay__resize { position: absolute; padding: 0; border: 0; opacity: 0; pointer-events: none; transition: opacity .14s ease, background-color .14s ease; }
.is-chrome-visible .theater-character-overlay__minimize, .is-chrome-visible .theater-character-overlay__resize, .theater-character-overlay:focus-within .theater-character-overlay__minimize, .theater-character-overlay:focus-within .theater-character-overlay__resize { opacity: 1; pointer-events: auto; }
.theater-character-overlay__minimize { top: 2px; right: 4px; display: grid; width: 18px; height: 16px; place-items: center; color: rgba(255, 255, 255, .76); background: rgba(255, 255, 255, .09); border-radius: 2px; cursor: pointer; }
.theater-character-overlay__minimize:hover, .theater-character-overlay__minimize:focus-visible { color: #fff; background: rgba(255, 255, 255, .18); outline: none; }
.theater-character-overlay__resize { right: 0; bottom: 0; width: 20px; height: 20px; background: linear-gradient(135deg, transparent 0 48%, rgba(255, 255, 255, .68) 49% 55%, transparent 56% 66%, rgba(255, 255, 255, .4) 67% 73%, transparent 74%); cursor: nwse-resize; touch-action: none; }
.theater-character-overlay.is-minimized { min-width: 0; min-height: 0; overflow: visible; outline: 0; }
.theater-character-overlay__minimized-tab { width: 100%; height: 100%; padding: 0; border: 0; background: transparent; cursor: pointer; }
.theater-character-overlay__minimized-tab:hover, .theater-character-overlay__minimized-tab:focus-visible { background: rgba(255, 255, 255, .2); outline: 1px solid rgba(255, 255, 255, .42); outline-offset: -1px; }
.is-minimized-left .theater-character-overlay__minimized-tab, .is-minimized-right .theater-character-overlay__minimized-tab { border-radius: 0 3px 3px 0; }
.is-minimized-right .theater-character-overlay__minimized-tab { border-radius: 3px 0 0 3px; }
.is-minimized-top .theater-character-overlay__minimized-tab { border-radius: 0 0 3px 3px; }
.is-minimized-bottom .theater-character-overlay__minimized-tab { border-radius: 3px 3px 0 0; }
@media (max-width: 640px) {
  .theater-character-overlay__content { gap: 5px; padding: 5px; }
  .theater-character-stat-card { grid-template-columns: 52px minmax(0, 1fr); }
}
</style>
