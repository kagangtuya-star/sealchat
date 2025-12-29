const ATTACHMENT_TOKEN_PATTERN = /^[0-9A-Za-z_-]+$/;
const IMAGE_TOKEN_PATTERN = /!\[([^\]]*)\]\(id:([^)]+)\)/;

export const isValidAttachmentToken = (token: string): boolean => {
  return ATTACHMENT_TOKEN_PATTERN.test(token);
};

export const createImageTokenRegex = (): RegExp => {
  return new RegExp(IMAGE_TOKEN_PATTERN.source, 'g');
};

export const stripValidImageTokens = (text: string): string => {
  if (!text) return '';
  const regex = createImageTokenRegex();
  return text.replace(regex, (match, _alt, token) => {
    return isValidAttachmentToken(token) ? '' : match;
  });
};

export const clampTextWithImageTokens = (text: string, maxLength: number): string => {
  if (!text || maxLength <= 0) return text ? '' : '';
  const regex = createImageTokenRegex();
  let result = '';
  let count = 0;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = regex.exec(text)) !== null) {
    const [full, _alt, token] = match;
    const before = text.slice(lastIndex, match.index);
    if (before) {
      const remaining = maxLength - count;
      if (remaining <= 0) return result;
      if (before.length > remaining) {
        result += before.slice(0, remaining);
        return result;
      }
      result += before;
      count += before.length;
    }

    if (isValidAttachmentToken(token)) {
      result += full;
    } else {
      const remaining = maxLength - count;
      if (remaining <= 0) return result;
      if (full.length > remaining) {
        result += full.slice(0, remaining);
        return result;
      }
      result += full;
      count += full.length;
    }

    lastIndex = match.index + full.length;
  }

  if (lastIndex < text.length) {
    const remaining = maxLength - count;
    if (remaining > 0) {
      result += text.slice(lastIndex, lastIndex + remaining);
    }
  }

  return result;
};
