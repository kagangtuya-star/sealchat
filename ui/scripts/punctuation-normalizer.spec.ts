import assert from 'node:assert/strict'
import fixtureCases from '../../testdata/punctuation_normalizer_cases.json'

import {
  normalizeChinesePunctuation,
  normalizePunctuationForMessageSend,
} from '../src/utils/punctuationNormalizer'
import { buildAutoCorrectPunctuationExportPayload } from '../src/utils/exportPunctuationOption'

interface TestCase {
  input: string
  output: string
}

const fixture = fixtureCases as { version: number; profile: string; cases: TestCase[] }
assert.equal(fixture.version, 1)
assert.equal(fixture.profile, 'sealchat')
const cases = fixture.cases

for (const testCase of cases) {
  const normalized = normalizeChinesePunctuation(testCase.input)
  assert.equal(normalized, testCase.output, testCase.input)
  assert.equal(normalizeChinesePunctuation(normalized), normalized, `idempotency: ${testCase.input}`)
}

const outgoing = '今天很好,真的.'
assert.equal(normalizePunctuationForMessageSend(outgoing, true, 'plain'), '今天很好，真的。')
assert.equal(normalizePunctuationForMessageSend(outgoing, false, 'plain'), outgoing)
assert.equal(normalizePunctuationForMessageSend(outgoing, true, 'rich'), outgoing)
assert.equal(normalizePunctuationForMessageSend(outgoing, true, 'plain', true), outgoing)
assert.equal(normalizePunctuationForMessageSend(outgoing, true, 'plain', false, true), outgoing)
assert.equal(normalizeChinesePunctuation('[[图片:marker_1]]你好!'), '[[图片:marker_1]]你好！')

assert.equal(
  JSON.stringify(buildAutoCorrectPunctuationExportPayload(false)),
  '{"auto_correct_punctuation":false}',
)
assert.equal(buildAutoCorrectPunctuationExportPayload(undefined).auto_correct_punctuation, true)

for (const input of ['', '👨‍👩‍👧‍👦', 'e\u0301', '第一行,好.\r\n\t第二行,好.']) {
  const normalized = normalizeChinesePunctuation(input)
  assert.equal(normalizeChinesePunctuation(normalized), normalized)
  assert.equal((normalized.match(/\n/g) || []).length, (input.match(/\n/g) || []).length)
  assert.equal((normalized.match(/\r/g) || []).length, (input.match(/\r/g) || []).length)
}

console.log('punctuation normalizer runtime tests passed')
