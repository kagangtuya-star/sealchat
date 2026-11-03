<script setup lang="ts">
// 章节编排的检查器：编辑选中的章节或来源片段。单来源章节以“章节”的口吻呈现（标题 + 来源），
// 多来源章节才出现片段名称、拆分与跨章移动。成员看到的是只读的来源摘要而不是禁用的表单。
import { computed } from 'vue';
import {
  ArrowsSplit,
  ChevronLeft,
  ExternalLink,
  Lock,
  PlayerTrackNext,
  Plus,
  Trash,
} from '@vicons/tabler';
import type { LogSection, LogSourceSpec } from '@/types/log';
import { channelHue, chapterTitle, formatSourceRange, sectionLabel, sourceFlags, type LogChapterView } from '../logChapterOps';

const props = defineProps<{
  chapter: LogChapterView;
  chapterIndex: number;
  /** 选中的片段；为 null 表示选中的是（多来源）章节本身。 */
  section: LogSection | null;
  chapters: LogChapterView[];
  canManage: boolean;
  /** 窄屏层级导航时显示返回按钮。 */
  showBack: boolean;
  currentSectionId: string | null;
  channelOptions: Array<{ label: string; value: string }>;
  channelsLoading: boolean;
  channelName: (id: string) => string;
  channelKnown: (id: string) => boolean;
  identityOptions: (s: LogSection) => Array<{ label: string; value: string }>;
  elementCount: (sectionId: string) => number;
}>();
const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'select-section', id: string): void;
  (e: 'select-chapter', key: string): void;
  (e: 'open', sectionId: string): void;
  (e: 'rename-chapter', key: string, title: string): void;
  (e: 'rename-section', id: string, title: string): void;
  (e: 'patch-source', id: string, patch: Partial<LogSourceSpec>): void;
  (e: 'add-source', key: string, channelId: string): void;
  (e: 'continue-section', id: string): void;
  (e: 'detach-section', id: string): void;
  (e: 'move-section', id: string, key: string): void;
  (e: 'merge-previous', key: string): void;
  (e: 'remove-section', id: string): void;
  (e: 'remove-chapter', key: string): void;
}>();

const multi = computed(() => props.chapter.sections.length > 1);
const title = computed(() => chapterTitle(props.chapter, props.chapterIndex));
const label = (s: LogSection) => sectionLabel(s, props.channelName);
const moveOptions = computed(() => props.chapters
  .map((c, i) => ({ label: chapterTitle(c, i), value: c.key }))
  .filter((o) => o.value !== props.chapter.key));

const toMs = (iso: string | null) => (iso ? Date.parse(iso) : null);
const fromMs = (ms: number | null) => (ms === null || !Number.isFinite(ms) ? null : new Date(ms).toISOString());
const longTime = (iso: string | null) => (iso ? new Date(iso).toLocaleString() : '最早的消息');
const patch = (p: Partial<LogSourceSpec>) => props.section && emit('patch-source', props.section.id, p);
const onChannel = (id: string) => patch({ channelId: id, identityIds: [], fromExclusive: false });
const identityNames = (s: LogSection) => {
  const opts = props.identityOptions(s);
  return s.source.identityIds.map((id) => opts.find((o) => o.value === id)?.label || id);
};
const addChannel = (v: string | null) => v && emit('add-source', props.chapter.key, v);
</script>

<template>
  <div class="log-insp">
    <header class="log-insp__head">
      <button v-if="showBack" type="button" class="log-insp__back" aria-label="返回章节列表" @click="emit('back')">
        <n-icon :component="ChevronLeft" size="18" />
      </button>
      <nav class="log-insp__crumbs" aria-label="位置">
        <button v-if="section && multi" type="button" class="log-insp__crumb" @click="emit('select-chapter', chapter.key)">
          第 {{ chapterIndex + 1 }} 章 · {{ title }}
        </button>
        <span v-else class="log-insp__crumb is-plain">第 {{ chapterIndex + 1 }} 章</span>
        <template v-if="section && multi">
          <span class="log-insp__sep" aria-hidden="true">/</span>
          <span class="log-insp__crumb is-plain">片段 {{ chapter.sections.indexOf(section) + 1 }} / {{ chapter.sections.length }}</span>
        </template>
      </nav>
      <span v-if="!canManage" class="log-insp__lock"><n-icon :component="Lock" size="13" />来源只读</span>
    </header>

    <div class="log-insp__body">
      <!-- 章节标题：单来源章节与章节本身都在这里改名；多来源章节的片段页不重复显示。 -->
      <label v-if="!section || !multi" class="log-insp__field">
        <span class="log-insp__label">章节标题</span>
        <n-input
          :value="chapter.title"
          maxlength="200"
          :placeholder="`第 ${chapterIndex + 1} 章`"
          @update:value="(v: string) => emit('rename-chapter', chapter.key, v)"
        />
        <span class="log-insp__help">公开 LOG 与页眉中显示的章节名。</span>
      </label>

      <!-- 多来源章节：片段总览 -->
      <section v-if="!section" class="log-insp__group">
        <div class="log-insp__group-title">来源片段 · 按 LOG 顺序</div>
        <button
          v-for="(s, i) in chapter.sections"
          :key="s.id"
          type="button"
          class="log-insp__frag"
          @click="emit('select-section', s.id)"
        >
          <span class="log-insp__no">{{ i + 1 }}</span>
          <i class="log-insp__dot" :style="{ '--hue': channelHue(s.source.channelId) }" aria-hidden="true" />
          <span class="log-insp__frag-main">
            <span class="log-insp__frag-name">{{ label(s) }}</span>
            <span class="log-insp__frag-meta">{{ channelName(s.source.channelId) }} · {{ formatSourceRange(s.source) }}</span>
          </span>
          <span v-if="s.id === currentSectionId" class="log-insp__current">编辑中</span>
        </button>
        <div v-if="canManage" class="log-insp__add">
          <n-select
            :value="null"
            size="small"
            filterable
            :options="channelOptions"
            :loading="channelsLoading"
            placeholder="＋ 在本章末尾添加来源片段（选择频道）"
            @update:value="addChannel"
          />
        </div>
      </section>

      <!-- 片段（或单来源章节的来源） -->
      <template v-if="section">
        <label v-if="multi" class="log-insp__field">
          <span class="log-insp__label">片段名称</span>
          <n-input
            :value="section.title"
            maxlength="200"
            :placeholder="channelName(section.source.channelId)"
            @update:value="(v: string) => emit('rename-section', section!.id, v)"
          />
          <span class="log-insp__help">只用于编排时辨认，例如“大厅 · 汇合”；不会出现在公开 LOG。</span>
        </label>

        <section class="log-insp__group">
          <div class="log-insp__group-title">
            <i class="log-insp__dot" :style="{ '--hue': channelHue(section.source.channelId) }" aria-hidden="true" />
            来源
          </div>
          <div v-if="canManage" class="log-insp__form">
            <label class="log-insp__field">
              <span class="log-insp__label">频道</span>
              <n-select
                :value="section.source.channelId"
                filterable
                :options="channelOptions"
                :loading="channelsLoading"
                :status="channelKnown(section.source.channelId) || channelsLoading ? undefined : 'warning'"
                @update:value="onChannel"
              />
            </label>
            <div class="log-insp__row2">
              <label class="log-insp__field">
                <span class="log-insp__label">开始</span>
                <n-date-picker
                  :value="toMs(section.source.fromTime)"
                  type="datetime"
                  clearable
                  placeholder="从最早的消息"
                  @update:value="(v: number | null) => patch({ fromTime: fromMs(v), fromExclusive: false })"
                />
              </label>
              <label class="log-insp__field">
                <span class="log-insp__label">结束</span>
                <n-date-picker
                  :value="toMs(section.source.toTime)"
                  type="datetime"
                  @update:value="(v: number | null) => v !== null && patch({ toTime: fromMs(v) || section!.source.toTime })"
                />
              </label>
            </div>
            <div class="log-insp__quick">
              <n-button size="tiny" quaternary @click="patch({ toTime: new Date().toISOString() })">结束 = 现在</n-button>
              <n-button v-if="section.source.fromTime" size="tiny" quaternary @click="patch({ fromTime: null, fromExclusive: false })">从最早开始</n-button>
            </div>
            <div class="log-insp__toggles">
              <n-checkbox :checked="section.source.includeOoc" @update:checked="(v: boolean) => patch({ includeOoc: v })">包含场外 (OOC)</n-checkbox>
              <n-checkbox :checked="section.source.includeArchived" @update:checked="(v: boolean) => patch({ includeArchived: v })">包含已归档</n-checkbox>
              <n-checkbox :checked="!!section.source.showQuoteContent" @update:checked="(v: boolean) => patch({ showQuoteContent: v })">显示引用内容</n-checkbox>
            </div>
            <label class="log-insp__field">
              <span class="log-insp__label">只看角色</span>
              <n-select
                :value="section.source.identityIds"
                multiple
                filterable
                clearable
                max-tag-count="responsive"
                placeholder="不筛选（全部角色）"
                :options="identityOptions(section)"
                @update:value="(v: string[]) => patch({ identityIds: v.slice(0, 200) })"
              />
            </label>
            <p class="log-insp__help">时间范围只决定哪些消息属于本片段；片段内部仍按聊天顺序排列。</p>
          </div>
          <dl v-else class="log-insp__facts">
            <dt>频道</dt>
            <dd>{{ channelName(section.source.channelId) }}</dd>
            <dt>时间</dt>
            <dd>{{ longTime(section.source.fromTime) }} → {{ longTime(section.source.toTime) }}</dd>
            <dt>角色</dt>
            <dd>{{ section.source.identityIds.length ? identityNames(section).join('、') : '全部角色' }}</dd>
            <dt>包含</dt>
            <dd>
              <span v-for="f in sourceFlags(section.source).slice(1)" :key="f.key" class="log-insp__flag" :class="{ 'is-notable': f.notable }">{{ f.text }}</span>
            </dd>
          </dl>
        </section>

        <section class="log-insp__group log-insp__actions">
          <n-button type="primary" secondary size="small" @click="emit('open', section!.id)">
            <template #icon><n-icon :component="ExternalLink" /></template>
            在工作台中打开
          </n-button>
          <template v-if="canManage">
            <n-button size="small" @click="emit('continue-section', section!.id)">
              <template #icon><n-icon :component="PlayerTrackNext" /></template>
              添加接续片段
            </n-button>
            <n-button v-if="!multi" size="small" @click="addChannel(section!.source.channelId)">
              <template #icon><n-icon :component="Plus" /></template>
              本章再加一个来源
            </n-button>
            <n-button v-if="multi" size="small" @click="emit('detach-section', section!.id)">
              <template #icon><n-icon :component="ArrowsSplit" /></template>
              拆为独立章节
            </n-button>
          </template>
        </section>
        <section v-if="canManage" class="log-insp__group">
          <label class="log-insp__field">
            <span class="log-insp__label">{{ multi ? '移动到其他章节' : '并入其他章节' }}</span>
            <n-select
              :value="null"
              size="small"
              :options="moveOptions"
              :disabled="!moveOptions.length"
              placeholder="选择目标章节（放在末尾）"
              @update:value="(k: string) => emit('move-section', section!.id, k)"
            />
          </label>
        </section>
      </template>

      <section v-if="canManage" class="log-insp__group log-insp__danger">
        <n-button v-if="!section && chapterIndex > 0" size="small" quaternary @click="emit('merge-previous', chapter.key)">并入上一章</n-button>
        <n-button v-if="section && multi" size="small" type="error" quaternary @click="emit('remove-section', section!.id)">
          <template #icon><n-icon :component="Trash" /></template>
          删除此片段{{ elementCount(section.id) ? `（含 ${elementCount(section.id)} 个元素）` : '' }}
        </n-button>
        <n-button size="small" type="error" quaternary @click="emit('remove-chapter', chapter.key)">
          <template #icon><n-icon :component="Trash" /></template>
          删除整章
        </n-button>
      </section>
      <p v-if="canManage" class="log-insp__help log-insp__foot">删除只影响 LOG 排版，不会删除聊天消息。</p>
    </div>
  </div>
</template>

<style scoped>
.log-insp {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.log-insp__head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 40px;
  padding: 6px 14px;
  border-bottom: 1px solid var(--log-arr-line-soft);
}

.log-insp__back {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  margin-left: -6px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.log-insp__back:hover,
.log-insp__back:focus-visible {
  background: var(--log-arr-hover);
}

.log-insp__crumbs {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--log-arr-muted);
  white-space: nowrap;
  overflow: hidden;
}

.log-insp__crumb {
  border: 0;
  background: none;
  padding: 2px 4px;
  border-radius: 4px;
  font: inherit;
  color: var(--log-arr-accent);
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-insp__crumb.is-plain {
  color: inherit;
  cursor: default;
}

.log-insp__lock {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 12px;
  color: var(--log-arr-muted);
}

.log-insp__body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.log-insp__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.log-insp__label {
  font-size: 12px;
  font-weight: 600;
  color: var(--log-arr-muted);
}

.log-insp__help {
  margin: 0;
  font-size: 12px;
  color: var(--log-arr-muted);
  line-height: 1.5;
}

.log-insp__group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.log-insp__group-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: var(--log-arr-muted);
}

.log-insp__form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--log-arr-line-soft);
  border-radius: 10px;
  background: var(--log-arr-panel);
}

.log-insp__row2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.log-insp__quick {
  display: flex;
  gap: 4px;
  margin-top: -6px;
}

.log-insp__toggles {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
}

.log-insp__facts {
  display: grid;
  grid-template-columns: 48px 1fr;
  gap: 8px 10px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--log-arr-line-soft);
  border-radius: 10px;
  background: var(--log-arr-panel);
  font-size: 13px;
}

.log-insp__facts dt {
  color: var(--log-arr-muted);
  font-size: 12px;
  padding-top: 1px;
}

.log-insp__facts dd {
  margin: 0;
  min-width: 0;
  overflow-wrap: anywhere;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.log-insp__flag {
  padding: 0 6px;
  border-radius: 999px;
  font-size: 12px;
  background: var(--log-arr-hover);
}

.log-insp__flag.is-notable {
  color: var(--log-arr-accent);
  background: color-mix(in srgb, var(--log-arr-accent) 14%, transparent);
}

.log-insp__frag {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--log-arr-line-soft);
  border-radius: 8px;
  background: var(--log-arr-panel);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.log-insp__frag:hover,
.log-insp__frag:focus-visible {
  border-color: var(--log-arr-accent);
}

.log-insp__no {
  width: 18px;
  font-size: 11px;
  color: var(--log-arr-muted);
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.log-insp__dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: hsl(var(--hue) 62% 52%);
}

.log-insp__frag-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.log-insp__frag-name,
.log-insp__frag-meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-insp__frag-meta {
  font-size: 12px;
  color: var(--log-arr-muted);
}

.log-insp__current {
  flex: none;
  font-size: 11px;
  color: var(--log-arr-accent);
}

.log-insp__actions {
  flex-direction: row;
  flex-wrap: wrap;
  gap: 6px;
}

.log-insp__danger {
  flex-direction: row;
  flex-wrap: wrap;
  gap: 4px;
  padding-top: 12px;
  border-top: 1px dashed var(--log-arr-line-soft);
}

.log-insp__foot {
  margin-top: -8px;
}

@media (max-width: 520px) {
  .log-insp__row2 {
    grid-template-columns: 1fr;
  }
}
</style>
