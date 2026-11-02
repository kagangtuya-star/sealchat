// Per-browser decisions about world scripts. A decision binds packageId to
// the exact jsHash: any script change invalidates both trust and denial.
// Value format (v1): { [packageId]: { jsHash, decision, trustedAt|deniedAt, worldId, name } }.
// Entries without `decision` are treated as trusted (the original v1 shape).
export const UI_CUSTOMIZATION_TRUST_KEY = 'sealchat.uiCustomization.trust.v1'

export type TrustDecision = 'trusted' | 'denied'

export interface TrustEntry {
  runtimeFingerprint?: string
  jsHash: string
  decision?: TrustDecision
  trustedAt?: number
  deniedAt?: number
  worldId?: string
  name?: string
}

export interface TrustSubject {
  runtimeFingerprint?: string
  channelCapabilities?: string[]
  id: string
  jsHash: string
  worldId: string
  name: string
}

type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

export function createTrustStore(storage: StorageLike | null, now: () => number = Date.now) {
  const read = (): Record<string, TrustEntry> => {
    try {
      const parsed = JSON.parse(storage?.getItem(UI_CUSTOMIZATION_TRUST_KEY) || '{}')
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {}
    } catch {
      return {}
    }
  }
  const write = (data: Record<string, TrustEntry>) => {
    try { storage?.setItem(UI_CUSTOMIZATION_TRUST_KEY, JSON.stringify(data)) } catch { /* quota / private mode */ }
  }
  const decisionOf = (id: string, jsHash: string): TrustDecision | null => {
    const entry = read()[id]
    if (!entry || typeof entry !== 'object' || !jsHash || (entry.runtimeFingerprint || entry.jsHash) !== jsHash) return null
    return entry.decision === 'denied' ? 'denied' : 'trusted'
  }
  const decide = (subjects: TrustSubject[], decision: TrustDecision) => {
    const data = read()
    const at = now()
    for (const s of subjects) {
      if (!s.id || !s.jsHash) continue
      const fingerprint = s.runtimeFingerprint && s.runtimeFingerprint !== s.jsHash ? { runtimeFingerprint: s.runtimeFingerprint } : {}
      data[s.id] = decision === 'trusted'
        ? { jsHash: s.jsHash, ...fingerprint, decision, trustedAt: at, worldId: s.worldId, name: s.name }
        : { jsHash: s.jsHash, ...fingerprint, decision, deniedAt: at, worldId: s.worldId, name: s.name }
    }
    write(data)
  }
  return {
    decisionOf,
    isTrusted: (id: string, jsHash: string) => decisionOf(id, jsHash) === 'trusted',
    trust: (subjects: TrustSubject[]) => decide(subjects, 'trusted'),
    deny: (subjects: TrustSubject[]) => decide(subjects, 'denied'),
    entriesForWorld(worldId: string) {
      return Object.entries(read())
        .filter(([, e]) => e && e.worldId === worldId)
        .map(([id, e]) => ({ id, ...e, decision: e.decision === 'denied' ? 'denied' as const : 'trusted' as const }))
    },
    forget(ids: string[]) {
      const data = read()
      for (const id of ids) delete data[id]
      write(data)
    },
  }
}

export type TrustStore = ReturnType<typeof createTrustStore>
