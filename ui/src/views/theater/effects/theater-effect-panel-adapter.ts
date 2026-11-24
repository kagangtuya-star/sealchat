import type { StageObject } from '../shared/stage-types'
import type { TheaterEffectKind } from './theater-effect-types'

export interface TheaterEffectPanelAdapter {
  add(kind: TheaterEffectKind): void
  edit(label: string, ids: string[], update: (object: StageObject) => void): void
  select(id: string): void
  remove(ids: string[]): void
  preview(object: StageObject): void
  move(id: string, direction: 1 | -1): void
  scene?: {
    isFixed(id: string): boolean
    setFixed(id: string, value: boolean): void
    setVisible(id: string, value: boolean): void
    beginEdit(label: string): void
    commitEdit(): void
  }
}
