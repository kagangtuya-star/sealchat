import type { LogElement, LogElementPlacement } from '@/types/log';

interface DropNode { id: string; kind: 'message' | 'element' }

export interface LogDropPlan {
  afterId?: string;
  beforeId?: string;
  placements: LogElementPlacement[];
}

/** 按实际混合节点顺序插入；原消息附带的元素随消息移动，落点元素组在插入处分开。 */
export const planMessageDrop = (
  nodes: readonly DropNode[], elements: readonly LogElement[], sectionId: string,
  messageId: string, targetId: string, side: 'before' | 'after',
): LogDropPlan | null => {
  if (messageId === targetId || !nodes.some((n) => n.id === messageId && n.kind === 'message')) return null;
  const own = elements.filter((e) => e.anchor.sectionId === sectionId && e.anchor.kind === 'message' && e.anchor.messageId === messageId);
  const ownIds = new Set(own.map((e) => `el-${e.id}`));
  // 拖向自己的元素时，只在原元素组内移动消息，不把目标随消息一起取走。
  const movingOwn = !ownIds.has(targetId);
  const rest = nodes.filter((n) => n.id !== messageId && !(movingOwn && ownIds.has(n.id)));
  const targetIndex = rest.findIndex((n) => n.id === targetId);
  if (targetIndex < 0) return null;
  const at = targetIndex + (side === 'after' ? 1 : 0);
  let start = at, end = at;
  while (start > 0 && rest[start - 1].kind === 'element') start--;
  while (end < rest.length && rest[end].kind === 'element') end++;
  const byId = new Map(elements.map((e) => [`el-${e.id}`, e]));
  const group = (list: readonly DropNode[]) => list.map((n) => byId.get(n.id)).filter((e): e is LogElement => !!e);
  const beforeOwn = movingOwn ? own.filter((e) => e.anchor.kind === 'message' && e.anchor.side === 'before').sort((a, b) => a.order - b.order) : [];
  const afterOwn = movingOwn ? own.filter((e) => e.anchor.kind === 'message' && e.anchor.side === 'after').sort((a, b) => a.order - b.order) : [];
  const before = [...group(rest.slice(start, at)), ...beforeOwn];
  const after = [...afterOwn, ...group(rest.slice(at, end))];
  const nextIds = [...rest.slice(0, start).map((n) => n.id), ...before.map((e) => `el-${e.id}`), messageId,
    ...after.map((e) => `el-${e.id}`), ...rest.slice(end).map((n) => n.id)];
  if (nextIds.length === nodes.length && nextIds.every((id, i) => id === nodes[i].id)) return null;
  const placements: LogElementPlacement[] = [];
  for (const [list, placementSide] of [[before, 'before'], [after, 'after']] as const) {
    list.forEach((e, order) => {
      if (e.anchor.kind === 'message' && e.anchor.messageId === messageId && e.anchor.side === placementSide && e.order === order) return;
      placements.push({ elementId: e.id, anchor: { kind: 'message', sectionId, messageId, side: placementSide }, order });
    });
  }
  const afterId = rest[start - 1]?.id;
  const beforeId = rest[end]?.id;
  const originalMessages = nodes.filter((n) => n.kind === 'message').map((n) => n.id);
  const originalIndex = originalMessages.indexOf(messageId);
  const unchanged = originalMessages[originalIndex - 1] === afterId && originalMessages[originalIndex + 1] === beforeId;
  if (unchanged && !placements.length) return null;
  return { afterId, beforeId, placements };
};
