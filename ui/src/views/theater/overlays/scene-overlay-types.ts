import type {
  StageSceneOverlayBinding,
  StageSceneOverlayBlendMode,
  StageSceneOverlayLayer,
  StageSceneOverlayParams,
} from '../shared/stage-types'

export type SceneOverlayCategory = 'weather' | 'environment' | 'lighting' | 'special'

export type SceneOverlayControl =
  | {
      type: 'number'
      key: string
      label: string
      min: number
      max: number
      step: number
      suffix?: string
    }
  | {
      type: 'color'
      key: string
      label: string
    }
  | {
      type: 'boolean'
      key: string
      label: string
    }
  | {
      type: 'select'
      key: string
      label: string
      options: Array<{ label: string, value: string }>
    }

export interface SceneOverlayBuildContext {
  reducedMotion: boolean
}

export interface SceneOverlayRenderDescriptor {
  renderer: string
  config: unknown
}

export interface SceneOverlayEffectDefinition {
  id: string
  name: string
  description?: string
  category: SceneOverlayCategory
  defaultParams: StageSceneOverlayParams
  controls: SceneOverlayControl[]
  defaultOpacity?: number
  defaultBlendMode?: StageSceneOverlayBlendMode
  defaultLayer?: StageSceneOverlayLayer
  buildRenderDescriptor(
    params: StageSceneOverlayParams,
    context: SceneOverlayBuildContext,
  ): SceneOverlayRenderDescriptor
}

export interface SceneOverlayRenderContext extends SceneOverlayBuildContext {
  binding: StageSceneOverlayBinding
}

export interface SceneOverlayRendererHandle {
  update?(config: unknown): void | Promise<void>
  destroy(): void | Promise<void>
}

export interface SceneOverlayRenderer {
  type: string
  mount(
    host: HTMLElement,
    config: unknown,
    context: SceneOverlayRenderContext,
  ): SceneOverlayRendererHandle | Promise<SceneOverlayRendererHandle>
}
