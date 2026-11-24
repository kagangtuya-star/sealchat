import type { StageObjectTransform } from '../shared/stage-types'
import { THEATER_EFFECT_DESIGN_HEIGHT, THEATER_EFFECT_DESIGN_WIDTH } from './theater-effect-types'

// Editor-only placement helpers over the existing StageObject.transform; anchors are
// never persisted, they only compute a centre position inside the 1920×1080 design stage.
export type TheaterEffectAnchor =
  | 'top-left' | 'top' | 'top-right'
  | 'left' | 'center' | 'right'
  | 'bottom-left' | 'bottom' | 'bottom-right'

export const theaterEffectAnchors: TheaterEffectAnchor[] = [
  'top-left', 'top', 'top-right',
  'left', 'center', 'right',
  'bottom-left', 'bottom', 'bottom-right',
]

export const theaterEffectAnchorLabels: Record<TheaterEffectAnchor, string> = {
  'top-left': '左上',
  top: '上方',
  'top-right': '右上',
  left: '左侧',
  center: '居中',
  right: '右侧',
  'bottom-left': '左下',
  bottom: '下方',
  'bottom-right': '右下',
}

export const DEFAULT_THEATER_EFFECT_FRAME = { x: 960, y: 540, width: 1600, height: 900 }

// Rendered size of the frame: the overlay applies scaleX/scaleY around the centre.
const visualSize = (transform: StageObjectTransform) => ({
  width: transform.width * Math.abs(transform.scaleX || 1),
  height: transform.height * Math.abs(transform.scaleY || 1),
})

const axis = (anchor: TheaterEffectAnchor) => ({
  column: anchor.endsWith('left') ? 0 : anchor.endsWith('right') ? 2 : 1,
  row: anchor.startsWith('top') ? 0 : anchor.startsWith('bottom') ? 2 : 1,
})

const axisPosition = (index: number, size: number, stage: number) => (
  index === 0 ? size / 2 : index === 2 ? stage - size / 2 : stage / 2
)

export const theaterEffectAnchorPosition = (transform: StageObjectTransform, anchor: TheaterEffectAnchor) => {
  const size = visualSize(transform)
  const { column, row } = axis(anchor)
  return {
    x: Math.round(axisPosition(column, size.width, THEATER_EFFECT_DESIGN_WIDTH)),
    y: Math.round(axisPosition(row, size.height, THEATER_EFFECT_DESIGN_HEIGHT)),
  }
}

// A full-stage frame satisfies every anchor at once; report it as centred.
const anchorMatchOrder: TheaterEffectAnchor[] = ['center', ...theaterEffectAnchors.filter((anchor) => anchor !== 'center')]

export const matchTheaterEffectAnchor = (transform: StageObjectTransform): TheaterEffectAnchor | null => (
  anchorMatchOrder.find((anchor) => {
    const position = theaterEffectAnchorPosition(transform, anchor)
    return Math.abs(position.x - transform.x) < 1 && Math.abs(position.y - transform.y) < 1
  }) ?? null
)
