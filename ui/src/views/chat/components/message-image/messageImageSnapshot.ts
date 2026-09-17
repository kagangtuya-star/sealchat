import DOMPurify from 'dompurify';
import Element from '@satorijs/element';
import type { Message } from '@satorijs/protocol';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { isBotCommandLikeContent, renderBotCommandTextAsHtml } from '@/utils/botCommand';
import { renderQuickFormatHtmlFromEscaped } from '@/utils/plainQuickFormat';
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render';
import { contentEscape } from '@/utils/tools';
import {
  resolveWhisperTargetIdsForMerge,
  shouldMergeNeighborMessages,
  shouldRenderWhisperLabel,
} from '../../messageMerge';

export type MessageImageSnapshotTone = 'ic' | 'ooc' | 'archived';

export interface MessageImageSnapshotEntry {
  id: string;
  contentHtml: string;
  tone: MessageImageSnapshotTone;
}

export interface MessageImageSnapshotGroup {
  key: string;
  senderName: string;
  avatarUrl: string;
  senderColor?: string;
  whisperLabel?: string;
  entries: MessageImageSnapshotEntry[];
}

export interface MessageImageSnapshotPalette {
  background: string;
  primaryText: string;
  secondaryText: string;
  subtleBorder: string;
  codeBackground: string;
  link: string;
}

export interface BuildMessageImageSnapshotOptions {
  botCommandPrefixes?: unknown;
  currentUserId?: string;
  resolveUserName?: (userId: string) => string;
}

export const MESSAGE_IMAGE_SNAPSHOT_WIDTH = 380;

export const resolveMessageImageSnapshotPalette = (isNight: boolean): MessageImageSnapshotPalette => (
  isNight
    ? {
      background: '#3f3f46',
      primaryText: '#e5e7eb',
      secondaryText: '#8f949e',
      subtleBorder: 'rgba(255,255,255,0.11)',
      codeBackground: '#202228',
      link: '#7fb4ff',
    }
    : {
      background: '#ffffff',
      primaryText: '#202124',
      secondaryText: '#6b7280',
      subtleBorder: 'rgba(15,23,42,0.10)',
      codeBackground: '#f3f4f6',
      link: '#2563eb',
    }
);

const resolveSenderName = (message: Message): string => {
  const raw = message as any;
  return String(
    raw?.whisperMeta?.senderMemberName
    || raw?.identity?.displayName
    || raw?.sender_identity_name
    || raw?.sender_member_name
    || raw?.member?.nick
    || raw?.user?.nick
    || raw?.user?.name
    || raw?.whisperMeta?.senderUserNick
    || raw?.whisperMeta?.senderUserName
    || '未知成员',
  );
};

const resolveSenderUserId = (message: Message): string => {
  const raw = message as any;
  return String(
    raw?.user?.id
    || raw?.member?.user?.id
    || raw?.member?.userId
    || raw?.member?.user_id
    || raw?.sender_user_id
    || raw?.senderUserId
    || raw?.user_id
    || raw?.whisperMeta?.senderUserId
    || '',
  ).trim();
};

const resolveAvatarSource = (message: Message): string => {
  const raw = message as any;
  const candidates = [
    raw?.identity?.avatarAttachment,
    raw?.identity?.avatarAttachmentId,
    raw?.sender_identity_avatar_id,
    raw?.sender_identity_avatar,
    raw?.senderIdentityAvatarID,
    raw?.senderIdentityAvatarId,
    raw?.member?.avatar,
    raw?.user?.avatar,
  ];
  const source = candidates.find((candidate) => typeof candidate === 'string' && candidate.trim());
  return source ? (resolveAttachmentUrl(source) || source) : '';
};

const resolveSenderColor = (message: Message): string | undefined => {
  const raw = message as any;
  const color = String(raw?.identity?.color || raw?.sender_identity_color || '').trim();
  return /^#[0-9a-f]{3,8}$/i.test(color) ? color : undefined;
};

const resolveTone = (message: Message): MessageImageSnapshotTone => {
  const raw = message as any;
  if (raw?.isArchived || raw?.is_archived) return 'archived';
  return String(raw?.icMode ?? raw?.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic';
};

const renderLegacyContent = (content: string, botCommandPrefixes?: unknown): string => {
  try {
    const items = Element.parse(content);
    const fragments: string[] = [];
    for (const item of items) {
      if (item.type === 'img') {
        if (item.attrs.src) item.attrs.src = resolveAttachmentUrl(item.attrs.src) || item.attrs.src;
        fragments.push(item.toString());
        continue;
      }
      if (item.type === 'audio') {
        fragments.push('<span>[语音]</span>');
        continue;
      }
      if (item.type === 'at') {
        const name = String(item.attrs?.name || item.attrs?.id || '用户');
        fragments.push(`<span class="mention-capsule">@${contentEscape(name)}</span>`);
        continue;
      }
      if (item.type === 'text') {
        const raw = typeof item.attrs?.content === 'string' ? item.attrs.content : item.toString();
        fragments.push(renderQuickFormatHtmlFromEscaped(contentEscape(String(raw || '')), {
          disableAllFormatting: isBotCommandLikeContent(content, botCommandPrefixes),
        }));
        continue;
      }
      fragments.push(item.toString());
    }
    return fragments.join('');
  } catch {
    return renderQuickFormatHtmlFromEscaped(
      contentEscape(content),
      {
        disableAllFormatting: isBotCommandLikeContent(
          content,
          botCommandPrefixes,
        ),
      },
    );
  }
};

const renderMessageContent = (message: Message, options: BuildMessageImageSnapshotOptions): string => {
  const content = typeof message.content === 'string' ? message.content : '';
  let html = '';
  if (isBotCommandLikeContent(content, options.botCommandPrefixes)) {
    html = renderBotCommandTextAsHtml(content);
  } else if (isTipTapJson(content)) {
    html = tiptapJsonToHtml(content, {
      imageClass: 'message-image-snapshot__inline-image',
      linkClass: 'message-image-snapshot__link',
      attachmentResolver: resolveAttachmentUrl,
    });
  } else {
    html = renderLegacyContent(content, options.botCommandPrefixes);
  }
  return String(DOMPurify.sanitize(html));
};

const collectWhisperTargetNames = (
  message: Message,
  options: BuildMessageImageSnapshotOptions,
): string[] => {
  const raw = message as any;
  const ids = resolveWhisperTargetIdsForMerge(raw);
  const metaNames = Array.isArray(raw?.whisperMeta?.targetDisplayNames)
    ? raw.whisperMeta.targetDisplayNames.map((name: unknown) => String(name || '').trim()).filter(Boolean)
    : [];
  const targetList = raw?.whisperToIds || raw?.whisper_to_ids || raw?.whisperTargets || raw?.whisper_targets;
  const listNames = Array.isArray(targetList)
    ? targetList.map((target: any) => typeof target === 'string'
      ? ''
      : String(target?.nick || target?.name || target?.username || '').trim())
    : [];
  const directName = String(
    raw?.whisperMeta?.targetMemberName
    || raw?.whisperTo?.nick
    || raw?.whisperTo?.name
    || raw?.whisper_to?.nick
    || raw?.whisper_to?.name
    || raw?.whisper_target?.nick
    || raw?.whisper_target?.name
    || raw?.whisperMeta?.targetUserNick
    || raw?.whisperMeta?.targetUserName
    || '',
  ).trim();

  return ids.map((id, index) => (
    metaNames[index]
    || listNames[index]
    || options.resolveUserName?.(id)
    || id
  )).filter(Boolean).concat(ids.length === 0 && directName ? [directName] : []);
};

const buildWhisperLabel = (
  message: Message,
  options: BuildMessageImageSnapshotOptions,
): string | undefined => {
  if (!shouldRenderWhisperLabel(message as any, false)) return undefined;
  const senderName = resolveSenderName(message);
  const senderUserId = resolveSenderUserId(message);
  const targets = collectWhisperTargetNames(message, options);
  if (senderUserId && senderUserId === options.currentUserId) {
    return targets.length > 0 ? `🔒 悄悄话给 ${targets.join('、')}` : '🔒 悄悄话';
  }
  return senderName ? `🔒 来自 @${senderName} 的悄悄话` : '🔒 悄悄话';
};

const resolveRoleKey = (message: Message): string => {
  const raw = message as any;
  return String(
    raw?.senderRoleId
    || raw?.sender_role_id
    || raw?.sender_identity_id
    || raw?.identity?.id
    || raw?.member?.id
    || raw?.member?.member_id
    || raw?.sender_member_id
    || resolveSenderUserId(message),
  ).trim();
};

const toMergeComparable = (message: Message) => {
  const raw = message as any;
  const decorations = raw?.identity?.avatarDecorations
    || raw?.sender_identity_decoration
    || raw?.identity?.avatarDecoration
    || null;
  return {
    ...raw,
    roleKey: resolveRoleKey(message),
    sceneKey: String(raw?.icMode ?? raw?.ic_mode ?? 'ic').toLowerCase(),
    avatarMergeKey: `${resolveAvatarSource(message)}__${JSON.stringify(decorations)}`,
  };
};

export const buildMessageImageSnapshotGroups = (
  selectedMessages: Message[],
  allRows: Message[],
  options: BuildMessageImageSnapshotOptions = {},
): MessageImageSnapshotGroup[] => {
  const originalIndexes = new Map<string, number>();
  allRows.forEach((row, index) => {
    if (row.id) originalIndexes.set(row.id, index);
  });

  const groups: MessageImageSnapshotGroup[] = [];
  let previousMessage: Message | null = null;
  let previousOriginalIndex = -1;

  selectedMessages.forEach((selectedMessage, selectedIndex) => {
    const id = String(selectedMessage.id || `selected-${selectedIndex}`);
    const currentOriginalIndex = selectedMessage.id
      ? (originalIndexes.get(selectedMessage.id) ?? -1)
      : -1;
    const entry: MessageImageSnapshotEntry = {
      id,
      contentHtml: renderMessageContent(selectedMessage, options),
      tone: resolveTone(selectedMessage),
    };
    const canMerge = Boolean(
      previousMessage
      && currentOriginalIndex >= 0
      && currentOriginalIndex === previousOriginalIndex + 1
      && shouldMergeNeighborMessages(
        toMergeComparable(previousMessage),
        toMergeComparable(selectedMessage),
      ),
    );

    if (canMerge) {
      groups[groups.length - 1].entries.push(entry);
    } else {
      groups.push({
        key: `${id}-${selectedIndex}`,
        senderName: resolveSenderName(selectedMessage),
        avatarUrl: resolveAvatarSource(selectedMessage),
        senderColor: resolveSenderColor(selectedMessage),
        whisperLabel: buildWhisperLabel(selectedMessage, options),
        entries: [entry],
      });
    }

    previousMessage = selectedMessage;
    previousOriginalIndex = currentOriginalIndex;
  });

  return groups;
};
