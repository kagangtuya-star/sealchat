import { registerSceneOverlayRenderer } from '../scene-overlay-registry'
import { colorSceneOverlayRenderer } from './color-renderer'
import { particlesSceneOverlayRenderer } from './particles-renderer'

let registered = false

export const registerBuiltInSceneOverlayRenderers = () => {
  if (registered) return
  registered = true
  registerSceneOverlayRenderer(particlesSceneOverlayRenderer)
  registerSceneOverlayRenderer(colorSceneOverlayRenderer)
}
