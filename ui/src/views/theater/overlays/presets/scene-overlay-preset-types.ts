import type {
  StageSceneOverlayBlendMode,
  StageSceneOverlayLayer,
  StageSceneOverlayParams,
} from '../../shared/stage-types'

export const sceneOverlayPresetCategories = [
  'city', 'indoor', 'dungeon', 'cave', 'forest', 'swamp', 'snow', 'desert', 'ocean', 'volcanic',
  'battlefield', 'graveyard', 'temple', 'magic', 'horror', 'dream', 'planar', 'modern', 'cyberpunk', 'eastern',
] as const

export type SceneOverlayPresetCategory = typeof sceneOverlayPresetCategories[number]
export type SceneOverlayPresetApplyMode = 'append' | 'replace'

export const sceneOverlayPresetCategoryLabels: Record<SceneOverlayPresetCategory, string> = {
  city: '城镇', indoor: '室内', dungeon: '地牢', cave: '洞穴', forest: '森林', swamp: '沼泽', snow: '雪地', desert: '沙漠', ocean: '海洋', volcanic: '火山',
  battlefield: '战场', graveyard: '墓地', temple: '神殿', magic: '魔法', horror: '恐怖', dream: '梦境', planar: '位面', modern: '现代', cyberpunk: '赛博', eastern: '东方',
}

export interface SceneOverlayPresetItem {
  effectId: string
  name?: string
  enabled?: boolean
  opacity?: number
  blendMode?: StageSceneOverlayBlendMode
  layer?: StageSceneOverlayLayer
  params?: StageSceneOverlayParams
}

export interface SceneOverlayPresetDefinition {
  id: string
  name: string
  description: string
  category: SceneOverlayPresetCategory
  tags?: string[]
  overlays: SceneOverlayPresetItem[]
}
