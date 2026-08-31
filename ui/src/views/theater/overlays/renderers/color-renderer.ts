import type { SceneOverlayRenderer } from '../scene-overlay-types'

interface ColorRendererConfig {
  mode?: 'solid' | 'fog' | 'lightning' | 'pulse'
  color?: string
  secondaryColor?: string
  strength?: number
  frequency?: number
  durationMs?: number
}

const colorPattern = /^#[0-9a-f]{6}$/i
const safeColor = (value: unknown, fallback: string) => (
  typeof value === 'string' && colorPattern.test(value) ? value : fallback
)
const finiteRange = (value: unknown, fallback: number, minimum: number, maximum: number) => (
  typeof value === 'number' && Number.isFinite(value)
    ? Math.min(maximum, Math.max(minimum, value))
    : fallback
)

export const colorSceneOverlayRenderer: SceneOverlayRenderer = {
  type: 'color',
  mount(host, config, context) {
    const layer = document.createElement('div')
    layer.style.position = 'absolute'
    layer.style.inset = '0'
    layer.style.pointerEvents = 'none'
    host.append(layer)

    const animations = new Set<Animation>()
    let timer: ReturnType<typeof setTimeout> | null = null
    let destroyed = false

    const cleanupMotion = () => {
      animations.forEach((animation) => animation.cancel())
      animations.clear()
      if (timer !== null) clearTimeout(timer)
      timer = null
    }

    const trackAnimation = (animation: Animation) => {
      animations.add(animation)
      void animation.finished.catch(() => undefined).finally(() => animations.delete(animation))
      return animation
    }

    const apply = (input: unknown) => {
      cleanupMotion()
      const value = input && typeof input === 'object' ? input as ColorRendererConfig : {}
      const mode = value.mode === 'fog' || value.mode === 'lightning' || value.mode === 'pulse' ? value.mode : 'solid'
      const color = safeColor(value.color, '#000000')
      const secondaryColor = safeColor(value.secondaryColor, color)
      layer.style.opacity = String(finiteRange(value.strength, 1, 0, 1))
      layer.style.transform = 'none'
      layer.style.background = mode === 'fog'
        ? `linear-gradient(105deg, ${color} 0%, ${secondaryColor} 48%, ${color} 100%)`
        : secondaryColor !== color
          ? `linear-gradient(180deg, ${color}, ${secondaryColor})`
          : color

      if (mode === 'fog' && !context.reducedMotion) {
        trackAnimation(layer.animate([
          { transform: 'translateX(-4%) scale(1.1)' },
          { transform: 'translateX(4%) scale(1.16)' },
          { transform: 'translateX(-4%) scale(1.1)' },
        ], {
          duration: finiteRange(value.durationMs, 18_000, 4_000, 60_000),
          iterations: Infinity,
          easing: 'ease-in-out',
        }))
      }

      if (mode === 'lightning') {
        layer.style.opacity = '0'
        if (context.reducedMotion) return
        const strength = finiteRange(value.strength, 0.85, 0.05, 1)
        const frequency = finiteRange(value.frequency, 0.2, 0.03, 2)
        const schedule = () => {
          if (destroyed) return
          const delay = 1_000 / frequency * (0.65 + Math.random() * 0.7)
          timer = setTimeout(() => {
            if (destroyed) return
            trackAnimation(layer.animate([
              { opacity: 0 },
              { opacity: strength, offset: 0.16 },
              { opacity: strength * 0.12, offset: 0.38 },
              { opacity: strength * 0.72, offset: 0.55 },
              { opacity: 0 },
            ], { duration: 420, easing: 'linear' }))
            schedule()
          }, delay)
        }
        schedule()
      }

      if (mode === 'pulse') {
        if (context.reducedMotion) return
        const strength = finiteRange(value.strength, 0.65, 0.05, 1)
        const frequency = finiteRange(value.frequency, 0.3, 0.03, 2)
        trackAnimation(layer.animate([
          { opacity: strength * 0.12 },
          { opacity: strength },
          { opacity: strength * 0.12 },
        ], {
          duration: 1_000 / frequency,
          iterations: Infinity,
          easing: 'ease-in-out',
        }))
      }
    }

    apply(config)
    return {
      update: apply,
      destroy() {
        destroyed = true
        cleanupMotion()
        layer.remove()
      },
    }
  },
}
