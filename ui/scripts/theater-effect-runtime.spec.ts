import assert from 'node:assert/strict'
import { reactive } from 'vue'

import type { StageObject } from '../src/views/theater/shared/stage-types'
import type { TheaterDialogueMessage } from '../src/views/theater/bridge/theater-dialogue-queue'
import { TheaterDialogueRuntime } from '../src/views/theater/dialogue/theater-dialogue-runtime'
import { TheaterEffectRuntime, theaterEffectMatchesMessage } from '../src/views/theater/effects/theater-effect-runtime'
import { createDefaultTheaterEffectConfig } from '../src/views/theater/effects/theater-effect-types'

class FakeScheduler {
  nowValue = 1_000
  nextId = 1
  timers = new Map<number, { at: number, callback: () => void }>()

  now = () => this.nowValue
  setTimeout = (callback: () => void, delayMs: number) => {
    const id = this.nextId++
    this.timers.set(id, { at: this.nowValue + delayMs, callback })
    return id as unknown as ReturnType<typeof setTimeout>
  }
  clearTimeout = (timer: ReturnType<typeof setTimeout>) => {
    this.timers.delete(timer as unknown as number)
  }
  tick(durationMs: number) {
    this.nowValue += durationMs
    const due = [...this.timers.entries()].filter(([, timer]) => timer.at <= this.nowValue)
    due.forEach(([id, timer]) => {
      this.timers.delete(id)
      timer.callback()
    })
  }
}

const message = (messageId: string, contentText: string, displayName = 'Actor'): TheaterDialogueMessage => ({
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
    displayName,
    color: '#fff',
    appearance: {},
  },
})

const effectObject = (id: string, keywords: string[], targetActorName: string | null = null): StageObject => {
  const effect = createDefaultTheaterEffectConfig()
  effect.keywords = keywords
  effect.targetActorName = targetActorName
  effect.durationMs = 1_000
  effect.cooldownMs = 2_000
  return {
    id,
    parentId: null,
    type: 'effect',
    name: id,
    transform: { x: 960, y: 540, width: 1600, height: 900, rotation: 0, scaleX: 1, scaleY: 1, z: 0, order: 0 },
    visible: true,
    locked: false,
    aspectRatioLocked: false,
    interactive: false,
    editable: false,
    fill: '#fff',
    content: { effect },
    actions: [],
    metadata: {},
  }
}

const config = createDefaultTheaterEffectConfig()
config.keywords = ['Critical Hit']
assert.equal(theaterEffectMatchesMessage(config, message('match', 'A CRITICAL HIT!')), true)
config.targetActorName = 'Mage'
assert.equal(theaterEffectMatchesMessage(config, message('wrong-actor', 'critical hit')), false)
assert.equal(theaterEffectMatchesMessage(config, message('right-actor', 'critical hit', 'Mage')), true)
config.targetActorName = null
assert.equal(theaterEffectMatchesMessage(config, message('all-actors', 'critical hit', 'Warrior')), true)

const scheduler = new FakeScheduler()
const dialogueRuntime = new TheaterDialogueRuntime({ reducedMotion: true })
const objects = [effectObject('global', ['爆击']), effectObject('actor-only', ['爆击'], '法师')]
const runtime = new TheaterEffectRuntime({ dialogueRuntime, getObjects: () => objects, scheduler })

dialogueRuntime.created(message('one', '发生爆击'))
assert.deepEqual(runtime.getActive().map((item) => item.effectId), ['global'])

dialogueRuntime.updated(message('one', '发生爆击并更新'))
assert.deepEqual(runtime.getActive().map((item) => item.effectId), ['global'])

scheduler.tick(1_000)
assert.deepEqual(runtime.getActive(), [])

dialogueRuntime.reset()
dialogueRuntime.created(message('two', '再次爆击'))
assert.deepEqual(runtime.getActive(), [])

scheduler.tick(1_001)
dialogueRuntime.reset()
dialogueRuntime.created(message('three', '角色爆击', '法师'))
assert.deepEqual(runtime.getActive().map((item) => item.effectId), ['global', 'actor-only'])

const previewSource = effectObject('preview', [])
previewSource.image = {
  resourceId: 'preview-resource',
  url: 'https://example.com/effect.webp',
  mimeType: 'image/webp',
}
const previewConfig = previewSource.content.effect as ReturnType<typeof createDefaultTheaterEffectConfig>
previewConfig.media = previewSource.image
const previewObject = reactive(previewSource)
assert.doesNotThrow(() => runtime.preview(previewObject))
assert.equal(runtime.getActive().at(-1)?.effectId, 'preview')

const lateScheduler = new FakeScheduler()
const lateDialogueRuntime = new TheaterDialogueRuntime({ reducedMotion: true })
const lateObjects: StageObject[] = []
const lateRuntime = new TheaterEffectRuntime({
  dialogueRuntime: lateDialogueRuntime,
  getObjects: () => lateObjects,
  scheduler: lateScheduler,
})
lateDialogueRuntime.created(message('late', '远端爆击'))
assert.deepEqual(lateRuntime.getActive(), [])
lateObjects.push(effectObject('late-effect', ['爆击']))
lateRuntime.reconcile()
assert.deepEqual(lateRuntime.getActive().map((item) => item.effectId), ['late-effect'])
lateRuntime.reconcile()
assert.deepEqual(lateRuntime.getActive().map((item) => item.effectId), ['late-effect'])
lateRuntime.dispose()
lateDialogueRuntime.dispose()

objects.splice(0, objects.length)
runtime.reconcile()
assert.deepEqual(runtime.getActive(), [])

runtime.dispose()
dialogueRuntime.dispose()

console.log('theater effect runtime tests passed')
