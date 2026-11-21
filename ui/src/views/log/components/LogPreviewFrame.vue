<script setup lang="ts">
// 隔离预览宿主：拥有 iframe 与分页引擎的完整生命周期；每次输入变化产生新修订，旧结果丢弃。
import { onBeforeUnmount, onMounted, ref, shallowRef, toRaw, watch } from 'vue';
import {
  LogFrameEngine,
  type FrameAnchor,
  type FrameDocument,
  type FrameEvent,
  type FrameLayoutResult,
  type FramePrintRange,
  type FrameProofNode,
  type FrameProofView,
  type FrameRelocation,
} from '@/services/log/pagination/logFrameHost';
import type { PreparedLogFrame } from '../composables/useLogAssets';

const props = withDefaults(defineProps<{
  doc: FrameDocument | null;
  frameRevision?: number;
  assetTokens: string[];
  prepareAssets: (doc: FrameDocument, tokens: string[], hasAsset: (token: string) => boolean) => Promise<PreparedLogFrame>;
  mode?: 'paged' | 'flow';
  spread?: boolean;
  design?: boolean;
  canResizeImages?: boolean;
  selection?: string[];
  /** 校对审阅层：批注与审阅状态（编辑器辅助 UI，不进入排版结果）。 */
  proofMarks?: FrameProofNode[] | null;
  proofView?: FrameProofView | null;
  relocation?: FrameRelocation | null;
  /** 多选模式（仅设计态生效）。 */
  multi?: boolean;
  zoom?: number;
  title?: string;
  /** 工作区底色（纸张之外）；不传时使用 iframe 默认底色。 */
  canvas?: { color: string; dark: boolean; accent?: string } | null;
}>(), {
  mode: 'paged',
  frameRevision: 0,
  spread: false,
  design: false,
  canResizeImages: false,
  selection: () => [],
  proofMarks: null,
  proofView: null,
  relocation: null,
  multi: false,
  zoom: 1,
  title: 'LOG 预览',
  canvas: null,
});

const emit = defineEmits<{
  (e: 'frame-event', event: FrameEvent): void;
  (e: 'layout', result: FrameLayoutResult, frameRevision: number): void;
  (e: 'layout-error', message: string, frameRevision: number): void;
  (e: 'busy', busy: boolean): void;
  (e: 'near-end'): void;
}>();

const iframeRef = ref<HTMLIFrameElement | null>(null);
const engine = shallowRef<LogFrameEngine | null>(null);
const progress = ref<{ done: number; total: number } | null>(null);
const busy = ref(false);
let revision = 0;
let lastAnchor: FrameAnchor | null = null;

const onEvent = (event: FrameEvent) => {
  if (event.type === 'progress') {
    if (event.revision === revision) progress.value = { done: event.done, total: event.total };
    return;
  }
  if (event.type === 'anchor') {
    lastAnchor = event.anchor;
    if (event.nearEnd) emit('near-end');
  }
  if (event.type === 'context') {
    // iframe 视口坐标换算为外层页面坐标，供外层菜单定位。
    const rect = iframeRef.value?.getBoundingClientRect();
    const x = Number(event.x);
    const y = Number(event.y);
    if (!rect || !Number.isFinite(x) || !Number.isFinite(y)) return;
    emit('frame-event', { ...event, x: rect.left + Math.min(Math.max(x, 0), rect.width), y: rect.top + Math.min(Math.max(y, 0), rect.height) });
    return;
  }
  emit('frame-event', event);
};

const run = async () => {
  const e = engine.value;
  const doc = props.doc;
  const frameRevision = props.frameRevision;
  if (!e || !doc) return;
  const rev = ++revision;
  busy.value = true;
  emit('busy', true);
  progress.value = null;
  try {
    const prepared = await props.prepareAssets(doc, props.assetTokens, token => e.hasAsset(token));
    if (rev !== revision) return;
    await e.provideAssets(prepared.assets);
    if (rev !== revision) return;
    const result = await e.layout({
      doc: JSON.parse(JSON.stringify(prepared.doc)) as FrameDocument,
      mode: props.mode,
      spread: props.spread,
      design: props.design,
      canResizeImages: props.canResizeImages,
      selection: props.selection.slice(),
      relocation: props.design ? props.relocation : null,
      multi: props.design && props.multi,
      anchor: lastAnchor,
    }, rev);
    if (!result || rev !== revision) return;
    emit('layout', result, frameRevision);
  } catch (err) {
    if (rev === revision) emit('layout-error', err instanceof Error ? err.message : String(err), frameRevision);
  } finally {
    if (rev === revision) {
      busy.value = false;
      progress.value = null;
      emit('busy', false);
    }
  }
};

onMounted(() => {
  if (!iframeRef.value) return;
  engine.value = new LogFrameEngine(iframeRef.value, onEvent);
  void engine.value.ready.then(() => {
    engine.value?.zoom(props.zoom);
    engine.value?.proofView(toRaw(props.proofView) ?? null);
    engine.value?.proofMarks(toRaw(props.proofMarks) ?? null);
    if (props.canvas) engine.value?.canvas(props.canvas.color, props.canvas.dark, props.canvas.accent);
    void run();
  });
});

watch(() => [props.doc, props.frameRevision, props.mode, props.spread, props.design, props.canResizeImages] as const, () => {
  void run();
});
watch(() => props.selection, (ids) => engine.value?.select(ids.slice()), { deep: false });
watch(() => props.proofMarks, (nodes) => engine.value?.proofMarks(toRaw(nodes) ?? null));
watch(() => props.proofView, (view) => engine.value?.proofView(toRaw(view) ?? null));
watch(() => props.relocation, (state) => engine.value?.relocate(props.design ? state : null));
watch(() => props.multi, (active) => engine.value?.multi(props.design && active));
watch(() => props.zoom, (z) => engine.value?.zoom(z));
watch(() => props.canvas, (c) => engine.value?.canvas(c?.color ?? null, c?.dark ?? false, c?.accent));

onBeforeUnmount(() => {
  revision += 1;
  engine.value?.dispose();
  engine.value = null;
});

defineExpose({
  scrollTo: (nodeId: string, flash = true) => engine.value?.scrollTo(nodeId, flash),
  proofReveal: (key: string | null) => engine.value?.proofReveal(key),
  print: (range?: FramePrintRange) => engine.value?.print(range),
  rerun: () => run(),
});
</script>

<template>
  <div class="log-preview-frame">
    <iframe ref="iframeRef" class="log-preview-frame__iframe" :title="title" :style="canvas ? { background: canvas.color } : undefined" />
    <div v-if="busy" class="log-preview-frame__busy" role="status" aria-live="polite">
      <n-spin size="small" />
      <span>{{ progress ? `排版中 ${progress.done}/${progress.total}` : '排版中…' }}</span>
    </div>
  </div>
</template>

<style scoped>
.log-preview-frame {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 0;
}

.log-preview-frame__iframe {
  display: block;
  width: 100%;
  height: 100%;
  border: 0;
  background: #e9e7e2;
}

.log-preview-frame__busy {
  position: absolute;
  top: 10px;
  right: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.15);
  font-size: 12px;
  color: #444;
  pointer-events: none;
}
</style>
