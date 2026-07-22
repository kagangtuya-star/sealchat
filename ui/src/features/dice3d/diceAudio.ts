import { normalizeAttachmentId, resolveAttachmentUrl } from '@/composables/useAttachmentResolver'

export type DiceAudioConfigLike = {
  enabled?: boolean
  volume?: number
  soundAssetId?: string
}

export type DiceAudioFailure =
  | 'disabled'
  | 'no_asset'
  | 'volume_zero'
  | 'load_failed'
  | 'play_blocked'
  | 'unsupported'

export type DiceAudioPlayResult = {
  ok: boolean
  reason?: DiceAudioFailure
  message: string
}

export const describeDiceAudioFailure = (reason: DiceAudioFailure): string => {
  switch (reason) {
    case 'disabled':
      return '投掷音效已关闭'
    case 'no_asset':
      return '尚未上传自定义音效，投掷时不会播放声音'
    case 'volume_zero':
      return '音量为 0，投掷时不会播放声音'
    case 'load_failed':
      return '音效文件加载失败，请重新上传或更换格式（mp3/ogg/wav/webm）'
    case 'play_blocked':
      return '浏览器拦截了自动播放，请先点击页面任意处或「试听」以允许音效'
    case 'unsupported':
      return '当前浏览器不支持音频播放'
    default:
      return '音效不可用'
  }
}

const normalizeAssetId = (value?: string) => (value || '').trim()

type PendingPlay = {
  config: DiceAudioConfigLike
  resolve: (result: DiceAudioPlayResult) => void
}

class DiceAudioService {
  private unlocked = false
  private unlockHooked = false
  private context: AudioContext | null = null
  private buffers = new Map<string, AudioBuffer | null>()
  private loading = new Map<string, Promise<AudioBuffer | null>>()
  private pending: PendingPlay | null = null

  constructor() {
    this.hookUnlock()
  }

  private getAudioContextClass(): typeof AudioContext | null {
    if (typeof window === 'undefined') return null
    return window.AudioContext
      || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
      || null
  }

  private getContext(): AudioContext | null {
    const Ctor = this.getAudioContextClass()
    if (!Ctor) return null
    if (!this.context || this.context.state === 'closed') {
      this.context = new Ctor()
    }
    return this.context
  }

  private hookUnlock() {
    if (this.unlockHooked || typeof window === 'undefined') return
    this.unlockHooked = true
    const onInteract = () => {
      void this.unlock()
    }
    window.addEventListener('pointerdown', onInteract, { capture: true, passive: true })
    window.addEventListener('keydown', onInteract, { capture: true, passive: true })
    window.addEventListener('touchstart', onInteract, { capture: true, passive: true })
  }

  /** 在用户手势中调用；解锁后 WebAudio 可在 WS 回传时播放给本机及所有已交互客户端 */
  async unlock(): Promise<boolean> {
    const ctx = this.getContext()
    if (!ctx) return false
    try {
      if (ctx.state === 'suspended') await ctx.resume()
      // 极短静音缓冲，建立站点媒体参与度，避免仅掷骰异步回调时被拦
      if (!this.unlocked) {
        const silence = ctx.createBuffer(1, 1, ctx.sampleRate || 22050)
        const source = ctx.createBufferSource()
        source.buffer = silence
        source.connect(ctx.destination)
        source.start(0)
      }
      this.unlocked = ctx.state === 'running'
      if (this.unlocked) this.flushPending()
      return this.unlocked
    } catch {
      return false
    }
  }

  markUnlocked() {
    void this.unlock()
  }

  isUnlocked() {
    return this.unlocked
  }

  resolveUrl(assetId: string) {
    const id = normalizeAssetId(assetId)
    if (!id) return ''
    if (/^(?:https?:|data:|blob:)/i.test(id)) return id
    if (id.startsWith('//')) {
      return `${typeof window !== 'undefined' ? window.location.protocol : 'https:'}${id}`
    }
    const resolved = resolveAttachmentUrl(id)
    if (!resolved) {
      const bare = normalizeAttachmentId(id)
      if (!bare) return ''
      if (typeof window !== 'undefined' && window.location?.origin) {
        return `${window.location.origin}/api/v1/attachment/${encodeURIComponent(bare)}`
      }
      return `/api/v1/attachment/${encodeURIComponent(bare)}`
    }
    // 协议相对 URL 转绝对，避免部分环境 HTML/WebAudio 加载异常
    if (resolved.startsWith('//') && typeof window !== 'undefined') {
      return `${window.location.protocol}${resolved}`
    }
    if (resolved.startsWith('/') && typeof window !== 'undefined' && window.location?.origin) {
      return `${window.location.origin}${resolved}`
    }
    return resolved
  }

  inspect(config?: DiceAudioConfigLike | null): DiceAudioPlayResult {
    if (!config?.enabled) {
      return { ok: false, reason: 'disabled', message: describeDiceAudioFailure('disabled') }
    }
    const volume = Number(config.volume)
    if (!(volume > 0)) {
      return { ok: false, reason: 'volume_zero', message: describeDiceAudioFailure('volume_zero') }
    }
    if (!normalizeAssetId(config.soundAssetId)) {
      return { ok: false, reason: 'no_asset', message: describeDiceAudioFailure('no_asset') }
    }
    if (!this.getAudioContextClass() && typeof Audio === 'undefined') {
      return { ok: false, reason: 'unsupported', message: describeDiceAudioFailure('unsupported') }
    }
    return { ok: true, message: '将播放已上传的自定义投掷音效；本机与其他客户端均需曾点击页面以允许声音' }
  }

  invalidate(assetId?: string) {
    if (assetId === undefined) {
      this.buffers.clear()
      this.loading.clear()
      return
    }
    const id = normalizeAssetId(assetId)
    this.buffers.delete(id)
    this.loading.delete(id)
  }

  /** 投掷 payload 到达时预解码，提升各端同步听感 */
  prefetch(config?: DiceAudioConfigLike | null) {
    const status = this.inspect(config)
    if (!status.ok || !config?.soundAssetId) return
    void this.ensureLoaded(config.soundAssetId)
  }

  async ensureLoaded(assetId: string): Promise<AudioBuffer | null> {
    const id = normalizeAssetId(assetId)
    if (!id) return null
    if (this.buffers.has(id)) return this.buffers.get(id) || null
    const pending = this.loading.get(id)
    if (pending) return pending

    const task = (async () => {
      const url = this.resolveUrl(id)
      const ctx = this.getContext()
      if (!url || !ctx) {
        this.buffers.set(id, null)
        return null
      }
      try {
        const response = await fetch(url, { credentials: 'include', cache: 'force-cache' })
        if (!response.ok) throw new Error(`http_${response.status}`)
        const raw = await response.arrayBuffer()
        const buffer = await ctx.decodeAudioData(raw.slice(0))
        this.buffers.set(id, buffer)
        return buffer
      } catch {
        // WebAudio 失败时回退 HTMLAudio 路径仍可尝试播放
        this.buffers.set(id, null)
        return null
      } finally {
        this.loading.delete(id)
      }
    })()

    this.loading.set(id, task)
    return task
  }

  private playBuffer(buffer: AudioBuffer, volume: number) {
    const ctx = this.getContext()
    if (!ctx) throw new Error('unsupported')
    const source = ctx.createBufferSource()
    const gain = ctx.createGain()
    gain.gain.value = Math.max(0, Math.min(1, volume))
    source.buffer = buffer
    source.connect(gain).connect(ctx.destination)
    source.start(0)
  }

  private async playHtmlAudio(assetId: string, volume: number) {
    const url = this.resolveUrl(assetId)
    if (!url || typeof Audio === 'undefined') throw new Error('unsupported')
    const el = new Audio(url)
    el.volume = Math.max(0, Math.min(1, volume))
    el.preload = 'auto'
    await el.play()
  }

  private flushPending() {
    const item = this.pending
    if (!item) return
    this.pending = null
    void this.play(item.config).then(item.resolve)
  }

  private enqueueWhenBlocked(config: DiceAudioConfigLike): Promise<DiceAudioPlayResult> {
    return new Promise(resolve => {
      this.pending = { config, resolve }
    })
  }

  async play(config?: DiceAudioConfigLike | null, options?: { queueIfBlocked?: boolean }): Promise<DiceAudioPlayResult> {
    const status = this.inspect(config)
    if (!status.ok || !config) return status

    const assetId = normalizeAssetId(config.soundAssetId)
    const volume = Math.max(0, Math.min(1, Number(config.volume) || 0))
    const queueIfBlocked = options?.queueIfBlocked !== false

    // 尝试恢复上下文（若已有手势激活会成功）
    await this.unlock()

    const buffer = await this.ensureLoaded(assetId)
    try {
      if (buffer) {
        if (!this.unlocked) {
          if (queueIfBlocked) {
            return this.enqueueWhenBlocked(config)
          }
          return { ok: false, reason: 'play_blocked', message: describeDiceAudioFailure('play_blocked') }
        }
        this.playBuffer(buffer, volume)
        return { ok: true, message: 'ok' }
      }
      await this.playHtmlAudio(assetId, volume)
      this.unlocked = true
      return { ok: true, message: 'ok' }
    } catch (error) {
      if (error instanceof DOMException && error.name === 'NotAllowedError') {
        if (queueIfBlocked) {
          // 等用户下一次点击页面后再播一次（只保留最新）
          return this.enqueueWhenBlocked(config)
        }
        return { ok: false, reason: 'play_blocked', message: describeDiceAudioFailure('play_blocked') }
      }
      this.invalidate(assetId)
      return { ok: false, reason: 'load_failed', message: describeDiceAudioFailure('load_failed') }
    }
  }

  /**
   * 仅用于设置页「试听默认音效」：播一段本机合成探测音，验证浏览器是否允许出声。
   * 不参与真实投掷；投掷仍只使用已上传的自定义文件。
   */
  async playDefaultProbe(volume = 0.45): Promise<DiceAudioPlayResult> {
    if (!this.getAudioContextClass()) {
      return { ok: false, reason: 'unsupported', message: describeDiceAudioFailure('unsupported') }
    }
    const unlocked = await this.unlock()
    if (!unlocked) {
      return { ok: false, reason: 'play_blocked', message: describeDiceAudioFailure('play_blocked') }
    }
    const ctx = this.getContext()
    if (!ctx) {
      return { ok: false, reason: 'unsupported', message: describeDiceAudioFailure('unsupported') }
    }
    try {
      const now = ctx.currentTime
      const gain = ctx.createGain()
      const osc = ctx.createOscillator()
      osc.type = 'triangle'
      osc.frequency.setValueAtTime(660, now)
      osc.frequency.exponentialRampToValueAtTime(220, now + 0.16)
      const level = Math.max(0.05, Math.min(1, volume)) * 0.22
      gain.gain.setValueAtTime(level, now)
      gain.gain.exponentialRampToValueAtTime(0.0001, now + 0.18)
      osc.connect(gain).connect(ctx.destination)
      osc.start(now)
      osc.stop(now + 0.2)
      return { ok: true, message: 'ok' }
    } catch (error) {
      if (error instanceof DOMException && error.name === 'NotAllowedError') {
        return { ok: false, reason: 'play_blocked', message: describeDiceAudioFailure('play_blocked') }
      }
      return { ok: false, reason: 'unsupported', message: describeDiceAudioFailure('unsupported') }
    }
  }
}

export const diceAudio = new DiceAudioService()
