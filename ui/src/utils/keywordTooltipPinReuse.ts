export function shouldReuseHoverTooltipForPin(tooltip: { style: { display?: string } } | null | undefined) {
  return tooltip?.style.display === 'block'
}
