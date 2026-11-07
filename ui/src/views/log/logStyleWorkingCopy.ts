// Style Studio 的工作副本（纯函数，不含 Vue / DOM）：编辑期间的临时状态，不是新的样式模型。
// 保存时始终组装回普通 LogStylePackage，交给既有的 版本保存 / 新建 / ZIP 导入 接口与服务端 ValidateLogStyle。
// tokens / rules / settingsSchema 以 JSON 文本为唯一来源：友好界面解析文本、修改后重新序列化写回，
// 高级源码直接编辑同一段文本，两种视图因此永远一致。
import type { LogDraft, LogStyleAsset, LogStylePackage, LogStyleRule, LogTokenValue } from '@/types/log';
import { imageUsages } from './logAssetRefs';

export type StyleOrigin = 'version' | 'new' | 'zip';

export interface StyleWorkingCopy {
  /** version：已保存的私有样式（或只读查看的样式）；new：尚未保存的新样式；zip：尚未导入的 ZIP。 */
  origin: StyleOrigin;
  /** 已保存的基准版本；new / zip 为 null。 */
  base: LogStylePackage | null;
  id: string;
  version: string;
  name: string;
  license: string;
  minRendererVersion: string;
  templates: Record<string, string>;
  css: string;
  tokensText: string;
  rulesText: string;
  schemaText: string;
  assets: LogStyleAsset[];
}

export const STYLE_KEY = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/;
export const STYLE_SEMVER = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;

export const formatJson = (value: unknown) => JSON.stringify(value, null, 2);

export const workingCopyFromPackage = (pkg: LogStylePackage, origin: StyleOrigin): StyleWorkingCopy => ({
  origin,
  base: origin === 'version' ? pkg : null,
  id: pkg.id,
  version: pkg.version,
  name: pkg.name,
  license: pkg.license || '',
  minRendererVersion: pkg.minRendererVersion,
  templates: { ...pkg.templates },
  css: pkg.css || '',
  tokensText: formatJson(pkg.tokens || {}),
  rulesText: formatJson(pkg.rules || []),
  schemaText: formatJson(pkg.settingsSchema || {}),
  assets: (pkg.assets || []).map((a) => ({ ...a })),
});

export type StyleJsonField = 'tokens' | 'rules' | 'schema';
export const STYLE_JSON_LABEL: Record<StyleJsonField, string> = { tokens: '设计变量', rules: '消息规则', schema: '参数 Schema' };

export interface ParsedStyleJson {
  tokens: Record<string, LogTokenValue> | null;
  rules: LogStyleRule[] | null;
  schema: Record<string, unknown> | null;
  errors: Partial<Record<StyleJsonField, string>>;
}

const isPlainObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);

/** 解析三段 JSON 文本；语法或结构错误只记录，不抛出。 */
export const parseStyleJson = (copy: Pick<StyleWorkingCopy, 'tokensText' | 'rulesText' | 'schemaText'>): ParsedStyleJson => {
  const errors: ParsedStyleJson['errors'] = {};
  const read = <T>(field: StyleJsonField, text: string, check: (v: unknown) => boolean, shape: string): T | null => {
    try {
      const value = JSON.parse(text);
      if (check(value)) return value as T;
      errors[field] = `需要 ${shape}`;
    } catch (error) {
      errors[field] = `JSON 语法错误：${error instanceof Error ? error.message : String(error)}`;
    }
    return null;
  };
  return {
    tokens: read('tokens', copy.tokensText, isPlainObject, 'JSON 对象'),
    rules: read('rules', copy.rulesText, (v) => Array.isArray(v) && v.every(isPlainObject), '由对象组成的 JSON 数组'),
    schema: read('schema', copy.schemaText, isPlainObject, '以模板 ID 为键的 JSON 对象'),
    errors,
  };
};

/**
 * 组装为普通 LogStylePackage；JSON 无法解析或缺少必填信息时返回 null 与问题列表。
 * 模板 / CSS / 规则语义由 checkStyleSource（前端快速反馈）与服务端 ValidateLogStyle（权威）校验。
 */
export const assembleStylePackage = (copy: StyleWorkingCopy, parsed = parseStyleJson(copy)): { pkg: LogStylePackage | null; issues: string[] } => {
  const issues = (Object.keys(parsed.errors) as StyleJsonField[]).map((f) => `${STYLE_JSON_LABEL[f]}：${parsed.errors[f]}`);
  if (!copy.name.trim()) issues.push('名称不能为空');
  if (copy.name.trim().length > 200) issues.push('名称不能超过 200 字');
  if (!Object.keys(copy.templates).length) issues.push('至少需要一个模板');
  if (!parsed.tokens || !parsed.rules || !parsed.schema || issues.length) return { pkg: null, issues };
  return {
    pkg: {
      schemaVersion: 1,
      id: copy.id,
      version: copy.version,
      name: copy.name.trim(),
      license: copy.license.trim(),
      minRendererVersion: copy.minRendererVersion,
      tokens: parsed.tokens,
      templates: { ...copy.templates },
      css: copy.css,
      rules: parsed.rules,
      settingsSchema: parsed.schema,
      assets: copy.assets.map((a) => ({ ...a })),
    },
    issues,
  };
};

const sortedEntries = (o: Record<string, string>) => Object.keys(o).sort().map((k) => [k, o[k]]);
const normalizedJson = (text: string) => {
  try {
    return JSON.stringify(JSON.parse(text));
  } catch {
    return `raw:${text}`;
  }
};

/** 判断未保存修改用的指纹：模板键序与 JSON 空白不影响结果。 */
export const workingCopyFingerprint = (copy: StyleWorkingCopy) => JSON.stringify([
  copy.id, copy.version, copy.name.trim(), copy.license.trim(), copy.minRendererVersion,
  sortedEntries(copy.templates), copy.css,
  normalizedJson(copy.tokensText), normalizedJson(copy.rulesText), normalizedJson(copy.schemaText),
  copy.assets.map((a) => `${a.id}|${a.path}|${a.mime}`),
]);

// ---- 版本 ----

export type VersionBump = 'patch' | 'minor' | 'major';

export const nextStyleVersion = (version: string, bump: VersionBump = 'patch') => {
  const [a, b, c] = (STYLE_SEMVER.test(version) ? version : '1.0.0').split('.').map((x) => Number.parseInt(x, 10));
  if (bump === 'major') return `${a + 1}.0.0`;
  if (bump === 'minor') return `${a}.${b + 1}.0`;
  return `${a}.${b}.${c + 1}`;
};

export const compareStyleVersions = (x: string, y: string) => {
  const a = x.split('.').map((n) => Number.parseInt(n, 10) || 0);
  const b = y.split('.').map((n) => Number.parseInt(n, 10) || 0);
  for (let i = 0; i < 3; i += 1) if ((a[i] || 0) !== (b[i] || 0)) return (a[i] || 0) > (b[i] || 0) ? 1 : -1;
  return 0;
};

// ---- 设计变量 ----

export type TokenKind = 'color' | 'number' | 'boolean' | 'text';
const HEX_COLOR = /^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/;
export const tokenKind = (v: LogTokenValue): TokenKind =>
  typeof v === 'boolean' ? 'boolean' : typeof v === 'number' ? 'number' : HEX_COLOR.test(v) ? 'color' : 'text';
export const tokenDefault = (kind: TokenKind): LogTokenValue =>
  kind === 'boolean' ? false : kind === 'number' ? 1 : kind === 'color' ? '#333333' : '';
/** 渲染器的映射：布尔变量是根节点 data-log-<key> 属性，其余是 --log-<key> CSS 变量。 */
export const tokenCssHint = (key: string, v: LogTokenValue) =>
  typeof v === 'boolean' ? `[data-log-${key.toLowerCase()}="true"]` : `var(--log-${key})`;

// ---- 模板 ----

export type TemplateStarterKind = 'message' | 'element' | 'custom' | 'part' | 'page';

export const TEMPLATE_STARTERS: Array<{ kind: TemplateStarterKind; label: string; hint: string }> = [
  { kind: 'message', label: '消息模板', hint: '含一个正文插槽，可被消息规则或单条消息选用' },
  { kind: 'element', label: '元素模板', hint: '封面、标题、插图等 LOG 元素；按模板 ID 前缀匹配元素类型' },
  { kind: 'custom', label: '自定义元素', hint: '带参数 Schema 的 custom 元素，参数显示为 props.*' },
  { kind: 'page', label: '页眉 / 页脚', hint: '在“页面”面板中选作页眉或页脚' },
  { kind: 'part', label: '片段', hint: '被其他模板用 data-log-part 引用的公共结构' },
];

const ELEMENT_STARTERS: Record<string, string> = {
  cover: '<section class="log-cover"><h1 data-log-text="title"></h1><p data-log-text="subtitle"></p></section>',
  heading: '<h2 data-log-text="text"></h2>',
  transition: '<p class="log-transition" data-log-text="label"></p>',
  imagePlate: '<figure class="log-plate"><img data-log-bind-src="assetUrl"><figcaption data-log-text="caption"></figcaption></figure>',
  pageBreak: '<div class="log-page-break"></div>',
  quote: '<blockquote class="log-quote-block"><p data-log-text="text"></p></blockquote>',
  prologue: '<section class="log-prologue"><p data-log-text="text"></p></section>',
  epilogue: '<section class="log-epilogue"><p data-log-text="text"></p></section>',
  toc: '<section class="log-toc"></section>',
};
export const ELEMENT_STARTER_TYPES = Object.keys(ELEMENT_STARTERS);

export const templateStarter = (kind: TemplateStarterKind, id: string, elementType = 'heading'): { html: string; schema?: Record<string, unknown> } => {
  const cls = `log-${id.replace(/[^a-zA-Z0-9_-]/g, '').toLowerCase()}`;
  switch (kind) {
    case 'message':
      return { html: `<article class="${cls}"><header data-log-text="speaker.name"></header><div data-log-slot="body"></div></article>` };
    case 'element':
      return { html: ELEMENT_STARTERS[elementType] || ELEMENT_STARTERS.heading };
    case 'custom':
      return {
        html: `<aside class="${cls}"><strong data-log-text="props.label"></strong><p data-log-text="text"></p></aside>`,
        schema: {
          type: 'object',
          additionalProperties: false,
          properties: {
            props: { type: 'object', additionalProperties: false, properties: { label: { type: 'string', title: '标签', maxLength: 40 } } },
          },
        },
      };
    case 'page':
      return { html: `<div class="${cls}"><span data-log-text="title"></span><span data-log-text="section.title"></span></div>` };
    default:
      return { html: `<span class="${cls}"></span>` };
  }
};

/** 新模板 ID 的建议值：元素模板按类型前缀命名，渲染器才会把它列为该元素的候选。 */
export const suggestTemplateId = (copy: Pick<StyleWorkingCopy, 'templates'>, kind: TemplateStarterKind, elementType = 'heading') => {
  const base = kind === 'element' ? elementType : kind === 'custom' ? 'custom' : kind === 'page' ? 'pageHeader' : kind === 'part' ? 'part' : 'message';
  if (!(base in copy.templates) && kind !== 'part') return base;
  for (let n = 2; n < 1000; n += 1) if (!(`${base}${n}` in copy.templates)) return `${base}${n}`;
  return `${base}-${Date.now().toString(36)}`;
};

export const templateIdProblem = (copy: Pick<StyleWorkingCopy, 'templates'>, id: string): string => {
  if (!STYLE_KEY.test(id)) return '模板 ID 须以字母开头，只含字母、数字、下划线或连字符，最多 64 位';
  if (id in copy.templates) return `已存在模板 ${id}`;
  if (Object.keys(copy.templates).length >= 64) return '一个样式最多 64 个模板';
  return '';
};

/** 新增模板；custom 模板同时写入参数 Schema（Schema 文本无法解析时拒绝，以免覆盖用户源码）。 */
export const addTemplate = (copy: StyleWorkingCopy, id: string, html: string, schema?: Record<string, unknown>): string => {
  const problem = templateIdProblem(copy, id);
  if (problem) return problem;
  if (schema) {
    const parsed = parseStyleJson(copy).schema;
    if (!parsed) return '参数 Schema 源码当前无法解析，请先在高级源码中修正';
    copy.schemaText = formatJson({ ...parsed, [id]: schema });
  }
  copy.templates = { ...copy.templates, [id]: html };
  return '';
};

/** 模板关系：由已解析的模板库得出，不另立“模板类型”协议。 */
export interface TemplateGraph {
  /** 每个模板直接引用的片段（data-log-part）。 */
  parts: Record<string, string[]>;
  /** 展开片段后的正文插槽数；模板本身无效时缺省。 */
  bodySlots: Record<string, number>;
}

export interface TemplateDraftRefs {
  elements: number;
  header: boolean;
  footer: boolean;
  displayRules: number;
  overrides: number;
}

export interface TemplateUsage {
  ruleIndexes: number[];
  usedAsPartBy: string[];
  parts: string[];
  /** 展开片段后的正文插槽数；null 表示模板当前无效、无法判断。 */
  bodySlots: number | null;
  elementTypes: string[];
  propNames: string[];
  /** 当前 LOG 绑定的就是这个样式包时，草稿对该模板的引用。 */
  draftRefs: TemplateDraftRefs | null;
}

export const templateDraftRefs = (draft: LogDraft, id: string): TemplateDraftRefs => ({
  elements: draft.elements.filter((e) => e.templateId === id).length,
  header: draft.page.headerTemplateId === id,
  footer: draft.page.footerTemplateId === id,
  displayRules: (draft.displayRules || []).filter((r) => r.templateId === id).length,
  overrides: Object.values(draft.messageTemplateOverrides || {}).filter((t) => t === id).length,
});

const schemaPropNames = (schema: Record<string, unknown> | null, id: string): string[] => {
  const props = (schema?.[id] as any)?.properties?.props?.properties;
  return props && typeof props === 'object' ? Object.keys(props) : [];
};

export const analyzeTemplates = (
  copy: Pick<StyleWorkingCopy, 'templates'>,
  parsed: Pick<ParsedStyleJson, 'rules' | 'schema'>,
  graph: TemplateGraph,
  elementTypesFor: (id: string) => string[],
  draft: LogDraft | null,
): Record<string, TemplateUsage> => {
  const out: Record<string, TemplateUsage> = {};
  for (const id of Object.keys(copy.templates)) {
    out[id] = {
      ruleIndexes: (parsed.rules || []).flatMap((r, i) => (r.templateId === id ? [i] : [])),
      usedAsPartBy: Object.keys(graph.parts).filter((other) => other !== id && graph.parts[other].includes(id)),
      parts: graph.parts[id] || [],
      bodySlots: id in graph.bodySlots ? graph.bodySlots[id] : null,
      elementTypes: elementTypesFor(id),
      propNames: schemaPropNames(parsed.schema, id),
      draftRefs: draft ? templateDraftRefs(draft, id) : null,
    };
  }
  return out;
};

/** 以现有引用推导的用途标签；无法可靠判断时不猜测。 */
export const templateRoles = (u: TemplateUsage): string[] => {
  const roles: string[] = [];
  if (u.ruleIndexes.length) roles.push('消息规则');
  if (u.elementTypes.length) roles.push(`元素：${u.elementTypes.join('、')}`);
  if (u.usedAsPartBy.length) roles.push('片段');
  if (u.draftRefs?.header) roles.push('页眉');
  if (u.draftRefs?.footer) roles.push('页脚');
  if (!u.ruleIndexes.length && u.bodySlots === 1) roles.push('可选消息模板');
  return roles;
};

export const draftRefCount = (r: TemplateDraftRefs | null) =>
  r ? r.elements + r.displayRules + r.overrides + (r.header ? 1 : 0) + (r.footer ? 1 : 0) : 0;

/** 删除前的阻断项：规则与其他模板仍引用时必须先解除，否则服务端也会拒绝保存。 */
export const templateRemovalBlockers = (copy: Pick<StyleWorkingCopy, 'templates'>, id: string, usage: TemplateUsage | undefined): string[] => {
  const out: string[] = [];
  if (Object.keys(copy.templates).length <= 1) out.push('样式至少需要保留一个模板');
  if (usage?.ruleIndexes.length) out.push(`被第 ${usage.ruleIndexes.map((i) => i + 1).join('、')} 条消息规则引用`);
  if (usage?.usedAsPartBy.length) out.push(`被模板 ${usage.usedAsPartBy.join('、')} 作为片段引用`);
  return out;
};

/** 删除模板，并同步移除同名参数 Schema；Schema 文本无法解析时保留原文，由校验提示。 */
export const removeTemplate = (copy: StyleWorkingCopy, id: string) => {
  const next = { ...copy.templates };
  delete next[id];
  copy.templates = next;
  const schema = parseStyleJson(copy).schema;
  if (schema && id in schema) {
    const rest = { ...schema };
    delete rest[id];
    copy.schemaText = formatJson(rest);
  }
};

// ---- 资源 ----

/** 与服务端一致的字节嗅探：只认 PNG / JPEG / WebP 与 TTF / OTF（服务端另做完整解码）。 */
export const sniffStyleAssetMime = (bytes: Uint8Array): string => {
  const at = (i: number, ...sig: number[]) => sig.every((b, k) => bytes[i + k] === b);
  if (at(0, 0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a)) return 'image/png';
  if (at(0, 0xff, 0xd8, 0xff)) return 'image/jpeg';
  if (at(0, 0x52, 0x49, 0x46, 0x46) && at(8, 0x57, 0x45, 0x42, 0x50)) return 'image/webp';
  if (at(0, 0x4f, 0x54, 0x54, 0x4f)) return 'font/otf';
  if (at(0, 0x00, 0x01, 0x00, 0x00) || at(0, 0x74, 0x72, 0x75, 0x65)) return 'font/ttf';
  return '';
};

const EXT_BY_MIME: Record<string, string> = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp', 'font/ttf': 'ttf', 'font/otf': 'otf' };
export const isFontMime = (mime: string) => mime === 'font/ttf' || mime === 'font/otf';

/** 资源路径：assets/<规范化文件名>，与现有路径冲突时追加序号。 */
export const styleAssetPath = (fileName: string, mime: string, taken: Iterable<string>) => {
  const used = new Set(taken);
  const ext = EXT_BY_MIME[mime] || 'bin';
  const base = fileName.replace(/\.[^.]+$/, '').toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 60) || 'asset';
  let path = `assets/${base}.${ext}`;
  for (let n = 1; used.has(path); n += 1) path = `assets/${base}-${n}.${ext}`;
  return path;
};

const CSS_URL = /url\(\s*(['"]?)([^'")]*)\1\s*\)/gi;

/** CSS 中的 url(...) 文本引用计数（不解析完整 CSS，只作保守提示）。 */
export const cssAssetRefs = (css: string): Map<string, number> => {
  const out = new Map<string, number>();
  for (const m of css.replace(/\/\*[\s\S]*?\*\//g, ' ').matchAll(CSS_URL)) {
    const path = m[2].trim();
    out.set(path, (out.get(path) || 0) + 1);
  }
  return out;
};

/** 当前 LOG 草稿对样式资源的结构化引用（背景、元素图片、页面字体）。 */
export const draftStyleAssetRefs = (draft: LogDraft): Map<string, number> => {
  const out = new Map<string, number>();
  for (const [id, uses] of imageUsages(draft, 'style')) out.set(id, uses.length);
  const font = draft.page.fontFamily;
  if (font) out.set(font, (out.get(font) || 0) + 1);
  return out;
};

/** 冲突改基准前检查：旧基准资源若在最新版本中被移除或替换字节，保存接口无法继续继承工作副本看到的旧内容。 */
export const inheritedAssetsDivergedFromLatest = (
  copy: Pick<StyleWorkingCopy, 'base' | 'assets'>,
  latest: Pick<LogStylePackage, 'assets'>,
): LogStyleAsset[] => {
  const baseById = new Map((copy.base?.assets || []).map((a) => [a.id, a]));
  const latestById = new Map((latest.assets || []).map((a) => [a.id, a]));
  return copy.assets.filter((a) => {
    const base = baseById.get(a.id);
    if (!base) return false;
    const next = latestById.get(a.id);
    return !next || (!!base.sha256 && !!next.sha256 && base.sha256 !== next.sha256);
  });
};

// ---- 新建 ----

/** 空白起点：最小但完整的普通样式包；保存时由服务端分配 ID 与首个版本。 */
export const blankStylePackage = (rendererVersion: string): LogStylePackage => ({
  schemaVersion: 1,
  id: 'new-style',
  version: '1.0.0',
  name: '未命名样式',
  license: '',
  minRendererVersion: rendererVersion,
  tokens: { accent: '#3b5b8c', muted: '#6b6b6b' },
  templates: {
    message: '<article class="log-message"><header class="log-message__name" data-log-text="speaker.name"></header><div data-log-slot="body"></div></article>',
    heading: '<h2 class="log-heading" data-log-text="text"></h2>',
    cover: '<section class="log-cover"><h1 data-log-text="title"></h1><p data-log-text="subtitle"></p></section>',
    pageBreak: '<div class="log-page-break"></div>',
  },
  css: [
    '.log-message { margin: 0 0 0.9em; }',
    '.log-message__name { font-weight: bold; color: var(--log-accent); }',
    '.log-heading { margin: 1.2em 0 0.6em; color: var(--log-accent); }',
    '.log-cover { break-after: page; text-align: center; }',
    '.log-cover p { color: var(--log-muted); }',
    '.log-page-break { break-after: page; }',
  ].join('\n'),
  rules: [{ templateId: 'message' }],
  settingsSchema: {},
  assets: [],
});

/** 新样式 ID（ZIP 另存为新样式时使用）；服务端新建接口会自行分配，不使用此值。 */
export const freshStyleId = () => `style-${Array.from(crypto.getRandomValues(new Uint8Array(8)), (b) => b.toString(16).padStart(2, '0')).join('')}`;

// ---- 与工作台的接口 ----

/** 工作台或样式面板请 Studio 打开的目标；nonce 由发起方递增。 */
export type StyleStudioRequest =
  | { kind: 'library' }
  | { kind: 'open'; id: string; version: string }
  | { kind: 'fork'; id: string; version: string; name: string }
  | { kind: 'new' };

/** 本地预览：只替换工作台预览用的样式包，不写草稿；未保存资源以本地字节提供。 */
export interface StyleStudioPreview {
  pkg: LogStylePackage;
  assets: Record<string, { data: ArrayBuffer; mime: string }>;
  /** 每打开一个新的工作副本递增，工作台据此丢弃旧的资源缓存。 */
  session: number;
}
