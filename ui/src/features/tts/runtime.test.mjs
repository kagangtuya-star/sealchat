import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

// Compile only the pure TypeScript module; no app, DOM or heavyweight test runner.
const source = readFileSync(new URL('./runtime.ts', import.meta.url), 'utf8')
const { outputText, diagnostics } = ts.transpileModule(source, { reportDiagnostics: true, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } })
assert.deepEqual(diagnostics, [], 'pure runtime module must have no syntax diagnostics')
const { SpeechEpoch, SpeechRequestKeys, SpeechQueue, speechAvailableAmount } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

test('stop/reset invalidates all pending playback callbacks', () => {
  const epoch = new SpeechEpoch()
  const previous = epoch.capture()
  assert.equal(epoch.current(previous), true)
  epoch.invalidate()
  assert.equal(epoch.current(previous), false)
  assert.equal(epoch.current(epoch.capture()), true)
})

test('ambiguous POST response reuses request key for identical operation', () => {
  const keys = new SpeechRequestKeys()
  assert.equal(keys.resolve('audition', { requestKey: 'first', text: 'hello', voiceId: 'a' }), 'first')
  assert.equal(keys.resolve('audition', { voiceId: 'a', text: 'hello', requestKey: 'second' }), 'first')
  assert.equal(keys.resolve('design', { requestKey: 'design', text: 'hello', voiceId: 'a' }), 'design')
  assert.equal(keys.resolve('audition', { requestKey: 'new-text', text: 'changed', voiceId: 'a' }), 'new-text')
  keys.clear()
  assert.equal(keys.resolve('audition', { requestKey: 'new-user', text: 'hello', voiceId: 'a' }), 'new-user')
})

test('request key memory is bounded', () => {
  const keys = new SpeechRequestKeys()
  for (let i = 0; i < 256; i++) keys.resolve('audition', { requestKey: String(i), text: String(i) })
  assert.throws(() => keys.resolve('audition', { requestKey: 'overflow', text: 'overflow' }))
  assert.equal(keys.resolve('audition', { requestKey: 'retry', text: '0' }), '0')
})

test('idle page plays at once; busy page queues successors in server order without consuming their PCM', () => {
  const queue = new SpeechQueue()
  assert.equal(queue.start(1, 'a', true), 'play')
  assert.equal(queue.current, 'a')
  // A is playing: B and C wait for their archives; A is never interrupted.
  assert.equal(queue.start(2, 'b', false), 'queue')
  assert.equal(queue.start(3, 'c', false), 'queue')
  assert.deepEqual(queue.pending, ['b', 'c'])
  // A's own archive announcement is ignored while it plays and after it finished.
  assert.equal(queue.start(4, 'a', false), 'ignore')
  queue.end('a', 'played', true)
  assert.equal(queue.current, '')
  assert.equal(queue.start(5, 'a', true), 'ignore')
  // B's archive arrives: B is the head, so it plays; C stays owed, never skipped.
  assert.equal(queue.start(6, 'c', true), 'queue')
  assert.equal(queue.start(7, 'b', true), 'play')
  assert.deepEqual(queue.pending, ['c'])
  queue.end('b', 'played', false)
  assert.equal(queue.start(8, 'c', true), 'play')
  assert.deepEqual(queue.pending, [])
})

test('an idle page with owed messages queues a newcomer behind them', () => {
  const queue = new SpeechQueue()
  queue.offer('a')
  assert.equal(queue.start(1, 'b', true), 'queue')
  assert.deepEqual(queue.pending, ['a', 'b'])
})

test('duplicates are ignored across start, ready and recovery sources', () => {
  const queue = new SpeechQueue()
  assert.equal(queue.start(1, 'a', false), 'queue')
  assert.equal(queue.offer('a'), false)
  assert.equal(queue.start(2, 'a', false), 'queue')
  assert.deepEqual(queue.pending, ['a'])
  queue.begin('a')
  assert.equal(queue.offer('a'), false)
  queue.end('a', 'played', false)
  // Reconnect recovery must not replay what this page already finished.
  assert.equal(queue.offer('a'), false)
  assert.deepEqual(queue.pending, [])
})

test('only natural completion or explicit stop settles; a failed realtime attempt stays owed at the head', () => {
  const queue = new SpeechQueue()
  queue.start(1, 'a', true)
  queue.offer('b')
  queue.end('a', 'failed', true)
  assert.deepEqual(queue.pending, ['a', 'b'])
  assert.equal(queue.start(2, 'a', true), 'play')
  // The file attempt failing does not loop forever.
  queue.end('a', 'failed', false)
  assert.deepEqual(queue.pending, ['b'])
  assert.equal(queue.offer('a'), false)
  queue.begin('b')
  queue.end('b', 'stopped', false)
  assert.equal(queue.offer('b'), false)
})

test('a stale end never settles or clears the successor', () => {
  const queue = new SpeechQueue()
  queue.start(1, 'a', true)
  queue.begin('b')
  queue.end('a', 'stopped', false)
  assert.equal(queue.current, 'b')
  assert.equal(queue.offer('b'), false)
  queue.end('b', 'played', false)
  assert.equal(queue.current, '')
})

test('control frames map epochs to messages; cancel dismisses a queued message', () => {
  const queue = new SpeechQueue()
  queue.start(1, 'a', true)
  queue.start(2, 'b', false)
  assert.equal(queue.message(1), 'a')
  assert.equal(queue.message(2), 'b')
  assert.equal(queue.message(3), '')
  queue.dismiss(queue.message(2))
  assert.deepEqual(queue.pending, [])
  assert.equal(queue.start(3, 'b', true), 'ignore')
})

test('epochs ignore replays and invalid values until a new connection resets them', () => {
  const queue = new SpeechQueue()
  for (const value of [-1, NaN, Infinity, 1.2]) assert.equal(queue.start(value, 'x', true), 'ignore')
  assert.equal(queue.start(10, 'a', false), 'queue')
  assert.equal(queue.start(10, 'b', false), 'ignore')
  assert.equal(queue.start(9, 'b', false), 'ignore')
  // A restarted server counts epochs from 1 again.
  queue.resetEpochs()
  assert.equal(queue.message(10), '')
  assert.equal(queue.start(1, 'b', false), 'queue')
  assert.deepEqual(queue.pending, ['a', 'b'])
})

test('queue memory is bounded', () => {
  const queue = new SpeechQueue(4)
  for (let i = 0; i < 6; i++) queue.offer(String(i))
  assert.deepEqual(queue.pending, ['2', '3', '4', '5'])
  for (let i = 0; i < 40; i++) queue.dismiss(`done-${i}`)
  assert.equal(queue.offer('done-0'), true)
  assert.equal(queue.offer('done-39'), false)
})

test('speech available amount deducts reservations and preserves unlimited versus zero', () => {
  assert.equal(speechAvailableAmount(10, 3, 2), 5)
  assert.equal(speechAvailableAmount(10, 8, 5), 0)
  assert.equal(speechAvailableAmount(0, 0, 0), 0)
  assert.equal(speechAvailableAmount(null, 100, 50), null)
})
