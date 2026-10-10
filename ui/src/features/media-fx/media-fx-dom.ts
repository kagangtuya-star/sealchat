import type { Directive } from 'vue'

import {
  mediaFxFilterToCss,
  normalizeMediaFxSpec,
  prefersReducedMotion,
  resolveMediaFxMotionTrack,
  type MediaFxSpec,
} from './media-fx'

// DOM adapter: static filters via CSS `filter`, motion via the Web Animations API.
//
// Motion writes `transform` through WAAPI on `motionTarget`, so the target must be a
// wrapper owned by Media FX, never an element that already carries a business
// transform. Filter is written to `filterTarget.style.filter`; keep that element free
// of string `:style` bindings, which would replace the whole inline style.

export interface DomMediaFxTargets {
  motionTarget: HTMLElement
  filterTarget?: HTMLElement | null
}

export interface DomMediaFxOptions {
  // Explicit override; when omitted the controller follows prefers-reduced-motion.
  reducedMotion?: boolean
  motion?: boolean
  filters?: boolean
  paused?: boolean
  blurScale?: number
}

export interface DomMediaFxController {
  update(spec: unknown, options?: DomMediaFxOptions): void
  clear(): void
  dispose(): void
}

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

const motionSignature = (spec: MediaFxSpec) => {
  const motion = spec.motion
  return `${motion.preset}|${motion.intensity}|${motion.durationMs}|${motion.loop}`
}

export const createDomMediaFxController = (input: HTMLElement | DomMediaFxTargets): DomMediaFxController => {
  const motionTarget = input instanceof HTMLElement ? input : input.motionTarget
  const filterTarget = input instanceof HTMLElement ? input : (input.filterTarget || input.motionTarget)
  let animation: Animation | null = null
  let activeMotionSignature = ''
  let appliedFilter = ''
  let lastSpec: MediaFxSpec | null = null
  let lastOptions: DomMediaFxOptions = {}
  let disposed = false
  const mediaQuery = typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia(REDUCED_MOTION_QUERY)
    : null

  const cancelMotion = () => {
    animation?.cancel()
    animation = null
    activeMotionSignature = ''
  }

  const applyFilter = (css: string) => {
    if (css === appliedFilter) return
    appliedFilter = css
    if (css) filterTarget.style.filter = css
    else filterTarget.style.removeProperty('filter')
  }

  const applyMotion = (spec: MediaFxSpec, options: DomMediaFxOptions) => {
    const reduced = options.reducedMotion ?? prefersReducedMotion()
    const track = options.motion === false || options.paused || reduced ? null : resolveMediaFxMotionTrack(spec.motion)
    if (!track || typeof motionTarget.animate !== 'function') {
      cancelMotion()
      return
    }
    const signature = motionSignature(spec)
    // Same spec: keep the running loop, and do not replay a finished one-shot motion.
    if (signature === activeMotionSignature) return
    cancelMotion()
    activeMotionSignature = signature
    const current = motionTarget.animate(
      track.frames.map((frame) => ({
        offset: frame.offset,
        transform: `translate(${frame.x * 100}%, ${frame.y * 100}%) scale(${frame.scale})`,
        easing: track.easing,
      })),
      {
        duration: track.durationMs,
        iterations: track.loop ? Number.POSITIVE_INFINITY : 1,
        fill: 'none',
      },
    )
    current.onfinish = () => {
      if (animation === current) animation = null
    }
    animation = current
  }

  const handleReducedMotionChange = () => {
    if (!disposed && lastSpec && lastOptions.reducedMotion === undefined) applyMotion(lastSpec, lastOptions)
  }
  mediaQuery?.addEventListener?.('change', handleReducedMotionChange)

  const clear = () => {
    cancelMotion()
    applyFilter('')
    lastSpec = null
  }

  return {
    update(input, options = {}) {
      if (disposed) return
      const spec = normalizeMediaFxSpec(input)
      lastSpec = spec
      lastOptions = { ...options }
      // Reduced motion only stops motion; static filters stay visible.
      applyFilter(options.filters === false ? '' : mediaFxFilterToCss(spec.filter, { blurScale: options.blurScale }))
      applyMotion(spec, options)
    },
    clear,
    dispose() {
      if (disposed) return
      clear()
      disposed = true
      mediaQuery?.removeEventListener?.('change', handleReducedMotionChange)
    },
  }
}

// Vue binding for simple wrappers: <div v-media-fx="{ spec, filters, motion }">.
export interface MediaFxDirectiveValue extends DomMediaFxOptions {
  spec: unknown
}

const directiveControllers = new WeakMap<HTMLElement, DomMediaFxController>()

export const vMediaFx: Directive<HTMLElement, MediaFxDirectiveValue | null | undefined> = {
  mounted(element, binding) {
    const controller = createDomMediaFxController(element)
    directiveControllers.set(element, controller)
    const { spec, ...options } = binding.value || { spec: null }
    controller.update(spec, options)
  },
  updated(element, binding) {
    const { spec, ...options } = binding.value || { spec: null }
    directiveControllers.get(element)?.update(spec, options)
  },
  beforeUnmount(element) {
    directiveControllers.get(element)?.dispose()
    directiveControllers.delete(element)
  },
}
