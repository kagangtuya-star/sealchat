export const WORLD_DESCRIPTION_MAX_DISPLAY_CHARS = 100;
export const WORLD_DESCRIPTION_MAX_WIDTH_UNITS = WORLD_DESCRIPTION_MAX_DISPLAY_CHARS * 2;
export const WORLD_DESCRIPTION_PREVIEW_MAX_WIDTH_UNITS = 66;
export const WORLD_DESCRIPTION_PREVIEW_LINE_WIDTH_UNITS = 22;

const isFullWidthCodePoint = (codePoint: number) => (
  codePoint >= 0x1100 && (
    codePoint <= 0x115f ||
    codePoint === 0x2329 ||
    codePoint === 0x232a ||
    (codePoint >= 0x2e80 && codePoint <= 0x3247 && codePoint !== 0x303f) ||
    (codePoint >= 0x3250 && codePoint <= 0x4dbf) ||
    (codePoint >= 0x4e00 && codePoint <= 0xa4c6) ||
    (codePoint >= 0xa960 && codePoint <= 0xa97c) ||
    (codePoint >= 0xac00 && codePoint <= 0xd7a3) ||
    (codePoint >= 0xf900 && codePoint <= 0xfaff) ||
    (codePoint >= 0xfe10 && codePoint <= 0xfe19) ||
    (codePoint >= 0xfe30 && codePoint <= 0xfe6b) ||
    (codePoint >= 0xff01 && codePoint <= 0xff60) ||
    (codePoint >= 0xffe0 && codePoint <= 0xffe6) ||
    (codePoint >= 0x1aff0 && codePoint <= 0x1aff3) ||
    (codePoint >= 0x1aff5 && codePoint <= 0x1affb) ||
    (codePoint >= 0x1affd && codePoint <= 0x1affe) ||
    (codePoint >= 0x1b000 && codePoint <= 0x1b122) ||
    codePoint === 0x1b132 ||
    (codePoint >= 0x1b150 && codePoint <= 0x1b152) ||
    (codePoint >= 0x1f200 && codePoint <= 0x1f23b) ||
    (codePoint >= 0x1f240 && codePoint <= 0x1f248) ||
    (codePoint >= 0x1f250 && codePoint <= 0x1f251) ||
    (codePoint >= 0x20000 && codePoint <= 0x3fffd)
  )
);

const getCodePointDisplayWidthUnits = (char: string) => {
  const codePoint = char.codePointAt(0);
  if (!codePoint) return 0;
  return isFullWidthCodePoint(codePoint) ? 2 : 1;
};

export const getTextDisplayWidthUnits = (value: string) => {
  let total = 0;
  for (const char of value) {
    total += getCodePointDisplayWidthUnits(char);
  }
  return total;
};

export const truncateTextByDisplayWidth = (value: string, maxUnits: number) => {
  if (!value || maxUnits <= 0) return '';
  let total = 0;
  let result = '';
  for (const char of value) {
    const units = getCodePointDisplayWidthUnits(char);
    if (total + units > maxUnits) break;
    result += char;
    total += units;
  }
  return result;
};

export const splitTextByDisplayWidth = (value: string, lineUnits: number) => {
  if (!value) return [];
  if (lineUnits <= 0) return [value];
  const lines: string[] = [];
  let currentLine = '';
  let currentUnits = 0;

  for (const char of value) {
    if (char === '\n') {
      lines.push(currentLine);
      currentLine = '';
      currentUnits = 0;
      continue;
    }

    const units = getCodePointDisplayWidthUnits(char);
    if (currentLine && currentUnits + units > lineUnits) {
      lines.push(currentLine);
      currentLine = char;
      currentUnits = units;
      continue;
    }

    currentLine += char;
    currentUnits += units;
  }

  lines.push(currentLine);
  return lines;
};

export const formatDisplayWidthAsCharCount = (units: number) => (
  units % 2 === 0 ? String(units / 2) : `${Math.floor(units / 2)}.5`
);
