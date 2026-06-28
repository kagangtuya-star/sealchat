/**
 * TipTap JSON 渲染工具
 * 将 TipTap JSON 格式转换为 HTML，支持自定义样式
 */

import { urlBase } from '@/stores/_config';
import { isLocalChatLink, parseChatLink } from './messageLink';
import { normalizePerformanceEffect } from './tiptap-performance-mark';
import {
  SMART_LINK_DATA_ATTR,
  SMART_LINK_IMAGE_ROLE_ATTR,
  SMART_LINK_NODE_TYPE,
  SMART_LINK_TEXT_IMAGE_ROLE,
  normalizeSmartLinkAttrs,
  smartLinkToPlainText,
} from './tiptapSmartLink';

interface TipTapNode {
  type: string;
  attrs?: Record<string, any>;
  content?: TipTapNode[];
  text?: string;
  marks?: Array<{ type: string; attrs?: Record<string, any> }>;
}

interface RenderOptions {
  baseUrl?: string;
  imageClass?: string;
  linkClass?: string;
  attachmentResolver?: (value: string) => string;
  textRenderer?: (text: string) => string;
}

const DAY_TEXT_LUMINANCE_THRESHOLD = 0.9;
const NIGHT_TEXT_LUMINANCE_THRESHOLD = 0.15;
const DAY_TEXT_DISTANCE_THRESHOLD = 24;
const NIGHT_TEXT_DISTANCE_THRESHOLD = 24;
const DAY_TEXT_BACKGROUNDS = [
  { r: 255, g: 255, b: 255 },
  { r: 245, g: 245, b: 245 },
  { r: 251, g: 253, b: 247 },
];
const NIGHT_TEXT_BACKGROUNDS = [
  { r: 63, g: 63, b: 70 },
  { r: 45, g: 45, b: 49 },
];

function normalizeCssColor(value: string): string {
  return value.replace(/!important/gi, '').trim();
}

function parseCssColor(value: string): { r: number; g: number; b: number } | null {
  const raw = value.trim();
  if (!raw) return null;

  const hexMatch = raw.match(/^#([0-9a-fA-F]{3,8})$/);
  if (hexMatch) {
    const hex = hexMatch[1];
    if (hex.length === 3 || hex.length === 4) {
      const r = parseInt(hex[0] + hex[0], 16);
      const g = parseInt(hex[1] + hex[1], 16);
      const b = parseInt(hex[2] + hex[2], 16);
      return { r, g, b };
    }
    if (hex.length === 6 || hex.length === 8) {
      const r = parseInt(hex.slice(0, 2), 16);
      const g = parseInt(hex.slice(2, 4), 16);
      const b = parseInt(hex.slice(4, 6), 16);
      return { r, g, b };
    }
  }

  const rgbMatch = raw.match(/^rgba?\((.+)\)$/i);
  if (rgbMatch) {
    const parts = rgbMatch[1].split(',').map((part) => part.trim());
    if (parts.length >= 3) {
      const r = Number.parseFloat(parts[0]);
      const g = Number.parseFloat(parts[1]);
      const b = Number.parseFloat(parts[2]);
      if (Number.isFinite(r) && Number.isFinite(g) && Number.isFinite(b)) {
        return { r, g, b };
      }
    }
  }

  return null;
}

function relativeLuminance({ r, g, b }: { r: number; g: number; b: number }): number {
  const toLinear = (value: number) => {
    const channel = value / 255;
    return channel <= 0.03928 ? channel / 12.92 : Math.pow((channel + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b);
}

function colorDistance(a: { r: number; g: number; b: number }, b: { r: number; g: number; b: number }): number {
  const dr = a.r - b.r;
  const dg = a.g - b.g;
  const db = a.b - b.b;
  return Math.sqrt(dr * dr + dg * dg + db * db);
}

function getDisplayPalette(): 'day' | 'night' {
  if (typeof document === 'undefined') return 'day';
  const palette = document.documentElement?.dataset?.displayPalette;
  return palette === 'night' ? 'night' : 'day';
}

function shouldFilterTextColor(value: string): boolean {
  const rgb = parseCssColor(value);
  if (!rgb) return false;
  const palette = getDisplayPalette();
  if (palette === 'night') {
    if (relativeLuminance(rgb) <= NIGHT_TEXT_LUMINANCE_THRESHOLD) return true;
    return NIGHT_TEXT_BACKGROUNDS.some((bg) => colorDistance(rgb, bg) <= NIGHT_TEXT_DISTANCE_THRESHOLD);
  }
  if (relativeLuminance(rgb) >= DAY_TEXT_LUMINANCE_THRESHOLD) return true;
  return DAY_TEXT_BACKGROUNDS.some((bg) => colorDistance(rgb, bg) <= DAY_TEXT_DISTANCE_THRESHOLD);
}

const MENTION_TOKEN_REGEX = /<at\s+id=(['"])([^'"]*)\1(?:\s+name=(['"])(.*?)\3)?\s*\/?\s*>/g;
const SPOILER_OPEN_TAG = '<span class="tiptap-spoiler" data-spoiler="true">';
const SPOILER_CLOSE_TAG = '</span>';
type TextDecorationLine = 'underline' | 'line-through';
type TextDecorationThickness = 'thin' | 'regular' | 'bold';
type TextDecorationPattern = 'solid' | 'dotted' | 'dense-dotted';
type TextDecorationWave = 'none' | 'soft' | 'heavy';
type TextDecorationCount = 'single' | 'double' | 'triple';

const DECORATION_THICKNESS_CSS: Record<TextDecorationThickness, string> = {
  thin: '1px',
  regular: '0.12em',
  bold: '0.18em',
};

function isTextDecorationLine(value: unknown): value is TextDecorationLine {
  return value === 'underline' || value === 'line-through';
}

function normalizeTextDecorationStyle(attrs: Record<string, any> = {}) {
  const thickness = attrs.textDecorationThickness;
  const pattern = attrs.textDecorationPattern;
  const wave = attrs.textDecorationWave;
  const count = attrs.textDecorationCount;
  return {
    thickness: (thickness === 'thin' || thickness === 'regular' || thickness === 'bold' ? thickness : 'regular') as TextDecorationThickness,
    pattern: (pattern === 'solid' || pattern === 'dotted' || pattern === 'dense-dotted' ? pattern : 'solid') as TextDecorationPattern,
    wave: (wave === 'none' || wave === 'soft' || wave === 'heavy' ? wave : 'none') as TextDecorationWave,
    count: (count === 'single' || count === 'double' || count === 'triple' ? count : 'single') as TextDecorationCount,
  };
}

function renderTextDecorationStyleAttrs(sourceAttrs: Record<string, any> = {}) {
  const line = isTextDecorationLine(sourceAttrs.textDecorationLine) ? sourceAttrs.textDecorationLine : null;
  if (!line) {
    return { attrs: [] as string[], styles: [] as string[] };
  }
  const style = normalizeTextDecorationStyle(sourceAttrs);
  const modifierClasses = [
    `tiptap-text-decoration--${line === 'underline' ? 'underline' : 'strike'}`,
    `tiptap-text-decoration--${style.pattern}`,
    `tiptap-text-decoration--wave-${style.wave}`,
    `tiptap-text-decoration--${style.count}`,
  ].join(' ');
  return {
    attrs: [
      `class="tiptap-text-decoration ${modifierClasses}"`,
      `data-text-decoration-line="${line}"`,
      `data-text-decoration-thickness="${style.thickness}"`,
      `data-text-decoration-pattern="${style.pattern}"`,
      `data-text-decoration-wave="${style.wave}"`,
      `data-text-decoration-count="${style.count}"`,
    ],
    styles: [
      `--tiptap-decoration-line-kind: ${line}`,
      `--tiptap-decoration-thickness: ${DECORATION_THICKNESS_CSS[style.thickness]}`,
      `--tiptap-decoration-pattern: ${style.pattern}`,
      `--tiptap-decoration-wave: ${style.wave}`,
      `--tiptap-decoration-count: ${style.count}`,
    ],
  };
}

function unwrapSpoilerFragment(fragment: string): string | null {
  if (!fragment.startsWith(SPOILER_OPEN_TAG) || !fragment.endsWith(SPOILER_CLOSE_TAG)) {
    return null;
  }
  return fragment.slice(SPOILER_OPEN_TAG.length, -SPOILER_CLOSE_TAG.length);
}

function mergeAdjacentSpoilerFragments(fragments: string[]): string {
  if (fragments.length <= 1) {
    return fragments.join('');
  }

  const merged: string[] = [];

  fragments.forEach((fragment) => {
    if (!fragment) {
      return;
    }

    const currentInner = unwrapSpoilerFragment(fragment);
    const previous = merged.length > 0 ? merged[merged.length - 1] : '';
    const previousInner = previous ? unwrapSpoilerFragment(previous) : null;

    if (currentInner !== null && previousInner !== null) {
      merged[merged.length - 1] = `${SPOILER_OPEN_TAG}${previousInner}${currentInner}${SPOILER_CLOSE_TAG}`;
      return;
    }

    merged.push(fragment);
  });

  return merged.join('');
}

function decodeMentionText(value: string): string {
  return value
    .replace(/&quot;/g, '"')
    .replace(/&#039;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');
}

function renderMentionAwareText(text: string): string {
  if (!text) {
    return '';
  }

  MENTION_TOKEN_REGEX.lastIndex = 0;
  let lastIndex = 0;
  let result = '';
  let match: RegExpExecArray | null;

  while ((match = MENTION_TOKEN_REGEX.exec(text)) !== null) {
    if (match.index > lastIndex) {
      result += escapeHtmlPreservingBoundarySpaces(text.slice(lastIndex, match.index));
    }

    const atId = decodeMentionText(match[2] || '').trim();
    const atName = decodeMentionText(match[4] || '').trim();
    const displayName = atName || atId || '用户';
    const className = atId === 'all' ? 'mention-capsule mention-capsule--all' : 'mention-capsule';
    result += `<span class="${className}">@${escapeHtml(displayName)}</span>`;

    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < text.length) {
    result += escapeHtmlPreservingBoundarySpaces(text.slice(lastIndex));
  }

  return result;
}

function mentionAwarePlainText(text: string): string {
  if (!text) {
    return '';
  }

  MENTION_TOKEN_REGEX.lastIndex = 0;
  return text.replace(MENTION_TOKEN_REGEX, (_full, _quote, id: string, _nameQuote, name: string) => {
    const atId = decodeMentionText(id || '').trim();
    const atName = decodeMentionText(name || '').trim();
    return `@${atName || atId || '用户'}`;
  });
}

function resolveRenderableSmartLinkValue(
  value: string,
  options: RenderOptions,
  baseUrl: string,
): string {
  const resolver = options.attachmentResolver;
  if (resolver) {
    const resolved = resolver(value);
    if (resolved) {
      return resolved;
    }
  }
  return buildFallbackAttachmentUrl(value, baseUrl);
}

function applyCombinedTextStyle(text: string, marks: Array<{ type: string; attrs?: Record<string, any> }>): string {
  const textStyleMark = marks.find((mark) => mark?.type === 'textStyle');
  const highlightMark = marks.find((mark) => mark?.type === 'highlight');
  if (!textStyleMark && !highlightMark) {
    return text;
  }

  const attrs: string[] = [];
  const styles: string[] = [];

  if (textStyleMark?.attrs?.fontAssetId) {
    attrs.push(`data-platform-font-id="${escapeHtml(String(textStyleMark.attrs.fontAssetId))}"`);
  }
  if (textStyleMark?.attrs?.platformFontFamily) {
    attrs.push(`data-platform-font-family="${escapeHtml(String(textStyleMark.attrs.platformFontFamily))}"`);
  }
  if (textStyleMark?.attrs?.fontSize) {
    const fontSize = escapeHtml(String(textStyleMark.attrs.fontSize));
    attrs.push(`data-font-size="${fontSize}"`);
    styles.push(`font-size: ${fontSize} !important`);
  }
  if (textStyleMark?.attrs?.fontFamily) {
    styles.push(`font-family: ${escapeHtml(String(textStyleMark.attrs.fontFamily))} !important`);
  }
  if (textStyleMark?.attrs?.color) {
    const normalizedColor = normalizeCssColor(String(textStyleMark.attrs.color));
    if (normalizedColor && !shouldFilterTextColor(normalizedColor)) {
      styles.push(`color: ${escapeHtml(normalizedColor)} !important`);
    }
  }
  if (textStyleMark?.attrs) {
    const decoration = renderTextDecorationStyleAttrs(textStyleMark.attrs);
    attrs.push(...decoration.attrs);
    styles.push(...decoration.styles);
  }
  if (highlightMark) {
    const bgColor = escapeHtml(String(highlightMark.attrs?.color || '#fef08a'));
    styles.push(`background-color: ${bgColor} !important`);
  }

  if (!attrs.length && !styles.length) {
    return text;
  }

  const tag = highlightMark ? 'mark' : 'span';
  const attrString = [
    ...attrs,
    styles.length ? `style="${styles.join('; ')}"` : '',
  ]
    .filter(Boolean)
    .join(' ');

  return `<${tag}${attrString ? ` ${attrString}` : ''}>${text}</${tag}>`;
}

function applyRubyMark(text: string, mark: { type: string; attrs?: Record<string, any> }): string {
  const rubyText = escapeHtml(String(mark.attrs?.rubyText || '').trim());
  if (!rubyText) {
    return text;
  }
  const rubyAttrs = mark.attrs || {};
  const variables: string[] = [];
  const dataAttrs: string[] = [`data-ruby-text="${rubyText}"`];
  const pushRubyAttr = (key: string, cssVar?: string, dataKeyOverride?: string) => {
    const value = String(rubyAttrs[key] || '').trim();
    if (!value) {
      return;
    }
    const escaped = escapeHtml(value);
    const dataKey = dataKeyOverride || `data-${key.replace(/[A-Z]/g, (char) => `-${char.toLowerCase()}`)}`;
    dataAttrs.push(`${dataKey}="${escaped}"`);
    if (cssVar) {
      variables.push(`${cssVar}: ${escaped}`);
    }
  };
  pushRubyAttr('rubyBaseFontFamily', '--ruby-base-font-family');
  pushRubyAttr('rubyRtFontFamily', '--ruby-rt-font-family');
  pushRubyAttr('rubyBaseFontSize', '--ruby-base-font-size');
  pushRubyAttr('rubyFontFamily', '--ruby-font-family');
  pushRubyAttr('rubyFontSize', '--ruby-font-size');
  pushRubyAttr('rubyRtFontSize', '--ruby-rt-font-size');
  pushRubyAttr('rubyColor', '--ruby-color');
  pushRubyAttr('rubyFontWeight', '--ruby-font-weight');
  pushRubyAttr('rubyFontStyle', '--ruby-font-style');
  pushRubyAttr('rubyRtScale', '--ruby-rt-scale');
  pushRubyAttr('rubyTextDecoration', '--ruby-text-decoration');
  pushRubyAttr('rubyBackgroundColor', '--ruby-background-color');
  pushRubyAttr('rubyFontAssetId', undefined, 'data-platform-font-id');
  pushRubyAttr('rubyPlatformFontFamily', undefined, 'data-platform-font-family');
  pushRubyAttr('rubyBaseFontAssetId', undefined, 'data-ruby-base-font-asset-id');
  pushRubyAttr('rubyRtFontAssetId', undefined, 'data-ruby-rt-font-asset-id');
  pushRubyAttr('rubyBasePlatformFontFamily', undefined, 'data-ruby-base-platform-font-family');
  pushRubyAttr('rubyRtPlatformFontFamily', undefined, 'data-ruby-rt-platform-font-family');
  pushRubyAttr('rubySpoiler');
  const styleAttr = variables.length ? ` style="${variables.join('; ')}"` : '';
  return `<ruby class="tiptap-ruby" ${dataAttrs.join(' ')}${styleAttr}>${text}<rt>${rubyText}</rt></ruby>`;
}

function applyPerformanceMark(text: string, mark: { type: string; attrs?: Record<string, any> }): string {
  const attrs = mark.attrs || {};
  const normalizedEffect = normalizePerformanceEffect(attrs.effect);
  const effect = normalizedEffect ? escapeHtml(normalizedEffect) : '';
  const enterMode = escapeHtml(String(attrs.enterMode || '').trim());
  const enterSpeed = Number(attrs.enterSpeed);
  const toneIntensity = Number(attrs.toneIntensity);
  const scale = escapeHtml(String(attrs.scale || '').trim());
  const dataAttrs: string[] = [];
  const classNames = ['tiptap-performance'];
  const styleVars: string[] = [];

  if (effect) {
    dataAttrs.push(`data-performance-effect="${effect}"`);
    classNames.push(`fx-${effect}`);
  }
  if (enterMode) {
    dataAttrs.push(`data-performance-enter-mode="${enterMode}"`);
  }
  if (Number.isFinite(enterSpeed)) {
    dataAttrs.push(`data-performance-enter-speed="${escapeHtml(String(enterSpeed))}"`);
    styleVars.push(`--performance-enter-speed: ${escapeHtml(String(enterSpeed))}`);
  }
  if (Number.isFinite(toneIntensity)) {
    dataAttrs.push(`data-performance-tone-intensity="${escapeHtml(String(toneIntensity))}"`);
    styleVars.push(`--performance-tone-intensity: ${escapeHtml(String(toneIntensity))}`);
  }
  if (scale) {
    dataAttrs.push(`data-performance-scale="${scale}"`);
    classNames.push(`scale-${scale}`);
  }

  return `<span class="${classNames.join(' ')}"${dataAttrs.length ? ` ${dataAttrs.join(' ')}` : ''}${styleVars.length ? ` style="${styleVars.join('; ')}"` : ''}>${text}</span>`;
}

/**
 * 渲染单个节点
 */
function renderNode(node: TipTapNode, options: RenderOptions = {}): string {
  const { baseUrl = urlBase, imageClass = 'inline-image', linkClass = 'text-blue-500' } = options;

  if (!node) return '';

  // 处理文本节点
  if (node.text !== undefined) {
    let text = options.textRenderer
      ? options.textRenderer(node.text)
      : renderMentionAwareText(node.text);

    // 应用文本标记
    if (node.marks && node.marks.length > 0) {
      text = applyCombinedTextStyle(text, node.marks);
      node.marks.forEach((mark) => {
        switch (mark.type) {
          case 'performance':
            text = applyPerformanceMark(text, mark);
            break;
          case 'ruby':
            text = applyRubyMark(text, mark);
            break;
          case 'bold':
            text = `<strong>${text}</strong>`;
            break;
          case 'italic':
            text = `<em>${text}</em>`;
            break;
          case 'underline':
            text = `<u>${text}</u>`;
            break;
          case 'strike':
            text = `<s>${text}</s>`;
            break;
          case 'code':
            text = `<code>${text}</code>`;
            break;
          case 'highlight':
            break;
          case 'spoiler':
            text = `<span class="tiptap-spoiler" data-spoiler="true">${text}</span>`;
            break;
          case 'link':
            const href = mark.attrs?.href || '#';
            const target = mark.attrs?.target || '_blank';
            // 检查是否为本站消息链接，添加特殊标记供后续处理
            if (isLocalChatLink(href)) {
              const params = parseChatLink(href);
              if (params) {
                const messageIdAttr = params.messageId ? ` data-message-id="${escapeHtml(params.messageId)}"` : '';
                text = `<a href="${escapeHtml(href)}" class="message-jump-link-pending" data-world-id="${escapeHtml(params.worldId)}" data-channel-id="${escapeHtml(params.channelId)}"${messageIdAttr}>${text}</a>`;
                break;
              }
            }
            text = `<a href="${escapeHtml(href)}" class="${linkClass}" target="${target}" rel="noopener noreferrer">${text}</a>`;
            break;
          case 'textStyle':
            break;
        }
      });
    }

    return text;
  }

  // 渲染子节点
  const childrenHtml = node.content
    ? mergeAdjacentSpoilerFragments(node.content.map((child) => renderNode(child, options)))
    : '';

  // 渲染块级节点
  switch (node.type) {
    case SMART_LINK_NODE_TYPE: {
      const attrs = normalizeSmartLinkAttrs(node.attrs);
      if (!attrs) {
        return '';
      }
      const smartLinkClass = `${linkClass} message-smart-link`;
      const textHtml = attrs.textType === 'image'
        ? `<img src="${escapeHtml(resolveRenderableSmartLinkValue(attrs.textValue, options, baseUrl))}" alt="链接图片" class="${imageClass} message-smart-link__image" ${SMART_LINK_IMAGE_ROLE_ATTR}="${SMART_LINK_TEXT_IMAGE_ROLE}" />`
        : escapeHtml(attrs.textValue);
      const dataset = `${SMART_LINK_DATA_ATTR}="true" data-text-type="${escapeHtml(attrs.textType)}" data-text-value="${escapeHtml(attrs.textValue)}" data-url-type="${escapeHtml(attrs.urlType)}" data-url-value="${escapeHtml(attrs.urlValue)}" data-target="${escapeHtml(attrs.target)}"`;
      if (attrs.urlType === 'url') {
        return `<a href="${escapeHtml(attrs.urlValue)}" class="${smartLinkClass}" target="${escapeHtml(attrs.target)}" rel="noopener noreferrer" ${dataset}>${textHtml}</a>`;
      }
      return `<span class="${smartLinkClass}" role="button" tabindex="0" ${dataset}>${textHtml}</span>`;
    }
    case 'doc':
      return childrenHtml;

    case 'performanceCommand': {
      const command = escapeHtml(String(node.attrs?.command || ''));
      const value = node.attrs?.value == null ? '' : escapeHtml(String(node.attrs.value));
      return `<span data-performance-command="${command}" data-performance-value="${value}" class="tiptap-performance-command"></span>`;
    }

    case 'paragraph':
      const textAlign = node.attrs?.textAlign;
      const style = textAlign ? ` style="text-align: ${escapeHtml(textAlign)}"` : '';
      return `<p${style}>${childrenHtml || '<br />'}</p>`;

    case 'heading':
      const level = node.attrs?.level || 1;
      const headingAlign = node.attrs?.textAlign;
      const headingStyle = headingAlign ? ` style="text-align: ${escapeHtml(headingAlign)}"` : '';
      return `<h${level}${headingStyle}>${childrenHtml}</h${level}>`;


    case 'bulletList':
      return `<ul>${childrenHtml}</ul>`;

    case 'orderedList':
      return `<ol>${childrenHtml}</ol>`;

    case 'listItem':
      return `<li>${childrenHtml}</li>`;

    case 'blockquote':
      return `<blockquote>${childrenHtml}</blockquote>`;

    case 'codeBlock':
      const language = node.attrs?.language || '';
      return `<pre><code${language ? ` class="language-${escapeHtml(language)}"` : ''}>${childrenHtml}</code></pre>`;

    case 'hardBreak':
      return '<br />';

    case 'horizontalRule':
      return '<hr />';

    case 'image':
      let src = node.attrs?.src || '';
      const resolver = options.attachmentResolver;
      if (resolver) {
        const resolved = resolver(src);
        if (resolved) {
          src = resolved;
        } else {
          src = buildFallbackAttachmentUrl(src, baseUrl);
        }
      } else {
        src = buildFallbackAttachmentUrl(src, baseUrl);
      }

      const alt = node.attrs?.alt || '';
      const title = node.attrs?.title || '';

      return `<img src="${escapeHtml(src)}" alt="${escapeHtml(alt)}" ${title ? `title="${escapeHtml(title)}"` : ''} class="${imageClass}" />`;

    case 'mention':
    case 'satoriMention':
      const mentionId = String(node.attrs?.id || '').trim();
      const mentionName = String(node.attrs?.name || '').trim();
      const mentionDisplay = mentionName || mentionId || '用户';
      const mentionClassName = mentionId === 'all' ? 'mention-capsule mention-capsule--all' : 'mention-capsule';
      return `<span class="${mentionClassName}">@${escapeHtml(mentionDisplay)}</span>`;

    default:
      // 未知节点类型，返回子内容
      return childrenHtml;
  }
}

/**
 * 将 TipTap JSON 转换为 HTML
 */
export function tiptapJsonToHtml(json: TipTapNode | string, options: RenderOptions = {}): string {
  try {
    const parsedJson = typeof json === 'string' ? JSON.parse(json) : json;
    let html = renderNode(parsedJson, options);

    // 移除尾部的空段落（TipTap 默认会在文档末尾添加空段落）
    html = stripTrailingEmptyParagraphs(html);

    return html;
  } catch (error) {
    console.error('TipTap JSON 渲染失败:', error);
    return '';
  }
}

/**
 * 移除 HTML 尾部的空段落
 */
function stripTrailingEmptyParagraphs(html: string): string {
  // 匹配尾部的空段落: <p><br /></p> 或 <p></p> 或带样式的空段落
  const emptyParagraphPattern = /<p(?:\s[^>]*)?>(?:<br\s*\/?>)?<\/p>$/i;

  let result = html;
  // 循环移除，因为可能有多个连续的空段落
  while (emptyParagraphPattern.test(result)) {
    result = result.replace(emptyParagraphPattern, '');
  }

  return result;
}

function buildFallbackAttachmentUrl(value: string, baseUrl: string): string {
  if (!value) {
    return value;
  }
  if (/^(https?:|blob:|data:|\/\/)/i.test(value)) {
    return value;
  }
  if (value.startsWith('id:')) {
    const attachmentId = value.slice(3);
    return `${baseUrl}/api/v1/attachment/${attachmentId}`;
  }
  if (/^[0-9A-Za-z_-]+$/.test(value)) {
    return `${baseUrl}/api/v1/attachment/${value}`;
  }
  return value;
}

/**
 * 检测内容是否为 TipTap JSON 格式
 */
export function isTipTapJson(content: string): boolean {
  if (!content || typeof content !== 'string') {
    return false;
  }

  try {
    const parsed = JSON.parse(content);
    return parsed && typeof parsed === 'object' && parsed.type === 'doc';
  } catch {
    return false;
  }
}

/**
 * 将 HTML 转换为纯文本（用于搜索、摘要等）
 */
export function tiptapJsonToPlainText(json: TipTapNode | string): string {
  try {
    const parsedJson = typeof json === 'string' ? JSON.parse(json) : json;
    return extractText(parsedJson).replace(/\n+$/, '');
  } catch {
    return '';
  }
}

function extractText(node: TipTapNode): string {
  if (!node) return '';

  if (node.text !== undefined) {
    const text = mentionAwarePlainText(node.text);
    const rubyMark = node.marks?.find((mark) => mark?.type === 'ruby');
    const rubyText = String(rubyMark?.attrs?.rubyText || '').trim();
    return rubyText ? `${text}（${rubyText}）` : text;
  }

  if (node.type === 'hardBreak') {
    return '\n';
  }

  if (node.type === 'mention' || node.type === 'satoriMention') {
    const mentionId = String(node.attrs?.id || '').trim();
    const mentionName = String(node.attrs?.name || '').trim();
    return `@${mentionName || mentionId || '用户'}`;
  }

  if (node.type === SMART_LINK_NODE_TYPE) {
    return smartLinkToPlainText(node.attrs);
  }

  if (node.content && node.content.length > 0) {
    const childTexts = node.content.map((child) => extractText(child));
    const joined = childTexts.join('');
    // 段落、标题等块级元素后添加换行
    if (node.type === 'paragraph' || node.type === 'heading' || node.type === 'listItem') {
      return joined + '\n';
    }
    return joined;
  }

  return '';
}

/**
 * 将纯文本转换为 TipTap JSON 格式
 */
export function plainTextToTiptapJson(text: string): TipTapNode {
  if (!text || !text.trim()) {
    return {
      type: 'doc',
      content: [{ type: 'paragraph' }],
    };
  }

  const lines = text.replace(/\r\n/g, '\n').split('\n');
  const paragraphContent: TipTapNode[] = [];

  lines.forEach((line, index) => {
    if (line) {
      paragraphContent.push({ type: 'text', text: line });
    }
    if (index < lines.length - 1) {
      paragraphContent.push({ type: 'hardBreak' });
    }
  });

  return {
    type: 'doc',
    content: [
      paragraphContent.length
        ? { type: 'paragraph', content: paragraphContent }
        : { type: 'paragraph' },
    ],
  };
}

/**
 * HTML 转义
 */
function escapeHtml(text: string): string {
  const map: Record<string, string> = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;',
  };
  return text.replace(/[&<>"']/g, (char) => map[char] || char);
}

function escapeHtmlPreservingBoundarySpaces(text: string): string {
  const escaped = escapeHtml(text);
  return escaped
    .replace(/^ +/, (spaces) => '&nbsp;'.repeat(spaces.length))
    .replace(/ +$/, (spaces) => '&nbsp;'.repeat(spaces.length));
}

/**
 * 将旧的 HTML 内容转换为 TipTap JSON（简单转换，用于向后兼容）
 */
export function htmlToTiptapJson(html: string): TipTapNode {
  // 简单实现：将 HTML 包装成段落
  // 更复杂的转换可以使用 DOMParser 或其他库
  const lines = html.split(/<br\s*\/?>/gi).filter((line) => line.trim());

  if (lines.length === 0) {
    return {
      type: 'doc',
      content: [{ type: 'paragraph' }],
    };
  }

  const content = lines.map((line) => ({
    type: 'paragraph' as const,
    content: [
      {
        type: 'text' as const,
        text: stripHtml(line),
      },
    ],
  }));

  return {
    type: 'doc',
    content,
  };
}

/**
 * 简单移除 HTML 标签
 */
function stripHtml(html: string): string {
  return html.replace(/<[^>]*>/g, '');
}
