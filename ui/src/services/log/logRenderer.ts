// 规范化节点 → 安全渲染模型（FrameDocument）。设计预览与公开页共用，模板只读只读 DTO。
import dayjs from 'dayjs';
import type {
  LogDesignMessage,
  LogDisplayRule,
  LogDraft,
  LogElement,
  LogElementType,
  LogPageSettings,
  LogPublicDocument,
  LogPublicNode,
  LogPublicStylePackage,
  LogQuotePresentation,
  LogStylePackage,
  LogStyleRule,
  LogTokenValue,
  LogWhisperTarget,
  LogWorldClue,
} from '@/types/log';
import { normalizeMentionClasses, renderMessageBody, renderPresentedBody, attachmentToken, bodyAssetToken } from './logContent';
import {
  buildTemplateLibrary,
  fragmentToHtml,
  instantiateTemplate,
  isSafeTokenValue,
  messageTemplateIds,
  sanitizeBodyHtml,
  templateUsesField,
  type TemplateData,
  type TemplateLibrary,
} from './logTemplateCompiler';
import type { FrameBlock, FrameDocument } from './pagination/logFrameHost';
import { logPlatformFontId } from './logFontPlan';
import { markLogImages, messageImageKey, normalizeImageWidth } from './logImageLayout';

export interface RenderMessageNode {
  kind: 'message';
  id: string;
  sourceId: string;
  messageType: string;
  speakerKey: string;
  identityKey: string;
  userKey: string;
  variantKey: string;
  templateOverride?: string;
  speakerName: string;
  speakerColor: string | null;
  avatarToken: string | null;
  bodyHtml: string;
  plainText: string;
  createdAt: string;
  movable: boolean;
  assets: string[];
  /** 正文为展开后的站内嵌入时的种类（stickyNote / worldClue）。 */
  embed: string | null;
  /** embed 为 worldClue 时的线索展示语义（按正文顺序）；与 messageType 正交。 */
  clues: LogWorldClue[];
  /** 悄悄话收件人；与 messageType 正交（IC/OOC/骰子都可能是悄悄话）。 */
  whisper: { targets: LogWhisperTarget[] } | null;
  quote: LogQuotePresentation | null;
  imageWidths?: Record<string, number>;
}

export interface RenderElementNode {
  kind: 'element';
  id: string;
  sourceId: string;
  type: LogElementType | string;
  templateId: string;
  fields: Record<string, string>;
  assetToken: string | null;
  bodyHtml: string | null;
  style: Record<string, unknown>;
  fit: 'contain' | 'cover' | null;
  imageWidth?: number;
  level: number | null;
  depth: number | null;
}

export type RenderNode = RenderMessageNode | RenderElementNode;

export interface RenderStyle {
  name: string;
  templates: Record<string, string>;
  css: string;
  rules: LogStyleRule[];
  tokens: Record<string, LogTokenValue>;
  settingsSchema: Record<string, unknown>;
}

const HEX = /^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/;
const SYSTEM_FONTS: Record<string, string> = {
  'sans-serif': '"PingFang SC","Hiragino Sans GB","Microsoft YaHei","Noto Sans CJK SC","Source Han Sans SC",sans-serif',
  serif: '"Songti SC","STSong","SimSun","Noto Serif CJK SC","Source Han Serif SC",serif',
  monospace: '"SFMono-Regular",Consolas,"Liberation Mono",monospace',
};

const formatTime = (value: number | string | null | undefined) => {
  if (value === null || value === undefined || value === '') return '';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm') : '';
};

// ---- 样式包规范化 ----

const PACKAGE_URL = /url\(\s*([^'")\s]+)\s*\)/g;

export const designStyleFromPackage = (pkg: LogStylePackage, themeTokens: Record<string, LogTokenValue>): RenderStyle => {
  const byPath = new Map(pkg.assets.map((a) => [a.path, a.id]));
  const css = String(pkg.css || '').replace(PACKAGE_URL, (whole, path: string) => {
    const id = byPath.get(path);
    return id ? `url(__LOGASSET_style:${id}__)` : 'url(about:blank)';
  });
  return {
    name: pkg.name,
    templates: pkg.templates,
    css,
    rules: pkg.rules,
    tokens: { ...pkg.tokens, ...pickKnownTokens(pkg.tokens, themeTokens) },
    settingsSchema: pkg.settingsSchema,
  };
};

const pickKnownTokens = (base: Record<string, LogTokenValue>, overrides: Record<string, LogTokenValue>) => {
  const out: Record<string, LogTokenValue> = {};
  for (const [k, v] of Object.entries(overrides || {})) {
    if (k in base && typeof base[k] === typeof v && isSafeTokenValue(v)) out[k] = v;
  }
  return out;
};

const PUBLIC_URL = /url\(\s*(['"]?)\/api\/v1\/public\/log\/[a-z0-9]{4,64}\/assets\/([a-f0-9]{32})\1\s*\)/g;

export const publicStyleFromDocument = (style: LogPublicStylePackage | null): RenderStyle | null => {
  if (!style) return null;
  return {
    name: style.name,
    templates: style.templates,
    css: String(style.css || '').replace(PUBLIC_URL, (_w, _q, key: string) => `url("__LOGASSET_pub:${key}__")`),
    rules: style.rules,
    tokens: style.tokens,
    settingsSchema: style.settingsSchema,
  };
};

// ---- 节点规范化 ----

export const resolveDesignMessageType = (m: LogDesignMessage['message']) => {
  if (m.diceVisual) return 'dice';
  if (m.user?.is_bot || m.user?.isBot) return 'bot';
  if (String(m.icMode || '').toLowerCase() === 'ooc') return 'ooc';
  return 'ic';
};

export const resolveDesignSpeakerName = (m: LogDesignMessage['message']) =>
  m.identity?.displayName || m.sender_member_name || m.member?.nick || m.member?.name || m.user?.nick || m.user?.name || m.user?.username || '匿名';

const whisperOf = (value: { targets?: LogWhisperTarget[] } | null | undefined) => {
  const targets = (value?.targets || []).filter((t) => t && typeof t.name === 'string' && t.name.trim());
  return targets.length ? { targets: targets.map((t) => ({ key: String(t.key || t.name), name: t.name.trim() })) } : null;
};

const quoteOf = (value: LogQuotePresentation | null | undefined): LogQuotePresentation | null =>
  value && typeof value.text === 'string' && value.text.trim() ? { speakerName: String(value.speakerName || ''), text: value.text } : null;

const CLUE_KINDS = new Set(['text', 'image', 'iframe']);

/** 只接受服务端线索投影中的公开展示字段；未知类型按文本线索处理。 */
const cluesOf = (embed: string | null, value: unknown): LogWorldClue[] => {
  if (embed !== 'worldClue' || !Array.isArray(value)) return [];
  const out: LogWorldClue[] = [];
  for (const raw of value.slice(0, 64)) {
    if (!raw || typeof raw !== 'object') continue;
    const c = raw as Record<string, unknown>;
    const title = typeof c.title === 'string' ? c.title.trim() : '';
    if (!title) continue;
    const kind = typeof c.kind === 'string' && CLUE_KINDS.has(c.kind) ? c.kind : 'text';
    const kindLabel = typeof c.kindLabel === 'string' && c.kindLabel.trim() ? c.kindLabel.trim() : kind;
    const source = typeof c.source === 'string' && /^https?:\/\//i.test(c.source) ? c.source : '';
    out.push(source ? { title, kind, kindLabel, source } : { title, kind, kindLabel });
  }
  return out;
};

const uniqueJoin = (values: string[], sep: string) => Array.from(new Set(values.filter(Boolean))).join(sep);

/** 线索模板字段：单条线索即其值；多条线索按正文顺序以「、」连接（kind 以空格连接，可用于 CSS ~=）。 */
export const clueTemplateFields = (clues: LogWorldClue[]) => ({
  'clue.title': clues.map((c) => c.title).join('、'),
  'clue.kind': uniqueJoin(clues.map((c) => c.kind), ' '),
  'clue.kindLabel': uniqueJoin(clues.map((c) => c.kindLabel), '、'),
  'clue.source': uniqueJoin(clues.map((c) => c.source || ''), '、'),
});

/** 模板字段 whisper.targets：收件人显示名以「、」连接，与传统导出「对 A、B」一致。 */
export const whisperTargetsText = (whisper: RenderMessageNode['whisper']) => (whisper ? whisper.targets.map((t) => t.name).join('、') : '');

/** 收件人集合不同的相邻悄悄话不能合并，否则被隐藏的头部会吞掉新的收件人。 */
export const whisperMergeKey = (whisper: RenderMessageNode['whisper']) =>
  whisper ? whisper.targets.map((t) => t.key).sort().join('\u0001') : '';

/** 引用属于单条消息，带引用的消息总是另起一组，合并不会隐藏引用。 */
export const canMergeMessageNodes = (prev: RenderMessageNode | null, node: RenderMessageNode) =>
  !!prev && !node.quote && prev.speakerKey === node.speakerKey && prev.variantKey === node.variantKey && prev.messageType === node.messageType &&
  prev.speakerName === node.speakerName && whisperMergeKey(prev.whisper) === whisperMergeKey(node.whisper);

export const designMessageToNode = (item: LogDesignMessage): RenderMessageNode => {
  const m = item.message;
  const presented = item.presentation?.embed;
  // 只有服务端展开的站内嵌入替换正文；普通消息始终走聊天现有 TipTap 渲染，
  // 避免仅因含图片就丢失 spoiler / ruby / performance / smart-link 等格式。
  const body = presented ? renderPresentedBody(presented.bodyHtml, presented.plainText) : renderMessageBody(m.content);
  const projectedAvatar = item.presentation?.avatarToken;
  const avatar = projectedAvatar !== undefined
    ? (projectedAvatar.startsWith('body:') ? bodyAssetToken(projectedAvatar.slice(5)) : '')
    : m.identity?.avatarAttachment ? attachmentToken(m.identity.avatarAttachment) : '';
  const color = m.identity?.color && HEX.test(m.identity.color) ? m.identity.color : null;
  return {
    kind: 'message',
    id: m.id,
    sourceId: m.id,
    messageType: resolveDesignMessageType(m),
    speakerKey: m.identity?.id || m.user?.id || '',
    identityKey: m.identity?.id || '',
    userKey: m.user?.id || '',
    variantKey: m.identity?.variantId || '',
    speakerName: resolveDesignSpeakerName(m),
    speakerColor: color,
    avatarToken: avatar || null,
    bodyHtml: body.html,
    plainText: body.plainText,
    createdAt: formatTime(m.createdAt),
    movable: item.capabilities.reorder,
    assets: avatar ? [...body.assets, avatar] : body.assets,
    embed: presented ? String(presented.kind || 'embed') : null,
    clues: presented ? cluesOf(String(presented.kind || ''), presented.clues) : [],
    whisper: whisperOf(item.presentation?.whisper),
    quote: quoteOf(item.presentation?.quote),
  };
};

const stringField = (value: unknown) => (typeof value === 'string' ? value : typeof value === 'number' ? String(value) : '');

const elementFields = (content: Record<string, unknown>) => {
  const fields: Record<string, string> = {};
  for (const key of ['title', 'subtitle', 'caption', 'label', 'text']) {
    const v = stringField(content[key]);
    fields[key] = v;
    fields[`content.${key}`] = v;
  }
  const props = content.props;
  if (props && typeof props === 'object' && !Array.isArray(props)) {
    for (const [k, v] of Object.entries(props as Record<string, unknown>)) {
      if (typeof v === 'string' || typeof v === 'number') fields[`props.${k}`] = String(v);
      else if (typeof v === 'boolean') fields[`props.${k}`] = v ? '是' : '';
    }
  }
  return fields;
};

const PROJECT_ASSET_ID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

/** 设计态图片令牌：projectAssetId → proj:（LOG 工程素材），assetId → style:（样式包资源）。 */
export const designElementAssetToken = (content: Record<string, unknown>): string | null => {
  const project = typeof content.projectAssetId === 'string' ? content.projectAssetId : '';
  if (project) return PROJECT_ASSET_ID.test(project) ? `proj:${project}` : null;
  const assetId = typeof content.assetId === 'string' && content.assetId ? content.assetId : '';
  return assetId ? `style:${assetId}` : null;
};

/** 设计态页面背景令牌，规则同元素图片。 */
export const designBackgroundToken = (page: LogPageSettings): string | null => {
  const project = page.backgroundProjectAssetId;
  if (project) return PROJECT_ASSET_ID.test(project) ? `proj:${project}` : null;
  return page.backgroundAssetId ? `style:${page.backgroundAssetId}` : null;
};

export const designElementToNode = (e: LogElement): RenderElementNode => {
  const content = e.content || {};
  const body = typeof content.body === 'string' && content.body ? renderMessageBody(content.body) : null;
  return {
    kind: 'element',
    id: `el-${e.id}`,
    sourceId: e.id,
    type: e.type,
    templateId: e.templateId,
    fields: elementFields(content),
    assetToken: designElementAssetToken(content),
    bodyHtml: body ? body.html : null,
    style: e.style || {},
    fit: content.fit === 'cover' ? 'cover' : content.fit === 'contain' ? 'contain' : null,
    imageWidth: normalizeImageWidth(content.imageWidth),
    level: typeof content.level === 'number' ? content.level : null,
    depth: typeof content.depth === 'number' ? content.depth : null,
  };
};

const PUBLIC_IMG = /^\/api\/v1\/public\/log\/[a-z0-9]{4,64}\/assets\/([a-f0-9]{32})$/;

/** 公开正文中的资源路径转换为 pub 令牌；其他图片一律替换为占位。 */
export const publicHtmlToBody = (html: string): { html: string; assets: string[] } => {
  const holder = document.createElement('template');
  holder.innerHTML = String(html || '');
  const assets: string[] = [];
  holder.content.querySelectorAll('img').forEach((img) => {
    const match = PUBLIC_IMG.exec(img.getAttribute('src') || '');
    img.removeAttribute('src');
    if (match) {
      const token = `pub:${match[1]}`;
      img.setAttribute('data-log-asset', token);
      assets.push(token);
    }
  });
  normalizeMentionClasses(holder.content);
  const box = document.createElement('div');
  box.appendChild(holder.content);
  return { html: box.innerHTML, assets };
};

export const publicNodeToRender = (node: LogPublicNode): RenderNode => {
  if (node.kind === 'message') {
    const body = publicHtmlToBody(node.bodyHtml);
    const avatar = node.speaker.avatarAssetKey ? `pub:${node.speaker.avatarAssetKey}` : null;
    return {
      kind: 'message',
      id: node.id,
      sourceId: node.id,
      messageType: node.messageType,
      speakerKey: node.speaker.key,
      identityKey: node.identityKey || '',
      userKey: node.userKey || '',
      variantKey: node.speaker.variantKey || '',
      templateOverride: node.templateOverride,
      speakerName: node.speaker.name,
      speakerColor: node.speaker.color && HEX.test(node.speaker.color) ? node.speaker.color : null,
      avatarToken: avatar,
      bodyHtml: body.html,
      imageWidths: Object.fromEntries(Object.entries(node.imageWidths || {}).filter(([index]) => /^(0|[1-9]\d*)$/.test(index)).map(([index, width]) => [messageImageKey(node.id, Number(index)), width])),
      plainText: node.plainText,
      createdAt: formatTime(node.createdAt),
      movable: false,
      assets: avatar ? [...body.assets, avatar] : body.assets,
      embed: typeof node.embed === 'string' && node.embed ? node.embed : null,
      clues: cluesOf(typeof node.embed === 'string' ? node.embed : null, node.clues),
      whisper: whisperOf(node.whisper),
      quote: quoteOf(node.quote),
    };
  }
  const content = node.content || {};
  const key = typeof content.assetKey === 'string' && /^[a-f0-9]{32}$/.test(content.assetKey) ? content.assetKey : '';
  const body = typeof content.body === 'string' ? publicHtmlToBody(content.body) : null;
  return {
    kind: 'element',
    id: node.id,
    sourceId: node.id,
    type: node.type,
    templateId: node.templateId,
    fields: elementFields(content),
    assetToken: key ? `pub:${key}` : null,
    bodyHtml: body ? body.html : null,
    style: node.style || {},
    fit: content.fit === 'cover' ? 'cover' : content.fit === 'contain' ? 'contain' : null,
    imageWidth: normalizeImageWidth(content.imageWidth),
    level: typeof content.level === 'number' ? content.level : null,
    depth: typeof content.depth === 'number' ? content.depth : null,
  };
};

// ---- 组装 ----

export interface TocEntry { targetId: string; text: string; level: number }

export interface BuildOptions {
  style: RenderStyle | null;
  page: LogPageSettings;
  title: string;
  sectionTitle: string;
  nodes: RenderNode[];
  toc: TocEntry[];
  /** 页面背景资源令牌（style:<id> 或 pub:<key>）。 */
  backgroundToken: string | null;
  pageNumberStart?: number;
  imageWidths?: Record<string, number>;
  templateSelection?: Pick<LogDraft, 'displayRules' | 'messageTemplateOverrides'>;
}

export interface BuildResult {
  doc: FrameDocument;
  assetTokens: string[];
  library: TemplateLibrary | null;
  issues: string[];
}

const matchesDisplayRule = (match: LogDisplayRule['match'], n: RenderMessageNode): boolean =>
  !!(match.messageType || match.identityId || match.userId || match.variantId) &&
  (!match.messageType || match.messageType === n.messageType) &&
  (!match.identityId || match.identityId === n.identityKey) &&
  (!match.userId || match.userId === n.userKey) &&
  (!match.variantId || match.variantId === n.variantKey);

/** 唯一的最终模板选择流程。公开态以 release 哈希匹配同一流程，不需要真实用户 ID。 */
export const resolveMessageTemplate = (
  n: RenderMessageNode,
  templates: ReadonlySet<string>,
  rules: LogStyleRule[],
  selection?: BuildOptions['templateSelection'],
): string | null => {
  const override = n.templateOverride || selection?.messageTemplateOverrides?.[n.sourceId];
  if (override && templates.has(override)) return override;
  for (const rule of selection?.displayRules || []) {
    if (templates.has(rule.templateId) && matchesDisplayRule(rule.match, n)) return rule.templateId;
  }
  for (const rule of rules) {
    if (rule.messageType && rule.messageType !== n.messageType) continue;
    if (rule.identityKey && rule.identityKey !== n.speakerKey) continue;
    if (rule.variantKey && rule.variantKey !== n.variantKey) continue;
    if (templates.has(rule.templateId)) return rule.templateId;
  }
  return null;
};

const escapeHtml = (s: string) => s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c] as string));

const fallbackMessageHtml = (n: RenderMessageNode, widths?: Record<string, number>) =>
  `<article class="log-fallback-message"><header class="log-speaker">${escapeHtml(n.speakerName)}</header><div class="log-body">${markLogImages(sanitizeBodyHtml(n.bodyHtml), { kind: 'message', id: n.id, widths })}</div></article>`;

/** 样式模板未引用新字段时的默认呈现（旧样式包、模板缺失时的兜底），保证语义不丢失。 */
const fallbackMetaHtml = (n: RenderMessageNode, lib: TemplateLibrary | null, templateId: string | null) => {
  const uses = (field: string) => !!(lib && templateId && templateUsesField(lib, templateId, field));
  let html = '';
  if (n.quote && !uses('quote.text')) {
    const who = n.quote.speakerName ? `<span class="log-quote-fallback__who">${escapeHtml(n.quote.speakerName)}</span>` : '';
    html += `<div class="log-quote-fallback">${who}<span class="log-quote-fallback__text">${escapeHtml(n.quote.text)}</span></div>`;
  }
  if (n.whisper && !uses('whisper.targets')) {
    html += `<div class="log-whisper-fallback"><span class="log-whisper-fallback__label">悄悄话</span>对 <span class="log-whisper-fallback__targets">${escapeHtml(whisperTargetsText(n.whisper))}</span></div>`;
  }
  return html;
};

const styleVars = (style: Record<string, unknown>) => {
  const vars: Record<string, string> = {};
  const classes: string[] = [];
  for (const [k, v] of Object.entries(style || {})) {
    if (!/^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/.test(k)) continue;
    if (typeof v === 'boolean') {
      if (v) classes.push(`log-style-${k}`);
    } else if (isSafeTokenValue(v)) {
      vars[`--log-style-${k}`] = String(v);
    }
  }
  return { vars, classes };
};

const tocListElement = (entries: TocEntry[], depth: number) => {
  const list = document.createElement('ol');
  list.className = 'log-toc-list';
  for (const e of entries) {
    if (e.level > depth) continue;
    const li = document.createElement('li');
    li.className = 'log-toc-item';
    li.setAttribute('data-level', String(e.level));
    const text = document.createElement('span');
    text.className = 'log-toc-text';
    text.textContent = e.text;
    const page = document.createElement('span');
    page.className = 'log-toc-page';
    page.setAttribute('data-log-toc-target', e.targetId);
    li.append(text, page);
    list.appendChild(li);
  }
  return list;
};

export const buildFrameDocument = (opts: BuildOptions): BuildResult => {
  const issues: string[] = [];
  const library = opts.style ? buildTemplateLibrary(opts.style.templates, opts.style.settingsSchema) : null;
  if (library) issues.push(...library.issues.map((i) => `${i.templateId ? `[${i.templateId}] ` : ''}${i.message}`));
  const assets = new Set<string>();
  const blocks: FrameBlock[] = [];
  const merge = opts.page.mergeNeighbors;
  let prev: RenderMessageNode | null = null;
  let prevTemplateId: string | null = null;
  const messageTemplates = new Set(library ? messageTemplateIds(library) : []);

  for (const node of opts.nodes) {
    if (node.kind === 'message') {
      node.assets.forEach((a) => assets.add(a));
      const templateId = resolveMessageTemplate(node, messageTemplates, opts.style?.rules || [], opts.templateSelection);
      const merged = merge && prevTemplateId === templateId && canMergeMessageNodes(prev, node);
      let html = '';
      if (templateId && library) {
        const data: TemplateData = {
          fields: {
            'speaker.name': node.speakerName,
            'speaker.color': node.speakerColor || '',
            plainText: node.plainText,
            createdAt: node.createdAt,
            'section.title': opts.sectionTitle,
            title: opts.title,
            'whisper.targets': whisperTargetsText(node.whisper),
            'quote.speaker': node.quote?.speakerName || '',
            'quote.text': node.quote?.text || '',
            ...clueTemplateFields(node.clues),
          },
          assets: node.avatarToken ? { 'speaker.avatarUrl': node.avatarToken } : {},
          bodyHtml: node.bodyHtml,
          bodyImageLayout: { messageId: node.id, widths: opts.imageWidths || node.imageWidths },
        };
        const frag = instantiateTemplate(library, templateId, data);
        html = frag ? fragmentToHtml(frag) : '';
      }
      const fromTemplate = !!html;
      if (!html) html = fallbackMessageHtml(node, opts.imageWidths || node.imageWidths);
      html = fallbackMetaHtml(node, fromTemplate ? library : null, templateId) + html;
      const attrs: Record<string, string> = {
        'data-log-type': node.messageType,
        'data-log-speaker': node.speakerKey,
      };
      if (node.variantKey) attrs['data-log-variant'] = node.variantKey;
      if (node.whisper) attrs['data-log-whisper'] = String(node.whisper.targets.length);
      if (node.quote) attrs['data-log-quote'] = '1';
      if (node.embed) attrs['data-log-embed'] = node.embed;
      if (node.clues.length) attrs['data-log-clue-kind'] = clueTemplateFields(node.clues)['clue.kind'];
      if (merged) attrs['data-log-merged'] = '1';
      if (node.movable) attrs['data-log-movable'] = '1';
      blocks.push({
        id: node.id,
        kind: 'message',
        html,
        classes: merged ? ['log-merged'] : [],
        attrs,
        vars: node.speakerColor ? { '--log-speaker-color': node.speakerColor } : {},
        breakBefore: false,
        breakAfter: false,
      });
      prev = node;
      prevTemplateId = templateId;
      continue;
    }
    prev = null;
    if (node.assetToken) assets.add(node.assetToken);
    const { vars, classes } = styleVars(node.style);
    if (node.fit) classes.push(`log-fit-${node.fit}`);
    let html = '';
    if (library && library.templates.has(node.templateId)) {
      const data: TemplateData = {
        fields: { ...node.fields, 'section.title': opts.sectionTitle },
        assets: node.assetToken ? { assetUrl: node.assetToken, 'content.assetUrl': node.assetToken } : {},
        bodyHtml: node.bodyHtml || undefined,
      };
      const frag = instantiateTemplate(library, node.templateId, data);
      if (frag && node.type === 'toc') {
        const target = frag.querySelector('.log-body') || frag.firstElementChild;
        const list = tocListElement(opts.toc, node.depth || 2);
        if (target) target.appendChild(list); else frag.appendChild(list);
      }
      html = frag ? fragmentToHtml(frag) : '';
    }
    if (!html && node.type !== 'pageBreak') {
      html = `<div class="log-element-placeholder">${escapeHtml(String(node.type))}：模板 ${escapeHtml(node.templateId)} 不可用</div>`;
    }
    const attrs: Record<string, string> = { 'data-log-element-type': String(node.type) };
    if (node.type === 'cover' || node.type === 'imagePlate') {
      html = markLogImages(html, { kind: 'element', id: node.id, assetToken: node.assetToken, width: node.imageWidth });
    }
    if (node.level) attrs['data-log-level'] = String(node.level);
    blocks.push({
      id: node.id,
      kind: 'element',
      html,
      classes,
      attrs,
      vars,
      breakBefore: false,
      breakAfter: node.type === 'pageBreak',
    });
  }

  const safePlatformFontId = logPlatformFontId(opts.page.fontFamily);
  const platformFontFamily = safePlatformFontId ? `sc-log-platform-${safePlatformFontId}` : '';
  const rootVars: Record<string, string> = {
    '--log-page-bg': HEX.test(opts.page.backgroundColor) ? opts.page.backgroundColor : '#ffffff',
    '--log-font-family': platformFontFamily
      ? `"${platformFontFamily}",${SYSTEM_FONTS['sans-serif']}`
      : SYSTEM_FONTS[opts.page.fontFamily] || `"${opts.page.fontFamily.replace(/["\\]/g, '')}",${SYSTEM_FONTS['sans-serif']}`,
    '--log-font-size': `${opts.page.fontSizePt}pt`,
    '--log-line-height': String(opts.page.lineHeight),
  };
  const rootAttrs: Record<string, string> = {};
  for (const [k, v] of Object.entries(opts.style?.tokens || {})) {
    if (!/^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/.test(k) || !isSafeTokenValue(v)) continue;
    if (typeof v === 'boolean') rootAttrs[`data-log-${k.toLowerCase()}`] = v ? 'true' : 'false';
    else rootVars[`--log-${k}`] = String(v);
  }

  const pageTemplate = (id: string | null): string | null => {
    if (!id || !library || !library.templates.has(id)) return null;
    const frag = instantiateTemplate(library, id, {
      fields: { title: opts.title, 'section.title': opts.sectionTitle },
      assets: {},
    });
    return frag ? fragmentToHtml(frag) : null;
  };
  let footerHtml = pageTemplate(opts.page.footerTemplateId);
  if (!footerHtml && opts.page.showPageNumbers) footerHtml = '<span class="log-page-number" data-log-fill="pageNumber"></span>';
  if (opts.backgroundToken) assets.add(opts.backgroundToken);

  return {
    doc: {
      page: opts.page,
      css: opts.style?.css || '',
      rootVars,
      rootAttrs,
      blocks,
      headerHtml: pageTemplate(opts.page.headerTemplateId),
      footerHtml,
      backgroundToken: opts.backgroundToken,
      pageNumberStart: opts.pageNumberStart || 1,
    },
    assetTokens: Array.from(assets),
    library,
    issues,
  };
};

// ---- 设计态章节组装：元素锚点解析 ----

export type AnchorState = 'attached' | 'orphaned' | 'unloaded';

export interface SectionComposition {
  nodes: RenderNode[];
  anchorStates: Map<string, AnchorState>;
  toc: TocEntry[];
}

/**
 * 以源消息顺序组装章节：sectionStart → (before, 消息, after)* → sectionEnd。
 * 无法解析的消息锚点不静默移动到章节末尾；完整加载后标记为 orphaned。
 */
export interface DesignNodeCacheEntry {
  key: string;
  presentation: LogDesignMessage['presentation'];
  node: RenderMessageNode;
}

/**
 * 设计态章节的规范顺序：sectionStart → (before, 消息, after)* → sectionEnd。
 * composeDesignSection 与 designSectionToc 共用这一遍历，保证分片后的目录与整章预览一致。
 */
const walkDesignSection = (
  sectionId: string,
  messageIds: string[],
  elements: LogElement[],
  complete: boolean,
  visit: { element: (e: LogElement) => void; message: (index: number) => void; started: () => void },
) => {
  const own = elements.filter((e) => e.anchor.sectionId === sectionId).slice().sort((a, b) => a.order - b.order);
  const byAnchor = new Map<string, LogElement[]>();
  for (const e of own) {
    const key = e.anchor.kind === 'message' ? `${e.anchor.messageId}:${e.anchor.side}` : e.anchor.kind;
    const list = byAnchor.get(key) || [];
    list.push(e);
    byAnchor.set(key, list);
  }
  const pushElements = (key: string) => {
    for (const e of byAnchor.get(key) || []) visit.element(e);
  };
  pushElements('sectionStart');
  visit.started();
  messageIds.forEach((id, index) => {
    pushElements(`${id}:before`);
    visit.message(index);
    pushElements(`${id}:after`);
  });
  if (complete) pushElements('sectionEnd');
  return own;
};

const headingTocEntry = (e: LogElement, n: RenderElementNode): TocEntry | null =>
  e.type === 'heading' ? { targetId: n.id, text: n.fields.text || '', level: n.level || 1 } : null;

export const composeDesignSection = (
  sectionId: string,
  sectionTitle: string,
  messages: LogDesignMessage[],
  elements: LogElement[],
  complete: boolean,
  nodeCache: Map<string, DesignNodeCacheEntry>,
): SectionComposition => {
  const nodes: RenderNode[] = [];
  const anchorStates = new Map<string, AnchorState>();
  const toc: TocEntry[] = [];
  let sectionMarker = 0;
  const own = walkDesignSection(sectionId, messages.map((m) => m.message.id), elements, complete, {
    element: (e) => {
      const n = designElementToNode(e);
      nodes.push(n);
      anchorStates.set(e.id, 'attached');
      const entry = headingTocEntry(e, n);
      if (entry) toc.push(entry);
    },
    started: () => { sectionMarker = nodes.length; },
    message: (index) => {
      const item = messages[index];
      const cacheKey = `${item.version}:${item.capabilities.reorder ? 1 : 0}`;
      let cached = nodeCache.get(item.message.id);
      // 展示投影（便签/线索内容、引用）可在原消息版本不变时变化；重载后按对象身份失效。
      if (!cached || cached.key !== cacheKey || cached.presentation !== item.presentation) {
        cached = { key: cacheKey, presentation: item.presentation, node: designMessageToNode(item) };
        nodeCache.set(item.message.id, cached);
      }
      nodes.push(cached.node);
    },
  });
  for (const e of own) {
    if (!anchorStates.has(e.id)) anchorStates.set(e.id, complete ? 'orphaned' : 'unloaded');
  }
  const firstId = nodes[sectionMarker]?.id || nodes[0]?.id;
  if (firstId) toc.unshift({ targetId: firstId, text: sectionTitle, level: 1 });
  return { nodes, anchorStates, toc };
};

/**
 * 只计算整章目录而不渲染消息：后台 PDF 把超长章节切成多段时，每段都需要整章目录，
 * 但只持有本段消息。messageIds 为整章的消息顺序。
 */
export const designSectionToc = (
  sectionId: string,
  sectionTitle: string,
  messageIds: string[],
  elements: LogElement[],
): TocEntry[] => {
  const toc: TocEntry[] = [];
  const ids: string[] = [];
  let sectionMarker = 0;
  walkDesignSection(sectionId, messageIds, elements, true, {
    element: (e) => {
      const n = designElementToNode(e);
      ids.push(n.id);
      const entry = headingTocEntry(e, n);
      if (entry) toc.push(entry);
    },
    started: () => { sectionMarker = ids.length; },
    message: (index) => { ids.push(messageIds[index]); },
  });
  const firstId = ids[sectionMarker] || ids[0];
  if (firstId) toc.unshift({ targetId: firstId, text: sectionTitle, level: 1 });
  return toc;
};

export const LOG_ELEMENT_LABELS: Record<LogElementType, string> = {
  cover: '封面',
  imagePlate: '插图页',
  pageBreak: '分页符',
  transition: '转场',
  quote: '引言',
  prologue: '序章',
  epilogue: '尾声',
  heading: '标题',
  toc: '目录',
  custom: '自定义组件',
};

/** 每种元素的默认 content（符合合同最小 Schema）；图片来源由调用方按选择写入。 */
export const defaultElementContent = (type: LogElementType): Record<string, unknown> => {
  switch (type) {
    case 'cover': return { title: '标题', subtitle: '' };
    case 'imagePlate': return { caption: '', fit: 'contain' };
    case 'pageBreak': return {};
    case 'transition': return { label: '※ ※ ※' };
    case 'quote':
    case 'prologue':
    case 'epilogue': return { body: '' };
    case 'heading': return { text: '小节标题', level: 2 };
    case 'toc': return { depth: 2 };
    case 'custom': return { props: {} };
    default: return {};
  }
};

export const publicDocumentStyle = (doc: LogPublicDocument) => publicStyleFromDocument(doc.style);

/** 元素类型默认模板：同名模板优先；正文类元素可回退到 body 模板；custom 必须有参数 Schema。 */
export const templatesForElementType = (type: LogElementType, templates: Record<string, string>, settingsSchema: Record<string, unknown>): string[] => {
  const ids = Object.keys(templates || {});
  if (type === 'custom') return ids.filter((id) => !!settingsSchema?.[id] && (id === 'custom' || id.startsWith('custom')));
  const preferred = type === 'quote' || type === 'prologue' || type === 'epilogue' ? [type, 'body'] : [type];
  const head = preferred.filter((id) => ids.includes(id));
  const rest = ids.filter((id) => !head.includes(id) && id.toLowerCase().startsWith(type.toLowerCase()));
  return [...head, ...rest];
};

export interface SchemaField {
  key: string;
  type: 'string' | 'number' | 'integer' | 'boolean';
  title: string;
  enum: Array<string | number> | null;
  minimum: number | null;
  maximum: number | null;
  maxLength: number | null;
}

/** 从模板 settingsSchema 中取出 style / props 的平面字段（v1 只做一级标量表单）。 */
export const schemaFieldsFor = (settingsSchema: Record<string, unknown>, templateId: string, part: 'style' | 'props'): SchemaField[] => {
  const root = settingsSchema?.[templateId] as Record<string, any> | undefined;
  const props = root?.properties?.[part]?.properties;
  if (!props || typeof props !== 'object') return [];
  const out: SchemaField[] = [];
  for (const [key, raw] of Object.entries(props as Record<string, any>)) {
    const t = raw?.type;
    if (t !== 'string' && t !== 'number' && t !== 'integer' && t !== 'boolean') continue;
    out.push({
      key,
      type: t,
      title: typeof raw.title === 'string' ? raw.title : key,
      enum: Array.isArray(raw.enum) ? raw.enum.filter((v: unknown) => typeof v === 'string' || typeof v === 'number') : null,
      minimum: typeof raw.minimum === 'number' ? raw.minimum : null,
      maximum: typeof raw.maximum === 'number' ? raw.maximum : null,
      maxLength: typeof raw.maxLength === 'number' ? raw.maxLength : null,
    });
  }
  return out;
};
