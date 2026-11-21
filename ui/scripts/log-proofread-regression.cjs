// node --test scripts/log-proofread-regression.cjs
// LOG 批量校对：文本段抽取、保护区、规则分级、写回与批量撤销，执行真实源码，不依赖浏览器或后端。
// 已安装 zhlint 时用 Node 入口执行真实规则；浏览器 chunk 由 Vite 构建验证。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const read = file => fs.readFileSync(path.join(__dirname, '../src', file), 'utf8');

function loadModule(file, modules = {}) {
  const output = ts.transpileModule(read(file), { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText;
  const module = { exports: {} };
  vm.runInThisContext(`(function (module, exports, require) {${output}\n})`)(module, module.exports, id => {
    if (id in modules) return modules[id];
    throw new Error(`unexpected import ${id}`);
  });
  return module.exports;
}

const diff = loadModule('services/ai/ai-polish-diff.ts');
const proof = loadModule('services/log/proofread/logProofread.ts', { '@/services/ai/ai-polish-diff': diff });
const batch = loadModule('views/log/logProofreadBatch.ts', { '@/services/log/proofread/logProofread': proof });

// 近似 zhlint 在本模块配置下的行为：非连续的半角 , ; : ? ! 转全角（两侧都是半角字母时跳过），去掉全角标点旁空格。
const FULL = { ',': '，', ';': '；', ':': '：', '?': '？', '!': '！' };
const stubLint = text => text
  .replace(/[,;:?!]/g, (ch, at, s) => {
    const prev = s[at - 1] || '';
    const next = s[at + 1] || '';
    if (/[,;:?!.]/.test(prev) || /[,;:?!.]/.test(next)) return ch;
    if (/[A-Za-z0-9]/.test(prev) && /[A-Za-z0-9]/.test(next)) return ch;
    return FULL[ch];
  })
  .replace(/ +(?=[，；：？！。])/g, '')
  .replace(/([，；：？！。]) +/g, '$1');

const hasZhlint = fs.existsSync(path.join(__dirname, '../node_modules/zhlint/package.json'));
let realLint = null;
const loadReal = async () => {
  if (realLint || !hasZhlint) return realLint;
  // Node 使用包主入口：浏览器 dist 依赖 document，Windows 绝对路径也不能直接用于 import()。
  const { run } = await import('zhlint');
  const runner = loadModule('services/log/proofread/zhlintRunner.ts', {});
  realLint = text => run(text, { rules: { ...runner.ZHLINT_RULES, skipAbbrs: [...runner.ZHLINT_RULES.skipAbbrs] }, logger: { log() {}, warn() {}, error() {} } }).result;
  return realLint;
};

const scan = (content, lint = stubLint) => proof.proofreadContent(content, lint);
const applyAll = (content, issues) => proof.applyProofEdits(content, issues);
const fixSafe = (content, lint = stubLint) => applyAll(content, scan(content, lint).filter(i => i.level === 'safe'));
const tiptap = (...paragraphs) => JSON.stringify({ type: 'doc', content: paragraphs.map(content => ({ type: 'paragraph', content })) });
const text = (value, marks) => (marks ? { type: 'text', text: value, marks } : { type: 'text', text: value });

for (const [name, getLint] of [['stub', async () => stubLint], ['zhlint', loadReal]]) {
  test(`[${name}] 中文语境的半角标点安全修正为全角，英文与数字不动`, { skip: name === 'zhlint' && !hasZhlint && 'zhlint 未安装' }, async () => {
    const lint = await getLint();
    assert.equal(fixSafe('你好,我是调查员.今天天气不错?', lint), '你好，我是调查员.今天天气不错？');
    assert.equal(fixSafe('Hello, world! 1,000 HP:10', lint), 'Hello, world! 1,000 HP:10');
    assert.equal(fixSafe('他说 , 好的', lint), '他说，好的');
    const period = scan('我知道了.走吧', lint).find(i => i.original === '.');
    assert.equal(period?.level, 'suggest', '单个中文后句点只提示');
  });

  test(`[${name}] 连续省略号、停顿与语气标点保留`, { skip: name === 'zhlint' && !hasZhlint && 'zhlint 未安装' }, async () => {
    const lint = await getLint();
    for (const value of ['我……我不知道...', '嗯。。。好吧', '什么?!', '真的吗???', '啊!!!', '等等......你说什么', '呃 ... 那个']) {
      const fixed = fixSafe(value, lint);
      assert.equal(fixed, value, `不应自动修改：${value}`);
    }
    assert.equal(scan('等等......你说什么', lint).length, 0);
  });
}

test('真实 zhlint 的结尾文本不会重复（回归 hyperParse 空数组问题）', { skip: !hasZhlint && 'zhlint 未安装' }, async () => {
  const lint = await loadReal();
  assert.equal(lint('你好'), '你好');
  assert.equal(lint('你好,世界!'), '你好，世界！');
  assert.equal(lint('Hello'), 'Hello');
  assert.equal(lint('短语测试。'), '短语测试。');
});

test('跑团补充规则：重复与混用标点安全合并，“。.”只提示', () => {
  assert.equal(fixSafe('好，，我们走', null), '好，我们走');
  assert.equal(fixSafe('好，,我们走', null), '好，我们走');
  const issues = scan('结束了。.', null);
  assert.deepEqual(issues.map(i => [i.rule, i.level, i.replacement]), [['mixed', 'suggest', '。']]);
});

test('CQ 码、骰子指令、URL、代码与实体转义受保护', () => {
  const samples = [
    '[CQ:at,qq=123,name=a,b]你好',
    '.r 1d100,侦查',
    '。ra 侦查,60',
    '投掷{1d20+3,优势}看看',
    '伤害 2d6+3,好痛',
    '链接 https://example.com/a?b=1,c=2 看看',
    '代码 `a,b:c` 结束',
    '```\n一,二\n```',
    '转义&amp;,&lt;',
  ];
  for (const value of samples) {
    const fixed = fixSafe(value);
    for (const keep of ['[CQ:at,qq=123,name=a,b]', '.r 1d100,侦查', '。ra 侦查,60', '{1d20+3,优势}', '2d6+3', 'https://example.com/a?b=1,c=2', '`a,b:c`', '```\n一,二\n```', '&amp;', '&lt;']) {
      if (value.includes(keep)) assert.ok(fixed.includes(keep), `${keep} 被修改：${fixed}`);
    }
  }
  assert.equal(fixSafe('伤害 2d6+3,好痛'), '伤害 2d6+3,好痛', '保护区紧邻的标点不确定，只提示');
  assert.equal(fixSafe('[CQ:face,id=1]好的,走吧'), '[CQ:face,id=1]好的，走吧');
});

test('Markdown 与快捷格式：链接地址与代码不变，正文标点修正', () => {
  const value = '看[这里,那里](https://a.example/x,y)吧,**注意,安全**';
  const fixed = fixSafe(value);
  assert.ok(fixed.includes('](https://a.example/x,y)'));
  assert.ok(fixed.includes('**注意，安全**'));
  assert.ok(fixed.includes('吧，'));
});

test('旧格式富文本：只改标签之间的文本，标签、属性、code/pre 与骰子结果原样保留', () => {
  const value = '<p class="a,b">你好,世界</p><code>x,y</code><span class="dice-chip" data-dice-source="1d6,2">结果,3</span><img src="id:abc,1"/><at id="u1" name="甲,乙"/>好的,走';
  const fixed = fixSafe(value);
  assert.equal(fixed, '<p class="a,b">你好，世界</p><code>x,y</code><span class="dice-chip" data-dice-source="1d6,2">结果,3</span><img src="id:abc,1"/><at id="u1" name="甲,乙"/>好的，走');
});

test('TipTap 富文本：只改文本节点，标记、提及、代码与换行结构不变；跨标记的修改被放弃', () => {
  const value = tiptap(
    [text('你好,'), text('朋友', [{ type: 'bold' }]), text('!'), { type: 'hardBreak' }, text('代码,内容', [{ type: 'code' }])],
    [{ type: 'mention', attrs: { id: 'u', label: '甲,乙' } }, text('来吧,走')],
  );
  const issues = scan(value);
  const fixed = JSON.parse(fixSafe(value));
  const [p1, p2] = fixed.content;
  assert.equal(p1.content[0].text, '你好，');
  assert.deepEqual(p1.content[1], { type: 'text', text: '朋友', marks: [{ type: 'bold' }] });
  assert.equal(p1.content[2].text, '！');
  assert.equal(p1.content[3].type, 'hardBreak');
  assert.equal(p1.content[4].text, '代码,内容');
  assert.deepEqual(p2.content[0], { type: 'mention', attrs: { id: 'u', label: '甲,乙' } });
  assert.equal(p2.content[1].text, '来吧，走');
  assert.ok(issues.every(i => i.end > i.start), '没有空修正');
  const codeBlock = JSON.stringify({ type: 'doc', content: [{ type: 'codeBlock', content: [text('a,b,中,文')] }] });
  assert.equal(scan(codeBlock).length, 0);
});

test('空消息、空白消息与无问题消息不产生建议；重复扫描结果一致', () => {
  assert.deepEqual(scan(''), []);
  assert.deepEqual(scan('   \n '), []);
  assert.deepEqual(scan(tiptap([])), []);
  assert.deepEqual(scan('一切正常。'), []);
  const value = '你好,世界!走吧,';
  assert.deepEqual(scan(value), scan(value));
});

test('写回检查原文：正文在扫描后变化时拒绝应用', () => {
  const issues = scan('你好,世界');
  assert.equal(applyAll('你好,世界', issues), '你好，世界');
  assert.equal(applyAll('你好;世界', issues), null);
  assert.equal(applyAll('', issues), null);
});

test('批量计划与撤销：只写工程覆盖，跳过已变化消息，撤销仅还原仍是本次结果的消息', () => {
  const results = [
    batch.toProofResult({ messageId: 'm1', sectionId: 's', speaker: '甲', createdAt: 1, source: '你好,世界', original: '你好,世界' }, scan('你好,世界')),
    batch.toProofResult({ messageId: 'm2', sectionId: 's', speaker: '乙', createdAt: 2, source: '走吧,快', original: '走吧,快' }, scan('走吧,快')),
    batch.toProofResult({ messageId: 'm3', sectionId: 's', speaker: '乙', createdAt: 3, source: '好的,', original: '好的,' }, scan('好的,')),
  ];
  assert.equal(results.length, 3);
  const decisions = { [`m1:${results[0].issues[0].id}`]: 'accepted', [`m2:${results[1].issues[0].id}`]: 'accepted', [`m3:${results[2].issues[0].id}`]: 'rejected' };
  const overrides = { m2: '已被别处改过' };
  const plan = batch.planProofApply(results, decisions, id => overrides[id], () => undefined);
  assert.deepEqual(plan.writes, { m1: '你好，世界' });
  assert.deepEqual(plan.stale, ['m2']);
  const draft = { messageContentOverrides: { m2: '已被别处改过', keep: '其他覆盖' } };
  const record = batch.commitProofApply(draft, plan.writes);
  assert.deepEqual(draft.messageContentOverrides, { m2: '已被别处改过', keep: '其他覆盖', m1: '你好，世界' });
  assert.deepEqual(record, { m1: { before: undefined, after: '你好，世界' } });
  draft.messageContentOverrides.keep = '用户后来改的';
  const undo = batch.undoProofApply(draft, record);
  assert.equal(undo.restored, 1);
  assert.deepEqual(draft.messageContentOverrides, { m2: '已被别处改过', keep: '用户后来改的' });
  const second = batch.commitProofApply(draft, { m1: 'x' });
  draft.messageContentOverrides.m1 = '撤销前被手动修改';
  assert.deepEqual(batch.undoProofApply(draft, second), { restored: 0, skipped: 1 });
  assert.equal(draft.messageContentOverrides.m1, '撤销前被手动修改');
  delete draft.messageContentOverrides.m1;
  delete draft.messageContentOverrides.m2;
  delete draft.messageContentOverrides.keep;
  batch.commitProofApply(draft, {});
  assert.equal(draft.messageContentOverrides, undefined, '空覆盖表不写入草稿');
});

test('应用结果与原文一致时移除覆盖，而不是保存一份相同的正文', () => {
  const draft = { messageContentOverrides: { m1: '旧覆盖' } };
  const record = batch.commitProofApply(draft, { m1: '原文' }, id => (id === 'm1' ? '原文' : undefined));
  assert.equal(draft.messageContentOverrides, undefined);
  assert.deepEqual(record, { m1: { before: '旧覆盖', after: undefined } });
});

test('保存前容量预检：单条 64 KiB 与草稿总计 2 MiB 按 UTF-8 字节数判定', () => {
  const draft = { title: '常规', messageContentOverrides: { m1: '你好，世界' } };
  assert.equal(batch.proofDraftSizeError(draft), null);
  draft.messageContentOverrides.m1 = '中'.repeat(22000);
  assert.match(batch.proofDraftSizeError(draft), /64 KiB/);
  draft.messageContentOverrides.m1 = '短正文';
  draft.title = 'a'.repeat(2 * 1024 * 1024);
  assert.match(batch.proofDraftSizeError(draft), /2 MiB/);
});

test('跨章节校对：保存扫描来源版本，并在应用前重新检查所有选中片段', () => {
  const panel = read('views/log/components/LogProofreadPanel.vue');
  const workbench = read('views/log/components/LogWorkbench.vue');
  assert.match(workbench, /:window-source-version="win\.sourceVersion\.value"/);
  assert.match(panel, /sourceVersions\[sectionId\] = version/);
  assert.match(panel, /fetchLogSectionMessages\(projectId, sectionId, null, 1, ctrl\.signal\)/);
  assert.match(panel, /scannedSourceVersions\.value\[sectionId\] === page\.sourceVersion/);
  assert.match(panel, /fetchLogMessageContext\(projectId, sectionId, result\.messageId, ctrl\.signal\)/);
  assert.match(panel, /message\.content !== result\.original/);
  assert.match(panel, /current\.filter\(\(r\) => !changedMessages\.has\(r\.messageId\)\)/);
  assert.match(panel, /const saved = await props\.flush\(\)/);
  assert.match(panel, /decisionOf\(r, i\) === 'accepted'/);
});

test('频道中其他消息更新不等于校对目标过期：原文未变化仍可应用', () => {
  const original = '你好,世界';
  const r = batch.toProofResult({ messageId: 'm1', sectionId: 's1', speaker: '甲', createdAt: 1, original, source: original }, scan(original));
  const decisions = { [batch.proofDecisionKey(r.messageId, r.issues[0].id)]: 'accepted' };
  const unchanged = batch.planProofApply([r], decisions, () => undefined, () => original);
  assert.deepEqual(unchanged.writes, { m1: '你好，世界' });
  assert.deepEqual(unchanged.stale, []);
  const edited = batch.planProofApply([r], decisions, () => undefined, () => '你好;世界');
  assert.deepEqual(edited.stale, ['m1']);
});

test('审阅导航：未加载消息逐页追加、跨章节等待布局与批注映射', () => {
  const wb = read('views/log/components/LogWorkbench.vue');
  const win = read('views/log/composables/useLogMessageWindow.ts');
  assert.match(win, /const loadedSectionId = ref\(''\)/);
  assert.match(win, /loadedSectionId\.value = sec\.id/);
  assert.match(win, /loadedSectionId\.value = ''/);
  assert.match(wb, /const pendingProofReveal = shallowRef/);
  assert.match(wb, /if \(win\.loadedSectionId\.value !== target\.sectionId \|\| win\.loading\.value\) return/);
  assert.match(wb, /if \(win\.nextCursor\.value\) \{ void win\.loadMore\(\); return; \}/);
  assert.match(wb, /if \(!proofMarks\.value\?\.some/);
  assert.match(wb, /case 'proofMapped':/);
});

test('撤销保存状态、快捷键输入保护及筛选计数', () => {
  const panel = read('views/log/components/LogProofreadPanel.vue');
  const runtime = read('services/log/pagination/logFrameRuntime.js');
  assert.match(panel, /const undoLast = async \(\) =>/);
  assert.match(panel, /const saved = await props\.flush\(\)\.catch\(\(\) => false\)/);
  assert.match(panel, /但仅更新本地草稿，保存失败/);
  assert.match(panel, /index: at >= 0 \? at \+ 1 : 0, total: list\.length/);
  assert.match(panel, /replace\(\/\\u00a0\/g, '⍽'\)/);
  assert.match(runtime, /keyTarget = e\.target && e\.target\.closest/);
  assert.match(runtime, /input, textarea, select, button, a\[href\], \[contenteditable\]/);
});

test('版本能力保护：旧后端只能预览校对结果，不得提交正文覆盖', () => {
  const panel = read('views/log/components/LogProofreadPanel.vue');
  const workbench = read('views/log/components/LogWorkbench.vue');
  assert.match(panel, /!props\.supportsContentOverrides \|\| busy\.value/);
  assert.match(panel, /:disabled="!canEditDraft \|\| !supportsContentOverrides \|\| !acceptedCount \|\| busy"/);
  assert.match(workbench, /:supports-content-overrides="caps\.supportsContentOverrides === true"/);
  assert.match(workbench, /delete next\.messageContentOverrides/);
});

test('原文审阅层：批注经隔离预览协议传入，决定与导航回到审阅中心的同一状态', () => {
  const workbench = read('views/log/components/LogWorkbench.vue');
  const panel = read('views/log/components/LogProofreadPanel.vue');
  const preview = read('views/log/components/LogPreviewFrame.vue');
  const runtime = read('services/log/pagination/logFrameRuntime.js');
  const channel = read('services/log/pagination/logFrameChannel.ts');
  assert.match(workbench, /@marks="\(nodes: FrameProofNode\[\] \| null\) => \(proofMarks = nodes\)"/);
  assert.match(workbench, /:proof-marks="previewPublish \? null : proofMarks"/, '发布效果预览不显示审阅层');
  assert.match(workbench, /proofreadPanelRef\.value\?\.onFrameEvent\(event\)/);
  assert.match(preview, /engine\.value\?\.proofMarks\(toRaw\(nodes\) \?\? null\)/);
  assert.match(channel, /type: 'proofMarks', nodes/);
  assert.match(channel, /case 'proofDecide':/);
  for (const type of ['proofPick', 'proofDecide', 'proofNav', 'proofMapped']) assert.match(runtime, new RegExp(`type: '${type}'`));
  // 文字标记只用 Custom Highlight，批注层挂在排版根节点之外；打印时暂停。
  assert.match(runtime, /HIGHLIGHTS\.set\('log-proof-' \+ k/);
  assert.match(runtime, /document\.body\.appendChild\(layer\)/);
  assert.match(runtime, /suspendProof\(true\);/);
  assert.doesNotMatch(runtime, /root\.appendChild\(layer\)/);
  assert.match(panel, /defineExpose\(\{ onFrameEvent, handleKey, escape \}\)/);
  assert.match(panel, /decideActive\(event\.decision, true\)/);
});

test('审阅中心的批注映射只在按需加载的模块中进行', () => {
  const panel = read('views/log/components/LogProofreadPanel.vue');
  const marks = read('services/log/proofread/logProofMarks.ts');
  const runtime = read('services/log/pagination/logFrameRuntime.js');
  assert.match(panel, /import \{ buildProofNode \} from '@\/services\/log\/proofread\/logProofMarks'/);
  // 外层与 iframe 计算正文位置时跳过同一组元素。
  const outer = /PROOF_TEXT_SKIP = '([^']+)'/.exec(marks)[1];
  const inner = /PROOF_SKIP = '([^']+)'/.exec(runtime)[1];
  assert.equal(outer, inner);
});

test('保存失败时显示真实错误，章节和工程扫描先保存完整来源范围', () => {
  const draft = read('views/log/composables/useLogDraft.ts');
  const panel = read('views/log/components/LogProofreadPanel.vue');
  const workbench = read('views/log/components/LogWorkbench.vue');
  assert.match(draft, /throw new Error\(lastError\.value \|\|/);
  assert.match(panel, /if \(scope\.value !== 'selected' && !\(await props\.flush\(\)\)\)/);
  assert.doesNotMatch(panel, /props\.windowComplete/);
  assert.match(panel, /props\.saveError \|\|/);
  assert.match(workbench, /:save-error="draftCtl\.lastError\.value"/);
});

test('懒加载：排版器不静态引入校对面板或 zhlint；zhlint 只加载一次，失败后可重试', async () => {
  const workbench = read('views/log/components/LogWorkbench.vue');
  assert.match(workbench, /defineAsyncComponent\(\(\) => import\('\.\/LogProofreadPanel\.vue'\)\)/);
  assert.doesNotMatch(workbench, /^import (?!type\b).*(LogProofreadPanel|zhlint|logProofread)/m);
  assert.doesNotMatch(workbench, /^import (?!type\b).*(LogProofReviewBar|logProofMarks)/m);
  for (const file of ['views/log/components/LogProofreadPanel.vue', 'views/log/components/LogProofReviewBar.vue', 'services/log/proofread/logProofread.ts', 'services/log/proofread/logProofMarks.ts', 'views/log/logProofreadBatch.ts', 'services/log/logRenderer.ts']) {
    assert.doesNotMatch(read(file), /^import .*['"]zhlint/m, `${file} 静态引入了 zhlint`);
  }
  let loads = 0;
  let fail = true;
  const fake = { run: (text, options) => {
    assert.equal(options.hyperParse, undefined, '空 hyperParse 会让 zhlint 重复末尾字符');
    return { origin: text, result: `${text}|${options.rules.fullwidthPunctuation}`, validations: [] };
  } };
  const runner = loadModule('services/log/proofread/zhlintRunner.ts', new Proxy({}, {
    has: (_, id) => id === 'zhlint/dist/zhlint.es.js',
    get: (_, id) => {
      if (id !== 'zhlint/dist/zhlint.es.js') return undefined;
      loads++;
      if (fail) throw new Error('chunk load failed');
      return fake;
    },
  }));
  await assert.rejects(runner.loadZhlintLint());
  fail = false;
  const [a, b] = await Promise.all([runner.loadZhlintLint(), runner.loadZhlintLint()]);
  assert.equal(a, b);
  assert.equal(loads, 2, '失败一次后重试一次，并发调用共享同一实例');
  assert.equal(a('好'), '好|，：；？！', '规则不含句号与引号');
  assert.ok(!runner.ZHLINT_RULES.fullwidthPunctuation.includes('。'));
  assert.equal(runner.ZHLINT_RULES.preset, undefined, '不启用 zhlint 默认 preset');
});
