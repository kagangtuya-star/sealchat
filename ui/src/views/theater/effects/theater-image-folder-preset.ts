import {
  normalizeStageEntranceConfig,
  type StageEntranceConfig,
  type StageObject,
} from '../shared/stage-types'

export interface TheaterImageObjectPreset {
  version: 1
  width?: number
  height?: number
  visible?: boolean
  interactive?: boolean
  editable?: boolean
  locked?: boolean
  aspectRatioLocked?: boolean
  entrance?: StageEntranceConfig
}

export type TheaterImageFolderPreset = TheaterImageObjectPreset

const optionalDimension = (value: unknown) => (
  typeof value === 'number' && Number.isFinite(value) && value >= 0.5 && value <= 10_000
    ? value
    : undefined
)

export const normalizeTheaterImageObjectPreset = (input: unknown): TheaterImageObjectPreset | null => {
  if (!input || typeof input !== 'object') return null
  const value = input as Record<string, unknown>
  if (value.version !== 1) return null
  const preset: TheaterImageObjectPreset = { version: 1 }
  const width = optionalDimension(value.width)
  const height = optionalDimension(value.height)
  if (width !== undefined) preset.width = width
  if (height !== undefined) preset.height = height
  for (const key of ['visible', 'interactive', 'editable', 'locked', 'aspectRatioLocked'] as const) {
    const candidate = value[key]
    if (typeof candidate === 'boolean') preset[key] = candidate
  }
  if (value.entrance && typeof value.entrance === 'object') {
    preset.entrance = normalizeStageEntranceConfig(value.entrance)
  }
  return preset
}

export const resolveImageObjectPreset = (
  folderInput: unknown,
  assetInput: unknown,
): TheaterImageObjectPreset | undefined => {
  const folderPreset = normalizeTheaterImageObjectPreset(folderInput)
  const assetPreset = normalizeTheaterImageObjectPreset(assetInput)
  if (!folderPreset && !assetPreset) return undefined
  return {
    ...(folderPreset || {}),
    ...(assetPreset || {}),
    version: 1,
  }
}

export const applyImageObjectPreset = (object: StageObject, input: unknown): StageObject => {
  const preset = normalizeTheaterImageObjectPreset(input)
  if (!preset) return object
  if (preset.width !== undefined) object.transform.width = preset.width
  if (preset.height !== undefined) object.transform.height = preset.height
  for (const key of ['visible', 'interactive', 'editable', 'locked', 'aspectRatioLocked'] as const) {
    if (preset[key] !== undefined) object[key] = preset[key]
  }
  if (preset.entrance) {
    object.metadata = { ...object.metadata, entrance: normalizeStageEntranceConfig(preset.entrance) }
  }
  return object
}

export const normalizeTheaterImageFolderPreset = normalizeTheaterImageObjectPreset
export const applyImageFolderPreset = applyImageObjectPreset
