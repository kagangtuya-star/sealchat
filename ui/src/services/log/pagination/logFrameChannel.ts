// 分页运行时的消息协议端：握手、资源投递、排版与打印请求。
// 预览 iframe（LogFrameEngine）与后台 PDF Render Surface（运行时位于顶层文档）共用同一实现，
// 因此两条路径的分页语义只有一份。
import type {
  FrameAsset,
  FrameEvent,
  FrameLayoutResult,
  FramePrintRange,
  FrameProofNode,
  FrameProofView,
  FrameRelocation,
  LayoutRequest,
} from './logFrameHost';

export class LogFrameChannel {
  protected channel: MessageChannel | null = null;
  private readyPromise: Promise<void>;
  private resolveReady: (() => void) | null = null;
  private pending = new Map<number, { resolve: (r: FrameLayoutResult) => void; reject: (e: Error) => void; started: number }>();
  private pendingPrint = new Map<number, { resolve: () => void; reject: (e: Error) => void }>();
  private printSeq = 0;
  private deliveredAssets = new Set<string>();
  protected disposed = false;

  constructor(protected readonly token: string, private readonly onEvent: (event: FrameEvent) => void) {
    this.readyPromise = new Promise((resolve) => { this.resolveReady = resolve; });
  }

  /** 与运行时建立端口；target 为运行时所在窗口（iframe.contentWindow 或顶层 window）。 */
  protected connect(target: Window) {
    if (this.disposed) return;
    this.channel?.port1.close();
    this.deliveredAssets.clear();
    const channel = new MessageChannel();
    this.channel = channel;
    channel.port1.onmessage = (ev) => this.receive(ev.data);
    // opaque origin 为 "null"，因此只能用 '*' 投递；接收端校验 source 与会话令牌。
    target.postMessage({ type: 'log-frame-init', token: this.token }, '*', [channel.port2]);
  }

  private receive(msg: any) {
    if (!msg || typeof msg.type !== 'string') return;
    switch (msg.type) {
      case 'ready':
        this.resolveReady?.();
        break;
      case 'layout': {
        const entry = this.pending.get(msg.revision);
        if (!entry) return;
        this.pending.delete(msg.revision);
        entry.resolve({ ...msg, elapsedMs: Math.round(performance.now() - entry.started) } as FrameLayoutResult);
        break;
      }
      case 'error': {
        const entry = this.pending.get(msg.revision);
        if (!entry) return;
        this.pending.delete(msg.revision);
        entry.reject(new Error(String(msg.message || '排版失败')));
        break;
      }
      case 'printDone': {
        const entry = this.pendingPrint.get(Number(msg.requestId));
        if (!entry) return;
        this.pendingPrint.delete(Number(msg.requestId));
        entry.resolve();
        break;
      }
      case 'printError': {
        const entry = this.pendingPrint.get(Number(msg.requestId));
        if (!entry) return;
        this.pendingPrint.delete(Number(msg.requestId));
        entry.reject(new Error(String(msg.message || '打印准备失败')));
        break;
      }
      case 'select':
      case 'imageResize':
      case 'open':
      case 'drop':
      case 'action':
      case 'anchor':
      case 'key':
      case 'context':
      case 'progress':
      case 'scrollMiss':
      case 'proofPick':
      case 'proofDecide':
      case 'proofNav':
      case 'proofMapped':
        this.onEvent(msg as FrameEvent);
        break;
      default:
        break;
    }
  }

  get ready() {
    return this.readyPromise;
  }

  /** 只投递运行时尚未持有的资源；字节被复制而非转移，外层缓存保持可用。 */
  async provideAssets(assets: FrameAsset[]) {
    await this.readyPromise;
    const fresh = assets.filter((a) => !this.deliveredAssets.has(a.token));
    if (!fresh.length || !this.channel) return;
    fresh.forEach((a) => this.deliveredAssets.add(a.token));
    this.channel.port1.postMessage({ type: 'assets', items: fresh });
  }

  hasAsset(token: string) {
    return this.deliveredAssets.has(token);
  }

  async layout(input: LayoutRequest, revision: number): Promise<FrameLayoutResult | null> {
    await this.readyPromise;
    if (this.disposed || !this.channel) return null;
    // 新修订开始时，所有更早的等待者以 null 结束（结果作废）。
    for (const [rev, entry] of this.pending) {
      if (rev < revision) {
        this.pending.delete(rev);
        entry.resolve(null as unknown as FrameLayoutResult);
      }
    }
    const result = new Promise<FrameLayoutResult>((resolve, reject) => {
      this.pending.set(revision, { resolve, reject, started: performance.now() });
    });
    this.channel.port1.postMessage({ type: 'render', revision, ...input });
    const r = await result;
    return r && r.revision === revision ? r : null;
  }

  select(ids: string[]) {
    this.channel?.port1.postMessage({ type: 'select', ids });
  }

  relocate(state: FrameRelocation | null) {
    this.channel?.port1.postMessage({ type: 'relocate', state });
  }

  multi(active: boolean) {
    this.channel?.port1.postMessage({ type: 'multi', active });
  }

  scrollTo(nodeId: string, flash = true) {
    this.channel?.port1.postMessage({ type: 'scrollTo', nodeId, flash });
  }

  /** 校对审阅层：批注位置随每次排版在 iframe 内重新映射；null 关闭审阅层。 */
  proofMarks(nodes: FrameProofNode[] | null) {
    this.channel?.port1.postMessage({ type: 'proofMarks', nodes });
  }

  proofView(view: FrameProofView | null) {
    this.channel?.port1.postMessage({ type: 'proofView', view });
  }

  /** 滚动到批注；批注所在消息尚未排版时，等待之后的排版完成再滚动。 */
  proofReveal(key: string | null) {
    this.channel?.port1.postMessage({ type: 'proofReveal', key });
  }

  zoom(value: number) {
    this.channel?.port1.postMessage({ type: 'zoom', zoom: value });
  }

  /** 设置工作区（纸张之外）的底色；color 为 null 时恢复默认。 */
  canvas(color: string | null, dark = false, accent?: string) {
    this.channel?.port1.postMessage({ type: 'canvas', color, dark, accent });
  }

  async print(range?: FramePrintRange) {
    await this.readyPromise;
    if (this.disposed || !this.channel) throw new Error('打印预览不可用');
    const requestId = ++this.printSeq;
    const result = new Promise<void>((resolve, reject) => {
      this.pendingPrint.set(requestId, { resolve, reject });
    });
    this.channel.port1.postMessage({
      type: 'print',
      requestId,
      fromPage: range?.fromPage,
      toPage: range?.toPage,
    });
    return result;
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    try {
      this.channel?.port1.postMessage({ type: 'dispose' });
    } catch {
      // 端口可能已关闭
    }
    this.channel?.port1.close();
    this.channel = null;
    for (const entry of this.pending.values()) entry.resolve(null as unknown as FrameLayoutResult);
    this.pending.clear();
    for (const entry of this.pendingPrint.values()) entry.reject(new Error('打印已取消'));
    this.pendingPrint.clear();
    this.deliveredAssets.clear();
  }
}

/** 运行时直接加载在当前顶层文档中时使用的协议端（后台 PDF Render Surface）。 */
export class LogFrameSelfChannel extends LogFrameChannel {
  constructor(token: string, onEvent: (event: FrameEvent) => void = () => {}) {
    super(token, onEvent);
    this.connect(window);
  }
}
