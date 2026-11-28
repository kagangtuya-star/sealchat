<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { ChevronDown, ChevronUp, Refresh, Replace, Search } from '@vicons/tabler';
import { describeLogError } from '@/services/log/logApi';
import { findTextMatches } from '@/services/log/proofread/logProofread';
import type { LogDesignMessage, LogDraft, LogProtocolMessage, LogSection } from '@/types/log';
import { applyLogReplacements, scanLogMatches, setLogContentOverride, undoLogReplacements, verifyLogMatchSources, type LogReplacementRecord } from '../logFindReplace';

const props = defineProps<{
  projectId: string;
  draft: LogDraft;
  chapterSections: LogSection[];
  chapterEpoch: number;
  sections: LogSection[];
  windowItems: LogDesignMessage[];
  canReplace: boolean;
  canEditDraft: boolean;
  update: (mutate: (draft: LogDraft) => void) => boolean;
  flush: () => Promise<boolean>;
  saveError: string;
}>();
const emit = defineEmits<{
  locate: [messageId: string, sectionId: string, signal: AbortSignal];
  saved: [message: LogProtocolMessage, version: number];
}>();

const toast = useMessage();
const query = ref('');
const replacement = ref('');
const expanded = ref(false);
const scope = ref<'chapter' | 'project'>('chapter');
const scopeOptions = [{ value: 'chapter', label: '当前章节' }, { value: 'project', label: '整个工程' }];
const sections = computed(() => scope.value === 'project' ? props.sections : props.chapterSections);
const scopeKey = computed(() => JSON.stringify(sections.value.map(({ id, source }) => [id, source])));

const snapshot = shallowRef<Awaited<ReturnType<typeof scanLogMatches>> | null>(null);
const active = ref(-1);
const stale = shallowRef<ReadonlySet<string>>(new Set());
const phase = ref<'scan' | 'replace' | 'undo' | null>(null);
const scanError = ref('');
const undoRecord = shallowRef<LogReplacementRecord[] | null>(null);
let request: AbortController | null = null;
let navigation: AbortController | null = null;
let scanDirection: 1 | -1 | undefined;
let timer: ReturnType<typeof setTimeout> | undefined;

const hits = computed(() => snapshot.value?.results.flatMap((result) => result.matches.map((match) => ({ result, match }))) ?? []);
const current = computed(() => hits.value[active.value]);
const isReplaceable = ({ result, match }: (typeof hits.value)[number]) => result.canEdit && !!result.channelId && match.replaceable
  && (props.canEditDraft || props.draft.messageContentOverrides?.[result.messageId] === undefined)
  && !stale.value.has(result.messageId) && match.original !== replacement.value;
const replaceableCount = computed(() => hits.value.filter(isReplaceable).length);
const summary = computed(() => `${hits.value.length} 处命中，${replaceableCount.value} 处可替换${stale.value.size ? `，${stale.value.size} 条消息已变化` : ''}`);
const status = computed(() => {
  if (!query.value) return '';
  if (scanError.value) return '查找失败';
  if (!snapshot.value) return '查找中';
  if (!hits.value.length) return '无命中';
  return active.value < 0 ? `${hits.value.length} 处` : `${active.value + 1} / ${hits.value.length}`;
});

const reset = () => {
  clearTimeout(timer);
  timer = undefined;
  request?.abort();
  request = null;
  navigation?.abort();
  navigation = null;
  scanDirection = undefined;
  phase.value = null;
  snapshot.value = null;
  active.value = -1;
  stale.value = new Set();
  scanError.value = '';
};
const showHit = (index: number) => {
  active.value = index;
  navigation?.abort();
  const hit = hits.value[index];
  if (!hit) return;
  navigation = new AbortController();
  emit('locate', hit.result.messageId, hit.result.sectionId, navigation.signal);
};
const scan = async (direction?: 1 | -1) => {
  reset();
  if (!query.value) return;
  const needle = query.value;
  const projectId = props.projectId;
  const ids = sections.value.map((section) => section.id);
  const operation = request = new AbortController();
  scanDirection = direction;
  phase.value = 'scan';
  try {
    if (ids.length && !(await props.flush())) throw new Error(props.saveError || '草稿尚未保存，请稍后再查找');
    if (request !== operation) return;
    const found = await scanLogMatches(projectId, ids, props.draft.messageContentOverrides, needle, operation.signal);
    if (request !== operation) return;
    snapshot.value = found;
    if (scanDirection && hits.value.length) showHit(scanDirection === 1 ? 0 : hits.value.length - 1);
  } catch (error) {
    if (request === operation) scanError.value = describeLogError(error, '读取 LOG 正文失败');
  } finally {
    if (request === operation) {
      request = null;
      phase.value = null;
    }
  }
};
const navigate = (direction: 1 | -1) => {
  if (phase.value === 'scan') scanDirection = direction;
  if (phase.value) return;
  if (!snapshot.value) {
    void scan(direction);
    return;
  }
  const total = hits.value.length;
  if (!total) return;
  showHit(active.value < 0 ? (direction === 1 ? 0 : total - 1) : (active.value + direction + total) % total);
};
const onFindEnter = (event: KeyboardEvent) => {
  if (!event.isComposing) navigate(event.shiftKey ? -1 : 1);
};
watch([query, scope, scopeKey, () => props.projectId, () => props.chapterEpoch], () => {
  reset();
  if (query.value) timer = setTimeout(() => void scan(), 300);
});
watch(() => props.projectId, () => { undoRecord.value = null; });
onBeforeUnmount(reset);

const saveResult = async (operation: AbortController, summary: string) => {
  const saved = await props.flush().catch(() => false);
  if (request !== operation) return;
  if (saved) toast.success(summary);
  else toast.error(`${summary}，但旧工程正文的同步保存失败：${props.saveError || '请检查草稿保存状态'}`);
};

const onMessageSaved = (projectId: string, message: LogProtocolMessage, expectedOverride: string | undefined, restoredOverride?: string) => {
  if (props.projectId !== projectId) return;
  emit('saved', message, message.version!);
  if (expectedOverride === restoredOverride) return;
  let synchronized = false;
  if (!props.canEditDraft || !props.update((draft) => {
    synchronized = setLogContentOverride(draft, message.id, expectedOverride, restoredOverride);
  }) || !synchronized) throw new Error('原消息已保存，但旧工程正文未同步，请检查草稿权限、冲突或并发修改');
};

const apply = async (all: boolean) => {
  const scanned = snapshot.value;
  if (!scanned || !props.canReplace || phase.value) return;
  const text = replacement.value;
  const selected = current.value;
  const targets = (all ? scanned.results : selected ? [{ ...selected.result, matches: [selected.match] }] : [])
    .map((result) => ({ ...result, matches: result.matches.filter((match) => isReplaceable({ result, match })) }))
    .filter((result) => result.matches.length);
  if (!targets.length) {
    toast.info('没有需要替换的正文');
    return;
  }
  const oldIndex = active.value;
  const projectId = props.projectId;
  const operation = request = new AbortController();
  phase.value = 'replace';
  try {
    if (!(await props.flush())) throw new Error(props.saveError || '草稿尚未保存，未替换原消息');
    if (request !== operation) return;
    const invalidSources = await verifyLogMatchSources(props.projectId, targets, scanned.versions, operation.signal);
    if (request !== operation || !props.canReplace) return;
    const { count, writes, stale: skipped, error, record } = await applyLogReplacements(
      () => props.draft, targets, text, invalidSources,
      (id) => props.windowItems.find(({ message }) => message.id === id)?.message,
      operation.signal, (message, expected) => onMessageSaved(projectId, message, expected),
    );
    if (props.projectId !== projectId) return;
    if (record.length) undoRecord.value = record;
    // 即使切换查询取消了后续操作，也保存已确认写入对应的旧覆盖清理。
    if (request !== operation) { await props.flush(); return; }
    stale.value = new Set([...stale.value, ...skipped]);
    if (skipped.size) toast.warning(`${skipped.size} 条消息的正文、来源或权限已变化，已跳过`);
    if (error) toast.error(error);
    if (!count) return;
    // 仅重算已确认写回的消息；后续替换使用服务端返回的新版本。
    snapshot.value = { ...scanned, results: scanned.results.flatMap((result) => {
      const source = writes[result.messageId];
      if (source === undefined) return [result];
      const matches = findTextMatches(source, query.value);
      return matches.length ? [{ ...result, original: source, source, matches,
        version: record.find((item) => item.messageId === result.messageId)!.version }] : [];
    }) };
    active.value = -1;
    if (!all && hits.value.length) showHit(Math.min(oldIndex, hits.value.length - 1));
    await saveResult(operation, `已替换并写回原消息 ${count} 处正文`);
  } catch (error) {
    if (request === operation) toast.error(describeLogError(error, '替换失败，请检查原消息与草稿状态'));
  } finally {
    if (request === operation) {
      request = null;
      phase.value = null;
    }
  }
};

const undo = async () => {
  const record = undoRecord.value;
  if (!record || !props.canReplace || phase.value) return;
  if (!props.canEditDraft && record.some((item) => item.beforeOverride !== undefined)) {
    toast.warning('本次撤销涉及旧工程正文，需要草稿编辑权限');
    return;
  }
  const projectId = props.projectId;
  const operation = request = new AbortController();
  phase.value = 'undo';
  try {
    if (!(await props.flush()) || request !== operation) return;
    const { restored, skipped, error, remaining } = await undoLogReplacements(record, () => props.draft, operation.signal,
      (message, expected, restoredOverride) => onMessageSaved(projectId, message, expected, restoredOverride));
    if (props.projectId !== projectId) return;
    undoRecord.value = remaining.length ? remaining : null;
    if (error && request === operation) toast.error(error);
    await saveResult(operation, `已撤销 ${restored} 条原消息的替换${skipped ? `，${skipped} 条已变化或无权限，已跳过` : ''}`);
    if (request === operation) void scan();
  } catch (error) {
    if (request === operation) toast.error(describeLogError(error, '撤销失败'));
  } finally {
    if (request === operation) { request = null; phase.value = null; }
  }
};
</script>

<template>
  <div class="log-find">
    <n-input
      v-model:value="query"
      class="log-find__input"
      size="small"
      clearable
      :disabled="phase === 'replace' || phase === 'undo'"
      placeholder="查找正文"
      aria-label="查找 LOG 正文"
      @keydown.enter="onFindEnter"
    >
      <template #prefix><n-icon :component="Search" size="15" /></template>
      <template #suffix>
        <span class="log-find__status" role="status" :title="scanError || (snapshot ? summary : undefined)">{{ status }}</span>
      </template>
    </n-input>
    <n-button size="tiny" quaternary :disabled="!hits.length || !!phase" title="上一处" aria-label="上一处" @click="navigate(-1)">
      <template #icon><n-icon :component="ChevronUp" /></template>
    </n-button>
    <n-button size="tiny" quaternary :disabled="!hits.length || !!phase" title="下一处" aria-label="下一处" @click="navigate(1)">
      <template #icon><n-icon :component="ChevronDown" /></template>
    </n-button>
    <n-popover v-model:show="expanded" placement="bottom-end" trigger="click">
      <template #trigger>
        <n-button size="tiny" quaternary :aria-expanded="expanded" title="替换与查找范围" aria-label="替换与查找范围">
          <template #icon><n-icon :component="Replace" /></template>
        </n-button>
      </template>
      <div class="log-find__options">
        <div class="log-find__row">
          <n-input v-model:value="replacement" size="small" placeholder="替换为" aria-label="替换为" :disabled="!canReplace || !!phase" />
          <n-button size="small" :disabled="!canReplace || !!phase || !current || !isReplaceable(current)" @click="apply(false)">替换</n-button>
          <n-button size="small" :disabled="!canReplace || !!phase || !replaceableCount" @click="apply(true)">全部替换</n-button>
        </div>
        <div class="log-find__row">
          <n-select v-model:value="scope" size="small" :options="scopeOptions" :disabled="!!phase" aria-label="查找范围" />
          <n-button v-if="scanError || stale.size" size="small" :disabled="!!phase" title="重新查找" aria-label="重新查找" @click="scan()">
            <template #icon><n-icon :component="Refresh" /></template>
          </n-button>
          <n-button v-if="undoRecord" size="small" :disabled="!canReplace || !!phase" @click="undo">撤销替换</n-button>
        </div>
        <span v-if="scanError || snapshot" class="log-find__status" role="status">{{ scanError || summary }}</span>
        <span class="log-find__status">替换会写回聊天原消息；无编辑权限的消息会跳过。</span>
      </div>
    </n-popover>
  </div>
</template>

<style scoped>
.log-find,
.log-find__row {
  display: flex;
  align-items: center;
  gap: 4px;
}

.log-find {
  flex: 1 0 206px;
}

.log-find__input {
  min-width: 124px;
}

.log-find__status {
  font-size: 12px;
}

.log-find__input .log-find__status {
  white-space: nowrap;
}

.log-find__options {
  display: grid;
  gap: 6px;
  width: min(330px, calc(100vw - 36px));
}

.log-find__row > :first-child {
  flex: 1;
  min-width: 0;
}
</style>
