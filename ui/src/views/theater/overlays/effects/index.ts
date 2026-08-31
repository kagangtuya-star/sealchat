import { registerSceneOverlayEffect } from '../scene-overlay-registry'
import { colorWashEffect } from './color-wash'
import { duskEffect } from './dusk'
import { dustEffect } from './dust'
import { embersEffect } from './embers'
import { firefliesEffect } from './fireflies'
import { fogEffect } from './fog'
import { lightningEffect } from './lightning'
import { nightEffect } from './night'
import { rainHeavyEffect } from './rain-heavy'
import { rainLightEffect } from './rain-light'
import { sandstormEffect } from './sandstorm'
import { snowEffect } from './snow'

const builtInSceneOverlayEffects = [
  rainLightEffect,
  rainHeavyEffect,
  snowEffect,
  fogEffect,
  dustEffect,
  embersEffect,
  firefliesEffect,
  sandstormEffect,
  duskEffect,
  nightEffect,
  lightningEffect,
  colorWashEffect,
]

let registered = false

export const registerBuiltInSceneOverlayEffects = () => {
  if (registered) return
  registered = true
  builtInSceneOverlayEffects.forEach(registerSceneOverlayEffect)
}
