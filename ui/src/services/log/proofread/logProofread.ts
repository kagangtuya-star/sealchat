// LOG 批量校对核心：从正文抽取可编辑文本、保护特殊标记、给 zhlint 与少量跑团规则的结果分级，
// 并把采纳的修正按原格式写回正文。纯函数，不导入 zhlint：调用方注入 lint，懒加载由 zhlintRunner 负责。
//
// 文本位置：issue 的 start/end 是“文本段”（run）内的 UTF-16 偏移。TipTap 正文的 run 是同一父节点下
// 连续的非代码文本节点；旧格式正文的 run 是标签之间的一段文本。跨文本节点的修正不会产生。
import { buildAIPolishDiffTokens } from '@/services/ai/ai-polish-diff';

export type ProofLint = (text: string) => string;
export type ProofLevel = 'safe' | 'suggest';
export type ProofRule = 'width' | 'space' | 'duplicate' | 'mixed' | 'period' | 'zhlint' | 'ai_typo' | 'ai_punctuation' | 'ai_grammar';

export const PROOF_RULE_LABELS: Record<ProofRule, string> = {
  width: '半角标点应为全角',
  space: '全角标点旁的多余空格',
  duplicate: '重复的标点',
  mixed: '全半角标点混用',
  period: '中文后的半角句点',
  zhlint: '其他排版建议',
  ai_typo: 'AI · 错别字',
  ai_punctuation: 'AI · 标点',
  ai_grammar: 'AI · 轻度语病',
};

export interface ProofIssue {
  /** 在单条消息内唯一且稳定：同一正文重复扫描得到相同 id。 */
  id: string;
  run: number;
  start: number;
  end: number;
  original: string;
  replacement: string;
  rule: ProofRule;
  level: ProofLevel;
  /** 原文上下文，仅用于展示。 */
  before: string;
  after: string;
  source?: 'ai';
  reason?: string;
}

export interface ProofEdit {
  run: number;
  start: number;
  end: number;
  original: string;
  replacement: string;
}

interface ProofRun {
  text: string;
  /** 各文本节点在 run 内的起点（升序，首项为 0）。 */
  bounds: number[];
}

interface ParsedDocument {
  runs: ProofRun[];
  /** 把每个 run 的新文本写回并序列化；pieces[run][i] 对应 bounds[i] 开始的文本节点。 */
  write: (texts: string[][]) => string;
  pieces: string[][];
  aiAllowed?: boolean[][];
}

type TipTapNode = { type?: unknown; text?: unknown; marks?: unknown; content?: unknown };

const isTipTapDoc = (raw: string) => {
  if (!raw.trimStart().startsWith('{')) return null;
  try {
    const parsed = JSON.parse(raw) as TipTapNode;
    return parsed && typeof parsed === 'object' && parsed.type === 'doc' ? parsed : null;
  } catch {
    return null;
  }
};

const hasCodeMark = (node: TipTapNode) => Array.isArray(node.marks)
  && node.marks.some((m) => !!m && typeof m === 'object' && (m as { type?: unknown }).type === 'code');

const parseTipTap = (doc: TipTapNode): ParsedDocument => {
  const groups: TipTapNode[][] = [];
  const allowed = new WeakSet<TipTapNode>();
  const containers = new Set(['doc', 'paragraph', 'heading', 'blockquote', 'bulletList', 'orderedList', 'listItem']);
  const marks = new Set(['bold', 'italic', 'strike', 'underline', 'textStyle', 'highlight', 'subscript', 'superscript']);
  const walk = (node: TipTapNode, inCode: boolean, aiSafe: boolean) => {
    if (!Array.isArray(node.content)) return;
    const code = inCode || node.type === 'codeBlock';
    const safe = aiSafe && containers.has(String(node.type));
    let current: TipTapNode[] | null = null;
    for (const child of node.content as TipTapNode[]) {
      if (!child || typeof child !== 'object') {
        current = null;
        continue;
      }
      if (child.type === 'text' && typeof child.text === 'string') {
        if (code || hasCodeMark(child)) {
          current = null;
          continue;
        }
        if (!current) groups.push((current = []));
        current.push(child);
        if (safe && (!Array.isArray(child.marks) || child.marks.every((m: { type?: unknown }) => m && marks.has(String(m.type))))) allowed.add(child);
        continue;
      }
      current = null;
      walk(child, code, safe);
    }
  };
  walk(doc, false, true);
  const pieces = groups.map((g) => g.map((n) => n.text as string));
  return {
    pieces,
    aiAllowed: groups.map((g) => g.map((n) => allowed.has(n))),
    runs: pieces.map(runOf),
    write: (texts) => {
      groups.forEach((g, i) => g.forEach((n, j) => { n.text = texts[i][j]; }));
      return JSON.stringify(doc);
    },
  };
};

// 与 logContent 的旧格式解析一致：注释与完整标签整体视为结构，不进入校对。
const LEGACY_TAG = /(<!--[\s\S]*?-->|<(?:[^"'<>]|"[^"]*"|'[^']*')*>)/g;
const LEGACY_SKIP_OPEN = /^<(code|pre|script|style)\b/i;

const parseLegacy = (raw: string): ParsedDocument => {
  const parts = raw.split(LEGACY_TAG);
  const textParts: number[] = [];
  // 代码、预格式与骰子结果里的文本保持原样；只按标签名配对，未闭合时保守地跳过其后全部文本。
  const stack: Array<{ name: string; skip: boolean; aiSkip: boolean }> = [];
  const aiBlockedParts = new Set<number>();
  parts.forEach((part, index) => {
    if (index % 2 === 1) {
      if (part.startsWith('<!--') || /\/\s*>$/.test(part)) return;
      const close = /^<\/\s*([a-z0-9-]+)/i.exec(part);
      if (close) {
        const at = stack.map((s) => s.name).lastIndexOf(close[1].toLowerCase());
        if (at >= 0) stack.length = at;
        return;
      }
      const open = /^<\s*([a-z0-9-]+)/i.exec(part);
      if (!open) return;
      const name = open[1].toLowerCase();
      if (name === 'br' || name === 'hr' || name === 'img') return;
      stack.push({ name, skip: LEGACY_SKIP_OPEN.test(part) || /class\s*=\s*["'][^"']*dice/i.test(part), aiSkip: /^(?:a|button|iframe|object|embed|svg|math|template|audio|video|select|textarea)$/.test(name) });
      return;
    }
    if (part && !stack.some((s) => s.skip)) {
      textParts.push(index);
      if (stack.some((s) => s.aiSkip)) aiBlockedParts.add(index);
    }
  });
  const pieces = textParts.map((i) => [parts[i]]);
  // 旧格式可能把标签嵌进代码/特殊标记。按原始正文求保护区，避免分段后误暴露其内部文本。
  const rawMask = aiProtectedMask(raw);
  const offsets: number[] = [];
  let offset = 0;
  parts.forEach((part) => { offsets.push(offset); offset += part.length; });
  return {
    pieces,
    aiAllowed: textParts.map((i) => {
      const local = aiProtectedMask(parts[i]);
      return [!aiBlockedParts.has(i) && !rawMask.slice(offsets[i], offsets[i] + parts[i].length).some((v, k) => v && !local[k])];
    }),
    runs: pieces.map(runOf),
    write: (texts) => {
      const next = parts.slice();
      textParts.forEach((partIndex, i) => { next[partIndex] = texts[i][0]; });
      return next.join('');
    },
  };
};

/** 偏移所在的文本节点序号。 */
const pieceAt = (bounds: number[], offset: number) => {
  let index = 0;
  while (index + 1 < bounds.length && bounds[index + 1] <= offset) index++;
  return index;
};

const runOf = (texts: string[]): ProofRun => {
  const bounds: number[] = [];
  let at = 0;
  for (const t of texts) {
    bounds.push(at);
    at += t.length;
  }
  return { text: texts.join(''), bounds };
};

const parseDocument = (content: string): ParsedDocument => {
  const doc = isTipTapDoc(content);
  return doc ? parseTipTap(doc) : parseLegacy(content);
};

// ---- 保护区：命中的字符既不送入 zhlint，也不被本地规则修改 ----
const PROTECTED_PATTERNS: RegExp[] = [
  /```[\s\S]*?```/g, // 代码块
  /`[^`\n]+`/g, // 行内代码
  /\[CQ:[^\]]*\]/gi, // CQ 码
  /\]\([^)\s]*\)/g, // Markdown 链接地址
  /(?:https?|ftp):\/\/[\w\-.~:/?#[\]@!$&'()*+,;=%]+/gi, // URL（只取 ASCII 地址字符，遇到中文即止）
  /\bwww\.[a-z0-9-]+(?:\.[a-z0-9-]+)+[\w\-.~:/?#[\]@!$&'()*+,;=%]*/gi,
  /&(?:[a-z][a-z0-9]{1,31}|#\d{1,7}|#x[0-9a-f]{1,6});/gi, // 实体转义
  /^[ \t]*[.。．/!！][a-z][^\n]*$/gim, // 骰子 / 机器人指令整行
  /\{[^{}\n]*\}/g, // 骰子表达式 {1d20+3}
  /\b\d*d\d+(?:\s*[+\-*/x×]\s*\d+(?:d\d+)?)*/gi, // 1d100、2d6+3
  /<[^<>\n]*>/g, // 正文里残留的尖括号片段
];

const protectedMask = (text: string): Uint8Array => {
  const mask = new Uint8Array(text.length);
  for (const pattern of PROTECTED_PATTERNS) {
    pattern.lastIndex = 0;
    for (const m of text.matchAll(pattern)) {
      mask.fill(1, m.index!, m.index! + m[0].length);
    }
  }
  return mask;
};

const aiProtectedMask = (text: string): Uint8Array => {
  const mask = protectedMask(text);
  const extra = /~~~[\s\S]*?(?:~~~|(?![\s\S]))|```[\s\S]*?(?:```|(?![\s\S]))|!?\[[^\]\n]*\](?:\([^\n)]*\)|\[[^\]\n]*\])|[*_~`\\#[\]{}]|^[ \t]*(?:[-+>] |\d+\. )|^ {4}[^\n]*/gm;
  for (const m of text.matchAll(extra)) mask.fill(1, m.index!, m.index! + m[0].length);
  return mask;
};

/** 不含保护字符、不跨行的待检查片段。 */
const editableSlices = (text: string, mask: Uint8Array): Array<[number, number]> => {
  const out: Array<[number, number]> = [];
  let start = -1;
  for (let i = 0; i <= text.length; i++) {
    const free = i < text.length && !mask[i] && text[i] !== '\n';
    if (free && start < 0) start = i;
    if (!free && start >= 0) {
      out.push([start, i]);
      start = -1;
    }
  }
  return out;
};

// ---- 分级 ----
const HAN = /[㐀-䶿一-鿿豈-﫿]/;
const FULL_PUNCT = /[，。、；：？！…—“”‘’「」『』（）《》〈〉【】〔〕～]/;
const WIDTH: Record<string, string> = { ',': '，', ';': '；', ':': '：', '?': '？', '!': '！' };
const PAUSE = /\.\.|…|。。|·{2,}/;

/** 判断语境时跳过 Markdown 强调标记（**粗体**、~~删除线~~），它们不改变两侧文字的语言。 */
const neighbor = (text: string, from: number, step: 1 | -1) => {
  let i = from;
  while (i >= 0 && i < text.length && /[*_~]/.test(text[i])) i += step;
  return i >= 0 ? text[i] : undefined;
};

const isHan = (ch: string | undefined) => !!ch && HAN.test(ch);
const isFull = (ch: string | undefined) => !!ch && FULL_PUNCT.test(ch);
const isEnd = (ch: string | undefined) => ch === undefined || ch === '\n';

interface Candidate {
  start: number;
  end: number;
  original: string;
  replacement: string;
  rule: ProofRule;
  level: ProofLevel;
}

/** zhlint 的改动只在左右语境能确定为中文时自动采纳；靠近省略、停顿的改动直接放弃。 */
const classifyHunk = (text: string, start: number, end: number, replacement: string): Candidate | null => {
  const original = text.slice(start, end);
  const prev = neighbor(text, start - 1, -1);
  const next = neighbor(text, end, 1);
  if (PAUSE.test(text.slice(Math.max(0, start - 2), end + 2))) return null;
  // zhlint 可能把“空格 + 半角标点 + 空格”合并为一个全角标点，按标点宽度修正处理。
  if (WIDTH[original.trim()] === replacement && original.trim().length === 1) {
    const safe = isHan(prev) && (isEnd(next) || isHan(next) || isFull(next));
    return { start, end, original, replacement, rule: 'width', level: safe ? 'safe' : 'suggest' };
  }
  if (original && !original.trim() && !replacement) {
    const safe = (isFull(prev) && (isHan(next) || isFull(next))) || (isHan(prev) && isFull(next));
    return { start, end, original, replacement, rule: 'space', level: safe ? 'safe' : 'suggest' };
  }
  return { start, end, original, replacement, rule: 'zhlint', level: 'suggest' };
};

const diffHunks = (source: string, result: string) => {
  const hunks: Array<{ start: number; end: number; replacement: string }> = [];
  if (!result || source === result) return hunks;
  let pos = 0;
  let cur: { start: number; end: number; replacement: string } | null = null;
  for (const token of buildAIPolishDiffTokens(source, result)) {
    if (token.type === 'equal') {
      if (cur) hunks.push(cur);
      cur = null;
      pos += token.text.length;
      continue;
    }
    cur ||= { start: pos, end: pos, replacement: '' };
    if (token.type === 'delete') {
      cur.end += token.text.length;
      pos += token.text.length;
    } else {
      cur.replacement += token.text;
    }
  }
  if (cur) hunks.push(cur);
  return hunks;
};

/** 跑团文本的补充规则：只处理确定的重复与混用；句点只提示，不自动改。 */
const localCandidates = (text: string, from: number, to: number): Candidate[] => {
  const slice = text.slice(from, to);
  const out: Candidate[] = [];
  for (const m of slice.matchAll(/([，、；：])\1+/g)) {
    out.push({ start: from + m.index!, end: from + m.index! + m[0].length, original: m[0], replacement: m[1], rule: 'duplicate', level: 'safe' });
  }
  for (const m of slice.matchAll(/，,|,，|；;|;；|：:|:：/g)) {
    const full = /[，；：]/.exec(m[0])![0];
    out.push({ start: from + m.index!, end: from + m.index! + 2, original: m[0], replacement: full, rule: 'mixed', level: 'safe' });
  }
  for (const m of slice.matchAll(/。\.(?!\.)/g)) {
    const at = from + m.index!;
    out.push({ start: at, end: at + 2, original: m[0], replacement: '。', rule: 'mixed', level: 'suggest' });
  }
  for (const m of slice.matchAll(/\./g)) {
    const at = from + m.index!;
    if (text[at - 1] === '.' || text[at + 1] === '.' || text[at - 1] === '。') continue;
    if (isHan(text[at - 1]) && (isEnd(text[at + 1]) || isHan(text[at + 1]))) {
      out.push({ start: at, end: at + 1, original: '.', replacement: '。', rule: 'period', level: 'suggest' });
    }
  }
  return out;
};

const overlaps = (a: { start: number; end: number }, b: { start: number; end: number }) => a.start < b.end && b.start < a.end;

/** 插入点与相邻替换共享边界时也视为冲突，避免应用顺序影响结果。 */
export const proofEditsConflict = (a: ProofEdit, b: ProofEdit): boolean => a.run === b.run && (
  a.start === a.end || b.start === b.end
    ? a.start <= b.end && b.start <= a.end
    : overlaps(a, b)
);

export interface EditableProofFragment { run: number; start: number; end: number; text: string }

/** 删除也可能把相邻文本拼成指令或链接，逐段比较未修改字符的保护状态。 */
const preservesAIProtection = (before: string, after: string, edits: ProofEdit[]): boolean => {
  const originalMask = aiProtectedMask(before);
  const nextMask = aiProtectedMask(after);
  let oldAt = 0;
  let nextAt = 0;
  const unchanged = (end: number) => {
    while (oldAt < end) {
      if (nextMask[nextAt] && !originalMask[oldAt]) return false;
      oldAt++; nextAt++;
    }
    return true;
  };
  for (const edit of edits.slice().sort((a, b) => a.start - b.start)) {
    if (!unchanged(edit.start) || nextMask.slice(nextAt, nextAt + edit.replacement.length).some(Boolean)) return false;
    oldAt = edit.end;
    nextAt += edit.replacement.length;
  }
  return unchanged(before.length);
};

/** AI 只接收单节点内的安全纯文本。Markdown 结构与链接整体跳过；不发送保护区作为上下文。 */
export const editableProofFragments = (content: string): EditableProofFragment[] => {
  if (/^\s*[\[{]/.test(content) && !isTipTapDoc(content)) {
    try { JSON.parse(content); return []; } catch { /* 普通对白仍按保护区检查。 */ }
  }
  const doc = parseDocument(content);
  const out: EditableProofFragment[] = [];
  doc.runs.forEach((run, r) => {
    const mask = aiProtectedMask(run.text);
    for (const [from, to] of editableSlices(run.text, mask)) {
      run.bounds.forEach((base, piece) => {
        if (doc.aiAllowed?.[r]?.[piece] === false) return;
        const start = Math.max(from, base);
        const end = Math.min(to, run.bounds[piece + 1] ?? run.text.length);
        if (end > start && run.text.slice(start, end).trim()) out.push({ run: r, start, end, text: run.text.slice(start, end) });
      });
    }
  });
  return out;
};

/** AI 写回时重新校验保护区与结构，不能只相信扫描阶段的定位。 */
export const validAIProofEdit = (content: string, e: ProofEdit): boolean => {
  if (!Number.isInteger(e.run) || !Number.isInteger(e.start) || !Number.isInteger(e.end) || e.start < 0 || e.end < e.start) return false;
  if (e.original.length > 40 || e.replacement.length > 40 || e.original === e.replacement) return false;
  if (/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?:^|[^\uD800-\uDBFF])[\uDC00-\uDFFF]/.test(e.replacement)) return false;
  if (/[<>\[\]{}*_~`\\&\r\n\u0000-\u001f\u007f\ue000-\uf8ff]/.test(e.replacement)) return false;
  // 轻度修改最多改变 12 个字符；长片段还必须保留过半原文。
  const tokens = buildAIPolishDiffTokens(e.original, e.replacement);
  const changed = tokens.filter((t) => t.type !== 'equal').reduce((n, t) => n + t.text.length, 0);
  if (e.original.length > 4 && !tokens.some((t) => t.type === 'equal')) return false;
  if (changed > 12 || (e.original.length > 16 && changed > e.original.length)) return false;
  const doc = parseDocument(content);
  const run = doc.runs[e.run];
  if (!run || e.end > run.text.length) return false;
  const splitsPair = (p: number) => p > 0 && /[\uD800-\uDBFF]/.test(run.text[p - 1]) && /[\uDC00-\uDFFF]/.test(run.text[p] || '');
  if (splitsPair(e.start) || splitsPair(e.end)) return false;
  if (e.start === e.end && run.bounds.slice(1).includes(e.start)) return false;
  const piece = pieceAt(run.bounds, e.start);
  if (!e.replacement && e.start === run.bounds[piece] && e.end === (run.bounds[piece + 1] ?? run.text.length)) return false;
  const next = run.text.slice(0, e.start) + e.replacement + run.text.slice(e.end);
  if (!preservesAIProtection(run.text, next, [e])) return false;
  return editableProofFragments(content).some((f) => e.run === f.run && e.start >= f.start && e.end <= f.end
    && f.text.slice(e.start - f.start, e.end - f.start) === e.original);
};

/**
 * 扫描一条正文。lint 为 null 时只运行本地规则。
 * 返回的 issue 互不重叠、按位置排序，且不跨越文本节点边界。
 */
export const proofreadContent = (content: string, lint: ProofLint | null): ProofIssue[] => {
  if (!content || !content.trim()) return [];
  const doc = parseDocument(content);
  const issues: ProofIssue[] = [];
  doc.runs.forEach((run, runIndex) => {
    const { text } = run;
    if (!text.trim()) return;
    const mask = protectedMask(text);
    const found: Candidate[] = [];
    for (const [from, to] of editableSlices(text, mask)) {
      const local = localCandidates(text, from, to);
      found.push(...local);
      if (!lint) continue;
      const slice = text.slice(from, to);
      let result: string;
      try {
        result = lint(slice);
      } catch {
        continue;
      }
      for (const h of diffHunks(slice, result)) {
        const c = classifyHunk(text, from + h.start, from + h.end, h.replacement);
        if (c && !local.some((l) => overlaps(l, c) || (l.start === c.start && l.end === c.end))) found.push(c);
      }
    }
    found.sort((a, b) => a.start - b.start || b.end - a.end);
    let lastEnd = -1;
    for (const c of found) {
      if (c.start < lastEnd || c.end <= c.start) continue;
      // 只接受完整落在单个文本节点内的修改，避免改动标记边界。
      const piece = pieceAt(run.bounds, c.start);
      const pieceEnd = run.bounds[piece + 1] ?? text.length;
      if (c.end > pieceEnd) continue;
      lastEnd = c.end;
      issues.push({
        ...c,
        id: `${runIndex}:${c.start}:${c.end}:${c.rule}`,
        run: runIndex,
        before: text.slice(Math.max(0, c.start - 12), c.start),
        after: text.slice(c.end, c.end + 12),
      });
    }
  });
  return issues;
};

/**
 * 把修正写回正文。正文自扫描后变化（文本不再匹配）时返回 null，调用方应跳过该消息。
 * TipTap 正文只改写文本节点的 text，其余节点、属性与标记原样保留。
 */
export const applyProofEdits = (content: string, edits: ProofEdit[]): string | null => {
  if (!edits.length) return content;
  const doc = parseDocument(content);
  const texts = doc.pieces.map((p) => p.slice());
  const sorted = edits.slice().sort((a, b) => a.run - b.run || b.start - a.start);
  let prev: ProofEdit | null = null;
  for (const e of sorted) {
    const run = doc.runs[e.run];
    if (!run || e.start < 0 || e.end > run.text.length || e.end < e.start) return null;
    if (run.text.slice(e.start, e.end) !== e.original) return null;
    if (prev && proofEditsConflict(prev, e)) return null;
    if ('source' in e && e.source === 'ai' && !validAIProofEdit(content, e)) return null;
    const piece = pieceAt(run.bounds, e.start);
    const base = run.bounds[piece];
    const pieceEnd = run.bounds[piece + 1] ?? run.text.length;
    if (e.end > pieceEnd) return null;
    const t = texts[e.run][piece];
    texts[e.run][piece] = t.slice(0, e.start - base) + e.replacement + t.slice(e.end - base);
    prev = e;
  }
  // 多处局部修改组合后也不能形成新的指令、链接或 Markdown 结构。
  for (const run of new Set(sorted.filter((e) => 'source' in e && e.source === 'ai').map((e) => e.run))) {
    if (!preservesAIProtection(doc.runs[run].text, texts[run].join(''), sorted.filter((e) => e.run === run))) return null;
  }
  return doc.write(texts);
};

/** 对比展示用：正文中参与校对的文本段（按 run 顺序以换行连接）在修正前后的样子。 */
export const previewProofEdits = (content: string, edits: ProofEdit[]): { before: string; after: string } => {
  const runs = parseDocument(content).runs.map((r) => r.text);
  const next = runs.slice();
  const sorted = edits.slice().sort((a, b) => a.run - b.run || b.start - a.start);
  for (const e of sorted) {
    const t = next[e.run];
    if (t === undefined || runs[e.run].slice(e.start, e.end) !== e.original) continue;
    next[e.run] = t.slice(0, e.start) + e.replacement + t.slice(e.end);
  }
  return { before: runs.join('\n'), after: next.join('\n') };
};
