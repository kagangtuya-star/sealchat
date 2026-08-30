export interface StageResizeBox {
  x: number
  y: number
  width: number
  height: number
  rotation: number
}

export interface StageGridResizeSession {
  anchor: string
  gridSize: number
  baseWidth: number
  baseHeight: number
  baseBoxWidth: number
  baseBoxHeight: number
  gridOriginX: number
  gridOriginY: number
  gridStepPx: number
  lockedWidthCells: number | null
  lockedHeightCells: number | null
}

const resizedAxes = (anchor: string) => ({
  width: anchor.includes('left') || anchor.includes('right'),
  height: anchor.includes('top') || anchor.includes('bottom'),
})

const snapDimension = (value: number, gridSize: number, lockedCells: number | null) => {
  const step = Math.max(0.25, gridSize)
  const minimumCells = Math.max(1, Math.ceil(0.5 / step))
  const nearestCells = Math.max(minimumCells, Math.round(value / step))
  const cells = lockedCells !== null && Math.abs(value - lockedCells * step) <= step * 0.6
    ? lockedCells
    : nearestCells
  return { cells, value: Number((cells * step).toFixed(6)) }
}

const snapCoordinate = (value: number, origin: number, step: number) => (
  origin + Math.round((value - origin) / step) * step
)

export const snapStageResizeBox = (
  box: StageResizeBox,
  session: StageGridResizeSession,
): StageResizeBox => {
  const axes = resizedAxes(session.anchor)
  let width = box.width
  let height = box.height

  if (axes.width && session.baseBoxWidth > 0 && session.baseWidth > 0) {
    const rawWidth = session.baseWidth * Math.abs(box.width) / session.baseBoxWidth
    const snapped = snapDimension(rawWidth, session.gridSize, session.lockedWidthCells)
    session.lockedWidthCells = snapped.cells
    width = Math.sign(box.width || 1) * session.baseBoxWidth * snapped.value / session.baseWidth
  }
  if (axes.height && session.baseBoxHeight > 0 && session.baseHeight > 0) {
    const rawHeight = session.baseHeight * Math.abs(box.height) / session.baseBoxHeight
    const snapped = snapDimension(rawHeight, session.gridSize, session.lockedHeightCells)
    session.lockedHeightCells = snapped.cells
    height = Math.sign(box.height || 1) * session.baseBoxHeight * snapped.value / session.baseHeight
  }

  const angle = box.rotation
  const widthDelta = box.width - width
  const heightDelta = box.height - height
  const horizontal = { x: Math.cos(angle), y: Math.sin(angle) }
  const vertical = { x: -Math.sin(angle), y: Math.cos(angle) }
  const keepRight = session.anchor.includes('left')
  const keepBottom = session.anchor.includes('top')
  let x = box.x
    + (keepRight ? horizontal.x * widthDelta : 0)
    + (keepBottom ? vertical.x * heightDelta : 0)
  let y = box.y
    + (keepRight ? horizontal.y * widthDelta : 0)
    + (keepBottom ? vertical.y * heightDelta : 0)

  // Only axis-aligned boxes can have both opposing edges sit on square-grid lines.
  // Rotated boxes still receive integer-cell local dimensions without a position jump.
  const axisAligned = Math.abs(Math.sin(angle)) < 0.000001 && Math.cos(angle) > 0
  if (axisAligned && session.gridStepPx > 0) {
    if (axes.width) {
      x = keepRight
        ? snapCoordinate(x + width, session.gridOriginX, session.gridStepPx) - width
        : snapCoordinate(x, session.gridOriginX, session.gridStepPx)
    }
    if (axes.height) {
      y = keepBottom
        ? snapCoordinate(y + height, session.gridOriginY, session.gridStepPx) - height
        : snapCoordinate(y, session.gridOriginY, session.gridStepPx)
    }
  }

  return {
    ...box,
    x,
    y,
    width,
    height,
  }
}
