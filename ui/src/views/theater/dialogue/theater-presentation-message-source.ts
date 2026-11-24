import type { PresentationMessageSource } from '../shared/presentation-message-source'
import type { TheaterDialogueRuntime } from './theater-dialogue-runtime'

export const theaterDialogueMessageSource = (
  runtime: Pick<TheaterDialogueRuntime, 'subscribe'>,
  options: { includeCurrent?: boolean } = {},
): PresentationMessageSource => ({
  subscribe(listener) {
    let initial = true
    let previousId = ''
    return runtime.subscribe((snapshot) => {
      const message = snapshot.queue.current?.message ?? null
      const id = message?.messageId ?? ''
      const changed = initial ? options.includeCurrent === true : id !== previousId
      initial = false
      previousId = id
      if (changed) listener(message)
    })
  },
})
