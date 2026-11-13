// node --test scripts/log-ai-proofread-regression.cjs
// 执行真实协议、调度器与面板脚本，用可控 AI 验证；不调用付费服务。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const vue = require('vue');
const read = (file) => fs.readFileSync(path.join(__dirname, '../src', file), 'utf8');
function load(file, modules = {}, source = read(file), args = {}) {
  const output = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText;
  const module = { exports: {} };
  vm.runInThisContext(`(function(module,exports,require,${Object.keys(args).join(',')}){${output}\n})`)(module, module.exports, (id) => {
    if (id in modules) return modules[id];
    throw new Error(`unexpected import ${id}`);
  }, ...Object.values(args));
  return module.exports;
}
const diff = load('services/ai/ai-polish-diff.ts');
const proof = load('services/log/proofread/logProofread.ts', { '@/services/ai/ai-polish-diff': diff });
const aiProof = load('services/log/proofread/logAIProofread.ts', { './logProofread': proof });
const batchOps = load('views/log/logProofreadBatch.ts', { '@/services/log/proofread/logProofread': proof });
const localAI = load('services/ai/local-ai.ts');
const meta = (source, id = 'm1', sectionId = 's1') => ({ source, original: source, messageId: id, sectionId, speaker: '甲', createdAt: 1 });
const prepared = (source) => {
  const message = meta(source);
  return { message, batch: aiProof.prepareAIProofBatches([message]).batches[0], snapshots: new Map([[message.messageId, message]]) };
};
const suggestion = (batch, changes = {}) => ({ fragmentId: batch.fragments[0].id, original: '以经', replacement: '已经', category: 'typo', reason: '客观错别字', ...changes });
const parse = (p, items) => aiProof.parseAIProofResponse(JSON.stringify({ suggestions: items }), p.batch, p.snapshots);

test('错别字、标点、轻度语病转换；ID 稳定且模型不能升级为 safe', () => {
  const p = prepared('我以经回来，走吧吧。');
  for (const category of ['typo', 'punctuation', 'grammar']) {
    const issue = parse(p, [suggestion(p.batch, { category })]).results[0].issues[0];
    assert.equal(issue.rule, `ai_${category}`);
    assert.equal(issue.level, 'suggest');
    assert.equal(issue.source, 'ai');
    assert.equal(proof.applyProofEdits(p.message.source, [issue]), '我已经回来，走吧吧。');
    assert.deepEqual(parse(p, [suggestion(p.batch, { category })]).results[0].issues[0], issue);
  }
  assert.equal(parse(p, [suggestion(p.batch, { confidence: 1 })]).results.length, 0);
});

test('严格 JSON、数量、长度、未知片段、非局部重写与结构注入', () => {
  const p = prepared('我以经回来。');
  for (const raw of ['```json\n{}\n```', '{bad', '{}', JSON.stringify({ suggestions: [], content: '重写' }), JSON.stringify({ suggestions: Array(101).fill({}) }), 'x'.repeat(65537)]) {
    assert.throws(() => aiProof.parseAIProofResponse(raw, p.batch, p.snapshots));
  }
  for (const changes of [
    { fragmentId: 'unknown' }, { original: '不存在' }, { original: '我以经回来。', replacement: '完全不同的精彩长篇故事剧情' },
    { replacement: '<img src=x>' }, { replacement: '[CQ:at,id=1]' }, { replacement: '**已经**' }, { replacement: '已经\n' },
    { replacement: 'a'.repeat(41) }, { replacement: '\uD800' }, { reason: '' }, { original: '' },
  ]) assert.equal(parse(p, [suggestion(p.batch, changes)]).results.length, 0, JSON.stringify(changes));
  const duplicate = parse(p, [suggestion(p.batch), suggestion(p.batch)]);
  assert.equal(duplicate.results[0].issues.length, 1);
  assert.equal(duplicate.discarded['重复建议'], 1);
});

test('相同文本多次出现须用原文上下文消歧；插入必须精确唯一', () => {
  const p = prepared('甲以经来，乙以经走');
  assert.equal(parse(p, [suggestion(p.batch)]).discarded['原文定位歧义'], 1);
  const issue = parse(p, [suggestion(p.batch, { before: '乙' })]).results[0].issues[0];
  assert.equal(proof.applyProofEdits(p.message.source, [issue]), '甲以经来，乙已经走');
  const q = prepared('你好世界');
  const insert = parse(q, [suggestion(q.batch, { original: '', replacement: '，', category: 'punctuation', before: '你好', after: '世界' })]).results[0].issues[0];
  assert.equal(insert.start, insert.end);
  assert.equal(proof.applyProofEdits(q.message.source, [insert]), '你好，世界');
  assert.equal(proof.applyProofEdits(q.message.source, [insert, { ...insert, replacement: '。' }]), null);
});

test('TipTap marks/attrs 与未知节点、HTML、Markdown、链接、CQ、指令、模板保护', () => {
  const rich = JSON.stringify({ type: 'doc', attrs: { a: 1 }, content: [{ type: 'paragraph', content: [
    { type: 'text', text: '我以经来了', marks: [{ type: 'bold' }] },
    { type: 'text', text: '链接以经', marks: [{ type: 'link', attrs: { href: 'https://example.com' } }] },
    { type: 'mention', attrs: { label: '以经' } },
  ] }, { type: 'customTemplate', content: [{ type: 'text', text: '模板以经' }] }] });
  const p = prepared(rich);
  assert.equal(p.batch.fragments.length, 1);
  const issue = parse(p, [suggestion(p.batch)]).results[0].issues[0];
  const after = JSON.parse(proof.applyProofEdits(rich, [issue]));
  const before = JSON.parse(rich);
  before.content[0].content[0].text = '我已经来了';
  assert.deepEqual(after, before);
  for (const source of [
    '.r 1d100 以经', '。ra 侦查 以经', '`以经`', '```\n以经<b>以经</b>\n```', '~~~\n以经\n~~~',
    '[CQ:at,name=以经]', '{以经1d20}', '![以经](id:abc)', '[以经](https://example.com)',
    '<code>以经</code><pre>以经</pre><span class="dice-chip">以经</span>',
    '<a href="https://example.com">以经</a><template><span>以经</span></template><iframe>以经</iframe>',
  ]) assert.equal(proof.editableProofFragments(source).length, 0, source);
  const mixed = '普通以经 [CQ:at,id=1] `秘密以经` **正文以经** https://example.com';
  const texts = proof.editableProofFragments(mixed).map((f) => f.text).join('|');
  assert.ok(texts.includes('普通以经'));
  assert.ok(texts.includes('正文以经'));
  assert.ok(!texts.includes('秘密') && !texts.includes('http') && !texts.includes('CQ'));
  const boundary = JSON.stringify({ type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: '以' }, { type: 'text', text: '经', marks: [{ type: 'bold' }] }] }] });
  assert.equal(proof.validAIProofEdit(boundary, { run: 0, start: 0, end: 2, original: '以经', replacement: '已经' }), false);
});

test('合并重复、本地 safe 不变；互斥修改必须人工选择，写回再次拒绝冲突', () => {
  const source = '我以经回来，，好';
  const p = prepared(source);
  const local = proof.proofreadContent(source, null);
  const additions = parse(p, [suggestion(p.batch), suggestion(p.batch, { original: '，，', replacement: '，', category: 'punctuation' }), suggestion(p.batch, { replacement: '早已' })]).results[0].issues;
  const merged = aiProof.mergeAIProofIssues(local, additions);
  assert.equal(merged.length, 3);
  assert.equal(merged.find((i) => i.level === 'safe').id, local[0].id);
  const r = { ...p.message, issues: merged };
  const keys = merged.map((i) => batchOps.proofDecisionKey(r.messageId, i.id));
  const all = batchOps.decideProofIssues([r], {}, keys, 'accepted');
  assert.equal(all.conflicts, 2);
  assert.equal(Object.keys(all.decisions).length, 1);
  const one = batchOps.decideProofIssues([r], all.decisions, [keys[0]], 'accepted');
  const other = batchOps.decideProofIssues([r], one.decisions, [keys[1]], 'accepted');
  assert.equal(other.conflicts, 1);
  assert.deepEqual(other.decisions, one.decisions);
  const plan = batchOps.planProofApply([r], one.decisions, () => undefined, () => source);
  const draft = {};
  const undo = batchOps.commitProofApply(draft, plan.writes);
  assert.equal(source, p.message.original);
  assert.ok(draft.messageContentOverrides.m1.includes('，好'));
  assert.deepEqual(batchOps.undoProofApply(draft, undo), { restored: 1, skipped: 0 });
  assert.equal(draft.messageContentOverrides, undefined);
  assert.deepEqual(batchOps.planProofApply([r], Object.fromEntries(keys.map((k) => [k, 'accepted'])), () => undefined, () => source).writes, {});
});

test('跨章节、大量消息、超长节点及 JSON 转义均受输入、数量与正文预算限制', () => {
  const messages = Array.from({ length: 505 }, (_, i) => meta('我以经来了' + '\\"'.repeat(100), `m${i}`, `s${i % 4}`));
  messages.push(meta('长'.repeat(12000) + '😀'.repeat(1000), 'long'));
  const prepared = aiProof.prepareAIProofBatches(messages, 1800);
  assert.equal(prepared.messageCount, 506);
  assert.ok(prepared.batches.length > 25);
  for (const b of prepared.batches) {
    assert.ok(b.input.length <= 1800);
    assert.ok(new Set(b.fragments.map((f) => f.messageId)).size <= 20);
    assert.ok(b.fragments.reduce((n, f) => n + f.text.length, 0) <= 3500);
    assert.ok(!b.fragments.some((f) => /[\uD800-\uDBFF]$/.test(f.text)));
    assert.deepEqual(Object.keys(JSON.parse(b.input).fragments[0]), ['id', 'messageId', 'sectionId', 'text']);
  }
  for (const budget of [100, 0, -100, NaN]) assert.throws(() => aiProof.prepareAIProofBatches(messages, budget));
});

test('失败与非法 JSON 独立重试，成功批次不重新调用；取消/过期阻止后续派发和结果', async () => {
  const make = () => [1, 2, 3].map((id) => ({ id, input: '{}', fragments: [], status: 'pending' }));
  const batches = make();
  const calls = [];
  const complete = [];
  const options = { batches, signal: new AbortController().signal, isCurrent: () => true, changed() {},
    request: async (b) => { calls.push(b.id); if (b.id === 2 && calls.length === 2) throw Error('网络失败'); return b.id === 3 && calls.length === 3 ? 'bad' : '{"suggestions":[]}'; },
    complete: async (b, raw) => { aiProof.parseAIProofResponse(raw, b, new Map()); complete.push(b.id); } };
  await aiProof.runAIProofBatches(options);
  assert.deepEqual(batches.map((b) => b.status), ['succeeded', 'failed', 'failed']);
  await aiProof.runAIProofBatches(options);
  assert.deepEqual(calls, [1, 2, 3, 2, 3]);
  assert.deepEqual(complete, [1, 2, 3]);
  for (const stale of [false, true]) {
    const pending = make(); const ctrl = new AbortController(); let live = true; let count = 0;
    await aiProof.runAIProofBatches({ batches: pending, signal: ctrl.signal, isCurrent: () => live, changed() {},
      request: async () => { count++; if (stale) live = false; else ctrl.abort(); return '[]'; },
      complete: async () => assert.fail('不可发布取消/过期结果') });
    assert.equal(count, 1);
    assert.equal(pending[0].status, 'cancelled');
    assert.equal(pending[1].status, 'pending');
  }
});

test('超时中断底层请求，单批失败后仍可继续下一批', async () => {
  const batches = [1, 2].map((id) => ({ id, input: '{}', fragments: [], status: 'pending' }));
  let aborted = false;
  await aiProof.runAIProofBatches({ batches, timeoutMs: 5, signal: new AbortController().signal, isCurrent: () => true, changed() {},
    request: async (b, signal) => b.id === 1 ? new Promise(() => signal.addEventListener('abort', () => { aborted = true; })) : 'ok',
    complete: async () => {} });
  assert.equal(aborted, true);
  assert.match(batches[0].error, /超时/);
  assert.equal(batches[1].status, 'succeeded');
});

test('个人 API 要求独立功能绑定，保留其他功能回退行为；能力、参数和 AbortSignal 传递', async () => {
  const profile = { id: 'p', enabled: true, name: '假服务商', baseUrl: 'https://ai.invalid/v1', apiKey: 'test-only', models: ['mock'] };
  assert.throws(() => localAI.resolveLocalAIProfileForFeature({ featureKey: 'log_proofread', profiles: [profile] }), /绑定/);
  assert.equal(localAI.resolveLocalAIProfileForFeature({ featureKey: 'polish', profiles: [profile] }).model, 'mock');
  const bindings = { log_proofread: { providerId: 'p', model: 'selected' } };
  const ctrl = new AbortController();
  const originalFetch = global.fetch;
  global.fetch = async (url, options) => {
    assert.equal(url, 'https://ai.invalid/v1/chat/completions');
    assert.equal(options.signal, ctrl.signal);
    const body = JSON.parse(options.body);
    assert.equal(body.model, 'selected');
    assert.equal(body.temperature, 0.1);
    return { ok: true, json: async () => ({ choices: [{ message: { content: '{"suggestions":[]}' } }] }) };
  };
  try {
    const options = { featureKey: 'log_proofread', input: '{}', profiles: [profile], featureBindings: bindings, signal: ctrl.signal };
    await assert.rejects(localAI.runLocalAIChat(options), /未启用/);
    const result = await localAI.runLocalAIChat({ ...options, feature: { enabled: true, defaultPrompt: 'test', params: { temperature: 0.1 } } });
    assert.equal(result.result, '{"suggestions":[]}');
  } finally { global.fetch = originalFetch; }
});

function panelHarness({ remote = false, save = true, prepareBatches } = {}) {
  const cleanups = []; const notices = []; const calls = []; const requests = [];
  const props = vue.reactive({ projectId: 'p', worldId: 'world', draft: { sections: [], messageContentOverrides: undefined },
    canEditDraft: true, supportsContentOverrides: true, saveError: save ? '' : '模拟保存失败',
    sections: [{ id: 's1', source: { channelId: 'c1' } }, ...(remote ? [{ id: 's2', source: { channelId: 'c2' } }] : [])],
    chapterSectionIds: ['s1'], windowSectionId: 's1', windowComplete: true, windowSourceVersion: 1,
    windowItems: [{ message: { id: 'm1', content: '我以经回来，，走吧', createdAt: 1 } }], selectedIds: [], design: false,
    canvasEl: null, narrow: false, focused: false,
    update: (mutate) => { mutate(props.draft); return true; }, flush: async () => save });
  props.draft.sections = props.sections;
  const aiStore = vue.reactive({ currentSource: 'platform', userProfiles: [], userFeatureBindings: {},
    loadCapabilities: async () => {}, loadUserAISettings: async () => {}, resolveEffectiveSource: (_f, s) => s,
    getFeatureCapability: () => ({ enabled: true, defaultModel: 'fake', params: { maxInputChars: 8000 } }),
    runTask: async (_f, body) => {
      calls.push(body);
      const f = JSON.parse(body.input).fragments.find((f) => f.text.includes('以经'));
      return { data: { result: JSON.stringify({ suggestions: f ? [{ fragmentId: f.id, original: '以经', replacement: '已经', category: 'typo', reason: '错字' }] : [] }) } };
    } });
  const api = {
    describeLogError: (_e, message) => message, isLogError: () => false,
    fetchLogSectionMessages: async (_p, s, cursor, limit) => {
      requests.push({ s, cursor, limit });
      return { sourceVersion: 1, nextCursor: limit === 1 || cursor ? null : 'next', items: limit === 1 ? [] : [
        { message: { id: cursor ? 'm3' : 'm2', content: cursor ? '一切正常' : '他以经走了', createdAt: 1 } },
      ] };
    },
    fetchLogMessageContext: async () => assert.fail('版本相同不需额外读取原消息'),
  };
  const source = read('views/log/components/LogProofreadPanel.vue').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
    + '\nexport { scan, startAI, apply, undoLast, cancel, reset, results, decisions, mode, scope, phase, progress, aiBatches, aiPrepared, aiInvalid, scannedCount, setDecisionKey, acceptSafe };';
  const modules = { 'vue': { ...vue, onBeforeUnmount: (fn) => cleanups.push(fn) }, '@vueuse/core': { useElementSize: () => ({ width: vue.ref(900), height: vue.ref(500) }), useEventListener() {} },
    'naive-ui': { useMessage: () => Object.fromEntries(['info', 'success', 'error', 'warning'].map((kind) => [kind, (s) => notices.push({ kind, s })])) }, '@vicons/tabler': {},
    '@/services/ai/ai-polish-diff': diff, '@/services/log/logApi': api, '@/services/log/proofread/logProofread': proof,
    '@/services/log/proofread/logAIProofread': { ...aiProof, ...(prepareBatches ? { prepareAIProofBatchesAsync: prepareBatches } : {}) }, '@/stores/ai': { useAIStore: () => aiStore }, '@/services/ai/local-ai': localAI,
    '@/services/log/proofread/logProofMarks': { buildProofNode: () => ({}) }, '@/services/log/proofread/zhlintRunner': { loadZhlintLint: async () => (s) => s },
    '@/services/log/logRenderer': { resolveDesignSpeakerName: () => '甲' }, '../logProofreadBatch': batchOps,
    '../logChapterOps': { chapterTitle: () => '当前章节', chapterViews: () => [] }, './LogProofReviewBar.vue': {},
  };
  const instance = vue.effectScope(); let panel;
  instance.run(() => { panel = load('', modules, source, { defineProps: () => props, defineEmits: () => () => {}, defineExpose() {}, window: {} }); });
  return { panel, props, aiStore, calls, notices, requests, close: () => { cleanups.forEach((fn) => fn()); instance.stop(); } };
}

test('真实面板链路：默认基础校对不调用 AI；跨章节分页的全部消息参与准备，AI 需主动发送', async () => {
  const h = panelHarness({ remote: true });
  try {
    assert.equal(h.panel.mode.value, 'basic');
    await h.panel.scan();
    assert.equal(h.calls.length, 0);
    assert.equal(h.panel.aiPrepared.value, false);
    h.panel.mode.value = 'ai'; h.panel.scope.value = 'project';
    await h.panel.scan();
    assert.equal(h.calls.length, 0, '准备不发送正文');
    assert.equal(h.panel.scannedCount.value, 3);
    const wire = h.panel.aiBatches.value.flatMap((b) => JSON.parse(b.input).fragments);
    assert.deepEqual(wire.map((f) => f.messageId), ['m1', 'm2', 'm3']);
    assert.ok(h.requests.some((r) => r.s === 's2' && r.cursor === 'next'));
    await h.panel.startAI();
    assert.equal(h.calls.length, 1, '按批次调用而非逐消息');
    assert.equal(h.panel.results.value[0].issues.some((i) => i.source === 'ai'), true);
    assert.equal(h.props.draft.messageContentOverrides, undefined, '生成建议不写草稿');
  } finally { h.close(); }
});

test('真实面板链路：混合采纳、草稿保存失败反馈、撤销、原消息不变', async () => {
  const h = panelHarness({ save: false });
  try {
    h.panel.mode.value = 'ai'; await h.panel.scan(); await h.panel.startAI();
    h.panel.acceptSafe();
    const result = h.panel.results.value[0];
    const aiIssue = result.issues.find((i) => i.source === 'ai');
    assert.equal(h.panel.decisions.value[batchOps.proofDecisionKey('m1', aiIssue.id)], undefined);
    h.panel.setDecisionKey(batchOps.proofDecisionKey('m1', aiIssue.id), 'accepted');
    await h.panel.apply();
    assert.equal(h.props.draft.messageContentOverrides.m1, '我已经回来，走吧');
    assert.equal(h.props.windowItems[0].message.content, '我以经回来，，走吧');
    assert.ok(h.notices.some((n) => n.kind === 'error' && n.s.includes('模拟保存失败')));
    await h.panel.undoLast();
    assert.equal(h.props.draft.messageContentOverrides, undefined);
    assert.equal(h.panel.results.value.length, 1);
  } finally { h.close(); }
});

test('真实面板链路：切换工程、重新扫描、正文变化使在途结果失效', async () => {
  for (const change of ['project', 'rescan', 'content']) {
    const h = panelHarness();
    try {
      h.panel.mode.value = 'ai'; await h.panel.scan();
      let resolve;
      h.aiStore.runTask = () => new Promise((r) => { resolve = r; });
      const running = h.panel.startAI();
      if (change === 'project') h.props.projectId = 'other';
      if (change === 'rescan') h.panel.reset();
      if (change === 'content') h.props.draft.messageContentOverrides = { m1: '变化后的正文' };
      await vue.nextTick();
      resolve({ data: { result: '{"suggestions":[]}' } });
      await running;
      assert.ok(!h.panel.aiBatches.value.some((b) => b.status === 'succeeded'));
    } finally { h.close(); }
  }
});

test('应用计划让出主线程期间的新草稿正文不会被旧计划覆盖', async () => {
  const h = panelHarness();
  let stop;
  try {
    await h.panel.scan(); h.panel.acceptSafe();
    stop = vue.watch(h.panel.progress, (value) => {
      if (h.panel.phase.value === 'apply' && value.done === value.total) h.props.draft.messageContentOverrides = { m1: '用户刚刚编辑的新正文' };
    }, { flush: 'sync' });
    await h.panel.apply();
    assert.equal(h.props.draft.messageContentOverrides.m1, '用户刚刚编辑的新正文');
    assert.ok(h.notices.some((n) => n.kind === 'warning' && n.s.includes('已跳过')));
  } finally { stop?.(); h.close(); }
});

test('删除不能拼出新指令；已有草稿覆盖也不能遮蔽来源正文变化', () => {
  const source = 'x.r 侦查';
  const edit = { run: 0, start: 0, end: 1, original: 'x', replacement: '', source: 'ai' };
  assert.equal(proof.validAIProofEdit(source, edit), false);
  assert.equal(proof.applyProofEdits(source, [edit]), null);
  const p = prepared('我以经回来');
  const result = parse(p, [suggestion(p.batch)]).results[0];
  result.original = '没有覆盖的原消息';
  const decisions = { [batchOps.proofDecisionKey(result.messageId, result.issues[0].id)]: 'accepted' };
  assert.deepEqual(batchOps.planProofApply([result], decisions, () => result.source, () => '更新后的原消息'), { writes: {}, stale: ['m1'] });
});

test('准备批次期间修改正文时拒绝旧快照；大量文本准备可取消', async () => {
  const h = panelHarness();
  try {
    h.panel.mode.value = 'ai';
    h.aiStore.loadCapabilities = async () => { h.props.draft.messageContentOverrides = { m1: '新的正文' }; };
    await h.panel.scan();
    assert.equal(h.panel.aiPrepared.value, false);
    assert.equal(h.calls.length, 0);
  } finally { h.close(); }
  const ctrl = new AbortController();
  const preparing = aiProof.prepareAIProofBatchesAsync(Array.from({ length: 300 }, (_, i) => meta('正文'.repeat(100), `m${i}`)), 8000, ctrl.signal);
  ctrl.abort();
  await assert.rejects(preparing, /取消/);
});

test('准备末尾再次校验原消息及来源版本；过长 Prompt 不能绕过输入预算', async () => {
  for (const change of ['message', 'version', 'prompt', 'settings']) {
    const h = panelHarness({ prepareBatches: async (...args) => {
      const result = await aiProof.prepareAIProofBatchesAsync(...args);
      if (change === 'message') h.props.windowItems[0].message.content = '新原消息';
      if (change === 'version') h.props.windowSourceVersion++;
      if (change === 'settings') h.aiStore.currentSource = 'user';
      return result;
    } });
    try {
      h.panel.mode.value = 'ai';
      if (change === 'prompt') h.aiStore.getFeatureCapability = () => ({ enabled: true, defaultPrompt: '长'.repeat(8100), params: { maxInputChars: 8000 } });
      await h.panel.scan();
      assert.equal(h.panel.aiPrepared.value, false, change);
      assert.equal(h.panel.aiBatches.value.length, 0, change);
      assert.equal(h.calls.length, 0);
    } finally { h.close(); }
  }
});

test('真实 AI store：来源选择、仅个人模式、工程权限预检与信号贯通', async () => {
  const pinia = require('pinia'); pinia.setActivePinia(pinia.createPinia());
  let allowed = true; const preflight = []; const network = [];
  const storeModule = load('stores/ai.ts', {
    pinia, vue,
    '@/stores/_config': { urlBase: '', buildAuthorizedHeaders: (h) => h,
      api: { post: async (url, body, options) => { preflight.push({ url, body, options }); if (!allowed) throw Error('工程来源不可读'); } } },
    '@/stores/user': { useUserStore: () => ({ token: 'test-session' }) },
    '@/services/ai/local-ai': localAI, '@/utils/authToken': { persistAccessToken() {} },
  });
  const store = storeModule.useAIStore();
  store.features = { log_proofread: { enabled: true, userCustomOnly: false, defaultPrompt: 'test prompt', params: { temperature: 0.1 } } };
  const ctrl = new AbortController();
  const originalFetch = global.fetch;
  global.fetch = async (url, options) => {
    network.push({ url, options });
    if (url.startsWith('/api/')) return { headers: new Map([['content-type', 'text/event-stream']]), ok: true,
      text: async () => 'event: result\ndata: {"result":"{\\"suggestions\\":[]}","model":"platform","providerId":"p"}\n\n' };
    return { ok: true, json: async () => ({ choices: [{ message: { content: '{"suggestions":[]}' } }] }) };
  };
  try {
    const payload = { projectId: 'p', worldId: 'w', sourceVersions: { s: 1 }, input: JSON.stringify({ version: 1, fragments: [{ id: 'f0', messageId: 'm1', sectionId: 's', text: '校对原文不得发送到服务端预检' }] }) };
    await store.runTask('log_proofread', { ...payload, source: 'platform' }, { signal: ctrl.signal });
    assert.equal(preflight.length, 0);
    assert.equal(network[0].options.signal, ctrl.signal);
    await store.saveUserAISettings({ profiles: [{ id: 'personal', enabled: true, name: 'test', baseUrl: 'https://ai.invalid/v1', apiKey: 'test-only', models: ['m'] }], featureBindings: { log_proofread: { providerId: 'personal', model: 'm' } } });
    store.features.log_proofread.userCustomOnly = true;
    await store.runTask('log_proofread', { ...payload, source: 'platform' }, { signal: ctrl.signal });
    assert.equal(preflight.length, 1);
    assert.equal(preflight[0].body.source, 'user');
    assert.equal(preflight[0].body.projectId, 'p');
    assert.equal(preflight[0].body.worldId, undefined);
    assert.deepEqual(JSON.parse(preflight[0].body.input), { version: 1, fragments: [{ id: 'f0', messageId: 'm1', sectionId: 's' }] });
    assert.ok(!JSON.stringify(preflight[0].body).includes('校对原文不得发送到服务端预检'));
    assert.equal(preflight[0].options.signal, ctrl.signal);
    assert.ok(!JSON.stringify(preflight[0].body).includes('test-only'));
    assert.equal(network[1].url, 'https://ai.invalid/v1/chat/completions');
    assert.equal(JSON.parse(network[1].options.body).messages[1].content, payload.input);
    allowed = false;
    await assert.rejects(store.runTask('log_proofread', { ...payload, source: 'user' }), /不可读/);
    assert.equal(network.length, 2, '权限拒绝后不能向个人服务商发起请求');
  } finally { global.fetch = originalFetch; }
});
