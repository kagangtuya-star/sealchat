// 后台 PDF Render Surface：由服务端 Render Session 在回环地址提供，只读取本 chunk 的受控 payload 与资源。
// 组装、模板、分页、字体规划全部复用正常 LOG 预览的实现（logRenderer / logFrameRuntime / logAssetLoader），
// 本文件只负责把多个章节片段按顺序排进同一文档、续接页码并补全跨片段目录页码。
import type { LogDesignMessage, LogDraft, LogElement, LogStylePackage } from '@/types/log';
import type { PlatformFontAsset, PlatformFontSubsetManifest } from '@/services/font/platformFontTypes';
import {
  buildFrameDocument,
  composeDesignSection,
  designBackgroundToken,
  designSectionToc,
  designStyleFromPackage,
  type BuildResult,
} from '@/services/log/logRenderer';
import { createLogAssetLoader, type LogAssetLoader } from '@/services/log/logAssetLoader';
import type { FrameAsset, FrameDocument, FrameLayoutResult } from '@/services/log/pagination/logFrameHost';
import { chapterViews } from '@/views/log/logChapterOps';

export const READY_GLOBAL = '__SEALCHAT_LOG_PDF__';

export interface LogPdfPart {
  sectionId: string;
  /** 本段在所属章节中的序号与总段数；超长章节被切成多段时 sliceCount > 1。 */
  slice: number;
  sliceCount: number;
  messages: LogDesignMessage[];
  /** 仅分段章节提供：整章消息顺序，用于计算整章目录。 */
  sectionMessageIds?: string[];
}

export interface LogPdfChapterSection {
  sectionId: string;
  messageIds: string[];
}

export interface LogPdfPayload {
  version: 1;
  chunk: { index: number; count: number; pageStart: number };
  draft: LogDraft;
  style: LogStylePackage | null;
  parts: LogPdfPart[];
  /** 当前 chunk 涉及的跨 chunk 视觉章节，其全部来源片段消息顺序。 */
  chapterSections?: LogPdfChapterSection[];
  /** 服务端汇总的跨 chunk 目录页码（全局页码）。 */
  tocPages?: Record<string, number>;
  font?: { id: string; meta?: PlatformFontAsset; manifest?: PlatformFontSubsetManifest } | null;
}

export interface LogPdfReport {
  status: 'ready';
  pageCount: number;
  /** 本 chunk 中可能被目录引用的节点（元素节点与各片段首节点）的全局起始页。 */
  nodePages: Record<string, number>;
  /** 本 chunk 目录中仍无法确定页码的目标。 */
  unresolvedToc: string[];
  warnings: Array<{ code: string; nodeId: string | null }>;
  missingAssets: number;
  issues: string[];
}

export interface LogPdfLayoutPort {
  hasAsset: (token: string) => boolean;
  provideAssets: (assets: FrameAsset[]) => Promise<void>;
  layout: (doc: FrameDocument, revision: number) => Promise<FrameLayoutResult | null>;
}

export interface LogPdfDom {
  /** 运行时排版完成后，把当前 .log-pages 移入累积容器并返回其中的页数。 */
  collectPages: () => number;
  /** 写入累积文档中某目录单元格的页码。 */
  tocCells: () => Array<{ target: string; set: (text: string) => void; current: () => string }>;
}

/** 按 sectionStart / sectionEnd / 消息锚点筛出本段可见的元素，保证切段后元素不重复、不丢失。 */
export const partElements = (part: LogPdfPart, elements: LogElement[]): LogElement[] => {
  const ids = new Set(part.messages.map((m) => m.message.id));
  const first = part.slice === 0;
  const last = part.slice === part.sliceCount - 1;
  return elements.filter((e) => {
    if (e.anchor.sectionId !== part.sectionId) return false;
    const anchor = e.anchor;
    if (anchor.kind === 'message') return ids.has(anchor.messageId);
    return anchor.kind === 'sectionStart' ? first : last;
  });
};

export const sectionTitles = (draft: LogDraft) => {
  const titles = new Map<string, string>();
  for (const view of chapterViews(draft)) {
    for (const s of view.sections) titles.set(s.id, view.title);
  }
  return titles;
};

export interface LogPdfPartBuild extends BuildResult { tocTargets: string[] }

/** 同一视觉章节在 chunk 内连续组装，再交给共享 Renderer 合并消息与生成目录。 */
export const buildPartDocuments = (payload: LogPdfPayload): LogPdfPartBuild[] => {
  const { draft } = payload;
  const style = payload.style ? designStyleFromPackage(payload.style, draft.themeTokens) : null;
  const titles = sectionTitles(draft);
  const views = chapterViews(draft);
  const chapterKeys = new Map(views.flatMap((v) => v.sections.map((s) => [s.id, v.key] as const)));
  const viewsByKey = new Map(views.map((v) => [v.key, v] as const));
  const chapterMessageIds = new Map((payload.chapterSections || []).map((s) => [s.sectionId, s.messageIds] as const));
  const groups: LogPdfPart[][] = [];
  for (const part of payload.parts) {
    const prev = groups.at(-1)?.at(-1);
    if (prev && prev.sliceCount === 1 && part.sliceCount === 1 && chapterKeys.get(prev.sectionId) === chapterKeys.get(part.sectionId)) {
      groups[groups.length - 1].push(part);
    } else groups.push([part]);
  }
  return groups.map((parts) => {
    const title = titles.get(parts[0].sectionId) || '';
    const compositions = parts.map((part) => {
      const elements = partElements(part, draft.elements);
      const comp = composeDesignSection(part.sectionId, title, part.messages, elements, part.slice === part.sliceCount - 1, new Map());
      const toc = part.sliceCount > 1 && part.sectionMessageIds
        ? designSectionToc(part.sectionId, title, part.sectionMessageIds, draft.elements)
        : comp.toc;
      return { nodes: comp.nodes, toc };
    });
    const chapterKey = chapterKeys.get(parts[0].sectionId);
    const chapter = chapterKey ? viewsByKey.get(chapterKey) : undefined;
    const fullChapterToc = chapter && chapter.sections.every((s) => chapterMessageIds.has(s.id))
      ? chapter.sections.map((s) => designSectionToc(s.id, title, chapterMessageIds.get(s.id)!, draft.elements))
      : null;
    const tocSources = fullChapterToc || compositions.map((comp) => comp.toc);
    const chapterEntry = tocSources.find((toc) => toc.length)?.[0];
    const toc = [...(chapterEntry ? [chapterEntry] : []), ...tocSources.flatMap((partToc) => partToc.slice(1))];
    const result = buildFrameDocument({
      style,
      page: draft.page,
      title: draft.title,
      sectionTitle: title,
      nodes: compositions.flatMap((comp) => comp.nodes),
      toc,
      backgroundToken: designBackgroundToken(draft.page),
      imageWidths: draft.imageWidths,
      templateSelection: draft,
    });
    return { ...result, tocTargets: toc.map((e) => e.targetId) };
  });
};

const firstPage = (pages: number[] | undefined) => {
  if (!pages || !pages.length) return 0;
  return Math.min(...pages.filter((p) => p > 0));
};

/**
 * 依次排版所有片段。字体分片按整 chunk 的文本一次规划，所有片段共享同一套 @font-face，
 * 避免后续片段替换样式时让已排版页面失去字体。
 */
export const renderChunk = async (
  payload: LogPdfPayload,
  loader: LogAssetLoader,
  port: LogPdfLayoutPort,
  dom: LogPdfDom,
): Promise<LogPdfReport> => {
  const built = buildPartDocuments(payload);
  if (!built.length) throw new Error('渲染内容为空');
  const merged: FrameDocument = { ...built[0].doc, blocks: built.flatMap((b) => b.doc.blocks) };
  const tokens = Array.from(new Set(built.flatMap((b) => b.assetTokens)));
  const prepared = await loader.prepare(merged, tokens, port.hasAsset);
  await port.provideAssets(prepared.assets);
  const css = prepared.doc.css;

  let pageStart = Math.max(1, Math.floor(payload.chunk.pageStart || 1));
  let total = 0;
  const nodePages: Record<string, number> = {};
  const warnings: LogPdfReport['warnings'] = [];
  let missingAssets = 0;
  const issues = Array.from(new Set(built.flatMap((b) => b.issues)));
  // 目录只会指向标题元素或章节首节点；只汇报这些节点，报告大小与消息数无关。
  const tocTargets = new Set(built.flatMap((b) => b.tocTargets));
  for (let i = 0; i < built.length; i++) {
    const doc: FrameDocument = { ...built[i].doc, css, pageNumberStart: pageStart };
    const result = await port.layout(doc, i + 1);
    if (!result) throw new Error('排版被中断');
    const count = dom.collectPages();
    if (count !== result.pageCount || count < 1) throw new Error('排版结果页数异常');
    for (const [id, pages] of Object.entries(result.map)) {
      if (!id.startsWith('el-') && !tocTargets.has(id)) continue;
      const p = firstPage(pages);
      if (p > 0 && !(id in nodePages)) nodePages[id] = pageStart + p - 1;
    }
    warnings.push(...result.warnings);
    missingAssets += result.missingAssets;
    pageStart += count;
    total += count;
  }

  // 运行时只能填写同一片段内的目录项；跨片段 / 跨 chunk 的页码在整份文档完成后补全。
  const overrides = payload.tocPages || {};
  const unresolved = new Set<string>();
  for (const cell of dom.tocCells()) {
    const page = nodePages[cell.target] ?? overrides[cell.target];
    if (page && page > 0) cell.set(String(page));
    else if (!cell.current()) unresolved.add(cell.target);
  }
  return { status: 'ready', pageCount: total, nodePages, unresolvedToc: Array.from(unresolved), warnings, missingAssets, issues };
};

/** Render Session 的受控取数端：所有字节都经由同源 asset 端点，服务端按白名单与 LOG 权限解析。 */
export const createSessionAssetLoader = (payload: LogPdfPayload, base = '') => {
  const font = payload.font || null;
  return createLogAssetLoader({
    fontInfo: async (id) => (font && font.id === id ? { meta: font.meta, manifest: font.manifest } : {}),
    fetchFont: async (token, _fontId, _chunk, signal) => {
      const r = await fetch(`${base}asset?t=${encodeURIComponent(token)}`, { signal, credentials: 'omit' });
      if (!r.ok) return null;
      return { data: await r.arrayBuffer(), mime: r.headers.get('content-type') || '' };
    },
    fetchAsset: async (token, signal) => {
      const r = await fetch(`${base}asset?t=${encodeURIComponent(token)}`, { signal, credentials: 'omit' });
      if (!r.ok) return { token, data: null, mime: '' };
      return { token, data: await r.arrayBuffer(), mime: (r.headers.get('content-type') || '').split(';')[0].trim() };
    },
  });
};
