import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync, mkdtempSync, rmSync, realpathSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname, extname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import http from 'node:http';
import { build } from 'vite';
import { createRequire } from 'node:module';
import ts from 'typescript';

const ui = dirname(dirname(fileURLToPath(import.meta.url)));
const require = createRequire(join(ui, 'package.json'));
const chrome = process.env.CHROME_BIN || [
  join(process.env.PROGRAMFILES || '', 'Google/Chrome/Application/chrome.exe'),
  join(process.env.LOCALAPPDATA || '', 'Google/Chrome/Application/chrome.exe'),
  '/usr/bin/google-chrome', '/usr/bin/chromium', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
].find(path => existsSync(path));

test('real context menus remain reachable at viewport edges and when scrolled', { timeout: 60000 }, async () => {
  assert.ok(chrome, 'Install Chrome/Chromium or set CHROME_BIN');
  const temp = realpathSync(tmpdir());
  const dir = mkdtempSync(join(temp, 'sealchat-menu-test-'));
  assert.equal(dirname(realpathSync(dir)), temp);
  assert.ok(basename(dir).startsWith('sealchat-menu-test-'));
  const source = readFileSync(join(ui, 'src/views/chat/components/ChatRightClickMenu.vue'), 'utf8');
  const script = source.match(/<script setup[^>]*>([\s\S]*?)<\/script>/)[1];
  const ast = ts.createSourceFile('menu.tsx', script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const names = ['contextMenuClass', 'contextMenuTheme', 'contextMenuZIndex', 'contextMenuOptions'];
  const optionsSource = ts.transpileModule(ast.statements.filter(node => ts.isVariableStatement(node)
    && node.declarationList.declarations.some(entry => names.includes(entry.name.getText(ast))))
    .map(node => node.getText(ast)).join('\n'), { compilerOptions: { target: ts.ScriptTarget.ES2020 } }).outputText;
  let template = source.match(/<template>\s*([\s\S]*?<\/context-menu>)/)[1];
  template = template.replace('<context-menu-group label="更多">', '<context-menu-item v-for="i in extraItems" :key="i" :label="`Extra ${i}`" /><context-menu-group label="更多">');
  let server;
  try {
    await build({ root: ui, configFile: false, logLevel: 'silent',
      resolve: { alias: { vue: require.resolve('vue/dist/vue.esm-bundler.js') } },
      define: { 'process.env.NODE_ENV': '"production"', __VUE_OPTIONS_API__: 'true', __VUE_PROD_DEVTOOLS__: 'false', __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: 'false' },
      build: { outDir: dir, emptyOutDir: false, minify: false,
        lib: { entry: join(ui, 'scripts/fixtures/context-menu-viewport.js'), name: 'MenuViewportTest', formats: ['iife'], fileName: () => 'fixture.js' },
      },
    });
    const html = '<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="/style.css"><style>html,body{margin:0;height:100%;font-family:Arial,sans-serif}.reaction-picker-slot{min-height:57px;box-sizing:border-box;padding:6px 8px}</style><div id="app"></div><script id="result" type="application/json"></script><script>window.menuTemplate=' + JSON.stringify(template) + ';window.menuOptionsSource=' + JSON.stringify(optionsSource) + '</script><script src="/fixture.js"></script>';
    server = http.createServer((req, res) => {
      if (req.url === '/') { res.setHeader('Content-Type', 'text/html; charset=utf-8'); return res.end(html); }
      const name = req.url.slice(1);
      if (!['style.css', 'fixture.js'].includes(name)) return res.writeHead(404).end();
      res.setHeader('Content-Type', extname(name) === '.css' ? 'text/css' : 'application/javascript');
      res.end(readFileSync(join(dir, name)));
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    const { stdout } = await promisify(execFile)(chrome, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-gpu', '--disable-background-networking', '--force-device-scale-factor=1', '--window-size=1280,720', '--virtual-time-budget=5000', '--dump-dom', '--user-data-dir=' + join(dir, 'profile'), `http://127.0.0.1:${server.address().port}/`], { timeout: 30000, maxBuffer: 4 * 1024 * 1024, encoding: 'utf8' });
    const result = stdout.match(/<script id="result" type="application\/json">([\s\S]*?)<\/script>/)?.[1];
    assert.ok(result, 'Browser must emit its test result');
    const report = JSON.parse(result);
    assert.equal(report.passed, true, JSON.stringify(report));
    console.log(JSON.stringify({ dependency: require('@imengyu/vue3-context-menu/package.json').version, ...report }));
  } finally {
    if (server) await new Promise(resolve => server.close(resolve));
    // Only this test's unique Temp directory, never a project or user profile.
    rmSync(dir, { recursive: true, force: true, maxRetries: 3, retryDelay: 200 });
  }
});
