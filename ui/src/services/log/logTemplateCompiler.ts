// 受限模板编译器：语法见 01-contract.md §7 与 ai-style-authoring.md。
// 客户端校验用于即时反馈与安全预览，权威校验由服务端 ValidateLogStyle 执行。
import DOMPurify from 'dompurify';
import { markLogImages } from './logImageLayout';

export const LOG_RENDERER_VERSION = '1.2.0';

const ALLOWED_TAGS = new Set([
  'div', 'span', 'p', 'section', 'article', 'header', 'footer', 'main', 'aside', 'figure', 'figcaption', 'img',
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'blockquote', 'strong', 'em', 'b', 'i', 'br', 'hr',
  'ul', 'ol', 'li', 'table', 'thead', 'tbody', 'tr', 'td', 'th',
]);
const PLAIN_ATTRS = new Set(['class', 'title', 'alt', 'role']);
export const LOG_TEMPLATE_FIELDS = [
  'speaker.name', 'speaker.color', 'speaker.avatarUrl', 'body', 'plainText', 'createdAt',
  'title', 'subtitle', 'caption', 'label', 'text', 'pageNumber', 'section.title',
  'assetUrl', 'content.assetUrl', 'content.title', 'content.subtitle', 'content.caption', 'content.label', 'content.text',
  // 1.1.0：悄悄话收件人（「、」连接的显示名）与引用摘要；非悄悄话/无引用时为空，配合 data-log-if 使用。
  'whisper.targets', 'quote.speaker', 'quote.text',
  // 1.2.0：整条消息为线索嵌入时的线索标题、类型（text/image/iframe）、类型名与网页来源；多条线索按正文顺序以「、」连接。
  'clue.title', 'clue.kind', 'clue.kindLabel', 'clue.source',
] as const;
const BASE_FIELDS = new Set<string>(LOG_TEMPLATE_FIELDS);
const SRC_FIELDS = new Set(['speaker.avatarUrl', 'assetUrl', 'content.assetUrl']);
const KEY_PATTERN = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/;

export interface TemplateIssue {
  templateId: string | null;
  message: string;
}

export interface TemplateData {
  /** 文本字段，例如 `speaker.name`、`content.title`、`props.label`。 */
  fields: Record<string, string>;
  /** 图片字段 → 资源令牌（由外层按权限解析为字节后传入 iframe）。 */
  assets: Record<string, string>;
  /** 已清洗的正文 HTML，仅插入 data-log-slot="body"。 */
  bodyHtml?: string;
  /** 可信调用方提供的 LOG 图片布局；必须在正文清洗后才写入内部标记。 */
  bodyImageLayout?: { messageId: string; widths?: Record<string, number> };
}

interface ParsedTemplate {
  fragment: DocumentFragment;
  parts: string[];
  bodySlots: number;
  /** data-log-text / data-log-if 引用的字段。 */
  fields: Set<string>;
}

export interface TemplateLibrary {
  templates: Map<string, ParsedTemplate>;
  issues: TemplateIssue[];
}

const propsFieldsFor = (settingsSchema: Record<string, unknown>, templateId: string): Set<string> => {
  const out = new Set<string>();
  const schema = settingsSchema?.[templateId] as Record<string, any> | undefined;
  const props = schema?.properties?.props?.properties;
  if (props && typeof props === 'object') {
    for (const name of Object.keys(props)) {
      if (KEY_PATTERN.test(name)) out.add(`props.${name}`);
    }
  }
  return out;
};

const parseTemplate = (templateId: string, src: string, customFields: Set<string>, issues: TemplateIssue[]): ParsedTemplate | null => {
  if (!KEY_PATTERN.test(templateId)) {
    issues.push({ templateId, message: '模板 ID 须为字母开头、最多 64 位的字母数字/下划线/连字符' });
    return null;
  }
  if (src.length > 512 * 1024) {
    issues.push({ templateId, message: '模板超过 512KB' });
    return null;
  }
  const holder = document.createElement('template');
  holder.innerHTML = src;
  const parts: string[] = [];
  const fields = new Set<string>();
  let bodySlots = 0;
  let count = 0;
  let ok = true;
  const fail = (message: string) => {
    if (ok) issues.push({ templateId, message });
    ok = false;
  };
  const walk = (node: Node, depth: number) => {
    count += 1;
    if (depth > 32 || count > 4096) {
      fail('模板嵌套过深或节点过多');
      return;
    }
    if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node as Element;
      const tag = el.localName;
      if (el.namespaceURI !== 'http://www.w3.org/1999/xhtml' || !ALLOWED_TAGS.has(tag)) {
        fail(`不允许的标签 <${tag}>`);
        return;
      }
      for (const attr of Array.from(el.attributes)) {
        const name = attr.name;
        const value = attr.value;
        if (value.length > 512) fail(`属性 ${name} 过长`);
        if (PLAIN_ATTRS.has(name)) {
          if (/[<>\\]/.test(value)) fail(`属性 ${name} 含有非法字符`);
          continue;
        }
        switch (name) {
          case 'data-log-text':
          case 'data-log-if':
            if (!BASE_FIELDS.has(value) && !customFields.has(value)) fail(`未声明字段 ${value}`);
            fields.add(value);
            break;
          case 'data-log-bind-src':
            if (tag !== 'img' || !SRC_FIELDS.has(value)) fail('data-log-bind-src 只能用于 img，且只接受 speaker.avatarUrl / assetUrl / content.assetUrl');
            break;
          case 'data-log-slot':
            if (value !== 'body' || (tag !== 'div' && tag !== 'section')) fail('data-log-slot 只能是 body，且只用于 div/section');
            else bodySlots += 1;
            break;
          case 'data-log-part':
            if (!KEY_PATTERN.test(value)) fail(`片段引用 ${value} 无效`);
            else parts.push(value);
            break;
          default:
            fail(`不允许的属性 ${name}（禁止事件、内联样式、id、src/href 与源映射属性）`);
        }
      }
    } else if (node.nodeType !== Node.TEXT_NODE && node.nodeType !== Node.COMMENT_NODE) {
      fail('不允许的节点类型');
      return;
    }
    node.childNodes.forEach((child) => walk(child, depth + 1));
  };
  holder.content.childNodes.forEach((child) => walk(child, 0));
  if (!ok) return null;
  return { fragment: holder.content, parts, bodySlots, fields };
};

/** 解析并校验整个模板库：标签/属性白名单、片段有向无环与深度。 */
export const buildTemplateLibrary = (
  templates: Record<string, string>,
  settingsSchema: Record<string, unknown> = {},
): TemplateLibrary => {
  const issues: TemplateIssue[] = [];
  const parsed = new Map<string, ParsedTemplate>();
  const entries = Object.entries(templates || {});
  if (entries.length === 0) issues.push({ templateId: null, message: '至少需要一个模板' });
  if (entries.length > 64) issues.push({ templateId: null, message: '模板数量超过 64' });
  for (const [id, src] of entries) {
    const result = parseTemplate(id, String(src ?? ''), propsFieldsFor(settingsSchema, id), issues);
    if (result) parsed.set(id, result);
  }
  const visit = (id: string, stack: Set<string>, depth: number): boolean => {
    const t = parsed.get(id);
    if (!t || stack.has(id) || depth > 8) return false;
    stack.add(id);
    const ok = t.parts.every((part) => visit(part, stack, depth + 1));
    stack.delete(id);
    return ok;
  };
  for (const id of parsed.keys()) {
    if (!visit(id, new Set(), 1)) {
      issues.push({ templateId: id, message: '片段引用不存在、成环或深度超过 8' });
      parsed.delete(id);
    }
  }
  return { templates: parsed, issues };
};

/** 规则引用的模板展开后必须恰好有一个 body 插槽。 */
export const countBodySlots = (lib: TemplateLibrary, id: string, depth = 0): number => {
  const t = lib.templates.get(id);
  if (!t || depth > 8) return 0;
  return t.bodySlots + t.parts.reduce((sum, part) => sum + countBodySlots(lib, part, depth + 1), 0);
};

/** 与消息规则校验共用：展开后恰好含一个 body slot 才能用于消息。 */
export const messageTemplateIds = (lib: TemplateLibrary): string[] =>
  Array.from(lib.templates.keys()).filter((id) => countBodySlots(lib, id) === 1);

const BODY_PURIFY = {
  ALLOWED_TAGS: ['p', 'br', 'span', 'strong', 'em', 'b', 'i', 'u', 's', 'del', 'code', 'pre', 'blockquote', 'ul', 'ol', 'li',
    'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'hr', 'a', 'img', 'ruby', 'rt', 'rp', 'sub', 'sup', 'mark', 'div', 'table', 'thead', 'tbody', 'tr', 'td', 'th'],
  ALLOWED_ATTR: ['class', 'title', 'alt', 'href', 'data-log-asset', 'data-log-missing', 'colspan', 'rowspan', 'width', 'height',
    'style', 'data-platform-font-id', 'data-ruby-base-font-asset-id', 'data-ruby-rt-font-asset-id'],
  ALLOW_DATA_ATTR: false,
  // data-log-asset carries an opaque body:/att:/style:/pub: token. Without this,
  // DOMPurify applies ALLOWED_URI_REGEXP to the explicitly allowed attribute and
  // strips every non-http token before the iframe can bind its Blob URL.
  ADD_URI_SAFE_ATTR: ['data-log-asset', 'data-platform-font-id', 'data-ruby-base-font-asset-id', 'data-ruby-rt-font-asset-id'],
  ALLOWED_URI_REGEXP: /^(?:https?:)/i,
};

const FONT_ID = /^[A-Za-z0-9_-]{1,100}$/;
const BODY_CLASS = /^(?:log-(?:mention|audio|img-missing|link|embed(?:--[a-z-]+)?|clue(?:__[a-z-]+|--[a-z-]+)*)|mention(?:-capsule)?(?:--all)?|tiptap-ruby|tiptap-text-decoration(?:--(?:underline|strike|solid|dotted|dense-dotted|wave-none|wave-soft|wave-heavy|single|double|triple))?|language-[a-zA-Z0-9_-]+)$/;

/** Values are checked before CSSOM parsing: no escapes, comments, URLs or arbitrary functions. */
const bodyCssValue = (property: string, value: string): boolean => {
  if (!value || value.length > 512 || /[\\;{}<>!\u0000-\u001f]|\/\*|\*\//.test(value)) return false;
  const name = property.replace(/^--ruby-(?:base-|rt-)?/, '');
  if (name === 'font-family') return /^[\p{L}\p{N}\s_,"'-]+$/u.test(value) && !/^(?:inherit|initial|unset|revert(?:-layer)?)$/i.test(value);
  if (name === 'font-size' || property === '--ruby-rt-scale') {
    const size = /^(\d+(?:\.\d+)?)(px|pt|em|rem|%)$/.exec(value);
    return !!size && Number(size[1]) > 0 && Number(size[1]) <= (size[2] === '%' ? 1000 : /^(em|rem)$/.test(size[2]) ? 20 : 200);
  }
  if (name === 'color' || name === 'background-color' || property === 'text-decoration-color') {
    return /^(?:#[0-9a-f]{3,8}|[a-z]+|(?:rgb|rgba|hsl|hsla)\([\d\s.,%+/-]+\))$/i.test(value)
      && !/^(?:inherit|initial|unset|revert(?:-layer)?)$/i.test(value)
      && CSS.supports('color', value);
  }
  if (name === 'font-weight') return /^(?:normal|bold|[1-9]00)$/.test(value);
  if (name === 'font-style') return /^(?:normal|italic|oblique)$/.test(value);
  if (name === 'text-decoration' || property === 'text-decoration-line') return /^(?:none|underline|line-through|underline line-through)$/.test(value);
  if (property === 'text-align') return /^(?:left|center|right|justify|start|end)$/.test(value);
  if (property === 'text-decoration-style') return /^(?:solid|double|dotted|dashed|wavy)$/.test(value);
  if (property === 'text-decoration-thickness' || property === '--tiptap-decoration-thickness') return /^(?:1px|0\.12em|0\.18em)$/.test(value);
  return false;
};

const BODY_CSS = new Set(['font-family', 'font-size', 'color', 'background-color', 'font-weight', 'font-style',
  'text-align', 'text-decoration', 'text-decoration-line', 'text-decoration-style', 'text-decoration-color', 'text-decoration-thickness',
  '--tiptap-decoration-thickness', '--ruby-base-font-family', '--ruby-rt-font-family', '--ruby-font-family',
  '--ruby-base-font-size', '--ruby-rt-font-size', '--ruby-font-size', '--ruby-color', '--ruby-font-weight',
  '--ruby-font-style', '--ruby-rt-scale', '--ruby-text-decoration', '--ruby-background-color']);

/** 消息正文专用；模板依然禁止 style。旧消息的属性已由 legacy 解析器丢弃。 */
const sanitizeBodyStyle = (el: Element) => {
  const styles = new Map<string, string>();
  for (const declaration of (el.getAttribute('style') || '').split(';')) {
    const match = /^\s*([a-z-]+)\s*:\s*(.*?)\s*$/.exec(declaration);
    if (match && BODY_CSS.has(match[1]) && bodyCssValue(match[1], match[2])) styles.set(match[1], match[2]);
  }
  for (const [attr, property] of [
    ['data-platform-font-id', el.localName === 'ruby' ? '--ruby-font-family' : 'font-family'],
    ['data-ruby-base-font-asset-id', '--ruby-base-font-family'],
    ['data-ruby-rt-font-asset-id', '--ruby-rt-font-family'],
  ]) {
    const id = el.getAttribute(attr);
    if (!id) continue;
    if (FONT_ID.test(id)) styles.set(property, `"sc-log-platform-${id}", var(--log-font-family)`);
    else el.removeAttribute(attr);
  }
  el.removeAttribute('style');
  // Only these internally constructed fallbacks contain var(). Source CSS cannot supply it.
  if (styles.size) el.setAttribute('style', Array.from(styles, ([key, value]) => `${key}: ${value}`).join('; '));
  const classes = (el.getAttribute('class') || '').split(/\s+/).filter(value => BODY_CLASS.test(value));
  el.removeAttribute('class');
  if (classes.length) el.setAttribute('class', classes.join(' '));
};

/** 正文清洗：有限静态文字样式；图片仅能通过 data-log-asset 令牌引用。 */
export const sanitizeBodyHtml = (html: string): string => {
  const clean = DOMPurify.sanitize(html, { ...BODY_PURIFY, RETURN_DOM_FRAGMENT: true }) as DocumentFragment;
  clean.querySelectorAll('*').forEach(sanitizeBodyStyle);
  clean.querySelectorAll('img').forEach((img) => {
    if (!img.getAttribute('data-log-asset')) {
      const holder = document.createElement('span');
      holder.className = 'log-img-missing';
      holder.textContent = img.getAttribute('alt') || '图片不可用';
      img.replaceWith(holder);
    }
    img.removeAttribute('src');
  });
  clean.querySelectorAll('a').forEach((a) => {
    // iframe 内不导航；保留文本与可见地址。
    a.removeAttribute('href');
    a.classList.add('log-link');
  });
  const box = document.createElement('div');
  box.appendChild(clean);
  return box.innerHTML;
};

/** 模板（含展开的片段）是否引用了某个字段；旧模板不认识新字段时由渲染器补默认呈现。 */
export const templateUsesField = (lib: TemplateLibrary, id: string, field: string, depth = 0): boolean => {
  const t = lib.templates.get(id);
  if (!t || depth > 8) return false;
  return t.fields.has(field) || t.parts.some((part) => templateUsesField(lib, part, field, depth + 1));
};

const hasValue = (data: TemplateData, field: string) =>
  SRC_FIELDS.has(field) ? !!data.assets[field] : !!(data.fields[field] ?? '').trim();

/** 将模板实例化为 DOM 片段；不执行表达式，只按字段名填充。 */
export const instantiateTemplate = (lib: TemplateLibrary, id: string, data: TemplateData, depth = 0): DocumentFragment | null => {
  const t = lib.templates.get(id);
  if (!t || depth > 8) return null;
  const frag = t.fragment.cloneNode(true) as DocumentFragment;
  // 先展开片段（插入到引用元素内部），再处理条件与绑定。
  frag.querySelectorAll('[data-log-part]').forEach((el) => {
    const part = instantiateTemplate(lib, el.getAttribute('data-log-part') || '', data, depth + 1);
    el.removeAttribute('data-log-part');
    if (part) el.appendChild(part);
  });
  frag.querySelectorAll('[data-log-if]').forEach((el) => {
    if (!hasValue(data, el.getAttribute('data-log-if') || '')) el.remove();
    else el.removeAttribute('data-log-if');
  });
  frag.querySelectorAll('[data-log-text]').forEach((el) => {
    const field = el.getAttribute('data-log-text') || '';
    el.removeAttribute('data-log-text');
    if (field === 'pageNumber') {
      el.textContent = '';
      el.setAttribute('data-log-fill', 'pageNumber');
      return;
    }
    el.textContent = data.fields[field] ?? '';
  });
  frag.querySelectorAll('img[data-log-bind-src]').forEach((el) => {
    const token = data.assets[el.getAttribute('data-log-bind-src') || ''];
    el.removeAttribute('data-log-bind-src');
    if (token) el.setAttribute('data-log-asset', token);
    else el.remove();
  });
  frag.querySelectorAll('[data-log-slot]').forEach((el) => {
    el.removeAttribute('data-log-slot');
    el.classList.add('log-body');
    if (data.bodyHtml) {
      const clean = sanitizeBodyHtml(data.bodyHtml);
      el.innerHTML = data.bodyImageLayout
        ? markLogImages(clean, { kind: 'message', id: data.bodyImageLayout.messageId, widths: data.bodyImageLayout.widths })
        : clean;
    }
  });
  return frag;
};

export const fragmentToHtml = (frag: DocumentFragment) => {
  const box = document.createElement('div');
  box.appendChild(frag);
  return box.innerHTML;
};

// ---- CSS 预检（服务端用真实解析器做权威校验；iframe 另有 CSP 禁止外部请求） ----

const CSS_FUNCTIONS = new Set(['var', 'calc', 'min', 'max', 'clamp', 'rgb', 'rgba', 'hsl', 'hsla', 'counter', 'counters',
  'linear-gradient', 'radial-gradient', 'repeating-linear-gradient', 'repeating-radial-gradient', 'format', 'supports',
  'not', 'is', 'where', 'nth-child', 'nth-of-type', 'url']);
const CSS_AT_RULES = new Set(['media', 'page', 'font-face', 'supports', 'top-left', 'top-center', 'top-right', 'bottom-left', 'bottom-center', 'bottom-right']);

export const checkStyleCss = (css: string, assetPaths: Set<string>): TemplateIssue[] => {
  const issues: TemplateIssue[] = [];
  const add = (message: string) => issues.push({ templateId: null, message: `CSS：${message}` });
  if (css.length > 512 * 1024) add('超过 512KB');
  if (/[\\<>\u0000]/.test(css)) add('不允许反斜杠、尖括号（含子选择器 >）或空字符');
  // 去掉注释与字符串，避免字符串中的文字误报；url(...) 单独检查。
  const stripped = css.replace(/\/\*[\s\S]*?\*\//g, ' ');
  const urlPattern = /url\(\s*(['"]?)([^'")]*)\1\s*\)/gi;
  for (const m of stripped.matchAll(urlPattern)) {
    if (m[1]) add(`url(${m[2]}) 不能加引号（服务端只接受未加引号的包内路径）`);
    else if (!assetPaths.has(m[2].trim())) add(`url(${m[2]}) 只能引用包内已声明资源`);
  }
  const noStrings = stripped.replace(/"(?:[^"\n])*"|'(?:[^'\n])*'/g, '""');
  for (const m of noStrings.matchAll(/([a-zA-Z-]+)\(/g)) {
    if (!CSS_FUNCTIONS.has(m[1].toLowerCase())) add(`不允许的函数 ${m[1]}()`);
  }
  for (const m of noStrings.matchAll(/@([a-zA-Z-]+)/g)) {
    if (!CSS_AT_RULES.has(m[1].toLowerCase())) add(`不允许的 @${m[1]}`);
  }
  if (/(^|[;{\s])(behavior|-moz-binding|all)\s*:/i.test(noStrings)) add('不允许的属性 behavior/-moz-binding/all');
  let depth = 0;
  for (const ch of noStrings) {
    if (ch === '{') depth += 1;
    if (ch === '}') depth -= 1;
    if (depth < 0) break;
  }
  if (depth !== 0) add('花括号不配对');
  return issues;
};

const TOKEN_FORBIDDEN = /[;{}\\<>@()]/;

/** 设计变量：值不可含 `;{}\<>@()`，与服务端一致。 */
export const isSafeTokenValue = (value: unknown): value is string | number | boolean => {
  if (typeof value === 'boolean') return true;
  if (typeof value === 'number') return Number.isFinite(value);
  return typeof value === 'string' && value.length <= 200 && !TOKEN_FORBIDDEN.test(value);
};

export const LOG_MESSAGE_TYPES = ['ic', 'ooc', 'narration', 'dice', 'bot', 'image', 'unknown'] as const;

export interface StyleSourceLike {
  templates: Record<string, string>;
  css: string;
  tokens: Record<string, unknown>;
  rules: Array<{ messageType?: string; identityKey?: string; variantKey?: string; templateId: string }>;
  settingsSchema: Record<string, unknown>;
  assets?: Array<{ id: string; path: string }>;
}

/** 样式源码编辑器的整体预检。 */
export const checkStyleSource = (pkg: StyleSourceLike): { library: TemplateLibrary; issues: TemplateIssue[] } => {
  const library = buildTemplateLibrary(pkg.templates, pkg.settingsSchema);
  const issues = [...library.issues];
  for (const [key, value] of Object.entries(pkg.tokens || {})) {
    if (!KEY_PATTERN.test(key) || !isSafeTokenValue(value)) issues.push({ templateId: null, message: `设计变量 ${key} 无效` });
  }
  for (const [key, schema] of Object.entries(pkg.settingsSchema || {})) {
    if (!(key in (pkg.templates || {}))) issues.push({ templateId: key, message: 'settingsSchema 引用了不存在的模板' });
    const s = schema as Record<string, unknown> | null;
    if (!s || s.type !== 'object' || s.additionalProperties !== false) issues.push({ templateId: key, message: 'settingsSchema 必须是 additionalProperties:false 的 object' });
    if (JSON.stringify(schema).includes('"$ref"')) issues.push({ templateId: key, message: '禁止 $ref' });
  }
  for (const rule of pkg.rules || []) {
    if (!library.templates.has(rule.templateId)) {
      issues.push({ templateId: rule.templateId, message: '规则引用的模板不存在或无效' });
      continue;
    }
    if (rule.messageType && !(LOG_MESSAGE_TYPES as readonly string[]).includes(rule.messageType)) {
      issues.push({ templateId: rule.templateId, message: `未知消息类型 ${rule.messageType}` });
    }
    if (countBodySlots(library, rule.templateId) !== 1) {
      issues.push({ templateId: rule.templateId, message: '消息规则模板展开后必须恰好包含一个 body 插槽' });
    }
  }
  issues.push(...checkStyleCss(pkg.css || '', new Set((pkg.assets || []).map((a) => a.path))));
  return { library, issues };
};
