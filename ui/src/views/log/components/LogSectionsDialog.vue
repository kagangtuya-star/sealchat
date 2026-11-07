<script setup lang="ts">
// 章节与来源：编排“章节 → 来源片段”。左侧是最终 LOG 顺序的大纲（拖拽 / 方向键排序），右侧检查器编辑选中项；
// 窄屏改为“大纲 → 详情”的层级导航。管理者可增删、拆分、合并与跨章移动片段并修改来源；
// 成员只能改章节标题、片段名称与整章顺序（服务端同样校验）。
import { computed, nextTick, ref, watch } from 'vue';
import { useDialog } from 'naive-ui';
import { useMediaQuery } from '@vueuse/core';
import { GripVertical, Plus, Stack2, X } from '@vicons/tabler';
import { describeLogError, fetchChannelIdentitiesForLog, type LogIdentityOption, type LogSourceRangeCheck } from '@/services/log/logApi';
import { listWorldChannels, type LogWorldChannel } from '@/services/log/logMessageCommands';
import type { LogDraft, LogSection, LogSourceSpec } from '@/types/log';
import { newSection, uuid } from '../logDraftOps';
import {
  channelHue,
  chapterTitle,
  chapterViews,
  continuationSourceRange,
  detachSection,
  formatSourceRange,
  layoutOf,
  mergeIntoPrevious,
  moveChapter,
  moveSection,
  removeSections,
  renameChapter,
  sectionLabel,
  sourceFlags,
  writeLayout,
  type LogChapterLayoutEntry,
  type LogChapterView,
} from '../logChapterOps';
import { useOutlineDrag, type OutlineDragKind, type OutlineDropTarget } from '../composables/useOutlineDrag';
import LogSourceInspector from './LogSourceInspector.vue';

const props = defineProps<{
  show: boolean;
  draft: LogDraft;
  update: (mutate: (d: LogDraft) => void) => boolean;
  checkSourceRange: (sectionId: string) => Promise<LogSourceRangeCheck>;
  worldId: string;
  canManage: boolean;
  currentSectionId?: string | null;
}>();
const emit = defineEmits<{
  (e: 'update:show', v: boolean): void;
  (e: 'select', id: string): void;
}>();

const dialog = useDialog();
const narrow = useMediaQuery('(max-width: 760px)');
const channels = ref<LogWorldChannel[]>([]);
const channelError = ref('');
const loadingChannels = ref(false);
const identityCache = ref<Record<string, LogIdentityOption[]>>({});
const newChannelId = ref<string | null>(null);

const views = computed(() => chapterViews(props.draft));
const layout = computed(() => layoutOf(views.value));
const multiCount = computed(() => views.value.filter((v) => v.sections.length > 1).length);
const channelName = (id: string) => channels.value.find((c) => c.id === id)?.name || (loadingChannels.value ? '…' : `未知频道 ${id.slice(0, 8)}`);
const channelKnown = (id: string) => channels.value.some((c) => c.id === id);
const channelOptions = computed(() => channels.value.map((c) => ({ label: c.name, value: c.id })));
const label = (s: LogSection) => sectionLabel(s, channelName);
const elementCounts = computed(() => {
  const m = new Map<string, number>();
  for (const e of props.draft.elements) m.set(e.anchor.sectionId, (m.get(e.anchor.sectionId) || 0) + 1);
  return m;
});
const elementCount = (id: string) => elementCounts.value.get(id) || 0;

// ---- 频道与角色 ----
let channelSeq = 0;
const loadChannels = async () => {
  if (channels.value.length || loadingChannels.value) return;
  const seq = ++channelSeq;
  const world = props.worldId;
  loadingChannels.value = true;
  channelError.value = '';
  try {
    const list = await listWorldChannels(world);
    if (seq !== channelSeq || world !== props.worldId) return;
    channels.value = list;
  } catch (error) {
    if (seq === channelSeq) channelError.value = describeLogError(error, '读取频道失败');
  } finally {
    if (seq === channelSeq) loadingChannels.value = false;
  }
};
watch(() => props.worldId, () => {
  channelSeq += 1;
  channels.value = [];
  identityCache.value = {};
  loadingChannels.value = false;
});

const identityPending = new Set<string>();
const loadIdentities = async (channelId: string) => {
  if (!channelId || identityCache.value[channelId] || identityPending.has(channelId)) return;
  identityPending.add(channelId);
  try {
    const res = await fetchChannelIdentitiesForLog(channelId);
    identityCache.value = { ...identityCache.value, [channelId]: res.identities };
  } catch {
    identityCache.value = { ...identityCache.value, [channelId]: [] };
  } finally {
    identityPending.delete(channelId);
  }
};
const identityOptions = (s: LogSection) => {
  const known = identityCache.value[s.source.channelId] || [];
  const opts = known.map((i) => ({ label: i.displayName || i.id, value: i.id }));
  for (const id of s.source.identityIds) if (!opts.some((o) => o.value === id)) opts.push({ label: id, value: id });
  return opts;
};

// ---- 选择 ----
type Selection = { kind: 'chapter'; key: string } | { kind: 'section'; id: string };
const selection = ref<Selection | null>(null);

/** 解析选择：单来源章节总是落到它唯一的片段上；旧 key 在章节物化后仍能找到所在章节。 */
const resolved = computed(() => {
  const sel = selection.value;
  if (!sel) return null;
  const list = views.value;
  if (sel.kind === 'section') {
    const index = list.findIndex((v) => v.sections.some((s) => s.id === sel.id));
    if (index < 0) return null;
    return { chapter: list[index], index, section: list[index].sections.find((s) => s.id === sel.id)! };
  }
  let index = list.findIndex((v) => v.key === sel.key);
  if (index < 0) index = list.findIndex((v) => v.sections.some((s) => s.id === sel.key));
  if (index < 0) return null;
  const chapter = list[index];
  return { chapter, index, section: chapter.sections.length === 1 ? chapter.sections[0] : null };
});
const selectedSectionId = computed(() => resolved.value?.section?.id || null);
const selectedChapterKey = computed(() => resolved.value?.chapter.key || null);

const selectSection = (id: string) => {
  selection.value = { kind: 'section', id };
  const s = props.draft.sections.find((x) => x.id === id);
  if (s) void loadIdentities(s.source.channelId);
};
const selectChapter = (v: LogChapterView) => {
  if (v.sections.length === 1) selectSection(v.sections[0].id);
  else selection.value = { kind: 'chapter', key: v.key };
};

watch(() => props.show, (open) => {
  if (!open) return;
  void loadChannels();
  const current = props.currentSectionId && props.draft.sections.some((s) => s.id === props.currentSectionId) ? props.currentSectionId : null;
  // 窄屏先看到完整结构；宽屏直接定位到工作台当前片段。
  if (narrow.value) selection.value = null;
  else if (current) selectSection(current);
  else if (views.value[0]) selectChapter(views.value[0]);
  else selection.value = null;
}, { immediate: true });

// 选中项被删除（本地或冲突重载）时回到合理位置，不保留悬空选择。
watch(resolved, (r) => {
  if (r || !selection.value) return;
  selection.value = null;
  if (!narrow.value && views.value[0]) selectChapter(views.value[0]);
});

// ---- 写入 ----
const commit = (next: LogChapterLayoutEntry[]) => {
  let keys: Map<string, string> | null = null;
  props.update((d) => {
    keys = writeLayout(d, next);
  });
  const sel = selection.value;
  const mapped = sel?.kind === 'chapter' ? (keys as Map<string, string> | null)?.get(sel.key) : undefined;
  if (sel?.kind === 'chapter' && mapped) selection.value = { kind: 'chapter', key: mapped };
};

const mutateSection = (id: string, fn: (s: LogSection) => void) => props.update((d) => {
  const s = d.sections.find((x) => x.id === id);
  if (s) fn(s);
});

/** 在 mutate 内部追加片段并按 place 计算的布局写回。 */
const insertSection = (section: LogSection, place: (base: LogChapterLayoutEntry[]) => LogChapterLayoutEntry[]) => {
  props.update((d) => {
    const base = layoutOf(chapterViews(d));
    d.sections.push(section);
    writeLayout(d, place(base));
  });
  selectSection(section.id);
};

const createChapter = () => {
  const ch = newChannelId.value;
  if (!ch) return;
  const s = newSection(ch, '', 0);
  insertSection(s, (base) => [...base, { key: uuid(), title: channelName(ch), sectionIds: [s.id] }]);
  newChannelId.value = null;
  void loadIdentities(ch);
  emit('select', s.id);
  void nextTick(() => scrollIntoView(`[data-row="${s.id}"]`));
};

const addPopover = ref<string | null>(null);
const addSource = (key: string, channelId: string) => {
  addPopover.value = null;
  const s = newSection(channelId, '', 0);
  insertSection(s, (base) => base.map((e) => (e.key === key ? { ...e, sectionIds: [...e.sectionIds, s.id] } : e)));
  void loadIdentities(channelId);
};

/** 同频道接续：复制筛选条件，从原片段结束之后开始，常用于把一个频道切成“开场 / 汇合”多段。 */
const continueSection = (id: string) => {
  const src = props.draft.sections.find((x) => x.id === id);
  if (!src) return;
  const s = newSection(src.source.channelId, '', 0);
  s.source = { ...src.source, ...continuationSourceRange(src), identityIds: src.source.identityIds.slice() };
  insertSection(s, (base) => base.map((e) => {
    const i = e.sectionIds.indexOf(id);
    if (i < 0) return e;
    const ids = e.sectionIds.slice();
    ids.splice(i + 1, 0, s.id);
    return { ...e, sectionIds: ids };
  }));
};

const onRenameChapter = (key: string, title: string) => commit(renameChapter(layout.value, key, title.slice(0, 200)));
const onRenameSection = (id: string, title: string) => mutateSection(id, (x) => (x.title = title.slice(0, 200)));
const onPatchSource = (id: string, patch: Partial<LogSourceSpec>) => {
  mutateSection(id, (x) => Object.assign(x.source, patch));
  if (patch.channelId) void loadIdentities(patch.channelId);
};
const onDetach = (id: string) => {
  const s = props.draft.sections.find((x) => x.id === id);
  if (!s) return;
  commit(detachSection(layout.value, id, label(s), uuid()));
};
const onMoveTo = (id: string, key: string) => {
  const target = layout.value.find((e) => e.key === key);
  if (target) commit(moveSection(layout.value, id, key, target.sectionIds.length));
};
const onMergePrevious = (key: string) => {
  const i = layout.value.findIndex((e) => e.key === key);
  commit(mergeIntoPrevious(layout.value, key));
  const prev = views.value[i - 1];
  if (prev) selection.value = { kind: 'chapter', key: prev.key };
};

const confirmRemove = (title: string, ids: string[], what: string) => {
  const count = ids.reduce((n, id) => n + elementCount(id), 0);
  dialog.warning({
    title,
    content: `${what}只影响 LOG，不会删除聊天消息${count ? `；锚定在其中的 ${count} 个 LOG 元素会一并删除` : ''}。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => {
      props.update((d) => {
        removeSections(d, ids);
      });
    },
  });
};
const onRemoveSection = (id: string) => {
  const s = props.draft.sections.find((x) => x.id === id);
  if (s) confirmRemove('删除来源片段', [id], `删除片段“${label(s)}”`);
};
const onRemoveChapter = (key: string) => {
  const i = views.value.findIndex((v) => v.key === key);
  const v = views.value[i];
  if (!v) return;
  confirmRemove('删除章节', v.sections.map((s) => s.id), `删除“${chapterTitle(v, i)}”及其 ${v.sections.length} 个来源片段`);
};
const openSection = (id: string) => {
  emit('select', id);
  emit('update:show', false);
};

// ---- 拖拽与键盘排序 ----
const outlineRef = ref<HTMLElement | null>(null);
const onDrop = (kind: OutlineDragKind, id: string, target: OutlineDropTarget) => {
  if (kind === 'chapter') commit(moveChapter(layout.value, id, target.index));
  else if (props.canManage && target.listKey) commit(moveSection(layout.value, id, target.listKey, target.index));
};
const drag = useOutlineDrag(outlineRef, onDrop);
const isDragging = (kind: OutlineDragKind, id: string) => drag.dragging.value?.kind === kind && drag.dragging.value.id === id;

const scrollIntoView = (selector: string) => outlineRef.value?.querySelector<HTMLElement>(selector)?.scrollIntoView({ block: 'nearest' });
const refocus = (handle: string) => void nextTick(() => {
  const el = outlineRef.value?.querySelector<HTMLElement>(`[data-handle="${handle}"]`);
  el?.focus();
  el?.scrollIntoView({ block: 'nearest' });
});

const onChapterKey = (e: KeyboardEvent, v: LogChapterView, i: number) => {
  if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
  e.preventDefault();
  const to = e.key === 'ArrowUp' ? i - 1 : i + 2;
  if (to < 0 || to > views.value.length) return;
  const first = v.sections[0]?.id;
  commit(moveChapter(layout.value, v.key, to));
  // 物化后章节 key 会变化，按其首个片段重新定位手柄。
  const key = views.value.find((x) => x.sections.some((s) => s.id === first))?.key;
  if (key) refocus(`chapter:${key}`);
};
const onSectionKey = (e: KeyboardEvent, v: LogChapterView, i: number, j: number) => {
  if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
  e.preventDefault();
  const id = v.sections[j].id;
  const up = e.key === 'ArrowUp';
  // 到达章节边界时跨入相邻章节，键盘也能完成跨章编排。
  if (up && j === 0) {
    const prev = layout.value[i - 1];
    if (!prev) return;
    commit(moveSection(layout.value, id, prev.key, prev.sectionIds.length));
  } else if (!up && j === v.sections.length - 1) {
    const next = layout.value[i + 1];
    if (!next) return;
    commit(moveSection(layout.value, id, next.key, 0));
  } else {
    commit(moveSection(layout.value, id, v.key, up ? j - 1 : j + 2));
  }
  refocus(`section:${id}`);
};

const notableFlags = (s: LogSection) => sourceFlags(s.source).filter((f) => f.notable);
const dropHint = computed(() => drag.dragging.value?.kind === 'section');
</script>

<template>
  <n-modal :show="show" :auto-focus="false" :mask-closable="!drag.dragging.value" @update:show="(v: boolean) => emit('update:show', v)">
    <div class="log-arr" :class="{ 'is-narrow': narrow, 'is-dragging': !!drag.dragging.value }" role="dialog" aria-modal="true" aria-labelledby="log-arr-title">
      <header class="log-arr__head">
        <n-icon :component="Stack2" size="20" class="log-arr__head-icon" />
        <div class="log-arr__head-text">
          <h2 id="log-arr-title">章节与来源</h2>
          <p>
            {{ views.length }} 章 · {{ draft.sections.length }} 个来源片段<template v-if="multiCount"> · {{ multiCount }} 章由多个片段串联</template>
          </p>
        </div>
        <button type="button" class="log-arr__close" aria-label="关闭" @click="emit('update:show', false)">
          <n-icon :component="X" size="18" />
        </button>
      </header>
      <n-alert v-if="channelError" type="error" :show-icon="false" class="log-arr__alert">{{ channelError }}</n-alert>

      <div class="log-arr__main">
        <section v-show="!narrow || !resolved" class="log-arr__outline-wrap" aria-label="LOG 章节结构">
          <p class="log-arr__intro">
            <template v-if="canManage">按这里的顺序输出最终 LOG。拖动 <n-icon :component="GripVertical" size="12" /> 调整章节或片段顺序；一章可以串联多个频道、多个时间段的来源片段。</template>
            <template v-else>你可以修改章节标题、拖动整章调整顺序；来源片段由世界管理者编排。</template>
          </p>
          <div ref="outlineRef" class="log-arr__outline">
            <div v-if="!views.length" class="log-arr__empty">
              <strong>还没有章节</strong>
              <span v-if="canManage">在下方选择一个频道即可创建第一章。</span>
              <span v-else>等待世界管理者添加章节来源。</span>
            </div>
            <ol class="log-arr__chapters">
              <li
                v-for="(v, i) in views"
                :key="v.key"
                data-drop-chapter
                class="log-arr__chapter"
                :class="{
                  'is-multi': v.sections.length > 1,
                  'is-selected': selectedChapterKey === v.key,
                  'is-dragged': isDragging('chapter', v.key),
                }"
              >
                <div class="log-arr__chapter-head" @click="selectChapter(v)">
                  <button
                    type="button"
                    class="log-arr__handle"
                    :data-handle="`chapter:${v.key}`"
                    :aria-label="`第 ${i + 1} 章：拖动或用上下方向键调整顺序`"
                    @pointerdown.stop="(e: PointerEvent) => drag.begin(e, 'chapter', v.key, chapterTitle(v, i))"
                    @keydown="(e: KeyboardEvent) => onChapterKey(e, v, i)"
                    @click.stop
                  >
                    <n-icon :component="GripVertical" size="16" />
                  </button>
                  <span class="log-arr__no">{{ i + 1 }}</span>
                  <span class="log-arr__chapter-title" :class="{ 'is-placeholder': !v.title.trim() }">{{ chapterTitle(v, i) }}</span>
                  <span v-if="v.sections.length > 1" class="log-arr__count">{{ v.sections.length }} 个片段</span>
                  <n-popover
                    v-if="canManage"
                    trigger="click"
                    placement="bottom-end"
                    :show-arrow="false"
                    :show="addPopover === v.key"
                    @update:show="(open: boolean) => (addPopover = open ? v.key : null)"
                  >
                    <template #trigger>
                      <button type="button" class="log-arr__mini" :aria-label="`在第 ${i + 1} 章添加来源片段`" title="在本章添加来源片段" @click.stop>
                        <n-icon :component="Plus" size="15" />
                      </button>
                    </template>
                    <div class="log-arr__pop">
                      <div class="log-arr__pop-title">添加来源片段到本章末尾</div>
                      <n-select
                        :value="null"
                        size="small"
                        filterable
                        :options="channelOptions"
                        :loading="loadingChannels"
                        placeholder="选择频道"
                        @update:value="(ch: string) => addSource(v.key, ch)"
                      />
                    </div>
                  </n-popover>
                </div>
                <ul class="log-arr__frags" :class="{ 'is-drop-target': dropHint }" :data-drop-list="v.key">
                  <li
                    v-for="(s, j) in v.sections"
                    :key="s.id"
                    data-drop-row
                    :data-row="s.id"
                    class="log-arr__frag"
                    :class="{
                      'is-selected': selectedSectionId === s.id,
                      'is-dragged': isDragging('section', s.id),
                      'is-single': v.sections.length === 1,
                    }"
                    :style="{ '--hue': channelHue(s.source.channelId) }"
                    @click="selectSection(s.id)"
                  >
                    <button
                      v-if="canManage && v.sections.length > 1"
                      type="button"
                      class="log-arr__handle is-small"
                      :data-handle="`section:${s.id}`"
                      :aria-label="`片段 ${label(s)}：拖动或用上下方向键调整顺序`"
                      @pointerdown.stop="(e: PointerEvent) => drag.begin(e, 'section', s.id, label(s))"
                      @keydown="(e: KeyboardEvent) => onSectionKey(e, v, i, j)"
                      @click.stop
                    >
                      <n-icon :component="GripVertical" size="14" />
                    </button>
                    <i class="log-arr__dot" aria-hidden="true" />
                    <span class="log-arr__frag-main">
                      <span v-if="v.sections.length > 1" class="log-arr__frag-name">{{ label(s) }}</span>
                      <span class="log-arr__frag-meta">
                        <span class="log-arr__channel">{{ channelName(s.source.channelId) }}</span>
                        <span class="log-arr__range">{{ formatSourceRange(s.source) }}</span>
                      </span>
                    </span>
                    <span class="log-arr__flags">
                      <span v-for="f in notableFlags(s)" :key="f.key" class="log-arr__flag">{{ f.text }}</span>
                      <span v-if="s.id === currentSectionId" class="log-arr__flag is-current" title="工作台当前打开的片段">编辑中</span>
                    </span>
                  </li>
                </ul>
              </li>
            </ol>
            <div
              v-if="drag.indicator.value"
              class="log-arr__indicator"
              :style="{ top: `${drag.indicator.value.top}px`, left: `${drag.indicator.value.left}px`, width: `${drag.indicator.value.width}px` }"
              aria-hidden="true"
            />
          </div>
          <div v-if="canManage" class="log-arr__create">
            <n-select
              v-model:value="newChannelId"
              :options="channelOptions"
              :loading="loadingChannels"
              filterable
              placement="top-start"
              placeholder="选择来源频道（仅公开且可读的频道）"
            />
            <n-button type="primary" :disabled="!newChannelId" @click="createChapter">
              <template #icon><n-icon :component="Plus" /></template>
              新建章节
            </n-button>
          </div>
        </section>

        <section v-if="resolved" class="log-arr__inspector" aria-label="章节详情">
          <LogSourceInspector
            :chapter="resolved.chapter"
            :chapter-index="resolved.index"
            :section="resolved.section"
            :chapters="views"
            :can-manage="canManage"
            :show-back="narrow"
            :current-section-id="currentSectionId || null"
            :channel-options="channelOptions"
            :channels-loading="loadingChannels"
            :channel-name="channelName"
            :channel-known="channelKnown"
            :identity-options="identityOptions"
            :element-count="elementCount"
            :check-source-range="checkSourceRange"
            @back="selection = null"
            @select-section="selectSection"
            @select-chapter="(key: string) => (selection = { kind: 'chapter', key })"
            @open="openSection"
            @rename-chapter="onRenameChapter"
            @rename-section="onRenameSection"
            @patch-source="onPatchSource"
            @add-source="addSource"
            @continue-section="continueSection"
            @detach-section="onDetach"
            @move-section="onMoveTo"
            @merge-previous="onMergePrevious"
            @remove-section="onRemoveSection"
            @remove-chapter="onRemoveChapter"
          />
        </section>
        <section v-else-if="!narrow" class="log-arr__inspector is-empty">
          <span>选择左侧的章节或来源片段查看详情</span>
        </section>
      </div>

      <div
        v-if="drag.dragging.value && drag.ghost.value"
        class="log-arr__ghost"
        :style="{ transform: `translate(${drag.ghost.value.x + 14}px, ${drag.ghost.value.y + 10}px)` }"
        aria-hidden="true"
      >
        {{ drag.dragging.value.kind === 'chapter' ? '章节' : '片段' }} · {{ drag.dragging.value.label }}
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.log-arr {
  --log-arr-accent: var(--primary-color, #2f6fde);
  --log-arr-bg: var(--sc-bg-elevated, #fff);
  --log-arr-panel: var(--sc-bg-surface, #fff);
  --log-arr-sunken: color-mix(in srgb, var(--sc-bg-page, #f5f5f7) 70%, var(--log-arr-bg));
  --log-arr-line: var(--sc-border-strong, rgba(15, 23, 42, 0.12));
  --log-arr-line-soft: var(--sc-border-mute, rgba(15, 23, 42, 0.06));
  --log-arr-hover: var(--sc-chip-bg, rgba(15, 23, 42, 0.05));
  --log-arr-muted: var(--sc-text-secondary, #475569);
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(1080px, 96vw);
  height: min(760px, 90vh);
  border-radius: 14px;
  overflow: hidden;
  background: var(--log-arr-bg);
  color: var(--sc-text-primary, #0f172a);
  box-shadow: 0 24px 64px rgba(0, 0, 0, 0.28);
  font-size: 13px;
}

.log-arr.is-narrow {
  width: 100vw;
  height: 100vh;
  height: 100dvh;
  border-radius: 0;
}

.log-arr.is-dragging {
  user-select: none;
  cursor: grabbing;
}

.log-arr__head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px 12px 18px;
  border-bottom: 1px solid var(--log-arr-line);
}

.log-arr__head-icon {
  color: var(--log-arr-accent);
}

.log-arr__head-text {
  flex: 1;
  min-width: 0;
}

.log-arr__head-text h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}

.log-arr__head-text p {
  margin: 1px 0 0;
  font-size: 12px;
  color: var(--log-arr-muted);
}

.log-arr__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.log-arr__close:hover,
.log-arr__close:focus-visible {
  background: var(--log-arr-hover);
}

.log-arr__alert {
  margin: 8px 14px 0;
}

.log-arr__main {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: minmax(320px, 420px) 1fr;
}

.is-narrow .log-arr__main {
  grid-template-columns: 1fr;
}

/* ---- 大纲 ---- */
.log-arr__outline-wrap {
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--log-arr-sunken);
  border-right: 1px solid var(--log-arr-line-soft);
}

.is-narrow .log-arr__outline-wrap {
  border-right: 0;
}

.log-arr__intro {
  margin: 0;
  padding: 10px 14px 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--log-arr-muted);
}

.log-arr__outline {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 4px 10px 12px;
  overscroll-behavior: contain;
}

.log-arr__empty {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 24px 4px;
  padding: 18px;
  border: 1px dashed var(--log-arr-line);
  border-radius: 10px;
  text-align: center;
  color: var(--log-arr-muted);
}

.log-arr__empty strong {
  color: var(--sc-text-primary, #0f172a);
}

.log-arr__chapters {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.log-arr__chapter {
  border: 1px solid var(--log-arr-line-soft);
  border-radius: 10px;
  background: var(--log-arr-panel);
  transition: border-color 0.12s, box-shadow 0.12s, opacity 0.12s;
}

.log-arr__chapter.is-selected {
  border-color: var(--log-arr-accent);
  box-shadow: 0 0 0 1px var(--log-arr-accent);
}

.log-arr__chapter.is-dragged,
.log-arr__frag.is-dragged {
  opacity: 0.4;
}

.log-arr__chapter-head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 38px;
  padding: 4px 6px 0 2px;
  cursor: pointer;
}

.log-arr__handle {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 30px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--log-arr-muted);
  cursor: grab;
  touch-action: none;
}

.log-arr__handle.is-small {
  width: 20px;
  height: 26px;
  margin-left: -4px;
}

.log-arr__handle:hover,
.log-arr__handle:focus-visible {
  background: var(--log-arr-hover);
  color: var(--sc-text-primary, #0f172a);
}

.log-arr__handle:focus-visible,
.log-arr__mini:focus-visible {
  outline: 2px solid var(--log-arr-accent);
  outline-offset: 1px;
}

.log-arr__no {
  flex: none;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--log-arr-hover);
  font-size: 11px;
  font-weight: 600;
  line-height: 20px;
  text-align: center;
  font-variant-numeric: tabular-nums;
}

.log-arr__chapter-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
  font-size: 14px;
}

.log-arr__chapter-title.is-placeholder {
  color: var(--log-arr-muted);
  font-weight: 500;
}

.log-arr__count {
  flex: none;
  padding: 1px 7px;
  border-radius: 999px;
  font-size: 11px;
  color: var(--log-arr-accent);
  background: color-mix(in srgb, var(--log-arr-accent) 12%, transparent);
}

.log-arr__mini {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--log-arr-muted);
  cursor: pointer;
}

.log-arr__mini:hover {
  background: var(--log-arr-hover);
  color: var(--log-arr-accent);
}

.log-arr__pop {
  width: 240px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.log-arr__pop-title {
  font-size: 12px;
  color: var(--log-arr-muted);
}

.log-arr__frags {
  list-style: none;
  margin: 0;
  padding: 2px 6px 6px 32px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-radius: 0 0 10px 10px;
  transition: background-color 0.12s;
}

.log-arr__frags.is-drop-target {
  background: color-mix(in srgb, var(--log-arr-accent) 5%, transparent);
}

.log-arr__frag {
  position: relative;
  display: flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  padding: 4px 8px;
  border-radius: 7px;
  cursor: pointer;
}

.log-arr__frag:hover {
  background: var(--log-arr-hover);
}

.log-arr__frag.is-selected {
  background: color-mix(in srgb, var(--log-arr-accent) 13%, transparent);
}

.log-arr__frag.is-single {
  min-height: 26px;
  padding-top: 0;
}

.log-arr__dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: hsl(var(--hue) 62% 52%);
  box-shadow: 0 0 0 2px color-mix(in srgb, hsl(var(--hue) 62% 52%) 22%, transparent);
}

.log-arr__frag-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  line-height: 1.35;
}

.log-arr__frag-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.log-arr__frag-meta {
  display: flex;
  gap: 6px;
  min-width: 0;
  font-size: 12px;
  color: var(--log-arr-muted);
  white-space: nowrap;
}

.log-arr__channel {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-arr__range {
  flex: none;
  font-variant-numeric: tabular-nums;
}

.log-arr__flags {
  flex: none;
  display: flex;
  gap: 3px;
}

.log-arr__flag {
  padding: 0 5px;
  border-radius: 4px;
  font-size: 11px;
  line-height: 18px;
  background: var(--log-arr-hover);
  color: var(--log-arr-muted);
}

.log-arr__flag.is-current {
  color: var(--log-arr-accent);
  background: color-mix(in srgb, var(--log-arr-accent) 12%, transparent);
}

.log-arr__indicator {
  position: absolute;
  height: 3px;
  margin-top: -1px;
  border-radius: 2px;
  background: var(--log-arr-accent);
  pointer-events: none;
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--log-arr-accent) 25%, transparent);
}

.log-arr__indicator::before {
  content: '';
  position: absolute;
  left: -5px;
  top: -3px;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: var(--log-arr-accent);
}

.log-arr__create {
  display: flex;
  gap: 8px;
  padding: 10px 12px;
  border-top: 1px solid var(--log-arr-line-soft);
  background: var(--log-arr-bg);
}

.log-arr__create .n-select {
  flex: 1;
  min-width: 0;
}

/* ---- 检查器 ---- */
.log-arr__inspector {
  min-height: 0;
  min-width: 0;
  background: var(--log-arr-bg);
}

.log-arr__inspector.is-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--log-arr-muted);
}

.log-arr__ghost {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 10;
  max-width: 260px;
  padding: 5px 10px;
  border-radius: 8px;
  background: var(--log-arr-accent);
  color: #fff;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.25);
}

@media (max-width: 760px) {
  .log-arr__frags {
    padding-left: 22px;
  }

  .log-arr__chapter-head,
  .log-arr__frag {
    min-height: 44px;
  }

  .log-arr__frag.is-single {
    min-height: 32px;
  }

  .log-arr__handle {
    width: 34px;
    height: 40px;
  }

  .log-arr__handle.is-small {
    width: 30px;
    height: 36px;
  }

  .log-arr__mini {
    width: 36px;
    height: 36px;
  }

  .log-arr__create {
    padding-bottom: max(10px, env(safe-area-inset-bottom));
  }
}
</style>
