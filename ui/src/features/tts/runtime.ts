// Pure runtime guards shared by async playback and charged request submission.
export class SpeechEpoch {
  private revision = 0
  capture() { return this.revision }
  invalidate() { this.revision++ }
  current(revision: number) { return revision === this.revision }
}

// File compatibility mode must not cut off an utterance when its successor arrives.
export class SpeechPlaybackOrder {
  private latest = -1
  private playing = -1
  start(epoch: number, busy: boolean): 'play' | 'skip' | 'ignore' {
    if (!Number.isSafeInteger(epoch) || epoch < 0 || epoch <= this.latest) return 'ignore'
    this.latest = epoch
    if (busy) return 'skip'
    this.playing = epoch
    return 'play'
  }
  cancel(epoch: number) { return epoch === this.playing }
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
