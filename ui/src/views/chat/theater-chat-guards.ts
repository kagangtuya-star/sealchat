export interface TheaterIdentityLike {
  id?: string
}

export interface TheaterVariantLike {
  id?: string
  enabled?: boolean
}

export type TheaterChatGuardError = {
  ok: false
  error: {
    code: string
    message: string
  }
}

export const validateTheaterCharacter = (
  identities: TheaterIdentityLike[],
  identityId: string,
): TheaterChatGuardError | null => {
  if (!identityId) {
    return { ok: false, error: { code: 'INVALID_CHARACTER', message: '角色参数无效' } }
  }
  if (!identities.some((identity) => String(identity?.id || '') === identityId)) {
    return { ok: false, error: { code: 'CHARACTER_NOT_AVAILABLE', message: '指定角色不可用于当前频道' } }
  }
  return null
}

export const validateTheaterVariant = ({
  activeIdentityId,
  identityId,
  variantId,
  variants,
}: {
  activeIdentityId: string
  identityId: string
  variantId: string
  variants: TheaterVariantLike[]
}): TheaterChatGuardError | null => {
  if (activeIdentityId !== identityId) {
    return { ok: false, error: { code: 'CHARACTER_NOT_SELECTED', message: '请先选择目标角色' } }
  }
  if (variantId && !variants.some((variant) => (
    String(variant?.id || '') === variantId && variant.enabled !== false
  ))) {
    return { ok: false, error: { code: 'VARIANT_NOT_AVAILABLE', message: '指定差分不可用于当前角色' } }
  }
  return null
}

export const shouldResolveTheaterIdentityShortcut = ({
  identityIdOverride,
  inputMode,
  channelId,
  draft,
  trigger,
}: {
  identityIdOverride?: string
  inputMode: string
  channelId: string
  draft: string
  trigger: string
}) => !identityIdOverride && inputMode === 'plain' && Boolean(channelId) && draft.startsWith(trigger)

export const hasTheaterComposerDraft = ({
  meaningfulText,
  inlineImageCount,
}: {
  meaningfulText: boolean
  inlineImageCount: number
}) => meaningfulText || inlineImageCount > 0
