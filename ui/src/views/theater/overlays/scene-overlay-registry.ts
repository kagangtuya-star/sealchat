import type { StageSceneOverlayBinding } from '../shared/stage-types'
import type { SceneOverlayEffectDefinition, SceneOverlayRenderer } from './scene-overlay-types'

const effectDefinitions = new Map<string, SceneOverlayEffectDefinition>()
const renderers = new Map<string, SceneOverlayRenderer>()
const effectRegistrationIssues: string[] = []

const createId = () => {
  const value = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `scene-overlay-${value}`
}

export const registerSceneOverlayEffect = (definition: SceneOverlayEffectDefinition) => {
  if (effectDefinitions.has(definition.id)) {
    const message = `Duplicate scene overlay effect: ${definition.id}`
    effectRegistrationIssues.push(message)
    if (import.meta.env.DEV) throw new Error(message)
    return
  }
  effectDefinitions.set(definition.id, definition)
}

export const getSceneOverlayEffect = (id: string) => effectDefinitions.get(id)

export const listSceneOverlayEffects = () => [...effectDefinitions.values()]

export const validateSceneOverlayEffectRegistry = () => {
  const issues: string[] = [...effectRegistrationIssues]
  const ids = new Set<string>()
  for (const definition of effectDefinitions.values()) {
    if (!definition.id.trim()) issues.push('Scene overlay effect id cannot be empty')
    if (ids.has(definition.id)) issues.push(`Duplicate scene overlay effect: ${definition.id}`)
    ids.add(definition.id)
  }
  return issues
}

export const createSceneOverlayBinding = (effectId: string): StageSceneOverlayBinding => {
  const definition = getSceneOverlayEffect(effectId)
  if (!definition) throw new Error(`Unknown scene overlay effect: ${effectId}`)
  return {
    version: 1,
    id: createId(),
    effectId: definition.id,
    name: definition.name,
    enabled: true,
    opacity: typeof definition.defaultOpacity === 'number'
      ? Math.min(1, Math.max(0, definition.defaultOpacity))
      : 1,
    blendMode: definition.defaultBlendMode || 'normal',
    layer: definition.defaultLayer || 'aboveCharacters',
    params: structuredClone(definition.defaultParams),
  }
}

export const registerSceneOverlayRenderer = (renderer: SceneOverlayRenderer) => {
  if (renderers.has(renderer.type)) {
    if (import.meta.env.DEV) throw new Error(`Duplicate scene overlay renderer: ${renderer.type}`)
    return
  }
  renderers.set(renderer.type, renderer)
}

export const getSceneOverlayRenderer = (type: string) => renderers.get(type)
