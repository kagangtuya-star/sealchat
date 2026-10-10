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
const providers = [{ kind: 'aliyun', name: '阿里' }, { kind: 'tencent', name: '腾讯' }]
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
      async role() { return { ...globalThis.__voiceTest.role } },
      async saveRole(id, request) { globalThis.__voiceTest.savedRole = { id, request } },
    },
    speechError: e => String(e),
  },
  player: { speechPlayer: {} },
  selection: vue.ref({ type: 'inherit' }),
  queries: [],
  submissions: [],
  directory: { system, providers, items: [personal('mine'), personal('other-account', 'aliyun-backup'), personal('other-model', flash.providerId, next.modelId)], total: 3 },
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
const VoiceCard = await component('./VoiceCard.vue')
const AdminSpeechSettings = await component('./AdminSpeechSettings.vue')
const SpeechPanel = await component('./SpeechPanel.vue')
const RoleVoiceDialog = await component('./RoleVoiceDialog.vue')
async function settle() { for (let i = 0; i < 30; i++) await vue.nextTick() }

test('speech language labels include Chinese and native names while preserving values and fallbacks', () => {
  const options = [
    { value: 'zh', label: '中文（中文）' },
    { value: 'en', label: '英语（English）' },
    { value: 'fr', label: '法语（Français）' },
    { value: 'de', label: '德语（Deutsch）' },
    { value: 'ja', label: '日语（日本語）' },
    { value: 'ko', label: '韩语（한국어）' },
    { value: 'yue', label: '粤语（粤语）' },
    { value: 'ru', label: '俄语（Русский）' },
    { value: 'pt', label: '葡萄牙语（Português）' },
    { value: 'th', label: '泰语（ไทย）' },
    { value: 'id', label: '印尼语（Bahasa Indonesia）' },
    { value: 'vi', label: '越南语（Tiếng Việt）' },
    { value: 'it', label: '意大利语（Italiano）' },
    { value: 'es', label: '西班牙语（Español）' },
    { value: 'ms', label: '马来语（Bahasa Melayu）' },
    { value: 'fil', label: '菲律宾语（Filipino）' },
    { value: 'ar', label: '阿拉伯语（العربية）' },
  ]
  assert.deepEqual(catalog.speechLanguageOptions(options.map(option => option.value)), [
    { value: '', label: '跟随原文' }, ...options,
  ])
  assert.deepEqual(catalog.speechLanguageOptions(['xx']), [
    { value: '', label: '跟随原文' }, { value: 'xx', label: 'xx' },
  ])
  assert.equal(catalog.voiceLanguageLabel('zh'), '中文')
  assert.equal(catalog.voiceLanguageLabel('en'), '英文')
  assert.equal(catalog.voiceLanguageLabel('xx'), 'xx')
})

test('speech languages use speech capability, independently of catalog language filters', async () => {
  const speechLanguages = ['zh', 'en', 'ja', 'ko', 'fr', 'de', 'pt', 'it', 'vi', 'id']
  const bundled = JSON.parse(readFileSync(new URL('../../../../pkg/ttsprovider/tts_catalog_31_20260930.json', import.meta.url), 'utf8'))
  const voices = bundled.filter(voice => ['longanhuan_v3.1', 'longanlingxin_v3.1', 'longanfengyue_v3.1', 'xunanchuan_v3.1'].includes(voice.id))
    .map(voice => ({ ...voice, providerKind: 'aliyun', models: [next.modelId], speechLanguages }))
  assert.equal(voices.length, 4)
  for (const voice of voices) {
    const item = catalog.systemVoiceItem(voice, next)
    assert.equal(item.languages.includes('en'), false)
    assert.deepEqual(item.languages, voice.languages)
    assert.deepEqual(catalog.speechLanguageOptions(item.speechLanguages).map(option => option.value), ['', ...speechLanguages])
    assert.equal(catalog.matchesVoiceFilters(item, { language: 'en', kind: '', tag: '' }), false)
    assert.equal(catalog.matchesVoiceFilters(item, { language: 'ja', kind: '', tag: '' }), true)
  }
  const selected = vue.ref(catalog.itemSelection(catalog.systemVoiceItem(voices[0], next)))
  globalThis.__voiceTest.selection = selected
  const scope = vue.effectScope()
  const props = vue.reactive({ mode: 'select', voiceContext: next, voiceContexts: [flash, next], defaultVoice: voices[0].id })
  const state = scope.run(() => VoicePicker.setup(props, { expose() {} }))
  try {
    await settle()
    state.system.value = voices
    await settle()
    assert.deepEqual(catalog.speechLanguageOptions(state.speechLanguages.value), [
      { value: '', label: '跟随原文' }, { value: 'zh', label: '中文（中文）' }, { value: 'en', label: '英语（English）' },
      { value: 'ja', label: '日语（日本語）' }, { value: 'ko', label: '韩语（한국어）' }, { value: 'fr', label: '法语（Français）' },
      { value: 'de', label: '德语（Deutsch）' }, { value: 'pt', label: '葡萄牙语（Português）' }, { value: 'it', label: '意大利语（Italiano）' },
      { value: 'vi', label: '越南语（Tiếng Việt）' }, { value: 'id', label: '印尼语（Bahasa Indonesia）' },
    ])
    state.filters.value.language = 'en'
    assert.equal(state.filteredPresets.value.length, 0)
    state.setSource('mine')
    assert.deepEqual(state.speechLanguages.value, speechLanguages)
    selected.value = { type: 'inherit' }
    assert.deepEqual(state.speechLanguages.value, speechLanguages)
    props.defaultVoice = 'missing'
    assert.deepEqual(catalog.speechLanguageOptions(state.speechLanguages.value), [{ value: '', label: '跟随原文' }])
    selected.value = { type: 'personal', id: 'mine' }
    assert.deepEqual(catalog.speechLanguageOptions(state.speechLanguages.value), [{ value: '', label: '跟随原文' }])
    assert.deepEqual(catalog.personalVoiceItem(personal('mine'), 'user').speechLanguages, [])
    assert.deepEqual(catalog.systemVoiceItem(system[0], flash).speechLanguages, system[0].languages)
    assert.deepEqual(catalog.systemVoiceItem({ ...system[0], speechLanguages: [] }, flash).speechLanguages, system[0].languages)
    assert.deepEqual(catalog.speechLanguageOptions(['xx']), [{ value: '', label: '跟随原文' }, { value: 'xx', label: 'xx' }])
    assert.equal(catalog.selectedSpeechLanguages({ type: 'system', id: 'pending' }, null, [], null, ''), null)
  } finally { scope.stop() }
})

test('role speech language resets on incompatible selection and travels with save and audition', async () => {
  globalThis.__voiceTest.speech = vue.reactive({ quota: { voiceContext: flash, voiceContexts: [flash, next], defaultVoice: system[0].id }, async refresh() {} })
  globalThis.__voiceTest.role = { identityId: 'role', ...catalog.roleVoiceFields(catalog.itemSelection(catalog.systemVoiceItem(system[0], flash))), speechLanguage: 'zh', instruction: '', rate: 1, pitch: 1, volume: 50, revision: 0 }
  const scope = vue.effectScope()
  const role = scope.run(() => RoleVoiceDialog.setup({ identityId: 'role' }, { expose() {} }))
  try {
    await role.open()
    globalThis.__voiceTest.selection = role.selection
    const picker = scope.run(() => VoicePicker.setup({ mode: 'select', voiceContext: flash, voiceContexts: [flash, next], defaultVoice: system[0].id }, {
      expose() {}, emit(name, languages) { assert.equal(name, 'speech-languages'); role.speechLanguages.value = languages },
    }))
    await settle()
    assert.equal(role.role.value.speechLanguage, 'zh')
    await role.audition()
    assert.equal(globalThis.__voiceTest.submissions.at(-1).request.speechLanguage, 'zh')
    picker.system.value = [{ ...system[0], speechLanguages: ['ja'] }, system[1]]
    await settle()
    assert.equal(role.role.value.speechLanguage, '')
    role.role.value.speechLanguage = 'ja'
    role.selection.value = { type: 'personal', id: 'mine' }
    await settle()
    assert.equal(role.role.value.speechLanguage, '')
    assert.deepEqual(role.languageOptions.value, [{ value: '', label: '跟随原文' }])
    role.selection.value = catalog.itemSelection(catalog.systemVoiceItem(system[0], flash))
    picker.system.value = [{ ...system[0], speechLanguages: ['zh', 'en', 'ja'] }, system[1]]
    await settle()
    assert.ok(role.languageOptions.value.some(option => option.value === 'en' && option.label === '英语（English）'))
    role.role.value.speechLanguage = 'en'
    await role.save()
    assert.equal(globalThis.__voiceTest.savedRole.request.speechLanguage, 'en')
    assert.equal(globalThis.__voiceTest.savedRole.request.systemVoice, system[0].id)
  } finally { scope.stop() }
})

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
    assert.equal(state.creationOptions.value[0].label, `aliyun（target-31） · ${next.modelId}`)
    state.directory.value.providers = providers
    assert.equal(state.creationOptions.value[0].label, `阿里（target-31） · ${next.modelId}`)
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

test('VoicePicker resolves system routes from all contexts and preserves personal availability and stale selections', async () => {
  const scope = vue.effectScope()
  const props = vue.reactive({ mode: 'select', voiceContext: flash, voiceContexts: [flash] })
  const selected = vue.ref(catalog.itemSelection(catalog.systemVoiceItem(system[0], flash)))
  globalThis.__voiceTest.selection = selected
  const state = scope.run(() => VoicePicker.setup(props, { expose() {} }))
  try {
    await settle()
    assert.deepEqual(state.presetItems.value.map(item => item.id), [system[0].id])
    assert.deepEqual(state.personalItems.value.map(item => item.id), ['mine', 'other-account', 'other-model'])
    assert.equal(state.current.value.status, 'ready')
    assert.equal(globalThis.__voiceTest.queries.at(-1).providerId, undefined)
    assert.equal(globalThis.__voiceTest.queries.at(-1).model, undefined)
    props.voiceContext = next
    props.voiceContexts = [next]
    await settle()
    assert.deepEqual(state.presetItems.value.map(item => item.id), [system[1].id])
    assert.deepEqual(state.personalItems.value.map(item => item.id), ['mine', 'other-account', 'other-model'])
    assert.equal(state.current.value.status, 'unavailable')
    assert.equal(selected.value.id, system[0].id)
    selected.value = { type: 'personal', id: 'mine' }
    await settle()
    assert.equal(state.current.value.status, 'ready')
    props.voiceContext = flash
    props.voiceContexts = [flash]
    await settle()
    assert.equal(state.current.value.status, 'ready')
    props.voiceContext = { ...flash, providerId: 'aliyun-backup' }
    props.voiceContexts = [props.voiceContext]
    await settle()
    assert.deepEqual(state.personalItems.value.map(item => item.id), ['mine', 'other-account', 'other-model'])
    assert.equal(state.current.value.status, 'ready')
    assert.equal(selected.value.id, 'mine')
    selected.value = catalog.itemSelection(catalog.systemVoiceItem(system[0], flash))
    props.voiceContexts = [flash, props.voiceContext]
    await settle()
    assert.equal(state.current.value.status, 'ready')
    assert.equal(state.current.value.item.providerId, flash.providerId)
    props.voiceContexts = [props.voiceContext]
    await settle()
    assert.equal(state.current.value.status, 'unavailable')
    assert.equal(selected.value.providerId, flash.providerId)
  } finally { scope.stop() }
})

test('multi-model system voices retain the selected model namespace; legacy bindings resolve in context', () => {
  const voice = { ...system[0], models: [flash.modelId, next.modelId] }
  assert.ok(catalog.systemVoiceSupported(voice, flash))
  assert.ok(catalog.systemVoiceSupported(voice, next))
  assert.equal(catalog.systemVoiceSupported(voice, { ...next, providerKind: 'azure' }), false)
  const item = catalog.systemVoiceItem(voice, next)
  const legacy = catalog.roleVoiceSelection({ voiceId: '', systemVoice: voice.id, systemVoiceProvider: '', systemVoiceProviderId: '', systemVoiceModel: '' })
  assert.ok(catalog.selectsItem(legacy, item))
  assert.equal(catalog.selectsItem({ type: 'system', id: voice.id, providerKind: 'aliyun', modelId: flash.modelId }, item), false)
  const fields = catalog.roleVoiceFields(catalog.itemSelection(item))
  assert.deepEqual(fields, { voiceId: '', systemVoice: voice.id, systemVoiceProvider: 'aliyun', systemVoiceProviderId: next.providerId, systemVoiceModel: next.modelId })
  assert.deepEqual(catalog.roleVoiceSelection(fields), catalog.itemSelection(item))
  assert.deepEqual(catalog.requestVoiceFields(catalog.itemSelection(item)), { systemVoice: voice.id, systemVoiceProvider: 'aliyun', systemVoiceProviderId: next.providerId, systemVoiceModel: next.modelId })
  assert.equal(catalog.selectsItem({ type: 'system', id: voice.id, providerKind: 'aliyun' }, item), false)
  assert.equal(catalog.selectsItem({ ...catalog.itemSelection(item), providerId: 'other' }, item), false)
  assert.equal(catalog.sameSelection(catalog.itemSelection(item), { ...catalog.itemSelection(item), providerId: 'other' }), false)
  assert.equal(catalog.sameSelection(catalog.itemSelection(item), legacy), false)
  assert.deepEqual(catalog.requestVoiceFields({ type: 'inherit' }), {})
})

test('previous provider/model bindings resolve for display without rewriting the selection', async () => {
  const scope = vue.effectScope()
  const backup = { ...flash, providerId: 'aliyun-backup' }
  const props = vue.reactive({ mode: 'select', voiceContext: next, voiceContexts: [next, flash] })
  const binding = { type: 'system', id: system[0].id, providerKind: flash.providerKind, modelId: flash.modelId }
  const selected = vue.ref(binding)
  globalThis.__voiceTest.selection = selected
  const state = scope.run(() => VoicePicker.setup(props, { expose() {} }))
  try {
    await settle()
    assert.equal(state.current.value.status, 'ready')
    assert.equal(state.current.value.item.providerId, flash.providerId)
    assert.equal(catalog.roleVoiceFields(catalog.itemSelection(state.current.value.item)).systemVoiceProviderId, flash.providerId)
    assert.deepEqual(selected.value, binding)
    props.voiceContexts = [next, flash, backup]
    await settle()
    assert.equal(state.current.value.status, 'unavailable')
    assert.deepEqual(selected.value, binding)
    props.voiceContext = backup
    await settle()
    assert.equal(state.current.value.status, 'ready')
    assert.equal(state.current.value.item.providerId, backup.providerId)
    assert.deepEqual(selected.value, binding)
    props.voiceContext = next
    selected.value = { ...binding, providerId: backup.providerId }
    await settle()
    assert.equal(state.current.value.status, 'ready')
    assert.equal(state.current.value.item.providerId, backup.providerId)
    for (const change of [{ providerId: 'missing' }, { providerKind: 'tencent' }, { modelId: next.modelId }, { id: 'missing' }]) {
      selected.value = { ...binding, providerId: backup.providerId, ...change }
      assert.equal(state.current.value.status, 'unavailable')
    }
    selected.value = { type: 'system', id: binding.id }
    assert.equal(state.current.value.status, 'unavailable')
    for (const partial of [
      { providerKind: flash.providerKind }, { modelId: flash.modelId },
      { providerId: flash.providerId }, { providerId: flash.providerId, providerKind: flash.providerKind },
      { providerId: flash.providerId, modelId: flash.modelId },
    ]) {
      selected.value = { type: 'system', id: binding.id, ...partial }
      assert.equal(state.current.value.status, 'unavailable')
    }
  } finally { scope.stop() }
})

test('legacy system cards follow unique resolution and never highlight ambiguous or stale bindings', async () => {
  const backup = { ...flash, providerId: 'aliyun-backup' }
  const oldest = { type: 'system', id: system[0].id }
  const previous = { ...oldest, providerKind: flash.providerKind, modelId: flash.modelId }
  const cards = [flash, backup].map(context => catalog.systemVoiceItem(system[0], context))
  const cases = [
    { name: 'oldest unique', binding: oldest, defaultContext: next, contexts: [next, flash], expected: flash },
    { name: 'oldest ambiguous', binding: oldest, defaultContext: next, contexts: [next, flash, backup], expected: null },
    { name: 'oldest default priority', binding: oldest, defaultContext: backup, contexts: [flash, backup], expected: backup },
    { name: 'oldest no candidate', binding: oldest, defaultContext: next, contexts: [next], expected: null },
    { name: 'previous unique', binding: previous, defaultContext: next, contexts: [next, flash], expected: flash },
    { name: 'previous ambiguous', binding: previous, defaultContext: next, contexts: [next, flash, backup], expected: null },
    { name: 'previous default priority', binding: previous, defaultContext: backup, contexts: [flash, backup], expected: backup },
    { name: 'complete route', binding: { ...previous, providerId: backup.providerId }, defaultContext: flash, contexts: [flash, backup], expected: backup },
    { name: 'stale route', binding: { ...previous, providerId: 'missing' }, defaultContext: flash, contexts: [flash, backup], expected: null },
    { name: 'partial namespace', binding: { ...oldest, providerKind: flash.providerKind }, defaultContext: flash, contexts: [flash], expected: null },
  ]
  for (const entry of cases) {
    const scope = vue.effectScope()
    const props = vue.reactive({ mode: 'select', voiceContext: entry.defaultContext, voiceContexts: entry.contexts })
    const selected = vue.ref({ ...entry.binding })
    const original = selected.value
    globalThis.__voiceTest.selection = selected
    const state = scope.run(() => VoicePicker.setup(props, { expose() {} }))
    try {
      await settle()
      const resolved = catalog.resolveLegacySystemSelection(selected.value, system, entry.contexts, entry.defaultContext)
      assert.equal(state.current.value.status, entry.expected ? 'ready' : 'unavailable', entry.name)
      if (entry.expected) {
        assert.equal(resolved.providerId, entry.expected.providerId, entry.name)
        assert.equal(state.current.value.item.key, resolved.key, entry.name)
        assert.equal(state.isItemSelected(state.current.value.item), true, entry.name)
      } else {
        assert.equal(resolved, null, entry.name)
      }
      assert.deepEqual(cards.filter(state.isItemSelected).map(item => item.providerId), entry.expected ? [entry.expected.providerId] : [], entry.name)
      assert.deepEqual(state.visible.value.filter(state.isItemSelected).map(item => item.key), state.visible.value.filter(item => item.key === resolved?.key).map(item => item.key), entry.name)
      assert.equal(selected.value, original, entry.name)
      assert.deepEqual(selected.value, entry.binding, entry.name)
    } finally { scope.stop() }
  }
  const scope = vue.effectScope()
  const selected = vue.ref({ type: 'personal', id: 'mine' })
  globalThis.__voiceTest.selection = selected
  const state = scope.run(() => VoicePicker.setup({ mode: 'select', voiceContext: flash, voiceContexts: [flash] }, { expose() {} }))
  try {
    await settle()
    assert.deepEqual(state.personalItems.value.filter(state.isItemSelected).map(item => item.id), ['mine'])
    assert.ok(cards.every(item => !state.isItemSelected(item)))
    assert.deepEqual(selected.value, { type: 'personal', id: 'mine' })
  } finally { scope.stop() }
  const template = parse(readFileSync(new URL('./VoicePicker.vue', import.meta.url), 'utf8')).descriptor.template.content
  assert.match(template, /:selected="isItemSelected\(item\)"/)
})

test('previous bindings keep their model route even when a multi-model voice prefers a different default', () => {
  const voice = { ...system[0], models: [flash.modelId, next.modelId] }
  const binding = { type: 'system', id: voice.id, providerKind: flash.providerKind, modelId: flash.modelId }
  const item = catalog.resolveLegacySystemSelection(binding, [voice], [next, flash], next)
  assert.equal(item.providerId, flash.providerId)
  assert.equal(item.modelId, flash.modelId)
  assert.equal(catalog.resolveLegacySystemSelection(binding, [voice], [next], next), null)
  assert.deepEqual(binding, { type: 'system', id: voice.id, providerKind: flash.providerKind, modelId: flash.modelId })
})

test('provider display names drive system labels while personal ownership labels stay unchanged', () => {
  const item = catalog.systemVoiceItem(system[0], flash)
  assert.equal(catalog.voiceSourceLabel(item, providers), '阿里预设')
  assert.equal(catalog.voiceSourceLabel({ ...item, providerKind: 'tencent' }, providers), '腾讯预设')
  assert.equal(catalog.voiceSourceLabel(item, [{ kind: 'aliyun', name: '自定义名称' }]), '自定义名称预设')
  assert.equal(catalog.voiceSourceLabel(item, []), 'aliyun预设')
  assert.equal(catalog.providerDisplayName(providers, 'unknown'), 'unknown')
  assert.equal(catalog.providerDisplayName([], 'tencent'), 'tencent')
  const mine = catalog.personalVoiceItem(personal('mine'), 'user')
  assert.equal(catalog.voiceSourceLabel(mine, providers), '我的 · 私有')
  assert.equal(catalog.voiceSourceLabel({ ...mine, visibility: 'public' }, providers), '我的 · 公开')
  assert.equal(catalog.voiceSourceLabel({ ...mine, ownership: 'public', visibility: 'public' }, providers), '公开音色')
  const pickerTemplate = parse(readFileSync(new URL('./VoicePicker.vue', import.meta.url), 'utf8')).descriptor.template.content
  assert.match(pickerTemplate, /voiceSourceLabel\(current.item, providers\)/)
  const cardTemplate = parse(readFileSync(new URL('./VoiceCard.vue', import.meta.url), 'utf8')).descriptor.template.content
  assert.match(cardTemplate, /voiceSourceLabel\(item, providers\)/)
})

test('provider metadata drives source order and brand registration supplies no Tencent voices', () => {
  assert.deepEqual(catalog.voiceSourceOptions(providers), [
    { value: 'all', label: '全部' },
    { value: 'system:aliyun', label: '阿里预设' },
    { value: 'system:tencent', label: '腾讯预设' },
    { value: 'mine', label: '我的音色' },
    { value: 'public', label: '公开音色' },
  ])
  const items = system.map(voice => catalog.systemVoiceItem(voice, { ...flash, providerKind: voice.providerKind }))
  assert.deepEqual(items.filter(item => catalog.matchesSource(item, 'system:tencent')), [])
  assert.deepEqual(items.filter(item => catalog.matchesSource(item, 'system:aliyun')).map(item => item.id), [system[0].id, system[1].id])
  assert.equal(catalog.matchesSource(catalog.personalVoiceItem(personal('mine'), 'user'), 'system:aliyun'), false)
})

test('system context uses a compatible default then first compatible context, with full frozen route', () => {
  const voice = { ...system[0], models: [flash.modelId, next.modelId] }
  const backup = { ...flash, providerId: 'backup' }
  assert.deepEqual(catalog.resolveSystemVoiceContext(voice, [backup, next], next), next)
  assert.deepEqual(catalog.resolveSystemVoiceContext(voice, [backup, next], { ...flash, providerKind: 'tencent' }), backup)
  assert.equal(catalog.resolveSystemVoiceContext(voice, [], null), null)
  assert.deepEqual(catalog.itemSelection(catalog.systemVoiceItem(voice, backup)), { type: 'system', id: voice.id, providerKind: 'aliyun', providerId: 'backup', modelId: flash.modelId })
})

test('personal availability uses lifecycle, provider status and server support across provider instances', () => {
  const voice = personal('foreign', 'another-cloud')
  assert.equal(catalog.personalVoiceItem(voice, 'user').available, true)
  assert.equal(catalog.personalVoiceItem({ ...voice, supported: undefined }, 'user').available, true)
  for (const change of [{ supported: false }, { providerStatus: 'DEPLOYING' }, { lifecycle: 'preview' }]) {
    assert.equal(catalog.personalVoiceItem({ ...voice, ...change }, 'user').available, false)
  }
})

test('source and attribute filters are independent; clear and context changes preserve search and binding', async () => {
  const scope = vue.effectScope()
  const props = vue.reactive({ mode: 'select', voiceContext: flash, voiceContexts: [flash] })
  const binding = { type: 'system', id: system[0].id }
  const selected = vue.ref(binding)
  globalThis.__voiceTest.selection = selected
  const state = scope.run(() => VoicePicker.setup(props, { expose() {} }))
  try {
    await settle()
    assert.equal(state.current.value.status, 'ready')
    assert.deepEqual(selected.value, binding)
    assert.equal(state.activeFilterCount.value, 0)
    assert.equal(state.hasFilters.value, false)
    assert.deepEqual(Object.keys(catalog.emptyVoiceFilters()), ['language', 'kind', 'tag'])
    assert.ok(!catalog.collectVoiceFacets(state.personalItems.value).some(facet => facet.key === 'providerId'))
    state.searchInput.value = '3.0'
    state.applySearch()
    state.filters.value = { language: 'zh', kind: 'system', tag: 'test' }
    state.tagsExpanded.value = true
    state.page.value = 2
    state.setSource('system:aliyun')
    await settle()
    assert.equal(state.page.value, 1)
    assert.equal(state.tagsExpanded.value, false)
    assert.equal(state.search.value, '3.0')
    assert.equal(state.searchInput.value, '3.0')
    assert.deepEqual(state.filters.value, catalog.emptyVoiceFilters())
    assert.equal(state.activeFilterCount.value, 0)
    const requestsBeforeTencent = globalThis.__voiceTest.queries.length
    state.setSource('system:tencent')
    await settle()
    assert.deepEqual(state.visible.value, [])
    assert.equal(state.emptyMessage.value, '当前没有可用的腾讯预设音色。')
    assert.equal(globalThis.__voiceTest.queries.length, requestsBeforeTencent)
    for (const key of ['language', 'kind', 'tag']) {
      state.toggleFilter(key, 'test')
      assert.equal(state.activeFilterCount.value, 1)
      assert.equal(state.hasFilters.value, true)
      state.tagsExpanded.value = true
      state.page.value = 2
      state.clearFilters()
      assert.equal(state.activeFilterCount.value, 0)
      assert.equal(state.hasFilters.value, false)
      assert.equal(state.page.value, 1)
      assert.equal(state.tagsExpanded.value, false)
      assert.equal(state.source.value, 'system:tencent')
      assert.equal(state.search.value, '3.0')
      assert.deepEqual(selected.value, binding)
    }
    state.filters.value = { language: 'zh', kind: 'system', tag: '' }
    state.resolved.value = { stale: null }
    state.tagsExpanded.value = true
    state.page.value = 2
    props.voiceContext = next
    props.voiceContexts = [next]
    await settle()
    assert.equal(state.page.value, 1)
    assert.deepEqual(state.filters.value, catalog.emptyVoiceFilters())
    assert.deepEqual(state.personal.value, [])
    assert.deepEqual(state.resolved.value, {})
    assert.equal(state.tagsExpanded.value, false)
    assert.equal(state.current.value.status, 'unavailable')
    assert.deepEqual(selected.value, binding)
    assert.equal(state.search.value, '3.0')
    assert.equal(state.source.value, 'system:tencent')
  } finally { scope.stop() }
})

test('clear action is inside expanded facets, only shown for active attributes, with existing text tabs', () => {
  const { descriptor } = parse(readFileSync(new URL('./VoicePicker.vue', import.meta.url), 'utf8'))
  const content = descriptor.template.content
  const filterStart = content.indexOf('<div v-if="filtersOpen" class="vp-filters">')
  const clearAt = content.indexOf('>清除筛选</button>')
  assert.ok(clearAt > filterStart && clearAt < content.indexOf('<div class="vp-current"'))
  assert.match(content.slice(filterStart, clearAt), /v-if="activeFilterCount > 0"/)
  assert.equal(content.match(/清除筛选/g).length, 1)
  assert.match(content, /class="vp-chip is-category"/)
})

test('VoiceCard preserves detail tags for multilingual voices and never renders model ids', () => {
  const scope = vue.effectScope()
  const item = catalog.systemVoiceItem({
    ...system[1],
    languages: ['zh', 'ja', 'ko', 'fr', 'de'],
    tags: '女 / 柔和 / 自然 / 知性 / 有声书',
  }, next)
  const state = scope.run(() => VoiceCard.setup({ item, providers, selected: false }, { expose() {}, emit() {} }))
  try {
    assert.deepEqual(state.chips.value, [
      { text: '多语 · 5', language: true },
      { text: '女', language: false },
      { text: '柔和', language: false },
      { text: '自然', language: false },
    ])
    const { descriptor } = parse(readFileSync(new URL('./VoiceCard.vue', import.meta.url), 'utf8'))
    assert.doesNotMatch(descriptor.template.content, /modelId|showModel/)
    assert.doesNotMatch(descriptor.scriptSetup.content, /showModel/)
  } finally { scope.stop() }
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
