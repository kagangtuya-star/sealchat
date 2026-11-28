import { fetchLogSectionMessages } from '@/services/log/logApi';
import { applyLogMessageEdits, type LogMessageEdit, type LogMessageSaved, type LogMessageSource } from '@/services/log/logMessageEdits';
import { findTextMatches, replaceTextMatches, type LogTextMatch } from '@/services/log/proofread/logProofread';
import type { LogDraft, LogProtocolMessage } from '@/types/log';

export { setLogContentOverride, undoLogMessageEdits as undoLogReplacements, verifyLogMessageSources as verifyLogMatchSources } from '@/services/log/logMessageEdits';
export type { LogMessageEditRecord as LogReplacementRecord } from '@/services/log/logMessageEdits';

const PAGE_LIMIT = 100;

export interface LogFindResult extends LogMessageSource {
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
      for (const { message, version, capabilities } of page.items) {
        if (message.isDeleted || seen.has(message.id)) continue;
        seen.add(message.id);
        const original = message.content;
        const source = overrides?.[message.id] ?? original;
        const matches = findTextMatches(source, query);
        if (matches.length) results.push({ messageId: message.id, sectionId, original, source, matches,
          channelId: message.channel?.id || '', version, canEdit: capabilities.edit });
      }
      cursor = page.nextCursor;
    } while (cursor !== null);
  }
  return { results, versions };
};

/** 查找替换只规划文本变化，保存与撤销复用原消息编辑服务。 */
export const applyLogReplacements = async (
  draft: () => LogDraft, targets: LogFindResult[], replacement: string, invalidSources: ReadonlySet<string>,
  currentMessage: (id: string) => Pick<LogProtocolMessage, 'content' | 'isDeleted'> | undefined,
  signal: AbortSignal, onSaved: LogMessageSaved,
) => {
  const edits: LogMessageEdit[] = [];
  const stale = new Set(invalidSources);
  for (const target of targets) {
    if (stale.has(target.messageId)) continue;
    const content = replaceTextMatches(target.source, target.matches, replacement);
    if (content === null) stale.add(target.messageId);
    else edits.push({ ...target, content, count: target.matches.filter((match) => match.original !== replacement).length });
  }
  const result = await applyLogMessageEdits(draft, edits, currentMessage, signal, onSaved);
  return { ...result, stale: new Set([...stale, ...result.stale]) };
};
