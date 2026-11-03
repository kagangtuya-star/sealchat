// LOG 章节编排纯函数回归：node scripts/log-chapter-arrangement-regression.cjs
// 使用现有 TypeScript 编译器与 node:assert；logRenderer 只被 logDraftOps 间接引用，这里替换为空模块。
const path = require('node:path')
const fs = require('node:fs')
const Module = require('node:module')
const assert = require('node:assert/strict')
const ts = require('typescript')

const src = path.join(__dirname, '../src')
const stub = path.join(__dirname, '__log-renderer-stub.cjs')
const original = Module._resolveFilename
Module._resolveFilename = function (request, parent, ...args) {
  if (request === '@/services/log/logRenderer') return stub
  if (request.startsWith('@/')) request = path.join(src, request.slice(2))
  return original.call(this, request, parent, ...args)
}
Module._cache[stub] = { id: stub, filename: stub, loaded: true, exports: { defaultElementContent: () => ({}), templatesForElementType: () => [] } }
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename)

const ops = require(path.join(src, 'views/log/logChapterOps.ts'))
const { cloneJson, newSection } = require(path.join(src, 'views/log/logDraftOps.ts'))

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const source = (channelId, fromTime = null) => ({ channelId, fromTime, toTime: '2026-10-12T14:00:00.000Z', includeOoc: true, includeArchived: false, identityIds: [] })
const section = (id, title, order, channelId, extra = {}) => ({ id, title, order, source: source(channelId), ...extra })
const legacyDraft = () => ({
  schemaVersion: 1, title: 'LOG', elements: [{ id: 'e1', type: 'heading', anchor: { kind: 'sectionStart', sectionId: 's2' }, order: 0, templateId: 'heading', content: {}, style: {} }],
  page: {}, stylePackageId: 'x', stylePackageVersion: '1', themeTokens: {},
  sections: [section('s1', '序章', 5, 'hall'), section('s2', '第二章', 9, 'alice'), section('s3', '第三章', 7, 'hall')],
})
/** 与服务端 validateLogChapters 相同的不变量。 */
const assertValid = (d) => {
  const sorted = d.sections.slice().sort((a, b) => a.order - b.order)
  assert.equal(new Set(d.sections.map((s) => s.order)).size, d.sections.length, 'section orders unique')
  if (!d.chapters?.length) {
    assert.ok(d.sections.every((s) => !('chapterId' in s)), 'legacy sections carry no chapterId')
    return
  }
  const chapters = new Map(d.chapters.map((c) => [c.id, c]))
  assert.equal(chapters.size, d.chapters.length)
  assert.equal(new Set(d.chapters.map((c) => c.order)).size, d.chapters.length)
  const seen = []
  for (const s of sorted) {
    assert.ok(chapters.has(s.chapterId), 'section references a chapter')
    if (seen[seen.length - 1] !== s.chapterId) {
      assert.ok(!seen.includes(s.chapterId), 'chapter contiguous')
      if (seen.length) assert.ok(chapters.get(s.chapterId).order > chapters.get(seen[seen.length - 1]).order, 'chapter order matches')
      seen.push(s.chapterId)
    }
    assert.ok(UUID.test(s.chapterId))
    assert.ok(!d.sections.some((x) => x.id === s.chapterId), 'chapter id not a section id')
  }
  assert.equal(seen.length, d.chapters.length, 'no empty chapters')
}
const flat = (d) => ops.chapterViews(d).map((v) => `${v.title}[${v.sections.map((s) => s.id).join(',')}]`).join(' ')

// 新来源默认隐藏回复引用；旧草稿缺字段时也按隐藏处理，显式开启后才显示。
{
  assert.equal(newSection('hall').source.showQuoteContent, false)
  const legacyFlags = ops.sourceFlags(source('hall'))
  assert.deepEqual(legacyFlags.find((f) => f.key === 'quote'), { key: 'quote', text: '隐藏引用', notable: false })
  assert.deepEqual(ops.sourceFlags({ ...source('hall'), showQuoteContent: true }).find((f) => f.key === 'quote'), { key: 'quote', text: '显示引用', notable: true })
}

// 旧草稿：每个 section 是一个隐式单来源章节；只改顺序/标题时保持旧格式。
{
  const d = legacyDraft()
  const views = ops.chapterViews(d)
  assert.equal(flat(d), '序章[s1] 第三章[s3] 第二章[s2]')
  assert.ok(views.every((v) => v.implicit && v.key === v.sections[0].id))
  const next = cloneJson(d)
  ops.writeLayout(next, ops.moveChapter(ops.layoutOf(views), 's2', 0))
  assert.equal(next.chapters, undefined)
  assert.equal(flat(next), '第二章[s2] 序章[s1] 第三章[s3]')
  assert.deepEqual(next.sections.map((s) => [s.id, s.order]), [['s1', 1], ['s2', 0], ['s3', 2]])
  ops.writeLayout(next, ops.renameChapter(ops.layoutOf(ops.chapterViews(next)), 's1', '新序章'))
  assert.equal(next.sections[0].title, '新序章')
  assert.equal(next.chapters, undefined)
  assertValid(next)
}

// 出现多来源章节时物化章节层：旧章节标题成为章节标题，片段名称留空；顺序稳定且连续。
{
  const d = legacyDraft()
  const layout = ops.layoutOf(ops.chapterViews(d))
  const keys = ops.writeLayout(d, ops.moveSection(layout, 's2', 's1', 1))
  assertValid(d)
  assert.equal(d.chapters.length, 2)
  assert.equal(flat(d), '序章[s1,s2] 第三章[s3]')
  assert.equal(d.sections.find((s) => s.id === 's1').title, '')
  assert.equal(d.sections.find((s) => s.id === 's2').title, '第二章', 'merged section keeps its old title as label')
  assert.ok(UUID.test(keys.get('s1')) && keys.get('s1') === d.sections[0].chapterId)
  assert.equal(d.elements.length, 1, 'anchors untouched')
  // 再次写回同一布局：id 与 order 不变（幂等，拖拽后保存再加载顺序稳定）。
  const before = JSON.stringify(d)
  ops.writeLayout(d, ops.layoutOf(ops.chapterViews(d)))
  assert.equal(JSON.stringify(d), before)
  // 章节模式下即使全部变回单来源也不回退到旧格式（章节 id 稳定）。
  const ids = d.chapters.map((c) => c.id)
  ops.writeLayout(d, ops.detachSection(ops.layoutOf(ops.chapterViews(d)), 's2', '第二章', 'tmp-key'))
  assertValid(d)
  assert.equal(flat(d), '序章[s1] 第二章[s2] 第三章[s3]')
  assert.ok(ids.every((id) => d.chapters.some((c) => c.id === id)))
}

// 同一频道可在同一章节出现多次；片段排序、跨章移动、合并与删除。
{
  const d = legacyDraft()
  d.sections.push(section('s4', '', 10, 'hall'), section('s5', '', 11, 'hall'))
  let layout = ops.layoutOf(ops.chapterViews(d))
  layout = ops.mergeIntoPrevious(layout, 's2') // 第三章 + 第二章
  layout = ops.moveSection(layout, 's4', 's3', 0)
  layout = ops.moveSection(layout, 's5', 's3', 3)
  ops.writeLayout(d, layout)
  assertValid(d)
  const [, third] = ops.chapterViews(d)
  assert.deepEqual(third.sections.map((s) => s.id), ['s4', 's3', 's2', 's5'])
  assert.equal(third.sections.filter((s) => s.source.channelId === 'hall').length, 3, 'same channel kept, not deduplicated')
  // 同一列表内向下拖：插入位置按移除前计算。
  layout = ops.moveSection(ops.layoutOf(ops.chapterViews(d)), 's4', third.key, 3)
  ops.writeLayout(d, layout)
  assert.deepEqual(ops.chapterViews(d)[1].sections.map((s) => s.id), ['s3', 's2', 's4', 's5'])
  // 移走章节最后一个片段时，空章节被移除而不是留下孤立章节。
  ops.writeLayout(d, ops.moveSection(ops.layoutOf(ops.chapterViews(d)), 's1', third.key, 0))
  assertValid(d)
  assert.equal(ops.chapterViews(d).length, 1)
  // 删除片段同时删除其元素锚点；删除整章的最后片段不留空章节。
  ops.removeSections(d, ['s2'])
  assertValid(d)
  assert.equal(d.elements.length, 0)
  assert.ok(!d.sections.some((s) => s.id === 's2'))
  ops.removeSections(d, d.sections.map((s) => s.id))
  assert.deepEqual(d.sections, [])
  assert.equal(d.chapters, undefined)
}

// 布局遗漏或重复的片段不会被静默丢弃。
{
  const d = legacyDraft()
  ops.writeLayout(d, [{ key: 'x', title: 'A', sectionIds: ['s1', 's1', 'missing'] }, { key: 'y', title: 'B', sectionIds: ['s2'] }])
  assert.equal(d.sections.length, 3)
  assert.deepEqual(d.sections.slice().sort((a, b) => a.order - b.order).map((s) => s.id), ['s1', 's2', 's3'])
}

// 章节整体移动只改顺序，不改变章节内片段序列（成员允许的操作）。
{
  const d = legacyDraft()
  ops.writeLayout(d, ops.moveSection(ops.layoutOf(ops.chapterViews(d)), 's3', 's1', 1))
  const inner = (x) => ops.chapterViews(x).map((v) => v.sections.map((s) => s.id).join(','))
  const before = inner(d)
  const moved = cloneJson(d)
  const views = ops.chapterViews(moved)
  ops.writeLayout(moved, ops.moveChapter(ops.layoutOf(views), views[1].key, 0))
  assertValid(moved)
  assert.deepEqual(inner(moved), before.slice().reverse())
  assert.deepEqual(moved.chapters.map((c) => c.id).sort(), d.chapters.map((c) => c.id).sort())
  assert.deepEqual(moved.sections.map((s) => s.chapterId), d.sections.map((s) => s.chapterId))
}

// 来源摘要与接续片段。
{
  const now = new Date('2026-10-12T00:00:00')
  const s = { ...source('hall', new Date('2026-10-12T20:00:00').toISOString()), toTime: new Date('2026-10-12T22:30:00').toISOString() }
  assert.equal(ops.formatSourceRange(s, now), '10/12 20:00 → 22:30')
  assert.equal(ops.formatSourceRange({ ...s, fromTime: null }, now), '最早 → 10/12 22:30')
  assert.equal(ops.formatSourceRange({ ...s, toTime: new Date('2025-01-02T03:04:00').toISOString(), fromTime: new Date('2024-12-31T01:00:00').toISOString() }, now), '2024/12/31 01:00 → 2025/01/02 03:04')
  assert.deepEqual(ops.sourceFlags({ ...s, identityIds: ['a', 'b'], includeOoc: false }).filter((f) => f.notable).map((f) => f.text), ['2 个角色', '不含 OOC'])
  const afterBoundary = new Date(Date.parse(s.toTime) + 60 * 60 * 1000)
  assert.deepEqual(ops.continuationSourceRange({ source: s }, afterBoundary), {
    fromTime: s.toTime,
    fromExclusive: true,
    toTime: afterBoundary.toISOString(),
  })
  const beforeBoundary = new Date(Date.parse(s.toTime) - 60 * 60 * 1000)
  assert.deepEqual(ops.continuationSourceRange({ source: s }, beforeBoundary), {
    fromTime: s.toTime,
    fromExclusive: true,
    toTime: s.toTime,
  })
  assert.equal(ops.sectionLabel({ title: '  ', source: s }, () => '大厅'), '大厅')
  assert.equal(ops.chapterTitle({ title: '' }, 2), '第 3 章')
}

console.log('log chapter arrangement regression: ok')
