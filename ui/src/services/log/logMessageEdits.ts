// LOG 中的正文编辑共用原消息命令；工程覆盖只用于兼容旧数据。
import { describeLogError, fetchLogMessageContext, fetchLogSectionMessages, isLogError } from './logApi';
import { updateMessageContent } from './logMessageCommands';
import type { LogDraft, LogProtocolMessage } from '@/types/log';

export interface LogMessageSource {
  messageId: string;
  sectionId: string;
  channelId: string;
  version: number;
  canEdit: boolean;
  original: string;
  source: string;
}

export interface LogMessageEdit extends LogMessageSource {
  content: string;
  count: number;
}

export interface LogMessageEditRecord {
  messageId: string;
  sectionId: string;
  channelId: string;
  before: string;
  after: string;
  beforeVersion: number;
  version: number;
  beforeOverride: string | undefined;
}

export type LogMessageSaved = (message: LogProtocolMessage, expectedOverride: string | undefined, restoredOverride?: string) => void;

/** 频道版本只用于决定是否重读目标；实际写入仍使用每条原消息的版本。 */
export const verifyLogMessageSources = async (
  projectId: string, targets: Pick<LogMessageSource, 'messageId' | 'sectionId' | 'original'>[],
  versions: ReadonlyMap<string, number>, signal: AbortSignal,
) => {
  const bySection = new Map<string, typeof targets>();
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

/** 仅清理/恢复本次涉及的旧覆盖，不覆盖同时发生的工程编辑。 */
export const setLogContentOverride = (draft: LogDraft, id: string, expected: string | undefined, value?: string) => {
  if (draft.messageContentOverrides?.[id] !== expected) return false;
  const overrides = { ...draft.messageContentOverrides };
  if (value === undefined) delete overrides[id];
  else overrides[id] = value;
  if (Object.keys(overrides).length) draft.messageContentOverrides = overrides;
  else delete draft.messageContentOverrides;
  return true;
};

export const applyLogMessageEdits = async (
  draft: () => LogDraft, edits: LogMessageEdit[],
  currentMessage: (id: string) => Pick<LogProtocolMessage, 'content' | 'isDeleted'> | undefined,
  signal: AbortSignal, onSaved: LogMessageSaved, progress?: (done: number) => void,
) => {
  const writes: Record<string, string> = Object.create(null);
  const record: LogMessageEditRecord[] = [];
  const stale = new Set<string>();
  let count = 0;
  let error: string | null = null;
  for (const [index, edit] of edits.entries()) {
    if (signal.aborted) break;
    progress?.(index + 1);
    const { messageId, original, source, content } = edit;
    const current = currentMessage(messageId);
    const beforeOverride = draft().messageContentOverrides?.[messageId];
    if (!edit.canEdit || !edit.channelId || !Number.isSafeInteger(edit.version) || edit.version < 1
      || current?.isDeleted || (current && current.content !== original) || (beforeOverride ?? original) !== source) {
      stale.add(messageId);
      continue;
    }
    if (content === source) continue;
    if (!content.trim()) {
      error = '部分修改会使原消息正文为空，已跳过；如需移除消息请使用撤回或删除';
      continue;
    }
    try {
      const message = await updateMessageContent({ channelId: edit.channelId, messageId, expectedVersion: edit.version }, content);
      if (!message || message.id !== messageId || typeof message.version !== 'number') {
        throw new Error('未收到原消息保存结果，请重新加载后确认');
      }
      writes[messageId] = message.content;
      record.push({ messageId, sectionId: edit.sectionId, channelId: edit.channelId,
        before: original, after: message.content, beforeVersion: edit.version, version: message.version, beforeOverride });
      count += edit.count;
      // 原消息命令不可取消，查询取消后仍同步已确认的保存。
      onSaved(message, beforeOverride);
    } catch (cause) {
      if (isLogError(cause, 'VERSION_CONFLICT', 'NOT_FOUND', 'PERMISSION_DENIED')) stale.add(messageId);
      else {
        error = describeLogError(cause, '保存原消息失败');
        break;
      }
    }
  }
  return { writes, stale, count, record, error };
};

/** 撤销也编辑原消息；后续手工编辑推进版本后，CAS 会阻止覆盖。 */
export const undoLogMessageEdits = async (
  record: LogMessageEditRecord[], draft: () => LogDraft, signal: AbortSignal, onSaved: LogMessageSaved,
  progress?: (done: number) => void,
) => {
  const messages = new Map<string, LogProtocolMessage>();
  let skipped = 0;
  let error: string | null = null;
  const remaining: LogMessageEditRecord[] = [];
  for (let index = 0; index < record.length; index++) {
    if (signal.aborted) {
      remaining.push(...record.slice(index));
      break;
    }
    progress?.(index + 1);
    const item = record[index];
    if (draft().messageContentOverrides?.[item.messageId] !== undefined) {
      skipped++;
      continue;
    }
    let saved = false;
    try {
      const message = await updateMessageContent({ channelId: item.channelId, messageId: item.messageId, expectedVersion: item.version }, item.before);
      if (!message || message.id !== item.messageId || typeof message.version !== 'number') {
        throw new Error('未收到原消息保存结果，请重新加载后确认');
      }
      saved = true;
      messages.set(message.id, message);
      onSaved(message, undefined, item.beforeOverride);
    } catch (cause) {
      if (isLogError(cause, 'VERSION_CONFLICT', 'NOT_FOUND', 'PERMISSION_DENIED')) skipped++;
      else {
        error = describeLogError(cause, '撤销原消息修改失败');
        remaining.push(...record.slice(index + (saved ? 1 : 0)));
        break;
      }
    }
  }
  return { restored: messages.size, skipped, error, remaining, messages };
};

/** 撤销较新的批次后，只为确实连续的旧编辑推进撤销版本。 */
export const advanceLogUndoVersions = (
  older: LogMessageEditRecord[], undone: LogMessageEditRecord[], messages: ReadonlyMap<string, LogProtocolMessage>,
) => older.map((record) => {
  const prior = undone.find((item) => item.messageId === record.messageId && item.beforeVersion === record.version && item.before === record.after);
  const message = messages.get(record.messageId);
  return prior && message?.version !== undefined ? { ...record, version: message.version } : record;
});
