import { registerBuiltInSceneOverlayEffects } from '../effects'
import { battlefieldSceneOverlayPresets } from './battlefield'
import { caveSceneOverlayPresets } from './cave'
import { citySceneOverlayPresets } from './city'
import { cyberpunkSceneOverlayPresets } from './cyberpunk'
import { desertSceneOverlayPresets } from './desert'
import { dreamSceneOverlayPresets } from './dream'
import { dungeonSceneOverlayPresets } from './dungeon'
import { easternSceneOverlayPresets } from './eastern'
import { forestSceneOverlayPresets } from './forest'
import { graveyardSceneOverlayPresets } from './graveyard'
import { horrorSceneOverlayPresets } from './horror'
import { indoorSceneOverlayPresets } from './indoor'
import { magicSceneOverlayPresets } from './magic'
import { modernSceneOverlayPresets } from './modern'
import { oceanSceneOverlayPresets } from './ocean'
import { planarSceneOverlayPresets } from './planar'
import { snowSceneOverlayPresets } from './snow'
import { swampSceneOverlayPresets } from './swamp'
import { templeSceneOverlayPresets } from './temple'
import { volcanicSceneOverlayPresets } from './volcanic'
import {
  registerSceneOverlayPreset,
  validateSceneOverlayPresetRegistry,
} from './scene-overlay-preset-registry'

export * from './scene-overlay-preset-registry'
export * from './scene-overlay-preset-types'

export const builtInSceneOverlayPresets = [
  ...citySceneOverlayPresets, ...indoorSceneOverlayPresets, ...dungeonSceneOverlayPresets, ...caveSceneOverlayPresets,
  ...forestSceneOverlayPresets, ...swampSceneOverlayPresets, ...snowSceneOverlayPresets, ...desertSceneOverlayPresets,
  ...oceanSceneOverlayPresets, ...volcanicSceneOverlayPresets, ...battlefieldSceneOverlayPresets, ...graveyardSceneOverlayPresets,
  ...templeSceneOverlayPresets, ...magicSceneOverlayPresets, ...horrorSceneOverlayPresets, ...dreamSceneOverlayPresets,
  ...planarSceneOverlayPresets, ...modernSceneOverlayPresets, ...cyberpunkSceneOverlayPresets, ...easternSceneOverlayPresets,
]

let registered = false

export const registerBuiltInSceneOverlayPresets = () => {
  if (registered) return
  registered = true
  registerBuiltInSceneOverlayEffects()
  builtInSceneOverlayPresets.forEach(registerSceneOverlayPreset)
  const issues = validateSceneOverlayPresetRegistry()
  if (!issues.length) return
  if (import.meta.env.DEV) throw new Error(issues.join('\n'))
  console.error('Invalid scene overlay preset registry', issues)
}
