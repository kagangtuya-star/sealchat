export interface HybridImageMarkerInfo {
  markerId: string;
  start: number;
  end: number;
}

const IMAGE_TOKEN_REGEX = /\[\[图片:([^\]]+)\]\]/g;

export const HYBRID_INPUT_CARET_ANCHOR_CLASS = 'hybrid-input__caret-anchor';

export const buildHybridCaretAnchorHtml = () =>
  `<span class="${HYBRID_INPUT_CARET_ANCHOR_CLASS}">\u200B</span>`;

export const findImageMarkerAtPosition = (
  text: string,
  position: number,
): HybridImageMarkerInfo | null => {
  if (!text || position < 0) {
    return null;
  }

  IMAGE_TOKEN_REGEX.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = IMAGE_TOKEN_REGEX.exec(text)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    if (position >= start && position < end) {
      return {
        markerId: match[1],
        start,
        end,
      };
    }
  }

  return null;
};

export interface HybridPendingInputContext {
  inputType: string;
  data: string;
  selectionStart: number;
  selectionEnd: number;
  previousValue: string;
}

export const normalizeCursorAfterTextInsertion = (
  nextValue: string,
  measuredCursor: number,
  context: HybridPendingInputContext | null,
): number => {
  if (!context) {
    return measuredCursor;
  }

  const { inputType, data, selectionStart, selectionEnd, previousValue } = context;
  if (inputType !== 'insertText' || !data) {
    return measuredCursor;
  }

  const start = Math.min(selectionStart, selectionEnd);
  const end = Math.max(selectionStart, selectionEnd);
  const expectedValue = `${previousValue.slice(0, start)}${data}${previousValue.slice(end)}`;
  if (nextValue !== expectedValue) {
    return measuredCursor;
  }

  const expectedCursor = start + data.length;
  return measuredCursor < expectedCursor ? expectedCursor : measuredCursor;
};
