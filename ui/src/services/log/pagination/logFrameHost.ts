// 分页引擎外层接口：一个接口、一个默认引擎（自研测量分页，运行于隔离 iframe）。
// iframe: srcdoc + sandbox="allow-scripts allow-modals"（无 same-origin / forms / popups / top-navigation），
// CSP 只允许带 nonce 的应用脚本，禁止 connect 与外部资源；图片/字体由外层按权限取字节后传入。
import runtimeSource from './logFrameRuntime.js?raw';
import baseCss from './logFrame.css?raw';
import type { LogPageSettings } from '@/types/log';
import type { LogImageResize } from '../logImageLayout';
import { LogFrameChannel } from './logFrameChannel';

export interface FrameBlock {
  id: string;
  kind: 'message' | 'element';
  html: string;
  classes: string[];
  attrs: Record<string, string>;
  vars: Record<string, string>;
  breakBefore: boolean;
  breakAfter: boolean;
}

export interface FrameDocument {
  page: LogPageSettings;
  css: string;
  rootVars: Record<string, string>;
  rootAttrs: Record<string, string>;
  blocks: FrameBlock[];
  headerHtml: string | null;
  footerHtml: string | null;
  backgroundToken: string | null;
  pageNumberStart: number;
}

export interface FrameAnchor { nodeId: string; fragmentIndex: number; offsetWithinViewport: number }

export interface FrameLayoutResult {
  revision: number;
  pageCount: number;
  map: Record<string, number[]>;
  warnings: Array<{ code: string; nodeId: string | null }>;
  missingAssets: number;
  nodes: number;
  domNodes: number;
  elapsedMs: number;
}

export type FrameEvent =
  | ({ type: 'imageResize' } & LogImageResize)
  | { type: 'select'; nodeId: string | null; kind?: string; shift?: boolean; additive?: boolean }
  | { type: 'open'; nodeId: string; kind?: string }
  | { type: 'drop'; nodeId: string; targetId: string; side: 'before' | 'after' }
  | { type: 'action'; nodeId: string; action: 'edit' | 'insert-before' | 'insert-after' }
  | { type: 'anchor'; anchor: FrameAnchor | null; nearEnd?: boolean }
  | { type: 'key'; key: string }
  /** 设计态在原消息上打开右键菜单；x/y 为 iframe 视口坐标。 */
  | { type: 'context'; nodeId: string; x: number; y: number }
  | { type: 'progress'; revision: number; done: number; total: number }
  | { type: 'scrollMiss'; nodeId: string }
  /** 校对审阅层：点选批注、在批注卡片上决定、在卡片上前后导航、无法精确定位的批注键。 */
  | { type: 'proofPick'; key: string }
  | { type: 'proofDecide'; key: string; decision: FrameProofDecision | null }
  | { type: 'proofNav'; step: 1 | -1 }
  | { type: 'proofMapped'; missed: string[] };

export type FrameProofDecision = 'accepted' | 'rejected';

/**
 * 一处校对批注。start/end 是消息 .log-body 可见文本（跳过按钮与图片占位）内的 UTF-16 区间；
 * start 为 -1 表示无法可靠定位，审阅层只把批注锚定到整条消息，不猜测字符位置。
 */
export interface FrameProofItem {
  reason?: string;
  key: string;
  start: number;
  end: number;
  level: 'safe' | 'suggest';
  label: string;
  original: string;
  replacement: string;
  /** 降级展示时的原文上下文。 */
  before: string;
  after: string;
  /** 无法定位的原因（只用于降级卡片）。 */
  note?: string;
}

export interface FrameProofNode {
  nodeId: string;
  /** 外层按同一规则得到的正文长度；与 iframe 内不一致时整条消息降级。 */
  length: number;
  items: FrameProofItem[];
}

export interface FrameProofView {
  active: string | null;
  states: Record<string, FrameProofDecision>;
  /** 当前批注在导航序列中的位置（1 起）。 */
  position: { index: number; total: number } | null;
}

export interface FrameAsset { token: string; data: ArrayBuffer | null; mime: string }
export interface FrameRelocation { sourceMessageIds: string[] }
export interface FramePrintRange { fromPage: number; toPage: number }

export interface LayoutRequest {
  doc: FrameDocument;
  mode: 'paged' | 'flow';
  spread: boolean;
  design: boolean;
  canResizeImages: boolean;
  selection: string[];
  relocation?: FrameRelocation | null;
  /** 多选模式：隐藏单条快捷操作与图片手柄，点击只切换选择。 */
  multi?: boolean;
  anchor?: FrameAnchor | null;
}

const randomToken = () => {
  const bytes = new Uint8Array(18);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
};

export const LOG_FRAME_SANDBOX = 'allow-scripts allow-modals';

const buildSrcdoc = (token: string, nonce: string) => {
  const csp = [
    "default-src 'none'",
    `script-src 'nonce-${nonce}'`,
    "style-src 'unsafe-inline'",
    'img-src blob: data:',
    'font-src blob: data:',
    "connect-src 'none'",
    "media-src 'none'",
    "object-src 'none'",
    "frame-src 'none'",
    "worker-src 'none'",
    "form-action 'none'",
    "base-uri 'none'",
  ].join('; ');
  const script = runtimeSource.replace("'__LOG_FRAME_TOKEN__'", JSON.stringify(token));
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">`
    + `<meta http-equiv="Content-Security-Policy" content="${csp}">`
    + `<meta name="referrer" content="no-referrer">`
    + `<style id="log-base">${baseCss}</style><style id="log-page-rule"></style><style id="log-package"></style>`
    + `</head><body><main id="log-root" role="document" aria-label="LOG 预览"></main>`
    + `<script nonce="${nonce}">${script.replace(/<\/script/gi, '<\\/script')}</script></body></html>`;
};

/**
 * 隔离预览宿主。layout(input, revision) 返回该修订的结果；旧修订即使完成也被丢弃。
 * 协议本身在 LogFrameChannel 中，与后台 PDF Render Surface 共用。
 */
export class LogFrameEngine extends LogFrameChannel {
  private readonly onLoad = () => this.handshake();

  constructor(private readonly iframe: HTMLIFrameElement, onEvent: (event: FrameEvent) => void) {
    super(randomToken(), onEvent);
    iframe.setAttribute('sandbox', LOG_FRAME_SANDBOX);
    iframe.setAttribute('referrerpolicy', 'no-referrer');
    iframe.addEventListener('load', this.onLoad);
    iframe.srcdoc = buildSrcdoc(this.token, randomToken());
  }

  private handshake() {
    if (this.disposed || !this.iframe.contentWindow) return;
    this.connect(this.iframe.contentWindow);
  }

  dispose() {
    if (this.disposed) return;
    this.iframe.removeEventListener('load', this.onLoad);
    super.dispose();
  }
}
