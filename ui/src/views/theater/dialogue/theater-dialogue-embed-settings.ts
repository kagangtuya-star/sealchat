export const THEATER_DIALOGUE_EMBED_SETTINGS_KEY = 'theater-dialogue-overlay.settings.v1'
export interface TheaterDialogueEmbedSettings {
  version: 1
  identityId: string
  fontSize: number
  speakerColor: string
  contentColor: string
  fontAssetId: string
  showSpeaker: boolean
  charactersPerSecond: number | null
}

export const normalizeTheaterDialogueEmbedSettings = (
  input: unknown,
  isColor: (value: string) => boolean = value => typeof CSS !== 'undefined' && CSS.supports('color', value),
): TheaterDialogueEmbedSettings => {
  const value = input && typeof input === 'object' && !Array.isArray(input) ? input as Record<string, unknown> : {}
  const data = value.version === 1 ? value : {}
  const id = (raw: unknown) => typeof raw === 'string' && raw.trim().length <= 100 ? raw.trim() : ''
  const color = (raw: unknown) => typeof raw === 'string' && raw.length <= 128 && isColor(raw.trim()) ? raw.trim() : ''
  const number = (raw: unknown, min: number, max: number, fallback: number) => typeof raw === 'number' && Number.isFinite(raw) ? Math.min(max, Math.max(min, raw)) : fallback
  return {
    version: 1,
    identityId: id(data.identityId),
    fontSize: number(data.fontSize, 12, 72, 24),
    speakerColor: color(data.speakerColor),
    contentColor: color(data.contentColor),
    fontAssetId: id(data.fontAssetId),
    showSpeaker: typeof data.showSpeaker === 'boolean' ? data.showSpeaker : true,
    charactersPerSecond: typeof data.charactersPerSecond === 'number' && Number.isFinite(data.charactersPerSecond)
      ? number(data.charactersPerSecond, 1, 60, 10) : null,
  }
}
