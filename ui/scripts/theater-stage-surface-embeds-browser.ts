// A real browser/DOM + Konva fixture for the production frame, surface adapter,
// render-band ordering and native pointer ownership. IForm's service is stubbed here;
// authentication/Bridge behavior remains covered by its existing implementation.
import { createApp, h, nextTick, reactive } from 'vue'
import Konva from 'konva'
import StageSurfaceIframe from '../src/views/theater/stage/StageSurfaceIframe.vue'
import StageTextOverlay from '../src/views/theater/stage/StageTextOverlay.vue'
import { planStageObjectRenderBands } from '../src/views/theater/stage/stage-layer-order'
import { chat } from '@/stores/chat'

const assert = (condition: unknown, message: string) => { if (!condition) throw new Error(message) }
const origin = location.origin
const config = () => ({ type: 'iframe' as const, iframe: { url: `${origin}/page`, scale: 1 }, interactive: true })
const state = reactive({
  sceneId: 'scene-a', worldId: 'world', channelId: 'channel', editing: true,
  test: null as 'background' | 'foreground' | null,
  camera: { x: 0, y: 0, zoom: 1 }, fieldWidth: 8, fieldHeight: 6,
  embeds: { background: config() as ReturnType<typeof config> | null, foreground: null as ReturnType<typeof config> | null },
})
const snapshot = { revision: 0, updatedAt: 0, activeIdentityId: null, characters: [] }
const object = {
  id: 'object-iframe', type: 'iframe', parentId: null, name: 'Object frame',
  transform: { x: 0, y: 0, width: 3, height: 2, scaleX: 1, scaleY: 1, rotation: 0, z: 1, order: 1 },
  visible: true, interactive: true, content: { iframe: { url: `${origin}/page`, scale: 1 } }, metadata: {},
}
const plan = planStageObjectRenderBands({ [object.id]: object } as never)
const interactive = (target: 'background' | 'foreground') => state.editing ? state.test === target : state.embeds[target]?.interactive === true
let stage: Konva.Stage, world: Konva.Group, foreground: Konva.Group, controls: Konva.Layer
const app = createApp({ render: () => h('div', { style: 'position:relative;width:300px;height:200px;isolation:isolate;overflow:hidden' }, [
  h('div', { class: 'scene-visual', style: 'position:absolute;inset:0;z-index:0;overflow:hidden' }, [
    h('div', { id: 'stage', style: 'position:absolute;inset:0' }),
    ...(['background', 'foreground'] as const).map(target => state.embeds[target] && h(StageSurfaceIframe, {
      key: `${state.worldId}:${state.channelId}:${state.sceneId}:${target}`, target, embed: state.embeds[target]!,
      interactive: interactive(target), camera: state.camera, viewportWidth: 300, viewportHeight: 200,
      fieldWidth: state.fieldWidth, fieldHeight: state.fieldHeight, characterSnapshot: snapshot as never,
      worldId: state.worldId, channelId: state.channelId, style: { zIndex: target === 'background' ? 5 : 9991 },
    })),
    h(StageTextOverlay, { objects: { [object.id]: object } as never, camera: state.camera, viewportWidth: 300, viewportHeight: 200,
      stackingOrder: plan.rootStackingOrder, entrancePlaybacks: {}, hiddenObjectIds: [], characterSnapshot: snapshot as never }),
  ]),
  state.test && h('button', { id: 'exit', style: 'position:absolute;top:0;right:0;z-index:10005', onClick: () => { state.test = null } }, 'Exit webpage test'),
]) })
const refresh = async () => {
  await nextTick()
  world.position({ x: 150 + state.camera.x, y: 100 + state.camera.y }); world.scale({ x: state.camera.zoom, y: state.camera.zoom })
  foreground.position(world.position()); foreground.scale(world.scale())
  world.getLayer()!.getNativeCanvasElement().style.pointerEvents = interactive('background') ? 'none' : ''
  stage.draw()
  await nextTick()
}
const surfaceFrame = (target: string) => document.querySelector(`[data-stage-surface="${target}"] iframe`) as HTMLIFrameElement | null
const hit = (x: number, y: number) => document.elementFromPoint(x + 8, y + 8)
const bounds = (target: string) => surfaceFrame(target)!.getBoundingClientRect()

async function run() {
  app.mount('#app')
  await nextTick()
  stage = new Konva.Stage({ container: 'stage', width: 300, height: 200 })
  const backgroundLayer = new Konva.Layer({ listening: false })
  const worldLayer = new Konva.Layer()
  const topLayer = new Konva.Layer({ listening: false })
  controls = new Konva.Layer()
  world = new Konva.Group(); foreground = new Konva.Group()
  worldLayer.add(world); topLayer.add(foreground)
  const worldRect = new Konva.Rect({ x: -60, y: -50, width: 40, height: 40, fill: 'red' })
  world.add(worldRect)
  // Both images remain real Canvas surfaces while webpages are DOM siblings.
  backgroundLayer.add(new Konva.Rect({ width: 300, height: 200, fill: 'blue' }))
  foreground.add(new Konva.Rect({ x: -96, y: -72, width: 192, height: 144, fill: 'green', opacity: .2 }))
  const transformer = new Konva.Transformer({ nodes: [worldRect] })
  controls.add(transformer)
  stage.add(backgroundLayer, worldLayer, topLayer, controls)
  backgroundLayer.getNativeCanvasElement().style.zIndex = '0'
  worldLayer.getNativeCanvasElement().style.zIndex = String(plan.bands[0].canvasZIndex)
  topLayer.getNativeCanvasElement().style.zIndex = '9990'; topLayer.getNativeCanvasElement().style.pointerEvents = 'none'
  controls.getNativeCanvasElement().style.zIndex = '10000'; controls.getNativeCanvasElement().style.pointerEvents = 'none'
  await refresh()
  const initialBackground = surfaceFrame('background')!
  const ordinary = document.querySelector('iframe[data-stage-object-id]')!
  assert(initialBackground && ordinary, 'surface and ordinary object are real DOM iframes')
  assert(initialBackground.closest('[inert]'), 'GM default blocks iframe focus')
  assert(hit(15, 180) instanceof HTMLCanvasElement, 'GM default keeps stage empty hits')
  assert(hit(150, 100) === ordinary, 'ordinary iframe remains above background')
  const saved = JSON.stringify(state.embeds)
  state.test = 'background'; await refresh()
  assert(JSON.stringify(state.embeds) === saved, 'test state is local, does not modify interactive config')
  assert(hit(15, 180) === initialBackground, 'background accepts native input in empty world areas')
  assert(hit(100, 60) === initialBackground, 'interactive background owns pointer input over canvas world visuals')
  assert(hit(150, 100) === ordinary, 'ordinary DOM object remains above background in test')
  assert(stage.getLayers().length === 4, 'surface adds no Konva Layers')
  document.querySelector<HTMLButtonElement>('#exit')!.click(); await refresh()
  assert(surfaceFrame('background') === initialBackground && !state.test, 'exit restores editing without reloading')
  assert(hit(100, 60) instanceof HTMLCanvasElement, 'exit restores native stage pointer input')
  state.editing = false; await refresh()
  assert(hit(15, 180) === initialBackground, 'player interactive config enables background')
  state.embeds.background!.interactive = false; await refresh()
  assert(hit(15, 180) instanceof HTMLCanvasElement, 'blocked background passes input to stage')
  assert(surfaceFrame('background') === initialBackground, 'interactive toggle keeps frame identity')
  state.embeds.foreground = config(); await refresh()
  const initialForeground = surfaceFrame('foreground')!
  assert(hit(150, 100) === initialForeground, 'foreground webpage is above ordinary iframe/world')
  assert(hit(90, 50) === initialForeground, 'interactive foreground owns pointer input inside the field')
  assert(hit(15, 180) instanceof HTMLCanvasElement, 'outside foreground field passes to stage')
  const backgroundBounds = bounds('background'), foregroundBounds = bounds('foreground')
  assert(Math.round(foregroundBounds.width) === 192 && Math.round(foregroundBounds.height) === 144, 'foreground uses field dimensions')
  state.camera = { x: 20, y: 30, zoom: 1.5 }; await refresh()
  assert(bounds('background').x === backgroundBounds.x && bounds('background').width === backgroundBounds.width, 'background stays in viewport coordinates')
  assert(Math.round(bounds('foreground').width) === 288 && Math.round(bounds('foreground').x) === 34, 'foreground follows camera pan and zoom')
  assert(surfaceFrame('foreground') === initialForeground, 'camera and other surface edits preserve frames')
  state.embeds.foreground!.interactive = false; await refresh()
  assert(getComputedStyle(initialForeground).pointerEvents === 'none', 'blocked frame pointer-events none')
  state.sceneId = 'scene-b'; await refresh()
  assert(!initialForeground.isConnected && !initialBackground.isConnected, 'scene switch destroys outgoing frames')
  state.embeds.foreground = null; await refresh()
  assert(!surfaceFrame('foreground'), 'remove destroys iframe')
  // Internal links still require both the actual chat context and theater context.
  // Use the parser's actual link generator rather than relying on URL shape.
  const { generateInternalSurfaceLink } = await import('../src/utils/internalSurfaceLink')
  state.embeds.background!.iframe.url = generateInternalSurfaceLink({ type: 'iform', worldId: 'world', channelId: 'channel', id: 'form' }, { base: origin })
  await refresh(); await refresh()
  assert((window as any).iformMounts === 1, 'internal IForm uses shared loader/component')
  ;(chat as any).curChannel.id = 'other'; await refresh()
  assert((window as any).iformStops === 1 && !surfaceFrame('background'), 'context change unmounts IForm/Bridge owner')
  app.unmount(); stage.destroy()
  assert(!document.querySelector('iframe'), 'unmount removes frames')
  document.querySelector('#result')!.textContent = 'PASS: surface DOM layering, pointer ownership, camera, iframe identity, local test state, IForm context and cleanup'
}
run().catch(error => { document.querySelector('#result')!.textContent = String(error.stack || error) })
