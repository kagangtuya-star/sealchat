import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync('src/views/theater/stage/StageApp.vue', 'utf8')

test('root object canvas leaves DOM pointer hit-testing when the layer is created', () => {
  const creationBlock = source.match(/if \(!entry\) \{([\s\S]*?)\n    \}/)?.[1]

  assert.ok(creationBlock)
  assert.match(
    creationBlock,
    /const layer = new Konva\.Layer\(\)[\s\S]*layer\.getNativeCanvasElement\(\)\.style\.pointerEvents = 'none'[\s\S]*stage!\.add\(layer\)/,
  )
  assert.equal(
    source.match(/getNativeCanvasElement\(\)\.style\.pointerEvents = 'none'/g)?.length,
    1,
  )
  assert.match(
    source,
    /entry\.layer\.getNativeCanvasElement\(\)\.style\.zIndex = String\(canvasZIndex\)/,
  )
})
