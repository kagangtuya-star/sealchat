// Pure runtime guards shared by async playback and charged request submission.
export class SpeechEpoch {
  private revision = 0
  capture() { return this.revision }
  invalidate() { this.revision++ }
  current(revision: number) { return revision === this.revision }
}

export type SpeechEnd = 'played' | 'stopped' | 'failed'

// One browser's automatic playback queue for one channel subscription. The
// server orders utterances; only this page knows what it actually finished.
// `settled` means played to the end, stopped or cancelled here: never again.
export class SpeechQueue {
  private latest = -1
  private epochs = new Map<number, string>()
  private settled = new Set<string>()
  readonly pending: string[] = []
  current = ''
  constructor(private limit = 64) {}

  // A start frame plays now only when nothing is playing or owed before it.
  // A busy page never consumes a successor's realtime PCM; it waits for the file.
  start(epoch: number, messageId: string, idle: boolean): 'play' | 'queue' | 'ignore' {
    if (!Number.isSafeInteger(epoch) || epoch < 0 || epoch <= this.latest || !messageId) return 'ignore'
    this.latest = epoch
    this.epochs.set(epoch, messageId)
    if (this.epochs.size > this.limit) this.epochs.delete(this.epochs.keys().next().value!)
    if (messageId === this.current || this.settled.has(messageId)) return 'ignore'
    if (idle && !this.current && (this.pending.length === 0 || this.pending[0] === messageId)) {
      this.begin(messageId)
      return 'play'
    }
    this.offer(messageId)
    return 'queue'
  }
  // Epochs are per server process and per connection; a new socket starts over.
  resetEpochs() {
    this.latest = -1
    this.epochs.clear()
  }
  // The message a control frame (end/cancel/desynced) refers to, if its start reached this page.
  message(epoch: number) { return this.epochs.get(epoch) ?? '' }
  offer(messageId: string) {
    if (!messageId || messageId === this.current || this.settled.has(messageId) || this.pending.includes(messageId)) return false
    this.pending.push(messageId)
    if (this.pending.length > this.limit) this.pending.shift()
    return true
  }
  begin(messageId: string) {
    this.remove(messageId)
    this.current = messageId
  }
  // A failed realtime attempt stays owed at the head: its archive plays instead.
  end(messageId: string, result: SpeechEnd, retry: boolean) {
    if (this.current === messageId) this.current = ''
    if (result === 'failed' && retry) {
      if (!this.pending.includes(messageId)) this.pending.unshift(messageId)
      return
    }
    this.dismiss(messageId)
  }
  dismiss(messageId: string) {
    this.remove(messageId)
    this.settled.add(messageId)
    if (this.settled.size > this.limit * 4) this.settled.delete(this.settled.values().next().value!)
  }
  private remove(messageId: string) {
    const index = this.pending.indexOf(messageId)
    if (index >= 0) this.pending.splice(index, 1)
  }
}

export function speechAvailableAmount(limit: number | null, settled: number, reserved: number): number | null {
  if (limit === null) return null
  return Math.max(0, limit - settled - reserved)
}

export class SpeechRequestKeys {
  private keys = new Map<string, string>()
  clear() { this.keys.clear() }
  resolve(operation: string, request: { requestKey: string; [key: string]: unknown }) {
    const fingerprint = JSON.stringify(Object.fromEntries(
      Object.entries({ ...request, operation }).filter(([key]) => key !== 'requestKey').sort(([a], [b]) => a.localeCompare(b)),
    ))
    const existing = this.keys.get(fingerprint)
    if (existing) return existing
    if (this.keys.size >= 256) throw new Error('本页语音操作数量已达上限，请刷新后查询已有任务。')
    this.keys.set(fingerprint, request.requestKey)
    return request.requestKey
  }
}
