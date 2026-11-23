// 后台 PDF Render Surface 纯逻辑回归：node scripts/log-pdf-surface-regression.cjs
// 覆盖：切段后元素按锚点分配且不重不漏、整章目录与正常预览一致、多片段页码续接与目录回填。
// 正文渲染依赖 DOM（DOMPurify / 模板编译），这里替换为直通模块，只验证编排与页码流转。
const path = require('node:path')
const fs = require('node:fs')
const Module = require('node:module')
const assert = require('node:assert/strict')
const ts = require('typescript')

const src = path.join(__dirname, '../src')
const contentStub = path.join(__dirname, '__log-pdf-content-stub.cjs')
const compilerStub = path.join(__dirname, '__log-pdf-template-compiler-stub.cjs')
const original = Module._resolveFilename
Module._resolveFilename = function (request, parent, ...args) {
  if (request === './logContent') return contentStub
  if (request === './logTemplateCompiler') return compilerStub
  if (request.startsWith('@/')) request = path.join(src, request.slice(2))
  return original.call(this, request, parent, ...args)
}
const body = (html, plainText) => ({ html, plainText, assets: [] })
Module._cache[contentStub] = { id: contentStub, filename: contentStub, loaded: true, exports: {
  renderMessageBody: (content) => body(`<p>${content}</p>`, content),
  renderPresentedBody: (html, plainText) => body(html, plainText),
  normalizeMentionClasses: () => {},
  attachmentToken: (id) => `att:${id}`,
  bodyAssetToken: () => '',
} }
Module._cache[compilerStub] = { id: compilerStub, filename: compilerStub, loaded: true, exports: {
  buildTemplateLibrary: () => null, fragmentToHtml: () => '', instantiateTemplate: () => null,
  isSafeTokenValue: () => true, sanitizeBodyHtml: (html) => html, templateUsesField: () => false,
} }
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename)

const r = require(path.join(src, 'services/log/logRenderer.ts'))
const surface = require(path.join(src, 'log-pdf/surface.ts'))

const msg = (id) => ({
  message: { id, content: id, createdAt: '2026-10-18T12:00:00Z', icMode: 'ic', identity: { id: 'alice', displayName: '艾丽丝' }, user: { id: 'u' } },
  version: 1,
  capabilities: { reorder: false },
})
const el = (id, type, anchor, content = {}) => ({ id, type, anchor, order: 0, templateId: type, content, style: {} })
const ids = Array.from({ length: 9 }, (_, i) => `m${i}`)
const elements = [
  el('toc', 'toc', { kind: 'sectionStart', sectionId: 's' }, { depth: 2 }),
  el('h1', 'heading', { kind: 'message', sectionId: 's', messageId: 'm0', side: 'before' }, { text: '开场', level: 1 }),
  el('pb', 'pageBreak', { kind: 'message', sectionId: 's', messageId: 'm3', side: 'after' }),
  el('h2', 'heading', { kind: 'message', sectionId: 's', messageId: 'm6', side: 'before' }, { text: '转折', level: 2 }),
  el('end', 'transition', { kind: 'sectionEnd', sectionId: 's' }),
  el('other', 'heading', { kind: 'message', sectionId: 'x', messageId: 'm1', side: 'before' }, { text: '别的章节', level: 1 }),
]
const slices = [[0, 3], [3, 6], [6, 9]].map(([a, b], slice) => ({ sectionId: 's', slice, sliceCount: 3, messages: ids.slice(a, b).map(msg), sectionMessageIds: ids }))

// 切段后的元素分配：sectionStart 只在首段、sectionEnd 只在末段、消息锚点随消息，其他章节的元素不出现。
{
  const got = slices.map((p) => surface.partElements(p, elements).map((e) => e.id))
  assert.deepEqual(got, [['toc', 'h1'], ['pb'], ['h2', 'end']])
  const whole = { sectionId: 's', slice: 0, sliceCount: 1, messages: ids.map(msg) }
  assert.deepEqual(surface.partElements(whole, elements).map((e) => e.id), ['toc', 'h1', 'pb', 'h2', 'end'])
}

// 整章目录：只持有消息 ID 时计算的目录与完整排版时的目录一致（包括章节首节点在 sectionStart 元素之后）。
{
  const full = r.composeDesignSection('s', '第一章', ids.map(msg), elements, true, new Map())
  const tocOnly = r.designSectionToc('s', '第一章', ids, elements)
  assert.deepEqual(tocOnly, full.toc)
  assert.equal(tocOnly[0].text, '第一章')
  assert.deepEqual(tocOnly.map((e) => e.text), ['第一章', '开场', '转折'])
  // 首节点是 sectionStart 元素之后的第一个节点，而不是目录元素本身。
  assert.equal(tocOnly[0].targetId, full.nodes[1].id)
}

// 多片段排版：页码续接、目录在本 chunk 内回填，跨 chunk 目标交给服务端覆盖或报告未解析。
{
  const draft = {
    schemaVersion: 1, title: 'LOG', stylePackageId: 'x', stylePackageVersion: '1', themeTokens: {},
    page: { preset: 'a5', widthMm: 148, heightMm: 210, marginsMm: { top: 10, right: 10, bottom: 10, left: 10 }, columns: 1, fontFamily: 'sans-serif', fontSizePt: 12, lineHeight: 1.5, showPageNumbers: true },
    sections: [{ id: 's', title: '第一章', order: 0, source: {} }],
    elements,
  }
  const run = async (parts, chunk, tocPages, sourceDraft = draft, chapterSections) => {
    const payload = { version: 1, chunk, draft: sourceDraft, style: null, parts, tocPages, chapterSections }
    const builds = surface.buildPartDocuments(payload)
    for (const { doc } of builds) assert.equal(doc.rootAttrs['data-log-message-image-layout'], sourceDraft.messageImageLayout || 'left')
    const layouts = []
    const cells = []
    let pending = 0
    const port = {
      hasAsset: () => false,
      provideAssets: async () => {},
      layout: async (doc) => {
        layouts.push(doc.pageNumberStart)
        const map = {}
        doc.blocks.forEach((b, i) => { map[b.id] = [Math.floor(i / 2) + 1] })
        pending = Math.max(1, Math.ceil(doc.blocks.length / 2))
        // 模板编译被替换为空实现，这里按“含目录元素的片段输出其整章目录单元格”模拟运行时 DOM。
        if (doc.blocks.some((b) => b.id === 'el-toc')) {
          for (const target of builds[layouts.length - 1].tocTargets) cells.push({ target, text: '' })
        }
        return { pageCount: pending, map, warnings: [], missingAssets: 0 }
      },
    }
    const dom = {
      collectPages: () => pending,
      tocCells: () => cells.map((c) => ({ target: c.target, set: (t) => { c.text = t }, current: () => c.text })),
    }
    const loader = { prepare: async (doc) => ({ doc, assets: [] }) }
    const report = await surface.renderChunk(payload, loader, port, dom)
    return { report, layouts, cells }
  }
  ;(async () => {
    const continuousDraft = {
      ...draft, page: { ...draft.page, mergeNeighbors: true },
      chapters: [{ id: 'c', title: '同一章', order: 0 }],
      sections: [{ id: 'a', chapterId: 'c', order: 0 }, { id: 'b', chapterId: 'c', order: 1 }],
      elements: [
        el('toc', 'toc', { kind: 'sectionStart', sectionId: 'a' }, { depth: 2 }),
        el('ha', 'heading', { kind: 'sectionStart', sectionId: 'a' }, { text: 'A', level: 2 }),
        el('hb', 'heading', { kind: 'sectionEnd', sectionId: 'b' }, { text: 'B', level: 2 }),
      ],
    }
    const parts = ['a', 'b'].map((sectionId) => ({ sectionId, slice: 0, sliceCount: 1, messages: [msg(sectionId)] }))
    const docs = surface.buildPartDocuments({ draft: continuousDraft, style: null, parts })
    for (const messageImageLayout of [undefined, 'none', 'left', 'center', 'right']) {
      const projected = surface.buildPartDocuments({ draft: { ...continuousDraft, messageImageLayout }, style: null, parts })
      assert.equal(projected[0].doc.rootAttrs['data-log-message-image-layout'], messageImageLayout || 'left')
    }
    assert.equal(docs.length, 1, '同一视觉 Chapter 只构造一个连续 FrameDocument')
    assert.deepEqual(docs[0].doc.blocks.map((b) => b.id), ['el-toc', 'el-ha', 'a', 'b', 'el-hb'])
    assert.equal(docs[0].doc.blocks.find((b) => b.id === 'b').attrs['data-log-merged'], '1', '跨 Section 合并相邻消息')
    assert.deepEqual(docs[0].tocTargets, ['a', 'el-ha', 'el-hb'])
    const emptyFirst = surface.buildPartDocuments({
      draft: { ...continuousDraft, elements: [] }, style: null,
      parts: [{ ...parts[0], messages: [] }, parts[1]],
    })
    assert.deepEqual(emptyFirst[0].tocTargets, ['b'], '空来源片段不能吞掉后续内容的章节目录项')
    const continuous = await run(parts, { index: 0, count: 1, pageStart: 1 }, undefined, continuousDraft)
    assert.deepEqual(continuous.layouts, [1], '不应对两个 Section 分别 layout')
    assert.deepEqual(continuous.report.unresolvedToc, [])
    for (const target of ['el-ha', 'el-hb']) {
      assert.equal(continuous.cells.find((c) => c.target === target).text, String(continuous.report.nodePages[target]))
    }
    // 同一视觉章跨 chunk：首 chunk 仍携带完整章节目录，后续 Section 的 heading 由第二遍全局页码回填。
    const chapterSections = [
      { sectionId: 'a', messageIds: ['a'] },
      { sectionId: 'b', messageIds: ['b'] },
    ]
    const crossFirst = await run([parts[0]], { index: 0, count: 2, pageStart: 1 }, undefined, continuousDraft, chapterSections)
    assert.deepEqual(surface.buildPartDocuments({ draft: continuousDraft, style: null, parts: [parts[0]], chapterSections })[0].tocTargets, ['a', 'el-ha', 'el-hb'])
    assert.deepEqual(crossFirst.report.unresolvedToc, ['el-hb'])
    const crossSecond = await run([parts[0]], { index: 0, count: 2, pageStart: 1 }, { 'el-hb': 42 }, continuousDraft, chapterSections)
    assert.equal(crossSecond.cells.find((c) => c.target === 'el-hb').text, '42')
    assert.deepEqual(crossSecond.report.unresolvedToc, [])
    // chunk 1：首段 + 第二段，起始页 5；第三段（h2）在下一个 chunk。
    const first = await run(slices.slice(0, 2), { index: 0, count: 2, pageStart: 5 })
    assert.equal(first.layouts[0], 5)
    assert.equal(first.layouts[1], 5 + Math.ceil(surface.buildPartDocuments({ draft, style: null, parts: [slices[0]] })[0].doc.blocks.length / 2))
    assert.equal(first.report.pageCount, first.layouts[1] - 5 + Math.ceil(surface.buildPartDocuments({ draft, style: null, parts: [slices[1]] })[0].doc.blocks.length / 2))
    assert.ok(first.report.nodePages['el-h1'] >= 5)
    assert.deepEqual(first.report.unresolvedToc, ['el-h2'])
    // 第二轮：服务端提供全局页码后，跨 chunk 目录被填写且不再报告未解析。
    const second = await run(slices.slice(0, 2), { index: 0, count: 2, pageStart: 5 }, { 'el-h2': 42 })
    assert.deepEqual(second.report.unresolvedToc, [])
    assert.equal(second.cells.find((c) => c.target === 'el-h2').text, '42')
    assert.equal(second.cells.find((c) => c.target === 'el-h1').text, String(second.report.nodePages['el-h1']))
    console.log('log-pdf-surface regression ok')
  })().catch((err) => {
    console.error(err)
    process.exit(1)
  })
}
