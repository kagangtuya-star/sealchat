// node scripts/log-proof-review-regression.cjs [--serve]
// LOG 校对审阅层：在真实浏览器中执行正文渲染、哨兵位置映射、分页运行时与 Custom Highlight 审阅层。
// 与 log-static-rich-regression 相同，TypeScript 只做转译打包，运行时直接加载在顶层文档中检查内部状态。
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { execFile } = require('node:child_process');
const { promisify } = require('node:util');
const assert = require('node:assert/strict');
const ts = require('typescript');
const ui = path.join(__dirname, '..');
const src = path.join(ui, 'src');
const modules = [];
const ids = new Map();
require.extensions['.ts'] ||= () => { throw Error('TypeScript is bundled, not required'); };
const stubs = {
  '@/stores/_config': 'exports.urlBase = "";',
  '@/composables/useAttachmentResolver': 'exports.normalizeAttachmentId = value => value.replace(/^id:/, "").replace(/^\\/api\\/v1\\/attachment\\//, "");',
  'lodash-es': 'exports.memoize = fn => fn;',
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
const renderer = bundle(path.join(src, 'services/log/logRenderer.ts'));
const proof = bundle(path.join(src, 'services/log/proofread/logProofread.ts'));
const marks = bundle(path.join(src, 'services/log/proofread/logProofMarks.ts'));
const compiler = bundle(path.join(src, 'services/log/logTemplateCompiler.ts'));

async function check(r, p, m, c) {
  const checks = [];
  const ok = (value, name) => { checks.push(name); if (!value) throw new Error(name); };
  const hooks = window.logProofRuntime;
  const posted = [];
  hooks.setPort({ postMessage: (message) => posted.push(message) });
  const page = { preset: 'a5', widthMm: 148, heightMm: 120, marginsMm: { top: 12, right: 12, bottom: 12, left: 12 }, gutterMm: 6,
    columns: 1, backgroundColor: '#fff', backgroundAssetId: null, fontFamily: 'sans-serif', fontSizePt: 11, lineHeight: 1.6,
    headerTemplateId: null, footerTemplateId: 'foot', showPageNumbers: true, mergeNeighbors: false };
  const style = {
    name: 'test',
    templates: {
      message: '<article class="msg"><header class="who" data-log-text="speaker.name"></header><section class="wrap"><div class="inner" data-log-slot></div></section></article>',
      foot: '<span data-log-text="pageNumber"></span>',
    },
    css: '.msg .who { font-weight: 700 } .msg .inner { color: #222 }',
    rules: [],
    tokens: {},
    settingsSchema: {},
  };
  const text = (value, marks) => (marks ? { type: 'text', text: value, marks } : { type: 'text', text: value });
  const tiptap = (...paragraphs) => JSON.stringify({ type: 'doc', content: paragraphs.map((content) => ({ type: 'paragraph', content })) });
  const samples = {
    plain: '你好,,世界，,走吧。.',
    markdown: '**注意，，安全**吧，，好 *斜体，，字*',
    tiptap: tiptap(
      [text('开头，，'), text('粗体，，', [{ type: 'bold' }]), text('漢字', [{ type: 'ruby', attrs: { rubyText: 'かんじ' } }]), text('后面，，'),
        { type: 'mention', attrs: { id: 'u1', label: '甲，，乙' } }, text('再见，，'), { type: 'hardBreak' }, text('代码，，', [{ type: 'code' }]), text('结尾，，')],
      [text('第二段，，'), text('链接，，', [{ type: 'link', attrs: { href: 'https://example.com' } }]), text('完')],
    ),
    legacy: '<p class="x">你好，，世界</p><span class="dice-chip" data-dice-source="1d6">结果，，3</span><img src="id:abcd1234"/>好的，，走<br/>换行，，了',
    cq: '[CQ:face,id=1]好的，，走吧[CQ:at,qq=1]，，',
    dice: '投掷{1d20+3}看看，，好 .r 1d100',
    entity: '你好&nbsp;，，世界&amp;朋友，，',
    pua: '\uE001你好，，世界',
  };
  const items = (entries) => entries.map(([id, content], i) => ({
    message: { id, content, createdAt: 1700000000000 + i * 1000, identity: { id: `i${i % 3}`, displayName: `角色${i % 3}` }, user: { id: 'u' } },
    version: 1,
    capabilities: { reorder: false, edit: true },
  }));
  const keyOf = (id) => (issue) => `${id}:${issue.id}`;

  // 1. 哨兵映射：映射到的区间在正常渲染的可见文本中恰好是原文；无法确定时不给位置。
  for (const [name, content] of Object.entries(samples)) {
    const issues = p.proofreadContent(content, null);
    const map = m.mapProofIssues(content, issues);
    const rendered = m.proofBodyText(c.sanitizeBodyHtml(r.designMessageToNode(items([['x', content]])[0]).bodyHtml));
    ok(map.length === rendered.length, `${name}: 正文长度一致`);
    map.ranges.forEach((range, k) => {
      if (range) ok(rendered.slice(range[0], range[1]) === issues[k].original, `${name}: 第 ${k} 处区间文本等于原文`);
    });
    if (name === 'pua') ok(map.ranges.every((range) => range === null), 'pua: 正文自带私用区字符时整条降级，不给出可能错误的位置');
    else ok(map.ranges.filter(Boolean).length === issues.length, `${name}: ${issues.length} 处全部可定位`);
  }
  const md = p.proofreadContent(samples.markdown, null);
  const mdMap = m.mapProofIssues(samples.markdown, md);
  ok(mdMap.ranges[0][0] === 2, 'markdown: 去掉 ** 标记后位置前移');

  // 2. 分页运行时：多页、跨分片、模板嵌套槽位、合并与校验。
  const entries = [];
  for (let i = 0; i < 60; i++) {
    const long = i % 7 === 3;
    const body = long ? `长消息开始，，${'这是一段用于跨页的很长的正文内容，'.repeat(28)}中段，，${'继续写下去直到换页，'.repeat(24)}末尾，，` : `第 ${i} 条，，内容,,还有。.`;
    entries.push([`m${i}`, body]);
  }
  entries.push(['rich', samples.tiptap], ['legacy', samples.legacy], ['md', samples.markdown], ['pua', samples.pua]);
  const list = items(entries);
  const comp = r.composeDesignSection('s1', '第一章', list, [], true, new Map(), {});
  const doc = r.buildFrameDocument({ style, page, title: 'T', sectionTitle: '第一章', nodes: comp.nodes, toc: comp.toc, backgroundToken: null, templateSelection: {} }).doc;
  await hooks.render({ revision: 1, mode: 'paged', doc, design: true, selection: [] });
  const pages = document.querySelectorAll('#log-root .log-page').length;
  ok(pages > 5, `生成多页（${pages} 页）`);
  const before = document.querySelector('#log-root').innerHTML;

  const started = performance.now();
  const nodes = list.map((item) => {
    const issues = p.proofreadContent(item.message.content, null);
    return m.buildProofNode(item.message.id, item.message.content, issues, keyOf(item.message.id), null);
  }).filter((n) => n.items.length);
  const outerMs = performance.now() - started;
  const total = nodes.reduce((n, node) => n + node.items.length, 0);
  posted.length = 0;
  const applyStart = performance.now();
  hooks.setProofMarks(nodes);
  const applyMs = performance.now() - applyStart;
  const mapped = posted.find((msg) => msg.type === 'proofMapped');
  ok(!!mapped, '运行时回报映射结果');
  const expectedMissed = nodes.filter((n) => n.nodeId === 'pua').flatMap((n) => n.items.map((it) => it.key));
  ok(nodes.find((n) => n.nodeId === 'pua').items.every((it) => it.start === -1 && it.note), '私用区消息在外层降级为消息级批注');
  ok(mapped.missed.length === 0, `其余批注在排版中全部核对通过（missed ${mapped.missed.length}）`);
  const state = hooks.proof;
  ok(state.marks.size === total, `全部 ${total} 处批注登记`);
  let spanned = 0;
  state.marks.forEach((mark) => {
    if (!mark.ranges) return;
    ok(mark.ranges.map(String).join('') === mark.item.original, `${mark.key} 排版后文本核对一致`);
    const frag = mark.ranges[0].startContainer.parentElement.closest('[data-log-node]');
    if (frag && frag.getAttribute('data-log-fragment') !== '0') spanned++;
  });
  ok(spanned > 0, `跨页消息续页中的批注也能定位（${spanned} 处）`);
  ok(document.querySelector('#log-root').innerHTML === before, '审阅层不改动排版 DOM');
  ok(!!(window.CSS && CSS.highlights && CSS.highlights.has('log-proof-suggest')), '使用 Custom Highlight 注册文字标记');

  // 3. 状态与卡片：当前批注、已采纳样式、点中批注。
  const first = nodes.find((n) => n.nodeId === 'm1').items[0];
  const second = nodes.find((n) => n.nodeId === 'm1').items[1];
  hooks.setProofView({ active: first.key, states: { [second.key]: 'accepted' }, position: { index: 1, total } });
  ok(CSS.highlights.get('log-proof-active').size === 1, '当前批注单独高亮');
  ok(CSS.highlights.get('log-proof-accepted').size === 1, '已采纳批注换用采纳样式');
  hooks.proofReveal(first.key);
  hooks.drawProof();
  const card = document.querySelector('.log-proof-layer .log-proof-note.is-active');
  ok(!!card && card.getAttribute('data-proof-key') === first.key, '页边栏出现当前批注卡片');
  ok(!!card.querySelector('[data-proof-act="accept"]') && !!card.querySelector('[data-proof-act="next"]'), '卡片带有采纳与导航操作');
  ok(card.textContent.includes('1 / ' + total), '卡片显示导航位置');
  const hitRect = state.marks.get(first.key).ranges[0].getBoundingClientRect();
  const target = document.elementFromPoint(hitRect.left + hitRect.width / 2, hitRect.top + hitRect.height / 2);
  ok(hooks.proofHit(hitRect.left + hitRect.width / 2, hitRect.top + hitRect.height / 2, target) === first.key, '点中原文标记即选中该批注');
  posted.length = 0;
  card.querySelector('[data-proof-act="reject"]').click();
  ok(posted.some((msg) => msg.type === 'proofDecide' && msg.key === first.key && msg.decision === 'rejected'), '卡片操作回报决定');

  // 4. 重排后重新映射；正文长度不一致时整条降级。
  await hooks.render({ revision: 2, mode: 'paged', doc, design: true, selection: [] });
  ok(hooks.proof.marks.size === total && [...hooks.proof.marks.values()].filter((x) => x.ranges).length === total - expectedMissed.length, '重排后重新映射');
  posted.length = 0;
  hooks.setProofMarks([{ ...nodes[0], length: nodes[0].length + 1 }]);
  ok(posted.find((msg) => msg.type === 'proofMapped').missed.length === nodes[0].items.length, '长度不一致时整条消息降级');

  // 5. 打印与关闭：不留下任何审阅痕迹。
  hooks.setProofMarks(nodes);
  hooks.suspendProof(true);
  ok(!CSS.highlights.has('log-proof-suggest') && document.querySelector('.log-proof-layer').style.display === 'none', '打印时移除标记并隐藏批注层');
  hooks.suspendProof(false);
  ok(CSS.highlights.has('log-proof-suggest'), '打印结束恢复标记');
  hooks.setProofMarks(null);
  ok(!document.querySelector('.log-proof-layer') && !CSS.highlights.has('log-proof-suggest') && !document.body.classList.contains('log-reviewing'), '关闭审阅层后清理干净');

  return { passed: true, checks: checks.length, pages, total, outerMs: Math.round(outerMs), applyMs: Math.round(applyMs) };
}

const runtime = fs.readFileSync(path.join(src, 'services/log/pagination/logFrameRuntime.js'), 'utf8')
  .replace(/\}\)\(\);\s*$/, 'window.logProofRuntime = { render, setPort: value => { port = value; }, setProofMarks, setProofView, proofReveal, drawProof, proofHit, suspendProof, proof };})();');
const css = fs.readFileSync(path.join(src, 'services/log/pagination/logFrame.css'), 'utf8');
const script = `const modules=[${modules.join(',\n')}], cache={}; const load=id=>{if(!cache[id]){const module=cache[id]={exports:{}};modules[id](module,module.exports,load);}return cache[id].exports;}; (${check.toString()})(load(${renderer}),load(${proof}),load(${marks}),load(${compiler})).then(result=>document.getElementById('result').textContent=JSON.stringify(result)).catch(error=>document.getElementById('result').textContent=JSON.stringify({passed:false,error:error.stack}));`;
const html = `<!doctype html><meta charset="utf-8"><style>${css}</style><body class="log-design"><main id="log-root"></main><style id="log-package"></style><style id="log-page-rule"></style><script id="result" type="application/json"></script><script>${runtime.replace(/<\/script/gi, '<\\/script')}</script><script>${script.replace(/<\/script/gi, '<\\/script')}</script></body>`;
const server = http.createServer((_req, res) => { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end(html); });
server.listen(0, '127.0.0.1', async () => {
  const url = `http://127.0.0.1:${server.address().port}/`;
  if (process.argv.includes('--serve')) return console.log(url);
  try {
    const chrome = process.env.CHROME_BIN || ['/usr/bin/google-chrome', '/usr/bin/chromium', 'C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(fs.existsSync);
    assert.ok(chrome, 'Set CHROME_BIN, or use --serve with an existing browser');
    const { stdout } = await promisify(execFile)(chrome, ['--headless=new', '--disable-gpu', '--disable-background-networking', '--window-size=1400,900', '--virtual-time-budget=30000', '--dump-dom', url], { timeout: 90000, maxBuffer: 8 * 1024 * 1024 });
    const report = JSON.parse(stdout.match(/<script id="result" type="application\/json">([\s\S]*?)<\/script>/)?.[1] || '{}');
    assert.equal(report.passed, true, JSON.stringify(report));
    console.log(report);
  } catch (error) { console.error(error); process.exitCode = 1; }
  finally { server.close(); }
});
