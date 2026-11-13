// 批量校对的工程写入：采纳决定 → 每条消息的新正文 → 草稿正文覆盖（messageContentOverrides）与撤销记录。
// 只读写 LOG 草稿，不调用 message.update；原聊天消息保持不变。
import { applyProofEdits, type ProofIssue } from '@/services/log/proofread/logProofread';
import type { LogDraft } from '@/types/log';

export type ProofDecision = 'accepted' | 'rejected';

export interface ProofMessageMeta {
  messageId: string;
  sectionId: string;
  speaker: string;
  createdAt: number;
  /** 扫描时的工程正文（已有覆盖时为覆盖值）。 */
  source: string;
  /** 扫描时的原消息正文。 */
  original: string;
}

export interface ProofMessageResult extends ProofMessageMeta {
  issues: ProofIssue[];
}

/** 撤销记录：messageId → 应用前后的覆盖值（undefined 表示没有覆盖）。 */
export type ProofUndoRecord = Record<string, { before: string | undefined; after: string | undefined }>;

export const proofDecisionKey = (messageId: string, issueId: string) => `${messageId}:${issueId}`;

export const toProofResult = (meta: ProofMessageMeta, issues: ProofIssue[]): ProofMessageResult => ({ ...meta, issues });

/**
 * 计算要写入的正文。正文自扫描后已变化（覆盖被改、已加载的原消息被编辑）的消息记入 stale，不写入。
 * currentOriginal 返回 undefined 表示该消息不在当前已加载窗口中，此时以扫描时的原文为准。
 */
export const planProofApply = (
  results: ProofMessageResult[],
  decisions: Record<string, ProofDecision>,
  currentOverride: (id: string) => string | undefined,
  currentOriginal: (id: string) => string | undefined,
) => {
  const writes: Record<string, string> = {};
  const stale: string[] = [];
  for (const r of results) {
    const accepted = r.issues.filter((i) => decisions[proofDecisionKey(r.messageId, i.id)] === 'accepted');
    if (!accepted.length) continue;
    const effective = currentOverride(r.messageId) ?? currentOriginal(r.messageId) ?? r.original;
    const next = effective === r.source ? applyProofEdits(r.source, accepted) : null;
    if (next === null) {
      stale.push(r.messageId);
      continue;
    }
    if (next !== r.source) writes[r.messageId] = next;
  }
  return { writes, stale };
};

/** 在一次草稿 mutation 中写入覆盖；新正文等于原文时移除覆盖。返回撤销记录。 */
export const commitProofApply = (
  draft: Pick<LogDraft, 'messageContentOverrides'>,
  writes: Record<string, string>,
  originalOf: (id: string) => string | undefined = () => undefined,
): ProofUndoRecord => {
  const overrides = { ...draft.messageContentOverrides };
  const record: ProofUndoRecord = {};
  for (const [id, content] of Object.entries(writes)) {
    const before = overrides[id];
    const after = content === originalOf(id) ? undefined : content;
    if (after === undefined) delete overrides[id];
    else overrides[id] = after;
    record[id] = { before, after };
  }
  if (Object.keys(overrides).length) draft.messageContentOverrides = overrides;
  else delete draft.messageContentOverrides;
  return record;
};

/** 提前检查服务端草稿限制，避免乐观应用后才发现不能保存。 */
export const proofDraftSizeError = (draft: LogDraft): string | null => {
  const size = (text: string) => new TextEncoder().encode(text).byteLength;
  if (Object.values(draft.messageContentOverrides || {}).some((text) => size(text) > 64 * 1024)) {
    return '单条消息的校对正文超过 64 KiB，无法保存';
  }
  if (size(JSON.stringify(draft)) > 2 * 1024 * 1024) {
    return '校对后的 LOG 草稿超过 2 MiB，无法保存，请缩小应用范围';
  }
  return null;
};

/** 撤销一次批量应用：只还原覆盖值仍等于本次结果的消息，之后被再次修改的消息保持现状。 */
export const undoProofApply = (draft: Pick<LogDraft, 'messageContentOverrides'>, record: ProofUndoRecord) => {
  const overrides = { ...draft.messageContentOverrides };
  let restored = 0;
  let skipped = 0;
  for (const [id, { before, after }] of Object.entries(record)) {
    if (overrides[id] !== after) {
      skipped++;
      continue;
    }
    if (before === undefined) delete overrides[id];
    else overrides[id] = before;
    restored++;
  }
  if (Object.keys(overrides).length) draft.messageContentOverrides = overrides;
  else delete draft.messageContentOverrides;
  return { restored, skipped };
};
