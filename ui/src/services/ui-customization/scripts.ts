import type { ScriptPackage } from './jsRuntime'
import type { TrustStore, TrustSubject } from './trust'
import type { RuntimePackage } from './types'
import { runtimeFingerprint } from './fingerprint'

export type PendingWorldScript = TrustSubject

interface ScriptRuntimeLike {
  reconcile(packages: ScriptPackage[]): Promise<void>
  dispose(): Promise<void>
}

const toScript = (p: RuntimePackage): ScriptPackage => ({
  id: p.id, scopeType: p.scopeType, name: p.name, js: p.js || '', jsHash: p.jsHash || '',
  runtimeFingerprint: runtimeFingerprint(p), channelCapabilities: p.channelCapabilities || [],
})

// Decides which server-approved scripts may run in this browser. Platform
// scripts run directly; world scripts additionally need a trust decision for
// their exact packageId + runtime fingerprint. Undecided ones become pending.
export function createScriptController({
  runtime,
  trust,
  getWorldId,
  onPending,
}: {
  runtime: ScriptRuntimeLike
  trust: TrustStore
  getWorldId: () => string
  onPending: (pending: PendingWorldScript[]) => void
}) {
  let last: RuntimePackage[] = []
  let worldId = ''
  // "Later" only hides a prompt until this page is reloaded.
  const postponed = new Set<string>()
  const key = (p: { id: string; jsHash?: string; runtimeFingerprint?: string }) => `${p.id}:${runtimeFingerprint(p)}`

  const evaluate = () => {
    const runnable: ScriptPackage[] = []
    const pending: PendingWorldScript[] = []
    for (const p of last) {
      if (!p.js || !p.jsHash) continue
      if (p.scopeType === 'platform') { runnable.push(toScript(p)); continue }
      if (!p.requiresTrust) continue
      const decision = trust.decisionOf(p.id, runtimeFingerprint(p))
      if (decision === 'trusted') runnable.push(toScript(p))
      else if (!decision && !postponed.has(key(p))) pending.push({ id: p.id, jsHash: p.jsHash, runtimeFingerprint: runtimeFingerprint(p), channelCapabilities: p.channelCapabilities || [], name: p.name, worldId })
    }
    onPending(pending)
    return runtime.reconcile(runnable)
  }

  const subjects = (ids: string[]) => last
    .filter(p => p.scopeType === 'world' && p.js && p.jsHash && ids.includes(p.id))
    .map(p => ({ id: p.id, jsHash: p.jsHash!, runtimeFingerprint: runtimeFingerprint(p), channelCapabilities: p.channelCapabilities || [], name: p.name, worldId }))

  return {
    apply(packages: RuntimePackage[], interim = false) {
      // Before a new context resolves, world scripts stop at once; platform
      // scripts keep running until the server says whether they still match.
      last = interim ? last.filter(p => p.scopeType === 'platform') : packages
      if (!interim) worldId = getWorldId()
      return evaluate()
    },
    reevaluate: evaluate,
    trust(ids: string[]) { trust.trust(subjects(ids)); return evaluate() },
    deny(ids: string[]) { trust.deny(subjects(ids)); return evaluate() },
    postpone(ids: string[]) {
      for (const s of subjects(ids)) postponed.add(key(s))
      return evaluate()
    },
    dispose() {
      last = []
      onPending([])
      return runtime.dispose()
    },
  }
}
