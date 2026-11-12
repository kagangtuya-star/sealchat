// Real shared table menu + native editor, without a signed-in chat session.
// node scripts/tiptap-table-ui-regression.cjs
const fs = require('node:fs');
const path = require('node:path');
const { execFile } = require('node:child_process');
const { promisify } = require('node:util');
const assert = require('node:assert/strict');
const { createServer } = require('vite');
const vue = require('@vitejs/plugin-vue');

async function check(createApp, h, ref, Editor, StarterKit, TableKit, TableMenu, NButton, NPopover, renderHtml) {
  const delay = () => new Promise(resolve => setTimeout(resolve, 150));
  let checks = 0;
  const ok = (value, label) => { checks++; if (!value) throw Error(label); };
  const version = ref(0), show = ref(false);
  const editor = new Editor({ element: document.querySelector('#editor'), editorProps: { attributes: { class: 'tiptap-content' } },
    extensions: [StarterKit, TableKit.configure({ table: { resizable: true, cellMinWidth: 80 } })],
    onUpdate: () => version.value++, onSelectionUpdate: () => version.value++,
  });
  const app = createApp({ render: () => h(TableMenu, { editor, stateVersion: version.value, show: show.value, contentClass: '', 'onUpdate:show': value => show.value = value }) });
  app.component('n-button', NButton); app.component('n-popover', NPopover); app.mount('#menu');
  const button = label => Array.from(document.querySelectorAll('button')).find(el => el.textContent.trim() === label);
  const open = async () => { button('表格').click(); await delay(); };
  await open();
  const target = document.querySelector('[aria-label="4 列 × 3 行"]');
  target.dispatchEvent(new PointerEvent('pointerenter', { pointerType: 'mouse' })); await delay();
  ok(document.querySelectorAll('.table-menu__grid .is-selected').length === 12, 'grid hover highlights dimensions');
  ok(document.querySelector('[role="dialog"]').contains(target), 'popover remains inside drawer/modal focus scope');
  // Direct click has no preceding hover, matching touch/keyboard activation.
  document.querySelector('[aria-label="3 列 × 2 行"]').click(); await delay();
  let table = editor.getJSON().content.find(node => node.type === 'table');
  ok(table.content.length === 2 && table.content[0].content.length === 3, 'direct grid click inserts requested dimensions');
  ok(table.content[0].content[0].type === 'tableHeader', 'insert includes header');
  ok(document.activeElement === editor.view.dom, 'insertion returns focus to editor');
  const wrapper = document.querySelector('.tableWrapper');
  ok(wrapper.clientWidth <= 220 && wrapper.scrollWidth > wrapper.clientWidth, 'narrow editor scrolls table');
  const message = document.createElement('div'); message.className = 'chat-item chat-item--layout-bubble';
  message.innerHTML = `<div class="right"><div class="content typo">${renderHtml(editor.getJSON())}</div></div>`;
  document.body.appendChild(message);
  for (const width of [640, 220]) {
    message.style.width = `${width}px`; await delay();
    const bubble = message.querySelector('.content');
    const scroll = message.querySelector('.tiptap-table-wrapper');
    ok(message.scrollWidth <= width && bubble.scrollWidth <= bubble.clientWidth, `chat bubble stays within ${width}px`);
    if (width === 220) ok(scroll.scrollWidth > scroll.clientWidth, 'mobile message keeps columns scrollable');
  }
  const header = message.querySelector('th');
  ok(getComputedStyle(header).color === getComputedStyle(message.querySelector('.content')).color, 'table overrides legacy typo light-only cell colors');
  message.remove();
  await open();
  ok(!document.querySelector('.table-menu__grid') && !!button('上方插入行'), 'table selection opens context actions');
  ok(button('合并单元格').disabled && button('拆分单元格').disabled, 'unavailable actions are disabled in UI');
  const positions = [];
  editor.state.doc.descendants((node, pos) => { if (node.type.name === 'tableHeader') positions.push(pos); });
  editor.commands.setCellSelection({ anchorCell: positions[0], headCell: positions[1] }); await delay();
  ok(!button('合并单元格').disabled, 'cell selection enables merge in UI');
  button('合并单元格').focus(); button('合并单元格').click(); await delay();
  table = editor.getJSON().content.find(node => node.type === 'table');
  ok(table.content[0].content[0].attrs.colspan === 2, 'popover focus preserves native cell selection');
  await open();
  ok(!button('拆分单元格').disabled, 'merged cell enables split in UI');
  button('拆分单元格').click(); await delay();
  await open(); button('删除整张表格').click(); await delay();
  ok(!editor.getJSON().content.some(node => node.type === 'table'), 'delete table from context menu');
  app.unmount(); editor.destroy();
  return { passed: true, checks };
}

const script = `import { createApp, h, ref } from 'vue';
import { Editor } from '@tiptap/core';
import { StarterKit } from '@tiptap/starter-kit';
import { TableKit } from '@tiptap/extension-table';
import { NButton, NPopover } from 'naive-ui';
import TableMenu from '/src/components/rich-text/TableMenu.vue';
import { tiptapJsonToHtml } from '/src/utils/tiptap-render.ts';
import '/src/assets/main.css';
import '/src/views/chat/components/chat-item.vue?vue&type=style&index=1&lang.scss';
(${check.toString()})(createApp,h,ref,Editor,StarterKit,TableKit,TableMenu,NButton,NPopover,tiptapJsonToHtml)
.then(result => document.getElementById('result').textContent=JSON.stringify(result))
.catch(error => document.getElementById('result').textContent=JSON.stringify({passed:false,error:error.stack}));`;
const html = `<!doctype html><meta charset="utf-8"><style>body{margin:0;--sc-bg-active:#ddd}#editor{width:220px}.tiptap-content{outline:none}</style><div role="dialog"><div id="menu"></div><div id="editor"></div></div><script id="result" type="application/json"></script><script type="module">${script}</script>`;

(async () => {
  const server = await createServer({ configFile: false, root: path.join(__dirname, '..'), plugins: [vue(), {
    name: 'table-test-page', configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        if (req.url !== '/table-fixture') return next();
        try { res.setHeader('Content-Type', 'text/html'); res.end(await server.transformIndexHtml('/table-fixture', html)); }
        catch (error) { next(error); }
      });
    },
  }], server: { host: '127.0.0.1', port: 0 }, resolve: { alias: { '@': path.join(__dirname, '../src') } } });
  try {
    await server.listen();
    const url = `http://127.0.0.1:${server.httpServer.address().port}/table-fixture`;
    const chrome = process.env.CHROME_BIN || ['/usr/bin/google-chrome', '/usr/bin/chromium', 'C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(fs.existsSync);
    assert.ok(chrome, 'Set CHROME_BIN to an existing Chrome/Chromium browser');
    const { stdout } = await promisify(execFile)(chrome, ['--headless=new', '--disable-gpu', '--disable-background-networking', '--virtual-time-budget=30000', '--dump-dom', url], { timeout: 60000, maxBuffer: 4 * 1024 * 1024 });
    const report = JSON.parse(stdout.match(/<script id="result" type="application\/json">([\s\S]*?)<\/script>/)?.[1] || '{}');
    assert.equal(report.passed, true, JSON.stringify(report)); console.log(report);
  } finally { await server.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
