// 章节编排的纯函数：章节（视觉分组）→ 来源片段（单频道消息范围）。
// 编排结果总是写回为稳定的全局 section.order：同章片段连续、章节按 chapter.order 排列，
// 发布器只按 section.order 逐片段输出即可得到最终顺序。旧草稿（无 chapters）在仍全部为
// 单来源章节时保持旧格式，只有出现多来源章节时才物化章节层。
import type { LogChapter, LogDraft, LogSection, LogSourceSpec } from '@/types/log';
import { uuid } from './logDraftOps';

export interface LogChapterView {
  /** 章节 id；旧草稿的隐式章节使用其唯一片段的 section id。 */
  key: string;
  title: string;
  implicit: boolean;
  sections: LogSection[];
}

export interface LogChapterLayoutEntry {
  key: string;
  title: string;
  sectionIds: string[];
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const isChapterMode = (d: Pick<LogDraft, 'chapters'>) => !!d.chapters?.length;

const byOrder = <T extends { order: number }>(list: T[]) => list.slice().sort((a, b) => a.order - b.order);

export const chapterViews = (d: Pick<LogDraft, 'sections' | 'chapters'>): LogChapterView[] => {
  const sections = byOrder(d.sections);
  if (!isChapterMode(d)) return sections.map((s) => ({ key: s.id, title: s.title, implicit: true, sections: [s] }));
  const views = byOrder(d.chapters || []).map((c) => ({ key: c.id, title: c.title, implicit: false, sections: [] as LogSection[] }));
  const index = new Map(views.map((v) => [v.key, v]));
  for (const s of sections) {
    const view = (s.chapterId && index.get(s.chapterId)) || null;
    if (view) view.sections.push(s);
    // 服务端拒绝孤立片段；本地若出现则作为单独的隐式章节显示，不让片段消失。
    else views.push({ key: s.id, title: s.title, implicit: true, sections: [s] });
  }
  return views.filter((v) => v.sections.length);
};

export const layoutOf = (views: LogChapterView[]): LogChapterLayoutEntry[] =>
  views.map((v) => ({ key: v.key, title: v.title, sectionIds: v.sections.map((s) => s.id) }));

/** 片段的展示名称：自定义名称优先，否则使用频道名。 */
export const sectionLabel = (s: LogSection, channelName: (id: string) => string) => s.title.trim() || channelName(s.source.channelId);

/** 章节的展示标题。 */
export const chapterTitle = (v: Pick<LogChapterView, 'title'>, index: number) => v.title.trim() || `第 ${index + 1} 章`;

/**
 * 把编排结构写回草稿（在 draftCtl.update 的 mutate 内调用）。
 * 返回旧 key → 新章节 id 的映射：隐式章节物化后 key 会变化，调用方据此保持选中状态。
 */
export const writeLayout = (d: LogDraft, layout: LogChapterLayoutEntry[]): Map<string, string> => {
  const byId = new Map(d.sections.map((s) => [s.id, s]));
  const placed = new Set<string>();
  const entries: LogChapterLayoutEntry[] = [];
  for (const e of layout) {
    const ids = e.sectionIds.filter((id) => byId.has(id) && !placed.has(id));
    ids.forEach((id) => placed.add(id));
    if (ids.length) entries.push({ ...e, sectionIds: ids });
  }
  // 布局之外的片段不能被静默丢弃：各自成章追加在末尾。
  for (const s of byOrder(d.sections)) {
    if (!placed.has(s.id)) entries.push({ key: s.id, title: s.title, sectionIds: [s.id] });
  }
  const keys = new Map<string, string>();
  if (!isChapterMode(d) && entries.every((e) => e.sectionIds.length === 1)) {
    entries.forEach((e, i) => {
      const s = byId.get(e.sectionIds[0])!;
      s.order = i;
      s.title = e.title.slice(0, 200);
      delete s.chapterId;
      keys.set(e.key, s.id);
    });
    delete d.chapters;
    return keys;
  }
  const materializing = !isChapterMode(d);
  const used = new Set<string>();
  const chapters: LogChapter[] = entries.map((e, i) => {
    let id = e.key;
    if (!UUID_RE.test(id) || byId.has(id) || used.has(id)) id = uuid();
    used.add(id);
    keys.set(e.key, id);
    return { id, title: e.title.slice(0, 200), order: i };
  });
  let order = 0;
  entries.forEach((e, i) => {
    for (const sid of e.sectionIds) {
      const s = byId.get(sid)!;
      // 旧章节标题已成为章节标题；片段名称留空（显示为频道名），避免同名重复。
      if (materializing && s.title === e.title) s.title = '';
      s.chapterId = chapters[i].id;
      s.order = order++;
    }
  });
  if (chapters.length) d.chapters = chapters;
  else delete d.chapters;
  return keys;
};

// ---- 布局变换（不修改输入） ----

const cloneLayout = (layout: LogChapterLayoutEntry[]) => layout.map((e) => ({ ...e, sectionIds: e.sectionIds.slice() }));

export const moveChapter = (layout: LogChapterLayoutEntry[], key: string, toIndex: number) => {
  const next = cloneLayout(layout);
  const from = next.findIndex((e) => e.key === key);
  if (from < 0) return next;
  const [entry] = next.splice(from, 1);
  next.splice(Math.max(0, Math.min(toIndex > from ? toIndex - 1 : toIndex, next.length)), 0, entry);
  return next;
};

/** 把片段移到目标章节的 toIndex 处（toIndex 为移除前目标列表中的插入位置）；原章节为空时移除。 */
export const moveSection = (layout: LogChapterLayoutEntry[], sectionId: string, toKey: string, toIndex: number) => {
  const next = cloneLayout(layout);
  const source = next.find((e) => e.sectionIds.includes(sectionId));
  const target = next.find((e) => e.key === toKey);
  if (!source || !target) return next;
  const from = source.sectionIds.indexOf(sectionId);
  let index = toIndex;
  if (source === target && index > from) index -= 1;
  source.sectionIds.splice(from, 1);
  target.sectionIds.splice(Math.max(0, Math.min(index, target.sectionIds.length)), 0, sectionId);
  return next.filter((e) => e.sectionIds.length);
};

/** 把片段拆出为紧随原章节之后的独立章节。 */
export const detachSection = (layout: LogChapterLayoutEntry[], sectionId: string, title: string, newKey: string) => {
  const next = cloneLayout(layout);
  const i = next.findIndex((e) => e.sectionIds.includes(sectionId));
  if (i < 0 || next[i].sectionIds.length < 2) return next;
  next[i].sectionIds = next[i].sectionIds.filter((id) => id !== sectionId);
  next.splice(i + 1, 0, { key: newKey, title, sectionIds: [sectionId] });
  return next;
};

/** 把章节的全部片段并入上一章末尾。 */
export const mergeIntoPrevious = (layout: LogChapterLayoutEntry[], key: string) => {
  const next = cloneLayout(layout);
  const i = next.findIndex((e) => e.key === key);
  if (i < 1) return next;
  next[i - 1].sectionIds.push(...next[i].sectionIds);
  next.splice(i, 1);
  return next;
};

export const renameChapter = (layout: LogChapterLayoutEntry[], key: string, title: string) =>
  cloneLayout(layout).map((e) => (e.key === key ? { ...e, title } : e));

/** 删除片段：同时删除锚定在片段上的 LOG 元素，避免留下孤立锚点。 */
export const removeSections = (d: LogDraft, ids: string[]) => {
  const gone = new Set(ids);
  const layout = layoutOf(chapterViews(d)).map((e) => ({ ...e, sectionIds: e.sectionIds.filter((id) => !gone.has(id)) }));
  d.sections = d.sections.filter((s) => !gone.has(s.id));
  d.elements = d.elements.filter((e) => !gone.has(e.anchor.sectionId));
  return writeLayout(d, layout);
};

/**
 * 接续片段复用上一片段的结束时间作为排他起点：后端查询使用 created_at > fromTime，
 * 因而不会重复边界消息，也不会用 +1ms 制造时间空隙。若原结束时间在未来，新片段先保持空范围。
 */
export const continuationSourceRange = (s: LogSection, now = new Date()): Pick<LogSourceSpec, 'fromTime' | 'fromExclusive' | 'toTime'> => {
  const boundary = s.source.toTime;
  const boundaryMs = Date.parse(boundary);
  const toTime = Number.isFinite(boundaryMs) && boundaryMs >= now.getTime() ? boundary : now.toISOString();
  return { fromTime: boundary, fromExclusive: true, toTime };
};

// ---- 来源摘要 ----

const pad = (n: number) => String(n).padStart(2, '0');
const stamp = (d: Date, withDate: boolean, withYear: boolean) =>
  `${withDate ? `${withYear ? `${d.getFullYear()}/` : ''}${pad(d.getMonth() + 1)}/${pad(d.getDate())} ` : ''}${pad(d.getHours())}:${pad(d.getMinutes())}`;

/** 紧凑的时间范围：同日结束只显示时刻；非今年显示年份。 */
export const formatSourceRange = (source: LogSourceSpec, now = new Date()) => {
  const to = new Date(source.toTime);
  const from = source.fromTime ? new Date(source.fromTime) : null;
  const toYear = Number.isFinite(to.getTime()) && to.getFullYear() !== now.getFullYear();
  if (!from || !Number.isFinite(from.getTime())) return `最早 → ${Number.isFinite(to.getTime()) ? stamp(to, true, toYear) : '?'}`;
  const fromYear = from.getFullYear() !== now.getFullYear();
  const sameDay = from.toDateString() === to.toDateString();
  return `${stamp(from, true, fromYear)} → ${stamp(to, !sameDay, toYear && !sameDay)}`;
};

export interface LogSourceFlag {
  key: 'identity' | 'ooc' | 'archived' | 'quote';
  text: string;
  /** 偏离默认（全部角色、含 OOC、不含归档、隐藏引用）时高亮。 */
  notable: boolean;
}

export const sourceFlags = (source: LogSourceSpec): LogSourceFlag[] => [
  { key: 'identity', text: source.identityIds.length ? `${source.identityIds.length} 个角色` : '全部角色', notable: source.identityIds.length > 0 },
  { key: 'ooc', text: source.includeOoc ? '含 OOC' : '不含 OOC', notable: !source.includeOoc },
  { key: 'archived', text: source.includeArchived ? '含归档' : '不含归档', notable: source.includeArchived },
  { key: 'quote', text: source.showQuoteContent ? '显示引用' : '隐藏引用', notable: !!source.showQuoteContent },
];

/** 频道的稳定色相，用于区分同一章节中来自不同频道的片段。 */
export const channelHue = (channelId: string) => {
  let h = 0;
  for (let i = 0; i < channelId.length; i++) h = (h * 31 + channelId.charCodeAt(i)) % 360;
  return h;
};
