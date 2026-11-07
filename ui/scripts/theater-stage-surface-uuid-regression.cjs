// node --test scripts/theater-stage-surface-uuid-regression.cjs
const assert = require('node:assert/strict')
const { webcrypto } = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const { test } = require('node:test')
const vm = require('node:vm')
const ts = require('typescript')
const root = path.resolve(__dirname, '..')
const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const read = file => fs.readFileSync(path.join(root, 'src', file), 'utf8')
const run = (source, context) => vm.runInContext(ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText, context)

// Execute the actual component handlers without mounting the unrelated stage UI.
const loadHandler = (file, name, context) => {
  const source = read(file).split('<script setup lang="ts">')[1].split('</script>')[0]
  const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  assert.ok(ast.statements.some(statement => ts.isImportDeclaration(statement)
    && statement.moduleSpecifier.text === '@/utils/uuid'
    && statement.importClause?.namedBindings?.elements?.some(binding => binding.name.text === 'uuid')))
  const statement = ast.statements.find(statement => ts.isVariableStatement(statement)
    && statement.declarationList.declarations.some(declaration => declaration.name.getText(ast) === name))
  assert.ok(statement, `${file} defines ${name}`)
  run(statement.getText(ast), context)
  return vm.runInContext(name, context)
}

for (const native of [true, false]) {
  test(`background/foreground add and copy with randomUUID ${native ? 'available' : 'unavailable'}`, () => {
    let nativeCalls = 0, fallbackCalls = 0
    const crypto = {
      getRandomValues(bytes) {
        assert.equal(this, crypto)
        fallbackCalls++
        return webcrypto.getRandomValues(bytes)
      },
    }
    if (native) crypto.randomUUID = function () {
      assert.equal(this, crypto)
      nativeCalls++
      return webcrypto.randomUUID()
    }
    const operations = []
    const context = vm.createContext({
      crypto, exports: {}, surfaceImageEpoch: 1,
      props: { store: {
        state: { activeSceneId: 'scene' },
        operateSurfaceVariant(target, operation) { operations.push({ target, ...operation }); return true },
      } },
      surfaceVariants: () => ({ slots: [] }),
      emit(event, operation) { assert.equal(event, 'operate'); operations.push(operation) },
    })
    run(read('utils/uuid.ts'), context)
    const add = loadHandler('views/theater/stage/StageApp.vue', 'applyImageUrl', context)
    const select = loadHandler('views/theater/stage/StageSurfaceVariants.vue', 'selectMenu', context)
    for (const target of ['background', 'foreground']) {
      assert.equal(add({ kind: 'scene', sceneId: 'scene', epoch: 1, target, add: true }, `http://example.com/${target}.png`), true)
      const added = operations.at(-1)
      assert.equal(added.type, 'add')
      assert.equal(added.target, target)
      assert.equal(added.image.url, `http://example.com/${target}.png`)
      select('copy', added.id)
      assert.equal(operations.at(-1).type, 'copy')
      assert.equal(operations.at(-1).id, added.id)
    }
    const ids = operations.map(operation => operation.type === 'copy' ? operation.newId : operation.id)
    ids.forEach(id => assert.match(id, uuidV4))
    assert.equal(new Set(ids).size, 4, 'added and copied slots all have different IDs')
    assert.equal(nativeCalls, native ? 4 : 0)
    assert.equal(fallbackCalls, native ? 0 : 4)
  })
}

test('fallback sets UUID v4 version and variant bits for all-zero and all-one bytes', () => {
  for (const [byte, expected] of [
    [0x00, '00000000-0000-4000-8000-000000000000'],
    [0xff, 'ffffffff-ffff-4fff-bfff-ffffffffffff'],
  ]) {
    const context = vm.createContext({ crypto: { getRandomValues: bytes => bytes.fill(byte) }, exports: {} })
    run(read('utils/uuid.ts'), context)
    assert.equal(context.exports.uuid(), expected)
  }
})

test('dialogue controller writes use fallback UUIDs for iframe messages and mutations', async () => {
  const messages = [], mutations = []
  let receiveMessage = null
  const origin = 'http://example.test'
  const parent = {
    postMessage(payload) {
      messages.push(payload)
      queueMicrotask(() => receiveMessage({ origin, source: parent, data: {
        type: 'sealchat.theater.dialogue-controller.result', requestId: payload.requestId, ok: true,
      } }))
    },
  }
  const window = {
    parent,
    location: { origin, hash: '#/theater?mode=theater' },
    addEventListener: (event, handler) => { assert.equal(event, 'message'); receiveMessage = handler },
    removeEventListener: (event, handler) => { if (receiveMessage === handler) receiveMessage = null },
  }
  const api = {
    get: async () => ({ data: { revision: 1, controller: {}, template: null } }),
    post: async (url, payload) => { mutations.push({ url, payload }); return { data: {} } },
  }
  const context = vm.createContext({
    crypto: { getRandomValues: bytes => webcrypto.getRandomValues(bytes) },
    exports: {}, window, URLSearchParams, setTimeout, clearTimeout, queueMicrotask,
  })
  run(read('utils/uuid.ts'), context)
  const uuidModule = context.exports
  context.exports = {}
  context.require = id => {
    if (id === 'vue') return { shallowRef: value => ({ value }) }
    if (id === '@/stores/_config') return { api }
    if (id === '@/utils/uuid') return uuidModule
    throw new Error(`unexpected import ${id}`)
  }
  run(read('views/theater/dialogue/theater-dialogue-controller.ts'), context)
  await context.exports.writeDialogueController('world', { enabled: true })
  assert.match(messages[0].requestId, uuidV4)
  assert.equal(receiveMessage, null, 'iframe listener is cleaned up after confirmation')
  window.parent = window
  await context.exports.writeDialogueController('world', { enabled: false })
  assert.match(mutations[0].payload.mutationId, uuidV4)
  assert.notEqual(mutations[0].payload.mutationId, messages[0].requestId)
})

