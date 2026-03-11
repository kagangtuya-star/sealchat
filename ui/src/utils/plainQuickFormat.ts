const inlineCodePattern = /`([^`\n]+)`/g;
const codeFenceLiteralPattern = /```([\s\S]*?)```/g;
const linkPattern = /\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)/gi;
const boldPattern = /\*\*([^\n*][^*\n]*?)\*\*/g;
const italicPattern = /(^|[^*])\*([^*\n]+)\*/g;

const normalizeNewlines = (value: string) => value.replace(/\r\n/g, '\n').replace(/\r/g, '\n');

const isSafeHttpUrl = (value: string) => {
  const normalized = value.replace(/&amp;/g, '&').trim();
  if (!/^https?:\/\//i.test(normalized)) {
    return false;
  }
  try {
    const parsed = new URL(normalized);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
};

const processInlineFromEscaped = (escapedInput: string) => {
  let text = escapedInput;
  const codeTokens: Array<{ token: string; html: string }> = [];
  const linkTokens: Array<{ token: string; html: string }> = [];

  text = text.replace(inlineCodePattern, (_, code: string) => {
    const token = `__QF_CODE_${codeTokens.length}__`;
    codeTokens.push({ token, html: `<code>${code}</code>` });
    return token;
  });

  text = text.replace(linkPattern, (full: string, label: string, url: string) => {
    if (!isSafeHttpUrl(url)) {
      return full;
    }
    const token = `__QF_LINK_${linkTokens.length}__`;
    linkTokens.push({
      token,
      html: `<a href="${url}" class="text-blue-500" target="_blank" rel="noopener noreferrer">${label}</a>`,
    });
    return token;
  });

  text = text.replace(boldPattern, '<strong>$1</strong>');
  text = text.replace(italicPattern, (_match, prefix: string, body: string) => `${prefix}<em>${body}</em>`);

  linkTokens.forEach((entry) => {
    text = text.split(entry.token).join(entry.html);
  });

  codeTokens.forEach((entry) => {
    text = text.split(entry.token).join(entry.html);
  });

  return text;
};

export const renderQuickFormatHtmlFromEscaped = (escapedInput: string) => {
  if (!escapedInput) {
    return '';
  }

  let text = normalizeNewlines(escapedInput);
  const fenceTokens: Array<{ token: string; html: string }> = [];

  text = text.replace(codeFenceLiteralPattern, (segment: string) => {
    const token = `__QF_FENCE_${fenceTokens.length}__`;
    fenceTokens.push({ token, html: segment });
    return token;
  });

  text = processInlineFromEscaped(text);

  fenceTokens.forEach((entry) => {
    text = text.split(entry.token).join(entry.html);
  });

  text = text.replace(/\n/g, '<br />');

  return text;
};

const normalizeQuickFormatText = (value: string) => value.replace(/\u00a0/g, ' ');

const isBlockNode = (tagName: string) => ['DIV', 'P', 'PRE', 'BLOCKQUOTE', 'LI'].includes(tagName);

const serializeQuickFormatNode = (node: Node, inCodeBlock = false): string => {
  if (node.nodeType === Node.TEXT_NODE) {
    return normalizeQuickFormatText(node.textContent || '');
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return '';
  }

  const el = node as HTMLElement;
  const tag = el.tagName.toUpperCase();
  const children = Array.from(el.childNodes).map((child) => serializeQuickFormatNode(child, inCodeBlock)).join('');

  switch (tag) {
    case 'BR':
      return '\n';
    case 'IMG':
      return '[图片]';
    case 'AT': {
      const name = (el.getAttribute('name') || '').trim();
      const id = (el.getAttribute('id') || '').trim();
      return `@${name || id || '用户'}`;
    }
    case 'STRONG':
    case 'B':
      return children ? `**${children}**` : '';
    case 'EM':
    case 'I':
      return children ? `*${children}*` : '';
    case 'S':
    case 'STRIKE':
    case 'DEL':
      return children ? `~~${children}~~` : '';
    case 'CODE':
      return inCodeBlock ? children : (children ? `\`${children}\`` : '');
    case 'PRE': {
      const code = children.replace(/\n+$/g, '');
      return code ? `\`\`\`\n${code}\n\`\`\`` : '';
    }
    case 'A': {
      const href = (el.getAttribute('href') || '').trim();
      return href ? `[${children}](${href})` : children;
    }
    default:
      return children;
  }
};

export const restoreQuickFormatTextFromHtml = (htmlInput: string) => {
  if (!htmlInput || !/[<>]/.test(htmlInput)) {
    return htmlInput || '';
  }

  const container = document.createElement('div');
  container.innerHTML = htmlInput;
  const parts: string[] = [];

  Array.from(container.childNodes).forEach((node) => {
    const chunk = serializeQuickFormatNode(node, false);
    if (!chunk) {
      return;
    }
    parts.push(chunk);
    if (node.nodeType === Node.ELEMENT_NODE) {
      const tag = (node as HTMLElement).tagName.toUpperCase();
      if (isBlockNode(tag) && !chunk.endsWith('\n')) {
        parts.push('\n');
      }
    }
  });

  return parts.join('').replace(/\n{3,}/g, '\n\n').replace(/\n+$/g, '');
};
