// node scripts/log-static-rich-regression.cjs [--serve]
// Real browser DOMPurify, CSS cascade and pagination; TypeScript-only bundling
// avoids downloading a browser or requiring a platform-specific build binary.
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { execFile } = require('node:child_process');
const { promisify } = require('node:util');
const assert = require('node:assert/strict');
const ts = require('typescript');
const ui = path.join(__dirname, '..');
const src = path.join(ui, 'src');
const fixture = JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures/log-static-rich.json'), 'utf8'));
const modules = [];
const ids = new Map();
require.extensions['.ts'] ||= () => { throw Error('TypeScript is bundled, not required'); };
const stubs = {
  '@/stores/_config': 'exports.urlBase = "";',
  '@/composables/useAttachmentResolver': 'exports.normalizeAttachmentId = value => value.replace(/^id:/, "").replace(/^\\/api\\/v1\\/attachment\\//, "");',
  'lodash-es': 'exports.memoize = () => { throw Error("unexpected memoize use") };',
};
const bundle = (file, parent = __filename) => {
  const request = file.startsWith('@/') ? path.join(src, file.slice(2)) : file.startsWith('.') ? path.resolve(path.dirname(parent), file) : file;
  const key = Object.hasOwn(stubs, file) ? file : require.resolve(request, { paths: [path.dirname(parent)] });
  if (ids.has(key)) return ids.get(key);
  const id = modules.length;
  ids.set(key, id); modules.push('');
  let code = stubs[key] || fs.readFileSync(key, 'utf8');
  if (key.endsWith('.ts')) code = ts.transpileModule(code, { fileName: key, compilerOptions: {
    target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true,
  } }).outputText;
  code = code.replace(/\brequire\(["']([^"']+)["']\)/g, (_all, request) => `require(${bundle(request, key)})`);
  modules[id] = `function(module,exports,require){\n${code}\n}`;
  return id;
};
const entry = bundle(path.join(src, 'services/log/logRenderer.ts'));
const compiler = bundle(path.join(src, 'services/log/logTemplateCompiler.ts'));
const content = bundle(path.join(src, 'services/log/logContent.ts'));
const tiptap = bundle(path.join(src, 'utils/tiptap-render.ts'));

async function check(r, c, content, tiptap, fixture) {
  let checks = 0;
  const ok = (value, message) => { checks++; if (!value) throw Error(message); };
  const mount = html => { const el = document.createElement('div'); el.className = 'log-body'; el.innerHTML = html; document.body.appendChild(el); return el; };
  const page = { fontFamily: 'serif', fontSizePt: 12, lineHeight: 1.5, backgroundColor: '#000000', mergeNeighbors: false,
    widthMm: 100, heightMm: 60, marginsMm: { top: 5, right: 5, bottom: 5, left: 5 }, columns: 1 };
  const item = { message: { id: 'test', content: JSON.stringify(fixture), user: { id: 'user' } }, capabilities: {} };
  const build = (node, style = null) => r.buildFrameDocument({ nodes: [node], page, style, title: '', sectionTitle: '', toc: [], backgroundToken: null }).doc;
  const design = r.designMessageToNode(item);
  const doc = build(design);
  const fallback = mount(doc.blocks[0].html);
  ok(fallback.querySelector('p').style.textAlign === 'center', 'paragraph alignment');
  const mixed = fallback.querySelector('mark[data-platform-font-id]');
  ok(mixed.style.fontSize === '28px' && mixed.style.color === 'rgb(255, 255, 255)', 'explicit size/color survive daytime palette');
  ok(mixed.style.backgroundColor === 'rgb(18, 52, 86)', 'highlight');
  ok(mixed.style.fontFamily.includes('sc-log-platform-body-font'), 'platform body family remapped');
  ok(['strong', 'em', 'u', 's'].every(tag => mixed.closest(tag)), 'stacked marks');
  ok(mixed.classList.contains('tiptap-text-decoration--triple') && mixed.classList.contains('tiptap-text-decoration--wave-heavy'), 'advanced decoration');
  ok(getComputedStyle(mixed).backgroundImage.includes('radial-gradient'), 'decoration has static CSS');
  ok(fallback.querySelector('p').firstChild.nodeType === Node.TEXT_NODE, 'unset text keeps inheritance');
  ok(getComputedStyle(fallback.querySelector('p')).fontSize === '16px', 'unset font size follows LOG');
  const ruby = fallback.querySelector('ruby');
  ok(ruby.querySelector('rt').textContent === 'hàn zì', 'real Ruby structure');
  ok(getComputedStyle(ruby).fontSize === '30px' && getComputedStyle(ruby.querySelector('rt')).fontSize === '12px', 'Ruby base/rt sizes');
  ok(getComputedStyle(ruby).color === 'rgb(171, 205, 239)', 'Ruby color overrides surrounding style');
  ok(ruby.style.getPropertyValue('--ruby-rt-font-family').includes('sc-log-platform-rt-font'), 'Ruby font mapping');
  ok(!/performance|spoiler|on\w+=|!important/.test(fallback.innerHTML), 'effects removed');
  ok(!fallback.querySelector('button,[tabindex]') && fallback.textContent.includes('@提及者'), 'mention and no interaction');
  ok(fallback.querySelector('img[data-log-asset]') && !fallback.querySelector('img[src]'), 'image tokens only');
  ok(fallback.querySelector('h2').style.textAlign === 'right' && fallback.querySelector('blockquote') && fallback.querySelector('ol li') && fallback.querySelector('pre code').textContent === '<安全代码>', 'block semantics');
  const libStyle = { templates: { message: '<article><div data-log-slot="body"></div></article>' }, css: '', rules: [{ templateId: 'message' }], settingsSchema: {}, tokens: {} };
  const templated = mount(build(design, libStyle).blocks[0].html);
  ok(templated.querySelector('.log-body').innerHTML === fallback.querySelector('.log-body').innerHTML, 'template and fallback share sanitizer');
  const published = r.publicNodeToRender({ kind: 'message', id: 'published', messageType: 'ic', speaker: { key: 'key', name: '' }, bodyHtml: design.bodyHtml, plainText: design.plainText });
  ok(mount(build(published).blocks[0].html).querySelector('ruby').outerHTML === ruby.outerHTML, 'public body uses same static CSS/whitelist');
  const dayHtml = c.sanitizeBodyHtml(tiptap.tiptapJsonToHtml(fixture, { staticDocument: true }));
  document.documentElement.dataset.displayPalette = 'night';
  ok(dayHtml === c.sanitizeBodyHtml(tiptap.tiptapJsonToHtml(fixture, { staticDocument: true })), 'LOG does not depend on theme');
  const attack = mount(c.sanitizeBodyHtml('<span onclick="bad()" class="tiptap-spoiler fx-shake log-node" style="position:fixed;inset:0;z-index:999;animation:shake 1s;transition:all 1s;opacity:0;filter:blur(4px);background-image:url(https://evil.test/x);color:var(--secret);font-size:calc(1px + 99vh);--ruby-color:url(https://evil.test);--tiptap-decoration-thickness:99em">safe</span><img src="javascript:alert(1)" onerror="bad()"><a href="javascript:bad()">link</a><script>bad()</script>'));
  ok(!attack.querySelector('[style],[onclick],[onerror],[href],[src],script,.tiptap-spoiler,.log-node'), 'unsafe styles/URLs/events/classes stripped');
  for (const value of ['url(https://evil.test)', 'expression(alert(1))', 'var(--secret)', 'red!important', 'red/*x*/', 'r\\65 d']) {
    ok(!mount(c.sanitizeBodyHtml(`<span style="color:${value}">x</span>`)).querySelector('[style]'), `reject ${value}`);
  }
  ok(c.buildTemplateLibrary({ bad: '<div style="color:red" data-log-slot="body"></div>' }).issues.length > 0, 'template style still forbidden');
  const legacy = content.renderMessageBody('<strong style="color:red" onclick="bad()">旧文字</strong><at id="user" name="甲"/><img src="id:image-123"/>');
  ok(!/style=|onclick/.test(legacy.html) && legacy.html.includes('@甲') && legacy.html.includes('data-log-asset'), 'legacy compatibility without style passthrough');
  ok(content.renderMessageBody('**粗体** `.ra`').html.includes('<strong>粗体</strong>'), 'plain quick format');
  const action = { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'messageAction', attrs: { label: '动作标签', message: 'secret', button: true }, marks: [{ type: 'bold' }] }] }] };
  ok(content.renderMessageBody(JSON.stringify(action)).html === '<p><strong>动作标签</strong></p>', 'action label remains static');
  const commandOnly = { type: 'doc', content: [
    { type: 'paragraph', content: [{ type: 'performanceCommand', attrs: { command: 'delay', value: '2000' } }] },
    { type: 'paragraph', content: [{ type: 'text', text: '正文' }] },
  ] };
  ok(content.renderMessageBody(JSON.stringify(commandOnly)).html === '<p>正文</p>', 'command-only paragraphs do not leave blank lines');
  const codeMarks = { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'inline', marks: [
    { type: 'code' }, { type: 'textStyle', attrs: { fontFamily: 'serif', fontSize: '26px', color: '#000000' } },
  ] }] }] };
  const code = mount(c.sanitizeBodyHtml(content.renderMessageBody(JSON.stringify(codeMarks)).html)).querySelector('code span');
  ok(code && getComputedStyle(code).fontFamily === 'serif' && getComputedStyle(code).fontSize === '26px', 'explicit font wins inside inline code');
  ok(getComputedStyle(code).color === 'rgb(0, 0, 0)', 'night palette does not filter explicit black');
  const first = c.sanitizeBodyHtml(design.bodyHtml);
  const normalized = html => {
    const root = document.createElement('template'); root.innerHTML = html;
    root.content.querySelectorAll('[style]').forEach(el => {
      const properties = Array.from(el.style).sort().map(name => `${name}:${el.style.getPropertyValue(name).trim()}`);
      el.setAttribute('style', properties.join(';'));
    });
    return root.content;
  };
  ok(normalized(c.sanitizeBodyHtml(first)).isEqualNode(normalized(first)), 'sanitizer idempotence');
  // Exercise actual iframe measurement/splitting with mixed local sizes and Ruby.
  const hooks = window.logRichRuntime;
  const layout = async (revision, size) => {
    const repeated = { ...design, bodyHtml: content.renderMessageBody(JSON.stringify({ type: 'doc', content: [{ type: 'paragraph', content: [
      { type: 'text', text: '分页回归文字。'.repeat(180), marks: [{ type: 'textStyle', attrs: { fontSize: `${size}px` } }] },
      ...fixture.content[0].content.slice(3, 4),
    ] }] })).html, assets: [] };
    const layoutDoc = build(repeated);
    let result;
    hooks.setPort({ postMessage: message => { if (message.type === 'layout') result = message; } });
    await hooks.render({ revision, mode: 'paged', doc: layoutDoc, selection: [] });
    ok(!!result && result.pageCount > 0, `layout revision ${revision}`);
    ok(document.querySelectorAll('#log-root .log-node').length >= result.pageCount, 'fragments retained');
    return result.pageCount;
  };
  const small = await layout(1, 12);
  const large = await layout(2, 28);
  ok(large > small, 'larger local size increases page count');
  return { passed: true, checks, pages: { small, large } };
}

const runtime = fs.readFileSync(path.join(src, 'services/log/pagination/logFrameRuntime.js'), 'utf8')
  .replace(/\}\)\(\);\s*$/, 'window.logRichRuntime = { render, setPort: value => { port = value; } };})();');
const css = fs.readFileSync(path.join(src, 'services/log/pagination/logFrame.css'), 'utf8');
const script = `const modules=[${modules.join(',\n')}], cache={}; const load=id=>{if(!cache[id]){const module=cache[id]={exports:{}};modules[id](module,module.exports,load);}return cache[id].exports;}; (${check.toString()})(load(${entry}),load(${compiler}),load(${content}),load(${tiptap}),${JSON.stringify(fixture)}).then(result=>document.getElementById('result').textContent=JSON.stringify(result)).catch(error=>document.getElementById('result').textContent=JSON.stringify({passed:false,error:error.stack}));`;
const html = `<!doctype html><meta charset="utf-8"><style>${css}</style><main id="log-root"></main><style id="log-package"></style><style id="log-page-rule"></style><script id="result" type="application/json"></script><script>${runtime.replace(/<\/script/gi, '<\\/script')}</script><script>${script.replace(/<\/script/gi, '<\\/script')}</script>`;
const server = http.createServer((_req, res) => { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end(html); });
server.listen(0, '127.0.0.1', async () => {
  const url = `http://127.0.0.1:${server.address().port}/`;
  if (process.argv.includes('--serve')) return console.log(url);
  try {
    const chrome = process.env.CHROME_BIN || ['/usr/bin/google-chrome', '/usr/bin/chromium'].find(fs.existsSync);
    assert.ok(chrome, 'Set CHROME_BIN, or use --serve with an existing browser');
    const { stdout } = await promisify(execFile)(chrome, ['--headless=new', '--disable-gpu', '--disable-background-networking', '--virtual-time-budget=30000', '--dump-dom', url], { timeout: 60000, maxBuffer: 4 * 1024 * 1024 });
    const report = JSON.parse(stdout.match(/<script id="result" type="application\/json">([\s\S]*?)<\/script>/)?.[1] || '{}');
    assert.equal(report.passed, true, JSON.stringify(report));
    console.log(report);
  } catch (error) { console.error(error); process.exitCode = 1; }
  finally { server.close(); }
});
