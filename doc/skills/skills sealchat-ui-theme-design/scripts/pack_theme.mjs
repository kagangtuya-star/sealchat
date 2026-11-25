#!/usr/bin/env node
// 把任意 theme-<slug>.css / .js 打包成可导入的 sealchat.ui-package JSON。
// 用法：
//   node pack_theme.mjs --css theme-x.css [--js theme-x.js] --name "名称" \
//     --description "说明" --targets global,observer,embed,log-public \
//     [--capabilities context.read,storage.read] [--runtime-api 2] [--out 输出.json]
// 只做格式层面的校验；完整检查请再运行 check_theme.mjs。
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { basename, dirname, join, resolve } from 'node:path'
import {
  PACKAGE_FORMAT, FORMAT_VERSION, RUNTIME_API_VERSION, MAX_NAME, MAX_DESCRIPTION, MAX_SURFACES,
  SURFACES, CHANNEL_CAPABILITIES,
} from './contract.mjs'

const usage = () => {
  console.error('用法：node pack_theme.mjs --css <file.css> [--js <file.js>] --name <名称> --description <说明> --targets <a,b> [--capabilities <a,b>] [--runtime-api 1|2] [--out <file.json>]')
  process.exit(2)
}

const args = {}
const argv = process.argv.slice(2)
for (let i = 0; i < argv.length; i++) {
  const key = argv[i]
  if (!key.startsWith('--')) usage()
  const value = argv[i + 1]
  if (value === undefined || value.startsWith('--')) usage()
  args[key.slice(2)] = value
  i++
}

const fail = message => { console.error(`ERROR ${message}`); process.exit(1) }
const list = value => (value || '').split(',').map(s => s.trim()).filter(Boolean)

if (!args.css || !args.name || !args.targets) usage()
const cssPath = resolve(args.css)
if (!existsSync(cssPath)) fail(`找不到 CSS 文件：${cssPath}`)
const jsPath = args.js ? resolve(args.js) : ''
if (jsPath && !existsSync(jsPath)) fail(`找不到 JS 文件：${jsPath}`)

const name = args.name.trim()
const description = (args.description || '').trim()
if (!name || [...name].length > MAX_NAME) fail(`名称须为 1～${MAX_NAME} 个字符`)
if ([...description].length > MAX_DESCRIPTION) fail(`说明最多 ${MAX_DESCRIPTION} 个字符，当前 ${[...description].length}`)

const targets = [...new Set(list(args.targets))]
if (targets.length === 0 || targets.length > MAX_SURFACES) fail(`targets 须为 1～${MAX_SURFACES} 项`)
for (const t of targets) {
  if (!SURFACES.includes(t)) fail(`未知 Surface：${t}（可选：${SURFACES.join(', ')}）`)
  if (t === 'internal') fail('internal Surface 永远不会加载 UI Package，不要写入 targets')
}

const capabilities = [...new Set(list(args.capabilities))]
for (const c of capabilities) {
  if (!CHANNEL_CAPABILITIES.includes(c)) fail(`未知频道能力：${c}（可选：${CHANNEL_CAPABILITIES.join(', ')}）`)
}

const runtimeApiVersion = args['runtime-api'] ? Number(args['runtime-api']) : RUNTIME_API_VERSION
if (![1, 2].includes(runtimeApiVersion)) fail('runtime-api 只能是 1 或 2')
if (capabilities.length && runtimeApiVersion < 2) fail('声明 channelCapabilities 需要 runtime-api 2')

const css = readFileSync(cssPath, 'utf8')
const js = jsPath ? readFileSync(jsPath, 'utf8') : ''
if (capabilities.length && !js.trim()) fail('声明了 channelCapabilities 但没有 JS')

const doc = {
  format: PACKAGE_FORMAT,
  formatVersion: FORMAT_VERSION,
  runtimeApiVersion,
  manifest: { name, description },
  targets,
  channelCapabilities: capabilities,
  css,
  js,
}

const slug = basename(cssPath).replace(/^theme-/, '').replace(/\.css$/, '')
const out = resolve(args.out || join(dirname(cssPath), `sealchat-ui-package-${slug}.json`))
writeFileSync(out, `${JSON.stringify(doc, null, 2)}\n`)
const kib = s => (Buffer.byteLength(s, 'utf8') / 1024).toFixed(1)
console.log(`${out}  css ${kib(css)} KiB  js ${kib(js)} KiB  targets ${targets.join(',')}${capabilities.length ? `  capabilities ${capabilities.join(',')}` : ''}`)
