// 对既有消息 WebSocket 命令的显式频道适配：复用聊天 store 的唯一连接，
// 但不读取/修改 curChannel，也不触发聊天页的频道切换。
import { nanoid } from 'nanoid';
import { useChatStore } from '@/stores/chat';
import { LogDomainError } from './logApi';
import type {
  LogAppearancePatch,
  LogAppearancePreview,
  LogAppearanceSelection,
  LogElementPlacement,
  LogProtocolMessage,
} from '@/types/log';

const WS_ERROR_CODE = /^([A-Z][A-Z_]{2,})(?::\s*(.*))?$/s;

/** WS 回包错误是字符串（服务端 `CODE: 消息`）；在这里一次性转换为领域错误。 */
export const toCommandError = (error: unknown): LogDomainError => {
  if (error instanceof LogDomainError) return error;
  const raw = error instanceof Error ? error.message : String(error || '');
  const match = WS_ERROR_CODE.exec(raw.trim());
  if (match) {
    return new LogDomainError(match[1], match[2]?.trim() || defaultCommandMessage(match[1]));
  }
  if (/timeout|not connected|connection replaced/i.test(raw)) {
    return new LogDomainError('NETWORK_ERROR', '连接中断，操作结果未知，请重新加载后确认');
  }
  return new LogDomainError('COMMAND_FAILED', raw || '操作失败');
};

const defaultCommandMessage = (code: string) => ({
  VERSION_CONFLICT: '消息已被修改，请先查看最新内容',
  PERMISSION_DENIED: '没有权限修改这条消息',
  INVALID_IDENTITY: '目标角色或差分不可用于该消息',
  INVALID_PATCH: '外观修改无效',
  NOT_FOUND: '消息不存在或已删除',
  PREVIEW_EXPIRED: '预览已过期，请重新预览',
} as Record<string, string>)[code] || '操作失败';

export const newClientOpId = (prefix: string) => `log-${prefix}-${nanoid(16)}`;

const send = async <T>(api: string, data: Record<string, unknown>, timeoutMs?: number): Promise<T> => {
  const chat = useChatStore();
  await chat.ensureConnectionReady();
  try {
    const resp = await chat.sendAPI<{ data: T }>(api, data as any, timeoutMs ? { timeoutMs } : undefined);
    return resp?.data as T;
  } catch (error) {
    throw toCommandError(error);
  }
};

export interface MessageTarget {
  channelId: string;
  messageId: string;
  expectedVersion: number;
  clientOpId?: string;
}

const base = (t: MessageTarget, prefix: string) => ({
  channel_id: t.channelId,
  message_id: t.messageId,
  expected_version: t.expectedVersion,
  client_op_id: t.clientOpId || newClientOpId(prefix),
});

/** 只改正文：省略 ic_mode / whisper_to_ids / identity 字段，服务端保持原值。 */
export const updateMessageContent = async (t: MessageTarget, content: string) => {
  const result = await send<{ message?: LogProtocolMessage }>('message.update', { ...base(t, 'edit'), content });
  return result?.message || null;
};

/** 切换角色/差分；正文原样回传，避免空正文被拒或被改写。 */
export const updateMessageIdentity = async (t: MessageTarget, content: string, identityId: string, variantId: string | null, ownerUserId?: string) => {
  const result = await send<{ message?: LogProtocolMessage }>('message.update', {
    ...base(t, 'identity'),
    content,
    identity_id: identityId,
    identity_variant_id: variantId ?? '',
    ...(ownerUserId ? { identity_owner_user_id: ownerUserId } : {}),
  });
  return result?.message || null;
};

/** 单条历史外观快照：省略字段保持，空字符串清除；不修改角色资料。 */
export const updateMessageAppearance = async (t: MessageTarget, patch: Pick<LogAppearancePatch, 'name' | 'color' | 'avatar_attachment_id'>) => {
  const result = await send<{ message?: LogProtocolMessage }>('message.update', { ...base(t, 'appearance'), appearance_patch: patch });
  return result?.message || null;
};

/** 删除原消息（message.remove）：真实影响聊天记录。 */
export const removeMessage = (t: MessageTarget) =>
  send<{ success?: boolean; message_ids?: string[] } | null>('message.remove', base(t, 'remove'));

export interface BatchMessageTarget {
  channelId: string;
  /** 每条消息的已知版本；服务端按全部版本做 CAS，任一冲突则整体不生效。 */
  items: Array<{ messageId: string; expectedVersion: number }>;
}

const batchBase = (t: BatchMessageTarget, prefix: string) => ({
  channel_id: t.channelId,
  message_ids: t.items.map((it) => it.messageId),
  expected_versions: Object.fromEntries(t.items.map((it) => [it.messageId, it.expectedVersion])),
  client_op_id: newClientOpId(prefix),
});

/** 批量删除原消息（message.remove + message_ids）：同一事务，全部成功或全部失败。 */
export const removeMessagesBatch = async (t: BatchMessageTarget) => {
  const result = await send<{ success?: boolean; message_ids?: string[] } | null>('message.remove', batchBase(t, 'remove-batch'));
  if (!result?.success || (result.message_ids?.length || 0) !== t.items.length) {
    throw new LogDomainError('COMMAND_FAILED', '删除结果与所选消息不一致，请重新加载后确认');
  }
  return result;
};

/**
 * 批量移动（message.relocate.batch）：每组按原有相对顺序放到各自目标消息之后/之前，
 * 服务端在一个事务内完成权限、版本与 display order 调整。
 */
export const relocateMessagesBatch = async (
  t: BatchMessageTarget,
  plan: { placement: 'before' | 'after'; groups: Array<{ messageIds: string[]; targetMessageId: string }> },
  logDraft?: { projectId: string; sectionId: string; expectedVersion: number; placements: LogElementPlacement[]; visualStep?: boolean },
) => {
  const base = batchBase(t, 'relocate-batch');
  const single = plan.groups.length === 1 ? plan.groups[0] : null;
  const payload: Record<string, unknown> = {
    channel_id: base.channel_id,
    expected_versions: base.expected_versions,
    client_op_id: base.client_op_id,
    placement: plan.placement,
    ...(logDraft ? { log_draft: logDraft } : {}),
    ...(single
      ? { message_ids: single.messageIds, target_message_id: single.targetMessageId }
      : { groups: plan.groups.map((g) => ({ message_ids: g.messageIds, target_message_id: g.targetMessageId })) }),
  };
  const result = await send<{ message_ids?: string[]; draft_version?: number } | null>('message.relocate.batch', payload);
  if (result?.message_ids?.length !== t.items.length || (logDraft && result.draft_version !== logDraft.expectedVersion + 1)) {
    throw new LogDomainError('COMMAND_FAILED', '移动结果与所选消息不一致，请重新加载后确认');
  }
  return result;
};

/**
 * 频道内移动：after_id 为移动后紧邻的上一条，before_id 为紧邻的下一条
 * （与服务端 resolveMessageDisplayOrderForPlacement 一致）。
 */
export const reorderMessage = (t: MessageTarget, placement: { afterId?: string; beforeId?: string }) =>
  send<{ message_id: string; display_order: number } | null>('message.reorder', {
    ...base(t, 'reorder'),
    after_id: placement.afterId || '',
    before_id: placement.beforeId || '',
  });

/** LOG 落点：原消息排序与元素锚点在同一事务中提交，普通聊天排序不受影响。 */
export const reorderLogMessage = (
  t: MessageTarget,
  placement: { afterId?: string; beforeId?: string },
  logDraft: { projectId: string; sectionId: string; expectedVersion: number; placements: LogElementPlacement[] },
) => send<{ draft_version: number }>('message.reorder.log', {
  ...base(t, 'reorder-log'),
  after_id: placement.afterId || '',
  before_id: placement.beforeId || '',
  log_draft: logDraft,
});

export const previewAppearanceBatch = (input: { channelId: string; selection: LogAppearanceSelection; patch: LogAppearancePatch }) => {
  const selection: Record<string, unknown> = {
    from_time: input.selection.from_time,
    to_time: input.selection.to_time,
    identity_id: input.selection.identity_id,
    shared_identity_id: input.selection.shared_identity_id,
    variant_id: input.selection.variant_id,
  };
  return send<LogAppearancePreview>('message.appearance.preview', { channel_id: input.channelId, selection, patch: input.patch }, 60000);
};

export const applyAppearanceBatch = (previewId: string, clientOpId: string) =>
  send<{ job_id: string }>('message.appearance.apply', { preview_id: previewId, client_op_id: clientOpId });

export interface LogWorldChannel { id: string; name: string }

/** 读取世界频道树（只读，不写入聊天 store 的频道树缓存）。 */
export const listWorldChannels = async (worldId: string): Promise<LogWorldChannel[]> => {
  const data = await send<{ data?: unknown[] }>('channel.list', { world_id: worldId, worldId });
  const out: LogWorldChannel[] = [];
  const visit = (list: unknown) => {
    if (!Array.isArray(list)) return;
    for (const entry of list) {
      if (!entry || typeof entry !== 'object') continue;
      const item = entry as Record<string, unknown>;
      if (typeof item.id === 'string' && item.id && item.is_private !== true && item.isPrivate !== true) {
        out.push({ id: item.id, name: String(item.name || item.id) });
      }
      visit(item.children);
    }
  };
  visit(data?.data);
  return out;
};
