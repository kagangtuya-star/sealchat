import { fetchLogMessageContext, fetchLogSectionMessages, isLogError } from '@/services/log/logApi';
import { findTextMatches, replaceTextMatches, type LogTextMatch } from '@/services/log/proofread/logProofread';
import type { LogDraft, LogProtocolMessage } from '@/types/log';
import { commitProofApply, proofDraftSizeError, type ProofUndoRecord } from './logProofreadBatch';

const PAGE_LIMIT = 100;

export interface LogFindResult {
  messageId: string;
  sectionId: string;
  original: string;
  source: string;
  matches: LogTextMatch[];
}

type Overrides = LogDraft['messageContentOverrides'];

export const scanLogMatches = async (
  projectId: string, sectionIds: string[], overrides: Overrides, query: string, signal: AbortSignal,
) => {
  const results: LogFindResult[] = [];
  const versions = new Map<string, number>();
  const seen = new Set<string>();
  for (const sectionId of sectionIds) {
    let cursor: string | null = null;
    do {
      signal.throwIfAborted();
      const page = await fetchLogSectionMessages(projectId, sectionId, cursor, PAGE_LIMIT, signal);
      signal.throwIfAborted();
      if (versions.has(sectionId) && versions.get(sectionId) !== page.sourceVersion) {
        throw new Error('来源消息在读取过程中发生变化，请重新查找');
      }
      versions.set(sectionId, page.sourceVersion);
      for (const { message } of page.items) {
        if (message.isDeleted || seen.has(message.id)) continue;
        seen.add(message.id);
        const original = message.content;
        const source = overrides?.[message.id] ?? original;
        const matches = findTextMatches(source, query);
        if (matches.length) results.push({ messageId: message.id, sectionId, original, source, matches });
      }
      cursor = page.nextCursor;
    } while (cursor !== null);
  }
  return { results, versions };
};

/** 频道版本变化时，只核对实际要替换的消息，其他消息的改动不影响该批。 */
export const verifyLogMatchSources = async (
  projectId: string, targets: LogFindResult[], versions: ReadonlyMap<string, number>, signal: AbortSignal,
) => {
  const bySection = new Map<string, LogFindResult[]>();
  for (const target of targets) {
    const group = bySection.get(target.sectionId) || [];
    group.push(target);
    bySection.set(target.sectionId, group);
  }
  const stale = new Set<string>();
  for (const [sectionId, group] of bySection) {
    signal.throwIfAborted();
    try {
      const page = await fetchLogSectionMessages(projectId, sectionId, null, 1, signal);
      signal.throwIfAborted();
      if (page.sourceVersion === versions.get(sectionId)) continue;
    } catch (error) {
      signal.throwIfAborted();
      if (isLogError(error, 'NOT_FOUND')) {
        group.forEach(({ messageId }) => stale.add(messageId));
        continue;
      }
      if (!isLogError(error, 'SOURCE_CHANGED')) throw error;
    }
    for (const { messageId, original } of group) {
      try {
        const context = await fetchLogMessageContext(projectId, sectionId, messageId, signal);
        signal.throwIfAborted();
        const message = context.items.find((item) => item.message.id === messageId)?.message;
        if (!message || message.isDeleted || message.content !== original) stale.add(messageId);
      } catch (error) {
        signal.throwIfAborted();
        if (!isLogError(error, 'NOT_FOUND', 'SOURCE_CHANGED')) throw error;
        stale.add(messageId);
      }
    }
  }
  return stale;
};

/** 在同步草稿 mutation 内重算覆盖；超限时不把候选结果写回草稿。 */
export const applyLogReplacements = (
  draft: LogDraft, targets: LogFindResult[], replacement: string, invalidSources: ReadonlySet<string>,
  currentMessage: (id: string) => Pick<LogProtocolMessage, 'content' | 'isDeleted'> | undefined,
) => {
  const writes: Record<string, string> = Object.create(null);
  const originals = new Map<string, string>();
  const stale = new Set(invalidSources);
  let count = 0;
  for (const target of targets) {
    const { messageId, source, original, matches } = target;
    const current = currentMessage(messageId);
    if (stale.has(messageId) || current?.isDeleted || (current && current.content !== original)
      || (draft.messageContentOverrides?.[messageId] ?? original) !== source) {
      stale.add(messageId);
      continue;
    }
    const next = replaceTextMatches(source, matches, replacement);
    if (next === null) stale.add(messageId);
    else if (next !== source) {
      // 后端不接受空白覆盖；空注释让删空的旧格式正文仍按原样显示为空。
      writes[messageId] = next === original || next.trim() ? next : `${next}<!-- -->`;
      originals.set(messageId, original);
      count += matches.filter((match) => match.original !== replacement).length;
    }
  }
  let record: ProofUndoRecord = {};
  if (!count) return { writes, stale, count, record, error: null };
  const candidate = { ...draft };
  record = commitProofApply(candidate, writes, (id) => originals.get(id));
  const error = proofDraftSizeError(candidate);
  if (!error) {
    if (candidate.messageContentOverrides) draft.messageContentOverrides = candidate.messageContentOverrides;
    else delete draft.messageContentOverrides;
  }
  return { writes, stale, count, record, error };
};
