import {
  stageSceneOverlayBlendModes,
  stageSceneOverlayLayers,
  type StageSceneOverlayBinding,
  type StageSceneOverlayParamValue,
} from '../../shared/stage-types'
import {
  createSceneOverlayBinding,
  getSceneOverlayEffect,
} from '../scene-overlay-registry'
import {
  sceneOverlayPresetCategories,
  type SceneOverlayPresetDefinition,
} from './scene-overlay-preset-types'

const presetDefinitions = new Map<string, SceneOverlayPresetDefinition>()
const registrationIssues: string[] = []

const reportRuntimeError = (message: string) => {
  if (import.meta.env.DEV) throw new Error(message)
  console.error(message)
}

export const registerSceneOverlayPreset = (definition: SceneOverlayPresetDefinition) => {
  if (presetDefinitions.has(definition.id)) {
    const message = `Duplicate scene overlay preset: ${definition.id}`
    registrationIssues.push(message)
    if (import.meta.env.DEV) throw new Error(message)
    return
  }
  presetDefinitions.set(definition.id, definition)
}

export const getSceneOverlayPreset = (id: string) => presetDefinitions.get(id)
export const listSceneOverlayPresets = () => [...presetDefinitions.values()]

const isPrimitive = (value: unknown): value is StageSceneOverlayParamValue => (
  value === null || typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'
)

export const validateSceneOverlayPresetRegistry = () => {
  const issues = [...registrationIssues]
  const ids = new Set<string>()
  const categories = new Set<string>(sceneOverlayPresetCategories)
  for (const definition of presetDefinitions.values()) {
    if (!definition.id.trim()) issues.push('Scene overlay preset id cannot be empty')
    if (ids.has(definition.id)) issues.push(`Duplicate scene overlay preset: ${definition.id}`)
    ids.add(definition.id)
    if (!categories.has(definition.category)) issues.push(`Invalid preset category: ${definition.id} (${definition.category})`)
    if (!definition.overlays.length) issues.push(`Scene overlay preset is empty: ${definition.id}`)
    definition.overlays.forEach((item, index) => {
      const location = `${definition.id}[${index}]`
      if (!getSceneOverlayEffect(item.effectId)) issues.push(`Unknown scene overlay effect: ${location} (${item.effectId})`)
      if (item.opacity !== undefined && (!Number.isFinite(item.opacity) || item.opacity < 0 || item.opacity > 1)) issues.push(`Invalid opacity: ${location}`)
      if (item.blendMode !== undefined && !stageSceneOverlayBlendModes.includes(item.blendMode)) issues.push(`Invalid blend mode: ${location}`)
      if (item.layer !== undefined && !stageSceneOverlayLayers.includes(item.layer)) issues.push(`Invalid layer: ${location}`)
      if (item.params) {
        Object.entries(item.params).forEach(([key, value]) => {
          if (!isPrimitive(value) || (typeof value === 'number' && !Number.isFinite(value))) issues.push(`Invalid param: ${location}.${key}`)
        })
      }
    })
  }
  return issues
}

export const instantiateSceneOverlayPreset = (presetId: string): StageSceneOverlayBinding[] => {
  const preset = getSceneOverlayPreset(presetId)
  if (!preset) {
    reportRuntimeError(`Unknown scene overlay preset: ${presetId}`)
    return []
  }
  const bindings: StageSceneOverlayBinding[] = []
  for (const item of preset.overlays) {
    if (!getSceneOverlayEffect(item.effectId)) {
      reportRuntimeError(`Unknown scene overlay effect in preset ${presetId}: ${item.effectId}`)
      continue
    }
    const binding = createSceneOverlayBinding(item.effectId)
    bindings.push({
      ...binding,
      ...(item.name !== undefined ? { name: item.name } : {}),
      ...(item.enabled !== undefined ? { enabled: item.enabled } : {}),
      ...(item.opacity !== undefined ? { opacity: Math.min(1, Math.max(0, item.opacity)) } : {}),
      ...(item.blendMode !== undefined ? { blendMode: item.blendMode } : {}),
      ...(item.layer !== undefined ? { layer: item.layer } : {}),
      params: { ...binding.params, ...(item.params ? structuredClone(item.params) : {}) },
    })
  }
  return bindings
}
