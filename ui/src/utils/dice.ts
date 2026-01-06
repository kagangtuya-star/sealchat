const COMMAND_PATTERN = /[\.。．｡]rh?[^\s　,，。！？!?;；:：]*/gi;
const BRACE_PATTERN = /\{([^{}]+)\}/g;
const INCOMPLETE_PATTERN = /(\b\d*)d\b/gi;

export interface DiceMatch {
  start: number;
  end: number;
  source: string;
  normalized: string;
  kind: 'command' | 'brace';
}

export const DEFAULT_DICE_EXPR = 'd20';

export const ensureDefaultDiceExpr = (value?: string): string => {
  const trimmed = (value || '').trim().toLowerCase();
  if (!trimmed) return DEFAULT_DICE_EXPR;
  if (trimmed.startsWith('d')) {
    const sides = trimmed.slice(1);
    if (/^\d+$/.test(sides) && Number(sides) > 0) {
      return `d${sides}`;
    }
  } else if (/^\d+$/.test(trimmed)) {
    return `d${trimmed}`;
  }
  return DEFAULT_DICE_EXPR;
};

export const isValidDefaultDiceExpr = (value: string): boolean => {
  const normalized = ensureDefaultDiceExpr(value);
  return normalized === value.toLowerCase();
};

export const looksLikeTipTapPayload = (value: string): boolean => {
  const trimmed = value?.trim();
  if (!trimmed || !trimmed.startsWith('{')) return false;
  return trimmed.includes('"type":"doc"');
};

export const matchDiceExpressions = (text: string, defaultDiceExpr?: string): DiceMatch[] => {
  if (!text) return [];
  const normalizedDefault = ensureDefaultDiceExpr(defaultDiceExpr);
  const used = new Array(text.length).fill(false);
  const matches: DiceMatch[] = [];

  const markRange = (start: number, end: number) => {
    for (let i = start; i < end && i < used.length; i += 1) {
      used[i] = true;
    }
  };

  const pushMatch = (start: number, end: number, raw: string, inner: string, kind: 'command' | 'brace') => {
    const normalized = normalizeFormula(inner, kind, normalizedDefault);
    matches.push({ start, end, source: raw, normalized, kind });
    markRange(start, end);
  };

  BRACE_PATTERN.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = BRACE_PATTERN.exec(text)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    if (start === end) continue;
    pushMatch(start, end, match[0], match[1], 'brace');
  }

  COMMAND_PATTERN.lastIndex = 0;
  while ((match = COMMAND_PATTERN.exec(text)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    if (start === end) continue;
    if (rangeOverlaps(used, start, end)) continue;
    pushMatch(start, end, match[0], match[0], 'command');
  }

  return matches.sort((a, b) => a.start - b.start);
};

const rangeOverlaps = (used: boolean[], start: number, end: number) => {
  for (let i = Math.max(0, start); i < Math.min(used.length, end); i += 1) {
    if (used[i]) return true;
  }
  return false;
};

const normalizeFormula = (raw: string, kind: DiceMatch['kind'], defaultDiceExpr: string): string => {
  let candidate = raw || '';
  if (kind === 'command') {
    candidate = candidate.trim();
    if (/^[\.。．｡]/.test(candidate)) {
      candidate = candidate.slice(1);
    }
    if (/^rh/i.test(candidate)) {
      candidate = candidate.slice(2);
    } else if (/^[rR]/.test(candidate)) {
      candidate = candidate.slice(1);
    }
  }
  let normalized = candidate.trim();
  if (!normalized) {
    normalized = defaultDiceExpr;
  }
  normalized = normalized
    .toLowerCase()
    .replace(/[×·]/g, '*')
    .replace(/，/g, ',')
    .replace(/（/g, '(')
    .replace(/）/g, ')');
  const sides = defaultDiceExpr.slice(1);
  if (sides) {
    normalized = normalized.replace(INCOMPLETE_PATTERN, (_, count: string) => `${count || ''}d${sides}`);
  }
  if (normalized === 'r' || normalized === 'rd') {
    normalized = defaultDiceExpr;
  }
  return normalized;
};

export const isHiddenDiceCommand = (text: string): boolean => /[\.。．｡]rh/i.test(text);
