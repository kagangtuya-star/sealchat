<script setup lang="ts">
// 校对审阅中心：扫描、筛选、逐处或批量决定、应用与撤销，并驱动画布上的审阅层
// （原文标记、页边批注卡片、审阅工具条）。采纳后只写入当前 LOG 草稿的正文覆盖（messageContentOverrides），
// 不修改聊天原消息。本组件、zhlint 与批注映射都按需加载；扫描与应用绑定工程，切换工程或卸载即作废。
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { useElementSize, useEventListener } from '@vueuse/core';
import { useMessage } from 'naive-ui';
import { Adjustments, ArrowBackUp, Check, Checks, ChevronDown, Eye, Focus2, Refresh, X } from '@vicons/tabler';
import { buildAIPolishDiffTokens } from '@/services/ai/ai-polish-diff';
import { describeLogError, fetchLogMessageContext, fetchLogSectionMessages, isLogError } from '@/services/log/logApi';
import {
  PROOF_RULE_LABELS,
  previewProofEdits,
  proofreadContent,
  type ProofIssue,
  type ProofLevel,
  type ProofLint,
  type ProofRule,
} from '@/services/log/proofread/logProofread';
import { buildProofNode } from '@/services/log/proofread/logProofMarks';
import { loadZhlintLint } from '@/services/log/proofread/zhlintRunner';
import { parseAIProofResponse, prepareAIProofBatchesAsync, runAIProofBatches, summarizeAIProofFailures, type AIProofBatch } from '@/services/log/proofread/logAIProofread';
import { useAIStore } from '@/stores/ai';
import { resolveLocalAIProfileForFeature } from '@/services/ai/local-ai';
import type { AIRunSource } from '@/types';
import { resolveDesignSpeakerName } from '@/services/log/logRenderer';
import type { FrameEvent, FrameProofNode, FrameProofView } from '@/services/log/pagination/logFrameHost';
import type { LogDesignMessage, LogDraft, LogSection } from '@/types/log';
import {
  commitProofApply,
  decideProofIssues,
  planProofApply,
  proofDraftSizeError,
  proofDecisionKey,
  toProofResult,
  undoProofApply,
  type ProofDecision,
  type ProofMessageResult,
  type ProofMessageMeta,
  type ProofUndoRecord,
} from '../logProofreadBatch';
import { chapterTitle, chapterViews } from '../logChapterOps';
import LogProofReviewBar from './LogProofReviewBar.vue';

const props = defineProps<{
  projectId: string;
  worldId: string;
  draft: LogDraft;
  update: (mutate: (next: LogDraft) => void) => boolean;
  flush: () => Promise<boolean>;
  saveError: string;
  canEditDraft: boolean;
  supportsContentOverrides: boolean;
  sections: LogSection[];
  chapterSectionIds: string[];
  windowSectionId: string;
  windowItems: LogDesignMessage[];
  windowComplete: boolean;
  windowSourceVersion: number | null;
  selectedIds: string[];
  /** 画布元素：审阅工具条挂在画布上，切换到其他设计面板时仍然可用。 */
  canvasEl: HTMLElement | null;
  narrow: boolean;
  /** 预览为设计态（非发布效果）时才显示审阅层。 */
  design: boolean;
  focused: boolean;
}>();
const emit = defineEmits<{
  (e: 'locate', messageId: string, sectionId: string): void;
  (e: 'marks', nodes: FrameProofNode[] | null): void;
  (e: 'view', view: FrameProofView | null): void;
  (e: 'reveal', key: string, messageId: string, sectionId: string): void;
  (e: 'focus', on: boolean): void;
}>();

const toast = useMessage();
const ai = useAIStore();
const mode = ref<'basic' | 'ai'>('basic');
const aiBatches = shallowRef<AIProofBatch[]>([]);
const aiEstimate = ref({ messageCount: 0, editableChars: 0 });
const aiPrepared = ref(false);
const aiStarted = ref(false);
const aiInvalid = ref(false);
const aiSource = ref<AIRunSource>('platform');
const aiDestination = ref('');
const aiWarning = ref('');
let aiSnapshots = new Map<string, ProofMessageMeta>();
let aiDraftSources = '';
let aiSettingsSnapshot = '';
const sourceOptions = computed(() => [{ value: 'platform', label: '平台 AI', disabled: ai.getFeatureCapability('log_proofread')?.userCustomOnly === true }, { value: 'user', label: '个人 API' }]);
const aiStats = computed(() => ({
  total: aiBatches.value.length,
  success: aiBatches.value.filter((b) => b.status === 'succeeded').length,
  failed: aiBatches.value.filter((b) => b.status === 'failed').length,
  remaining: aiBatches.value.filter((b) => b.status === 'pending' || b.status === 'cancelled').length,
  discarded: aiBatches.value.reduce((n, b) => n + Object.values(b.discarded || {}).reduce((a, c) => a + c, 0), 0),
}));
const aiFailures = computed(() => summarizeAIProofFailures(aiBatches.value));
const PAGE_LIMIT = 100;
const CHECK_BATCH = 25;
const UNDO_LIMIT = 10;
const ROW_HEIGHT = 34;

// ---- 范围 ----
type Scope = 'project' | 'chapter' | 'selected';
const scope = ref<Scope>('chapter');
const speakerKeys = ref<string[]>([]);
const timeRange = ref<[number, number] | null>(null);
const moreFilters = ref(false);

const speakerKeyOf = (it: LogDesignMessage) => (it.message.identity?.id ? `i:${it.message.identity.id}` : `u:${it.message.user?.id || ''}`);
/** 角色选项来自当前片段与之前扫描读取过的消息；未出现过的角色无法预先列出。 */
const knownMessages = shallowRef<Map<string, LogDesignMessage>>(new Map());
const speakerOptions = computed(() => {
  const map = new Map<string, string>();
  const add = (it: LogDesignMessage) => {
    const key = speakerKeyOf(it);
    if (key.length > 2 && !map.has(key)) map.set(key, resolveDesignSpeakerName(it.message) || '未命名');
  };
  props.windowItems.forEach(add);
  knownMessages.value.forEach(add);
  return Array.from(map, ([value, label]) => ({ value, label }));
});
const chapterLabels = computed(() => {
  const labels = new Map<string, string>();
  chapterViews(props.draft).forEach((view, index) => view.sections.forEach((s) => labels.set(s.id, chapterTitle(view, index))));
  return labels;
});
const chapterLabel = computed(() => chapterLabels.value.get(props.chapterSectionIds[0] || '') || '当前章节');
const scopeOptions = computed(() => [
  { value: 'chapter', label: '当前章节', disabled: false },
  { value: 'project', label: '整个工程', disabled: false },
  { value: 'selected', label: `已选 ${props.selectedIds.length}`, disabled: !props.selectedIds.length },
]);
const scopeText = computed(() => (scope.value === 'project' ? '整个工程' : scope.value === 'selected' ? '已选消息' : chapterLabel.value));
watch(() => props.selectedIds.length, (n) => {
  if (!n && scope.value === 'selected') scope.value = 'chapter';
});

// ---- 扫描状态（绑定工程；seq 递增即作废在途扫描） ----
const results = shallowRef<ProofMessageResult[]>([]);
const decisions = ref<Record<string, ProofDecision>>({});
const scannedCount = ref(0);
const scannedProject = ref('');
const scannedScope = ref('');
const scannedSourceVersions = shallowRef<Record<string, number>>({});
const lintMode = ref<'zhlint' | 'local' | ''>('');
const phase = ref<'' | 'load' | 'check' | 'ai' | 'apply'>('');
const progress = ref({ done: 0, total: 0 });
const scanError = ref('');
const staleIds = ref<Set<string>>(new Set());
/** 扫描顺序：撤销应用后把结果放回原位置。 */
let resultOrder = new Map<string, number>();
let seq = 0;
let controller: AbortController | null = null;

const busy = computed(() => phase.value !== '');
const scanned = computed(() => scannedProject.value === props.projectId);
const setupOpen = ref(false);
const showSetup = computed(() => !scanned.value || setupOpen.value);
const yieldFrame = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

const cancel = () => {
  seq++;
  controller?.abort();
  controller = null;
  phase.value = '';
  aiBatches.value = aiBatches.value.map((b) => b.status === 'running' ? { ...b, status: 'cancelled' } : b);
};
const reset = () => {
  cancel();
  results.value = [];
  decisions.value = {};
  scannedCount.value = 0;
  scannedProject.value = '';
  scannedSourceVersions.value = {};
  lintMode.value = '';
  scanError.value = '';
  staleIds.value = new Set();
  activeKey.value = null;
  resultOrder = new Map();
  aiBatches.value = [];
  aiPrepared.value = false;
  aiStarted.value = false;
  aiInvalid.value = false;
  aiWarning.value = '';
  aiSnapshots = new Map();
};
watch(mode, () => {
  reset();
  sourceFilter.value = null;
  levelFilter.value = null;
  ruleFilter.value = null;
}, { flush: 'sync' });

const draftSourceSnapshot = computed(() => JSON.stringify([props.sections.map((s) => [s.id, s.source]), props.draft.messageContentOverrides || {}]));
const draftSourceKey = () => draftSourceSnapshot.value;
const aiSettingsSnapshotValue = computed(() => JSON.stringify([ai.userProfiles, ai.userFeatureBindings, ai.currentSource, ai.resolveEffectiveSource('log_proofread', aiSource.value), ai.getFeatureCapability('log_proofread')]));
const aiSettingsKey = () => aiSettingsSnapshotValue.value;

const prepareAI = async (messages: ProofMessageMeta[], token: number, sourceKey: string, signal: AbortSignal) => {
  await ai.loadCapabilities(props.worldId, props.projectId);
  if (token !== seq) return;
  await ai.loadUserAISettings();
  if (token !== seq) return;
  const sourceById = new Map(messages.map((m) => [m.messageId, m]));
  const verifySnapshot = () => {
    const version = scannedSourceVersions.value[props.windowSectionId];
    if (sourceKey !== draftSourceKey()
      || (version !== undefined && props.windowSourceVersion !== null && version !== props.windowSourceVersion)
      || props.windowItems.some((it) => {
        const snapshot = sourceById.get(it.message.id);
        return snapshot && (it.message.isDeleted || it.message.content !== snapshot.original);
      })) throw new Error('工程正文或来源在准备期间已变化，请重新扫描');
  };
  verifySnapshot();
  const feature = ai.getFeatureCapability('log_proofread');
  if (!feature?.enabled) throw new Error('LOG AI 校对未启用或无权使用，请联系管理员开启此功能');
  aiSource.value = ai.resolveEffectiveSource('log_proofread', ai.currentSource);
  if (aiSource.value === 'user') {
    const { profile, model } = resolveLocalAIProfileForFeature({ featureKey: 'log_proofread', profiles: ai.userProfiles, featureBindings: ai.userFeatureBindings });
    aiDestination.value = `${profile.name || profile.baseUrl} · ${model}`;
  } else aiDestination.value = `${feature.providerNames?.join(' / ') || '平台配置的 AI 服务商'} · ${feature.defaultModel || '默认模型'}`;
  const settingsKey = aiSettingsKey();
  // 在可配置输入上限内，为系统提示词和额外上下文留余量；包装仍由分批器逐批精确计数。
  const inputBudget = Math.min(8000, feature.params?.maxInputChars || 8000) - (feature.defaultPrompt?.length || 0) - 256;
  const prepared = await prepareAIProofBatchesAsync(messages, inputBudget, signal);
  if (token !== seq) return;
  verifySnapshot();
  if (settingsKey !== aiSettingsKey()) throw new Error('AI 来源或配置在准备期间已变化，请重新扫描');
  aiBatches.value = prepared.batches;
  aiEstimate.value = { messageCount: prepared.messageCount, editableChars: prepared.editableChars };
  aiSnapshots = new Map(messages.map((m) => [m.messageId, m]));
  aiDraftSources = sourceKey;
  aiSettingsSnapshot = settingsKey;
  aiPrepared.value = true;
};

const startAI = async (runMode: 'unfinished' | 'failed' = 'unfinished') => {
  if (busy.value || mode.value !== 'ai' || !aiPrepared.value || aiInvalid.value) return;
  if (draftSourceKey() !== aiDraftSources || aiSettingsSnapshot !== aiSettingsKey()) {
    aiInvalid.value = true;
    scanError.value = '工程正文、来源或 AI 配置已变化，请重新扫描';
    return;
  }
  const token = ++seq;
  const projectId = props.projectId;
  const ctrl = new AbortController();
  controller = ctrl;
  phase.value = 'ai';
  aiStarted.value = true;
  const sourceVersions = { ...scannedSourceVersions.value };
  const current = () => token === seq && props.projectId === projectId && !aiInvalid.value && draftSourceKey() === aiDraftSources && aiSettingsSnapshot === aiSettingsKey();
  const changed = () => {
    if (!current()) return;
    aiBatches.value = aiBatches.value.slice();
    progress.value = { done: aiStats.value.success + aiStats.value.failed, total: aiStats.value.total };
  };
  try {
    await runAIProofBatches({
      batches: aiBatches.value, mode: runMode,
      signal: ctrl.signal, isCurrent: current, changed,
      request: async (batch, signal) => {
        const versions = Object.fromEntries(batch.fragments.map((f) => [f.sectionId, sourceVersions[f.sectionId]]));
        const response = await ai.runTask('log_proofread', { projectId, worldId: props.worldId, sourceVersions: versions, input: batch.input, source: aiSource.value }, { signal });
        if (current() && 'warning' in response.data && response.data.warning) aiWarning.value = String(response.data.warning);
        return response.data.result;
      },
      complete: async (batch, raw) => {
        const parsed = parseAIProofResponse(raw, batch, aiSnapshots);
        // 响应到达时再校验来源版本；跨章节不能只依赖当前窗口的缓存。
        for (const sectionId of new Set(batch.fragments.map((f) => f.sectionId))) {
          const page = await fetchLogSectionMessages(projectId, sectionId, null, 1, ctrl.signal);
          if (!current()) return;
          if (page.sourceVersion !== sourceVersions[sectionId]) {
            aiInvalid.value = true;
            scanError.value = '来源快照已失效，保留已完成结果；请重新扫描后继续 AI 校对';
            throw new Error('来源快照已失效');
          }
        }
        if (!current() || ctrl.signal.aborted) return;
        batch.discarded = parsed.discarded;
        const next = results.value.slice();
        for (const incoming of parsed.results) {
          const index = next.findIndex((r) => r.messageId === incoming.messageId);
          if (index < 0) next.push(incoming);
          else {
            const existing = next[index];
            const ids = new Set(existing.issues.map((issue) => issue.id));
            next[index] = { ...existing, issues: [...existing.issues, ...incoming.issues.filter((issue) => !ids.has(issue.id))]
              .sort((a, b) => a.run - b.run || a.start - b.start || a.id.localeCompare(b.id)) };
          }
        }
        results.value = next.sort((a, b) => (resultOrder.get(a.messageId) ?? 0) - (resultOrder.get(b.messageId) ?? 0));
      },
    });
  } finally {
    if (token === seq) { phase.value = ''; controller = null; aiBatches.value = aiBatches.value.slice(); }
  }
};

const canUseBasicWindow = () => mode.value === 'basic'
  && scope.value === 'chapter'
  && props.chapterSectionIds.length === 1
  && props.chapterSectionIds[0] === props.windowSectionId
  && props.windowComplete
  && props.windowSourceVersion !== null;

/** 章节/工程始终从片段开头读取完整快照；画布窗口只用于明确选中的消息。 */
const collectMessages = async (token: number, signal: AbortSignal): Promise<{
  items: Array<{ sectionId: string; item: LogDesignMessage }>;
  sourceVersions: Record<string, number>;
} | null> => {
  if (scope.value === 'selected') {
    if (props.windowSourceVersion === null) throw new Error('来源消息尚未加载完成，请稍后扫描');
    const picked = new Set(props.selectedIds);
    return {
      items: props.windowItems.filter((it) => picked.has(it.message.id)).map((item) => ({ sectionId: props.windowSectionId, item })),
      sourceVersions: { [props.windowSectionId]: props.windowSourceVersion },
    };
  }
  if (canUseBasicWindow()) {
    return {
      items: props.windowItems.map((item) => ({ sectionId: props.windowSectionId, item })),
      sourceVersions: { [props.windowSectionId]: props.windowSourceVersion! },
    };
  }
  const ids = scope.value === 'project' ? props.sections.map((s) => s.id) : props.chapterSectionIds.slice();
  const projectId = props.projectId;
  const out: Array<{ sectionId: string; item: LogDesignMessage }> = [];
  const sourceVersions: Record<string, number> = {};
  progress.value = { done: 0, total: ids.length };
  for (const sectionId of ids) {
    let cursor: string | null = null;
    let version: number | null = null;
    do {
      if (token !== seq || signal.aborted) return null;
      const page = await fetchLogSectionMessages(projectId, sectionId, cursor, PAGE_LIMIT, signal);
      if (token !== seq || signal.aborted || props.projectId !== projectId) return null;
      if (version !== null && page.sourceVersion !== version) throw new Error('来源消息在读取过程中发生变化，请重新扫描');
      version = page.sourceVersion;
      out.push(...page.items.map((item) => ({ sectionId, item })));
      cursor = page.nextCursor;
    } while (cursor !== null);
    if (version !== null) sourceVersions[sectionId] = version;
    progress.value = { done: progress.value.done + 1, total: ids.length };
  }
  return { items: out, sourceVersions };
};

const passesFilter = (it: LogDesignMessage) => {
  if (it.message.isDeleted) return false;
  if (speakerKeys.value.length && !speakerKeys.value.includes(speakerKeyOf(it))) return false;
  const range = timeRange.value;
  return !range || (it.message.createdAt >= range[0] && it.message.createdAt <= range[1]);
};

const scan = async () => {
  reset();
  const token = seq;
  const projectId = props.projectId;
  const ctrl = new AbortController();
  controller = ctrl;
  phase.value = 'load';
  try {
    // AI 与非完整/跨片段基础校对按已保存草稿读取完整快照；完整单章节基础校对直接复用本地窗口。
    if (scope.value !== 'selected' && !canUseBasicWindow() && !(await props.flush())) throw new Error(props.saveError || '草稿尚未保存成功，请稍后重试');
    if (token !== seq) return;
    const sourceKey = draftSourceKey();
    let lint: ProofLint | null = null;
    if (mode.value === 'basic') {
      try {
        lint = await loadZhlintLint();
        lintMode.value = 'zhlint';
      } catch {
        lintMode.value = 'local';
      }
    }
    if (token !== seq) return;
    const collected = await collectMessages(token, ctrl.signal);
    if (!collected || token !== seq) return;
    const known = new Map(knownMessages.value);
    collected.items.forEach(({ item }) => known.set(item.message.id, item));
    knownMessages.value = known;
    const seen = new Set<string>();
    const targets = collected.items.filter(({ item }) => {
      if (!passesFilter(item) || seen.has(item.message.id)) return false;
      seen.add(item.message.id);
      return true;
    });
    phase.value = 'check';
    progress.value = { done: 0, total: targets.length };
    const found: ProofMessageResult[] = [];
    const snapshots: ProofMessageMeta[] = [];
    const overrides = props.draft.messageContentOverrides || {};
    for (let i = 0; i < targets.length; i += CHECK_BATCH) {
      for (const { sectionId, item } of targets.slice(i, i + CHECK_BATCH)) {
        const original = item.message.content || '';
        const source = overrides[item.message.id] ?? original;
        const meta = { messageId: item.message.id, sectionId, speaker: resolveDesignSpeakerName(item.message) || '未命名', createdAt: item.message.createdAt, source, original };
        snapshots.push(meta);
        if (mode.value === 'basic') {
          const issues = proofreadContent(source, lint);
          if (issues.length) found.push(toProofResult(meta, issues));
        }
      }
      progress.value = { done: Math.min(targets.length, i + CHECK_BATCH), total: targets.length };
      await yieldFrame();
      if (token !== seq) return;
    }
    if (props.projectId !== projectId) return;
    if (sourceKey !== draftSourceKey()) throw new Error('工程正文或来源在读取期间已变化，请重新扫描');
    resultOrder = new Map(snapshots.map((r, index) => [r.messageId, index]));
    results.value = found;
    scannedCount.value = targets.length;
    scannedProject.value = projectId;
    scannedScope.value = scopeText.value;
    scannedSourceVersions.value = collected.sourceVersions;
    setupOpen.value = false;
    markup.value = true;
    // 直接进入审阅：选中当前排版窗口中的第一处批注（不为此切换章节）。
    const first = found.find((r) => r.sectionId === props.windowSectionId && props.windowItems.some((it) => it.message.id === r.messageId));
    if (first) activate(proofDecisionKey(first.messageId, first.issues[0].id));
    if (mode.value === 'ai') await prepareAI(snapshots, token, sourceKey, ctrl.signal);
  } catch (error) {
    if (token !== seq || ctrl.signal.aborted) return;
    scanError.value = error instanceof Error && !('code' in error) ? error.message : describeLogError(error, '扫描失败');
  } finally {
    if (token === seq) {
      phase.value = '';
      controller = null;
    }
  }
};

// ---- 审阅条目、决定与筛选 ----
interface ProofEntry { key: string; order: number; r: ProofMessageResult; i: ProofIssue }
type StatusFilter = 'all' | 'pending' | 'accepted' | 'rejected';

const entries = computed(() => {
  const out: ProofEntry[] = [];
  for (const r of results.value) {
    for (const i of r.issues) out.push({ key: proofDecisionKey(r.messageId, i.id), order: out.length, r, i });
  }
  return out;
});
const entryByKey = computed(() => new Map(entries.value.map((e) => [e.key, e])));
const stateOf = (key: string): ProofDecision | 'pending' => decisions.value[key] || 'pending';
const decisionOf = (r: ProofMessageResult, i: ProofIssue) => decisions.value[proofDecisionKey(r.messageId, i.id)];

const counts = computed(() => {
  const c = { all: 0, pending: 0, accepted: 0, rejected: 0, safePending: 0 };
  const rules = new Map<ProofRule, number>();
  for (const e of entries.value) {
    const s = stateOf(e.key);
    c.all++;
    c[s]++;
    if (s === 'pending' && e.i.level === 'safe') c.safePending++;
    rules.set(e.i.rule, (rules.get(e.i.rule) || 0) + 1);
  }
  return { ...c, decided: c.accepted + c.rejected, rules: Array.from(rules) };
});
const acceptedCount = computed(() => counts.value.accepted);

const statusFilter = ref<StatusFilter>('all');
const levelFilter = ref<ProofLevel | null>(null);
const ruleFilter = ref<ProofRule | null>(null);
const sourceFilter = ref<'local' | 'ai' | null>(null);
const proofSourceOptions = [{ value: null, label: '全部来源' }, { value: 'local', label: '本地规则' }, { value: 'ai', label: 'AI 建议' }];
const statusTabs = computed(() => [
  { value: 'all' as const, label: '全部', count: counts.value.all },
  { value: 'pending' as const, label: '待审', count: counts.value.pending },
  { value: 'accepted' as const, label: '已采纳', count: counts.value.accepted },
  { value: 'rejected' as const, label: '已放弃', count: counts.value.rejected },
]);
const ruleOptions = computed(() => [
  { value: null, label: '全部规则' },
  ...counts.value.rules.map(([rule, n]) => ({ value: rule, label: `${PROOF_RULE_LABELS[rule]} · ${n}` })),
]);
const matches = (e: ProofEntry) => {
  if (sourceFilter.value === 'ai' && e.i.source !== 'ai') return false;
  if (sourceFilter.value === 'local' && e.i.source === 'ai') return false;
  if (statusFilter.value !== 'all' && stateOf(e.key) !== statusFilter.value) return false;
  if (levelFilter.value && e.i.level !== levelFilter.value) return false;
  return !ruleFilter.value || e.i.rule === ruleFilter.value;
};
const filtered = computed(() => entries.value.filter(matches));
const filterActive = computed(() => statusFilter.value !== 'all' || !!levelFilter.value || !!ruleFilter.value || !!sourceFilter.value);

const setDecisionKey = (key: string, value: ProofDecision | null) => {
  const outcome = decideProofIssues(results.value, decisions.value, [key], value);
  decisions.value = outcome.decisions;
  if (outcome.conflicts) toast.warning('该建议与已采纳的修改冲突，请先撤回另一项决定再选择');
};
/** 列表中的 ✓ / ✕ 为切换：再次点击撤回决定。 */
const toggleDecision = (key: string, value: ProofDecision) => setDecisionKey(key, stateOf(key) === value ? null : value);
const decideAll = (value: ProofDecision | null, list: ProofEntry[]) => {
  const outcome = decideProofIssues(results.value, decisions.value, list.map((e) => e.key), value);
  decisions.value = outcome.decisions;
  if (outcome.conflicts) toast.warning(`${outcome.conflicts} 处互斥建议未批量采纳，请逐项选择`);
};
const acceptSafe = () => decideAll('accepted', entries.value.filter((e) => e.i.level === 'safe' && stateOf(e.key) === 'pending'));
const decideFiltered = (value: ProofDecision | null) => decideAll(value, filtered.value);
const decideMessage = (r: ProofMessageResult) => decideAll('accepted', entries.value.filter((e) => e.r === r));

// ---- 当前批注与连续导航 ----
const activeKey = ref<string | null>(null);
const activeEntry = computed(() => (activeKey.value ? entryByKey.value.get(activeKey.value) || null : null));
const position = computed(() => {
  const key = activeKey.value;
  const list = filtered.value;
  if (!key) return { index: 0, total: list.length };
  const at = list.findIndex((e) => e.key === key);
  return { index: at >= 0 ? at + 1 : 0, total: list.length };
});

const activate = (key: string, reveal = true) => {
  const entry = entryByKey.value.get(key);
  if (!entry) return;
  activeKey.value = key;
  ensureRowVisible(key);
  if (reveal) emit('reveal', key, entry.r.messageId, entry.r.sectionId);
};

/** 在当前筛选序列中，从 order 向前或向后找第一个满足条件的条目。 */
const seek = (order: number, step: 1 | -1, pick: (e: ProofEntry) => boolean) => {
  const list = filtered.value;
  if (step > 0) return list.find((e) => e.order > order && pick(e)) || null;
  for (let i = list.length - 1; i >= 0; i--) if (list[i].order < order && pick(list[i])) return list[i];
  return null;
};

const nav = (step: 1 | -1) => {
  const current = activeEntry.value;
  const target = seek(current ? current.order : step > 0 ? -1 : Number.MAX_SAFE_INTEGER, step, () => true);
  if (target) activate(target.key);
  else if (filtered.value.length) toast.info(step > 0 ? '已是最后一处' : '已是第一处');
};

/** 决定后自动前往下一处待审批注；后面没有时回到最前面的待审批注。 */
const advance = () => {
  const current = activeEntry.value;
  if (!current) return;
  const pending = (e: ProofEntry) => stateOf(e.key) === 'pending';
  const target = seek(current.order, 1, pending) || seek(-1, 1, pending);
  if (target && target.key !== current.key) activate(target.key);
  else if (!counts.value.pending) toast.success('全部批注已审阅，可以应用已采纳的修改');
};

const decideActive = (value: ProofDecision | null, next = true) => {
  const key = activeKey.value;
  if (!key) return;
  const before = stateOf(key);
  setDecisionKey(key, value);
  if (value && next && before === 'pending') advance();
};

// ---- 审阅层（原文标记与页边批注） ----
const markup = ref(true);
const reviewing = computed(() => markup.value && props.design && scanned.value && results.value.length > 0);
const marksNodes = shallowRef<FrameProofNode[]>([]);
const missedKeys = shallowRef<Set<string>>(new Set());
const markCache = new Map<string, { content: string; issues: ProofIssue[]; reason: string | null; node: FrameProofNode }>();
let marksToken = 0;
let marksTimer: ReturnType<typeof setTimeout> | null = null;

/** 只为当前排版窗口中的消息计算批注位置；正文与批注不变时复用缓存。 */
const rebuildMarks = async () => {
  const token = ++marksToken;
  if (!reviewing.value) {
    marksNodes.value = [];
    return;
  }
  const byId = new Map(results.value.filter((r) => r.sectionId === props.windowSectionId).map((r) => [r.messageId, r]));
  const overrides = props.draft.messageContentOverrides || {};
  const out: FrameProofNode[] = [];
  let started = performance.now();
  for (const item of props.windowItems) {
    const r = byId.get(item.message.id);
    if (!r) continue;
    const content = overrides[r.messageId] ?? item.message.content ?? '';
    const reason = item.presentation?.embed ? '该消息以嵌入卡片呈现，按原文显示'
      : content !== r.source ? '正文在扫描后已变化，建议重新扫描' : null;
    let cached = markCache.get(r.messageId);
    if (!cached || cached.content !== content || cached.issues !== r.issues || cached.reason !== reason) {
      cached = { content, issues: r.issues, reason, node: buildProofNode(r.messageId, content, r.issues, (i) => proofDecisionKey(r.messageId, i.id), reason) };
      markCache.set(r.messageId, cached);
    }
    out.push(cached.node);
    if (performance.now() - started > 12) {
      await yieldFrame();
      if (token !== marksToken) return;
      started = performance.now();
    }
  }
  if (token === marksToken) marksNodes.value = out;
};
watch([reviewing, results, () => props.windowItems, () => props.windowSectionId, () => props.draft.messageContentOverrides], () => {
  if (marksTimer) clearTimeout(marksTimer);
  marksTimer = setTimeout(() => {
    marksTimer = null;
    void rebuildMarks();
  }, 60);
}, { immediate: true });
watch([marksNodes, reviewing], () => emit('marks', reviewing.value && marksNodes.value.length ? marksNodes.value : null), { immediate: true });

/** 当前窗口中无法精确定位的批注（外层映射降级 + iframe 排版核对失败）。 */
const unmappedKeys = computed(() => {
  const out = new Set(missedKeys.value);
  for (const n of marksNodes.value) for (const it of n.items) if (it.start < 0) out.add(it.key);
  return out;
});

const view = computed<FrameProofView | null>(() => {
  if (!reviewing.value) return null;
  const states: Record<string, ProofDecision> = {};
  for (const [key, value] of Object.entries(decisions.value)) states[key] = value;
  return { active: activeKey.value, states, position: activeKey.value ? position.value : null };
});
watch(view, (v) => emit('view', v), { immediate: true });

const toggleMarkup = () => {
  markup.value = !markup.value;
  if (!markup.value && props.focused) emit('focus', false);
};

/** 画布审阅层发回的事件（由工作台转发）。 */
const onFrameEvent = (event: FrameEvent) => {
  switch (event.type) {
    case 'proofPick':
      activate(event.key, false);
      break;
    case 'proofDecide': {
      if (activeKey.value !== event.key) activeKey.value = event.key;
      decideActive(event.decision, true);
      break;
    }
    case 'proofNav':
      nav(event.step);
      break;
    case 'proofMapped':
      missedKeys.value = new Set(event.missed);
      break;
    default:
      break;
  }
};

/** 审阅快捷键；返回 true 表示已处理。 */
const handleKey = (key: string) => {
  if (!reviewing.value) return false;
  switch (key) {
    case 'j': nav(1); return true;
    case 'k': nav(-1); return true;
    case 'a': if (activeKey.value) decideActive('accepted'); else nav(1); return true;
    case 'r': if (activeKey.value) decideActive('rejected'); else nav(1); return true;
    case 'u': decideActive(null, false); return true;
    default: return false;
  }
};
/** Esc：先收起当前批注卡片。 */
const escape = () => {
  if (!reviewing.value || !activeKey.value) return false;
  activeKey.value = null;
  return true;
};
const isTyping = (target: EventTarget | null) => {
  const el = target instanceof HTMLElement ? target : null;
  return !!el && (el.isContentEditable || !!el.closest('input, textarea, select, button, a[href], [contenteditable], [role="textbox"], [role="dialog"], [role="combobox"], [role="menu"], [role="listbox"], .n-modal, .n-drawer'));
};
useEventListener(window, 'keydown', (e: KeyboardEvent) => {
  if (e.ctrlKey || e.metaKey || e.altKey || e.defaultPrevented || isTyping(e.target)) return;
  if (handleKey(e.key.toLowerCase())) e.preventDefault();
});

defineExpose({ onFrameEvent, handleKey, escape });

// ---- 列表（虚拟滚动：标题行与批注行等高） ----
type Row =
  | { key: string; kind: 'section'; label: string }
  | { key: string; kind: 'head'; r: ProofMessageResult; count: number }
  | { key: string; kind: 'issue'; e: ProofEntry };
const multiSection = computed(() => new Set(results.value.map((r) => r.sectionId)).size > 1);
const rows = computed(() => {
  const out: Row[] = [];
  let lastSection = '';
  let head = null as Extract<Row, { kind: 'head' }> | null;
  for (const e of filtered.value) {
    if (multiSection.value && e.r.sectionId !== lastSection) {
      lastSection = e.r.sectionId;
      out.push({ key: `s:${e.r.sectionId}`, kind: 'section', label: chapterLabels.value.get(e.r.sectionId) || '其他片段' });
    }
    if (head?.r !== e.r) {
      head = { key: `h:${e.r.messageId}`, kind: 'head', r: e.r, count: 0 };
      out.push(head);
    }
    head.count++;
    out.push({ key: e.key, kind: 'issue', e });
  }
  return out;
});
const listEl = ref<HTMLElement | null>(null);
const listRef = ref<{ scrollTo: (options: { top: number; behavior?: ScrollBehavior }) => void } | null>(null);
const listScrollTop = ref(0);
const { height: listHeight } = useElementSize(listEl);
const ensureRowVisible = (key: string) => {
  const index = rows.value.findIndex((row) => row.key === key);
  if (index < 0 || !listHeight.value) return;
  const top = index * ROW_HEIGHT;
  if (top >= listScrollTop.value && top + ROW_HEIGHT <= listScrollTop.value + listHeight.value) return;
  listRef.value?.scrollTo({ top: Math.max(0, top - listHeight.value / 3) });
};
const onListScroll = (e: Event) => {
  listScrollTop.value = (e.target as HTMLElement).scrollTop;
};
const showText = (s: string) => s.replace(/ /g, '␣').replace(/\u00a0/g, '⍽').replace(/\t/g, '⇥');
const formatTime = (ms: number) => (Number.isFinite(ms) ? new Date(ms).toLocaleString(undefined, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '');
const previewTokens = (r: ProofMessageResult) => {
  const edits = r.issues.filter((i) => decisionOf(r, i) === 'accepted');
  const { before, after } = previewProofEdits(r.source, edits);
  return buildAIPolishDiffTokens(before, after);
};

// ---- 应用与撤销 ----
interface UndoEntry { projectId: string; mode: 'basic' | 'ai'; record: ProofUndoRecord; count: number; results: ProofMessageResult[] }
const undoStack = ref<UndoEntry[]>([]);

const currentOriginal = (id: string) => props.windowItems.find((it) => it.message.id === id)?.message.content;
const canApply = computed(() => props.canEditDraft && props.supportsContentOverrides);

const apply = async () => {
  if (!props.canEditDraft || !props.supportsContentOverrides || busy.value || scannedProject.value !== props.projectId) return;
  const token = ++seq;
  const projectId = props.projectId;
  const list = results.value;
  const ctrl = new AbortController();
  controller = ctrl;
  phase.value = 'apply';
  progress.value = { done: 0, total: list.length };
  const writes: Record<string, string> = {};
  const stale: string[] = [];
  const accepted = (r: ProofMessageResult) => r.issues.some((i) => decisions.value[proofDecisionKey(r.messageId, i.id)] === 'accepted');
  try {
    // 来源版本是频道级的：其他消息更新不应误判当前校对目标已变化。
    // 仅版本不同的片段再按目标消息核对原文；跨章节不能依赖当前窗口缓存。
    const changedMessages = new Set<string>();
    const verifiedOriginals = new Map<string, string>();
    for (const sectionId of new Set(list.filter(accepted).map((r) => r.sectionId))) {
      const page = await fetchLogSectionMessages(projectId, sectionId, null, 1, ctrl.signal);
      if (token !== seq || props.projectId !== projectId) return;
      if (scannedSourceVersions.value[sectionId] === page.sourceVersion) continue;
      for (const result of list.filter((r) => r.sectionId === sectionId && accepted(r))) {
        try {
          const context = await fetchLogMessageContext(projectId, sectionId, result.messageId, ctrl.signal);
          if (token !== seq || props.projectId !== projectId) return;
          const message = context.items.find((item) => item.message.id === result.messageId)?.message;
          if (!message || message.isDeleted || message.content !== result.original) {
            changedMessages.add(result.messageId);
          } else {
            verifiedOriginals.set(result.messageId, message.content);
          }
        } catch (error) {
          if (ctrl.signal.aborted || token !== seq) return;
          if (!isLogError(error, 'NOT_FOUND')) throw error;
          changedMessages.add(result.messageId);
        }
      }
    }
    for (let i = 0; i < list.length; i += CHECK_BATCH) {
      const current = list.slice(i, i + CHECK_BATCH);
      stale.push(...current.filter((r) => changedMessages.has(r.messageId) && accepted(r)).map((r) => r.messageId));
      const plan = planProofApply(
        current.filter((r) => !changedMessages.has(r.messageId)),
        decisions.value,
        (id) => props.draft.messageContentOverrides?.[id],
        (id) => verifiedOriginals.get(id) ?? currentOriginal(id),
      );
      Object.assign(writes, plan.writes);
      stale.push(...plan.stale);
      progress.value = { done: Math.min(list.length, i + CHECK_BATCH), total: list.length };
      await yieldFrame();
      if (token !== seq || props.projectId !== projectId) return;
    }
    // 上面的分页校验与让出主线程期间，覆盖正文或审核决定仍可能被修改。
    // 在同步 mutation 前重算实际写入，避免旧计划覆盖用户刚编辑的草稿。
    const finalPlan = planProofApply(
      list.filter((r) => Object.prototype.hasOwnProperty.call(writes, r.messageId)),
      decisions.value,
      (id) => props.draft.messageContentOverrides?.[id],
      (id) => currentOriginal(id) ?? verifiedOriginals.get(id),
    );
    for (const id of Object.keys(writes)) delete writes[id];
    Object.assign(writes, finalPlan.writes);
    stale.push(...finalPlan.stale);
    const originals = new Map(list.map((r) => [r.messageId, r.original]));
    const count = Object.keys(writes).length;
    const done = new Set(Object.keys(writes));
    if (count) {
      const candidate = { ...props.draft };
      const record: ProofUndoRecord = commitProofApply(candidate, writes, (id) => originals.get(id));
      const sizeError = proofDraftSizeError(candidate);
      if (sizeError) {
        toast.error(sizeError);
        return;
      }
      if (!props.update((next) => {
        if (candidate.messageContentOverrides) next.messageContentOverrides = candidate.messageContentOverrides;
        else delete next.messageContentOverrides;
      })) {
        toast.error('草稿当前不可写入（可能存在冲突），未应用任何修改');
        return;
      }
      undoStack.value = [{ projectId, mode: mode.value, record, count, results: list.filter((r) => done.has(r.messageId)) }, ...undoStack.value].slice(0, UNDO_LIMIT);
    }
    // 已写入的消息离开审阅列表；当前批注随之移到之后（或之前）仍在列表中的批注。
    const current = activeEntry.value;
    let nextActive = activeKey.value;
    if (current && done.has(current.r.messageId)) {
      const remaining = entries.value.filter((e) => !done.has(e.r.messageId));
      const target = remaining.find((e) => e.order > current.order) || remaining.filter((e) => e.order < current.order).pop();
      nextActive = target?.key ?? null;
    }
    results.value = list.filter((r) => !done.has(r.messageId));
    staleIds.value = new Set([...staleIds.value, ...stale]);
    activeKey.value = nextActive;
    if (count) {
      const saved = await props.flush().catch(() => false);
      if (token !== seq || props.projectId !== projectId) return;
      if (saved) toast.success(`已应用并保存 ${count} 条消息的校对结果（仅当前 LOG 工程）`);
      else toast.error(`已更新 ${count} 条消息的本地草稿，但保存失败：${props.saveError || '请检查草稿保存状态'}`);
    }
    if (stale.length) toast.warning(`${stale.length} 条消息在扫描后已变化，已跳过，请重新扫描`);
    if (!count && !stale.length) toast.info('没有已采纳的修改');
  } catch (error) {
    if (token === seq && !ctrl.signal.aborted) toast.error(describeLogError(error, '校验来源失败，未应用修改'));
  } finally {
    if (token === seq) {
      controller = null;
      phase.value = '';
    }
  }
};

const undoLast = async () => {
  const entry = undoStack.value[0];
  if (!entry || entry.projectId !== props.projectId || !props.canEditDraft || busy.value) return;
  const projectId = props.projectId;
  let outcome = { restored: 0, skipped: 0 };
  let after: Record<string, string> = {};
  if (!props.update((next) => {
    outcome = undoProofApply(next, entry.record);
    after = { ...(next.messageContentOverrides || {}) };
  })) {
    toast.error('草稿当前不可写入，无法撤销');
    return;
  }
  undoStack.value = undoStack.value.slice(1);
  // 正文已回到扫描时的样子的消息重新进入审阅列表，保留原来的采纳决定。
  if (scannedProject.value === props.projectId && entry.mode === mode.value) {
    const back = entry.results.filter((r) => (after[r.messageId] ?? r.original) === r.source);
    if (back.length) {
      const present = new Set(results.value.map((r) => r.messageId));
      results.value = [...results.value, ...back.filter((r) => !present.has(r.messageId))]
        .sort((a, b) => (resultOrder.get(a.messageId) ?? 0) - (resultOrder.get(b.messageId) ?? 0));
    }
  }
  const saved = await props.flush().catch(() => false);
  if (projectId !== props.projectId) return;
  const summary = `已撤销 ${outcome.restored} 条${outcome.skipped ? `，${outcome.skipped} 条在应用后又被修改，保持现状` : ''}`;
  if (saved) toast.success(`${summary}，草稿已保存`);
  else toast.error(`${summary}，但仅更新本地草稿，保存失败：${props.saveError || '请检查草稿保存状态'}`);
};

const overrideCount = computed(() => Object.keys(props.draft.messageContentOverrides || {}).length);
const clearAllOverrides = () => {
  const current = props.draft.messageContentOverrides || {};
  const record: ProofUndoRecord = {};
  for (const [id, before] of Object.entries(current)) record[id] = { before, after: undefined };
  const count = Object.keys(record).length;
  if (!count || !props.canEditDraft) return;
  if (!props.update((next) => { delete next.messageContentOverrides; })) {
    toast.error('草稿当前不可写入');
    return;
  }
  // 还原也是一次可撤销的批量操作：撤销把“无覆盖”恢复为原覆盖值。
  undoStack.value = [{ projectId: props.projectId, mode: mode.value, record, count, results: [] }, ...undoStack.value].slice(0, UNDO_LIMIT);
  toast.success(`已将 ${count} 条消息改回显示原消息`);
};

watch(() => props.projectId, () => {
  reset();
  undoStack.value = [];
  knownMessages.value = new Map();
  staleIds.value = new Set();
  markCache.clear();
  missedKeys.value = new Set();
});
watch([draftSourceKey, aiSettingsKey, () => props.windowItems, () => props.windowSourceVersion], () => {
  if (!aiPrepared.value || aiInvalid.value) return;
  const changed = draftSourceKey() !== aiDraftSources || aiSettingsKey() !== aiSettingsSnapshot
    || props.windowItems.some((it) => {
      const snapshot = aiSnapshots.get(it.message.id);
      return snapshot && (it.message.isDeleted || it.message.content !== snapshot.original);
    });
  const version = scannedSourceVersions.value[props.windowSectionId];
  if (changed || (version !== undefined && props.windowSourceVersion !== null && version !== props.windowSourceVersion)) {
    aiInvalid.value = true;
    if (phase.value === 'ai') cancel();
    scanError.value = '校对来源或配置已变化，保留已完成结果；继续 AI 校对前请重新扫描';
  }
});
watch(results, (list) => {
  const live = new Set(list.map((r) => r.messageId));
  for (const id of markCache.keys()) if (!live.has(id)) markCache.delete(id);
  if (activeKey.value && !entryByKey.value.has(activeKey.value)) activeKey.value = null;
});
onBeforeUnmount(() => {
  cancel();
  marksToken++;
  if (marksTimer) clearTimeout(marksTimer);
  emit('marks', null);
  emit('view', null);
});

const progressPercent = computed(() => (progress.value.total ? Math.round((progress.value.done / progress.value.total) * 100) : 0));
const phaseText = computed(() => ({
  '': '',
  load: scope.value === 'selected' ? '准备中…' : `读取消息 · 片段 ${progress.value.done}/${progress.value.total}`,
  check: `检查中 · ${progress.value.done}/${progress.value.total} 条`,
  ai: `AI 批次 · ${aiStats.value.success} 成功 / ${aiStats.value.failed} 失败 / 共 ${aiStats.value.total} 批`,
  apply: `应用中 · ${progress.value.done}/${progress.value.total} 条`,
}[phase.value]));
const reviewedPercent = computed(() => (counts.value.all ? Math.round((counts.value.decided / counts.value.all) * 100) : 0));

// ---- 画布工具条 ----
const { width: canvasWidth } = useElementSize(computed(() => props.canvasEl));
const compactBar = computed(() => props.narrow || (canvasWidth.value > 0 && canvasWidth.value < 760));
</script>

<template>
  <div class="log-proof">
    <!-- 扫描设置：未扫描时展开；有结果后收起为一行摘要 -->
    <section v-if="showSetup" class="log-proof__setup" aria-label="校对范围">
      <n-select v-model:value="mode" size="small" :disabled="busy" :options="[{ value: 'basic', label: '基础校对（本地）' }, { value: 'ai', label: 'AI 综合校对' }]" aria-label="校对模式" />
      <n-select v-if="mode === 'ai'" :value="ai.resolveEffectiveSource('log_proofread', ai.currentSource)" :options="sourceOptions" size="small" :disabled="busy" aria-label="AI 来源" @update:value="ai.setSource" />
      <div class="log-proof__seg" role="radiogroup" aria-label="校对范围">
        <button
          v-for="o in scopeOptions"
          :key="o.value"
          type="button"
          role="radio"
          class="log-proof__seg-btn"
          :class="{ 'is-on': scope === o.value }"
          :aria-checked="scope === o.value"
          :disabled="o.disabled || busy"
          @click="scope = o.value as Scope"
        >{{ o.label }}</button>
      </div>
      <button type="button" class="log-proof__disclose" :aria-expanded="moreFilters" @click="moreFilters = !moreFilters">
        <n-icon :component="ChevronDown" size="14" :class="{ 'is-open': moreFilters }" />
        限定角色或时间
        <span v-if="speakerKeys.length || timeRange" class="log-proof__dot" aria-label="已设置" />
      </button>
      <div v-if="moreFilters" class="log-proof__filters">
        <n-select v-model:value="speakerKeys" size="small" :options="speakerOptions" multiple filterable clearable max-tag-count="responsive" placeholder="不限角色" :disabled="busy" />
        <n-date-picker v-model:value="timeRange" size="small" type="datetimerange" clearable :disabled="busy" />
      </div>
      <div class="log-proof__setup-actions">
        <n-button type="primary" :loading="phase === 'load' || phase === 'check'" :disabled="busy" @click="scan">
          {{ scanned ? '重新扫描' : '开始校对' }}
        </n-button>
        <n-button v-if="phase === 'load' || phase === 'check'" @click="cancel">取消</n-button>
        <n-button v-else-if="scanned" quaternary @click="setupOpen = false">收起</n-button>
      </div>
      <p v-if="!scanned" class="log-proof__muted">{{ mode === 'ai' ? '准备所选范围的完整消息，确认发送后由 AI 独立检查错别字、标点和轻度语病。' : '在本地检查中文标点。' }}采纳的修改只写入当前 LOG 工程，不修改聊天原消息。</p>
    </section>

    <section v-if="aiPrepared" class="log-proof__setup" aria-label="AI 校对批次">
      <p class="log-proof__muted">将向 {{ aiSource === 'user' ? '个人 API' : '平台 AI' }}（{{ aiDestination }}）发送所选范围内筛选后的 {{ aiEstimate.messageCount }} 条消息、{{ aiEstimate.editableChars }} 个可编辑字符，预计 {{ aiStats.total }} 次请求。字符数不代表精确费用。</p>
      <p v-if="aiStarted" class="log-proof__muted">已完成 {{ aiStats.success }}/{{ aiStats.total }} 批，失败 {{ aiStats.failed }} 批，丢弃 {{ aiStats.discarded }} 项无效建议。取消会停止后续批次，已发出的请求仍可能计费；重试失败批次也可能再次计费。</p>
      <div class="log-proof__setup-actions">
        <n-button v-if="aiStats.remaining" size="small" :disabled="busy || aiInvalid || mode !== 'ai'" @click="startAI()">{{ aiStarted ? '继续未完成批次' : '确认发送并运行 AI' }}</n-button>
        <n-button v-if="phase === 'ai'" size="small" @click="cancel">取消 AI</n-button>
        <n-button v-if="aiStats.failed" size="small" :disabled="busy || aiInvalid || mode !== 'ai'" @click="startAI('failed')">重试失败批次（{{ aiStats.failed }}）</n-button>
      </div>
      <p v-for="group in aiFailures" :key="group.reason" class="log-proof__muted is-warn">{{ group.ids.length }} 个批次失败：{{ group.reason }}</p>
      <details v-if="aiFailures.length" class="log-proof__failure-details">
        <summary>查看详情</summary>
        <p v-for="group in aiFailures" :key="group.reason" class="log-proof__muted">第 {{ group.ids.join('、') }} 批：{{ group.reason }}</p>
      </details>
      <p v-if="aiWarning" class="log-proof__muted is-warn">{{ aiWarning }}</p>
    </section>

    <div v-if="busy" class="log-proof__progress" role="status" aria-live="polite">
      <span>{{ phaseText }}</span>
      <n-button v-if="!showSetup && (phase === 'load' || phase === 'check')" size="tiny" text @click="cancel">取消</n-button>
      <n-progress type="line" :percentage="progressPercent" :show-indicator="false" :height="4" :processing="phase === 'load'" />
    </div>
    <n-alert v-if="scanError" type="error" :show-icon="false" class="log-proof__alert">{{ scanError }}</n-alert>

    <template v-if="scanned && !(busy && phase !== 'apply' && phase !== 'ai')">
      <!-- 概览：进度、摘要与审阅层开关 -->
      <section class="log-proof__summary">
        <div class="log-proof__summary-head">
          <div class="log-proof__ring" :style="{ '--p': `${reviewedPercent}` }" :aria-label="`已审 ${reviewedPercent}%`">
            <span>{{ reviewedPercent }}<small>%</small></span>
          </div>
          <div class="log-proof__summary-text">
            <strong>{{ counts.all ? `${counts.pending} 处待审` : '没有发现问题' }}</strong>
            <span>{{ scannedScope }} · {{ scannedCount }} 条消息 · {{ results.length }} 条有建议</span>
          </div>
          <div class="log-proof__summary-tools">
            <button type="button" class="log-proof__tool" :class="{ 'is-on': markup }" :title="markup ? '隐藏原文中的校对标记' : '在原文中显示校对标记'" :aria-pressed="markup" aria-label="原文标记" @click="toggleMarkup">
              <n-icon :component="Eye" size="16" />
            </button>
            <button type="button" class="log-proof__tool" :class="{ 'is-on': focused }" :disabled="!reviewing" :title="focused ? '退出专注审阅' : '专注审阅'" :aria-pressed="focused" aria-label="专注审阅" @click="emit('focus', !focused)">
              <n-icon :component="Focus2" size="16" />
            </button>
            <button type="button" class="log-proof__tool" title="范围与重新扫描" aria-label="范围与重新扫描" :aria-expanded="setupOpen" @click="setupOpen = !setupOpen">
              <n-icon :component="setupOpen ? Adjustments : Refresh" size="16" />
            </button>
          </div>
        </div>
        <div v-if="counts.all" class="log-proof__stack" aria-hidden="true">
          <i class="is-accepted" :style="{ flexGrow: counts.accepted }" />
          <i class="is-rejected" :style="{ flexGrow: counts.rejected }" />
          <i class="is-pending" :style="{ flexGrow: counts.pending }" />
        </div>
        <p v-if="lintMode === 'local'" class="log-proof__muted is-warn">zhlint 加载失败，本次只运行了跑团补充规则。</p>
        <p v-if="staleIds.size" class="log-proof__muted is-warn">{{ staleIds.size }} 条消息在扫描后已变化，未应用；重新扫描可获得最新建议。</p>
        <button v-if="counts.safePending" type="button" class="log-proof__quick" @click="acceptSafe">
          <n-icon :component="Checks" size="15" />
          <span>采纳全部可自动修正</span>
          <b>{{ counts.safePending }}</b>
        </button>
      </section>

      <!-- 筛选 -->
      <section v-if="counts.all" class="log-proof__filterbar" aria-label="筛选">
        <div class="log-proof__tabs" role="tablist">
          <button
            v-for="t in statusTabs"
            :key="t.value"
            type="button"
            role="tab"
            class="log-proof__tab"
            :class="{ 'is-on': statusFilter === t.value }"
            :aria-selected="statusFilter === t.value"
            @click="statusFilter = t.value"
          >{{ t.label }}<span>{{ t.count }}</span></button>
        </div>
        <div class="log-proof__chips">
          <button type="button" class="log-proof__chip is-safe" :class="{ 'is-on': levelFilter === 'safe' }" :aria-pressed="levelFilter === 'safe'" @click="levelFilter = levelFilter === 'safe' ? null : 'safe'">可自动</button>
          <button type="button" class="log-proof__chip is-suggest" :class="{ 'is-on': levelFilter === 'suggest' }" :aria-pressed="levelFilter === 'suggest'" @click="levelFilter = levelFilter === 'suggest' ? null : 'suggest'">待确认</button>
          <n-select v-model:value="ruleFilter" class="log-proof__rule" size="tiny" :options="ruleOptions" :consistent-menu-width="false" aria-label="按规则筛选" />
          <n-select v-model:value="sourceFilter" class="log-proof__rule" size="tiny" :options="proofSourceOptions" :consistent-menu-width="false" aria-label="按来源筛选" />
        </div>
        <div class="log-proof__batch">
          <span>{{ filterActive ? `筛选结果 ${filtered.length} 处` : `全部 ${filtered.length} 处` }}</span>
          <button type="button" class="log-proof__link" :disabled="!filtered.length" @click="decideFiltered('accepted')">全部采纳</button>
          <button type="button" class="log-proof__link" :disabled="!filtered.length" @click="decideFiltered('rejected')">全部放弃</button>
          <button type="button" class="log-proof__link is-muted" :disabled="!filtered.length" @click="decideFiltered(null)">重置</button>
        </div>
      </section>

      <!-- 批注列表 -->
      <div v-if="counts.all" ref="listEl" class="log-proof__list">
        <n-virtual-list
          v-if="rows.length"
          ref="listRef"
          :items="rows"
          :item-size="ROW_HEIGHT"
          key-field="key"
          style="height: 100%"
          @scroll="onListScroll"
        >
          <template #default="{ item: row }">
            <div v-if="row.kind === 'section'" class="log-proof__row is-section">{{ row.label }}</div>
            <div v-else-if="row.kind === 'head'" class="log-proof__row is-head">
              <button type="button" class="log-proof__who" :title="`定位到 ${row.r.speaker} 的消息`" @click="emit('locate', row.r.messageId, row.r.sectionId)">
                <strong>{{ row.r.speaker }}</strong>
                <span>{{ formatTime(row.r.createdAt) }}</span>
              </button>
              <n-popover trigger="click" placement="left-start" :show-arrow="false" style="max-width: 360px">
                <template #trigger>
                  <button type="button" class="log-proof__link is-muted">预览</button>
                </template>
                <div class="log-proof__diff" aria-label="原文与已采纳修改对比">
                  <template v-for="(t, index) in previewTokens(row.r)" :key="index">
                    <del v-if="t.type === 'delete'">{{ t.text }}</del>
                    <ins v-else-if="t.type === 'insert'">{{ t.text }}</ins>
                    <span v-else>{{ t.text }}</span>
                  </template>
                </div>
              </n-popover>
              <button type="button" class="log-proof__link" @click="decideMessage(row.r)">采纳本条</button>
            </div>
            <div
              v-else
              class="log-proof__row is-issue"
              :class="[`is-${stateOf(row.e.key)}`, `is-${row.e.i.level}`, { 'is-active': row.e.key === activeKey }]"
              role="button"
              tabindex="0"
              :aria-current="row.e.key === activeKey ? 'true' : undefined"
              :title="[PROOF_RULE_LABELS[(row.e as ProofEntry).i.rule], row.e.i.reason].filter(Boolean).join('：')"
              @click="activate(row.e.key)"
              @keydown.enter.self.prevent="activate(row.e.key)"
            >
              <span class="log-proof__change">
                <small v-if="row.e.i.source === 'ai'">AI</small><del>{{ row.e.i.original ? showText(row.e.i.original) : '插入' }}</del><ins :class="{ 'is-empty': !row.e.i.replacement }">{{ row.e.i.replacement ? showText(row.e.i.replacement) : '删' }}</ins>
              </span>
              <span class="log-proof__ctx">{{ row.e.i.before }}<b>{{ row.e.i.original }}</b>{{ row.e.i.after }}</span>
              <i v-if="unmappedKeys.has(row.e.key)" class="log-proof__loose" title="无法在排版中精确定位，按原文显示" />
              <span class="log-proof__acts">
                <button type="button" class="log-proof__act is-accept" :class="{ 'is-on': stateOf(row.e.key) === 'accepted' }" :aria-pressed="stateOf(row.e.key) === 'accepted'" title="采纳" aria-label="采纳" @click.stop="toggleDecision(row.e.key, 'accepted')">
                  <n-icon :component="Check" size="14" />
                </button>
                <button type="button" class="log-proof__act is-reject" :class="{ 'is-on': stateOf(row.e.key) === 'rejected' }" :aria-pressed="stateOf(row.e.key) === 'rejected'" title="放弃" aria-label="放弃" @click.stop="toggleDecision(row.e.key, 'rejected')">
                  <n-icon :component="X" size="14" />
                </button>
              </span>
            </div>
          </template>
        </n-virtual-list>
        <div v-else class="log-proof__empty">当前筛选下没有批注</div>
      </div>
      <div v-else class="log-proof__done">
        <n-icon :component="Check" size="26" />
        <p>{{ mode === 'ai' ? (aiStats.remaining || aiStats.failed ? 'AI 校对尚未完成' : aiStarted ? '没有发现可确定的问题' : '确认发送后开始 AI 校对') : '没有发现可确定的标点问题' }}</p>
      </div>
    </template>

    <!-- 应用与撤销 -->
    <footer v-if="(scanned && counts.all) || undoStack.length || overrideCount" class="log-proof__foot">
      <p v-if="!canEditDraft" class="log-proof__muted is-warn">没有编辑草稿权限，只能审阅。</p>
      <p v-else-if="!supportsContentOverrides" class="log-proof__muted is-warn">服务器未声明支持保存校对正文，应用已禁用；请同步更新后端后重新打开排版器。</p>
      <div class="log-proof__foot-row">
        <n-button
          v-if="scanned && counts.all"
          type="primary"
          class="log-proof__apply"
          :loading="phase === 'apply'"
          :disabled="!canEditDraft || !supportsContentOverrides || !acceptedCount || busy"
          @click="apply"
        >应用 {{ acceptedCount }} 处修改</n-button>
        <n-button v-if="undoStack.length" secondary :disabled="!canEditDraft || !supportsContentOverrides || busy" :title="`撤销上次应用（${undoStack[0].count} 条消息）`" @click="undoLast">
          <template #icon><n-icon :component="ArrowBackUp" /></template>
          撤销
        </n-button>
      </div>
      <div v-if="overrideCount" class="log-proof__overrides">
        <span>{{ overrideCount }} 条消息使用校对后的正文</span>
        <n-popconfirm @positive-click="clearAllOverrides">
          <template #trigger>
            <button type="button" class="log-proof__link is-muted" :disabled="!canEditDraft || !supportsContentOverrides || busy">全部改回原消息</button>
          </template>
          将 {{ overrideCount }} 条消息改回显示聊天原消息的正文？此操作可撤销。
        </n-popconfirm>
      </div>
    </footer>

    <Teleport v-if="canvasEl" :to="canvasEl">
      <Transition name="log-proof-rise">
        <div v-if="reviewing" class="log-proof__dock" :class="{ 'is-compact': compactBar }">
          <LogProofReviewBar
            :index="position.index"
            :total="position.total"
            :decided="counts.decided"
            :all="counts.all"
            :accepted="acceptedCount"
            :safe-pending="counts.safePending"
            :can-apply="canApply"
            :applying="phase === 'apply'"
            :focused="focused"
            :narrow="compactBar"
            @nav="nav"
            @apply="apply"
            @accept-safe="acceptSafe"
            @focus="emit('focus', !focused)"
            @hide="toggleMarkup"
          />
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.log-proof {
  --log-proof-accent: var(--log-wb-accent, var(--primary-color, #2f6fde));
  --log-proof-safe: #4f46e5;
  --log-proof-suggest: #d97706;
  --log-proof-accepted: #16a34a;
  --log-proof-rejected: #94a3b8;
  --log-proof-muted: var(--log-wb-muted, var(--sc-text-secondary, #64748b));
  --log-proof-line: var(--log-wb-line-soft, var(--sc-border-mute, rgba(15, 23, 42, 0.08)));
  --log-proof-hover: var(--log-wb-hover, var(--sc-chip-bg, rgba(15, 23, 42, 0.05)));
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  min-height: 0;
  min-width: 0;
  padding: 8px 0 12px;
  font-size: 13px;
}

.log-proof__muted {
  min-width: 0;
  overflow-wrap: anywhere;
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--log-proof-muted);
}

.log-proof__muted.is-warn {
  color: #b45309;
}

.log-proof__alert {
  flex: none;
  min-width: 0;
  overflow-wrap: anywhere;
}

.log-proof__failure-details {
  min-width: 0;
  max-height: 160px;
  overflow-y: auto;
  overflow-wrap: anywhere;
}

.log-proof__failure-details summary {
  cursor: pointer;
}

/* ---- 扫描设置 ---- */
.log-proof__setup {
  flex: none;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.log-proof__seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--log-proof-hover);
}

.log-proof__seg-btn {
  flex: 1 1 0;
  min-width: 0;
  height: 28px;
  padding: 0 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--log-proof-muted);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
}

.log-proof__seg-btn.is-on {
  color: var(--sc-text-primary, #0f172a);
  font-weight: 600;
  background: var(--sc-bg-elevated, #fff);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.12);
}

.log-proof__seg-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.log-proof__disclose {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0;
  border: 0;
  background: none;
  color: var(--log-proof-muted);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.log-proof__disclose .n-icon {
  transition: transform 0.15s ease;
  transform: rotate(-90deg);
}

.log-proof__disclose .n-icon.is-open {
  transform: none;
}

.log-proof__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--log-proof-accent);
}

.log-proof__filters {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.log-proof__setup-actions {
  display: flex;
  flex-wrap: wrap;
  min-width: 0;
  gap: 8px;
}

.log-proof__setup-actions > .n-button {
  min-width: 0;
  max-width: 100%;
  height: auto;
  min-height: 28px;
  white-space: normal;
}

.log-proof__setup-actions :deep(.n-button__content) {
  min-width: 0;
  white-space: normal;
  overflow-wrap: anywhere;
}

.log-proof__setup-actions > :first-child {
  flex: 1;
}

.log-proof__progress {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
  color: var(--log-proof-muted);
  font-variant-numeric: tabular-nums;
}

/* ---- 概览 ---- */
.log-proof__summary {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.log-proof__summary-head {
  display: flex;
  align-items: center;
  gap: 12px;
}

.log-proof__ring {
  --p: 0;
  flex: none;
  position: relative;
  display: grid;
  place-items: center;
  width: 46px;
  height: 46px;
  border-radius: 50%;
  background: conic-gradient(var(--log-proof-accepted) calc(var(--p) * 1%), var(--log-proof-hover) 0);
  transition: background 0.3s ease;
}

.log-proof__ring::before {
  content: '';
  position: absolute;
  inset: 5px;
  border-radius: 50%;
  background: var(--sc-bg-elevated, #fff);
}

.log-proof__ring span {
  position: relative;
  font-size: 13px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.log-proof__ring small {
  font-size: 9px;
  font-weight: 600;
  color: var(--log-proof-muted);
}

.log-proof__summary-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.log-proof__summary-text strong {
  font-size: 15px;
}

.log-proof__summary-text span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--log-proof-muted);
}

.log-proof__summary-tools {
  flex: none;
  display: flex;
  gap: 2px;
}

.log-proof__tool {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--log-proof-muted);
  cursor: pointer;
  transition: background-color 0.12s ease, color 0.12s ease;
}

.log-proof__tool:hover:not(:disabled) {
  background: var(--log-proof-hover);
  color: var(--sc-text-primary, #0f172a);
}

.log-proof__tool.is-on {
  color: var(--log-proof-accent);
  background: color-mix(in srgb, var(--log-proof-accent) 12%, transparent);
}

.log-proof__tool:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.log-proof__stack {
  display: flex;
  gap: 2px;
  height: 4px;
  overflow: hidden;
  border-radius: 999px;
}

.log-proof__stack i {
  flex-basis: 0;
  min-width: 0;
  transition: flex-grow 0.3s ease;
}

.log-proof__stack .is-accepted { background: var(--log-proof-accepted); }
.log-proof__stack .is-rejected { background: var(--log-proof-rejected); }
.log-proof__stack .is-pending { background: var(--log-proof-hover); }

.log-proof__quick {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 10px;
  border: 1px solid color-mix(in srgb, var(--log-proof-safe) 30%, transparent);
  border-radius: 8px;
  background: color-mix(in srgb, var(--log-proof-safe) 7%, transparent);
  color: var(--log-proof-safe);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.12s ease;
}

.log-proof__quick:hover {
  background: color-mix(in srgb, var(--log-proof-safe) 13%, transparent);
}

.log-proof__quick span {
  flex: 1;
  text-align: left;
}

.log-proof__quick b {
  font-variant-numeric: tabular-nums;
}

/* ---- 筛选 ---- */
.log-proof__filterbar {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.log-proof__tabs {
  display: flex;
  border-bottom: 1px solid var(--log-proof-line);
}

.log-proof__tab {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
  align-items: baseline;
  justify-content: center;
  gap: 4px;
  height: 30px;
  margin-bottom: -1px;
  padding: 0 4px;
  border: 0;
  border-bottom: 2px solid transparent;
  background: none;
  color: var(--log-proof-muted);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  transition: color 0.12s ease, border-color 0.12s ease;
}

.log-proof__tab span {
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  opacity: 0.8;
}

.log-proof__tab.is-on {
  color: var(--sc-text-primary, #0f172a);
  font-weight: 600;
  border-bottom-color: var(--log-proof-accent);
}

.log-proof__chips {
  display: flex;
  align-items: center;
  gap: 6px;
}

.log-proof__chip {
  flex: none;
  height: 24px;
  padding: 0 9px;
  border: 1px solid var(--log-proof-line);
  border-radius: 999px;
  background: transparent;
  color: var(--log-proof-muted);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: background-color 0.12s ease, color 0.12s ease, border-color 0.12s ease;
}

.log-proof__chip::before {
  content: '';
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 5px;
  border-radius: 50%;
  vertical-align: 1px;
  background: var(--chip-tone);
}

.log-proof__chip.is-safe { --chip-tone: var(--log-proof-safe); }
.log-proof__chip.is-suggest { --chip-tone: var(--log-proof-suggest); }

.log-proof__chip.is-on {
  color: var(--chip-tone);
  border-color: color-mix(in srgb, var(--chip-tone) 45%, transparent);
  background: color-mix(in srgb, var(--chip-tone) 9%, transparent);
}

.log-proof__rule {
  flex: 1;
  min-width: 0;
}

.log-proof__batch {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  color: var(--log-proof-muted);
}

.log-proof__batch > span {
  flex: 1;
  min-width: 0;
}

.log-proof__link {
  flex: none;
  padding: 0;
  border: 0;
  background: none;
  color: var(--log-proof-accent);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.log-proof__link.is-muted {
  color: var(--log-proof-muted);
}

.log-proof__link:hover:not(:disabled) {
  text-decoration: underline;
}

.log-proof__link:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* ---- 列表 ---- */
.log-proof__list {
  flex: 1 1 auto;
  min-height: 180px;
  margin: 0 -8px;
}

.log-proof__row {
  box-sizing: border-box;
  height: 34px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  min-width: 0;
}

.log-proof__row.is-section {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--log-proof-muted);
  text-transform: uppercase;
}

.log-proof__row.is-head {
  padding-top: 6px;
}

.log-proof__who {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.log-proof__who strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}

.log-proof__who span {
  flex: none;
  font-size: 11px;
  color: var(--log-proof-muted);
  font-variant-numeric: tabular-nums;
}

.log-proof__who:hover strong {
  color: var(--log-proof-accent);
}

.log-proof__row.is-issue {
  --row-tone: var(--log-proof-suggest);
  position: relative;
  border-radius: 7px;
  cursor: pointer;
  transition: background-color 0.12s ease;
}

.log-proof__row.is-issue::before {
  content: '';
  position: absolute;
  left: 2px;
  top: 8px;
  bottom: 8px;
  width: 3px;
  border-radius: 2px;
  background: var(--row-tone);
}

.log-proof__row.is-safe { --row-tone: var(--log-proof-safe); }
.log-proof__row.is-accepted { --row-tone: var(--log-proof-accepted); }
.log-proof__row.is-rejected { --row-tone: var(--log-proof-rejected); }

.log-proof__row.is-issue:hover {
  background: var(--log-proof-hover);
}

.log-proof__row.is-issue.is-active {
  background: color-mix(in srgb, var(--log-proof-accent) 11%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--log-proof-accent) 30%, transparent);
}

.log-proof__row.is-issue:focus-visible {
  outline: 2px solid var(--log-proof-accent);
  outline-offset: -2px;
}

.log-proof__change {
  flex: none;
  display: inline-flex;
  align-items: baseline;
  gap: 2px;
  margin-left: 6px;
  font-size: 13px;
}

.log-proof__change del,
.log-proof__diff del {
  padding: 0 2px;
  border-radius: 3px;
  color: #b42318;
  background: rgba(180, 35, 24, 0.08);
}

.log-proof__change ins,
.log-proof__diff ins {
  padding: 0 2px;
  border-radius: 3px;
  color: #067647;
  background: rgba(6, 118, 71, 0.1);
  text-decoration: none;
  font-weight: 600;
}

.log-proof__change ins.is-empty {
  font-size: 11px;
  font-weight: 500;
}

.log-proof__row.is-rejected .log-proof__change,
.log-proof__row.is-rejected .log-proof__ctx {
  opacity: 0.5;
}

.log-proof__ctx {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--log-proof-muted);
}

.log-proof__ctx b {
  color: var(--sc-text-primary, #0f172a);
  font-weight: 600;
  background: color-mix(in srgb, var(--row-tone) 18%, transparent);
  border-radius: 2px;
}

.log-proof__loose {
  flex: none;
  width: 8px;
  height: 8px;
  border: 1.5px dashed var(--log-proof-muted);
  border-radius: 50%;
}

.log-proof__acts {
  flex: none;
  display: flex;
  gap: 2px;
}

.log-proof__act {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--log-proof-muted);
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.12s ease, background-color 0.12s ease, color 0.12s ease;
}

.log-proof__row.is-issue:hover .log-proof__act,
.log-proof__row.is-issue.is-active .log-proof__act,
.log-proof__row.is-issue:focus-within .log-proof__act,
.log-proof__act.is-on {
  opacity: 1;
}

.log-proof__act:hover {
  background: color-mix(in srgb, var(--sc-text-primary, #0f172a) 8%, transparent);
}

.log-proof__act.is-accept.is-on {
  color: #fff;
  background: var(--log-proof-accepted);
}

.log-proof__act.is-reject.is-on {
  color: var(--sc-text-primary, #0f172a);
  background: color-mix(in srgb, var(--log-proof-rejected) 35%, transparent);
}

@media (hover: none) {
  .log-proof__act {
    opacity: 1;
  }
}

.log-proof__empty,
.log-proof__done {
  padding: 28px 0;
  text-align: center;
  font-size: 12px;
  color: var(--log-proof-muted);
}

.log-proof__done {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  color: var(--log-proof-accepted);
}

.log-proof__done p {
  margin: 0;
  color: var(--log-proof-muted);
}

.log-proof__diff {
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

/* ---- 底部 ---- */
.log-proof__foot {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--log-proof-line);
}

.log-proof__foot-row {
  display: flex;
  gap: 8px;
}

.log-proof__apply {
  flex: 1;
}

.log-proof__overrides {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 12px;
  color: var(--log-proof-muted);
}

/* ---- 画布工具条（Teleport 到画布） ---- */
.log-proof__dock {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 14px;
  z-index: 5;
  display: flex;
  justify-content: center;
  padding: 0 150px;
  pointer-events: none;
}

.log-proof__dock.is-compact {
  bottom: 56px;
  padding: 0 10px;
}

.log-proof-rise-enter-active,
.log-proof-rise-leave-active {
  transition: opacity 0.18s ease, transform 0.18s cubic-bezier(0.2, 0.7, 0.3, 1);
}

.log-proof-rise-enter-from,
.log-proof-rise-leave-to {
  opacity: 0;
  transform: translateY(10px);
}

@media (prefers-reduced-motion: reduce) {
  .log-proof-rise-enter-active,
  .log-proof-rise-leave-active,
  .log-proof__ring,
  .log-proof__stack i {
    transition: none;
  }
}
</style>
