import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { reactive } from 'vue'

import { resolveTheaterMediaCandidates } from '../src/components/theater-presentation/theaterPresentationMedia'
import { resolveCharactersPerSecondDelay, resolvePunctuationPauseExtra } from '../src/components/chat/twinLayerPlayback'
import { createDefaultTheaterPresentation } from '../src/types/theaterPresentation'
import {
  THEATER_DIALOGUE_HOLD_MS,
  TheaterDialogueRuntime,
  getTheaterDialogueTypingDuration,
  resolveTheaterDialoguePresentation,
} from '../src/views/theater/dialogue/theater-dialogue-runtime'
import type { TheaterDialogueMessage } from '../src/views/theater/bridge/theater-dialogue-queue'
import type { ChatCharactersSnapshotPayload } from '../src/views/theater/bridge/theater-bridge-protocol'

class FakeScheduler {
  now = 0
  nextId = 1
  timers = new Map<number, { at: number; callback: () => void }>()

  setTimeout = (callback: () => void, delayMs: number) => {
    const id = this.nextId++
    this.timers.set(id, { at: this.now + delayMs, callback })
    return id as unknown as ReturnType<typeof setTimeout>
  }

  clearTimeout = (timer: ReturnType<typeof setTimeout>) => {
    this.timers.delete(timer as unknown as number)
  }

  tick(durationMs: number) {
    const target = this.now + durationMs
    while (true) {
      const next = [...this.timers.entries()]
        .filter(([, timer]) => timer.at <= target)
        .sort((left, right) => left[1].at - right[1].at || left[0] - right[0])[0]
      if (!next) break
      this.now = next[1].at
      this.timers.delete(next[0])
      next[1].callback()
    }
    this.now = target
  }
}

const message = (messageId: string, contentText = messageId): TheaterDialogueMessage => ({
  messageId,
  createdAt: 1,
  icMode: 'ic',
  isWhisper: false,
  isArchived: false,
  isDeleted: false,
  contentText,
  actor: {
    userId: 'user-1',
    identityId: 'identity-1',
    variantId: null,
    displayName: 'Actor',
    color: '#fff',
    appearance: {},
  },
})

assert.equal(getTheaterDialogueTypingDuration(6), 1_000)
assert.equal(getTheaterDialogueTypingDuration(12, 12), 1_000)
assert.equal(getTheaterDialogueTypingDuration(6, Number.NaN), 1_000)
assert.equal(resolveCharactersPerSecondDelay(6), 1_000 / 6)
assert.equal(resolveCharactersPerSecondDelay(undefined), null)
assert.equal(resolvePunctuationPauseExtra('，', 100), 60)
assert.equal(resolvePunctuationPauseExtra('。', 100), 300)
assert.equal(resolvePunctuationPauseExtra('！', 10), 90)
assert.equal(resolvePunctuationPauseExtra('字', 100), 0)

const scheduler = new FakeScheduler()
const runtime = new TheaterDialogueRuntime({ scheduler })
runtime.created(message('one', 'AB😀'))
runtime.created(message('two', 'second'))
assert.equal(runtime.getSnapshot().queue.current?.message.messageId, 'one')
assert.deepEqual(runtime.getSnapshot().queue.waiting.map((item) => item.message.messageId), ['two'])
const firstMessageSnapshot = runtime.getSnapshot().queue.current?.message
scheduler.tick(399)
assert.equal(runtime.getSnapshot().queue.current?.revealedCharacters, 2)
assert.equal(runtime.getSnapshot().queue.current?.message, firstMessageSnapshot)
scheduler.tick(101)
assert.equal(runtime.getSnapshot().queue.current?.revealedCharacters, 3)
assert.equal(runtime.getSnapshot().phase, 'hold')
scheduler.tick(THEATER_DIALOGUE_HOLD_MS)
assert.equal(runtime.getSnapshot().queue.current?.message.messageId, 'two')
scheduler.tick(getTheaterDialogueTypingDuration('second'.length) + THEATER_DIALOGUE_HOLD_MS)
assert.equal(runtime.getSnapshot().queue.current?.message.messageId, 'two')
assert.equal(runtime.getSnapshot().phase, 'hold')

runtime.updated(message('two', 'xy'))
assert.equal(runtime.getSnapshot().queue.current?.message.contentText, 'xy')
runtime.created(message('three', 'waiting-old'))
runtime.updated(message('three', 'waiting-new'))
assert.equal(runtime.getSnapshot().queue.waiting[0].message.contentText, 'waiting-new')
runtime.removed('three')
assert.deepEqual(runtime.getSnapshot().queue.waiting, [])

runtime.created(message('four', 'queued'))
runtime.skip()
assert.equal(runtime.getSnapshot().queue.current?.message.messageId, 'four')
assert.equal(runtime.getSnapshot().queue.current?.revealedCharacters, 0)
runtime.skip()
assert.equal(runtime.getSnapshot().phase, 'hold')
runtime.skip()
assert.equal(runtime.getSnapshot().queue.current?.message.messageId, 'four')
runtime.close()
assert.equal(runtime.getSnapshot().queue.current, null)

runtime.created(message('five', 'close me'))
runtime.created(message('six', 'dismiss me'))
runtime.close()
const dismissedThroughSequence = runtime.getSnapshot().queue.dismissedThroughSequence
assert.equal(runtime.getSnapshot().queue.current, null)
assert.equal(dismissedThroughSequence, runtime.getSnapshot().queue.lastSequence)
runtime.created(message('five', 'must stay dismissed'))
assert.equal(runtime.getSnapshot().queue.current, null)
runtime.created(message('seven', 'new after close'))
assert.equal(runtime.getSnapshot().queue.current?.message.messageId, 'seven')

runtime.reset()
assert.equal(runtime.getSnapshot().queue.current, null)
assert.deepEqual(runtime.getSnapshot().queue.recentMessageIds, [])
runtime.created(message('reduced', 'paced'))
runtime.setReducedMotion(true)
assert.equal(runtime.getSnapshot().phase, 'typing')
assert.equal(runtime.getSnapshot().queue.current?.revealedCharacters, 0)
scheduler.tick(167)
assert.equal(runtime.getSnapshot().queue.current?.revealedCharacters, 1)

const customPresentation = createDefaultTheaterPresentation()
customPresentation.dialogue.textAlign = 'right'
customPresentation.dialogue.charactersPerSecond = 12
const customMessage = message('presentation')
customMessage.actor.appearance.theaterPresentation = customPresentation
assert.equal(resolveTheaterDialoguePresentation(customMessage).dialogue.textAlign, 'right')
assert.equal(resolveTheaterDialoguePresentation(customMessage).dialogue.charactersPerSecond, 12)
customPresentation.dialogue.textAlign = 'left'
assert.equal(resolveTheaterDialoguePresentation(customMessage).dialogue.textAlign, 'left')

const speedScheduler = new FakeScheduler()
const speedRuntime = new TheaterDialogueRuntime({ scheduler: speedScheduler })
speedRuntime.created(customMessage)
speedScheduler.tick(83)
assert.equal(speedRuntime.getSnapshot().queue.current?.revealedCharacters, 0)
speedScheduler.tick(1)
assert.equal(speedRuntime.getSnapshot().queue.current?.revealedCharacters, 1)

const punctuationScheduler = new FakeScheduler()
const punctuationRuntime = new TheaterDialogueRuntime({ scheduler: punctuationScheduler })
punctuationRuntime.created(message('punctuation', '甲。乙'))
punctuationScheduler.tick(334)
assert.equal(punctuationRuntime.getSnapshot().queue.current?.revealedCharacters, 2)
punctuationScheduler.tick(582)
assert.equal(punctuationRuntime.getSnapshot().queue.current?.revealedCharacters, 2)
punctuationScheduler.tick(1)
assert.equal(punctuationRuntime.getSnapshot().queue.current?.revealedCharacters, 3)

const performanceScheduler = new FakeScheduler()
const performanceRuntime = new TheaterDialogueRuntime({ scheduler: performanceScheduler })
const performanceMessage = message('performance', 'animated')
performanceMessage.hasPerformanceContent = true
performanceRuntime.created(performanceMessage)
performanceScheduler.tick(10_000)
assert.equal(performanceRuntime.getSnapshot().phase, 'typing')
assert.equal(performanceRuntime.getSnapshot().queue.current?.revealedCharacters, 0)
assert.equal(resolveTheaterDialoguePresentation(message('legacy')).dialogue.frame, null)
const snapshotPresentation = createDefaultTheaterPresentation()
snapshotPresentation.dialogue.textAlign = 'center'
const characterSnapshot = {
  characters: [{
    identityId: 'identity-1',
    activeVariantId: null,
    baseAppearance: { theaterPresentation: snapshotPresentation },
    resolvedAppearance: { theaterPresentation: snapshotPresentation },
    variants: [],
  }],
} as ChatCharactersSnapshotPayload
assert.equal(resolveTheaterDialoguePresentation(message('snapshot'), characterSnapshot).dialogue.textAlign, 'center')
assert.equal(
  resolveTheaterDialoguePresentation(
    reactive(message('reactive-snapshot')),
    reactive(characterSnapshot),
  ).dialogue.textAlign,
  'center',
)

const videoMedia = {
  assetId: 'asset-1',
  resourceAttachmentId: 'primary.webm',
  fallbackAttachmentId: 'fallback.webp',
  mimeType: 'video/webm' as const,
  kind: 'video' as const,
  width: 100,
  height: 100,
}
assert.deepEqual(resolveTheaterMediaCandidates(videoMedia), [
  { kind: 'video', attachmentId: 'primary.webm' },
  { kind: 'image', attachmentId: 'fallback.webp' },
])
assert.deepEqual(resolveTheaterMediaCandidates(videoMedia, { supportsVideo: false }), [
  { kind: 'image', attachmentId: 'fallback.webp' },
  { kind: 'video', attachmentId: 'primary.webm' },
])

const overlaySource = readFileSync('src/views/theater/dialogue/TheaterDialogueOverlay.vue', 'utf8')
assert.match(overlaySource, /theater-dialogue-shell__default/)
assert.match(overlaySource, /<RichTextContent/)
assert.doesNotMatch(overlaySource, /v-html/)
assert.match(overlaySource, /PlayerSkipForward/)
assert.match(overlaySource, /top: `max\(\$\{padding\.top \* 100\}%/)
assert.doesNotMatch(overlaySource, /paddingTop: `max\(\$\{padding\.top \* 100\}%/)
assert.doesNotMatch(overlaySource, /z-index: -100/)

let disposedUpdates = 0
const disposeScheduler = new FakeScheduler()
const disposable = new TheaterDialogueRuntime({ scheduler: disposeScheduler })
disposable.subscribe(() => { disposedUpdates += 1 })
disposable.created(message('dispose', 'timer'))
const beforeDispose = disposedUpdates
disposable.dispose()
disposeScheduler.tick(20_000)
assert.equal(disposedUpdates, beforeDispose)

console.log('theater dialogue runtime tests passed')
