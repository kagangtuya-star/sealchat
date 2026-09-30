import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import * as vue from 'vue'
import { parse, compileScript } from '@vue/compiler-sfc'

const moduleURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const transpile = source => ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
const catalogURL = moduleURL(transpile(readFileSync(new URL('./voice-catalog.ts', import.meta.url), 'utf8')))
const catalog = await import(catalogURL)
const flash = { providerKind: 'aliyun', providerId: 'aliyun-main', modelId: 'qwen-audio-3.0-tts-flash' }
const next = { ...flash, modelId: 'qwen-audio-3.1-tts-flash' }
const system = [
  { id: 'longanhuan_v3.6', name: '3.0', providerKind: 'aliyun', models: [flash.modelId], languages: ['zh'], kind: 'system' },
  { id: 'longanhuan_v3.1', name: '3.1', providerKind: 'aliyun', models: [next.modelId], languages: ['zh'], kind: 'system' },
  { id: 'foreign', name: 'Azure', providerKind: 'azure', models: [flash.modelId], languages: ['en'], kind: 'system' },
]
const personal = (id, providerId = flash.providerId, targetModel = flash.modelId) => ({ id, providerId, targetModel, ownerUserId: 'user', name: id, lifecycle: 'saved', providerStatus: 'OK', supported: true, kind: 'design' })
const models = [
  { id: flash.modelId, providerKind: 'aliyun', defaultVoice: system[0].id },
  { id: next.modelId, providerKind: 'aliyun', defaultVoice: system[1].id },
]

globalThis.__voiceTest = {
  vue: { ...vue, useModel: () => globalThis.__voiceTest.selection, onMounted() {}, onBeforeUnmount() {} },
  ui: { useThemeVars: () => ({}), NInput: {}, NSelect: {} },
  user: { useUserStore: () => ({ info: { id: 'user' } }) },
  store: { useSpeechStore: () => globalThis.__voiceTest.speech },
  chat: { useChatStore: () => ({}) },
  api: {
    speechAPI: {
      async voices(query) { globalThis.__voiceTest.queries.push(query); return globalThis.__voiceTest.directory },
      async voice(id) { return globalThis.__voiceTest.directory.items.find(voice => voice.id === id) },
      async submit(operation, request) { globalThis.__voiceTest.submissions.push({ operation, request }); return { id: 'create-job', status: 'failed' } },
      async job() { return { id: 'create-job', status: 'failed' } },
      async source() { return 'clone-source' },
    },
    speechError: e => String(e),
  },
  player: { speechPlayer: {} },
  selection: vue.ref({ type: 'inherit' }),
  queries: [],
  submissions: [],
  directory: { system, items: [personal('mine'), personal('other-account', 'aliyun-backup'), personal('other-model', flash.providerId, next.modelId)], total: 1 },
}

// Execute the real component setup with Vue reactivity. Rendering, stores and
// charged APIs are stubbed; selection/filtering/watchers are production code.
async function component(filename) {
  const { descriptor } = parse(readFileSync(new URL(filename, import.meta.url), 'utf8'))
  const stubs = { vue: 'vue', 'naive-ui': 'ui', './api': 'api', '@/stores/user': 'user', './player': 'player', './store': 'store', '@/stores/chat': 'chat' }
  const code = transpile(compileScript(descriptor, { id: filename }).content)
    .replace(/^import (\{[^}]*\}) from ['"]([^'"]+)['"];$/gm, (line, names, from) => {
      if (from === './voice-catalog') return `import ${names} from '${catalogURL}';`
      assert.ok(stubs[from], `unexpected import ${from}`)
      return `const ${names.replace(/ as /g, ': ')} = globalThis.__voiceTest['${stubs[from]}'];`
    })
    .replace(/^import (\w+) from ['"][^'"]+\.vue['"];$/gm, 'const $1 = {};')
  return (await import(moduleURL(code))).default
}
const VoicePicker = await component('./VoicePicker.vue')
const AdminSpeechSettings = await component('./AdminSpeechSettings.vue')
const SpeechPanel = await component('./SpeechPanel.vue')
async function settle() { for (let i = 0; i < 30; i++) await vue.nextTick() }

test('creation targets use provider/model capabilities and revalidate when switching operation', async () => {
  globalThis.__voiceTest.speech = vue.reactive({ quota: { enabled: true, voiceContext: flash }, async refresh() {} })
  const scope = vue.effectScope()
  const state = scope.run(() => SpeechPanel.setup({}, { expose() {} }))
  const provider = (providerId, modelId, voiceDesign, voiceClone, cloneLanguages = [], supportsClonePreprocess = false) => ({ providerId, providerKind: 'aliyun', designPrice: 0, clonePrice: 0, models: [{ id: modelId, providerKind: 'aliyun', capabilities: { voiceDesign, voiceClone }, cloneLanguages, supportsClonePreprocess }] })
  try {
    // Default is the second option, so initialization must use VoiceContext.
    state.creationProviders.value = [provider('target-31', next.modelId, true, true, ['ja', 'en'], true), provider(flash.providerId, flash.modelId, true, false), provider('clone-only', flash.modelId, false, true, ['en'])]
    assert.equal(state.selectedCreationTarget.value.providerId, flash.providerId)
    assert.equal(state.creationOptions.value.length, 2)
    assert.match(state.creationOptions.value[0].label, /阿里云（target-31） · qwen-audio-3.1-tts-flash/)
    state.operation.value = 'clone'
    assert.equal(state.selectedCreationTarget.value.providerId, 'target-31')
    state.creationTarget.value = state.creationOptions.value.find(option => option.providerId === 'clone-only').value
    state.operation.value = 'design'
    assert.equal(state.selectedCreationTarget.value.providerId, 'target-31')
    state.selection.value = { type: 'system', id: system[0].id, providerKind: flash.providerKind, modelId: flash.modelId }
    await state.submit('design')
    await settle()
    const submitted = globalThis.__voiceTest.submissions.at(-1)
    assert.equal(submitted.operation, 'design')
    assert.equal(submitted.request.providerId, 'target-31')
    assert.equal(submitted.request.modelId, next.modelId)
    assert.equal(submitted.request.systemVoice, undefined)
    assert.equal(submitted.request.voiceId, undefined)
    assert.equal(submitted.request.cloneLanguageHint, undefined)
    assert.equal(submitted.request.clonePreprocess, undefined)
    state.operation.value = 'clone'
    assert.deepEqual(state.cloneLanguageOptions.value, [{ value: '', label: '自动/默认' }, { value: 'ja', label: '日语' }, { value: 'en', label: '英语' }])
    assert.equal(state.cloneLanguageHint.value, '')
    assert.equal(state.clonePreprocess.value, false)
    state.cloneLanguageHint.value = 'ja'
    state.clonePreprocess.value = true
    state.source.value = { name: 'sample.wav' }
    state.authorized.value = true
    await state.submit('clone')
    await settle()
    const clone = globalThis.__voiceTest.submissions.at(-1)
    assert.equal(clone.operation, 'clone')
    assert.equal(clone.request.providerId, 'target-31')
    assert.equal(clone.request.modelId, next.modelId)
    assert.equal(clone.request.sourceResourceId, 'clone-source')
    assert.equal(clone.request.cloneLanguageHint, 'ja')
    assert.equal(clone.request.clonePreprocess, true)
    state.creationTarget.value = state.creationOptions.value.find(option => option.providerId === 'clone-only').value
    assert.equal(state.cloneLanguageHint.value, '')
    assert.equal(state.clonePreprocess.value, false)
    assert.equal(state.selectedCreationTarget.value.supportsClonePreprocess, false)
    assert.deepEqual(state.cloneLanguageOptions.value.map(option => option.value), ['', 'en'])
    await state.submit('clone')
    await settle()
    assert.equal(globalThis.__voiceTest.submissions.at(-1).request.cloneLanguageHint, '')
    assert.equal(globalThis.__voiceTest.submissions.at(-1).request.clonePreprocess, false)
    state.cloneLanguageHint.value = 'en'
    state.creationTarget.value = state.creationOptions.value.find(option => option.providerId === 'target-31').value
    assert.equal(state.cloneLanguageHint.value, 'en')
    state.clonePreprocess.value = true
    state.operation.value = 'design'
    await state.submit('design')
    await settle()
    assert.equal(globalThis.__voiceTest.submissions.at(-1).request.cloneLanguageHint, undefined)
    assert.equal(globalThis.__voiceTest.submissions.at(-1).request.clonePreprocess, undefined)
    await state.submit('audition')
    await settle()
    const audition = globalThis.__voiceTest.submissions.at(-1).request
    assert.equal(audition.systemVoice, system[0].id)
    assert.equal(audition.providerId, undefined)
    assert.equal(audition.modelId, undefined)
    assert.equal(audition.cloneLanguageHint, undefined)
    assert.equal(audition.clonePreprocess, undefined)
    state.creationProviders.value = [provider('clone-only', flash.modelId, false, true)]
    assert.equal(state.creationTarget.value, '')
    await assert.rejects(state.submit('design'), /目标模型/)
    state.operation.value = 'clone'
    assert.equal(state.selectedCreationTarget.value.providerId, 'clone-only')
  } finally { scope.stop() }
})

test('VoicePicker filters both system and personal voices by VoiceContext and keeps stale selections', async () => {
  const scope = vue.effectScope()
  const props = vue.reactive({ mode: 'select', voiceContext: flash })
  const selected = vue.ref({ type: 'system', id: system[0].id, providerKind: flash.providerKind, modelId: flash.modelId })
  globalThis.__voiceTest.selection = selected
  const state = scope.run(() => VoicePicker.setup(props, { expose() {} }))
  try {
    await settle()
    assert.deepEqual(state.presetItems.value.map(item => item.id), [system[0].id])
    assert.deepEqual(state.personalItems.value.map(item => item.id), ['mine'])
    assert.equal(state.current.value.status, 'ready')
    assert.equal(globalThis.__voiceTest.queries.at(-1).providerId, flash.providerId)
    assert.equal(globalThis.__voiceTest.queries.at(-1).model, flash.modelId)
    props.voiceContext = next
    await settle()
    assert.deepEqual(state.presetItems.value.map(item => item.id), [system[1].id])
    assert.deepEqual(state.personalItems.value.map(item => item.id), ['other-model'])
    assert.equal(state.current.value.status, 'unavailable')
    assert.equal(selected.value.id, system[0].id)
    selected.value = { type: 'personal', id: 'mine' }
    await settle()
    assert.equal(state.current.value.status, 'unavailable')
    props.voiceContext = flash
    await settle()
    assert.equal(state.current.value.status, 'ready')
    props.voiceContext = { ...flash, providerId: 'aliyun-backup' }
    await settle()
    assert.deepEqual(state.personalItems.value.map(item => item.id), ['other-account'])
    assert.equal(state.current.value.status, 'unavailable')
    assert.equal(selected.value.id, 'mine')
  } finally { scope.stop() }
})

test('multi-model system voices retain the selected model namespace; legacy bindings resolve in context', () => {
  const voice = { ...system[0], models: [flash.modelId, next.modelId] }
  assert.ok(catalog.systemVoiceSupported(voice, flash))
  assert.ok(catalog.systemVoiceSupported(voice, next))
  assert.equal(catalog.systemVoiceSupported(voice, { ...next, providerKind: 'azure' }), false)
  const item = catalog.systemVoiceItem(voice, next)
  const legacy = catalog.roleVoiceSelection({ voiceId: '', systemVoice: voice.id, systemVoiceProvider: '', systemVoiceModel: '' })
  assert.ok(catalog.selectsItem(legacy, item))
  assert.equal(catalog.selectsItem({ type: 'system', id: voice.id, providerKind: 'aliyun', modelId: flash.modelId }, item), false)
  const fields = catalog.roleVoiceFields(catalog.itemSelection(item))
  assert.deepEqual(fields, { voiceId: '', systemVoice: voice.id, systemVoiceProvider: 'aliyun', systemVoiceModel: next.modelId })
  assert.deepEqual(catalog.requestVoiceFields({ type: 'inherit' }), {})
})

test('admin default voice dropdown uses the current provider/model and catalog default', async () => {
  const scope = vue.effectScope()
  const state = scope.run(() => AdminSpeechSettings.setup({}, { expose() {} }))
  try {
    state.systemVoices.value = system
    state.modelCatalog.value = models
    state.config.value.providers = [
      { id: flash.providerId, providerKind: 'aliyun', model: flash.modelId },
      { id: 'backup', providerKind: 'aliyun', model: next.modelId },
    ]
    state.config.value.defaultProvider = flash.providerId
    await settle()
    assert.deepEqual(state.defaultVoiceOptions.value.map(option => option.value), [system[0].id])
    assert.equal(state.config.value.defaultVoice, system[0].id)
    state.config.value.defaultProvider = 'backup'
    await settle()
    assert.deepEqual(state.defaultVoiceOptions.value.map(option => option.value), [system[1].id])
    assert.equal(state.config.value.defaultVoice, system[1].id)
    state.config.value.providers[1].model = flash.modelId
    await settle()
    assert.equal(state.config.value.defaultVoice, system[0].id)
    assert.equal(catalog.defaultVoiceForContext([ ...system, { ...system[0], id: 'custom' } ], models, flash, 'custom'), 'custom')
    state.modelCatalog.value = [...models, { ...models[0], providerKind: 'azure', defaultVoice: 'foreign' }]
    state.config.value.providers[1].providerKind = 'azure'
    state.resolvedProvider.value = { providerId: 'backup', models: [models[0]] }
    state.selectProviderModel(state.config.value.providers[1], flash.modelId)
    await settle()
    assert.equal(state.config.value.providers[1].providerKind, 'azure')
    assert.deepEqual(state.defaultVoiceOptions.value.map(option => option.value), ['foreign'])
    assert.equal(state.config.value.defaultVoice, 'foreign')
  } finally { scope.stop() }
})
