import type { PlatformFontAsset, PlatformFontSubsetManifest } from '../font/platformFontTypes';
import {
  inferPlatformFontFormat,
  isSafePlatformFontChunkName,
  isValidPlatformFontUnicodeRange,
  selectPlatformFontChunksForText,
  type PlatformFontChunk,
} from '../font/platformFontSubsetRuntime';
import type { FrameDocument } from './pagination/logFrameHost';

export const logPlatformFontId = (family: string): string => {
  const match = /^platform:([A-Za-z0-9_-]{1,100})$/.exec(family.trim());
  return match?.[1] || '';
};

export const logFontChunkToken = (id: string, chunk: PlatformFontChunk): string => {
  // 文件名只用于稳定标识；取字节时重新匹配已验证清单，不从 token 解码路径。
  const key = Array.from(new TextEncoder().encode(chunk.name), byte => byte.toString(16).padStart(2, '0')).join('');
  return `platform-font:${id}:chunk:${key}`;
};

export interface LogFontText { text: string; reliable: boolean }

/** Final sanitized body attributes identify fonts independently of their chat family names. */
export const collectLogPlatformFontIds = (doc: FrameDocument): string[] => {
  const ids = new Set<string>();
  const pageId = logPlatformFontId(doc.page.fontFamily);
  if (pageId) ids.add(pageId);
  const holder = document.createElement('template');
  for (const block of doc.blocks) {
    holder.innerHTML = block.html;
    holder.content.querySelectorAll('[data-platform-font-id],[data-ruby-base-font-asset-id],[data-ruby-rt-font-asset-id]').forEach(el => {
      for (const attr of ['data-platform-font-id', 'data-ruby-base-font-asset-id', 'data-ruby-rt-font-asset-id']) {
        const id = el.getAttribute(attr) || '';
        if (/^[A-Za-z0-9_-]{1,100}$/.test(id)) ids.add(id);
      }
    });
  }
  return Array.from(ids);
};

export const collectLogFontText = (doc: FrameDocument): LogFontText => {
  const points = new Set<string>();
  const add = (text: string) => { for (const char of text) points.add(char); };
  const holder = document.createElement('template');
  let reliable = true;
  for (const html of [...doc.blocks.map(block => block.html), doc.headerHtml, doc.footerHtml]) {
    if (!html) continue;
    holder.innerHTML = html;
    holder.content.querySelectorAll('script,style,template').forEach(node => node.remove());
    add(holder.content.textContent || '');
    holder.content.querySelectorAll('img').forEach(img => add(`${img.getAttribute('alt') || ''}图片不可用图片加载失败`));
    if (holder.content.querySelector('[data-log-fill="pageNumber"],[data-log-toc-target]')) add('0123456789');
    if (holder.content.querySelector('.log-clue__source-label')) add('：');
    if (holder.content.querySelector('[style*="content"]')) reliable = false;
  }
  // 静态伪元素文本同样参与分片；动态 attr()/counter()/var() 无法可靠预判时使用完整字体。
  if (/\bcontent\s*:/i.test(doc.css)) {
    try {
      const sheet = new CSSStyleSheet();
      sheet.replaceSync(doc.css);
      const visit = (rules: CSSRuleList) => {
        for (const rule of Array.from(rules)) {
          const content = (rule as CSSStyleRule).style?.getPropertyValue('content').trim();
          if (content && content !== 'none' && content !== 'normal') {
            const rest = content.replace(/"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)'/g, (_all, double: string, single: string) => {
              add((double ?? single).replace(/\\([0-9a-f]{1,6})\s?|\\(.)/gi, (_escape, hex: string, char: string) =>
                hex ? String.fromCodePoint(Math.min(Number.parseInt(hex, 16), 0x10FFFF)) : char));
              return '';
            });
            if (rest.trim()) reliable = false;
          }
          if ('cssRules' in rule) visit((rule as CSSGroupingRule).cssRules);
        }
      };
      visit(sheet.cssRules);
    } catch { reliable = false; }
  }
  return { text: Array.from(points).join(''), reliable };
};

export interface LogFontPlan { tokens: string[]; css: string; subset: boolean }

export const buildLogFontPlan = (
  id: string,
  text: LogFontText,
  meta?: PlatformFontAsset,
  manifest?: PlatformFontSubsetManifest,
  forceFull = false,
): LogFontPlan => {
  if (!text.text && text.reliable) return { tokens: [], css: '', subset: false };
  const chunks = manifest?.chunks;
  const valid = Array.isArray(chunks) && chunks.length > 0 && chunks.every(chunk =>
    chunk && typeof chunk.name === 'string' && isSafePlatformFontChunkName(chunk.name) &&
    typeof chunk.unicodeRange === 'string' && isValidPlatformFontUnicodeRange(chunk.unicodeRange));
  const selected = !forceFull && text.reliable && meta?.deliveryMode === 'subset' && valid
    ? selectPlatformFontChunksForText(chunks, text.text) : [];
  const subset = selected.length > 0;
  const faces = subset ? selected.map(chunk => ({ token: logFontChunkToken(id, chunk), name: chunk.name, mime: chunk.mimeType, range: chunk.unicodeRange }))
    : [{ token: `platform-font:${id}:full`, name: meta?.sourceFileName || '', mime: meta?.sourceMimeType, range: undefined }];
  const css = faces.map(face => {
    const format = inferPlatformFontFormat(face.name, face.mime);
    // meta 的 weight/style 不是任意 CSS 注入入口。
    const weight = /^(normal|bold|[1-9]00)( [1-9]00)?$/.test(meta?.weight || '') ? meta!.weight : '400';
    const style = /^(normal|italic|oblique)$/.test(meta?.style || '') ? meta!.style : 'normal';
    return `@font-face{font-family:"sc-log-platform-${id}";src:url("__LOGASSET_${face.token}__")${format ? ` format("${format}")` : ''};font-weight:${weight};font-style:${style};font-display:swap;${face.range ? `unicode-range:${face.range};` : ''}}`;
  }).join('\n');
  return { tokens: faces.map(face => face.token), css, subset };
};
