// 草稿三方合并：本地基版本（base）、本地当前（mine）、服务端最新（theirs）。
// 多位设计者同时编辑同一工程时，CAS 冲突后用它把本地修改重放到最新版本上，而不是要求整页重新加载。
// 粒度按草稿结构划分：互不相关的字段、不同元素、不同消息的覆盖各自合并；
// 同一处被双方改成不同的值才算冲突，此时不猜测，交还给原有的冲突处理。
import type { LogDraft } from '@/types/log';

export type DraftMergeResult =
  | { ok: true; draft: LogDraft }
  | { ok: false; conflicts: string[] };

type Json = unknown;
type Obj = Record<string, Json>;

const isObj = (v: Json): v is Obj => !!v && typeof v === 'object' && !Array.isArray(v);

/** 结构相等：忽略对象键顺序，缺失字段与 undefined 等价。 */
export const sameJson = (a: Json, b: Json): boolean => {
  if (a === b) return true;
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
    return a.every((v, i) => sameJson(v, b[i]));
  }
  if (isObj(a) && isObj(b)) {
    const keys = new Set([...Object.keys(a), ...Object.keys(b)]);
    for (const k of keys) if (!sameJson(a[k], b[k])) return false;
    return true;
  }
  return false;
};

/** 可省略的集合字段：缺失、空数组、空对象、空串视为同一个“空”。 */
const emptyish = (v: Json) => v === undefined || v === null || v === ''
  || (Array.isArray(v) && v.length === 0) || (isObj(v) && Object.keys(v).length === 0);
const sameLoose = (a: Json, b: Json) => (emptyish(a) && emptyish(b)) || sameJson(a, b);

const clone = <T>(v: T): T => (v === undefined ? v : JSON.parse(JSON.stringify(v)));

class Conflicts {
  readonly list: string[] = [];
  add(label: string) {
    if (!this.list.includes(label)) this.list.push(label);
  }
}

/** 整体取值：一方未改取另一方；双方改成相同值也接受。 */
const pick = (base: Json, mine: Json, theirs: Json, label: string, c: Conflicts): Json => {
  if (sameLoose(mine, base)) return theirs;
  if (sameLoose(theirs, base) || sameLoose(mine, theirs)) return mine;
  c.add(label);
  return theirs;
};

/** 映射字段按键合并；存在约束的字段可作为一个值取舍。 */
const mergeMap = (base: Json, mine: Json, theirs: Json, label: (key: string) => string, c: Conflicts,
  group?: { keys: readonly string[]; label: string }): Obj => {
  const b = isObj(base) ? base : {};
  const m = isObj(mine) ? mine : {};
  const t = isObj(theirs) ? theirs : {};
  const out: Obj = {};
  let grouped: Obj | undefined;
  if (group) {
    const { keys, label } = group;
    const groupOf = (obj: Obj) => Object.fromEntries(keys.map((k) => [k, obj[k]]));
    grouped = pick(groupOf(b), groupOf(m), groupOf(t), label, c) as Obj;
  }
  const keys = [...Object.keys(t), ...Object.keys(m).filter((k) => !(k in t)), ...Object.keys(b).filter((k) => !(k in t) && !(k in m))];
  for (const k of keys) {
    const v = group?.keys.includes(k) ? grouped?.[k] : pick(b[k], m[k], t[k], label(k), c);
    if (v !== undefined) out[k] = v;
  }
  return out;
};

type Item = Obj & { id: string };
const byId = (v: Json) => new Map((Array.isArray(v) ? v : []).filter((x): x is Item => isObj(x) && typeof x.id === 'string').map((x) => [x.id, x]));

/** 带 id 的数组：按 id 合并；顺序沿用服务端，再追加只在本地新增的项。 */
const mergeById = (base: Json, mine: Json, theirs: Json, mergeItem: (b: Item | undefined, m: Item | undefined, t: Item | undefined, id: string) => Item | undefined): Item[] => {
  const b = byId(base);
  const m = byId(mine);
  const t = byId(theirs);
  const ids = [...t.keys(), ...[...m.keys()].filter((id) => !t.has(id))];
  const out: Item[] = [];
  for (const id of ids) {
    const v = mergeItem(b.get(id), m.get(id), t.get(id), id);
    if (v) out.push(v);
  }
  return out;
};

/** 规则内容按 id 取舍，优先级顺序单独处理，避免无声改变 first-match 语义。 */
const mergeDisplayRules = (base: Json, mine: Json, theirs: Json, c: Conflicts) => {
  const rules = mergeById(base, mine, theirs, (b, m, t, id) => pick(b, m, t, `显示规则 ${id.slice(0, 8)}`, c) as Item | undefined);
  const merged = byId(rules);
  const orderOf = (v: Json) => [...byId(v).keys()].filter((id) => merged.has(id));
  const order = pick(orderOf(base), orderOf(mine), orderOf(theirs), '显示规则优先级', c) as string[];
  return order.map((id) => merged.get(id)!);
};

const elementLabel = (el: Item | undefined, id: string) => {
  const type = el && typeof el.type === 'string' ? el.type : '元素';
  return `元素（${type} ${id.slice(0, 8)}）`;
};

/** 元素：位置（锚点 + 顺序）一起取舍；正文按字段合并；其余字段整体取舍。 */
const mergeElement = (b: Item | undefined, m: Item | undefined, t: Item | undefined, id: string, c: Conflicts): Item | undefined => {
  const label = elementLabel(t || m || b, id);
  if (!b) {
    // 双方都新建了同 id 的元素只可能是同一份数据的回声。
    if (m && t && !sameJson(m, t)) c.add(label);
    return t || m;
  }
  if (!m || !t) {
    // 一方删除：另一方未改则删除生效，否则冲突。
    const kept = m || t;
    if (kept && !sameJson(kept, b)) {
      c.add(label);
      return t;
    }
    return undefined;
  }
  const out: Obj = {};
  const place = pick({ anchor: b.anchor, order: b.order }, { anchor: m.anchor, order: m.order }, { anchor: t.anchor, order: t.order }, `${label}的位置`, c) as Obj;
  out.anchor = place.anchor;
  out.order = place.order;
  for (const key of new Set([...Object.keys(t), ...Object.keys(m), ...Object.keys(b)])) {
    if (key === 'anchor' || key === 'order' || key === 'id') continue;
    const v = key === 'content'
      ? mergeMap(b.content, m.content, t.content, () => `${label}的内容`, c, {
        keys: ['assetId', 'projectAssetId'], label: `${label}的图片来源`,
      })
      : pick(b[key], m[key], t[key], label, c);
    if (v !== undefined) out[key] = v;
  }
  return { id, ...out } as Item;
};

/** 以 group 整体取舍的字段：彼此存在约束，拆开合并可能得到服务端会拒绝的组合。 */
const GROUPS: Array<{ keys: string[]; label: string }> = [
  { keys: ['sections', 'chapters'], label: '章节与来源' },
  { keys: ['stylePackageId', 'stylePackageVersion'], label: '样式绑定' },
  { keys: ['replay'], label: 'Replay 数据' },
  { keys: ['title'], label: '标题' },
  { keys: ['schemaVersion'], label: '草稿版本格式' },
  { keys: ['messageImageLayout'], label: '正文图片布局' },
];
const MAPS: Record<string, (key: string) => string> = {
  themeTokens: (k) => `设计变量 ${k}`,
  imageWidths: () => '图片尺寸',
  messageTemplateOverrides: () => '消息模板覆盖',
  messageContentOverrides: () => '消息正文覆盖',
};
/** 可省略的映射：合并为空时删除字段，保持草稿与本地编辑习惯一致。 */
const OPTIONAL_MAPS = new Set(['imageWidths', 'messageTemplateOverrides', 'messageContentOverrides']);

export const mergeLogDrafts = (base: LogDraft, mine: LogDraft, theirs: LogDraft): DraftMergeResult => {
  const b = base as unknown as Obj;
  const m = mine as unknown as Obj;
  const t = theirs as unknown as Obj;
  const c = new Conflicts();
  const out: Obj = {};
  const handled = new Set<string>();
  const set = (key: string, value: Json) => {
    handled.add(key);
    if (value === undefined) delete out[key];
    else out[key] = clone(value);
  };

  for (const group of GROUPS) {
    const g = (src: Obj) => Object.fromEntries(group.keys.map((k) => [k, src[k]]));
    const chosen = pick(g(b), g(m), g(t), group.label, c) as Obj;
    for (const k of group.keys) set(k, chosen[k]);
  }
  for (const [key, label] of Object.entries(MAPS)) {
    const merged = mergeMap(b[key], m[key], t[key], label, c);
    set(key, OPTIONAL_MAPS.has(key) && !Object.keys(merged).length ? undefined : merged);
  }
  const rules = mergeDisplayRules(b.displayRules, m.displayRules, t.displayRules, c);
  set('displayRules', rules.length ? rules : undefined);
  // 页面设置：字段各自合并；页边距整体取舍，互斥的背景图来源也整体取舍。
  {
    const bp = isObj(b.page) ? b.page : {};
    const mp = isObj(m.page) ? m.page : {};
    const tp = isObj(t.page) ? t.page : {};
    const page: Obj = {};
    const backgroundKeys = ['backgroundAssetId', 'backgroundProjectAssetId'];
    const backgroundOf = (p: Obj) => Object.fromEntries(backgroundKeys.map((k) => [k, p[k] ?? null]));
    const background = pick(backgroundOf(bp), backgroundOf(mp), backgroundOf(tp), '页面背景图', c) as Obj;
    for (const k of new Set([...Object.keys(tp), ...Object.keys(mp), ...Object.keys(bp)])) {
      const v = backgroundKeys.includes(k) ? background[k] : pick(bp[k], mp[k], tp[k], '页面设置', c);
      if (v !== undefined) page[k] = v;
    }
    set('page', page);
  }
  set('elements', mergeById(b.elements, m.elements, t.elements, (bi, mi, ti, id) => mergeElement(bi, mi, ti, id, c)));
  {
    const assets = mergeById(b.assets, m.assets, t.assets, (bi, mi, ti, id) => pick(bi, mi, ti, `图片素材 ${id.slice(0, 8)}`, c) as Item | undefined);
    set('assets', assets.length || Array.isArray(m.assets) || Array.isArray(t.assets) ? assets : undefined);
  }
  for (const key of new Set([...Object.keys(t), ...Object.keys(m), ...Object.keys(b)])) {
    if (!handled.has(key)) set(key, pick(b[key], m[key], t[key], key, c));
  }

  if (c.list.length) return { ok: false, conflicts: c.list };
  // 本地没有改动时原样采用服务端版本（键顺序也一致），避免无意义的再次保存。
  if (sameJson(out, t)) return { ok: true, draft: clone(theirs) };
  return { ok: true, draft: out as unknown as LogDraft };
};
