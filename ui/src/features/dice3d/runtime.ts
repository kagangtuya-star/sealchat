import type { DiceVisualPayload } from '@/types'
import { useDisplayStore } from '@/stores/display'

type Listener = (payload: DiceVisualPayload) => void
type ActivationListener = () => void

const listeners = new Set<Listener>()
const activationListeners = new Set<ActivationListener>()
const seenRollIds = new Map<string, number>()
const pendingPayloads: DiceVisualPayload[] = []
let loadRequested = false

const pruneSeen = () => {
  if (seenRollIds.size <= 500) return
  const entries = [...seenRollIds.entries()].sort((left, right) => left[1] - right[1])
  entries.slice(0, entries.length - 400).forEach(([rollId]) => seenRollIds.delete(rollId))
}

const isDice3DLocallyEnabled = () => {
  try {
    return useDisplayStore().settings.dice3dEnabled !== false
  } catch {
    return true
  }
}

export const dice3dRuntime = {
  subscribe(listener: Listener) {
    listeners.add(listener)
		if (pendingPayloads.length > 0) {
			const pending = pendingPayloads.splice(0)
			pending.forEach(payload => listener(payload))
		}
		return () => listeners.delete(listener)
	},
	subscribeActivation(listener: ActivationListener) {
		activationListeners.add(listener)
		if (loadRequested || pendingPayloads.length > 0) listener()
		return () => activationListeners.delete(listener)
	},
	requestLoad() {
		if (!isDice3DLocallyEnabled()) return
		loadRequested = true
		activationListeners.forEach(listener => listener())
  },
  play(payload?: DiceVisualPayload | null) {
    if (!isDice3DLocallyEnabled()) return
    if (!payload?.rollId || !payload.groups?.length || seenRollIds.has(payload.rollId)) return
    seenRollIds.set(payload.rollId, Date.now())
		loadRequested = true
    pruneSeen()
		if (listeners.size === 0) pendingPayloads.push(payload)
		activationListeners.forEach(listener => listener())
		listeners.forEach(listener => listener(payload))
  },
  forwardToTheater(payload: DiceVisualPayload) {
    if (!isDice3DLocallyEnabled()) return false
    if (window.parent === window) return false
    window.parent.postMessage({ type: 'sealchat:dice3d-roll', payload }, window.location.origin)
    return true
  },
}

export const isDice3DTheaterMessage = (event: MessageEvent) => (
  event.origin === window.location.origin
  && event.data?.type === 'sealchat:dice3d-roll'
  && event.data?.payload?.rollId
)
