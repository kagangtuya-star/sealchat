import type { SceneOverlayEffectDefinition } from '../../scene-overlay-types'
import { numberParam } from '../effect-helpers'

export default { id: 'special.blackout', name: '瞬间黑暗', description: '周期性吞没画面的黑暗脉冲。', category: 'special', defaultParams: { strength: 0.92, frequency: 0.12 }, controls: [{ type: 'number', key: 'strength', label: '强度', min: 0.05, max: 1, step: 0.05 }, { type: 'number', key: 'frequency', label: '频率', min: 0.03, max: 1, step: 0.01, suffix: '次/秒' }], defaultBlendMode: 'multiply', buildRenderDescriptor(params) { return { renderer: 'color', config: { mode: 'pulse', color: '#000000', strength: numberParam(params, 'strength', 0.92, 0.05, 1), frequency: numberParam(params, 'frequency', 0.12, 0.03, 1) } } } } satisfies SceneOverlayEffectDefinition
