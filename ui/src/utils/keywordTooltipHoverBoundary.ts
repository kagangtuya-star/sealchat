interface BoundaryElementLike {
  contains?: (target: any) => boolean
  closest?: (selector: string) => BoundaryElementLike | null
}

type BoundaryTarget = EventTarget | BoundaryElementLike | null | undefined

function toBoundaryElement(target: BoundaryTarget): BoundaryElementLike | null {
  if (!target || typeof target !== 'object') {
    return null
  }
  return target as BoundaryElementLike
}

export function isTargetWithinElement(target: BoundaryTarget, container: BoundaryElementLike | null | undefined): boolean {
  const element = toBoundaryElement(target)
  if (!element || !container || typeof container.contains !== 'function') {
    return false
  }
  return container.contains(element)
}

export function shouldKeepKeywordTooltipOpenOnTransition({
  root,
  relatedTarget,
  hoverTooltip,
  highlightSelector = '.keyword-highlight',
}: {
  root: BoundaryElementLike
  relatedTarget: BoundaryTarget
  hoverTooltip?: BoundaryElementLike | null
  highlightSelector?: string
}): boolean {
  const element = toBoundaryElement(relatedTarget)
  const movedToHighlight = element && typeof element.closest === 'function'
    ? element.closest(highlightSelector)
    : null

  if (movedToHighlight && isTargetWithinElement(movedToHighlight, root)) {
    return true
  }

  return isTargetWithinElement(relatedTarget, hoverTooltip)
}
