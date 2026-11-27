import type { LogElement, LogElementPlacement } from '@/types/log';

// 多选批量移动规划：依据当前来源片段已加载的可见节点顺序计算，
// 由服务端 message.relocate.batch 在一个事务中执行（各组保持原有相对顺序）。

export interface LogBatchMoveGroup {
  messageIds: string[];
  targetMessageId: string;
}

export interface LogBatchMovePlan {
  placement: 'before' | 'after';
  groups: LogBatchMoveGroup[];
  placements?: LogElementPlacement[];
}

interface VisualStepContext {
  nodes: readonly { id: string; kind: 'message' | 'element' }[];
  elements: readonly LogElement[];
  sectionId: string;
}

/** 选中集合在可见顺序中的连续段；不在顺序中的 ID 被忽略。 */
export const selectionRuns = (order: readonly string[], selected: ReadonlySet<string>) => {
  const runs: Array<{ start: number; end: number; ids: string[] }> = [];
  order.forEach((id, index) => {
    if (!selected.has(id)) return;
    const last = runs[runs.length - 1];
    if (last && last.end === index - 1) {
      last.end = index;
      last.ids.push(id);
    } else {
      runs.push({ start: index, end: index, ids: [id] });
    }
  });
  return runs;
};

/**
 * 集合整体上移/下移一位：传入 visual 时按可见节点步进，否则按消息步进。
 * 每个连续段与相邻的未选节点交换位置，段内与段间顺序不变。
 * 首段已在顶部（末段已在底部）时整体不可移动，返回 null，不做部分移动。
 */
export const planBatchStep = (order: readonly string[], selected: ReadonlySet<string>, dir: -1 | 1, visual?: VisualStepContext): LogBatchMovePlan | null => {
  if (visual) return planVisualStep(order, selected, dir, visual);
  const runs = selectionRuns(order, selected);
  if (!runs.length) return null;
  if (dir < 0 ? runs[0].start === 0 : runs[runs.length - 1].end === order.length - 1) return null;
  return {
    placement: dir < 0 ? 'before' : 'after',
    groups: runs.map((run) => ({ messageIds: run.ids, targetMessageId: order[dir < 0 ? run.start - 1 : run.end + 1] })),
  };
};

/** 消息跨过一个可见节点；未选元素不随消息移动，必要时只改写受影响间隙的锚点。 */
const planVisualStep = (order: readonly string[], selected: ReadonlySet<string>, dir: -1 | 1, visual: VisualStepContext): LogBatchMovePlan | null => {
  const { nodes, elements, sectionId } = visual;
  const messages = nodes.filter(n => n.kind === 'message').map(n => n.id);
  if (!selected.size || messages.length !== order.length || messages.some((id, i) => id !== order[i])
    || [...selected].some(id => !messages.includes(id))) return null;
  const ids = nodes.map(n => n.id);
  const step = planBatchStep(ids, selected, dir);
  if (!step) return null;
  const next = applyBatchMovePlan(ids, step);
  const messageSet = new Set(messages);
  const nextMessages = next.filter(id => messageSet.has(id));
  // 空目标表示该组只跨越元素，不写消息排名；允许与真正重排的组一次提交。
  const neighbor = new Map<string, string>();
  let target = '';
  for (const id of dir < 0 ? messages.slice().reverse() : messages) {
    if (selected.has(id)) neighbor.set(id, target);
    else target = id;
  }
  const groups = selectionRuns(nextMessages, selected).map(run => {
    const targetMessageId = nextMessages[dir < 0 ? run.end + 1 : run.start - 1] || '';
    return { messageIds: run.ids, targetMessageId: run.ids.every(id => neighbor.get(id) === targetMessageId) ? '' : targetMessageId };
  });
  const byId = new Map(elements.filter(e => e.anchor.sectionId === sectionId).map(e => [`el-${e.id}`, e]));
  const positions = new Map(elements.map((e, i) => [e.id, i]));
  const placements: LogElementPlacement[] = [];
  let previous: string | undefined;
  for (let start = 0; start < next.length;) {
    if (messageSet.has(next[start])) { previous = next[start++]; continue; }
    let end = start;
    while (end < next.length && !messageSet.has(next[end])) end++;
    const following = next[end];
    const bucket = next.slice(start, end).map(id => byId.get(id));
    if (bucket.some(e => !e)) return null;
    const entries = bucket.filter((e): e is LogElement => !!e);
    const left = (e: LogElement) => previous ? e.anchor.kind === 'message' && e.anchor.messageId === previous && e.anchor.side === 'after'
      : e.anchor.kind === 'sectionStart';
    const right = (e: LogElement) => following ? e.anchor.kind === 'message' && e.anchor.messageId === following && e.anchor.side === 'before'
      : e.anchor.kind === 'sectionEnd';
    // 左邻的 after 桶始终先于右邻的 before 桶；选改锚最少的切分点。
    let leftInvalid = 0, rightInvalid = entries.filter(e => !right(e)).length;
    let best = Infinity, cut = 0;
    for (let i = 0; i <= entries.length; i++) {
      if ((previous || !leftInvalid) && (following || !rightInvalid) && leftInvalid + rightInvalid < best) {
        best = leftInvalid + rightInvalid;
        cut = i;
      }
      if (i < entries.length) {
        leftInvalid += Number(!left(entries[i]));
        rightInvalid -= Number(!right(entries[i]));
      }
    }
    for (const [part, messageId, side, matches] of [
      [entries.slice(0, cut), previous, 'after', left],
      [entries.slice(cut), following, 'before', right],
    ] as const) {
      const sorted = part.slice().sort((a, b) => a.order - b.order || positions.get(a.id)! - positions.get(b.id)!);
      const reorder = part.some((e, i) => e !== sorted[i]);
      part.forEach((e, i) => {
        const order = reorder ? i : e.order;
        if (matches(e) && order === e.order) return;
        // 章节端点中的元素只在跨过消息后改绑，新的落点始终有消息邻居。
        if (!messageId) return;
        placements.push({ elementId: e.id, anchor: { kind: 'message', sectionId, messageId, side }, order });
      });
    }
    previous = following;
    start = end;
  }
  return { placement: step.placement, groups, ...(placements.length ? { placements } : {}) };
};

/** 在本地顺序上模拟执行计划；用于判定“位置不变”与回归测试。 */
export const applyBatchMovePlan = (order: readonly string[], plan: LogBatchMovePlan): string[] => {
  const groups = plan.groups.filter(g => g.targetMessageId);
  const moving = new Set(groups.flatMap((g) => g.messageIds));
  const byTarget = new Map(groups.map((g) => [g.targetMessageId, g.messageIds.filter((id) => order.includes(id))]));
  const position = new Map(order.map((id, i) => [id, i]));
  byTarget.forEach((ids) => ids.sort((a, b) => position.get(a)! - position.get(b)!));
  const out: string[] = [];
  for (const id of order) {
    if (moving.has(id)) continue;
    const group = byTarget.get(id);
    if (group && plan.placement === 'before') out.push(...group);
    out.push(id);
    if (group && plan.placement === 'after') out.push(...group);
  }
  return out;
};
