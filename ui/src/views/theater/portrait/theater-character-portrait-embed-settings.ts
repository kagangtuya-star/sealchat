export const THEATER_CHARACTER_PORTRAIT_EMBED_SETTINGS_KEY = 'theater-character-portrait.settings.v1'

export interface TheaterCharacterPortraitEmbedSettings {
  version: 1
  identityId: string
}

export const normalizeTheaterCharacterPortraitEmbedSettings = (input: unknown): TheaterCharacterPortraitEmbedSettings => {
  const value = input && typeof input === 'object' && !Array.isArray(input) ? input as Record<string, unknown> : {}
  const data = value.version === 1 ? value : {}
  const identityId = typeof data.identityId === 'string' && data.identityId.trim().length <= 100
    ? data.identityId.trim()
    : ''
  return { version: 1, identityId }
}
