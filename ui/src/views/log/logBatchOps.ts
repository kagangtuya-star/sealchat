// 多选批量移动规划：只依据当前来源片段已加载的可见消息顺序计算，
// 由服务端 message.relocate.batch 在一个事务中执行（各组保持原有相对顺序）。

export interface LogBatchMoveGroup {
  messageIds: string[];
  targetMessageId: string;
}

export interface LogBatchMovePlan {
  placement: 'before' | 'after';
  groups: LogBatchMoveGroup[];
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
 * 集合整体上移/下移一位：每个连续段与其上方（下方）相邻的未选消息交换位置，段内与段间顺序不变。
 * 首段已在顶部（末段已在底部）时整体不可移动，返回 null，不做部分移动。
 */
export const planBatchStep = (order: readonly string[], selected: ReadonlySet<string>, dir: -1 | 1): LogBatchMovePlan | null => {
  const runs = selectionRuns(order, selected);
  if (!runs.length) return null;
  if (dir < 0 ? runs[0].start === 0 : runs[runs.length - 1].end === order.length - 1) return null;
  return {
    placement: dir < 0 ? 'before' : 'after',
    groups: runs.map((run) => ({ messageIds: run.ids, targetMessageId: order[dir < 0 ? run.start - 1 : run.end + 1] })),
  };
};

/** 在本地顺序上模拟执行计划；用于判定“位置不变”与回归测试。 */
export const applyBatchMovePlan = (order: readonly string[], plan: LogBatchMovePlan): string[] => {
  const moving = new Set(plan.groups.flatMap((g) => g.messageIds));
  const byTarget = new Map(plan.groups.map((g) => [g.targetMessageId, g.messageIds.filter((id) => order.includes(id))]));
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
