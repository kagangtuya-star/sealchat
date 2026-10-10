import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync('src/views/theater/stage/StageApp.vue', 'utf8')

test('render-band canvases leave DOM pointer hit-testing to the overlay', () => {
  const creationBlock = source.match(/while \(extraObjectRenderBands\.length < extraBandCount\) \{([\s\S]*?)\n  \}/)?.[1]

  assert.ok(creationBlock)
  assert.match(
    creationBlock,
    /const layer = new Konva\.Layer\(\)[\s\S]*layer\.getNativeCanvasElement\(\)\.style\.pointerEvents = 'none'[\s\S]*stage\.add\(layer\)/,
  )
  assert.match(source, /topVisualLayer\.getNativeCanvasElement\(\)\.style\.pointerEvents = 'none'/)
  assert.match(source, /interactionLayer\.getCanvas\(\)\._canvas\.style\.pointerEvents = 'none'/)
  assert.match(
    source,
    /entry\.layer\.getNativeCanvasElement\(\)\.style\.zIndex = String\(band\.canvasZIndex\)/,
  )
})
