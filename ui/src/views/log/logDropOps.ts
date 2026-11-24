import type { LogAnchor, LogElement, LogElementPlacement } from '@/types/log';

interface DropNode { id: string; kind: 'message' | 'element' }
type Side = 'before' | 'after';
// 草稿内移动还允许章节端点；消息排序命令的 placement 协议仍只接受消息锚点。
export type DraftPlacement = Omit<LogElementPlacement, 'anchor'> & { anchor: LogAnchor };
export interface FailedElementMove { elementId: string; anchor: LogAnchor; label: string }

/** 只重排目标锚点桶；直接落在消息边缘时，插在最靠近消息的一侧。 */
export const planElementDrop = (
  nodes: readonly DropNode[], elements: readonly LogElement[], sectionId: string,
  sourceId: string, targetId: string, side: Side, complete: boolean,
): DraftPlacement[] => {
  const source = nodes.findIndex(n => n.kind === 'element' && n.id === sourceId);
  if (source < 0 || sourceId === targetId) return [];
  const rest = nodes.filter(n => n.id !== sourceId);
  const index = rest.findIndex(n => n.id === targetId);
  const start = targetId === ':sectionStart', end = targetId === ':sectionEnd';
  if ((!start && !end && index < 0) || (end && !complete)) return [];
  const at = start ? 0 : end ? rest.length : index + Number(side === 'after');
  if (source === at && !start && !end) return [];
  const byId = new Map(elements.filter(e => e.anchor.sectionId === sectionId).map(e => [`el-${e.id}`, e]));
  const moving = byId.get(sourceId);
  if (!moving) return [];
  const anchor = start || end ? { kind: start ? 'sectionStart' as const : 'sectionEnd' as const, sectionId }
    : rest[index].kind === 'message' ? { kind: 'message' as const, sectionId, messageId: targetId, side }
    : byId.get(targetId)?.anchor;
  if (!anchor) return [];
  const matches = (e: LogElement) => e.anchor.kind === anchor.kind && (e.anchor.kind !== 'message'
    || (anchor.kind === 'message' && e.anchor.messageId === anchor.messageId && e.anchor.side === anchor.side));
  // 显式落到章节端点时，即使视觉位置不变，也要解除对首尾消息的绑定。
  if (source === at && matches(moving)) return [];
  const order = [...rest.slice(0, at), nodes[source], ...rest.slice(at)]
    .map(n => byId.get(n.id)).filter((e): e is LogElement => !!e && (e === moving || matches(e)));
  return order.flatMap((e, i) => matches(e) && e.order === i ? [] : [{ elementId: e.id, anchor, order: i }]);
};

export interface LogDropPlan {
  afterId?: string;
  beforeId?: string;
  placements: LogElementPlacement[];
}

/** nodes 已按 walkDesignSection 排列。只拆开落点间隙中无法留在原侧的元素。 */
export const planMessageDrop = (
  nodes: readonly DropNode[], elements: readonly LogElement[], sectionId: string,
  source: string | readonly string[], targetId: string, side: Side,
): LogDropPlan | null => {
  const selected = new Set(typeof source === 'string' ? [source] : source);
  const messages = nodes.filter(n => n.kind === 'message').map(n => n.id);
  const moving = messages.filter(id => selected.has(id));
  if (!moving.length || moving.length !== selected.size || selected.has(targetId)) return null;
  const byId = new Map(elements.filter(e => e.anchor.sectionId === sectionId).map(e => [`el-${e.id}`, e]));
  const target = byId.get(targetId)?.anchor;
  const held = (a: LogElement['anchor']) => a.kind === 'message' && target?.kind === 'message'
    && a.messageId === target.messageId && (moving.length === 1 || a.side === target.side);
  const rest = nodes.filter(n => {
    if (n.kind === 'message') return !selected.has(n.id);
    const a = byId.get(n.id)?.anchor;
    // 拖向自己的元素时，留下目标桶供消息穿过；其他附属元素随各自消息移动。
    return a?.kind !== 'message' || !selected.has(a.messageId)
      || held(a);
  });
  const index = rest.findIndex(n => n.id === targetId);
  if (index < 0) return null;
  const at = index + Number(side === 'after');
  let start = at, end = at;
  while (start && rest[start - 1].kind === 'element') start--;
  while (end < rest.length && rest[end].kind === 'element') end++;
  const afterId = rest[start - 1]?.id, beforeId = rest[end]?.id;
  const placements: LogElementPlacement[] = [];
  const positions = new Map(elements.map((e, i) => [e.id, i]));
  for (const edge of ['before', 'after'] as const) {
    const left = edge === 'before';
    const owner = left ? moving[0] : moving[moving.length - 1];
    const bucket = (left ? rest.slice(start, at) : rest.slice(at, end)).flatMap(n => {
      const e = byId.get(n.id);
      if (!e) return [];
      const a = e.anchor;
      const stays = a.kind === 'message'
        ? a.messageId === (left ? afterId : beforeId) && a.side === (left ? 'after' : 'before')
        : a.kind === (left ? 'sectionStart' : 'sectionEnd');
      return stays ? [] : [e];
    });
    const own = elements.filter(e => e.anchor.sectionId === sectionId && e.anchor.kind === 'message'
      && e.anchor.messageId === owner && e.anchor.side === edge
      && !held(e.anchor))
      .sort((a, b) => a.order - b.order);
    const desired = left ? [...bucket, ...own] : [...own, ...bucket];
    // 保留原桶中已有的排名；插入前缀时尽量利用空位。相同 order 按草稿数组顺序稳定排序。
    const stays = (e: LogElement) => e.anchor.kind === 'message' && e.anchor.messageId === owner && e.anchor.side === edge;
    let order = 0;
    const orders = desired.map((e, i) => {
      if (i && positions.get(desired[i - 1].id)! > positions.get(e.id)!) order++;
      return order = stays(e) ? Math.max(order, e.order) : order;
    });
    // Saturated legal ranks need room for an incoming suffix.
    if (order > Number.MAX_SAFE_INTEGER) {
      let ceiling = Number.MAX_SAFE_INTEGER;
      for (let i = orders.length - 1; i >= 0; i--) {
        ceiling = orders[i] = Math.min(ceiling, orders[i]);
        if (i && positions.get(desired[i - 1].id)! > positions.get(desired[i].id)!) ceiling--;
      }
    }
    desired.forEach((e, i) => {
      const order = orders[i];
      if (!stays(e) || order !== e.order) {
        placements.push({ elementId: e.id, anchor: { kind: 'message', sectionId, messageId: owner, side: edge }, order });
      }
    });
  }
  const remaining = messages.filter(id => !selected.has(id));
  const insertion = beforeId ? remaining.indexOf(beforeId) : remaining.length;
  remaining.splice(insertion, 0, ...moving);
  if (remaining.every((id, i) => messages[i] === id)) return placements.length ? { placements } : null;
  return { afterId, beforeId, placements };
};
