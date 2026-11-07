// Run with node scripts/world-clue-board-retry-regression.cjs from ui/.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')
const vue = require('vue')
const pinia = require('pinia')
const filename = path.resolve(__dirname, '../src/stores/worldClueBoard.ts')
const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename,
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

async function checkRetry(initiallyLoaded) {
  pinia.setActivePinia(pinia.createPinia())
  let offline = false
  let requests = 0
  const api = { async get() {
    requests++
    if (offline) throw { isAxiosError: true, message: 'network unavailable' }
    return { data: { document: { version: 1, placements: {}, relations: [], quickdraw: null }, canWrite: true, revision: 1 } }
  } }
  const loaded = { exports: {} }
  vm.runInNewContext(source, {
    module: loaded, exports: loaded.exports, setTimeout, clearTimeout, TextEncoder,
    require(name) {
      if (name === 'vue') return vue
      if (name === 'pinia') return pinia
      if (name === './_config') return { api }
      if (name === './user') return { useUserStore: () => ({ info: { id: 'test-user' } }) }
      throw new Error(`Unexpected import: ${name}`)
    },
  }, { filename })
  const store = loaded.exports.useWorldClueBoardStore()
  try {
    if (initiallyLoaded) await store.load('test-world')
    offline = true
    await store.load('test-world', { force: true })
    assert.equal(store.current.status, 'error')
    assert.equal(store.current.loadFailed, !initiallyLoaded)
    assert.equal(await store.retry(), false, `failed retry must return false (loaded=${initiallyLoaded})`)
    assert.equal(store.current.status, 'error')
    assert.equal(requests, initiallyLoaded ? 3 : 2, 'retry must issue another GET')

    offline = false
    assert.equal(await store.retry(), true, 'successful retry must return true')
    assert.equal(store.current.status, 'ready')
    assert.equal(store.current.loadFailed, false)
    assert.equal(requests, initiallyLoaded ? 4 : 3)
  } finally {
    store.clearSession()
    store.$dispose()
  }
}

(async () => {
  for (const initiallyLoaded of [false, true]) await checkRetry(initiallyLoaded)
  console.log('world clue board retry regression passed: initial/refresh failure and recovery')
})().catch(error => { console.error(error); process.exitCode = 1 })
