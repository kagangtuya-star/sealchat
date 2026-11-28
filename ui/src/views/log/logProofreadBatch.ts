// 批量校对只规划原消息的正文编辑；基础规则与 AI 建议共用相同的采纳和写回流程。
import { applyProofEdits, proofEditsConflict, type ProofIssue } from '@/services/log/proofread/logProofread';
import type { LogMessageSource } from '@/services/log/logMessageEdits';

export type ProofDecision = 'accepted' | 'rejected';

export interface ProofMessageMeta extends LogMessageSource {
  speaker: string;
  createdAt: number;
}

export interface ProofMessageResult extends ProofMessageMeta {
  issues: ProofIssue[];
}

export const proofDecisionKey = (messageId: string, issueId: string) => `${messageId}:${issueId}`;

export const toProofResult = (meta: ProofMessageMeta, issues: ProofIssue[]): ProofMessageResult => ({ ...meta, issues });

/** 批量决定不会替用户选择互斥方案；单项采纳也不能覆盖之前的选择。 */
export const decideProofIssues = (results: ProofMessageResult[], decisions: Record<string, ProofDecision>, keys: string[], value: ProofDecision | null) => {
  const next = { ...decisions };
  const picked = new Set(keys);
  let conflicts = 0;
  for (const r of results) for (const issue of r.issues) {
    const key = proofDecisionKey(r.messageId, issue.id);
    if (!picked.has(key)) continue;
    if (value === 'accepted' && r.issues.some((other) => other !== issue && proofEditsConflict(issue, other)
      && (decisions[proofDecisionKey(r.messageId, other.id)] === 'accepted' || picked.has(proofDecisionKey(r.messageId, other.id))))) {
      conflicts++; continue;
    }
    if (value) next[key] = value;
    else delete next[key];
  }
  return { decisions: next, conflicts };
};

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
    const original = currentOriginal(r.messageId);
    const effective = currentOverride(r.messageId) ?? original ?? r.original;
    const next = (original === undefined || original === r.original) && effective === r.source ? applyProofEdits(r.source, accepted) : null;
    if (next === null) {
      stale.push(r.messageId);
      continue;
    }
    if (next !== r.source) writes[r.messageId] = next;
  }
  return { writes, stale };
};
