import assert from 'node:assert/strict'

import {
  THEATER_DIALOGUE_DEDUPE_LIMIT,
  THEATER_DIALOGUE_QUEUE_LIMIT,
  createTheaterDialogueQueueState,
  getTheaterDialogueTextLength,
  isTheaterDialogueTyping,
  reduceTheaterDialogueQueue,
  shouldEnqueueTheaterDialogue,
  type TheaterDialogueMessage,
} from '../src/views/theater/bridge/theater-dialogue-queue'

const message = (messageId: string, contentText = `message ${messageId}`): TheaterDialogueMessage => ({
  messageId,
  createdAt: Number(messageId.replace(/\D/g, '')) || 1,
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

assert.equal(shouldEnqueueTheaterDialogue(message('1')), true)
assert.equal(shouldEnqueueTheaterDialogue({ ...message('2'), icMode: 'ooc' }), false)
assert.equal(shouldEnqueueTheaterDialogue({ ...message('3'), isWhisper: true }), false)
assert.equal(shouldEnqueueTheaterDialogue({ ...message('4'), isArchived: true }), false)
assert.equal(shouldEnqueueTheaterDialogue({ ...message('5'), isDeleted: true }), false)
assert.equal(shouldEnqueueTheaterDialogue({ ...message('6'), contentText: ' \n ' }), false)
assert.equal(shouldEnqueueTheaterDialogue({ ...message('7'), actor: { ...message('7').actor, identityId: null } }), false)
assert.equal(shouldEnqueueTheaterDialogue({ ...message('8'), actor: { ...message('8').actor, identityId: '  ' } }), false)
assert.equal(getTheaterDialogueTextLength(message('emoji', 'A😀B')), 3)

let state = createTheaterDialogueQueueState()
state = reduceTheaterDialogueQueue(state, { type: 'created', message: message('1', 'abcdef') })
state = reduceTheaterDialogueQueue(state, { type: 'created', message: message('2') })
state = reduceTheaterDialogueQueue(state, { type: 'created', message: message('2') })
assert.equal(state.current?.message.messageId, '1')
assert.deepEqual(state.waiting.map((item) => item.message.messageId), ['2'])
assert.equal(state.lastSequence, 2)

state = reduceTheaterDialogueQueue(state, { type: 'reveal', characterCount: 5 })
assert.equal(isTheaterDialogueTyping(state.current), true)
const beforeInvalidReveal = state
state = reduceTheaterDialogueQueue(state, { type: 'reveal', characterCount: Number.NaN })
assert.equal(state, beforeInvalidReveal)
state = reduceTheaterDialogueQueue(state, { type: 'updated', message: message('1', 'abc') })
assert.equal(state.current?.revealedCharacters, 3)
state = reduceTheaterDialogueQueue(state, { type: 'updated', message: message('2', 'updated') })
assert.equal(state.waiting[0].message.contentText, 'updated')

state = reduceTheaterDialogueQueue(state, { type: 'skip' })
assert.equal(state.current?.message.messageId, '2')
assert.equal(state.current?.revealedCharacters, 0)
assert.deepEqual(state.waiting, [])
state = reduceTheaterDialogueQueue(state, { type: 'skip' })
assert.equal(isTheaterDialogueTyping(state.current), false)
state = reduceTheaterDialogueQueue(state, { type: 'skip' })
assert.equal(state.current?.message.messageId, '2')
state = reduceTheaterDialogueQueue(state, { type: 'close' })
assert.equal(state.current, null)

state = reduceTheaterDialogueQueue(state, { type: 'created', message: message('3') })
state = reduceTheaterDialogueQueue(state, { type: 'created', message: message('4') })
state = reduceTheaterDialogueQueue(state, { type: 'close' })
assert.equal(state.current, null)
assert.deepEqual(state.waiting, [])
assert.equal(state.dismissedThroughSequence, 4)
state = reduceTheaterDialogueQueue(state, { type: 'created', message: message('5') })
assert.equal(state.current?.message.messageId, '5')

state = createTheaterDialogueQueueState()
for (let index = 0; index < THEATER_DIALOGUE_QUEUE_LIMIT + 10; index += 1) {
  state = reduceTheaterDialogueQueue(state, { type: 'created', message: message(`overflow-${index}`) })
}
assert.equal(1 + state.waiting.length, THEATER_DIALOGUE_QUEUE_LIMIT)
assert.equal(state.current?.message.messageId, 'overflow-0')
assert.equal(state.waiting[0].message.messageId, 'overflow-11')
const expectedLastOverflowId: string = `overflow-${THEATER_DIALOGUE_QUEUE_LIMIT + 9}`
assert.equal(state.waiting.at(-1)?.message.messageId, expectedLastOverflowId)

state = reduceTheaterDialogueQueue(state, { type: 'removed', messageId: 'overflow-0' })
assert.equal(state.current?.message.messageId, 'overflow-11')
state = reduceTheaterDialogueQueue(state, { type: 'removed', messageId: 'overflow-12' })
assert.equal(state.waiting.some((item) => item.message.messageId === 'overflow-12'), false)

state = createTheaterDialogueQueueState()
for (let index = 0; index < THEATER_DIALOGUE_DEDUPE_LIMIT + 2; index += 1) {
  state = reduceTheaterDialogueQueue(state, { type: 'created', message: message(`dedupe-${index}`) })
}
assert.equal(state.recentMessageIds.length, THEATER_DIALOGUE_DEDUPE_LIMIT)
assert.equal(state.recentMessageIds.includes('dedupe-0'), false)
state = reduceTheaterDialogueQueue(state, { type: 'reset' })
assert.deepEqual(state, createTheaterDialogueQueueState())

console.log('theater dialogue queue runtime tests passed')
