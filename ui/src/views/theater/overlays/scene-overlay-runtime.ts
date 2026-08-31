import type { StageSceneOverlayBinding, StageSceneOverlayLayer } from '../shared/stage-types'
import { cloneStageData } from '../stage/stage-editing'
import { getSceneOverlayEffect, getSceneOverlayRenderer } from './scene-overlay-registry'
import type { SceneOverlayBuildContext, SceneOverlayRenderer, SceneOverlayRendererHandle } from './scene-overlay-types'

interface RuntimeInstance {
  binding: StageSceneOverlayBinding
  wrapper: HTMLDivElement
  rendererType: string
  descriptorSignature: string
  handle: SceneOverlayRendererHandle
}

interface SceneOverlayRuntimeOptions {
  belowCharactersHost: HTMLElement
  aboveCharactersHost: HTMLElement
  buildContext: () => SceneOverlayBuildContext
}

export class SceneOverlayRuntime {
  private readonly instances = new Map<string, RuntimeInstance>()
  private queue = Promise.resolve()
  private disposed = false
  private sceneKey = ''

  constructor(private readonly options: SceneOverlayRuntimeOptions) {}

  reconcile(bindings: StageSceneOverlayBinding[], sceneKey: string) {
    const snapshot = cloneStageData(bindings)
    this.queue = this.queue.then(() => this.reconcileNow(snapshot, sceneKey)).catch((error) => {
      if (import.meta.env.DEV) console.error('Scene overlay reconcile failed', error)
    })
  }

  destroy() {
    this.disposed = true
    this.queue = this.queue.then(async () => {
      await Promise.all([...this.instances.values()].map((instance) => this.destroyInstance(instance)))
      this.instances.clear()
    })
    return this.queue
  }

  private hostFor(layer: StageSceneOverlayLayer) {
    return layer === 'belowCharacters'
      ? this.options.belowCharactersHost
      : this.options.aboveCharactersHost
  }

  private async reconcileNow(bindings: StageSceneOverlayBinding[], sceneKey: string) {
    if (this.disposed) return
    if (this.sceneKey !== sceneKey) {
      await Promise.all([...this.instances.values()].map((instance) => this.destroyInstance(instance)))
      this.instances.clear()
      this.sceneKey = sceneKey
    }
    const desiredIds = new Set(bindings.filter((binding) => binding.enabled).map((binding) => binding.id))
    for (const [id, instance] of this.instances) {
      if (desiredIds.has(id)) continue
      await this.destroyInstance(instance)
      this.instances.delete(id)
    }

    const seen = new Set<string>()
    for (const binding of bindings) {
      if (!binding.enabled || seen.has(binding.id)) continue
      seen.add(binding.id)
      const definition = getSceneOverlayEffect(binding.effectId)
      if (!definition) {
        const existing = this.instances.get(binding.id)
        if (existing) {
          await this.destroyInstance(existing)
          this.instances.delete(binding.id)
        }
        continue
      }
      const context = this.options.buildContext()
      const descriptor = definition.buildRenderDescriptor(binding.params, context, binding)
      const renderer = getSceneOverlayRenderer(descriptor.renderer)
      if (!renderer) {
        const existing = this.instances.get(binding.id)
        if (existing) {
          await this.destroyInstance(existing)
          this.instances.delete(binding.id)
        }
        continue
      }
      const signature = JSON.stringify({
        effectId: binding.effectId,
        renderer: descriptor.renderer,
        config: descriptor.config,
        media: binding.media,
        reducedMotion: context.reducedMotion,
      })
      let instance = this.instances.get(binding.id)
      if (instance && (instance.binding.effectId !== binding.effectId || instance.rendererType !== descriptor.renderer)) {
        await this.destroyInstance(instance)
        this.instances.delete(binding.id)
        instance = undefined
      }
      if (!instance) {
        instance = await this.mountInstance(binding, renderer, descriptor.renderer, descriptor.config, signature, context)
        if (!instance) continue
        this.instances.set(binding.id, instance)
      } else if (instance.descriptorSignature !== signature) {
        if (instance.handle.update) {
          await instance.handle.update(descriptor.config)
          instance.descriptorSignature = signature
        } else {
          await this.destroyInstance(instance)
          this.instances.delete(binding.id)
          instance = await this.mountInstance(binding, renderer, descriptor.renderer, descriptor.config, signature, context)
          if (!instance) continue
          this.instances.set(binding.id, instance)
        }
      }
      instance.binding = cloneStageData(binding)
      this.applyWrapperStyle(instance.wrapper, binding)
      this.hostFor(binding.layer).append(instance.wrapper)
    }
  }

  private async mountInstance(
    binding: StageSceneOverlayBinding,
    renderer: SceneOverlayRenderer,
    rendererType: string,
    config: unknown,
    signature: string,
    context: SceneOverlayBuildContext,
  ): Promise<RuntimeInstance | undefined> {
    const wrapper = document.createElement('div')
    wrapper.className = 'scene-overlay-effect-host'
    this.applyWrapperStyle(wrapper, binding)
    this.hostFor(binding.layer).append(wrapper)
    try {
      const handle = await renderer.mount(wrapper, config, { ...context, binding })
      if (this.disposed) {
        await handle.destroy()
        wrapper.remove()
        return undefined
      }
      return {
        binding: cloneStageData(binding),
        wrapper,
        rendererType,
        descriptorSignature: signature,
        handle,
      }
    } catch (error) {
      wrapper.remove()
      if (import.meta.env.DEV) console.error(`Scene overlay mount failed: ${binding.effectId}`, error)
      return undefined
    }
  }

  private applyWrapperStyle(wrapper: HTMLElement, binding: StageSceneOverlayBinding) {
    wrapper.style.opacity = String(binding.opacity)
    wrapper.style.mixBlendMode = binding.blendMode
  }

  private async destroyInstance(instance: RuntimeInstance) {
    try {
      await instance.handle.destroy()
    } finally {
      instance.wrapper.remove()
    }
  }
}
