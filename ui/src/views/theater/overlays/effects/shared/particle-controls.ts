import type { SceneOverlayControl } from '../../scene-overlay-types'

type NumericControl = Omit<Extract<SceneOverlayControl, { type: 'number' }>, 'type' | 'key' | 'label'>
type AngleControl = Omit<Extract<SceneOverlayControl, { type: 'angle' }>, 'type' | 'key' | 'label'>

export interface ParticleMotionControlOverrides {
  intensity?: false | Partial<NumericControl>
  speed?: false | Partial<NumericControl>
  windDirection?: false | Partial<AngleControl>
  windStrength?: false | Partial<NumericControl>
  spread?: false | Partial<NumericControl>
}

export const particleMotionControls = (
  overrides: ParticleMotionControlOverrides = {},
): SceneOverlayControl[] => {
  const controls: SceneOverlayControl[] = []
  if (overrides.intensity !== false) controls.push({ type: 'number', key: 'intensity', label: '强度', min: 0, max: 1, step: 0.05, ...overrides.intensity })
  if (overrides.speed !== false) controls.push({ type: 'number', key: 'speed', label: '速度', min: 0.1, max: 3, step: 0.1, ...overrides.speed })
  if (overrides.windDirection !== false) controls.push({ type: 'angle', key: 'windDirection', label: '风向', min: 0, max: 360, step: 5, ...overrides.windDirection })
  if (overrides.windStrength !== false) controls.push({ type: 'number', key: 'windStrength', label: '风力', min: 0, max: 1, step: 0.05, ...overrides.windStrength })
  if (overrides.spread !== false) controls.push({ type: 'number', key: 'spread', label: '飘散', min: 0, max: 1, step: 0.05, ...overrides.spread })
  return controls
}
