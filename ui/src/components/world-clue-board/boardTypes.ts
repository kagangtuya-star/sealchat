import type { WorldClueSummary } from '@/stores/worldClue'
import type { WorldClueBoardDocument, WorldClueBoardPlacement, WorldClueBoardRelation, WorldClueBoardRelationEndpointRef, WorldClueBoardRelationKind } from '@/stores/worldClueBoard'
import type { SealChatCamera } from './quickdraw-camera-adapter'

export type BoardCamera = SealChatCamera

export interface BoardNodeLayout {
  id: string
  summary: WorldClueSummary
  x: number
  y: number
  width: number
  height: number
  pinned: boolean
  temporary: boolean
}

export type BoardPlacementPatch = { id: string; placement: WorldClueBoardPlacement }

export interface BoardElementGeometry {
  id: string
  x: number
  y: number
  width: number
  height: number
}

export interface BoardDrawingEndpoint extends BoardElementGeometry {
  name: string
  quickdrawKind: 'sticky' | 'text'
  selected: boolean
}

export interface BoardRelationEndpoint {
  relation: WorldClueBoardRelation
  source: BoardElementGeometry
  target: BoardElementGeometry
}

export interface BoardConnectionPreview {
  source: WorldClueBoardRelationEndpointRef
  kind: WorldClueBoardRelationKind
  cursor: { x: number; y: number } | null
}

export type BoardDocumentUpdater = (mutator: (document: WorldClueBoardDocument) => void) => boolean
