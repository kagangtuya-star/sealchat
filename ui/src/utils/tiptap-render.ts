/**
 * TipTap JSON 渲染工具
 * 将 TipTap JSON 格式转换为 HTML，支持自定义样式
 */

import { urlBase } from '@/stores/_config';

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
}

/**
 * 渲染单个节点
 */
function renderNode(node: TipTapNode, options: RenderOptions = {}): string {
  const { baseUrl = urlBase, imageClass = 'inline-image', linkClass = 'text-blue-500' } = options;

  if (!node) return '';

  // 处理文本节点
  if (node.text !== undefined) {
    let text = escapeHtml(node.text);

    // 应用文本标记
    if (node.marks && node.marks.length > 0) {
      node.marks.forEach((mark) => {
        switch (mark.type) {
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
            const bgColor = mark.attrs?.color || '#fef08a';
            text = `<mark style="background-color: ${escapeHtml(bgColor)}">${text}</mark>`;
            break;
          case 'link':
            const href = mark.attrs?.href || '#';
            const target = mark.attrs?.target || '_blank';
            text = `<a href="${escapeHtml(href)}" class="${linkClass}" target="${target}" rel="noopener noreferrer">${text}</a>`;
            break;
          case 'textStyle':
            if (mark.attrs?.color) {
              text = `<span style="color: ${escapeHtml(mark.attrs.color)}">${text}</span>`;
            }
            break;
        }
      });
    }

    return text;
  }

  // 渲染子节点
  const childrenHtml = node.content ? node.content.map((child) => renderNode(child, options)).join('') : '';

  // 渲染块级节点
  switch (node.type) {
    case 'doc':
      return childrenHtml;

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
  const emptyParagraphPattern = /<p(?:\s[^>]*)?>(?:<br\s*\/?>)?\s*<\/p>\s*$/i;

  let result = html;
  // 循环移除，因为可能有多个连续的空段落
  while (emptyParagraphPattern.test(result)) {
    result = result.replace(emptyParagraphPattern, '');
  }

  return result.trim();
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
    return extractText(parsedJson);
  } catch {
    return '';
  }
}

function extractText(node: TipTapNode): string {
  if (!node) return '';

  if (node.text !== undefined) {
    return node.text;
  }

  if (node.content && node.content.length > 0) {
    return node.content.map(extractText).join('');
  }

  return '';
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
