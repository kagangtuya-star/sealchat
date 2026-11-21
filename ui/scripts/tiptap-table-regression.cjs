// node scripts/tiptap-table-regression.cjs
const fs = require('node:fs');
const path = require('node:path');
const Module = require('node:module');
const assert = require('node:assert/strict');
const ts = require('typescript');
const src = path.join(__dirname, '../src');
const original = Module._resolveFilename;
const config = path.join(__dirname, '__table-config-stub.cjs');
Module._cache[config] = { id: config, filename: config, loaded: true, exports: { urlBase: '' } };
Module._resolveFilename = function (request, parent, ...args) {
  if (request === '@/stores/_config') return config;
  if (request.startsWith('@/')) request = path.join(src, request.slice(2));
  return original.call(this, request, parent, ...args);
};
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename);

const { tiptapJsonToHtml: html, tiptapJsonToPlainText: plain } = require(path.join(src, 'utils/tiptap-render.ts'));
const { convertTextContentFormat } = require(path.join(src, 'utils/textContentFormat.ts'));
const text = (value, marks) => ({ type: 'text', text: value, marks });
const p = (...content) => ({ type: 'paragraph', content });
const cell = (value, type = 'tableCell', attrs) => ({ type, attrs, content: [p(text(value))] });
const row = (...content) => ({ type: 'tableRow', content });
const doc = (...content) => ({ type: 'doc', content: [{ type: 'table', content }] });
const basic = doc(row(cell('甲', 'tableHeader'), cell('乙', 'tableHeader')), row(cell('丙'), cell('丁')));
assert.equal((html(basic).match(/<tr>/g) || []).length, 2);
assert.equal((html(basic).match(/<th /g) || []).length, 2);
assert.equal((html(basic).match(/<td /g) || []).length, 2);
assert.match(html(basic), /class="tiptap-table-wrapper"><table class="tiptap-table" style="min-width: 160px"><colgroup>/);
assert.equal(plain(basic), '甲\t乙\n丙\t丁');
assert.equal(convertTextContentFormat(JSON.stringify(basic), 'tiptap', 'plain'), '甲\t乙\n丙\t丁');

const merged = doc(row(cell('合并', 'tableHeader', { colspan: 2, rowspan: 3, colwidth: [120, 180] })));
assert.match(html(merged), /<th colspan="2" rowspan="3">/);
assert.match(html(merged), /style="width: 300px"><colgroup><col style="width: 120px" \/><col style="width: 180px" \/>/);
assert.doesNotMatch(html(merged), /colwidth=/);
assert.match(html(doc(row(cell('小数', 'tableCell', { colspan: 2, colwidth: [120.5, 180.25] })))), /style="width: 300.75px"><colgroup><col style="width: 120.5px"/);
const partial = doc(row(cell('部分', 'tableCell', { colspan: 2, colwidth: [120, 0] })));
assert.match(html(partial), /style="min-width: 200px"/);

const rich = doc(row({ type: 'tableCell', content: [
  p(text('<&"\'>', [{ type: 'bold' }, { type: 'link', attrs: { href: 'https://example.com/?a=1&b=2' } }])),
  p(text('第二段')), p({ type: 'image', attrs: { src: 'id:asset', alt: '图' } }),
] }));
assert.match(html(rich), /<strong>&lt;&amp;&quot;&#039;&gt;<\/strong>/);
assert.match(html(rich), /href="https:\/\/example.com\/\?a=1&amp;b=2"/);
assert.match(html(rich), /<p>第二段<\/p><p><img/);
assert.match(plain(rich), /第二段/);

for (const invalid of [-1, 0, 1.5, Infinity, NaN, '2', '1" onclick="bad()', {}, 1e20]) {
  const attack = doc(row(cell('安全', 'tableCell', {
    colspan: invalid, rowspan: invalid, colwidth: [invalid], style: 'position:fixed', class: 'evil', onclick: 'bad()',
  })));
  const output = html(attack);
  assert.match(output, /<td colspan="1" rowspan="1">/);
  assert.match(output, invalid === 1.5 ? /width: 1.5px/ : /min-width: 80px/);
  assert.doesNotMatch(output, /onclick|position|evil|Infinity|NaN|\[object Object\]/);
}
assert.match(html(doc(row(cell('过大', 'tableCell', { colspan: 1001, rowspan: 65535, colwidth: [10001] })))), /<td colspan="1" rowspan="1">/);
assert.equal(html({ type: 'doc', content: [p(text('旧消息'))] }), '<p>旧消息</p>');
console.log('TipTap table renderer/plain-text regression passed');
