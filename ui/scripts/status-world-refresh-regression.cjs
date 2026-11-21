// Run with node scripts/status-world-refresh-regression.cjs from ui/.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const vue = require('vue');

const flush = () => new Promise((resolve) => setImmediate(resolve));

function loadSetup(relativePath, exports, api) {
  const filename = path.resolve(__dirname, relativePath);
  const script = fs.readFileSync(filename, 'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
  const source = ts.transpileModule(`${script}\nexport { ${exports.join(', ')} };`, {
    fileName: filename + '.ts',
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText;
  const mounted = [];
  const unmount = [];
  const intervals = new Map();
  const loaded = { exports: {} };
  const scope = vue.effectScope();
  scope.run(() => vm.runInNewContext(source, {
    module: loaded, exports: loaded.exports, console, defineExpose() {}, defineEmits() { return () => {}; },
    window: {
      setInterval(callback, delay) { intervals.set(1, { callback, delay }); return 1; },
      clearInterval(id) { intervals.delete(id); },
      addEventListener() {}, removeEventListener() {},
    },
    document: { body: { classList: { toggle() {}, remove() {} } } },
    require(name) {
      if (name === 'vue') return { ...vue, onMounted: (fn) => mounted.push(fn), onBeforeUnmount: (fn) => unmount.push(fn) };
      if (name === '@vueuse/core') return { useDebounceFn: (fn) => fn };
      if (name === 'dayjs') return require('dayjs');
      if (name === 'naive-ui') return { NTag: {}, useMessage: () => ({ error() {} }) };
      if (name === '@/stores/_config') return { api };
      if (name === '@/stores/user') return { useUserStore: () => ({ token: 'test', checkPerm: () => true }) };
      if (name.endsWith('.vue')) return {};
      if (name.endsWith('status-history')) return {
        formatStatusNumber: String,
        buildStatusHistoryParams: (query) => query,
        createMetricHistoryStore: (fetch) => ({ clearAll() {}, clearQuery() {}, getQueryData: fetch }),
      };
      throw new Error(`Unexpected import: ${name}`);
    },
  }, { filename }));
  return {
    ...loaded.exports, intervals,
    async mount() { mounted.forEach((fn) => fn()); await flush(); },
    dispose() { unmount.forEach((fn) => fn()); scope.stop(); },
  };
}

async function checkTable() {
  const requests = [];
  const table = loadSetup('../src/views/status/components/StatusWorldTable.vue', ['refresh', 'items', 'handlePageChange', 'handleSorterChange', 'keyword'], {
    get(url, { params }) {
      assert.equal(url, 'api/v1/admin/status/worlds');
      return new Promise((resolve) => requests.push({ params, resolve: (items) => resolve({
        data: { items, total: items.length, summary: { worldCount: 12, memberCount: 0, channelCount: 0, messageCount: 0, messageCountIc: 0, messageCountOoc: 0, messageChars: 0, messageCharsIc: 0, messageCharsOoc: 0 }, dataQuality: { messageChars: 'ready' } },
      }) }));
    },
  });
  await table.mount();
  assert.equal(requests.length, 1, 'a table mounted after permissions load must fetch its own data');
  assert.equal(requests[0].params.sortBy, 'createdAt');
  assert.equal(requests[0].params.order, 'asc');
  const refresh = table.refresh();
  requests[1].resolve([{ id: 'new', onlineCount: 2 }]);
  await refresh;
  requests[0].resolve([{ id: 'stale', onlineCount: 0 }]);
  await flush();
  assert.equal(table.items.value[0].id, 'new', 'an older response must not overwrite new online counts');

  table.handlePageChange(3);
  assert.equal(requests[2].params.page, 3);
  table.handleSorterChange({ columnKey: 'messageChars', order: 'descend', sorter: true });
  assert.equal(requests[3].params.page, 1);
  assert.equal(requests[3].params.sortBy, 'messageChars');
  assert.equal(requests[3].params.order, 'desc');
  table.keyword.value = '  Alpha  ';
  await flush();
  assert.equal(requests[4].params.keyword, 'Alpha');
  assert.equal(requests[4].params.page, 1);
  requests[4].resolve([{ id: 'first', messageChars: 50 }, { id: 'second', messageChars: 80 }]);
  requests[3].resolve([]);
  requests[2].resolve([]);
  await flush();
  assert.equal(table.items.value[0].id, 'first', 'the view must preserve the order returned by the server');

  const pending = table.refresh();
  table.dispose();
  requests[5].resolve([{ id: 'unmounted' }]);
  await pending;
  assert.equal(table.items.value[0].id, 'first', 'a response after unmount must be ignored');
  await table.refresh();
  assert.equal(requests.length, 6, 'a delayed refresh after unmount must not send a request');
}

async function checkDashboard() {
  const requests = [];
  const page = loadSetup('../src/views/status/StatusDashboard.vue', ['worldTable', 'refreshAll'], {
    async get(url) { requests.push(url); return { data: { points: [] } }; },
  });
  await page.mount();
  assert.equal(page.intervals.size, 1);
  const timer = page.intervals.get(1);
  assert.equal(timer.delay, 60_000);

  // 模拟权限异步加载后才挂载世界列表。
  let worldRefreshes = 0;
  page.worldTable.value = { async refresh() { worldRefreshes++; } };
  timer.callback();
  await flush();
  assert.equal(worldRefreshes, 1, 'automatic refresh must update the world table');
  assert.equal(requests.filter((url) => url === 'api/v1/status').length, 2);
  assert.equal(requests.filter((url) => url === 'api/v1/status/history').length, 1, 'automatic refresh must not clear historical queries');

  await page.refreshAll();
  assert.equal(worldRefreshes, 2, 'manual refresh must still update the world table');
  page.dispose();
  assert.equal(page.intervals.size, 0, 'unmount must clear the refresh timer');
}

(async () => {
  await checkTable();
  await checkDashboard();
  console.log('status world refresh regression passed: mount, periodic/manual refresh, stale responses, and cleanup');
})().catch((error) => { console.error(error); process.exitCode = 1; });
