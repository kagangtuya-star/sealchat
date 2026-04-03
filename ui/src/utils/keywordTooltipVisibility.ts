export function shouldHideTooltipBeforePositioning(tooltip: { style: { display?: string; visibility?: string } }) {
  return tooltip.style.display !== 'block' || tooltip.style.visibility !== 'visible'
}
