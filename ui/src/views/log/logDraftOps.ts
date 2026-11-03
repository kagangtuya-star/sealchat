// 草稿的纯函数操作：只产生合同合法的数据（元素、锚点、章节、样式绑定），不做网络请求。
import type { LogAnchor, LogDraft, LogElement, LogElementType, LogSection, LogStylePackage } from '@/types/log';
import { defaultElementContent, templatesForElementType } from '@/services/log/logRenderer';
import { elementImageRef, elementNeedsImage, setElementImageRef, type LogImageRef } from './logAssetRefs';

const IMAGE_MIME = new Set(['image/png', 'image/jpeg', 'image/webp']);
const FONT_MIME = new Set(['font/ttf', 'font/otf']);

export const uuid = (): string => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  // 回退：按 RFC 4122 v4 生成。
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
};

/**
 * 草稿是纯 JSON 数据：用 JSON 往返深拷贝，可同时剥离 Vue 响应式代理（structuredClone 无法克隆 Proxy）。
 */
export const cloneJson = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

/** 在一次 draft mutation 中设置整个消息集合；空模板即恢复自动匹配。 */
export const setMessageTemplateOverrides = (draft: LogDraft, messageIds: string[], templateId: string) => {
  const overrides = { ...draft.messageTemplateOverrides };
  for (const id of messageIds) {
    if (templateId) overrides[id] = templateId;
    else delete overrides[id];
  }
  draft.messageTemplateOverrides = overrides;
};

export const packageImageAssets = (pkg: LogStylePackage | null) => (pkg?.assets || []).filter((a) => IMAGE_MIME.has(a.mime));
export const packageFontAssets = (pkg: LogStylePackage | null) => (pkg?.assets || []).filter((a) => FONT_MIME.has(a.mime));

export const anchorKey = (a: LogAnchor) => (a.kind === 'message' ? `${a.sectionId}:${a.messageId}:${a.side}` : `${a.sectionId}:${a.kind}`);
export const sameAnchor = (a: LogAnchor, b: LogAnchor) => anchorKey(a) === anchorKey(b);

/** 同一锚点上追加到末尾的 order。 */
export const nextOrderAt = (draft: LogDraft, anchor: LogAnchor) => {
  let max = -1;
  for (const e of draft.elements) if (sameAnchor(e.anchor, anchor)) max = Math.max(max, e.order);
  return max + 1;
};

export const elementAvailability = (type: LogElementType, pkg: LogStylePackage | null): { templateId: string | null; reason: string } => {
  if (!pkg) return { templateId: null, reason: '样式包尚未加载' };
  const ids = templatesForElementType(type, pkg.templates, pkg.settingsSchema);
  if (!ids.length) return { templateId: null, reason: type === 'custom' ? '当前样式没有带参数 Schema 的 custom 模板' : `当前样式没有 ${type} 模板` };
  // 图片来源（工程素材或样式资源）在插入时选择，可用性只取决于模板。
  return { templateId: ids[0], reason: '' };
};

/** 创建元素；插图页必须带图片，封面图片可选。 */
export const createElement = (draft: LogDraft, type: LogElementType, anchor: LogAnchor, pkg: LogStylePackage | null, image: LogImageRef | null = null): LogElement | null => {
  const { templateId } = elementAvailability(type, pkg);
  if (!templateId || (elementNeedsImage(type) && !image)) return null;
  const content = defaultElementContent(type);
  if (image && (type === 'cover' || type === 'imagePlate')) setElementImageRef(content, image);
  return {
    id: uuid(),
    type,
    anchor,
    order: nextOrderAt(draft, anchor),
    templateId,
    content,
    style: {},
  };
};

export const newSection = (channelId: string, title: string, order: number): LogSection => ({
  id: uuid(),
  title,
  order,
  source: {
    channelId,
    fromTime: null,
    toTime: new Date().toISOString(),
    includeOoc: true,
    includeArchived: false,
    showQuoteContent: false,
    identityIds: [],
  },
});

export interface StyleBindProblem {
  kind: 'element' | 'header' | 'footer' | 'background' | 'font' | 'token';
  id: string;
  message: string;
  fixable: boolean;
}

/**
 * 切换样式包前的兼容性检查：服务端会整体拒绝引用不存在模板/资源的草稿，
 * 所以先在本地给出问题清单，并对可安全修复的项生成修复后的草稿。
 * 只检查样式资源引用；工程素材（draft.assets 及其引用）与样式无关，原样保留。
 */
export const rebindStyle = (draft: LogDraft, pkg: LogStylePackage): { next: LogDraft; problems: StyleBindProblem[] } => {
  const next = cloneJson(draft);
  const problems: StyleBindProblem[] = [];
  const assets = new Set(pkg.assets.map((a) => a.id));
  next.stylePackageId = pkg.id;
  next.stylePackageVersion = pkg.version;
  for (const e of next.elements) {
    const schemaOk = (id: string) => !!pkg.settingsSchema?.[id];
    let tid = e.templateId;
    if (!(tid in pkg.templates) || (e.type === 'custom' && !schemaOk(tid))) {
      const alt = templatesForElementType(e.type, pkg.templates, pkg.settingsSchema)[0];
      if (alt) {
        problems.push({ kind: 'element', id: e.id, message: `元素模板 ${tid} 不存在，改用 ${alt}`, fixable: true });
        tid = alt;
        e.templateId = alt;
        if (!schemaOk(alt)) e.style = {};
      } else {
        problems.push({ kind: 'element', id: e.id, message: `新样式没有 ${e.type} 元素可用的模板，请先删除该元素`, fixable: false });
      }
    }
    if (!schemaOk(tid) && Object.keys(e.style || {}).length) {
      e.style = {};
      problems.push({ kind: 'element', id: e.id, message: `模板 ${tid} 不接受局部样式，已清空`, fixable: true });
    }
    const image = elementImageRef(e.content);
    const assetId = image?.source === 'style' ? image.id : '';
    if (assetId && !assets.has(assetId)) {
      if (e.type === 'cover') {
        delete e.content.assetId;
        problems.push({ kind: 'element', id: e.id, message: '封面图片不属于新样式包，已移除', fixable: true });
      } else {
        const img = packageImageAssets(pkg)[0]?.id;
        if (img) {
          e.content.assetId = img;
          problems.push({ kind: 'element', id: e.id, message: '插图不属于新样式包，已替换为包内第一张图片', fixable: true });
        } else {
          problems.push({ kind: 'element', id: e.id, message: '插图页引用的样式图片不在新样式包中，请先把它换成工程素材或删除该插图页', fixable: false });
        }
      }
    }
  }
  for (const key of ['headerTemplateId', 'footerTemplateId'] as const) {
    const id = next.page[key];
    if (id && !(id in pkg.templates)) {
      next.page[key] = null;
      problems.push({ kind: key === 'headerTemplateId' ? 'header' : 'footer', id, message: `${key === 'headerTemplateId' ? '页眉' : '页脚'}模板 ${id} 不存在，已取消`, fixable: true });
    }
  }
  if (next.page.backgroundAssetId && !assets.has(next.page.backgroundAssetId)) {
    problems.push({ kind: 'background', id: next.page.backgroundAssetId, message: '背景图不属于新样式包，已取消', fixable: true });
    next.page.backgroundAssetId = null;
  }
  const font = next.page.fontFamily;
  if (!['sans-serif', 'serif', 'monospace'].includes(font) && !font.startsWith('platform:') && !packageFontAssets(pkg).some((a) => a.id === font)) {
    problems.push({ kind: 'font', id: font, message: '字体不属于新样式包，已改为无衬线', fixable: true });
    next.page.fontFamily = 'sans-serif';
  }
  for (const [k, v] of Object.entries(next.themeTokens || {})) {
    if (!(k in pkg.tokens) || typeof pkg.tokens[k] !== typeof v) {
      delete next.themeTokens[k];
      problems.push({ kind: 'token', id: k, message: `设计变量 ${k} 在新样式中不存在或类型不同，已移除`, fixable: true });
    }
  }
  return { next, problems };
};

/** 本地草稿 JSON 下载（冲突时人工合并用）。 */
export const downloadJson = (name: string, json: string) => {
  const url = URL.createObjectURL(new Blob([json], { type: 'application/json' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
};
