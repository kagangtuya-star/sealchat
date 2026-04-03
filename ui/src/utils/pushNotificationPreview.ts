const SPOILER_PLACEHOLDER = '【隐藏】';
const INLINE_IMAGE_PATTERN = /\[\[(?:图片:[^\]]+|img:id:[^\]]+)\]\]/g;
const HTML_IMAGE_PATTERN = /<img\s+[^>]*>/gi;
const HTML_BREAK_PATTERN = /<br\s*\/?>/gi;
const HTML_BLOCK_CLOSE_PATTERN = /<\/(p|div|li|h[1-6]|blockquote|pre)>/gi;
const HTML_TAG_PATTERN = /<[^>]*>/g;
const AT_TOKEN_FLEX_REGEX = /<at\s+id=(?:\\?"|')([^"'>]+)(?:\\?"|')(?:\s+name=(?:\\?"|')([^"']*)(?:\\?"|'))?\s*\/?\s*>/g;
const SPOILER_HTML_PATTERN = /<span\b(?=[^>]*(?:data-spoiler\b|class=(?:"[^"]*\btiptap-spoiler\b[^"]*"|'[^']*\btiptap-spoiler\b[^']*')))[^>]*>[\s\S]*?<\/span>/gi;

interface TipTapNode {
  type?: string;
  attrs?: Record<string, any>;
  content?: TipTapNode[];
  text?: string;
  marks?: Array<{ type?: string; attrs?: Record<string, any> }>;
}

const contentUnescape = (value: string): string => {
  let current = String(value || '');
  for (let i = 0; i < 4; i += 1) {
    const next = current
      .replace(/&quot;/g, '"')
      .replace(/&#039;/g, "'")
      .replace(/&apos;/g, "'")
      .replace(/&gt;/g, '>')
      .replace(/&lt;/g, '<')
      .replace(/&amp;/g, '&');
    if (next === current) {
      return next;
    }
    current = next;
  }
  return current;
};

const isTipTapJson = (content: string): boolean => {
  if (!content || typeof content !== 'string') {
    return false;
  }
  try {
    const parsed = JSON.parse(content);
    return !!parsed && typeof parsed === 'object' && parsed.type === 'doc';
  } catch {
    return false;
  }
};

const decodeAtTokenText = (value: string) => contentUnescape(value);

const replaceAtTokensWithDisplayText = (value: string) => {
  AT_TOKEN_FLEX_REGEX.lastIndex = 0;
  return value.replace(AT_TOKEN_FLEX_REGEX, (_full, id: string, name: string) => {
    const display = decodeAtTokenText(name || id || '用户');
    return `@${display}`;
  });
};

const hasSpoilerMark = (node: TipTapNode): boolean => {
  if (!Array.isArray(node.marks) || node.marks.length === 0) {
    return false;
  }
  return node.marks.some((mark) => String(mark?.type || '').trim().toLowerCase() === 'spoiler');
};

const extractTipTapPreviewText = (node: TipTapNode): string => {
  if (!node) {
    return '';
  }

  if (typeof node.text === 'string') {
    if (hasSpoilerMark(node)) {
      return SPOILER_PLACEHOLDER;
    }
    return replaceAtTokensWithDisplayText(node.text);
  }

  const nodeType = String(node.type || '').trim().toLowerCase();
  if (nodeType === 'hardbreak') {
    return '\n';
  }
  if (nodeType === 'mention' || nodeType === 'satorimention') {
    const mentionId = String(node.attrs?.id || '').trim();
    const mentionName = String(node.attrs?.name || '').trim();
    return `@${mentionName || mentionId || '用户'}`;
  }
  if (nodeType === 'image') {
    return '[图片]';
  }

  if (Array.isArray(node.content) && node.content.length > 0) {
    const joined = node.content.map((child) => extractTipTapPreviewText(child)).join('');
    if (nodeType === 'paragraph' || nodeType === 'heading' || nodeType === 'listitem') {
      return `${joined}\n`;
    }
    return joined;
  }

  return '';
};

const collapseSpoilerPlaceholders = (value: string): string => value.replace(
  new RegExp(`(?:\\s*${SPOILER_PLACEHOLDER}\\s*){2,}`, 'g'),
  SPOILER_PLACEHOLDER,
);

const normalizePreviewWhitespace = (value: string): string => collapseSpoilerPlaceholders(value)
  .replace(/\s+/g, ' ')
  .trim();

const extractHtmlPreviewText = (content: string): string => {
  const decoded = contentUnescape(content);
  return normalizePreviewWhitespace(
    replaceAtTokensWithDisplayText(decoded)
      .replace(INLINE_IMAGE_PATTERN, '[图片]')
      .replace(HTML_IMAGE_PATTERN, '[图片]')
      .replace(SPOILER_HTML_PATTERN, SPOILER_PLACEHOLDER)
      .replace(HTML_BREAK_PATTERN, '\n')
      .replace(HTML_BLOCK_CLOSE_PATTERN, '\n')
      .replace(HTML_TAG_PATTERN, ''),
  );
};

export const extractPushNotificationPreviewText = (content: string): string => {
  if (!content) {
    return '';
  }

  if (isTipTapJson(content)) {
    try {
      return normalizePreviewWhitespace(extractTipTapPreviewText(JSON.parse(content)).replace(/\n+$/g, ''));
    } catch {
      return '';
    }
  }

  return extractHtmlPreviewText(content);
};
