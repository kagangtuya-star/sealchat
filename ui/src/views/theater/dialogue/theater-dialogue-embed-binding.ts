import type { TheaterDialogueRuntime } from './theater-dialogue-runtime'

/** Serialized by the surface; reset before awaiting the old subscription's release. */
export const replaceTheaterDialogueEmbedIdentity = async (
  runtime: Pick<TheaterDialogueRuntime, 'reset'>,
  dialogue: { unsubscribe(): Promise<unknown>; subscribe(params: { identityId: string }): Promise<unknown> },
  identityId: string,
  isCurrent: () => boolean,
) => {
  runtime.reset()
  await dialogue.unsubscribe()
  if (identityId && isCurrent()) await dialogue.subscribe({ identityId })
}
