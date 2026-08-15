export type MessageVisibilityScope = 'all' | 'ic' | 'ooc'
export type AvatarVisibilityScope = MessageVisibilityScope

export interface AvatarRenderStateInput {
  avatarsEnabled: boolean
  avatarVisibilityScope: AvatarVisibilityScope
  icMode?: string | null
  mergedWithPrev?: boolean
}

export const normalizeMessageVisibilityScope = (
  value: unknown,
  fallback: MessageVisibilityScope = 'all',
): MessageVisibilityScope => {
  if (value === 'ic' || value === 'ooc') {
    return value
  }
  if (value === 'all') {
    return 'all'
  }
  return fallback
}

export const normalizeAvatarVisibilityScope = (value: unknown): AvatarVisibilityScope => {
  return normalizeMessageVisibilityScope(value)
}

export const normalizeMessageIcMode = (value: unknown): 'ic' | 'ooc' => {
  if (typeof value === 'string' && value.toLowerCase() === 'ooc') {
    return 'ooc'
  }
  return 'ic'
}

export const messageVisibilityScopeMatches = (
  scope: MessageVisibilityScope,
  icMode?: string | null,
) => {
  const normalizedScope = normalizeMessageVisibilityScope(scope)
  const normalizedIcMode = normalizeMessageIcMode(icMode)
  return normalizedScope === 'all' || normalizedScope === normalizedIcMode
}

export const resolveAvatarRenderState = ({
  avatarsEnabled,
  avatarVisibilityScope,
  icMode,
  mergedWithPrev = false,
}: AvatarRenderStateInput) => {
  if (!avatarsEnabled) {
    return {
      showAvatar: false,
      hideAvatar: false,
    }
  }

  const scopeMatched = messageVisibilityScopeMatches(avatarVisibilityScope, icMode)

  if (!scopeMatched) {
    return {
      showAvatar: true,
      hideAvatar: true,
    }
  }

  return {
    showAvatar: true,
    hideAvatar: Boolean(mergedWithPrev),
  }
}
