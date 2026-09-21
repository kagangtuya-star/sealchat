import { shallowRef } from 'vue'
import { api } from '@/stores/_config'
import type { TheaterPresentation, TheaterVisualStyle } from '@/types/theaterPresentation'
import type { DialogueInactiveStyle } from './theater-dialogue-residency'
import type { DialoguePosition } from './theater-dialogue-layout'

export interface DialogueController {
  version: number
  sharedIdentityId: string
  enabled: boolean
  maxPortraits: number
  inactiveStyle: DialogueInactiveStyle
  allowList: string[]
  denyList: string[]
  dragEnabled: boolean
  positions: Record<string, DialoguePosition>
}
export interface DialogueControllerTemplate { presentation: TheaterPresentation; portraitStyle: TheaterVisualStyle }
export type DialogueControllerPatch = Partial<Pick<DialogueController, 'enabled' | 'maxPortraits' | 'inactiveStyle' | 'allowList' | 'denyList' | 'dragEnabled'>> & { template?: DialogueControllerTemplate }
export interface DialogueControllerState {
  revision: number
  controller: DialogueController
  template: DialogueControllerTemplate | null
  canManage?: boolean
  canDrag?: boolean
}
export const defaultDialogueController = (): DialogueController => ({ version: 1, sharedIdentityId: '', enabled: false, maxPortraits: 4, inactiveStyle: 'dim', allowList: [], denyList: [], dragEnabled: false, positions: {} })

// Confirmed room state only. The queue and gesture previews never enter this cache.
export const dialogueControllerStates = shallowRef<Record<string, DialogueControllerState>>({})
export const publishDialogueController = (worldId: string, state: DialogueControllerState) => {
  const previous = dialogueControllerStates.value[worldId]
  if (previous && previous.revision > state.revision) return
  dialogueControllerStates.value = { ...dialogueControllerStates.value, [worldId]: { ...previous, ...state } }
}
export const readDialogueController = async (worldId: string) => {
  const { data } = await api.get<DialogueControllerState>(`api/v1/worlds/${encodeURIComponent(worldId)}/theater/dialogue-controller`)
  publishDialogueController(worldId, data)
  return data
}
type ControllerWriter = (patch: DialogueControllerPatch) => Promise<void>
const writers = new Map<string, ControllerWriter>()
export const registerDialogueControllerWriter = (worldId: string, writer: ControllerWriter) => {
  writers.set(worldId, writer)
  return () => { if (writers.get(worldId) === writer) writers.delete(worldId) }
}
export const writeDialogueController = async (worldId: string, patch: DialogueControllerPatch) => {
  const writer = writers.get(worldId)
  if (writer) return writer(patch)
  if (window.parent !== window && new URLSearchParams(window.location.hash.split('?')[1] || '').get('mode') === 'theater') {
    const requestId = crypto.randomUUID()
    return new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => { cleanup(); reject(new Error('公共模板保存确认超时')) }, 20_000)
      const cleanup = () => { clearTimeout(timer); window.removeEventListener('message', receive) }
      const receive = (event: MessageEvent) => {
        if (event.origin !== window.location.origin || event.source !== window.parent || event.data?.type !== 'sealchat.theater.dialogue-controller.result' || event.data.requestId !== requestId) return
        cleanup()
        if (event.data.ok) { if (event.data.state) publishDialogueController(worldId, event.data.state); resolve() }
        else reject(new Error(event.data.error || '保存失败'))
      }
      window.addEventListener('message', receive)
      window.parent.postMessage({ type: 'sealchat.theater.dialogue-controller.patch', requestId, worldId, patch }, window.location.origin)
    })
  }
  // A management panel outside the theater has no stage writer. Retry only the
  // requested fields against a freshly read revision, never a settings snapshot.
  for (let attempt = 0; attempt < 3; attempt++) {
    const state = await readDialogueController(worldId)
    try {
      await api.post(`api/v1/worlds/${encodeURIComponent(worldId)}/theater/mutations`, {
        mutationId: crypto.randomUUID(), worldId, channelId: '', expectedRevision: state.revision, type: 'room.dialogue.patch', payload: patch,
      })
      await readDialogueController(worldId)
      return
    } catch (error) {
      if ((error as { response?: { data?: { error?: { code?: string } } } }).response?.data?.error?.code !== 'STAGE_REVISION_CONFLICT' || attempt === 2) throw error
    }
  }
}

export interface DialogueCharacterOption { actorKey: string; identityId: string; sharedIdentityId?: string; sourceChannelId: string; displayName: string; channelName: string }
export interface DialogueCharacterPage { items: DialogueCharacterOption[]; total: number; page: number; pageSize: number }
const pages = new Map<string, Promise<DialogueCharacterPage>>()
const pageTimes = new Map<string, number>()
export const readDialogueCharacters = (worldId: string, keyword: string, page: number) => {
  const key = JSON.stringify([worldId, keyword, page])
  let request = Date.now() - (pageTimes.get(key) || 0) < 30_000 ? pages.get(key) : undefined
  if (!request) {
    request = api.get<DialogueCharacterPage>(`api/v1/worlds/${encodeURIComponent(worldId)}/theater/character-options`, { params: { keyword, page, pageSize: 30 } }).then(result => result.data)
    pages.set(key, request)
    pageTimes.set(key, Date.now())
    void request.catch(() => { pages.delete(key); pageTimes.delete(key) })
    if (pages.size > 100) {
      const oldest = pages.keys().next().value || ''
      pages.delete(oldest); pageTimes.delete(oldest)
    }
  }
  return request
}
