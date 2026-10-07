// Run with node scripts/theater-stage-render-bands-regression.cjs.
// Reuses the local TypeScript/SFC extraction pattern of the surface filter regression.
// Real Konva Stage/Layer/Group topology; canvas pixels and browser events are not rendered.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const { parse } = require('@vue/compiler-sfc')

const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const stageDir = path.join(__dirname, '../src/views/theater/stage')
const order = {}
new Function('exports', compile(fs.readFileSync(path.join(stageDir, 'stage-layer-order.ts'), 'utf8')))(order)
const hierarchy = {}
new Function('exports', 'require', compile(fs.readFileSync(path.join(stageDir, 'stage-layering.ts'), 'utf8')))(hierarchy, () => order)
const { planStageObjectRenderBands: plan, stageObjectHasDomVisualDescendant: hasDom } = order
const object = (id, z, type = 'image', parentId = null) => ({
  id, type, parentId, visible: true,
  transform: { z, order: z, x: 3, y: 4, scaleX: 1.5, scaleY: 0.75, rotation: 25 },
})
const objectsOf = (...objects) => Object.fromEntries(objects.map(value => [value.id, value]))
const ids = bands => bands.map(band => band.roots.map(root => root.id))

assert.deepEqual(ids(plan(objectsOf(object('C', 3), object('A', 1), object('B', 2))).bands), [['A', 'B', 'C']])
assert.deepEqual(ids(plan(objectsOf(object('A', 1), object('B', 2, 'text'), object('C', 3))).bands), [['A', 'B'], ['C']])
assert.deepEqual(ids(plan(objectsOf(object('A', 1, 'text'), object('B', 2, 'iframe'), object('C', 3))).bands), [['A'], ['B'], ['C']])
assert.deepEqual(ids(plan(objectsOf(object('A', 1), object('B', 2, 'text'))).bands), [['A', 'B']], 'no trailing empty band')
assert.deepEqual(ids(plan({}).bands), [[]], 'world band always exists')
for (const type of ['text', 'iframe', 'image']) {
  const group = object('B', 2, 'group')
  const tree = objectsOf(object('A', 1), group, object('C', 3), object('nested', 0, 'group', 'B'), object('child', 0, type, 'nested'))
  assert.equal(hasDom(group, tree), type !== 'image')
  assert.deepEqual(ids(plan(tree).bands), type === 'image' ? [['A', 'B', 'C']] : [['A', 'B'], ['C']])
  delete tree.child
  assert.deepEqual(ids(plan(tree).bands), [['A', 'B', 'C']], 'deleted descendant removes boundary')
}
const ties = objectsOf(object('C', 1), object('B', 1), object('A', 1))
ties.A.transform.order = 2
assert.deepEqual(ids(plan(ties).bands), [['B', 'C', 'A']], 'z, order, then ID')
const cycle = objectsOf(object('group', 0, 'group', 'child'), object('child', 0, 'group', 'group'))
assert.equal(hasDom(cycle.group, cycle), false, 'cycle guard')

// Exhaust every DOM/canvas combination up to eight roots against the old per-root z model.
for (let length = 1; length <= 8; length++) {
  for (let mask = 0; mask < 2 ** length; mask++) {
    const roots = Array.from({ length }, (_, index) => object(String(index), index, mask & (1 << index) ? 'text' : 'image'))
    const result = plan(objectsOf(...roots.reverse()))
    const bandFor = new Map(result.bands.flatMap(band => band.roots.map(root => [root.id, band])))
    for (let index = 0; index < length; index++) {
      assert.equal(result.rootStackingOrder[index], 101 + index * 2, 'unique logical root stacking order')
      if (!(mask & (1 << index))) continue
      const domZ = result.rootStackingOrder[index]
      for (let canvasIndex = 0; canvasIndex < length; canvasIndex++) {
        assert.equal(bandFor.get(String(canvasIndex)).canvasZIndex < domZ, canvasIndex <= index, 'same canvas/DOM ordering as before')
      }
    }
  }
}

async function run() {
  const Konva = (await import('konva')).default
  // No native canvas dependency required: rendering calls are observed, not simulated.
  Konva.Util.createCanvasElement = () => ({ style: {}, getContext: () => ({ scale() {}, clearRect() {} }) })
  Konva.autoDrawEnabled = false
  const draws = [], batches = [], destroyed = []
  Konva.Layer.prototype.draw = function () { draws.push(this); return this }
  Konva.Layer.prototype.batchDraw = function () { batches.push(this); return this }
  const originalDestroy = Konva.Layer.prototype.destroy
  Konva.Layer.prototype.destroy = function () {
    assert.equal(this.find(node => Boolean(node.getAttr('stageObjectId'))).length, 0, 'no live subtree destroyed with surplus band')
    destroyed.push(this)
    return originalDestroy.call(this)
  }
  Konva.Animation.prototype.start = function () { this.starts = (this.starts || 0) + 1; return this }
  Konva.Animation.prototype.stop = function () { this.stops = (this.stops || 0) + 1; return this }

  const script = parse(fs.readFileSync(path.join(stageDir, 'StageApp.vue'), 'utf8')).descriptor.scriptSetup.content
  const ast = ts.createSourceFile('StageApp.ts', script, ts.ScriptTarget.Latest, true)
  const declaration = name => {
    const statement = ast.statements.find(node => ts.isVariableStatement(node)
      && node.declarationList.declarations.some(value => value.name.getText(ast) === name))
    assert.ok(statement, `missing StageApp declaration ${name}`)
    return statement.getText(ast)
  }
  const mounted = ast.statements.find(node => ts.isExpressionStatement(node)
    && ts.isCallExpression(node.expression) && node.expression.expression.getText(ast) === 'onMounted')
  const initNames = new Set([
    'stage', 'backgroundLayer', 'worldLayer', 'topVisualLayer', 'interactionLayer',
    'backgroundCameraGroup', 'worldCameraGroup', 'worldOverlayCameraGroup', 'foregroundCameraGroup',
    'gridTopCameraGroup', 'gridGroup', 'objectRoot', 'drawingDraftRoot', 'pointerTraceRoot',
  ])
  // Execute the actual main-Stage construction/add/style statements, excluding unrelated UI setup.
  const initialization = mounted.expression.arguments[0].body.statements.filter(node => {
    if (!ts.isExpressionStatement(node)) return false
    const expression = node.expression
    if (ts.isBinaryExpression(expression)) {
      return initNames.has(expression.left.getText(ast)) || [...initNames].some(name => expression.left.getText(ast).startsWith(name + '.'))
    }
    return ts.isCallExpression(expression) && [...initNames].some(name => expression.expression.getText(ast) === name + '.add')
  }).map(node => node.getText(ast)).join('\n')
  const names = [...initNames, 'extraObjectRenderBands', 'rootStackingOrder', 'TOP_VISUAL_LAYER_Z',
    'drawWorldLayers', 'drawTopVisualLayer', 'syncMediaAnimation', 'syncObjectRenderBands', 'applyCamera',
    'getMainStageLayerCount', 'resolveObjectActionTarget', 'stageObjectIdFromTarget', 'syncGridLayer']
  const runtime = new Function('Konva', 'planStageObjectRenderBands', 'syncStageObjectHierarchy', compile(`
    const ref = value => ({ value })
    const containerRef = { value: {} }
    const props = { store: { state: { camera: { x: 10, y: -20, zoom: 2 }, liveState: { gridOnTop: false } } } }
    const objectNodes = new Map()
    const activeAnimatedMedia = new Set()
    let mediaAnimation = null
    const sceneMorphLayers = new Map(), sceneMorphTextCameras = new Map()
    const selectionRect = new Konva.Rect(), quickDeleteOutline = new Konva.Rect()
    const selectionGroupHitArea = new Konva.Rect(), transformer = new Konva.Group()
    let objects = {}
    const getObject = id => objects[id]
    const canInteractObject = object => Boolean(object)
    const objectNodeIntersectsStagePoint = () => true
    ${names.map(declaration).join('\n')}
    ${initialization}
    applyCamera()
    return {
      stage, backgroundLayer, backgroundCameraGroup, worldLayer, topVisualLayer, interactionLayer, objectRoot, worldCameraGroup,
      worldOverlayCameraGroup, foregroundCameraGroup, gridTopCameraGroup, gridGroup, props, objectNodes,
      extraObjectRenderBands, rootStackingOrder, drawWorldLayers, drawTopVisualLayer, applyCamera, getMainStageLayerCount,
      resolveObjectActionTarget, stageObjectIdFromTarget, syncGridLayer,
      animation: () => mediaAnimation,
      animate: () => { activeAnimatedMedia.add({}); syncMediaAnimation() },
      sync: next => {
        objects = next
        for (const object of Object.values(next)) {
          if (objectNodes.has(object.id)) continue
          const node = new Konva.Group({ ...object.transform, stageObjectId: object.id })
          node.add(new Konva.Rect({ width: 10, height: 10 }))
          objectNodes.set(object.id, node)
        }
        for (const [id, node] of objectNodes) {
          if (!next[id]) { node.destroy(); objectNodes.delete(id) }
        }
        syncStageObjectHierarchy(next, objectNodes, objectRoot)
        syncObjectRenderBands(next)
      },
    }
  `))(Konva, plan, hierarchy.syncStageObjectHierarchy)
  const canvasRoots = Object.fromEntries(Array.from({ length: 18 }, (_, index) => {
    const root = object(`image-${index}`, index)
    return [root.id, root]
  }))
  runtime.sync(canvasRoots)
  assert.equal(runtime.getMainStageLayerCount(), 4, '18 pure images: 24 -> 4 actual Layers')
  for (let index = 18; index < 118; index++) canvasRoots[`image-${index}`] = object(`image-${index}`, index)
  runtime.sync(canvasRoots)
  assert.equal(runtime.getMainStageLayerCount(), 4, '100 more canvas roots create no Layers')
  assert.equal(runtime.extraObjectRenderBands.length, 0)
  assert.equal(runtime.objectRoot.getParent(), runtime.worldCameraGroup)
  assert.deepEqual(runtime.topVisualLayer.getChildren(), [runtime.worldOverlayCameraGroup, runtime.foregroundCameraGroup, runtime.gridTopCameraGroup])
  assert.equal(runtime.topVisualLayer.listening(), false)

  runtime.sync(objectsOf(object('A', 1), object('B', 2, 'text'), object('C', 3)))
  assert.equal(runtime.getMainStageLayerCount(), 5)
  const band = runtime.extraObjectRenderBands[0]
  const c = runtime.objectNodes.get('C')
  const before = c.getAbsoluteTransform().getMatrix().slice()
  const content = c.getChildren()[0]
  let clicks = 0
  c.on('click', () => clicks++)
  runtime.animate()
  const animation = runtime.animation()
  assert.deepEqual(animation.getLayers(), [runtime.backgroundLayer, runtime.worldLayer, band.layer, runtime.topVisualLayer])
  assert.deepEqual(runtime.rootStackingOrder.value, { A: 101, B: 103, C: 105 })
  assert.equal(runtime.worldLayer.getNativeCanvasElement().style.zIndex, '100')
  assert.equal(band.layer.getNativeCanvasElement().style.zIndex, '104')
  assert.ok(runtime.objectNodes.get('A').getAbsoluteZIndex() < runtime.objectNodes.get('B').getAbsoluteZIndex())
  assert.ok(runtime.objectNodes.get('B').getAbsoluteZIndex() < c.getAbsoluteZIndex())
  assert.equal(runtime.resolveObjectActionTarget({ x: 0, y: 0 }).id, 'C')
  assert.equal(runtime.stageObjectIdFromTarget(content), 'C')

  // Reorder into Band 0 without changing the Layer set or node/event/content identity.
  runtime.sync(objectsOf(object('A', 1), object('C', 1.5), object('B', 2, 'text'), object('D', 3)))
  assert.equal(runtime.extraObjectRenderBands[0], band)
  assert.equal(runtime.animation(), animation, 'same topology reuses Animation')
  assert.equal(c.getParent(), runtime.objectRoot)
  assert.deepEqual(c.getAbsoluteTransform().getMatrix(), before)
  assert.equal(c.getChildren()[0], content)
  c.fire('click')
  assert.equal(clicks, 1)
  assert.deepEqual(runtime.objectRoot.getChildren().map(node => node.getAttr('stageObjectId')), ['A', 'C', 'B'])
  assert.equal(runtime.resolveObjectActionTarget({ x: 0, y: 0 }).id, 'D')

  draws.length = batches.length = 0
  runtime.drawWorldLayers(true)
  assert.deepEqual(draws, [runtime.worldLayer, band.layer], 'world redraw excludes expensive top visuals')
  runtime.drawTopVisualLayer(true)
  assert.deepEqual(draws, [runtime.worldLayer, band.layer, runtime.topVisualLayer])
  runtime.drawWorldLayers()
  assert.deepEqual(batches, [runtime.worldLayer, band.layer], 'batched world redraw excludes top visuals')
  runtime.drawTopVisualLayer()
  assert.deepEqual(batches, [runtime.worldLayer, band.layer, runtime.topVisualLayer])
  const backgroundTransform = runtime.backgroundCameraGroup.getAbsoluteTransform().getMatrix().slice()
  runtime.props.store.state.camera = { x: -100, y: 30, zoom: 0.5 }
  runtime.applyCamera()
  for (const camera of [band.camera, runtime.worldOverlayCameraGroup, runtime.foregroundCameraGroup, runtime.gridTopCameraGroup]) {
    assert.deepEqual(camera.position(), runtime.worldCameraGroup.position())
    assert.deepEqual(camera.scale(), runtime.worldCameraGroup.scale())
  }
  assert.ok(!batches.includes(runtime.backgroundLayer), 'camera does not redraw background')
  assert.deepEqual(runtime.backgroundCameraGroup.getAbsoluteTransform().getMatrix(), backgroundTransform)
  runtime.props.store.state.liveState.gridOnTop = true
  runtime.syncGridLayer()
  assert.equal(runtime.gridGroup.getParent(), runtime.gridTopCameraGroup)
  runtime.props.store.state.liveState.gridOnTop = false
  runtime.syncGridLayer()
  assert.equal(runtime.gridGroup.getParent(), runtime.worldCameraGroup)
  assert.equal(runtime.gridGroup.zIndex(), 0)

  const tree = objectsOf(object('A', 1), object('B', 2, 'group'), object('C', 3, 'group'), object('nested', 0, 'group', 'B'), object('child', 0, 'iframe', 'nested'))
  runtime.sync(tree)
  assert.equal(runtime.getMainStageLayerCount(), 5)
  assert.equal(runtime.objectNodes.get('child').getParent(), runtime.objectNodes.get('nested'))
  assert.equal(runtime.objectNodes.get('nested').getParent(), runtime.objectNodes.get('B'))
  tree.child.parentId = 'C'
  runtime.sync(tree)
  assert.equal(runtime.getMainStageLayerCount(), 4, 'moving DOM child to the last root removes boundary')
  assert.ok(destroyed.includes(band.layer))
  assert.ok(animation.stops > 0, 'old Animation stopped before destroying Layer')
  assert.ok(!runtime.animation().getLayers().includes(band.layer))
  tree.child.parentId = 'B'
  runtime.sync(tree)
  const obsolete = runtime.extraObjectRenderBands[0].layer
  delete tree.child
  runtime.sync(tree)
  assert.equal(runtime.getMainStageLayerCount(), 4, 'deleting DOM descendant converges to world band')
  assert.ok(destroyed.includes(obsolete))
  assert.equal(runtime.objectNodes.get('C'), c)
  assert.equal(c.getStage(), runtime.stage)

  runtime.sync(objectsOf(object('A', 1, 'text'), object('B', 2, 'iframe'), object('C', 3)))
  assert.equal(runtime.getMainStageLayerCount(), 6, 'only two real boundaries exceed five Layers')
  assert.equal(runtime.stage.getLayers().at(-1), runtime.interactionLayer)
  assert.ok(runtime.objectNodes.get('A').getAbsoluteZIndex() < runtime.objectNodes.get('B').getAbsoluteZIndex())
  assert.ok(runtime.objectNodes.get('B').getAbsoluteZIndex() < runtime.objectNodes.get('C').getAbsoluteZIndex())
  const liveLayers = runtime.stage.getLayers().slice()
  // Main Stage owns dynamic layers at unmount; allow live node disposal here.
  Konva.Layer.prototype.destroy = function () { destroyed.push(this); return originalDestroy.call(this) }
  runtime.stage.destroy()
  assert.ok(liveLayers.every(layer => destroyed.includes(layer)), 'Stage.destroy cleans all dynamic bands')
  console.log('theater stage render band regression checks passed (18/118 canvas roots: 4 Layers; one boundary: 5)')
}

run().catch(error => { console.error(error); process.exitCode = 1 })
