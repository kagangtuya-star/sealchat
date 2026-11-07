// Style Studio 工作副本与样式 ZIP 回归：node scripts/log-style-studio-regression.cjs
// 覆盖：旧样式（内置 JSON）往返、dirty 指纹、普通视图与源码共用 JSON 文本、模板新增/删除/用途推导、
// 资源嗅探与引用、版本号、空白起点，以及 ZIP 读写与服务端布局一致。
// logTemplateCompiler 依赖 DOM，这里只需要渲染器版本号，替换为最小模块。
// 设置 LOG_STYLE_ZIP_OUT=<目录> 时把打包结果写出，供服务端 ImportLogStyle 交叉验证。
const path = require('node:path')
const fs = require('node:fs')
const Module = require('node:module')
const assert = require('node:assert/strict')
const ts = require('typescript')

const src = path.join(__dirname, '../src')
const stubs = {
  './logTemplateCompiler': { LOG_RENDERER_VERSION: '1.2.0' },
  '@/services/log/logApi': { describeLogError: () => '', fetchLogStyles: async () => ({ items: [], nextCursor: null }) },
  '../logStyleCatalog': { builtinStyleOrder: () => 0 },
}
const original = Module._resolveFilename
Module._resolveFilename = function (request, parent, ...args) {
  if (request in stubs) return `stub:${request}`
  if (request.startsWith('@/')) request = path.join(src, request.slice(2))
  return original.call(this, request, parent, ...args)
}
for (const [name, exports] of Object.entries(stubs)) {
  Module._cache[`stub:${name}`] = { id: name, filename: name, loaded: true, exports }
}
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename)

const wc = require(path.join(src, 'views/log/logStyleWorkingCopy.ts'))
const styleLibrary = require(path.join(src, 'views/log/composables/useLogStyleLibrary.ts'))
const zipIo = require(path.join(src, 'services/log/logStyleZip.ts'))
const { zipSync, strToU8 } = require('fflate')

const builtinDir = path.join(__dirname, '../../service/log_styles')
const loadBuiltin = (name) => JSON.parse(fs.readFileSync(path.join(builtinDir, name), 'utf8'))
// 1×1 真实 PNG：交叉验证时服务端会完整解码资源。
const PNG = Uint8Array.from(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64'))
const draft = (patch = {}) => ({
  schemaVersion: 1, title: 'LOG', sections: [], elements: [],
  page: { backgroundAssetId: null, fontFamily: 'sans-serif', headerTemplateId: null, footerTemplateId: null },
  stylePackageId: 'style-x', stylePackageVersion: '1.0.0', themeTokens: {}, ...patch,
})

const tests = []
const test = (name, fn) => tests.push([name, fn])

test('旧样式（全部内置 JSON）经工作副本往返后与原包一致，且不算未保存修改', () => {
  for (const file of fs.readdirSync(builtinDir).filter((f) => f.endsWith('.json'))) {
    const pkg = loadBuiltin(file)
    const copy = wc.workingCopyFromPackage(pkg, 'version')
    const { pkg: back, issues } = wc.assembleStylePackage(copy)
    assert.deepEqual(issues, [], file)
    assert.deepEqual(back, { ...pkg, license: pkg.license.trim(), name: pkg.name.trim() }, file)
    assert.equal(copy.base, pkg)
    assert.equal(wc.workingCopyFingerprint(copy), wc.workingCopyFingerprint(wc.workingCopyFromPackage(pkg, 'version')))
  }
})

test('dirty 指纹：JSON 重新排版与模板键序不算修改，内容修改算，改回后恢复干净', () => {
  const copy = wc.workingCopyFromPackage(loadBuiltin('starter.json'), 'version')
  const base = wc.workingCopyFingerprint(copy)
  copy.rulesText = JSON.stringify(JSON.parse(copy.rulesText))
  copy.templates = Object.fromEntries(Object.entries(copy.templates).reverse())
  assert.equal(wc.workingCopyFingerprint(copy), base)
  const original = copy.templates.heading
  copy.templates = { ...copy.templates, heading: '<h3 data-log-text="text"></h3>' }
  assert.notEqual(wc.workingCopyFingerprint(copy), base)
  copy.templates = { ...copy.templates, heading: original }
  assert.equal(wc.workingCopyFingerprint(copy), base)
  copy.assets = [{ id: 'a1', path: 'assets/a.png', mime: 'image/png', sha256: '' }]
  assert.notEqual(wc.workingCopyFingerprint(copy), base, '资源变化是修改')
})

test('普通视图与高级源码共用同一段 JSON 文本；语法错误时组装失败并给出问题', () => {
  const copy = wc.workingCopyFromPackage(loadBuiltin('starter.json'), 'version')
  // 普通视图：解析 → 修改 → 序列化写回
  const tokens = wc.parseStyleJson(copy).tokens
  copy.tokensText = wc.formatJson({ ...tokens, accent: '#112233' })
  assert.equal(JSON.parse(copy.tokensText).accent, '#112233')
  assert.equal(wc.assembleStylePackage(copy).pkg.tokens.accent, '#112233')
  // 源码视图：直接改文本，普通视图读到同样结果
  copy.tokensText = '{ "accent": "#445566", "dense": true }'
  assert.deepEqual(wc.parseStyleJson(copy).tokens, { accent: '#445566', dense: true })
  copy.rulesText = '[{"templateId": "message"},'
  const parsed = wc.parseStyleJson(copy)
  assert.equal(parsed.rules, null)
  assert.match(parsed.errors.rules, /JSON 语法错误/)
  const out = wc.assembleStylePackage(copy, parsed)
  assert.equal(out.pkg, null)
  assert.ok(out.issues.some((m) => m.startsWith('消息规则')))
  copy.rulesText = '{"templateId":"message"}'
  assert.match(wc.parseStyleJson(copy).errors.rules, /数组/)
  copy.rulesText = '[]'
  copy.name = '  '
  assert.ok(wc.assembleStylePackage(copy).issues.includes('名称不能为空'))
})

test('模板：新增校验 ID、custom 同步参数 Schema，删除受规则/片段/最后一个模板阻断并清理 Schema', () => {
  const copy = wc.workingCopyFromPackage(loadBuiltin('starter.json'), 'version')
  assert.match(wc.addTemplate(copy, '1bad', '<p></p>'), /字母开头/)
  assert.match(wc.addTemplate(copy, 'message', '<p></p>'), /已存在/)
  const custom = wc.templateStarter('custom', 'customNote')
  assert.equal(wc.addTemplate(copy, 'customNote', custom.html, custom.schema), '')
  assert.ok(copy.templates.customNote.includes('props.label'))
  assert.deepEqual(Object.keys(wc.parseStyleJson(copy).schema), ['customNote'])
  assert.equal(wc.suggestTemplateId(copy, 'custom'), 'custom')
  assert.equal(wc.suggestTemplateId(copy, 'element', 'cover'), 'cover2')
  assert.equal(wc.suggestTemplateId(copy, 'part'), 'part2')
  const msg = wc.templateStarter('message', 'whisper')
  assert.equal((msg.html.match(/data-log-slot="body"/g) || []).length, 1, '消息模板恰好一个正文插槽')
  copy.templates = { ...copy.templates, shell: '<div data-log-part="heading"></div>' }

  const parsed = wc.parseStyleJson(copy)
  const graph = { parts: { shell: ['heading'], message: [], heading: [], customNote: [] }, bodySlots: { message: 1, heading: 0, shell: 0, customNote: 0 } }
  const d = draft({ elements: [{ id: 'e', type: 'heading', templateId: 'heading' }], page: { headerTemplateId: 'customNote', footerTemplateId: null, fontFamily: 'x' }, messageTemplateOverrides: { m1: 'message', m2: 'message' } })
  const usage = wc.analyzeTemplates(copy, parsed, graph, (id) => (id === 'heading' ? ['heading'] : []), d)
  assert.deepEqual(usage.message.ruleIndexes, [0])
  assert.deepEqual(usage.heading.usedAsPartBy, ['shell'])
  assert.deepEqual(usage.shell.parts, ['heading'])
  assert.deepEqual(usage.customNote.propNames, ['label'])
  assert.equal(usage.message.draftRefs.overrides, 2)
  assert.equal(usage.heading.draftRefs.elements, 1)
  assert.equal(usage.customNote.draftRefs.header, true)
  assert.equal(wc.draftRefCount(usage.customNote.draftRefs), 1)
  assert.equal(usage.cover.bodySlots, null, '无效或未解析的模板不猜测插槽数')
  assert.deepEqual(wc.templateRoles(usage.message), ['消息规则'])
  assert.ok(wc.templateRoles(usage.heading).includes('片段'))
  assert.ok(wc.templateRoles(usage.customNote).includes('页眉'))
  assert.equal(wc.analyzeTemplates(copy, parsed, graph, () => [], null).message.draftRefs, null, '未绑定此样式时不统计草稿引用')

  assert.equal(wc.templateRemovalBlockers(copy, 'message', usage.message).length, 1)
  assert.equal(wc.templateRemovalBlockers(copy, 'heading', usage.heading).length, 1)
  assert.deepEqual(wc.templateRemovalBlockers(copy, 'customNote', usage.customNote), [])
  wc.removeTemplate(copy, 'customNote')
  assert.ok(!('customNote' in copy.templates))
  assert.deepEqual(wc.parseStyleJson(copy).schema, {})
  const single = { templates: { only: '<p></p>' } }
  assert.ok(wc.templateRemovalBlockers(single, 'only', undefined)[0].includes('至少'))
  // Schema 源码坏掉时不覆盖用户源码
  copy.schemaText = '{'
  assert.match(wc.addTemplate(copy, 'customB', custom.html, custom.schema), /无法解析/)
  assert.ok(!('customB' in copy.templates))
})

test('资源：按字节嗅探 MIME、路径去重、CSS 文本引用与草稿结构化引用', () => {
  assert.equal(wc.sniffStyleAssetMime(PNG), 'image/png')
  assert.equal(wc.sniffStyleAssetMime(Uint8Array.from([0xff, 0xd8, 0xff, 0xe0])), 'image/jpeg')
  assert.equal(wc.sniffStyleAssetMime(Uint8Array.from([0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50])), 'image/webp')
  assert.equal(wc.sniffStyleAssetMime(Uint8Array.from([0x4f, 0x54, 0x54, 0x4f])), 'font/otf')
  assert.equal(wc.sniffStyleAssetMime(Uint8Array.from([0, 1, 0, 0])), 'font/ttf')
  assert.equal(wc.sniffStyleAssetMime(strToU8('GIF89a')), '')
  assert.equal(wc.styleAssetPath('My Paper.PNG', 'image/png', ['assets/my-paper.png']), 'assets/my-paper-1.png')
  assert.equal(wc.styleAssetPath('花纹.jpeg', 'image/jpeg', []), 'assets/asset.jpg')
  const refs = wc.cssAssetRefs('.a{background:url(assets/p.png)} /* url(assets/p.png) */ .b{background:url( assets/p.png )} @font-face{src:url(assets/f.ttf)}')
  assert.equal(refs.get('assets/p.png'), 2)
  assert.equal(refs.get('assets/f.ttf'), 1)
  const d = draft({ page: { backgroundAssetId: 'bg', fontFamily: 'font1' }, elements: [{ id: 'e', type: 'cover', templateId: 'cover', content: { assetId: 'bg' }, anchor: { sectionId: 's' } }] })
  const dr = wc.draftStyleAssetRefs(d)
  assert.equal(dr.get('bg'), 2)
  assert.equal(dr.get('font1'), 1)
})

test('版本冲突改基准：最新版本移除或替换的旧资源会被识别并阻止直接覆盖', () => {
  const base = { ...loadBuiltin('starter.json'), assets: [{ id: 'old', path: 'assets/old.png', mime: 'image/png', sha256: 'old-hash' }] }
  const copy = wc.workingCopyFromPackage(base, 'version')
  const removed = { ...base, assets: [] }
  assert.deepEqual(wc.inheritedAssetsDivergedFromLatest(copy, removed).map((a) => a.id), ['old'])
  const replaced = { ...base, assets: [{ ...base.assets[0], sha256: 'new-hash' }] }
  assert.deepEqual(wc.inheritedAssetsDivergedFromLatest(copy, replaced).map((a) => a.id), ['old'])
  const same = { ...base, assets: [{ ...base.assets[0] }] }
  assert.deepEqual(wc.inheritedAssetsDivergedFromLatest(copy, same), [])
  copy.assets = []
  assert.deepEqual(wc.inheritedAssetsDivergedFromLatest(copy, removed), [])
})

test('样式归属：列表未完整时缺失项为 unknown，完整列表后才判为 foreign', () => {
  const items = [{ id: 'mine', version: '1.0.0', name: 'Mine', builtin: false }, { id: 'sealchat-x', version: '1.0.0', name: 'Builtin', builtin: true }]
  assert.equal(styleLibrary.resolveLogStyleOwnership(items, { loaded: false, loading: true, error: '', truncated: false }, 'missing'), 'unknown')
  assert.equal(styleLibrary.resolveLogStyleOwnership(items, { loaded: true, loading: false, error: 'failed', truncated: false }, 'missing'), 'unknown')
  assert.equal(styleLibrary.resolveLogStyleOwnership(items, { loaded: true, loading: false, error: '', truncated: true }, 'missing'), 'unknown')
  assert.equal(styleLibrary.resolveLogStyleOwnership(items, { loaded: true, loading: false, error: '', truncated: false }, 'missing'), 'foreign')
  assert.equal(styleLibrary.resolveLogStyleOwnership(items, { loaded: true, loading: true, error: '', truncated: false }, 'missing'), 'foreign', '已有完整列表时刷新中沿用已知归属')
  assert.equal(styleLibrary.resolveLogStyleOwnership(items, { loaded: false, loading: true, error: '', truncated: false }, 'mine'), 'owned')
  assert.equal(styleLibrary.resolveLogStyleOwnership(items, { loaded: false, loading: true, error: '', truncated: false }, 'sealchat-x'), 'foreign')
})

test('版本号与设计变量辅助', () => {
  assert.equal(wc.nextStyleVersion('1.2.3'), '1.2.4')
  assert.equal(wc.nextStyleVersion('1.2.3', 'minor'), '1.3.0')
  assert.equal(wc.nextStyleVersion('1.2.3', 'major'), '2.0.0')
  assert.equal(wc.nextStyleVersion('bad'), '1.0.1')
  assert.equal(wc.compareStyleVersions('1.10.0', '1.9.9'), 1)
  assert.equal(wc.compareStyleVersions('1.0.0', '1.0.0'), 0)
  assert.equal(wc.tokenKind('#aabbcc'), 'color')
  assert.equal(wc.tokenKind('1px'), 'text')
  assert.equal(wc.tokenKind(2), 'number')
  assert.equal(wc.tokenKind(false), 'boolean')
  assert.equal(wc.tokenCssHint('Accent', '#fff'), 'var(--log-Accent)')
  assert.equal(wc.tokenCssHint('Dense', true), '[data-log-dense="true"]')
})

test('空白起点是普通样式包：规则模板存在且恰好一个正文插槽，没有资源', () => {
  const pkg = wc.blankStylePackage('1.2.0')
  const copy = wc.workingCopyFromPackage(pkg, 'new')
  assert.equal(copy.base, null)
  const { pkg: out, issues } = wc.assembleStylePackage(copy)
  assert.deepEqual(issues, [])
  for (const r of out.rules) assert.equal((out.templates[r.templateId].match(/data-log-slot="body"/g) || []).length, 1)
  for (const id of Object.keys(out.templates)) assert.match(id, wc.STYLE_KEY)
  assert.deepEqual(out.assets, [])
  assert.ok(!/[\\<>]/.test(out.css), 'CSS 不含反斜杠与尖括号')
  assert.match(wc.freshStyleId(), /^style-[0-9a-f]{16}$/)
})

test('ZIP：打包布局与服务端导出一致，读回后得到同一个包（含资源与预览图）', async () => {
  const pkg = { ...loadBuiltin('starter.json'), id: 'zip-style', version: '1.0.0', name: 'ZIP 样式' }
  pkg.assets = [{ id: 'paper', path: 'assets/paper.png', mime: 'image/png', sha256: '' }]
  pkg.css += ' .log-cover { background: url(assets/paper.png); }'
  const bytes = await zipIo.buildStyleZip(pkg, { paper: PNG }, { 'previews/cover.png': PNG, 'notes.txt': strToU8('x') })
  const files = require('fflate').unzipSync(bytes)
  const names = Object.keys(files).sort()
  assert.ok(names.includes('manifest.json') && names.includes('tokens.json') && names.includes('settings.schema.json') && names.includes('styles/main.css'))
  assert.ok(names.includes('templates/message.html') && names.includes('assets/paper.png') && names.includes('previews/cover.png'))
  assert.ok(!names.includes('notes.txt'), '只保留 previews/ 附加文件')
  const manifest = JSON.parse(Buffer.from(files['manifest.json']).toString())
  assert.deepEqual(Object.keys(manifest).sort(), ['assets', 'id', 'license', 'minRendererVersion', 'name', 'rules', 'schemaVersion', 'version'])
  if (process.env.LOG_STYLE_ZIP_OUT) fs.writeFileSync(path.join(process.env.LOG_STYLE_ZIP_OUT, 'studio-built.zip'), bytes)

  const back = await zipIo.readStyleZip(new Blob([bytes]))
  assert.deepEqual(back.issues, [])
  assert.deepEqual(back.pkg, pkg)
  assert.deepEqual(Array.from(back.assets.paper), Array.from(PNG))
  assert.deepEqual(Object.keys(back.extras), ['previews/cover.png'])
})

test('ZIP：本地预检列出服务端会拒绝的问题，并给出可继续编辑的工作包', async () => {
  const bad = zipSync({
    'manifest.json': strToU8(JSON.stringify({ schemaVersion: 1, id: 'bad', version: '1.0.0', name: 'Bad', license: '', minRendererVersion: '9.0.0', rules: [{ templateId: 'm' }], assets: [{ id: 'gone', path: 'assets/gone.png', mime: 'image/png' }], templates: { m: '<p></p>' }, extra: 1 })),
    'templates/m.html': strToU8('<div data-log-slot="body"></div>'),
    'tokens.json': strToU8('{oops'),
    'readme.md': strToU8('hi'),
    'previews/../evil.png': PNG,
  })
  const r = await zipIo.readStyleZip(new Blob([bad]))
  const has = (re) => assert.ok(r.issues.some((m) => re.test(m)), `${re} in ${r.issues.join(' | ')}`)
  has(/缺少 settings\.schema\.json/)
  has(/缺少 styles\/main\.css/)
  has(/未知字段 extra/)
  has(/不能内嵌 templates/)
  has(/tokens\.json 无法解析/)
  has(/assets\/gone\.png 不在 ZIP/)
  has(/无法识别的文件 readme\.md/)
  has(/可疑路径 previews\/\.\.\/evil\.png/)
  assert.ok(r.warnings.some((m) => /需要渲染器 9\.0\.0/.test(m)))
  assert.deepEqual(r.pkg.templates, { m: '<div data-log-slot="body"></div>' })
  assert.deepEqual(r.pkg.tokens, {})
  assert.deepEqual(r.pkg.assets, [])
  assert.deepEqual(r.pkg.rules, [{ templateId: 'm' }])
  assert.deepEqual(r.extras, {}, '不安全的 previews 路径不能进入可重新打包的 extras')

  const many = {}
  for (let i = 0; i < 201; i += 1) many[`templates/t${i}.html`] = strToU8('<p></p>')
  await assert.rejects(zipIo.readStyleZip(new Blob([zipSync(many)])), /文件数超过/)
  await assert.rejects(zipIo.readStyleZip(new Blob([strToU8('not a zip')])), /ZIP|zip|invalid/i)
  assert.equal(zipIo.isSafeStyleZipPath('templates/a.html'), true)
  for (const p of ['/abs', 'a/../b', 'a\\b', 'c:d', 'a//b', './a']) assert.equal(zipIo.isSafeStyleZipPath(p), false, p)
})

test('Style Studio 交互边界：切换工作副本清理旧预览，只读状态禁用另存为', () => {
  const source = fs.readFileSync(path.join(src, 'views/log/components/LogStyleStudio.vue'), 'utf8')
  const loadBlock = source.slice(source.indexOf('const load ='), source.indexOf('const goLibrary ='))
  assert.ok(loadBlock.includes('clearPreview();'), 'load 应清理上一个工作副本的预览')
  assert.ok(source.includes("{ key: 'new', label: '另存为新样式', disabled: readonly.value ||"), '只读工作副本应禁用另存为菜单')
  assert.ok(source.includes("const ownershipUnresolved = computed("), '初次加载归属未知时应保持只读等待确认，避免编辑到一半被判定为他人样式')
})

;(async () => {
  let failed = 0
  for (const [name, fn] of tests) {
    try {
      await fn()
      console.log(`ok - ${name}`)
    } catch (error) {
      failed += 1
      console.error(`not ok - ${name}\n${error && error.stack}`)
    }
  }
  if (failed) {
    console.error(`${failed} failed`)
    process.exit(1)
  }
  console.log(`${tests.length} passed`)
})()
