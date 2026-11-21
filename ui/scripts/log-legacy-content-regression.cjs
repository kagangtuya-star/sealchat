// node scripts/log-legacy-content-regression.cjs
// 复用 LOG 脚本的 TypeScript 加载方式，执行真实 Element / quick-format / TipTap / contentEscape。
// 最小 DOM 只验证 finalizeBody 的令牌和文本流转；DOMPurify 替身记录输入及配置，
// 不能替代浏览器中的 DOMPurify 安全验证。安全断言也检查清洗前 HTML，防止依赖替身掩盖问题。
const path = require('node:path')
const fs = require('node:fs')
const Module = require('node:module')
const assert = require('node:assert/strict')
const ts = require('typescript')
const src = path.join(__dirname, '../src')
const sanitizations = []
const decodeText = (value) => value.replace(/&(lt|gt|quot|amp|nbsp);/g, (_, name) => ({
  lt: '<', gt: '>', quot: '"', amp: '&', nbsp: '\u00a0',
})[name])
const escapeAttribute = (value) => value.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;')
const textOf = (html) => decodeText(html.replace(/<[^>]*>/g, ''))

const fragment = (html) => {
  const images = []
  const markup = html.replace(/<img\b[^>]*>/gi, (tag) => {
    const attrs = new Map(Array.from(tag.matchAll(/([\w-]+)="([^"]*)"/g), (m) => [m[1], decodeText(m[2])]))
    const img = {
      getAttribute: (key) => attrs.get(key) ?? null,
      removeAttribute: (key) => attrs.delete(key),
      setAttribute: (key, value) => attrs.set(key, value),
      replaceWith: (element) => { img.replacement = element },
      html: () => img.replacement ? `<span class="${img.replacement.className}">${img.replacement.textContent}</span>`
        : `<img ${Array.from(attrs, ([key, value]) => `${key}="${escapeAttribute(value)}"`).join(' ')}>`,
    }
    images.push(img)
    return `__IMAGE_${images.length - 1}__`
  })
  return {
    querySelectorAll: (selector) => selector === 'img' ? images : [],
    html: () => markup.replace(/__IMAGE_(\d+)__/g, (_, index) => images[Number(index)].html()),
  }
}
globalThis.document = {
  createElement: () => ({
    className: '', innerHTML: '',
    get textContent() { return textOf(this.innerHTML) },
    set textContent(value) { this.innerHTML = escapeAttribute(value) },
    appendChild(frag) { this.innerHTML = frag.html() },
  }),
}
const stubs = {
  // contentEscape is real; the unrelated memoization helper is never invoked here.
  'lodash-es': { memoize: () => { throw new Error('unexpected memoize use') } },
  dompurify: { sanitize: (html, options) => {
    sanitizations.push({ html, options })
    return fragment(html)
  } },
  '@/stores/_config': { urlBase: '' },
  '@/composables/useAttachmentResolver': { normalizeAttachmentId: (value) => value.replace(/^id:/, '').replace(/^\/api\/v1\/attachment\//, '') },
}
const originalResolve = Module._resolveFilename
Module._resolveFilename = function (request, parent, ...args) {
  if (Object.hasOwn(stubs, request)) return `__log_legacy_stub_${request}`
  if (request.startsWith('@/')) request = path.join(src, request.slice(2))
  return originalResolve.call(this, request, parent, ...args)
}
for (const [name, exports] of Object.entries(stubs)) {
  const id = `__log_legacy_stub_${name}`
  Module._cache[id] = { id, filename: id, loaded: true, exports }
}
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename)

const Element = require('@satorijs/element')
const { renderMessageBody, renderPresentedBody } = require(path.join(src, 'services/log/logContent.ts'))
const { tiptapJsonToHtml, tiptapJsonToPlainText } = require(path.join(src, 'utils/tiptap-render.ts'))
const render = (content) => {
  const before = sanitizations.length
  const result = renderMessageBody(content)
  assert.equal(sanitizations.length, before + 1, '消息必须继续经过 finalizeBody / DOMPurify')
  const { options } = sanitizations.at(-1)
  assert.equal(options.RETURN_DOM_FRAGMENT, true)
  for (const tag of ['style', 'script', 'iframe', 'svg', 'math', 'form', 'audio']) {
    assert.ok(options.FORBID_TAGS.includes(tag))
  }
  return result
}

// 真实解析行为：裸 br 回滚为 text；已转义 br 也解码为 text，必须在解析前区分。
assert.equal(Element.parse('a<br>b')[1].type, 'text')
assert.equal(Element.parse('&lt;br/&gt;')[0].attrs.content, '<br/>')
assert.equal(Element.parse('A&nbsp;B')[0].attrs.content, 'A&nbsp;B')
assert.equal(Element.parse('<span data-dice-source="2d6"/>')[0].attrs.dataDiceSource, '2d6')

{
  const body = render('第一行<br/>第二行<br />第三行<br>第四行')
  assert.equal(body.html.replace(/<br\s*\/?>/g, '\n'), '第一行\n第二行\n第三行\n第四行')
  assert.equal(body.plainText, '第一行\n第二行\n第三行\n第四行')
  assert.ok(!textOf(body.html).includes('<br'))
  for (const br of ['<BR/>', '<BR>', '<Br />', '<br    >']) assert.equal(render(`a${br}b`).html, 'a<br>b')
  for (const br of ['&lt;br/&gt;', '&lt;br&gt;', '&lt;BR /&gt;']) {
    const body = render(`文字 ${br} 文字`)
    assert.equal(body.html, `文字 ${br} 文字`)
    assert.ok(!/<br\b/i.test(body.html))
    assert.equal(body.plainText, `文字 ${decodeText(br)} 文字`)
  }
}
{
  assert.equal(render('<strong>粗体</strong><em>斜体</em><code>.ra</code>').html,
    '<strong>粗体</strong><em>斜体</em><code>.ra</code>')
  assert.equal(render('<b>B</b><i>I</i><s>S</s><strike>T</strike><del>D</del>').html,
    '<strong>B</strong><em>I</em><s>S</s><s>T</s><s>D</s>')
  const blocks = render('<p>第一段<br/>第二行</p><div>第三段</div>')
  assert.equal(blocks.html, '<p>第一段<br>第二行</p><div>第三段</div>')
  assert.equal(blocks.plainText, '第一段\n第二行\n第三段')
  assert.equal(render('<blockquote><p>引用</p></blockquote><ul><li>甲</li><li>乙</li></ul><hr>').html,
    '<blockquote><p>引用</p></blockquote><ul><li>甲</li><li>乙</li></ul><hr>')
  assert.equal(render('<HR />').html, '<hr>')
  assert.equal(render('<pre>\n  a\n    b\n</pre>').html, '<pre>\n  a\n    b\n</pre>')
  assert.equal(render('<pre><code>\n  **a**<br>b &lt;script&gt;\n</code></pre>').html,
    '<pre>\n  **a**\nb &lt;script&gt;\n</pre>')
  assert.equal(render('<code>**a** &lt;br/&gt;</code>').html, '<code>**a** &lt;br/&gt;</code>')
  assert.equal(render('**粗体** *斜体* `.ra`\na').html, '<strong>粗体</strong> <em>斜体</em> <code>.ra</code><br />a')
}
{
  const link = render('<a href="https://example.com?q=1&amp;b=2" onclick="bad()" style="color:red" data-secret="x">example</a>')
  assert.match(link.html, /href="https:\/\/example.com\/\?q=1&amp;b=2"/)
  assert.match(link.html, /rel="noopener noreferrer"/)
  assert.equal(textOf(link.html), 'example')
  assert.ok(!/onclick|style=|data-secret/.test(sanitizations.at(-1).html))
  for (const href of ['javascript:alert(1)', 'java&#x73;cript:alert(1)', 'data:text/html,evil', 'vbscript:evil', '//evil.test', '/local', 'https://']) {
    assert.equal(render(`<a href="${href}"><b>链接文字</b></a>`).html, '<strong>链接文字</strong>')
    assert.ok(!/href=/.test(sanitizations.at(-1).html))
  }
  assert.match(render('<a href="http://example.com">http</a>').html, /href="http:\/\/example.com\/"/)
}
{
  const chip = '<span class="dice-chip" data-dice-source="1d6"><b>结果6</b></span>'
  assert.equal(render(chip).html, '1d6')
  assert.equal(render(chip).plainText, '1d6')
  assert.equal(render(`<span class="dice-roll-group" data-dice-source="2#1d6">${chip}${chip}</span>`).html, '2#1d6')
  assert.equal(render('<span class="dice-chip"><b>结果6</b></span>').html, '结果6')
  assert.equal(render('<span class="dice-roll-group"><span class="dice-chip">6</span><span>、7</span></span>').html, '6、7')
  const malicious = render('<span class="dice-chip" data-dice-source="&lt;img src=x onerror=bad()&gt;">6</span>')
  assert.equal(malicious.html, '&lt;img src=x onerror=bad()&gt;')
  assert.ok(!/<(?:span|img)\b/.test(malicious.html))
  // 属性内的 br 文本不能被换行规范化修改。
  assert.equal(render('<span class="dice-chip" data-dice-source="1d6&lt;br&gt;">6</span>').html, '1d6&lt;br&gt;')
  assert.equal(render('<span class="dice-chip" data-dice-source>6</span>').html, '6')
  assert.equal(render('<span class="ordinary" style="color:red"><b>**原样正文**</b></span>').html, '**原样正文**')
}
{
  const nbsp = render('A&nbsp;B')
  assert.equal(textOf(nbsp.html), 'A\u00a0B')
  assert.ok(!nbsp.html.includes('&amp;nbsp;'))
  assert.equal(textOf(render('A&#160;B / A&#xA0;B').html), 'A\u00a0B / A\u00a0B')
  assert.equal(render('&amp;nbsp;').html, '&amp;nbsp;', '有意转义的 entity 不能多轮解码')
  assert.equal(render('&lt;script&gt;alert(1)&lt;/script&gt;').html, '&lt;script&gt;alert(1)&lt;/script&gt;')
  assert.equal(render('&amp;lt;script&amp;gt;').html, '&amp;lt;script&amp;gt;')
  assert.equal(render('A &amp; B &quot;C&quot;').plainText, 'A & B "C"')
  const unknown = render('<custom onclick="bad()"><strong>安全正文</strong></custom><script>bad()</script><style>bad</style><iframe>frame</iframe><form>form</form><svg>svg</svg>')
  assert.equal(unknown.html, '安全正文bad()badframeformsvg')
  assert.ok(!/<|&lt;/.test(sanitizations.at(-1).html), '未知节点的标签和属性不能进入 HTML')
  assert.equal(render('<strong onclick="bad()" class="evil" style="color:red" data-x="secret">B</strong>').html, '<strong>B</strong>')
}
{
  const body = render('<img src="id:image-123" onerror="bad()"/><audio src="id:voice-123"/><at id="user" name="甲 &lt;乙&gt;"/>')
  assert.deepEqual(body.assets, ['att:image-123'])
  assert.match(body.html, /data-log-asset="att:image-123"/)
  assert.match(body.html, /<span class="log-audio">\[语音\]<\/span>/)
  assert.match(body.html, /<span class="log-mention">@甲 &lt;乙&gt;<\/span>/)
  assert.equal(body.plainText, '[语音]@甲 <乙>')
  assert.ok(!/\bsrc=|onerror=|<audio\b/.test(body.html))
  assert.equal(render('<img src="https://evil.test/x"/>').html, '<span class="log-img-missing">图片不可用</span>')
  assert.equal(render('<at id="user-123"/>').html, '<span class="log-mention">@user-123</span>')
}
{
  const content = JSON.stringify({ type: 'doc', content: [{ type: 'paragraph', content: [
    { type: 'text', text: '汉字', marks: [{ type: 'ruby', attrs: { rubyText: 'hàn zì' } }] },
    { type: 'text', text: '秘密', marks: [{ type: 'spoiler' }] },
    { type: 'text', text: 'example', marks: [{ type: 'link', attrs: { href: 'https://example.com' } }] },
    { type: 'text', text: '<br/> &nbsp;' },
    { type: 'image', attrs: { src: 'id:image-123', alt: '图片' } },
  ] }] })
  const originalParse = Element.parse
  Element.parse = () => { throw new Error('TipTap 不应进入 legacy 解析') }
  try {
    const body = render(content)
    const expected = tiptapJsonToHtml(content, { attachmentResolver: () => 'log-asset:att:image-123', staticDocument: true })
    assert.equal(sanitizations.at(-1).html, expected, 'TipTap 清洗前 HTML 使用原渲染器的静态模式')
    assert.equal(body.plainText, tiptapJsonToPlainText(content))
    assert.deepEqual(body.assets, ['att:image-123'])
    assert.match(body.html, /<ruby/)
    assert.doesNotMatch(body.html, /tiptap-spoiler/)
    assert.match(body.html, /href="https:\/\/example.com"/)
    assert.match(body.html, /&lt;br\/&gt; &amp;nbsp;/)
  } finally { Element.parse = originalParse }
  const html = '<div class="log-embed">便签<br>正文</div>'
  assert.equal(renderPresentedBody(html, '便签正文').html, html, '服务端展示投影不进入 legacy 收口')
}

console.log('log legacy content regression: ok')
