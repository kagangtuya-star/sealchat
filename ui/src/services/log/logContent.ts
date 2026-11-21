// 原消息内容 → 设计预览正文。复用聊天现有的 TipTap / satori 元素解析，
// 但图片只转换为资源令牌，由外层带权限取字节后交给隔离 iframe。
import DOMPurify from 'dompurify';
import Element from '@satorijs/element';
import { normalizeAttachmentId } from '@/composables/useAttachmentResolver';
import { renderQuickFormatHtmlFromEscaped } from '@/utils/plainQuickFormat';
import { isTipTapJson, tiptapJsonToHtml, tiptapJsonToPlainText } from '@/utils/tiptap-render';
import { contentEscape } from '@/utils/tools';

const ASSET_SCHEME = 'log-asset:';
const BODY_CONTEXT = /^[A-Za-z0-9_-]{1,100}\/[A-Za-z0-9_-]{1,100}\/[a-f0-9]{32}$/;

/** 上下文只包含 LOG 来源片段、消息和不透明资源 key，不包含业务资源类型。 */
export const bodyAssetToken = (context: string): string => BODY_CONTEXT.test(context) ? `body:${context}` : '';

export const attachmentToken = (value: string): string => {
  const id = normalizeAttachmentId(String(value || '').trim());
  // 只接受附件 ID；外链 URL 不进入 iframe。
  return id && /^[A-Za-z0-9_-]{4,128}$/.test(id) ? `att:${id}` : '';
};

const resolveToMarker = (value: string) => {
  const token = attachmentToken(value);
  return token ? `${ASSET_SCHEME}${token}` : '';
};

export interface RenderedBody {
  html: string;
  plainText: string;
  assets: string[];
}

// 必须在 Element 解码 &lt; 之前识别真实标签，避免把用户输入的字面 <br/> 当成换行。
const prepareLegacySource = (content: string): string => {
  let inPre = false;
  // Element 已处理基础/数值 entity，但不处理 nbsp；只替换源文中的这一种，不做二次解码。
  return content.replace(/&nbsp;/g, '\u00a0')
    .split(/(<!--[\s\S]*?-->|<(?:[^"'<>]|"[^"]*"|'[^']*')*>)/g)
    .map((part, index) => {
      if (index % 2 === 0) {
        // Element.parse 会裁掉文本边界的换行和缩进；pre 内先用数值 entity 保护。
        return inPre ? part.replace(/\r/g, '&#13;').replace(/\n/g, '&#10;') : part;
      }
      if (/^<pre(?:\s|>)/i.test(part)) inPre = true;
      if (/^<\/pre\s*>$/i.test(part)) inPre = false;
      const separator = /^<(br|hr)\s*\/?\s*>$/i.exec(part);
      return separator ? `<${separator[1].toLowerCase()}/>` : part;
    }).join('');
};

/** 未知节点只取正文，不复制标签或属性；code/pre 和骰子回退也复用纯文本语义。 */
const legacyNodeText = (item: Element): string => {
  const type = item.type.toLowerCase();
  if (type === 'text') return String(item.attrs?.content || '');
  if (type === 'br') return '\n';
  if (type === 'img') return resolveToMarker(String(item.attrs?.src || '')) ? '' : '图片不可用';
  if (type === 'audio') return '[语音]';
  if (type === 'at') return `@${String(item.attrs?.name || item.attrs?.id || '用户')}`;
  const text = item.children.map(legacyNodeText).join('');
  if (type === 'span') {
    const classes = String(item.attrs?.class || '').split(/\s+/);
    if (classes.includes('dice-chip') || classes.includes('dice-roll-group')) {
      return typeof item.attrs?.dataDiceSource === 'string' && item.attrs.dataDiceSource ? item.attrs.dataDiceSource : text;
    }
  }
  return ['p', 'div', 'blockquote', 'pre', 'li', 'hr'].includes(type) ? `${text}\n` : text;
};

const renderLegacyNode = (item: Element): string => {
  const type = item.type.toLowerCase();
  const children = () => item.children.map(renderLegacyNode).join('');
  switch (type) {
    case 'img': {
      const marker = resolveToMarker(String(item.attrs?.src || ''));
      return marker ? `<img src="${contentEscape(marker)}" alt="图片">` : '<span class="log-img-missing">图片不可用</span>';
    }
    case 'audio':
      return '<span class="log-audio">[语音]</span>';
    case 'at': {
      const name = String(item.attrs?.name || item.attrs?.id || '用户');
      return `<span class="log-mention">@${contentEscape(name)}</span>`;
    }
    case 'text':
      return renderQuickFormatHtmlFromEscaped(contentEscape(legacyNodeText(item)));
    case 'br':
      return '<br>';
    case 'hr':
      return '<hr>';
    case 'strong':
    case 'b':
      return `<strong>${children()}</strong>`;
    case 'em':
    case 'i':
      return `<em>${children()}</em>`;
    case 's':
    case 'strike':
    case 'del':
      return `<s>${children()}</s>`;
    case 'code':
    case 'pre':
      return `<${type}>${contentEscape(item.children.map(legacyNodeText).join(''))}</${type}>`;
    case 'p':
    case 'div':
    case 'blockquote':
    case 'ul':
    case 'ol':
    case 'li':
      return `<${type}>${children()}</${type}>`;
    case 'a': {
      const href = String(item.attrs?.href || '').trim();
      if (/^https?:\/\//i.test(href)) {
        try {
          const url = new URL(href);
          if (url.protocol === 'http:' || url.protocol === 'https:') {
            return `<a href="${contentEscape(url.href)}" target="_blank" rel="noopener noreferrer">${children()}</a>`;
          }
        } catch { /* 无效链接只保留正文。 */ }
      }
      return children();
    }
    case 'span': {
      const classes = String(item.attrs?.class || '').split(/\s+/);
      if (classes.includes('dice-chip') || classes.includes('dice-roll-group')) {
        // Element 将 data-dice-source 规范化为 dataDiceSource；整组优先恢复原表达式。
        const source = typeof item.attrs?.dataDiceSource === 'string' ? item.attrs.dataDiceSource : '';
        return contentEscape(source || legacyNodeText(item));
      }
      break;
    }
  }
  return renderQuickFormatHtmlFromEscaped(contentEscape(legacyNodeText(item)), { disableAllFormatting: true });
};

const normalizeLegacyPlainText = (value: string) => value
  .replace(/\r\n?/g, '\n')
  .replace(/[^\S\n]+/g, ' ')
  .replace(/ *\n */g, '\n')
  .replace(/\n{3,}/g, '\n\n')
  .trim();

const renderLegacy = (content: string) => {
  try {
    const items = Element.parse(prepareLegacySource(content));
    return {
      html: items.map(renderLegacyNode).join(''),
      plainText: normalizeLegacyPlainText(items.map(legacyNodeText).join('')),
    };
  } catch {
    return {
      html: renderQuickFormatHtmlFromEscaped(contentEscape(content)),
      plainText: '',
    };
  }
};

const BODY_FORBID_TAGS = ['style', 'script', 'iframe', 'svg', 'math', 'form', 'input', 'button', 'video', 'audio', 'source'];
const ATTACHMENT_SRC = /^\/api\/v1\/attachment\/[A-Za-z0-9_-]{4,128}$/;

/** 聊天 TipTap、旧格式与服务端 AgentRich 的提及类名统一补上 log-mention，样式只需认一个类。 */
export const normalizeMentionClasses = (root: ParentNode) => {
  root.querySelectorAll('span.mention, span.mention-capsule').forEach((el) => el.classList.add('log-mention'));
};

/**
 * 清洗并收集资源：图片只认资源标记或站内附件路径（转为 att: 令牌），其余替换为占位。
 * src 不经 DOMPurify 的 URI 白名单（log-asset: 会被剥掉）；清洗后所有 src 都会被移除或转换。
 */
const finalizeBody = (html: string, plainText: string): RenderedBody => {
  const assets = new Set<string>();
  const frag = DOMPurify.sanitize(html, { RETURN_DOM_FRAGMENT: true, FORBID_TAGS: BODY_FORBID_TAGS, ADD_URI_SAFE_ATTR: ['src'] }) as DocumentFragment;
  frag.querySelectorAll('img').forEach((img) => {
    const src = img.getAttribute('src') || '';
    img.removeAttribute('src');
    img.removeAttribute('srcset');
    const token = src.startsWith('/__log_body_asset/') ? bodyAssetToken(src.slice('/__log_body_asset/'.length))
      : src.startsWith(ASSET_SCHEME) ? src.slice(ASSET_SCHEME.length) : ATTACHMENT_SRC.test(src) ? attachmentToken(src) : '';
    if (token) {
      img.setAttribute('data-log-asset', token);
      assets.add(token);
    } else {
      const holder = document.createElement('span');
      holder.className = 'log-img-missing';
      holder.textContent = '外链图片未嵌入';
      img.replaceWith(holder);
    }
  });
  normalizeMentionClasses(frag);
  const box = document.createElement('div');
  box.appendChild(frag);
  if (!plainText) plainText = (box.textContent || '').replace(/\s+/g, ' ').trim();
  return { html: box.innerHTML, plainText, assets: Array.from(assets) };
};

/** 将正文渲染为白名单 HTML，并收集需要的资源令牌。 */
export const renderMessageBody = (content: string): RenderedBody => {
  const raw = String(content || '');
  let html = '';
  let plainText = '';
  if (isTipTapJson(raw)) {
    html = tiptapJsonToHtml(raw, { attachmentResolver: resolveToMarker, staticDocument: true });
    try {
      plainText = tiptapJsonToPlainText(raw);
    } catch {
      plainText = '';
    }
  } else {
    const legacy = renderLegacy(raw);
    html = legacy.html;
    plainText = legacy.plainText;
  }
  return finalizeBody(html, plainText);
};

/** 服务端展示投影（便签/线索展开）的正文：与发布版同一渲染，只在这里转换资源令牌。 */
export const renderPresentedBody = (html: string, plainText: string): RenderedBody => finalizeBody(String(html || ''), String(plainText || ''));

/** 编辑器模式：TipTap JSON 使用富文本编辑器，其他内容按原格式在文本框中编辑。 */
export const detectContentFormat = (content: string): 'tiptap' | 'plain' => (isTipTapJson(String(content || '')) ? 'tiptap' : 'plain');
