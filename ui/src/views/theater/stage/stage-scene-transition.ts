import type { StageSceneTransitionPhase } from '../shared/stage-types'

export type StageSceneTransitionDirection = 'enter' | 'exit'

const endpoints = (direction: StageSceneTransitionDirection, hidden: Keyframe, visible: Keyframe) => (
  direction === 'enter' ? [hidden, visible] : [visible, hidden]
)

export const stageSceneTransitionKeyframes = (
  phase: StageSceneTransitionPhase,
  direction: StageSceneTransitionDirection,
): Keyframe[] => {
  switch (phase.type) {
    case 'none':
      return []
    case 'fade':
      return endpoints(direction, { opacity: 0 }, { opacity: 1 })
    case 'slide':
      return direction === 'enter'
        ? [{ opacity: 0, transform: 'translate3d(12%, 0, 0)' }, { opacity: 1, transform: 'translate3d(0, 0, 0)' }]
        : [{ opacity: 1, transform: 'translate3d(0, 0, 0)' }, { opacity: 0, transform: 'translate3d(-12%, 0, 0)' }]
    case 'dissolve':
      return endpoints(direction,
        { opacity: 0, filter: 'contrast(180%) brightness(160%) blur(8px)' },
        { opacity: 1, filter: 'contrast(100%) brightness(100%) blur(0px)' })
    case 'zoom':
      return endpoints(direction,
        { opacity: 0, transform: direction === 'enter' ? 'scale(1.08)' : 'scale(0.92)' },
        { opacity: 1, transform: 'scale(1)' })
    case 'mask':
      return endpoints(direction,
        { clipPath: 'circle(0% at 50% 50%)' },
        { clipPath: 'circle(150% at 50% 50%)' })
    case 'flip':
      return endpoints(direction,
        { opacity: 0, transform: `perspective(1200px) rotateY(${direction === 'enter' ? -90 : 90}deg) scale(0.96)` },
        { opacity: 1, transform: 'perspective(1200px) rotateY(0deg) scale(1)' })
    case 'blur':
      return endpoints(direction,
        { opacity: 0, filter: 'blur(24px)' },
        { opacity: 1, filter: 'blur(0px)' })
    case 'rotate':
      return endpoints(direction,
        { opacity: 0, transform: `rotate(${direction === 'enter' ? -8 : 8}deg) scale(${direction === 'enter' ? 1.08 : 0.92})` },
        { opacity: 1, transform: 'rotate(0deg) scale(1)' })
    case 'curtain':
      return []
  }
}

export const stageSceneTransitionOptions = (phase: StageSceneTransitionPhase): KeyframeAnimationOptions => ({
  duration: phase.durationMs,
  easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
  fill: 'both',
})
