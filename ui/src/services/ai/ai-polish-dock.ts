export type AIPolishSlotStatus = 'idle' | 'loading' | 'success' | 'error'
export type AIPolishResultViewMode = 'edit' | 'diff'

export interface AIPolishSlotState {
  sourceText: string
  resultText: string
  status: AIPolishSlotStatus
  error: string
  requestId: string
  updatedAt: number
  viewMode: AIPolishResultViewMode
}

export interface AIPolishDockState {
  minimized: boolean
  activeSlotIndex: number
  slots: AIPolishSlotState[]
}

const SLOT_COUNT = 5

const createEmptySlot = (): AIPolishSlotState => ({
  sourceText: '',
  resultText: '',
  status: 'idle',
  error: '',
  requestId: '',
  updatedAt: 0,
  viewMode: 'edit',
})

const now = () => Date.now()
const createRequestId = () => `req-${now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`

export function createAIPolishDockState(): AIPolishDockState {
  return {
    minimized: false,
    activeSlotIndex: 0,
    slots: Array.from({ length: SLOT_COUNT }, () => createEmptySlot()),
  }
}

function resolveTargetSlotIndex(state: AIPolishDockState, preferredIndex?: number): number {
  const explicitIndex = Number.isInteger(preferredIndex) ? Number(preferredIndex) : -1
  if (explicitIndex >= 0 && explicitIndex < state.slots.length) {
    return explicitIndex
  }

  const activeSlot = state.slots[state.activeSlotIndex]
  if (activeSlot && activeSlot.status === 'idle') {
    return state.activeSlotIndex
  }

  const nextIdleIndex = state.slots.findIndex((slot) => slot.status === 'idle')
  return nextIdleIndex >= 0 ? nextIdleIndex : state.activeSlotIndex
}

export function findNextIdleAIPolishSlot(state: AIPolishDockState): number {
  return state.slots.findIndex((slot) => slot.status === 'idle')
}

export function prepareAIPolishTask(state: AIPolishDockState, sourceText: string, preferredIndex?: number) {
  const slotIndex = resolveTargetSlotIndex(state, preferredIndex)
  const requestId = createRequestId()
  const slot = state.slots[slotIndex]
  slot.sourceText = sourceText
  slot.resultText = ''
  slot.status = 'loading'
  slot.error = ''
  slot.requestId = requestId
  slot.updatedAt = now()
  state.activeSlotIndex = slotIndex
  state.minimized = false
  return { slotIndex, requestId }
}

export function finishAIPolishTaskSuccess(state: AIPolishDockState, slotIndex: number, requestId: string, resultText: string) {
  const slot = state.slots[slotIndex]
  if (!slot || slot.requestId !== requestId) {
    return
  }
  slot.resultText = resultText
  slot.status = 'success'
  slot.error = ''
  slot.updatedAt = now()
  slot.viewMode = 'diff'
}

export function finishAIPolishTaskError(state: AIPolishDockState, slotIndex: number, requestId: string, error: string) {
  const slot = state.slots[slotIndex]
  if (!slot || slot.requestId !== requestId) {
    return
  }
  slot.status = 'error'
  slot.error = error
  slot.updatedAt = now()
}

export function readCurrentInputIntoSlot(state: AIPolishDockState, slotIndex: number, sourceText: string) {
  const slot = state.slots[slotIndex]
  if (!slot) {
    return
  }
  slot.sourceText = sourceText
  slot.updatedAt = now()
}

export function setActiveAIPolishSlot(state: AIPolishDockState, slotIndex: number) {
  if (slotIndex < 0 || slotIndex >= state.slots.length) {
    return
  }
  state.activeSlotIndex = slotIndex
}

export function setAIPolishSlotViewMode(
  state: AIPolishDockState,
  slotIndex: number,
  viewMode: AIPolishResultViewMode,
) {
  const slot = state.slots[slotIndex]
  if (!slot) {
    return
  }
  slot.viewMode = viewMode
}

export function toggleAIPolishDockMinimized(state: AIPolishDockState, minimized?: boolean) {
  state.minimized = typeof minimized === 'boolean' ? minimized : !state.minimized
}

export function clearAIPolishSlot(state: AIPolishDockState, slotIndex: number) {
  if (slotIndex < 0 || slotIndex >= state.slots.length) {
    return
  }
  state.slots[slotIndex] = createEmptySlot()
}
