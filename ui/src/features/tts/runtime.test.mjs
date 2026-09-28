import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

// Compile only the pure TypeScript module; no app, DOM or heavyweight test runner.
const source = readFileSync(new URL('./runtime.ts', import.meta.url), 'utf8')
const { outputText, diagnostics } = ts.transpileModule(source, { reportDiagnostics: true, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } })
assert.deepEqual(diagnostics, [], 'pure runtime module must have no syntax diagnostics')
const { SpeechEpoch, SpeechRequestKeys, SpeechPlaybackOrder, speechAvailableAmount } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

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

test('file playback keeps the active utterance when the next arrives too early', () => {
  const order = new SpeechPlaybackOrder()
  assert.equal(order.start(1, false), 'play')
  assert.equal(order.start(2, true), 'skip')
  assert.equal(order.cancel(2), false)
  assert.equal(order.cancel(1), true)
  assert.equal(order.start(2, false), 'ignore')
  assert.equal(order.start(3, false), 'play')
  assert.equal(order.cancel(1), false)
  assert.equal(order.cancel(3), true)
})

test('file playback ignores repeated, reordered and invalid epochs', () => {
  const order = new SpeechPlaybackOrder()
  for (const value of [-1, NaN, Infinity, 1.2]) assert.equal(order.start(value, false), 'ignore')
  assert.equal(order.start(10, false), 'play')
  assert.equal(order.start(10, false), 'ignore')
  assert.equal(order.start(9, false), 'ignore')
})

test('speech available amount deducts reservations and preserves unlimited versus zero', () => {
  assert.equal(speechAvailableAmount(10, 3, 2), 5)
  assert.equal(speechAvailableAmount(10, 8, 5), 0)
  assert.equal(speechAvailableAmount(0, 0, 0), 0)
  assert.equal(speechAvailableAmount(null, 100, 50), null)
})
