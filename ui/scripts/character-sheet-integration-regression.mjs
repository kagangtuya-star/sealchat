import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import vm from 'node:vm';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const vue = require('vue');
const pinia = require('pinia');

function loadTs(relativePath, imports = {}, sourceOverride) {
  const filename = new URL(relativePath, import.meta.url);
  const source = ts.transpileModule(sourceOverride ?? readFileSync(filename, 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText;
  const loaded = { exports: {} };
  vm.runInNewContext(source, {
    module: loaded,
    exports: loaded.exports,
    require(name) {
      if (name in imports) return imports[name];
      if (name === 'vue') return vue;
      if (name === 'pinia') return pinia;
      throw new Error(`Unexpected import: ${name}`);
    },
  }, { filename: filename.pathname });
  return loaded.exports;
}

const metadata = loadTs('../src/utils/characterSheetType.ts');
const {
  normalizeCharacterSheetType,
  getCharacterSheetTypeMeta,
  parseCharacterSheetTemplateManifest,
  getCharacterSheetTemplateIntegration,
  matchCharacterSheetProbeResponse,
} = metadata;
const shinobigamiHtml = readFileSync(new URL('../../doc/template/sealchat-shinobigami-template-v1.html', import.meta.url), 'utf8');
const daggerheartHtml = readFileSync(new URL('../../doc/template/sealchat-daggerheart-template-v1.html', import.meta.url), 'utf8');

function setup(interact = async () => ({ ok: true, content: '忍神掷骰插件 v1 帮助' })) {
  pinia.setActivePinia(pinia.createPinia());
  const channels = vue.reactive({
    'channel-1': { id: 'channel-1', primaryBotId: 'bot-1' },
    'channel-2': { id: 'channel-2', primaryBotId: 'bot-1' },
    'private-bot': {
      id: 'private-bot',
      isPrivate: true,
      permType: 'private',
      primaryBotId: '',
      friendInfo: { userInfo: { id: 'bot-private', is_bot: true } },
    },
  });
  const calls = [];
  const chat = vue.reactive({
    curChannel: channels['channel-1'],
    connectState: 'connected',
    observerMode: false,
    findChannelById: id => channels[id],
    botInteract(...args) { calls.push(args); return interact(...args); },
  });
  const { useCharacterSheetIntegrationStore } = loadTs('../src/stores/characterSheetIntegration.ts', {
    './chat': { useChatStore: () => chat },
    '@/utils/characterSheetType': metadata,
  });
  const rawStore = useCharacterSheetIntegrationStore();
  const hasTemplateHtml = value => typeof value === 'string' && value.includes('data-sealchat-template-manifest');
  const store = {
    getCacheKey(sheetType, templateHtmlOrChannelId, channelId) {
      return hasTemplateHtml(templateHtmlOrChannelId)
        ? rawStore.getCacheKey(sheetType, templateHtmlOrChannelId, channelId)
        : rawStore.getCacheKey(sheetType, shinobigamiHtml, templateHtmlOrChannelId);
    },
    getStatus(sheetType, templateHtmlOrChannelId, channelId) {
      return hasTemplateHtml(templateHtmlOrChannelId)
        ? rawStore.getStatus(sheetType, templateHtmlOrChannelId, channelId)
        : rawStore.getStatus(sheetType, shinobigamiHtml, templateHtmlOrChannelId);
    },
    check(sheetType, templateHtmlOrChannelId, channelIdOrForce, force) {
      return hasTemplateHtml(templateHtmlOrChannelId)
        ? rawStore.check(sheetType, templateHtmlOrChannelId, channelIdOrForce, force)
        : rawStore.check(sheetType, shinobigamiHtml, templateHtmlOrChannelId, Boolean(channelIdOrForce));
    },
  };
  return { store, rawStore, chat, channels, calls };
}

test('规则类型只维护别名，忍神插件声明来自 HTML manifest', () => {
  for (const input of ['忍神', 'shinobigami', ' SHINOBIGAMI ']) {
    assert.equal(normalizeCharacterSheetType(input), 'shinobigami');
  }
  assert.equal(normalizeCharacterSheetType(' Custom-Rule '), 'custom-rule');
  assert.equal(getCharacterSheetTypeMeta('忍神').integration, undefined);
  const manifest = parseCharacterSheetTemplateManifest(shinobigamiHtml);
  assert.equal(manifest.version, 1);
  assert.equal(manifest.sheetType, 'shinobigami');
  assert.equal(manifest.label, '忍神');
  const integration = getCharacterSheetTemplateIntegration(shinobigamiHtml, '忍神');
  assert.equal(integration.provider, 'sealdice');
  assert.equal(integration.probe.command, '。sbg h');
  assert.equal(integration.probe.timeoutMs, 5000);
  assert.equal(integration.plugin.acquireText, '可在 SealChat 用户群获取');
  assert.equal(integration.plugin.acquireUrl, undefined);
});

test('contains 与 equals 只按 manifest 声明的文本规则匹配', () => {
  const probe = getCharacterSheetTemplateIntegration(shinobigamiHtml, 'shinobigami').probe;
  assert.equal(matchCharacterSheetProbeResponse(probe, '欢迎使用忍神掷骰插件！'), true);
  assert.equal(matchCharacterSheetProbeResponse(probe, '未知指令'), false);
  const exact = { ...probe, matchMode: 'equals' };
  assert.equal(matchCharacterSheetProbeResponse(exact, '忍神掷骰插件'), true);
  assert.equal(matchCharacterSheetProbeResponse(exact, '欢迎使用忍神掷骰插件！'), false);
});

test('Daggerheart 别名、manifest 和快照预设复用统一注册表', () => {
  const presets = loadTs('../src/utils/characterSnapshotTemplatePresets.ts', {
    '@/utils/characterSheetType': metadata,
  });
  for (const alias of ['daggerheart', 'dh', '匕首之心', ' DH ']) {
    assert.equal(normalizeCharacterSheetType(alias), 'daggerheart');
    assert.equal(getCharacterSheetTypeMeta(alias).label, '匕首之心');
    assert.equal(presets.getCharacterSnapshotTemplatePreset(alias), 'daggerheart');
    const integration = getCharacterSheetTemplateIntegration(daggerheartHtml, alias);
    assert.equal(integration.provider, 'sealdice');
    assert.equal(integration.probe.command, '.dd help');
    assert.equal(integration.probe.matchMode, 'contains');
    assert.equal(integration.probe.expected, '.dd：动作掷骰；.ddr：反应掷骰。');
    assert.equal(integration.plugin.acquireUrl, 'https://github.com/ZZZZzzzzac/Daggerheart_sealdice');
  }
  assert.equal(presets.CHARACTER_SNAPSHOT_TEMPLATE_PRESET_LABELS.daggerheart, '匕首之心');
  assert.equal(presets.CHARACTER_SNAPSHOT_OVERLAY_TEMPLATE_PRESETS.daggerheart.items.length, 4);
  assert.equal(presets.AVATAR_WORLD_STATE_OVERLAY_TEMPLATE_PRESETS.daggerheart.items.length, 4);
});

test('Daggerheart 插件帮助匹配成功，明确不匹配才缺失，通信失败不能判为缺失', async () => {
  for (const [response, status] of [
    [{ ok: true, content: '帮助：.dd：动作掷骰；.ddr：反应掷骰。更多指令' }, 'available'],
    [{ ok: true, content: '未知指令' }, 'missing'],
    [{ ok: false, content: '' }, 'unavailable'],
  ]) {
    const { rawStore, calls } = setup(async () => response);
    assert.equal(await rawStore.check('dh', daggerheartHtml, 'channel-1'), status);
    assert.equal(rawStore.getStatus('匕首之心', daggerheartHtml, 'channel-1'), status);
    assert.equal(calls[0][1], '.dd help');
  }
});

test('Daggerheart 内置模板只注册一次，别名筛选复用内置内容识别和默认选择', async () => {
  pinia.setActivePinia(pinia.createPinia());
  const created = [];
  const { useCharacterCardTemplateStore } = loadTs('../src/stores/characterCardTemplate.ts', {
    '../../../doc/template/sealchat-default-template-v3-coc7th.html?raw': { default: '<html>coc</html>' },
    '../../../doc/template/sealchat-shinobigami-template-v1.html?raw': { default: shinobigamiHtml },
    '../../../doc/template/sealchat-daggerheart-template-v1.html?raw': { default: daggerheartHtml },
    '../../../doc/template/sealchat-triangle-agency-template-v1.html?raw': { default: '<html>ta</html>' },
    '@/utils/characterSheetType': metadata,
    './user': { useUserStore: () => ({ info: { id: 'user-1' } }) },
    './_config': { api: {
      async get() { return { data: { items: created } }; },
      async post(url, payload) {
        const item = { ...payload, id: String(created.length + 1) };
        created.push(item);
        return { data: { item } };
      },
    } },
  });
  const store = useCharacterCardTemplateStore();
  await store.ensureTemplatesLoaded();
  await store.ensureTemplatesLoaded();
  assert.equal(created.filter(item => item.sheetType === 'daggerheart').length, 1);
  assert.equal(store.isBuiltinTemplateContent(daggerheartHtml), true);
  for (const alias of ['daggerheart', 'dh', '匕首之心']) {
    const templates = store.getTemplatesBySheetType(alias);
    assert.equal(templates.length, 1);
    assert.equal(templates[0].name, '匕首之心角色卡');
    assert.equal(store.getPreferredTemplateBySheetType(alias).content, daggerheartHtml.trim());
  }
});

test('unknown → checking → available，同一键与别名共享缓存，支持手动重检', async () => {
  let finish;
  const { store, calls } = setup(() => new Promise(resolve => { finish = resolve; }));
  assert.equal(store.getStatus('忍神'), 'unknown');
  const result = store.check('忍神');
  assert.equal(store.getStatus('shinobigami'), 'checking');
  const concurrent = store.check('shinobigami', undefined, true);
  assert.equal(calls.length, 1, 'checking 时手动与自动检测均不重复请求');
  assert.equal(calls[0][0], 'channel-1');
  assert.equal(calls[0][1], '。sbg h');
  assert.equal(calls[0][2].timeoutMs, 5000);
  finish({ ok: true, content: '忍神掷骰插件帮助' });
  assert.equal(await result, 'available');
  assert.equal(await concurrent, 'available');
  assert.equal(await store.check('shinobigami'), 'available');
  assert.equal(calls.length, 1);
  const retry = store.check('忍神', undefined, true);
  assert.equal(calls.length, 2);
  finish({ ok: true, content: '未知指令' });
  assert.equal(await retry, 'missing');
});

test('成功获得不匹配的文本才判为 missing，自动检测缓存该结果', async () => {
  const { store, calls } = setup(async () => ({ ok: true, content: '未知指令' }));
  assert.equal(await store.check('忍神'), 'missing');
  assert.equal(store.getStatus('忍神'), 'missing');
  assert.equal(await store.check('忍神'), 'missing');
  assert.equal(calls.length, 1);
});

test('timeout、busy、离线、权限与通信异常均为 unavailable 且不会自动重试', async () => {
  for (const code of [
    'BOT_INTERACTION_TIMEOUT', 'bot.interact timeout', 'BOT_INTERACTION_BUSY',
    'BOT_INTERACTION_BOT_UNAVAILABLE', 'PERMISSION_DENIED', 'ws not connected', 'connection closed',
  ]) {
    const { store, calls } = setup(async () => { throw Object.assign(new Error(code), { response: { err: code } }); });
    assert.equal(await store.check('忍神'), 'unavailable', code);
    assert.equal(store.getStatus('忍神'), 'unavailable', code);
    assert.equal(await store.check('忍神'), 'unavailable');
    assert.equal(calls.length, 1);
  }
});

test('无频道、无主控 BOT、WS 未连接及观察者模式不发送请求', async () => {
  for (const change of [
    chat => { chat.curChannel = null; },
    chat => { chat.curChannel.primaryBotId = ''; },
    chat => { chat.connectState = 'disconnected'; },
    chat => { chat.observerMode = true; },
  ]) {
    const { store, chat, calls } = setup();
    change(chat);
    assert.equal(await store.check('忍神'), 'unavailable');
    assert.equal(calls.length, 0);
  }
});

test('BOT 私聊无 primaryBotId 仍可探测，并按对端 BOT 隔离缓存', async () => {
  const { store, chat, channels, calls } = setup();
  chat.curChannel = channels['private-bot'];
  assert.equal(await store.check('忍神'), 'available');
  assert.equal(calls.length, 1);
  const firstKey = store.getCacheKey('忍神');
  channels['private-bot'].friendInfo.userInfo.id = 'bot-private-2';
  assert.notEqual(store.getCacheKey('忍神'), firstKey);
  assert.equal(store.getStatus('忍神'), 'unknown');
  assert.equal(await store.check('忍神'), 'available');
  assert.equal(calls.length, 2);
});

test('结构化回复没有文本、空回复及失败结果均不能确认插件缺失', async () => {
  for (const response of [
    undefined, { ok: false, content: '忍神掷骰插件' },
    { ok: true, matched_by: 'structured', data: { installed: true } },
    { ok: true, matched_by: 'structured', content: '', data: { installed: true } },
    { ok: true, content: null }, { ok: true, content: '  ' },
  ]) {
    const { store } = setup(async () => response);
    assert.equal(await store.check('忍神'), 'unavailable');
  }
});

test('更换主控 BOT、频道会更新响应式状态并使用独立缓存', async () => {
  const { store, chat, channels, calls } = setup();
  const displayed = vue.computed(() => store.getStatus('忍神'));
  await store.check('忍神');
  assert.equal(displayed.value, 'available');
  chat.curChannel.primaryBotId = 'bot-2';
  assert.equal(displayed.value, 'unknown');
  await store.check('忍神');
  assert.equal(calls.length, 2);
  chat.curChannel = channels['channel-2'];
  assert.equal(displayed.value, 'unknown');
  await store.check('忍神');
  assert.equal(calls.length, 3);
  chat.curChannel = channels['channel-1'];
  chat.curChannel.primaryBotId = 'bot-1';
  assert.equal(displayed.value, 'available');
  await store.check('忍神');
  assert.equal(calls.length, 3);
});

test('主控 BOT 在请求期间变化，旧请求不得覆盖新 BOT 的结果', async () => {
  const resolvers = [];
  const { store, chat } = setup(() => new Promise(resolve => resolvers.push(resolve)));
  const oldRequest = store.check('忍神');
  chat.curChannel.primaryBotId = 'bot-2';
  const newRequest = store.check('忍神');
  resolvers[1]({ ok: true, content: '未知指令' });
  assert.equal(await newRequest, 'missing');
  resolvers[0]({ ok: true, content: '忍神掷骰插件' });
  assert.equal(await oldRequest, 'unavailable');
  assert.equal(store.getStatus('忍神'), 'missing');
});

test('无需插件依赖的规则不发送探测命令', async () => {
  const { store, calls } = setup();
  for (const sheetType of ['coc7', 'dnd5e', 'custom-rule']) {
    assert.equal(await store.check(sheetType), 'unknown');
  }
  assert.equal(calls.length, 0);
});

test('共用提示组件首次自动检测、重新挂载复用缓存，BOT 变化只检测一次', async () => {
  const { parse, compileScript } = require('@vue/compiler-sfc');
  const file = '../src/views/chat/components/CharacterSheetIntegrationNotice.vue';
  const { descriptor } = parse(readFileSync(new URL(file, import.meta.url), 'utf8'));
  const compiled = compileScript(descriptor, { id: 'integration-notice-test' });
  const { store, chat, calls } = setup();
  const component = loadTs(file, {
    'naive-ui': { NButton: {} },
    '@/stores/characterSheetIntegration': { useCharacterSheetIntegrationStore: () => store },
    '@/utils/characterSheetType': metadata,
  }, compiled.content).default;
  const props = vue.reactive({
    sheetType: '忍神',
    templateHtml: shinobigamiHtml,
    channelId: 'channel-1',
    autoCheck: true,
  });
  const mount = () => {
    const scope = vue.effectScope();
    scope.run(() => component.setup(props, { expose() {} }));
    return scope;
  };
  let scope = mount();
  try {
    await store.check('忍神');
    assert.equal(calls.length, 1);
    scope.stop();
    scope = mount();
    await vue.nextTick();
    assert.equal(calls.length, 1);
    props.sheetType = 'shinobigami';
    await vue.nextTick();
    assert.equal(calls.length, 1);
    chat.curChannel.primaryBotId = 'bot-2';
    await vue.nextTick();
    await store.check('忍神');
    assert.equal(calls.length, 2);
  } finally {
    scope.stop();
  }
});

test('内置模板正常检测后静默，第三方模板保留手动检测入口', () => {
  const source = readFileSync(new URL('../src/views/chat/components/CharacterSheetIntegrationNotice.vue', import.meta.url), 'utf8');
  assert.match(source, /status === 'missing'/);
  assert.doesNotMatch(source, /已检测到/);
  assert.match(source, /showManualPrompt/);
  assert.match(source, /!props\.autoCheck/);
  assert.match(source, /检测插件/);
  assert.match(source, />重新检测<\/n-button>/);
});

test('人物卡窗口统一挂载依赖提示，覆盖主窗口、独立窗口和只读快照', () => {
  const windowSource = readFileSync(new URL('../src/views/chat/components/character-sheet/CharacterSheetWindow.vue', import.meta.url), 'utf8');
  const internalSource = readFileSync(new URL('../src/views/internal/surfaces/CharacterInternalSurface.vue', import.meta.url), 'utf8');
  const panelSource = readFileSync(new URL('../src/views/chat/components/CharacterCardPanel.vue', import.meta.url), 'utf8');
  assert.match(windowSource, /<CharacterSheetIntegrationNotice[\s\S]*:sheet-type="windowData\.sheetType"[\s\S]*:template-html="windowData\.template"[\s\S]*:auto-check="integrationAutoCheck"/);
  assert.match(internalSource, /<CharacterSheetManager[\s\S]*standalone/);
  assert.match(internalSource, /readOnly:\s*true/);
  assert.match(panelSource, /readOnly:\s*true/);
  assert.match(panelSource, /sheetStore\.openSheet\(onlinePreviewCard\(item\)/);
});

test('manifest 校验拒绝危险或越界声明，且 sheetType 不匹配时不启用探测', () => {
  const wrap = body => `<script type="application/json" data-sealchat-template-manifest>${JSON.stringify(body)}</script>`;
  const base = {
    version: 1,
    sheetType: 'custom-rule',
    integration: {
      provider: 'sealdice',
      probe: { command: '。help', matchMode: 'contains', expected: '插件', timeoutMs: 5000 },
      plugin: { name: '测试插件', acquireUrl: 'https://example.com/plugin' },
    },
  };
  assert.equal(parseCharacterSheetTemplateManifest(wrap(base)).sheetType, 'custom-rule');
  assert.equal(getCharacterSheetTemplateIntegration(wrap(base), 'other-rule'), undefined);
  assert.equal(parseCharacterSheetTemplateManifest(wrap({ ...base, version: 2 })), null);
  assert.equal(parseCharacterSheetTemplateManifest(wrap({ ...base, integration: { ...base.integration, probe: { ...base.integration.probe, command: '。help\n。danger' } } })), null);
  assert.equal(parseCharacterSheetTemplateManifest(wrap({ ...base, integration: { ...base.integration, plugin: { ...base.integration.plugin, acquireUrl: 'javascript:alert(1)' } } })), null);
});

test('不同 manifest 探测声明使用不同缓存键', () => {
  const { rawStore } = setup();
  const alternate = shinobigamiHtml.replace('。sbg h', '。sbg help');
  assert.notEqual(
    rawStore.getCacheKey('忍神', shinobigamiHtml, 'channel-1'),
    rawStore.getCacheKey('忍神', alternate, 'channel-1'),
  );
});

test('第三方 manifest 不会被提示组件自动执行', async () => {
  const { parse, compileScript } = require('@vue/compiler-sfc');
  const file = '../src/views/chat/components/CharacterSheetIntegrationNotice.vue';
  const { descriptor } = parse(readFileSync(new URL(file, import.meta.url), 'utf8'));
  const compiled = compileScript(descriptor, { id: 'integration-notice-third-party-test' });
  const { store, calls } = setup();
  const component = loadTs(file, {
    'naive-ui': { NButton: {} },
    '@/stores/characterSheetIntegration': { useCharacterSheetIntegrationStore: () => store },
    '@/utils/characterSheetType': metadata,
  }, compiled.content).default;
  const props = vue.reactive({
    sheetType: '忍神',
    templateHtml: shinobigamiHtml.replace('<body>', '<body data-third-party="1">'),
    channelId: 'channel-1',
    autoCheck: false,
  });
  const scope = vue.effectScope();
  try {
    scope.run(() => component.setup(props, { expose() {} }));
    await vue.nextTick();
    assert.equal(calls.length, 0);
  } finally {
    scope.stop();
  }
});

test('复用现有 botInteract 的协议封装和客户端超时余量', async () => {
  const source = readFileSync(new URL('../src/stores/chat.ts', import.meta.url), 'utf8');
  const start = source.indexOf('async botInteract(');
  const end = source.indexOf('async botNicknameSyncDispatch(', start);
  const compiled = ts.transpileModule(`({ ${source.slice(start, end)} })`, {
    compilerOptions: { target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const action = vm.runInNewContext(compiled).botInteract;
  let request;
  const expected = { ok: true, content: '忍神掷骰插件' };
  const result = await action.call({
    async sendAPI(...args) { request = args; return { data: expected }; },
  }, 'channel-1', '。sbg h', { timeoutMs: 5000 });
  assert.equal(request[0], 'bot.interact');
  assert.equal(request[1].channel_id, 'channel-1');
  assert.equal(request[1].command, '。sbg h');
  assert.equal(request[1].timeout_ms, 5000);
  assert.equal(request[2].timeoutMs, 7000);
  assert.equal(result, expected);
});
