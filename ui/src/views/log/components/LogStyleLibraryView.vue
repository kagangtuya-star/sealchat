<script setup lang="ts">
// Style Studio 的样式库视图：浏览内置与私有样式，并从这里新建、导入、复制、打开或预览样式。
// 只负责展示与发出意图；工作副本、保存与绑定由 LogStyleStudio 处理。
import { computed, ref } from 'vue';
import type { DropdownOption } from 'naive-ui';
import { Copy, Download, Eye, FileZip, Pencil, Plus, Refresh, Search } from '@vicons/tabler';
import type { LogStylePackage, LogStyleSummary } from '@/types/log';
import { builtinStyleInfo } from '../logStyleCatalog';
import type { LogStyleLibrary } from '../composables/useLogStyleLibrary';
import LogStyleSketch from './LogStyleSketch.vue';

const props = defineProps<{
  library: LogStyleLibrary;
  /** 当前 LOG 绑定的样式包（可能是已下线内置或他人共享的样式，不一定在列表里）。 */
  boundPackage: LogStylePackage | null;
  /** 正在 LOG 预览中显示的样式 id@version。 */
  previewingKey: string;
  canBind: boolean;
  busy: boolean;
}>();
const emit = defineEmits<{
  (e: 'open', s: LogStyleSummary): void;
  (e: 'fork', s: LogStyleSummary): void;
  (e: 'preview', s: LogStyleSummary | null): void;
  (e: 'apply', s: LogStyleSummary): void;
  (e: 'export', s: LogStyleSummary): void;
  (e: 'delete', s: LogStyleSummary): void;
  (e: 'new', kind: 'blank' | 'copy-bound'): void;
  (e: 'import'): void;
}>();

const query = ref('');
const match = (s: LogStyleSummary) => {
  const q = query.value.trim().toLowerCase();
  return !q || s.name.toLowerCase().includes(q) || s.id.toLowerCase().includes(q);
};
const builtin = computed(() => props.library.builtin.value.filter(match));
const mine = computed(() => props.library.mine.value.filter(match));
const keyOf = (s: { id: string; version: string }) => `${s.id}@${s.version}`;

/** 当前 LOG 的样式条目；列表中没有时（下线内置 / 他人样式）用绑定包本身补一条。 */
const bound = computed<LogStyleSummary | null>(() => {
  const pkg = props.boundPackage;
  if (!pkg) return null;
  const listed = props.library.byId(pkg.id);
  return { id: pkg.id, version: pkg.version, name: pkg.name, builtin: listed?.builtin ?? pkg.id.startsWith('sealchat-'), previewAssetId: null };
});
const boundLatest = computed(() => {
  const pkg = props.boundPackage;
  const listed = pkg ? props.library.byId(pkg.id) : null;
  return listed && listed.version !== pkg!.version ? listed.version : '';
});
const isBound = (s: LogStyleSummary) => props.boundPackage?.id === s.id && props.boundPackage?.version === s.version;
const ownership = (s: LogStyleSummary) => props.library.ownership(s.id);
const editable = (s: LogStyleSummary) => ownership(s) === 'owned';
const ownerLabel = (s: LogStyleSummary) => ownership(s) === 'owned' ? '我的样式' : s.builtin ? '内置' : ownership(s) === 'foreign' ? '他人共享' : '归属待确认';

const newOptions: DropdownOption[] = [
  { key: 'blank', label: '空白起点' },
  { key: 'copy-bound', label: '复制当前 LOG 使用的样式' },
];
const moreOptions = (s: LogStyleSummary): DropdownOption[] => [
  { key: 'preview', label: props.previewingKey === keyOf(s) ? '取消 LOG 预览' : '在 LOG 中预览' },
  { key: 'apply', label: isBound(s) ? '当前 LOG 已在使用' : '应用到当前 LOG', disabled: !props.canBind || isBound(s) },
  { key: 'export', label: '导出 ZIP' },
  ...(editable(s) ? [{ key: 'fork', label: '复制为新样式' }] : []),
  ...(editable(s) ? [{ key: 'delete', label: '删除样式…' }] : []),
];
const onMore = (s: LogStyleSummary, key: string) => {
  if (key === 'preview') emit('preview', props.previewingKey === keyOf(s) ? null : s);
  else if (key === 'apply') emit('apply', s);
  else if (key === 'export') emit('export', s);
  else if (key === 'fork') emit('fork', s);
  else if (key === 'delete') emit('delete', s);
};
</script>

<template>
  <div class="lsl">
    <div class="lsl__toolbar">
      <n-dropdown trigger="click" :options="newOptions" :disabled="busy" @select="(k: string) => emit('new', k as 'blank' | 'copy-bound')">
        <n-button size="small" type="primary" :disabled="busy">
          <template #icon><n-icon :component="Plus" /></template>
          新建样式
        </n-button>
      </n-dropdown>
      <n-button size="small" :disabled="busy" @click="emit('import')">
        <template #icon><n-icon :component="FileZip" /></template>
        导入 ZIP…
      </n-button>
      <n-input v-model:value="query" class="lsl__search" size="small" clearable placeholder="搜索名称或 ID" aria-label="搜索样式">
        <template #prefix><n-icon :component="Search" /></template>
      </n-input>
      <n-button size="small" quaternary circle :loading="library.loading.value" title="刷新样式库" aria-label="刷新样式库" @click="library.reload()">
        <template #icon><n-icon :component="Refresh" /></template>
      </n-button>
    </div>
    <n-alert v-if="library.error.value" type="error" :show-icon="false">{{ library.error.value }}</n-alert>

    <section v-if="bound" class="lsl__section" aria-label="当前 LOG 使用的样式">
      <h3 class="lsl__heading">当前 LOG 使用</h3>
      <article class="lsl__card is-bound">
        <LogStyleSketch :kind="builtinStyleInfo(bound.id)?.sketch || 'generic'" />
        <div class="lsl__card-body">
          <div class="lsl__card-head">
            <strong>{{ bound.name }}</strong>
            <span class="lsl__tag">{{ ownerLabel(bound) }}</span>
          </div>
          <span class="lsl__meta">{{ bound.id }} @ {{ bound.version }}<template v-if="boundLatest"> · 样式库已有 {{ boundLatest }}</template></span>
          <div class="lsl__actions">
            <n-button v-if="editable(bound)" size="tiny" type="primary" secondary :disabled="busy" @click="emit('open', library.byId(bound.id) || bound)">
              <template #icon><n-icon :component="Pencil" /></template>编辑最新版本
            </n-button>
            <template v-else-if="ownership(bound) === 'foreign'">
              <n-button size="tiny" type="primary" secondary :disabled="busy" @click="emit('fork', bound)">
                <template #icon><n-icon :component="Copy" /></template>复制后编辑
              </n-button>
              <n-button size="tiny" :disabled="busy" @click="emit('open', bound)">
                <template #icon><n-icon :component="Eye" /></template>查看
              </n-button>
            </template>
            <n-button v-else size="tiny" type="primary" secondary :disabled="busy" @click="emit('open', bound)">
              <template #icon><n-icon :component="Pencil" /></template>打开编辑
            </n-button>
            <n-button size="tiny" quaternary :disabled="busy" @click="emit('export', bound)">
              <template #icon><n-icon :component="Download" /></template>导出
            </n-button>
          </div>
        </div>
      </article>
    </section>

    <section class="lsl__section" aria-label="我的样式">
      <h3 class="lsl__heading">我的样式 <span class="lsl__count">{{ mine.length }}</span></h3>
      <p v-if="!mine.length && library.loaded.value" class="lsl__empty">
        {{ query ? '没有匹配的私有样式。' : '还没有私有样式。可以新建空白样式、复制下方内置样式，或导入 ZIP。' }}
      </p>
      <div class="lsl__grid">
        <article v-for="s in mine" :key="s.id" class="lsl__card" :class="{ 'is-bound': boundPackage?.id === s.id, 'is-previewing': previewingKey === keyOf(s) }">
          <LogStyleSketch kind="generic" />
          <div class="lsl__card-body">
            <div class="lsl__card-head"><strong>{{ s.name }}</strong></div>
            <span class="lsl__meta">v{{ s.version }} · {{ s.id }}</span>
            <span v-if="boundPackage?.id === s.id" class="lsl__meta is-accent">当前 LOG 使用 v{{ boundPackage.version }}</span>
            <div class="lsl__actions">
              <n-button size="tiny" type="primary" secondary :disabled="busy" @click="emit('open', s)">
                <template #icon><n-icon :component="Pencil" /></template>编辑
              </n-button>
              <n-dropdown trigger="click" :options="moreOptions(s)" @select="(k: string) => onMore(s, k)">
                <n-button size="tiny" quaternary :disabled="busy">更多…</n-button>
              </n-dropdown>
            </div>
          </div>
        </article>
      </div>
    </section>

    <section class="lsl__section" aria-label="内置样式">
      <h3 class="lsl__heading">内置样式 <span class="lsl__count">{{ builtin.length }}</span></h3>
      <p class="lsl__hint">内置样式不能直接修改；“复制后编辑”会先复制成你的私有样式。</p>
      <div class="lsl__grid">
        <article v-for="s in builtin" :key="s.id" class="lsl__card" :class="{ 'is-bound': boundPackage?.id === s.id, 'is-previewing': previewingKey === keyOf(s) }">
          <LogStyleSketch :kind="builtinStyleInfo(s.id)?.sketch || 'generic'" />
          <div class="lsl__card-body">
            <div class="lsl__card-head">
              <strong>{{ s.name }}</strong>
              <span class="lsl__tag">{{ builtinStyleInfo(s.id)?.category || '内置' }}</span>
            </div>
            <p v-if="builtinStyleInfo(s.id)" class="lsl__summary">{{ builtinStyleInfo(s.id)!.summary }}</p>
            <span class="lsl__meta">v{{ s.version }}</span>
            <div class="lsl__actions">
              <n-button size="tiny" type="primary" secondary :disabled="busy" @click="emit('fork', s)">
                <template #icon><n-icon :component="Copy" /></template>复制后编辑
              </n-button>
              <n-button size="tiny" :disabled="busy" @click="emit('open', s)">查看</n-button>
              <n-dropdown trigger="click" :options="moreOptions(s)" @select="(k: string) => onMore(s, k)">
                <n-button size="tiny" quaternary :disabled="busy">更多…</n-button>
              </n-dropdown>
            </div>
          </div>
        </article>
      </div>
      <n-spin v-if="library.loading.value && !library.loaded.value" size="small" />
    </section>
  </div>
</template>

<style scoped>
.lsl {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 14px 16px 24px;
}

.lsl__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.lsl__search {
  flex: 1 1 180px;
  max-width: 320px;
  margin-left: auto;
}

.lsl__section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.lsl__heading {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--sc-text-secondary, #555);
}

.lsl__count {
  margin-left: 4px;
  font-weight: 400;
  font-size: 12px;
}

.lsl__hint,
.lsl__empty {
  margin: -2px 0 0;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.lsl__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 10px;
}

.lsl__card {
  display: flex;
  gap: 10px;
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--sc-border-mute, #e5e5e5);
  border-radius: 10px;
  background: var(--sc-bg-elevated, #fff);
  transition: border-color 0.15s ease;
}

.lsl__card:hover {
  border-color: var(--sc-border-strong, #cfcfcf);
}

.lsl__card.is-bound {
  border-color: var(--primary-color, #2f6fde);
  background: color-mix(in srgb, var(--primary-color, #2f6fde) 6%, var(--sc-bg-elevated, #fff));
}

.lsl__card.is-previewing {
  border-style: dashed;
  border-color: var(--primary-color, #2f6fde);
}

.lsl__card-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.lsl__card-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 6px;
  font-size: 13px;
}

.lsl__card-head strong {
  overflow-wrap: anywhere;
}

.lsl__tag {
  padding: 0 6px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 18px;
  color: var(--sc-text-secondary, #666);
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
}

.lsl__summary {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
}

.lsl__meta {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  overflow-wrap: anywhere;
}

.lsl__meta.is-accent {
  color: var(--primary-color, #2f6fde);
}

.lsl__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}
</style>
