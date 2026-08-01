import type { Dice3DMemberProfile, Dice3DWorldConfig, DiceVisualPayload } from '@/types'

const cloneSkin = (skin: DiceVisualPayload['appearance']): DiceVisualPayload['appearance'] => ({
  ...skin,
  textures: skin.textures ? { ...skin.textures } : {},
})

export const resolveDice3DPlaybackPayload = (
  payload: DiceVisualPayload,
  currentUserId: string,
  config: Dice3DWorldConfig | null,
  profile: Dice3DMemberProfile | null,
): DiceVisualPayload => {
  if (!config || !profile || !currentUserId || payload.actorUserId !== currentUserId) return payload

  return {
    ...payload,
    appearance: cloneSkin(profile.useOverride ? profile.skin : config.defaultSkin),
    audio: { ...(profile.audio ?? config.audio) },
  }
}
