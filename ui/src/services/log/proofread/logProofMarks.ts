// 校对批注 → 排版正文位置（只由审阅中心按需加载，不进入排版器首屏）。
//
// 校对位置是正文“文本段”内的偏移，而预览显示的是经过 TipTap / 旧格式 / 快捷格式渲染、清洗后的 HTML。
// 两者之间不做字符级推算：把每处批注两端替换为私用区哨兵字符写回正文，走与预览完全相同的渲染与清洗，
// 在得到的可见文本中读出哨兵位置后删去哨兵。删去哨兵的文本必须与正常渲染逐字一致、哨兵之间必须恰好是原文，
// 否则该处（或整条消息）降级为只锚定到消息，不显示可能错误的字符位置。iframe 内还会按当前排版再核对一次。
import { renderMessageBody } from '../logContent';
import { sanitizeBodyHtml } from '../logTemplateCompiler';
import type { FrameProofItem, FrameProofNode } from '../pagination/logFrameHost';
import { applyProofEdits, proofEditsConflict, PROOF_RULE_LABELS, type ProofIssue } from './logProofread';

const OPEN = 0xe000;
const CLOSE = 0xec00;
/** 单条消息可映射的批注数；超出部分降级为消息级批注。 */
const LIMIT = CLOSE - OPEN;

/** 不计入正文位置的元素；必须与 logFrameRuntime.js 中的 PROOF_SKIP 一致。 */
export const PROOF_TEXT_SKIP = 'button, .log-img-missing, .log-message-actions, .log-image-control';

export const UNMAPPED_NOTE = '无法在排版中精确定位，按原文显示';

/** 正文 HTML 的可见文本（文本节点按文档顺序连接）。 */
export const proofBodyText = (html: string): string => {
  const holder = document.createElement('template');
  holder.innerHTML = html;
  const walker = document.createTreeWalker(holder.content, NodeFilter.SHOW_TEXT, {
    acceptNode: (t) => (t.parentElement?.closest(PROOF_TEXT_SKIP) ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
  });
  const out: string[] = [];
  for (let t = walker.nextNode(); t; t = walker.nextNode()) out.push((t as Text).data);
  return out.join('');
};

/** 与预览相同的正文渲染：logRenderer.designMessageToNode → 模板槽位的 sanitizeBodyHtml。 */
const renderedText = (content: string) => proofBodyText(sanitizeBodyHtml(renderMessageBody(content).html));

export interface ProofBodyMap {
  /** 正常渲染的正文可见文本长度。 */
  length: number;
  /** 与 issues 一一对应；null 表示该处无法可靠定位。 */
  ranges: Array<[number, number] | null>;
}

export const mapProofIssues = (content: string, issues: ProofIssue[]): ProofBodyMap => {
  const reference = renderedText(content);
  const ranges: Array<[number, number] | null> = issues.map(() => null);
  const usable = issues.slice(0, LIMIT);
  if (!usable.length) return { length: reference.length, ranges };
  if (usable.some((issue, index) => usable.slice(0, index).some((other) => proofEditsConflict(issue, other)))) {
    // 互斥方案不能在同一次哨兵写回中覆盖彼此；最多分四组映射，更多方案按消息显示。
    const groups: Array<Array<{ issue: ProofIssue; index: number }>> = [];
    usable.forEach((issue, index) => {
      let group = groups.find((g) => !g.some((it) => proofEditsConflict(issue, it.issue)));
      if (!group && groups.length < 4) groups.push((group = []));
      group?.push({ issue, index });
    });
    for (const group of groups) {
      const mapped = mapProofIssues(content, group.map((it) => it.issue));
      group.forEach((it, k) => { ranges[it.index] = mapped.ranges[k]; });
    }
    return { length: reference.length, ranges };
  }
  const marked = applyProofEdits(content, usable.map((issue, k) => ({
    run: issue.run,
    start: issue.start,
    end: issue.end,
    original: issue.original,
    replacement: String.fromCharCode(OPEN + k) + issue.original + String.fromCharCode(CLOSE + k),
  })));
  if (marked === null) return { length: reference.length, ranges };
  const text = renderedText(marked);
  const opens = new Map<number, number>();
  const closes = new Map<number, number>();
  const chunks: string[] = [];
  let length = 0;
  let from = 0;
  for (let i = 0; i < text.length; i++) {
    const code = text.charCodeAt(i);
    if (code < OPEN || code >= CLOSE + LIMIT) continue;
    chunks.push(text.slice(from, i));
    length += i - from;
    from = i + 1;
    if (code < CLOSE) opens.set(code - OPEN, length);
    else closes.set(code - CLOSE, length);
  }
  chunks.push(text.slice(from));
  const plain = chunks.join('');
  // 渲染因哨兵而产生任何差异（格式识别、链接、正文自带私用区字符）时整条消息不定位。
  if (plain !== reference) return { length: reference.length, ranges };
  usable.forEach((issue, k) => {
    const start = opens.get(k);
    const end = closes.get(k);
    if (start === undefined || end === undefined || end <= start) return;
    if (plain.slice(start, end) === issue.original) ranges[k] = [start, end];
  });
  return { length: reference.length, ranges };
};

/**
 * 一条消息的审阅批注。reason 不为空时（嵌入展示、正文已变化）不尝试定位，全部锚定到消息。
 */
export const buildProofNode = (
  nodeId: string,
  content: string,
  issues: ProofIssue[],
  keyOf: (issue: ProofIssue) => string,
  reason: string | null,
): FrameProofNode => {
  const map = reason ? null : mapProofIssues(content, issues);
  return {
    nodeId,
    length: map ? map.length : -1,
    items: issues.map((issue, k): FrameProofItem => {
      const range = map?.ranges[k] || null;
      const item: FrameProofItem = {
        key: keyOf(issue),
        start: range ? range[0] : -1,
        end: range ? range[1] : -1,
        level: issue.level,
        label: `${issue.aiConfirmed && issue.source !== 'ai' ? '本地 + AI · ' : ''}${PROOF_RULE_LABELS[issue.rule]}`,
        original: issue.original,
        replacement: issue.replacement,
        before: issue.before,
        after: issue.after,
        reason: [issue.reason, issues.some((other) => other !== issue && proofEditsConflict(issue, other)) ? '此处有互斥建议，请选择其中一项' : ''].filter(Boolean).join('；'),
      };
      if (!range) item.note = reason || UNMAPPED_NOTE;
      return item;
    }),
  };
};
