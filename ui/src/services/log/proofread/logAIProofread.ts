// AI 只是检测器：快照、协议验证与批次调度；不依赖 UI，不写正文或聊天消息。
import { editableProofFragments, validAIProofEdit, type ProofIssue } from './logProofread';
import type { ProofMessageMeta, ProofMessageResult } from '@/views/log/logProofreadBatch';

export interface AIProofFragment { id: string; messageId: string; sectionId: string; text: string; run: number; start: number }
export interface AIProofBatch {
  id: number;
  input: string;
  fragments: AIProofFragment[];
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled';
  error?: string;
  discarded?: Record<string, number>;
}
const wireInput = (fragments: AIProofFragment[]) => JSON.stringify({ version: 1, fragments: fragments.map(({ id, messageId, sectionId, text }) => ({ id, messageId, sectionId, text })) });

interface AIProofPlan { batches: AIProofBatch[]; editableChars: number; messageCount: number }

function* prepareAIProofSteps(messages: ProofMessageMeta[], maxInput = 8000): Generator<void, AIProofPlan> {
  const limit = Math.min(8000, maxInput);
  if (!Number.isFinite(limit) || limit < 400) throw new Error('AI 模型输入上限过小，无法容纳校对协议');
  const batches: AIProofBatch[] = [];
  let fragments: AIProofFragment[] = [];
  let chars = 0;
  let editableChars = 0;
  let nextId = 0;
  let processed = 0;
  const eligible = new Set<string>();
  const flush = () => {
    if (!fragments.length) return;
    batches.push({ id: batches.length + 1, fragments, input: wireInput(fragments), status: 'pending' });
    fragments = []; chars = 0;
  };
  for (const message of messages) {
    for (const slice of editableProofFragments(message.source)) {
      let offset = 0;
      while (offset < slice.text.length) {
        let length = Math.min(1000, slice.text.length - offset);
        let fragment: AIProofFragment;
        // 包装、转义及标识符也计入输入；超长节点在 UTF-16 字符边界拆分。
        do {
          if (/[\uD800-\uDBFF]/.test(slice.text[offset + length - 1] || '')) length--;
          fragment = { id: `f${nextId}`, messageId: message.messageId, sectionId: message.sectionId, run: slice.run, start: slice.start + offset, text: slice.text.slice(offset, offset + length) };
          if (wireInput([fragment]).length <= limit) break;
          length = Math.floor(length / 2);
        } while (length > 0);
        if (!length) throw new Error('AI 输入上限无法容纳文本片段');
        const messageCount = new Set([...fragments.map((f) => f.messageId), message.messageId]).size;
        if (fragments.length && (messageCount > 20 || chars + length > 3500 || fragments.length >= 128 || wireInput([...fragments, fragment]).length > limit)) flush();
        fragments.push(fragment);
        chars += length; editableChars += length;
        eligible.add(message.messageId);
        offset += length; nextId++;
      }
    }
    if (++processed % 25 === 0) yield;
  }
  flush();
  return { batches, editableChars, messageCount: eligible.size };
}

export const prepareAIProofBatches = (messages: ProofMessageMeta[], maxInput = 8000): AIProofPlan => {
  const steps = prepareAIProofSteps(messages, maxInput);
  let next = steps.next();
  while (!next.done) next = steps.next();
  return next.value;
};

export const prepareAIProofBatchesAsync = async (messages: ProofMessageMeta[], maxInput: number | undefined, signal: AbortSignal): Promise<AIProofPlan> => {
  const steps = prepareAIProofSteps(messages, maxInput);
  while (true) {
    if (signal.aborted) throw new Error('AI 校对准备已取消');
    const next = steps.next();
    if (next.done) return next.value;
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
  }
};

const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
// 旧 prompt 的 category 继续兼容；新协议直接使用审阅规则名。
const RULES = { typo: 'ai_typo', punctuation: 'ai_punctuation', grammar: 'ai_grammar', ai_typo: 'ai_typo', ai_punctuation: 'ai_punctuation', ai_grammar: 'ai_grammar' } as const;
const OPEN = '<sealchat-proofread>';
const CLOSE = '</sealchat-proofread>';
const stripFence = (text: string) => text.trim().replace(/^```[^\r\n]*\r?\n/, '').replace(/\r?\n```\s*$/, '').trim();

/** 只恢复传输包装和完整 JSON 行，不猜测自然语言中的修改内容。 */
const readSuggestions = (raw: string, drop: (reason: string) => void): unknown[] => {
  const start = raw.indexOf(OPEN);
  if (start >= 0) {
    const end = raw.indexOf(CLOSE, start + OPEN.length);
    const body = stripFence(raw.slice(start + OPEN.length, end < 0 ? undefined : end));
    if (!body && end >= 0) return [];
    const lines = body.split(/\r?\n/).filter((line) => line.trim());
    if (lines.length > 100) throw new Error('AI 校对建议超过 100 项');
    const items: unknown[] = [];
    for (const line of lines) {
      try { items.push(JSON.parse(line)); } catch { drop('JSON 行无效或截断'); }
    }
    if (items.some((item) => record(item) && Object.prototype.hasOwnProperty.call(item, 'fragmentId'))) return items;
    throw new Error('AI 输出格式无法识别');
  }
  // 过渡兼容管理员自定义的旧提示词，包括单层 Markdown fence。
  let payload: unknown;
  try { payload = JSON.parse(stripFence(raw)); } catch { throw new Error('AI 输出格式无法识别'); }
  if (!record(payload) || !Array.isArray(payload.suggestions) || Object.keys(payload).some((k) => k !== 'suggestions')) throw new Error('AI 输出格式无法识别');
  if (payload.suggestions.length > 100) throw new Error('AI 校对建议超过 100 项');
  return payload.suggestions;
};

export const parseAIProofResponse = (raw: string, batch: AIProofBatch, snapshots: Map<string, ProofMessageMeta>) => {
  if (raw.length > 64 * 1024 || new TextEncoder().encode(raw).length > 64 * 1024) throw new Error('AI 校对响应超过 64 KiB');
  const fragments = new Map(batch.fragments.map((f) => [f.id, f]));
  const results = new Map<string, ProofMessageResult>();
  const discarded: Record<string, number> = {};
  const drop = (reason: string) => { discarded[reason] = (discarded[reason] || 0) + 1; };
  for (const item of readSuggestions(raw, drop)) {
    if (!record(item) || typeof item.fragmentId !== 'string' || typeof item.original !== 'string' || typeof item.replacement !== 'string'
      || typeof item.reason !== 'string' || !item.reason.trim() || item.reason.length > 160 || typeof item.category !== 'string' || !Object.prototype.hasOwnProperty.call(RULES, item.category)
      || Object.keys(item).some((k) => !['fragmentId', 'original', 'replacement', 'reason', 'category', 'before', 'after'].includes(k))
      || [item.before, item.after].some((v) => v !== undefined && (typeof v !== 'string' || v.length > 40))
      || item.original.length > 40 || item.replacement.length > 40) { drop('格式或长度无效'); continue; }
    const f = fragments.get(item.fragmentId);
    const meta = f && snapshots.get(f.messageId);
    if (!f || !meta || f.sectionId !== meta.sectionId) { drop('片段不属于快照'); continue; }
    const before = typeof item.before === 'string' ? item.before : '';
    const after = typeof item.after === 'string' ? item.after : '';
    if (!item.original && !before && !after) { drop('插入缺少定位上下文'); continue; }
    const positions: number[] = [];
    for (let p = 0; p <= f.text.length - item.original.length; p++) {
      if (f.text.slice(p, p + item.original.length) !== item.original) continue;
      if (!f.text.slice(0, p).endsWith(before) || !f.text.slice(p + item.original.length).startsWith(after)) continue;
      if (p > 0 && /[\uD800-\uDBFF]/.test(f.text[p - 1]) && /[\uDC00-\uDFFF]/.test(f.text[p] || '')) continue;
      positions.push(p);
      if (positions.length > 1) break;
    }
    if (positions.length !== 1) { drop(positions.length ? '原文定位歧义' : '原文不匹配'); continue; }
    const start = f.start + positions[0];
    const end = start + item.original.length;
    const edit = { run: f.run, start, end, original: item.original, replacement: item.replacement };
    if (!validAIProofEdit(meta.source, edit)) { drop('保护区或非局部修改'); continue; }
    const issue: ProofIssue = {
      ...edit,
      id: `ai:${f.run}:${start}:${end}:${encodeURIComponent(item.replacement)}`,
      rule: RULES[item.category as keyof typeof RULES], level: 'suggest', source: 'ai', reason: item.reason.trim(),
      before: f.text.slice(Math.max(0, positions[0] - 12), positions[0]),
      after: f.text.slice(positions[0] + item.original.length, positions[0] + item.original.length + 12),
    };
    let result = results.get(meta.messageId);
    if (!result) { result = { ...meta, issues: [] }; results.set(meta.messageId, result); }
    if (result.issues.some((i) => i.id === issue.id)) { drop('重复建议'); continue; }
    result.issues.push(issue);
  }
  return { results: [...results.values()], discarded };
};

/** 固定错误分类，避免服务商的长响应或逐批错误撑满面板。 */
export const summarizeAIProofFailures = (batches: AIProofBatch[]) => {
  const groups = new Map<string, number[]>();
  for (const batch of batches) {
    if (batch.status !== 'failed') continue;
    const error = batch.error || '';
    const reason = /超时|timeout/i.test(error) ? '请求超时'
      : /格式无法识别/.test(error) ? 'AI 输出格式无法识别'
      : /超过 64 KiB|超过 100 项/.test(error) ? 'AI 输出超过安全上限'
      : /来源|快照/.test(error) ? '来源快照校验失败'
      : 'AI 请求失败';
    const ids = groups.get(reason) || [];
    ids.push(batch.id);
    groups.set(reason, ids);
  }
  return Array.from(groups, ([reason, ids]) => ({ reason, ids }));
};

/** 顺序派发；取消/过期后不派发且不发布响应。失败与取消可单独重试，成功批次永不重跑。 */
export const runAIProofBatches = async (options: {
  batches: AIProofBatch[];
  signal: AbortSignal;
  isCurrent: () => boolean;
  request: (batch: AIProofBatch, signal: AbortSignal) => Promise<string>;
  complete: (batch: AIProofBatch, raw: string) => Promise<void>;
  changed: () => void;
  timeoutMs?: number;
  mode?: 'unfinished' | 'failed';
}) => {
  // 启动时固定本轮范围；失败重试不会顺带执行 pending/cancelled，成功项永不派发。
  const eligible = new Set(options.batches.filter((batch) => options.mode === 'failed'
    ? batch.status === 'failed' : batch.status === 'pending' || batch.status === 'cancelled'));
  for (const batch of options.batches) {
    if (options.signal.aborted || !options.isCurrent()) break;
    if (!eligible.has(batch) || batch.status === 'succeeded') continue;
    batch.status = 'running'; batch.error = undefined; options.changed();
    const requestController = new AbortController();
    let timedOut = false;
    const abort = () => requestController.abort();
    options.signal.addEventListener('abort', abort, { once: true });
    if (options.signal.aborted) requestController.abort();
    const timer = setTimeout(() => { timedOut = true; requestController.abort(); }, options.timeoutMs ?? 180000);
    try {
      const raw = await new Promise<string>((resolve, reject) => {
        if (requestController.signal.aborted) { reject(new Error('AI 校对已取消')); return; }
        requestController.signal.addEventListener('abort', () => reject(new Error(timedOut ? 'AI 批次请求超时' : 'AI 校对已取消')), { once: true });
        options.request(batch, requestController.signal).then(resolve, reject);
      });
      if (options.signal.aborted || !options.isCurrent()) { batch.status = 'cancelled'; break; }
      await options.complete(batch, raw);
      if (options.signal.aborted || !options.isCurrent()) { batch.status = 'cancelled'; break; }
      batch.status = 'succeeded';
    } catch (error) {
      batch.status = options.signal.aborted || !options.isCurrent() ? 'cancelled' : 'failed';
      batch.error = error instanceof Error ? error.message : 'AI 请求失败';
    } finally {
      clearTimeout(timer);
      options.signal.removeEventListener('abort', abort);
    }
    options.changed();
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
  }
  for (const batch of options.batches) if (batch.status === 'running') batch.status = 'cancelled';
  options.changed();
};
