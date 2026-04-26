interface VisibleActionCountInput {
  isMobile: boolean
  containerWidth: number
  moreButtonWidth: number
  buttonGap: number
  buttonWidths: number[]
}

export const calculateVisibleActionCount = ({
  isMobile,
  containerWidth,
  moreButtonWidth,
  buttonGap,
  buttonWidths,
}: VisibleActionCountInput): number => {
  if (isMobile) {
    return buttonWidths.length
  }

  if (buttonWidths.length === 0) {
    return 0
  }

  const totalButtonsWidth = buttonWidths.reduce((sum, width) => sum + width, 0) + Math.max(buttonWidths.length - 1, 0) * buttonGap
  if (totalButtonsWidth <= containerWidth) {
    return buttonWidths.length
  }

  const availableWidth = Math.max(containerWidth - moreButtonWidth - buttonGap, 0)
  let usedWidth = 0
  let visibleCount = 0

  for (const buttonWidth of buttonWidths) {
    const nextWidth = usedWidth + (visibleCount > 0 ? buttonGap : 0) + buttonWidth
    if (nextWidth > availableWidth) {
      break
    }
    usedWidth = nextWidth
    visibleCount += 1
  }

  return visibleCount
}
