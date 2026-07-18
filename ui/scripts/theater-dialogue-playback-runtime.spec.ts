import assert from 'node:assert/strict'

import {
  createTwinLayerPlayback,
  resolvePunctuationPauseExtra,
} from '../src/components/chat/twinLayerPlayback'
import { parsePerformanceInstructions } from '../src/utils/tiptap-performance-parser'
import {
  TheaterDialogueRuntime,
  hasTheaterDialoguePerformanceContent,
} from '../src/views/theater/dialogue/theater-dialogue-runtime'
import type { TheaterDialogueMessage } from '../src/views/theater/bridge/theater-dialogue-queue'

const waitFor = async (predicate: () => boolean, timeoutMs = 1_000) => {
  const startedAt = Date.now()
  while (!predicate()) {
    if (Date.now() - startedAt >= timeoutMs) throw new Error('playback state wait timed out')
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
}

const richDocument = {
  type: 'doc',
  content: [{
    type: 'paragraph',
    content: [
      { type: 'text', text: '甲', marks: [{ type: 'performance', attrs: { enterMode: 'typewriter' } }] },
      { type: 'performanceCommand', attrs: { command: 'delay', value: 20 } },
      { type: 'text', text: '乙', marks: [{ type: 'performance', attrs: { enterMode: 'blur' } }] },
      { type: 'performanceCommand', attrs: { command: 'pause' } },
      { type: 'text', text: '丙', marks: [{ type: 'performance', attrs: { enterMode: 'typewriter' } }] },
    ],
  }],
}

const message: TheaterDialogueMessage = {
  messageId: 'performance-fallback',
  createdAt: 1,
  icMode: 'ic',
  isWhisper: false,
  isArchived: false,
  isDeleted: false,
  contentText: '甲乙丙',
  contentRichText: JSON.stringify(richDocument),
  actor: {
    userId: 'user-1',
    identityId: 'identity-1',
    variantId: null,
    displayName: 'Actor',
    color: '#fff',
    appearance: {},
  },
}

const runTests = async () => {
  assert.equal(resolvePunctuationPauseExtra('，', 100), 60)
  assert.equal(resolvePunctuationPauseExtra('.', 100), 300)
  assert.equal(resolvePunctuationPauseExtra('!', 10), 90)
  assert.equal(resolvePunctuationPauseExtra('字', 100), 0)

  assert.equal(hasTheaterDialoguePerformanceContent(message), true)
  const runtime = new TheaterDialogueRuntime({ reducedMotion: true })
  runtime.created(message)
  assert.equal(runtime.getSnapshot().phase, 'typing')
  assert.equal(runtime.getSnapshot().queue.current?.revealedCharacters, 0)
  runtime.dispose()

  const instructions = parsePerformanceInstructions(richDocument)
  let playback!: ReturnType<typeof createTwinLayerPlayback>
  const states: string[] = []
  playback = createTwinLayerPlayback(instructions, {
    charactersPerSecond: 60,
    onStateChange: () => states.push(playback.getState()),
  })
  const run = playback.play()
  await waitFor(() => playback.getState() === 'waiting')
  assert.equal(playback.getVisibleText(), '甲乙')
  playback.continuePlayback()
  await run
  assert.equal(playback.getState(), 'completed')
  assert.equal(playback.getVisibleText(), '甲乙丙')
  assert.equal(states.includes('waiting'), true)

  let cancelledPlayback!: ReturnType<typeof createTwinLayerPlayback>
  const cancellationStates: string[] = []
  cancelledPlayback = createTwinLayerPlayback([{
    type: 'char',
    char: '甲',
    effects: { enterMode: 'typewriter' },
    index: 0,
  }], {
    charactersPerSecond: 6,
    onStateChange: () => cancellationStates.push(cancelledPlayback.getState()),
  })
  void cancelledPlayback.play()
  cancelledPlayback.setCharactersPerSecond(12)
  cancelledPlayback.dispose()
  assert.equal(cancelledPlayback.getState(), 'cancelled')
  assert.equal(cancellationStates.includes('completed'), false)

  console.log('theater dialogue playback runtime tests passed')
}

void runTests().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
