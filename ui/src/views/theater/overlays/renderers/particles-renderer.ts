import type { Container, Engine, ISourceOptions } from '@tsparticles/engine'

import type { SceneOverlayRenderer } from '../scene-overlay-types'

let enginePromise: Promise<Engine> | null = null

const loadEngine = () => {
  if (!enginePromise) {
    enginePromise = Promise.all([
      import('@tsparticles/engine'),
      import('@tsparticles/slim'),
    ]).then(async ([engineModule, slimModule]) => {
      await slimModule.loadSlim(engineModule.tsParticles)
      return engineModule.tsParticles
    })
  }
  return enginePromise
}

const particleOptions = (config: unknown) => config && typeof config === 'object'
  ? config as ISourceOptions
  : {}

export const particlesSceneOverlayRenderer: SceneOverlayRenderer = {
  type: 'particles',
  async mount(host, config, context) {
    host.style.width = '100%'
    host.style.height = '100%'
    host.style.pointerEvents = 'none'
    const engine = await loadEngine()
    let container: Container | undefined = await engine.load({
      element: host,
      options: particleOptions(config),
    })
    if (context.reducedMotion) container?.pause()
    return {
      async update(nextConfig) {
        if (!container) return
        await container.reset(particleOptions(nextConfig))
        if (context.reducedMotion) container.pause()
      },
      destroy() {
        container?.destroy()
        container = undefined
      },
    }
  },
}
