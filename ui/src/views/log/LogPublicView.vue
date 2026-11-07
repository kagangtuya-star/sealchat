<script setup lang="ts">
// 匿名展示页：只调用 /public/log 接口（无凭据客户端），不加载编辑器、聊天连接或管理 DTO。
// privacyVersion 或 releaseId 变化、链接失效时清空旧内容重新加载，不继续展示被下架的缓存。
// 阅读单位是视觉章节：章节模式发布版按章节读取（可跨多个来源片段），旧发布版每个 section 即一章。
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  describeLogError,
  fetchPublicLog,
  fetchPublicLogChapterContext,
  fetchPublicLogChapterItems,
  fetchPublicLogContext,
  fetchPublicLogItems,
  isLogError,
  searchPublicLog,
} from '@/services/log/logApi';
import { buildFrameDocument, publicDocumentStyle, publicNodeToRender, type RenderNode, type TocEntry } from '@/services/log/logRenderer';
import type { FrameDocument, FrameEvent, FrameLayoutResult, FramePrintRange } from '@/services/log/pagination/logFrameHost';
import type { LogPublicDocument, LogPublicNode, LogPublicSearchHit } from '@/types/log';
import { useLogAssets } from './composables/useLogAssets';
import LogPreviewFrame from './components/LogPreviewFrame.vue';
import LogPrintDialog from './components/LogPrintDialog.vue';
import type { GlobalThemeOverrides } from 'naive-ui';

const PAGE = 100;
const AUTO_CAP = 1000;
const LONG_PRINT_PAGES = 80;

const route = useRoute();
const router = useRouter();
const slug = computed(() => String(route.params.slug || ''));
const state = ref<'loading' | 'ready' | 'missing' | 'error'>('loading');
const errorText = ref('');
const doc = shallowRef<LogPublicDocument | null>(null);
const unitId = ref(''); // 当前阅读单位（章节模式为章节 id，旧发布版为 section id）
const items = shallowRef<LogPublicNode[]>([]);
const nextCursor = ref<string | null>(null);
const partial = ref(false); // 通过 context 接口定位得到的局部窗口
const loadingItems = ref(false);
const itemsError = ref('');
const mode = ref<'flow' | 'paged'>('flow');
const assets = useLogAssets({ publicSlug: () => slug.value });
let generation = 0;
let controller: AbortController | null = null;

interface ReadingUnit {
  id: string;
  title: string;
  sectionIds: string[];
}
const chapterMode = computed(() => !!doc.value?.chapters?.length);
const units = computed<ReadingUnit[]>(() => {
  const d = doc.value;
  if (!d) return [];
  if (d.chapters?.length) return d.chapters.map((c) => ({ id: c.id, title: c.title, sectionIds: c.sectionIds }));
  return d.sections.map((s) => ({ id: s.id, title: s.title, sectionIds: [s.id] }));
});
/** 由片段 id（旧链接、搜索结果）找到所属阅读单位。 */
const unitOfSection = (id: string) => units.value.find((u) => u.sectionIds.includes(id)) || null;
const sectionTitle = computed(() => units.value.find((u) => u.id === unitId.value)?.title || '');
const fetchUnitItems = (s: string, unit: string, cursor: string | null, signal: AbortSignal) =>
  (chapterMode.value ? fetchPublicLogChapterItems : fetchPublicLogItems)(s, unit, cursor, PAGE, signal);
const fetchUnitContext = (s: string, unit: string, node: string) =>
  (chapterMode.value ? fetchPublicLogChapterContext : fetchPublicLogContext)(s, unit, node);
const unitQuery = (unit: string, node?: string) => ({ [chapterMode.value ? 'c' : 's']: unit, ...(node ? { n: node } : {}) });
const complete = computed(() => !partial.value && nextCursor.value === null && !loadingItems.value && !itemsError.value);

const resetItems = () => {
  generation += 1;
  controller?.abort();
  controller = null;
  items.value = [];
  nextCursor.value = null;
  partial.value = false;
  itemsError.value = '';
};

/** 发布版或隐私版本变化：整页重载。 */
const onStale = () => {
  resetItems();
  doc.value = null;
  void loadDocument();
};

const checkVersion = (releaseId: string, privacyVersion: number) => {
  const d = doc.value;
  if (!d) return false;
  if (d.releaseId !== releaseId || d.privacyVersion !== privacyVersion) {
    onStale();
    return false;
  }
  return true;
};

const loadDocument = async () => {
  const s = slug.value;
  if (!/^[a-z0-9]{4,64}$/.test(s)) {
    state.value = 'missing';
    return;
  }
  state.value = 'loading';
  resetItems();
  try {
    const d = await fetchPublicLog(s);
    if (s !== slug.value) return;
    doc.value = d;
    document.title = d.title || 'LOG';
    state.value = 'ready';
    const wantedChapter = String(route.query.c || '');
    const wantedSection = String(route.query.s || '');
    const wanted = units.value.find((u) => u.id === wantedChapter) || (wantedSection ? unitOfSection(wantedSection) : null);
    unitId.value = wanted?.id || units.value[0]?.id || '';
    const node = String(route.query.n || '');
    if (node && unitId.value) await locateNode(unitId.value, node);
    else await loadItems();
  } catch (error) {
    doc.value = null;
    if (isLogError(error, 'NOT_FOUND')) state.value = 'missing';
    else {
      state.value = 'error';
      errorText.value = describeLogError(error, '加载失败');
    }
  }
};

const loadItems = async (more = false) => {
  const s = slug.value;
  const sec = unitId.value;
  if (!sec || !doc.value) return;
  if (!more) resetItems();
  const gen = generation;
  const ctrl = new AbortController();
  controller = ctrl;
  loadingItems.value = true;
  let budget = AUTO_CAP;
  try {
    let cursor = more ? nextCursor.value : null;
    let acc = more ? items.value.slice() : [];
    do {
      const r = await fetchUnitItems(s, sec, cursor, ctrl.signal);
      if (gen !== generation) return;
      if (!checkVersion(r.releaseId, r.privacyVersion)) return;
      acc = acc.concat(r.items);
      cursor = r.nextCursor || null;
      budget -= r.items.length;
      items.value = acc;
      nextCursor.value = cursor;
    } while (cursor && budget > 0);
  } catch (error) {
    if (gen !== generation || ctrl.signal.aborted) return;
    if (isLogError(error, 'NOT_FOUND', 'SOURCE_CHANGED')) {
      onStale();
      return;
    }
    itemsError.value = describeLogError(error, '加载失败');
  } finally {
    if (gen === generation) loadingItems.value = false;
  }
};

const loadAllItemsForPrint = async () => {
  const s = slug.value;
  const sec = unitId.value;
  if (!sec || !doc.value) throw new Error('当前章节不可用');
  const gen = generation;
  const ctrl = new AbortController();
  controller = ctrl;
  loadingItems.value = true;
  let cursor = partial.value ? null : nextCursor.value;
  let acc = partial.value ? [] : items.value.slice();
  try {
    while (cursor || !acc.length) {
      const r = await fetchUnitItems(s, sec, cursor, ctrl.signal);
      if (gen !== generation) throw new Error('打印准备已取消');
      if (!checkVersion(r.releaseId, r.privacyVersion)) throw new Error('LOG 已更新，请稍后重试');
      acc = acc.concat(r.items);
      cursor = r.nextCursor || null;
      if (!cursor) break;
    }
    items.value = acc;
    nextCursor.value = null;
    partial.value = false;
    itemsError.value = '';
  } catch (error) {
    if (gen !== generation || ctrl.signal.aborted) throw new Error('打印准备已取消');
    const message = describeLogError(error, '加载完整章节失败');
    itemsError.value = message;
    throw new Error(message);
  } finally {
    if (gen === generation) loadingItems.value = false;
  }
};

const pendingScroll = ref<string | null>(null);
const locateNode = async (sec: string, nodeId: string) => {
  if (sec !== unitId.value) unitId.value = sec;
  if (items.value.some((n) => n.id === nodeId)) {
    previewRef.value?.scrollTo(nodeId, true);
    return;
  }
  resetItems();
  const gen = generation;
  loadingItems.value = true;
  try {
    const r = await fetchUnitContext(slug.value, sec, nodeId);
    if (gen !== generation) return;
    if (!checkVersion(r.releaseId, r.privacyVersion)) return;
    items.value = r.items;
    partial.value = true;
    pendingScroll.value = nodeId;
  } catch (error) {
    if (gen !== generation) return;
    if (isLogError(error, 'NOT_FOUND')) {
      itemsError.value = '链接指向的位置已不存在，已显示章节开头';
      await loadItems();
      return;
    }
    itemsError.value = describeLogError(error, '定位失败');
  } finally {
    if (gen === generation) loadingItems.value = false;
  }
};

const switchSection = (id: string) => {
  if (id === unitId.value && !partial.value) return;
  unitId.value = id;
  void router.replace({ query: unitQuery(id) });
  void loadItems();
};

// ---- 渲染 ----
const frame = shallowRef<{ doc: FrameDocument; assetTokens: string[] } | null>(null);
let buildTimer: ReturnType<typeof setTimeout> | null = null;
const rebuild = () => {
  const d = doc.value;
  if (!d) {
    frame.value = null;
    return;
  }
  const nodes: RenderNode[] = items.value.map(publicNodeToRender);
  const toc: TocEntry[] = [];
  if (nodes[0]) toc.push({ targetId: nodes[0].id, text: sectionTitle.value, level: 1 });
  for (const n of nodes) {
    if (n.kind === 'element' && n.type === 'heading') toc.push({ targetId: n.id, text: n.fields.text || '', level: n.level || 1 });
  }
  const bg = typeof d.page.backgroundAssetId === 'string' && /^[a-f0-9]{32}$/.test(d.page.backgroundAssetId) ? `pub:${d.page.backgroundAssetId}` : null;
  const r = buildFrameDocument({ style: publicDocumentStyle(d), page: d.page, title: d.title, sectionTitle: sectionTitle.value, nodes, toc, backgroundToken: bg, templateSelection: { displayRules: d.displayRules } });
  frame.value = { doc: r.doc, assetTokens: r.assetTokens };
};
watch([items, doc, sectionTitle], () => {
  if (buildTimer) clearTimeout(buildTimer);
  buildTimer = setTimeout(() => {
    buildTimer = null;
    rebuild();
  }, 60);
});

const previewRef = ref<InstanceType<typeof LogPreviewFrame> | null>(null);
const layout = shallowRef<FrameLayoutResult | null>(null);
const currentNode = ref<string | null>(null);
let printLayoutResolve: ((result: FrameLayoutResult) => void) | null = null;
let printLayoutReject: ((error: Error) => void) | null = null;
const waitForPrintLayout = () => new Promise<FrameLayoutResult>((resolve, reject) => {
  printLayoutResolve = resolve;
  printLayoutReject = reject;
});
const clearPrintLayoutWaiter = () => {
  printLayoutResolve = null;
  printLayoutReject = null;
};
const onLayout = (r: FrameLayoutResult) => {
  layout.value = r;
  if (printLayoutResolve) {
    const resolve = printLayoutResolve;
    clearPrintLayoutWaiter();
    resolve(r);
  }
  if (pendingScroll.value) {
    previewRef.value?.scrollTo(pendingScroll.value, true);
    pendingScroll.value = null;
  }
};
const onLayoutError = (message: string) => {
  if (!printLayoutReject) return;
  const reject = printLayoutReject;
  clearPrintLayoutWaiter();
  reject(new Error(message || '打印排版失败'));
};
const onFrameEvent = (e: FrameEvent) => {
  if (e.type === 'anchor' && e.anchor) currentNode.value = e.anchor.nodeId;
};

// ---- 搜索 ----
const query = ref('');
const hits = ref<LogPublicSearchHit[]>([]);
const hitsCursor = ref<string | null>(null);
const searching = ref(false);
const searchError = ref('');
const runSearch = async (more = false) => {
  const q = query.value.trim();
  if (!q) {
    hits.value = [];
    return;
  }
  searching.value = true;
  searchError.value = '';
  try {
    const r = await searchPublicLog(slug.value, q.slice(0, 200), more ? hitsCursor.value : null, 20);
    hits.value = more ? [...hits.value, ...r.items] : r.items;
    hitsCursor.value = r.nextCursor || null;
  } catch (error) {
    if (isLogError(error, 'NOT_FOUND')) onStale();
    else searchError.value = describeLogError(error, '搜索失败');
  } finally {
    searching.value = false;
  }
};
const openHit = (hit: LogPublicSearchHit) => {
  const unit = unitOfSection(hit.sectionId);
  if (!unit) return;
  void router.replace({ query: unitQuery(unit.id, hit.nodeId) });
  void locateNode(unit.id, hit.nodeId);
};

const copyPosition = async () => {
  const url = new URL(location.href);
  const node = currentNode.value;
  const q = `?${new URLSearchParams(unitQuery(unitId.value, node || undefined)).toString()}`;
  const link = `${url.origin}${url.pathname}#/log/views/${slug.value}${q}`;
  try {
    await navigator.clipboard.writeText(link);
    copied.value = true;
    setTimeout(() => (copied.value = false), 1500);
  } catch {
    window.prompt('复制链接', link);
  }
};
const copied = ref(false);

const printShow = ref(false);
const printBusy = ref(false);
const printError = ref('');
const printPageCount = computed(() => layout.value?.pageCount || 0);

const runPrint = async (range?: FramePrintRange) => {
  if (printBusy.value || !previewRef.value) return;
  printBusy.value = true;
  printError.value = '';
  try {
    await previewRef.value.print(range);
  } catch (error) {
    printError.value = error instanceof Error ? error.message : String(error);
    printShow.value = true;
  } finally {
    printBusy.value = false;
  }
};

const print = async () => {
  if (printBusy.value || loadingItems.value || !previewRef.value) return;
  printBusy.value = true;
  printError.value = '';
  try {
    const needsFullLoad = partial.value || nextCursor.value !== null;
    const needsPagedLayout = mode.value !== 'paged';
    let result = layout.value;
    if (needsFullLoad) await loadAllItemsForPrint();
    if (needsFullLoad || needsPagedLayout || !result?.pageCount) {
      const layoutReady = waitForPrintLayout();
      mode.value = 'paged';
      await nextTick();
      result = await layoutReady;
    }
    if (!result?.pageCount) throw new Error('当前章节没有可打印页面');
    if (result.pageCount > LONG_PRINT_PAGES) {
      printShow.value = true;
      return;
    }
    await previewRef.value.print();
  } catch (error) {
    clearPrintLayoutWaiter();
    printError.value = error instanceof Error ? error.message : String(error);
    printShow.value = true;
  } finally {
    printBusy.value = false;
  }
};

// 阅读页的外框固定为浅色纸张风格；控件主题不跟随访客的夜间配色，否则在浅色外框上不可见。
const readerThemeOverrides: GlobalThemeOverrides = {
  common: { textColor1: '#0f172a', textColor2: '#475569', textColor3: '#475569', bodyColor: '#ffffff' },
};

watch(slug, () => void loadDocument(), { immediate: true });
onBeforeUnmount(() => {
  generation += 1;
  controller?.abort();
  if (buildTimer) clearTimeout(buildTimer);
});
</script>

<template>
  <n-config-provider abstract :theme="null" :theme-overrides="readerThemeOverrides">
    <div class="log-public">
      <template v-if="state === 'ready' && doc">
        <header class="log-public__bar">
          <h1 class="log-public__title">{{ doc.title }}</h1>
          <n-select
            v-if="units.length > 1"
            :value="unitId"
            size="small"
            class="log-public__section"
            :options="units.map((u, i) => ({ label: u.title || `第 ${i + 1} 章`, value: u.id }))"
            aria-label="章节"
            @update:value="switchSection"
          />
          <n-input v-model:value="query" size="small" class="log-public__search" clearable placeholder="搜索全文" aria-label="搜索" @keydown.enter="runSearch()" />
          <n-radio-group v-model:value="mode" size="small" aria-label="阅读方式">
            <n-radio-button value="flow">流式</n-radio-button>
            <n-radio-button value="paged">纸张</n-radio-button>
          </n-radio-group>
          <n-button size="small" @click="copyPosition">{{ copied ? '已复制' : '复制当前位置' }}</n-button>
          <n-button size="small" :loading="printBusy" :disabled="printBusy || loadingItems" :title="loadingItems ? '本章仍在加载' : (!complete || mode !== 'paged' ? '打印时会自动完整加载本章并切换到纸张模式' : '')" @click="print">打印本章</n-button>
        </header>
        <div v-if="hits.length || searchError || searching" class="log-public__hits" role="region" aria-label="搜索结果">
          <n-alert v-if="searchError" type="error" :show-icon="false">{{ searchError }}</n-alert>
          <button v-for="h in hits" :key="h.nodeId" type="button" class="log-public__hit" @click="openHit(h)">
            <span class="log-public__hit-sec">{{ unitOfSection(h.sectionId)?.title }}</span>
            {{ h.excerpt }}
          </button>
          <n-button v-if="hitsCursor" size="tiny" :loading="searching" @click="runSearch(true)">更多结果</n-button>
          <n-button size="tiny" quaternary @click="hits = []; query = ''">关闭</n-button>
        </div>
        <n-alert v-if="itemsError" type="warning" :show-icon="false" class="log-public__alert">{{ itemsError }}</n-alert>
        <main class="log-public__body">
          <LogPreviewFrame
            ref="previewRef"
            :doc="frame?.doc || null"
            :asset-tokens="frame?.assetTokens || []"
            :prepare-assets="assets.prepare"
            :mode="mode"
            :design="false"
            :title="doc.title"
            @layout="onLayout"
            @layout-error="onLayoutError"
            @frame-event="onFrameEvent"
          />
        </main>
        <footer class="log-public__status" role="status" aria-live="polite">
          <span>{{ sectionTitle }}</span>
          <span>{{ items.length }} 段{{ loadingItems ? '（加载中…）' : complete ? '' : partial ? '（定位片段）' : '' }}</span>
          <n-button v-if="partial" size="tiny" @click="loadItems()">从章节开头阅读</n-button>
          <n-button v-else-if="nextCursor && !loadingItems" size="tiny" @click="loadItems(true)">加载更多</n-button>
          <span v-if="mode === 'paged' && layout">{{ complete ? `共 ${layout.pageCount} 页` : '页数待完整加载后确定' }}</span>
        </footer>
        <LogPrintDialog
          v-model:show="printShow"
          :page-count="printPageCount"
          :busy="printBusy"
          :error-text="printError"
          @print="runPrint"
        />
      </template>
      <div v-else class="log-public__state">
        <n-spin v-if="state === 'loading'" size="large" />
        <n-result v-else-if="state === 'missing'" status="404" title="LOG 尚未发布或已下架" description="链接可能已更换，或管理者已停用公开展示。" />
        <n-result v-else status="error" title="加载失败" :description="errorText">
          <template #footer><n-button @click="loadDocument">重试</n-button></template>
        </n-result>
      </div>
    </div>
  </n-config-provider>
</template>

<style scoped>
.log-public {
  height: 100vh;
  height: 100dvh;
  display: flex;
  flex-direction: column;
  background: #f3f2ef;
  color: #222;
}

.log-public__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: #fff;
  border-bottom: 1px solid #e5e5e5;
}

.log-public__title {
  font-size: 16px;
  margin: 0 8px 0 0;
  flex: 1 1 160px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-public__section {
  width: 160px;
}

.log-public__search {
  width: 180px;
}

.log-public__hits {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 40vh;
  overflow: auto;
  padding: 8px 12px;
  background: #fff;
  border-bottom: 1px solid #e5e5e5;
}

.log-public__hit {
  text-align: left;
  border: 0;
  background: none;
  padding: 4px 0;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  color: inherit;
}

.log-public__hit:focus-visible {
  outline: 2px solid #2080f0;
}

.log-public__hit-sec {
  color: #888;
  margin-right: 6px;
  font-size: 12px;
}

.log-public__alert {
  margin: 6px 12px 0;
}

.log-public__body {
  flex: 1;
  min-height: 0;
}

.log-public__status {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  padding: 4px 12px;
  font-size: 12px;
  color: #666;
  background: #fff;
  border-top: 1px solid #e5e5e5;
}

.log-public__state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

@media (max-width: 640px) {
  .log-public__search,
  .log-public__section {
    width: 100%;
  }
}
</style>
