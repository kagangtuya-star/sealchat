export interface WorldKeywordTooltipInteractionSettings {
  tooltipEnabled: boolean
  hoverEnabled: boolean
  clickEnabled: boolean
}

export interface WorldKeywordTooltipInteractionPolicy {
  allowHoverOpen: boolean
  allowClickOpen: boolean
}

export const DEFAULT_WORLD_KEYWORD_TOOLTIP_INTERACTION: WorldKeywordTooltipInteractionSettings = {
  tooltipEnabled: true,
  hoverEnabled: true,
  clickEnabled: true,
}

const coerceBoolean = (value: unknown, fallback: boolean): boolean => (
  typeof value === 'boolean' ? value : fallback
)

export function normalizeWorldKeywordTooltipInteractionSettings(
  value?: Partial<WorldKeywordTooltipInteractionSettings> | null,
): WorldKeywordTooltipInteractionSettings {
  return {
    tooltipEnabled: coerceBoolean(value?.tooltipEnabled, DEFAULT_WORLD_KEYWORD_TOOLTIP_INTERACTION.tooltipEnabled),
    hoverEnabled: coerceBoolean(value?.hoverEnabled, DEFAULT_WORLD_KEYWORD_TOOLTIP_INTERACTION.hoverEnabled),
    clickEnabled: coerceBoolean(value?.clickEnabled, DEFAULT_WORLD_KEYWORD_TOOLTIP_INTERACTION.clickEnabled),
  }
}

export function resolveWorldKeywordTooltipInteractionPolicy(
  value: WorldKeywordTooltipInteractionSettings & { finePointer: boolean },
): WorldKeywordTooltipInteractionPolicy {
  const settings = normalizeWorldKeywordTooltipInteractionSettings(value)
  const tooltipEnabled = settings.tooltipEnabled

  return {
    allowHoverOpen: tooltipEnabled && settings.hoverEnabled && value.finePointer,
    allowClickOpen: tooltipEnabled && settings.clickEnabled,
  }
}
