import type { TheaterTransform } from '@/types/theaterPresentation'

export interface DialoguePosition { x: number; y: number }
export interface DialogueLayoutActor { actorKey: string; aspectRatio: number; transform?: TheaterTransform }
export interface DialoguePortraitRect extends DialoguePosition {
  actorKey: string
  width: number
  height: number
  rotation: number
  opacity: number
  zIndex: number
}
export interface DialogueLayoutInput {
  width: number
  height: number
  actors: readonly DialogueLayoutActor[]
  positions?: Readonly<Record<string, DialoguePosition>>
  margin?: number
  gap?: number
}

const finitePositive = (value: number) => Number.isFinite(value) ? Math.max(0, value) : 0
const clamp = (value: number, maximum: number) => Math.max(0, Math.min(maximum, Number.isFinite(value) ? value : 0))
const clampRange = (value: number, minimum: number, maximum: number) => Math.max(minimum, Math.min(maximum, Number.isFinite(value) ? value : minimum))
const rotationFactors = (rotation: number) => {
  const radians = (Number.isFinite(rotation) ? rotation : 0) * Math.PI / 180
  return { absCos: Math.abs(Math.cos(radians)), absSin: Math.abs(Math.sin(radians)) }
}

export const dialoguePortraitVisualBounds = (rect: DialoguePortraitRect) => {
  const { absCos, absSin } = rotationFactors(rect.rotation)
  const visualWidth = rect.width * absCos + rect.height * absSin
  const visualHeight = rect.width * absSin + rect.height * absCos
  const left = rect.x - (visualWidth - rect.width) / 2
  const top = rect.y - (visualHeight - rect.height) / 2
  return { left, top, right: left + visualWidth, bottom: top + visualHeight, width: visualWidth, height: visualHeight }
}

const legalOriginRange = (rect: DialoguePortraitRect, width: number, height: number) => {
  const visual = dialoguePortraitVisualBounds({ ...rect, x: 0, y: 0 })
  const offsetX = (visual.width - rect.width) / 2
  const offsetY = (visual.height - rect.height) / 2
  return {
    minX: offsetX,
    maxX: width - rect.width - offsetX,
    minY: offsetY,
    maxY: height - rect.height - offsetY,
  }
}
const intersects = (a: DialoguePortraitRect, b: DialoguePortraitRect, gap = 0) => {
  const aVisual = dialoguePortraitVisualBounds(a)
  const bVisual = dialoguePortraitVisualBounds(b)
  return aVisual.left < bVisual.right + gap && bVisual.left < aVisual.right + gap
    && aVisual.top < bVisual.bottom + gap && bVisual.top < aVisual.bottom + gap
}

/** Check actual media rectangles before exposing any layout to the renderer. */
export const validDialogueLayout = (rects: readonly DialoguePortraitRect[], width: number, height: number): boolean => (
  rects.every((rect, index) => (
    [rect.x, rect.y, rect.width, rect.height, rect.rotation].every(Number.isFinite)
    && rect.width >= 0 && rect.height >= 0
    && dialoguePortraitVisualBounds(rect).left >= 0 && dialoguePortraitVisualBounds(rect).top >= 0
    && dialoguePortraitVisualBounds(rect).right <= width && dialoguePortraitVisualBounds(rect).bottom <= height
    && !rects.slice(index + 1).some(other => intersects(rect, other))
  ))
)

/** Natural aspect ratio and deterministic packing; overflow falls back to one common height. */
export const layoutDialoguePortraits = (input: DialogueLayoutInput): DialoguePortraitRect[] => {
  const width = finitePositive(input.width)
  const height = finitePositive(input.height)
  if (!width || !height || !input.actors.length) return []
  const count = input.actors.length
  const margin = Math.min(finitePositive(input.margin ?? 16), width / 8, height / 8)
  const availableWidth = width - 2 * margin
  const availableHeight = height - 2 * margin
  const gap = count > 1 ? Math.min(finitePositive(input.gap ?? 12), availableWidth / (2 * (count - 1))) : 0
  const ratios = input.actors.map(actor => Number.isFinite(actor.aspectRatio) && actor.aspectRatio > 0 ? actor.aspectRatio : 1)
  const factors = input.actors.map((actor, index) => {
    const rotation = Number.isFinite(actor.transform?.rotation) ? actor.transform!.rotation : 0
    const { absCos, absSin } = rotationFactors(rotation)
    return {
      rotation,
      visualWidth: ratios[index] * absCos + absSin,
      visualHeight: absCos + ratios[index] * absSin,
    }
  })
  const desired = input.actors.map((actor, index) => {
    const transform = actor.transform
    const factor = factors[index]
    const maximumHeight = Math.min(width / factor.visualWidth, height / factor.visualHeight)
    if (!transform) {
      const portraitHeight = Math.min(height - 2 * margin, maximumHeight)
      return { actorKey: actor.actorKey, x: 0, y: height - margin - portraitHeight, width: portraitHeight * ratios[index], height: portraitHeight, rotation: factor.rotation, opacity: 1, zIndex: 0 }
    }
    const portraitHeight = Math.min(finitePositive(transform.height * height), maximumHeight)
    const rect = { actorKey: actor.actorKey, x: 0, y: 0, width: portraitHeight * ratios[index], height: portraitHeight, rotation: factor.rotation, opacity: Number.isFinite(transform.opacity) ? transform.opacity : 1, zIndex: Number.isFinite(transform.zIndex) ? transform.zIndex : 0 }
    const range = legalOriginRange(rect, width, height)
    return { ...rect, y: clampRange(transform.y * height, range.minY, range.maxY) }
  })
  const totalDesiredWidth = desired.reduce((sum, rect) => sum + dialoguePortraitVisualBounds({ ...rect, x: 0 }).width, 0) + gap * (count - 1)
  const forcedCommonHeight = totalDesiredWidth > availableWidth || desired.some(rect => dialoguePortraitVisualBounds({ ...rect, x: 0 }).height > availableHeight)
    ? Math.min(
        ...desired.map(rect => rect.height),
        (availableWidth - gap * (count - 1)) / factors.reduce((sum, factor) => sum + factor.visualWidth, 0),
        availableHeight / Math.max(...factors.map(factor => factor.visualHeight)),
      ) * (1 - 1e-12)
    : null
  let visualLeft = margin
  const fallback = desired.map((desiredRect, index) => {
    const portraitHeight = forcedCommonHeight ?? desiredRect.height
    const rect = {
      ...desiredRect,
      width: portraitHeight * ratios[index],
      height: portraitHeight,
    }
    const visual = dialoguePortraitVisualBounds({ ...rect, x: 0, y: 0 })
    const range = legalOriginRange(rect, width, height)
    const packed = {
      ...rect,
      x: visualLeft + (visual.width - rect.width) / 2,
      y: clampRange(desiredRect.y, Math.max(range.minY, margin + (visual.height - rect.height) / 2), Math.min(range.maxY, height - margin - rect.height - (visual.height - rect.height) / 2)),
    }
    visualLeft += visual.width + gap
    return packed
  })
  const placed: DialoguePortraitRect[] = []
  for (const rect of fallback) {
    const range = legalOriginRange(rect, width, height)
    const preference = input.positions?.[rect.actorKey]
    const target = preference
      ? { x: range.minX + clamp(preference.x, 1) * (range.maxX - range.minX), y: range.minY + clamp(preference.y, 1) * (range.maxY - range.minY) }
      : rect
    const visual = dialoguePortraitVisualBounds({ ...rect, x: 0, y: 0 })
    const offsetX = (visual.width - rect.width) / 2
    const offsetY = (visual.height - rect.height) / 2
    const xs = [target.x, range.minX, range.maxX]
    const ys = [target.y, range.minY, range.maxY]
    for (const other of placed) {
      const otherVisual = dialoguePortraitVisualBounds(other)
      xs.push(otherVisual.left - gap - visual.width + offsetX, otherVisual.right + gap + offsetX)
      ys.push(otherVisual.top - gap - visual.height + offsetY, otherVisual.bottom + gap + offsetY)
    }
    const candidates = xs.flatMap(left => ys.map(top => ({ ...rect, x: clampRange(left, range.minX, range.maxX), y: clampRange(top, range.minY, range.maxY) })))
      .filter(candidate => !placed.some(other => intersects(candidate, other, gap)))
      .sort((a, b) => ((a.x - target.x) ** 2 + (a.y - target.y) ** 2) - ((b.x - target.x) ** 2 + (b.y - target.y) ** 2) || a.x - b.x || a.y - b.y)
    if (!candidates[0]) return fallback
    placed.push(candidates[0])
  }
  return validDialogueLayout(placed, width, height) ? placed : fallback
}

export const normalizedDialoguePosition = (rect: DialoguePortraitRect, width: number, height: number): DialoguePosition => {
  const range = legalOriginRange(rect, width, height)
  return {
    x: range.maxX > range.minX ? clamp((rect.x - range.minX) / (range.maxX - range.minX), 1) : 0,
    y: range.maxY > range.minY ? clamp((rect.y - range.minY) / (range.maxY - range.minY), 1) : 0,
  }
}
