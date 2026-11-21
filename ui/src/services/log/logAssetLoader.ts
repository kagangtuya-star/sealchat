// 资源令牌 → 字节的核心：并发限制、缓存、平台字体分片规划与 full 降级。
// 只依赖注入的取数函数，不引用 SPA 的 axios / Store，供设计预览（useLogAssets）
// 与后台 PDF Render Surface 共用，保证两边的字体与资源语义一致。
import type { PlatformFontAsset, PlatformFontSubsetManifest } from '@/services/font/platformFontTypes';
import { inferPlatformFontFormat, type PlatformFontChunk } from '@/services/font/platformFontSubsetRuntime';
import { buildLogFontPlan, collectLogFontText, collectLogPlatformFontIds, logFontChunkToken } from './logFontPlan';
import type { FrameAsset, FrameDocument } from './pagination/logFrameHost';

const MAX_PARALLEL = 6;
const FONT_MIME = /^(font\/(ttf|otf|sfnt|woff|woff2)|application\/(x-font-ttf|x-font-otf|font-sfnt))$/;

export interface PreparedLogFrame { doc: FrameDocument; assets: FrameAsset[] }

export interface LogFontInfo { meta?: PlatformFontAsset; manifest?: PlatformFontSubsetManifest }

export interface LogFontBytes { data: ArrayBuffer; mime: string }

export interface LogAssetSource {
  /** 非字体令牌（att: / body: / style: / proj: / pub:）的取数；不支持的令牌返回 data:null。 */
  fetchAsset: (token: string, signal: AbortSignal) => Promise<FrameAsset>;
  fontInfo: (fontId: string) => Promise<LogFontInfo>;
  /** chunk 为空表示完整字体文件；返回 null 表示不可用。 */
  fetchFont: (token: string, fontId: string, chunk: PlatformFontChunk | undefined, signal: AbortSignal) => Promise<LogFontBytes | null>;
}

export const createLogAssetLoader = (source: LogAssetSource) => {
  const cache = new Map<string, Promise<FrameAsset>>();
  const controller = new AbortController();
  // 与当前资源宿主同生命周期，编辑文本时复用清单，不建立全局字体缓存。
  const fontInfo = new Map<string, Promise<LogFontInfo>>();
  const getFontInfo = (id: string) => {
    let task = fontInfo.get(id);
    if (!task) {
      task = source.fontInfo(id).catch(() => ({}));
      fontInfo.set(id, task);
    }
    return task;
  };
  let active = 0;
  const queue: Array<() => void> = [];

  const acquire = () => new Promise<void>((resolve) => {
    if (active < MAX_PARALLEL) {
      active += 1;
      resolve();
    } else {
      queue.push(() => {
        active += 1;
        resolve();
      });
    }
  });
  const release = () => {
    active -= 1;
    queue.shift()?.();
  };

  const fetchToken = async (token: string): Promise<FrameAsset> => {
    const [kind, ...rest] = token.split(':');
    const id = rest.join(':');
    await acquire();
    try {
      if (kind === 'platform-font') {
        const match = /^([A-Za-z0-9_-]{1,100}):(full|chunk:[a-f0-9]+)$/.exec(id);
        if (!match) return { token, data: null, mime: '' };
        const fontId = match[1];
        const info = await getFontInfo(fontId);
        const chunk = match[2] === 'full' ? undefined : info.manifest?.chunks?.find(item => logFontChunkToken(fontId, item) === token);
        if (match[2] !== 'full' && !chunk) return { token, data: null, mime: '' };
        const bytes = await source.fetchFont(token, fontId, chunk, controller.signal);
        if (!bytes) return { token, data: null, mime: '' };
        let mime = (bytes.mime || '').split(';')[0].trim().toLowerCase();
        if (!mime || mime === 'application/octet-stream') {
          const format = inferPlatformFontFormat(chunk?.name || info.meta?.sourceFileName || '', chunk?.mimeType || info.meta?.sourceMimeType);
          mime = ({ woff2: 'font/woff2', woff: 'font/woff', opentype: 'font/otf', truetype: 'font/ttf' } as Record<string, string>)[format] || '';
        }
        if (!FONT_MIME.test(mime)) return { token, data: null, mime: '' };
        return { token, data: bytes.data, mime };
      }
      return await source.fetchAsset(token, controller.signal);
    } catch {
      return { token, data: null, mime: '' };
    } finally {
      release();
    }
  };

  const ensure = (tokens: string[]) => Promise.all(tokens.map((token) => {
    let p = cache.get(token);
    if (!p) {
      p = fetchToken(token);
      cache.set(token, p);
    }
    return p;
  }));

  /** 在任何字节送入运行时前完成分片规划和必要的 full 降级。 */
  const prepare = async (doc: FrameDocument, tokens: string[], hasAsset: (token: string) => boolean): Promise<PreparedLogFrame> => {
    const ids = collectLogPlatformFontIds(doc);
    if (!ids.length) return { doc, assets: await ensure(tokens.filter(token => !hasAsset(token))) };
    const text = collectLogFontText(doc);
    const missing = (items: string[]) => items.filter(token => !hasAsset(token));
    const fonts = await Promise.all(ids.map(async id => {
      const info = text.text || !text.reliable ? await getFontInfo(id) : {};
      let plan = buildLogFontPlan(id, text, info.meta, info.manifest);
      let assets = await ensure(missing(plan.tokens));
      if (plan.subset && assets.some(asset => !asset.data)) {
        plan = buildLogFontPlan(id, text, info.meta, undefined, true);
        assets = await ensure(missing(plan.tokens));
      }
      return { plan, assets };
    }));
    return {
      doc: { ...doc, css: `${fonts.map(font => font.plan.css).join('\n')}\n${doc.css}` },
      assets: [...await ensure(missing(tokens)), ...fonts.flatMap(font => font.assets)],
    };
  };

  const seed = (token: string, data: ArrayBuffer, mime: string) => {
    if (!cache.has(token)) cache.set(token, Promise.resolve({ token, data, mime }));
  };

  const invalidate = (prefix: string) => {
    for (const key of Array.from(cache.keys())) {
      if (key.startsWith(prefix)) cache.delete(key);
    }
  };

  const dispose = () => {
    controller.abort();
    cache.clear();
    fontInfo.clear();
  };

  return { ensure, prepare, seed, invalidate, dispose, signal: controller.signal };
};

export type LogAssetLoader = ReturnType<typeof createLogAssetLoader>;
