// 样式 ZIP 的本地读写：沿用服务端 ImportLogStyle / ExportLogStyle 的既有文件布局，不定义新格式。
// - readStyleZip：导入前在浏览器里完整解析，列出服务端会拒绝的问题，并给出可在设计器中继续修改的工作包；
// - buildStyleZip：把（修改后的）工作包重新打成同样布局的 ZIP，仍提交既有导入接口，由服务端 ValidateLogStyle 权威校验。
// 这里不执行任何模板或 CSS 内容。
import type { LogStyleAsset, LogStylePackage, LogStyleRule, LogTokenValue } from '@/types/log';
import { LOG_RENDERER_VERSION } from './logTemplateCompiler';

let fflatePromise: Promise<typeof import('fflate')> | null = null;
const loadFflate = () => {
  fflatePromise ??= import('fflate');
  return fflatePromise;
};

export const STYLE_ZIP_LIMITS = {
  zipBytes: 20 * 1024 * 1024,
  files: 200,
  textBytes: 512 * 1024,
  assetBytes: 50 * 1024 * 1024,
  totalBytes: 50 * 1024 * 1024,
};

const REQUIRED = ['manifest.json', 'tokens.json', 'settings.schema.json', 'styles/main.css'];
const MANIFEST_KEYS = new Set(['schemaVersion', 'id', 'version', 'name', 'license', 'minRendererVersion', 'tokens', 'templates', 'css', 'rules', 'settingsSchema', 'assets']);
const SEMVER = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;

export interface StyleZipContent {
  /** 按 ZIP 内容组装的工作包；缺失或无效的部分已用安全默认值补齐，并记录在 issues。 */
  pkg: LogStylePackage;
  /** 资源字节，按资源 id。 */
  assets: Record<string, Uint8Array>;
  /** 原样保留的 previews/* 图片。 */
  extras: Record<string, Uint8Array>;
  files: string[];
  /** 原始 ZIP 直接导入会被服务端拒绝的问题。 */
  issues: string[];
  /** 不影响导入的提示。 */
  warnings: string[];
}

/** 与服务端相同的路径规则：规范化、无绝对路径、无 .. / 反斜杠 / 冒号 / NUL。 */
export const isSafeStyleZipPath = (name: string) => {
  if (!name || name.startsWith('/') || /[\\\0:]/.test(name)) return false;
  return name.split('/').every((seg) => seg !== '' && seg !== '.' && seg !== '..');
};

const semverGt = (a: string, b: string) => {
  const pa = a.split('.').map(Number);
  const pb = b.split('.').map(Number);
  for (let i = 0; i < 3; i += 1) {
    if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) > (pb[i] || 0);
  }
  return false;
};

const sniffImage = (b: Uint8Array) =>
  (b[0] === 0x89 && b[1] === 0x50 && b[2] === 0x4e && b[3] === 0x47)
  || (b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff)
  || (b[0] === 0x52 && b[1] === 0x49 && b[2] === 0x46 && b[3] === 0x46 && b[8] === 0x57 && b[9] === 0x45 && b[10] === 0x42 && b[11] === 0x50);

const isObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);

export const readStyleZip = async (file: Blob): Promise<StyleZipContent> => {
  if (file.size > STYLE_ZIP_LIMITS.zipBytes) throw new Error('ZIP 超过 20MB');
  const { unzipSync } = await loadFflate();
  const bytes = new Uint8Array(await file.arrayBuffer());
  const issues: string[] = [];
  const warnings: string[] = [];
  let count = 0;
  let declaredTotal = 0;
  let raw: Record<string, Uint8Array>;
  try {
    raw = unzipSync(bytes, {
      // 解压前按中央目录声明的大小拒绝超限条目，避免解压炸弹。
      filter: (f) => {
        if (f.name.endsWith('/')) return false;
        count += 1;
        if (count > STYLE_ZIP_LIMITS.files) throw new Error(`ZIP 文件数超过 ${STYLE_ZIP_LIMITS.files}`);
        const limit = /^(assets|previews)\//.test(f.name) ? STYLE_ZIP_LIMITS.assetBytes : STYLE_ZIP_LIMITS.textBytes;
        declaredTotal += f.originalSize;
        if (f.originalSize > limit || declaredTotal > STYLE_ZIP_LIMITS.totalBytes) throw new Error(`${f.name} 超过解压配额`);
        return true;
      },
    });
  } catch (error) {
    throw new Error(error instanceof Error && error.message ? error.message : '不是有效的 ZIP 文件');
  }
  const files = Object.keys(raw).sort();
  const text = (name: string) => new TextDecoder().decode(raw[name]);
  const known = new Set<string>();
  for (const name of files) if (!isSafeStyleZipPath(name)) issues.push(`可疑路径 ${name}，服务端会拒绝整个 ZIP`);
  for (const name of REQUIRED) if (!(name in raw)) issues.push(`缺少 ${name}`);

  let manifest: Record<string, unknown> = {};
  if ('manifest.json' in raw) {
    known.add('manifest.json');
    try {
      const parsed = JSON.parse(text('manifest.json'));
      if (isObject(parsed)) manifest = parsed;
      else issues.push('manifest.json 不是 JSON 对象');
    } catch {
      issues.push('manifest.json 无法解析');
    }
  }
  for (const key of Object.keys(manifest)) if (!MANIFEST_KEYS.has(key)) issues.push(`manifest.json 含未知字段 ${key}`);
  for (const key of ['tokens', 'templates', 'settingsSchema'] as const) {
    if (manifest[key] !== undefined && manifest[key] !== null) issues.push(`manifest.json 不能内嵌 ${key}，应放在独立文件中`);
  }
  if (manifest.css !== undefined && manifest.css !== '' && manifest.css !== null) issues.push('manifest.json 不能内嵌 css，应放在 styles/main.css');
  const str = (key: string, fallback: string) => {
    const v = manifest[key];
    if (typeof v === 'string') return v;
    if (v !== undefined) issues.push(`manifest.json 的 ${key} 应为字符串`);
    return fallback;
  };

  const readJson = <T>(name: string, fallback: T, check: (v: unknown) => boolean): T => {
    if (!(name in raw)) return fallback;
    known.add(name);
    try {
      const v = JSON.parse(text(name));
      if (check(v)) return v as T;
      issues.push(`${name} 结构不正确，已以空对象代替`);
    } catch {
      issues.push(`${name} 无法解析，已以空对象代替`);
    }
    return fallback;
  };
  const tokens = readJson<Record<string, LogTokenValue>>('tokens.json', {}, isObject);
  const settingsSchema = readJson<Record<string, unknown>>('settings.schema.json', {}, isObject);
  let css = '';
  if ('styles/main.css' in raw) {
    known.add('styles/main.css');
    css = text('styles/main.css');
  }
  const templates: Record<string, string> = {};
  for (const name of files) {
    const m = /^templates\/([^/]+)\.html$/.exec(name);
    if (!m) continue;
    known.add(name);
    templates[m[1]] = text(name);
  }
  if (!Object.keys(templates).length) issues.push('没有 templates/*.html 模板');

  let rules: LogStyleRule[] = [];
  if (Array.isArray(manifest.rules)) rules = manifest.rules.filter(isObject) as unknown as LogStyleRule[];
  else issues.push('manifest.json 缺少 rules 数组');

  const assets: LogStyleAsset[] = [];
  const assetBytes: Record<string, Uint8Array> = {};
  if (!Array.isArray(manifest.assets)) issues.push('manifest.json 缺少 assets 数组');
  for (const a of Array.isArray(manifest.assets) ? manifest.assets : []) {
    if (!isObject(a) || typeof a.id !== 'string' || typeof a.path !== 'string') {
      issues.push('manifest.json 中有无效的资源条目，已忽略');
      continue;
    }
    const data = raw[a.path];
    if (!data) {
      issues.push(`资源 ${a.path} 不在 ZIP 中，已从工作副本移除`);
      continue;
    }
    known.add(a.path);
    assets.push({ id: a.id, path: a.path, mime: typeof a.mime === 'string' ? a.mime : '', sha256: '' });
    assetBytes[a.id] = data;
  }

  const extras: Record<string, Uint8Array> = {};
  for (const name of files) {
    if (known.has(name)) continue;
    if (isSafeStyleZipPath(name) && name.startsWith('previews/') && sniffImage(raw[name])) {
      extras[name] = raw[name];
      continue;
    }
    issues.push(`无法识别的文件 ${name}，服务端会拒绝；已从工作副本移除`);
  }

  const minRendererVersion = str('minRendererVersion', LOG_RENDERER_VERSION);
  if (SEMVER.test(minRendererVersion) && semverGt(minRendererVersion, LOG_RENDERER_VERSION)) {
    warnings.push(`需要渲染器 ${minRendererVersion}，当前为 ${LOG_RENDERER_VERSION}`);
  }
  if (manifest.schemaVersion !== undefined && manifest.schemaVersion !== 1) issues.push('schemaVersion 必须为 1');

  return {
    pkg: {
      schemaVersion: 1,
      id: str('id', ''),
      version: str('version', '1.0.0'),
      name: str('name', ''),
      license: str('license', ''),
      minRendererVersion,
      tokens,
      templates,
      css,
      rules,
      settingsSchema,
      assets,
    },
    assets: assetBytes,
    extras,
    files,
    issues,
    warnings,
  };
};

/** 按服务端导出的布局打包：manifest 只含元信息、rules、assets；其余字段放独立文件。 */
export const buildStyleZip = async (pkg: LogStylePackage, assets: Record<string, Uint8Array>, extras: Record<string, Uint8Array> = {}): Promise<Uint8Array> => {
  const { zipSync, strToU8 } = await loadFflate();
  const { tokens, templates, css, settingsSchema, ...meta } = pkg;
  const files: Record<string, Uint8Array> = {
    'manifest.json': strToU8(JSON.stringify(meta)),
    'tokens.json': strToU8(JSON.stringify(tokens)),
    'settings.schema.json': strToU8(JSON.stringify(settingsSchema)),
    'styles/main.css': strToU8(css),
  };
  for (const [id, src] of Object.entries(templates)) files[`templates/${id}.html`] = strToU8(src);
  for (const a of pkg.assets) {
    const data = assets[a.id];
    if (!data) throw new Error(`资源 ${a.path} 缺少文件内容`);
    files[a.path] = data;
  }
  for (const [name, data] of Object.entries(extras)) if (isSafeStyleZipPath(name) && name.startsWith('previews/') && !(name in files)) files[name] = data;
  return zipSync(files, { level: 6 });
};
