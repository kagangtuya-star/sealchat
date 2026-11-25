#!/usr/bin/env node
// SealChat UI Package 静态检查：格式、安全、可访问性、对比度与原创性提醒。
// 用法：
//   node check_theme.mjs <package.json> [--max-kb 1024] [--json]
//   node check_theme.mjs --css theme-x.css [--js theme-x.js] [--targets a,b] [--capabilities a,b]
// 退出码：存在 ERROR 时为 1。只依赖 Node 内置模块。
import { readFileSync, existsSync } from 'node:fs'
import { resolve } from 'node:path'
import {
  PACKAGE_FORMAT, BUNDLE_FORMAT, FORMAT_VERSION, RUNTIME_API_VERSION, MAX_NAME, MAX_DESCRIPTION,
  MAX_SURFACES, DEFAULT_MAX_CODE_KB, SURFACES, PUBLIC_SURFACES, CHANNEL_CAPABILITIES,
  WRITE_CAPABILITIES, PROVIDER_CAPABILITIES, LEGACY_THEME_VARIABLES,
  PROTECTED_PERFORMANCE_CLASSES, PROTECTED_PERFORMANCE_PROPERTIES,
} from './contract.mjs'

/* ---------- 参数 ---------- */

const argv = process.argv.slice(2)
const opts = { json: false }
const positional = []
for (let i = 0; i < argv.length; i++) {
  const a = argv[i]
  if (a === '--json') opts.json = true
  else if (a.startsWith('--')) { opts[a.slice(2)] = argv[i + 1]; i++ }
  else positional.push(a)
}
if (!positional[0] && !opts.css) {
  console.error('用法：node check_theme.mjs <package.json> | --css <file.css> [--js <file.js>] [--targets a,b] [--capabilities a,b] [--max-kb N] [--json]')
  process.exit(2)
}
const maxKB = Number(opts['max-kb'] || DEFAULT_MAX_CODE_KB)

/* ---------- 收集结果 ---------- */

const findings = []
let scope = ''
const add = (level, area, message) => findings.push({ level, package: scope, area, message })
const ERROR = (a, m) => add('ERROR', a, m)
const WARN = (a, m) => add('WARN', a, m)
const INFO = (a, m) => add('INFO', a, m)
const contrastTables = []

/* ---------- 读取输入 ---------- */

const readText = p => {
  const full = resolve(p)
  if (!existsSync(full)) { console.error(`找不到文件：${full}`); process.exit(2) }
  return readFileSync(full, 'utf8')
}
const list = v => (v || '').split(',').map(s => s.trim()).filter(Boolean)

let packages = []
if (positional[0]) {
  let doc
  try { doc = JSON.parse(readText(positional[0])) } catch (e) {
    console.error(`不是有效 JSON：${e.message}`)
    process.exit(1)
  }
  if (doc && doc.format === BUNDLE_FORMAT) {
    scope = 'bundle'
    if (doc.formatVersion !== FORMAT_VERSION) ERROR('格式', `bundle formatVersion 应为 ${FORMAT_VERSION}`)
    if (!Array.isArray(doc.packages) || doc.packages.length === 0) ERROR('格式', 'bundle 中没有包')
    else if (doc.packages.length > 50) ERROR('格式', 'bundle 最多 50 个包')
    packages = (doc.packages || []).map((p, i) => ({
      ...p,
      format: p.format || PACKAGE_FORMAT,
      formatVersion: p.formatVersion ?? doc.formatVersion,
      runtimeApiVersion: p.runtimeApiVersion ?? doc.runtimeApiVersion,
      __label: `#${i + 1} ${p?.manifest?.name || ''}`.trim(),
    }))
  } else {
    packages = [{ ...doc, __label: doc?.manifest?.name || positional[0] }]
  }
} else {
  packages = [{
    format: PACKAGE_FORMAT,
    formatVersion: FORMAT_VERSION,
    runtimeApiVersion: RUNTIME_API_VERSION,
    manifest: { name: opts.name || '(源码检查)', description: '' },
    targets: list(opts.targets || 'global'),
    channelCapabilities: list(opts.capabilities),
    css: readText(opts.css),
    js: opts.js ? readText(opts.js) : '',
    __label: opts.css,
    __source: true,
  }]
}

/* ---------- 包格式 ---------- */

function checkPackage(p) {
  if (p.format !== PACKAGE_FORMAT) ERROR('格式', `format 应为 ${PACKAGE_FORMAT}`)
  if (p.formatVersion !== FORMAT_VERSION) ERROR('格式', `formatVersion 应为 ${FORMAT_VERSION}`)
  if (p.runtimeApiVersion !== undefined && ![1, 2].includes(p.runtimeApiVersion)) ERROR('格式', 'runtimeApiVersion 只能是 1 或 2')
  if (p.runtimeApiVersion === undefined) INFO('格式', `建议显式写 runtimeApiVersion: ${RUNTIME_API_VERSION}`)

  const name = String(p.manifest?.name || '').trim()
  const description = String(p.manifest?.description || '').trim()
  if (!p.__source) {
    if (!name || [...name].length > MAX_NAME) ERROR('格式', `manifest.name 须为 1～${MAX_NAME} 个字符`)
    if ([...description].length > MAX_DESCRIPTION) ERROR('格式', `manifest.description 最多 ${MAX_DESCRIPTION} 个字符（当前 ${[...description].length}）`)
    if (!description) WARN('格式', 'manifest.description 为空：请写明隐喻、日夜名称和脚本做了什么')
  }

  const targets = Array.isArray(p.targets) ? p.targets : []
  if (targets.length === 0 || targets.length > MAX_SURFACES) ERROR('Surface', `targets 须为 1～${MAX_SURFACES} 项`)
  for (const t of targets) {
    if (!SURFACES.includes(t)) ERROR('Surface', `未知 Surface：${t}`)
    if (t === 'internal') ERROR('Surface', 'internal 永远不加载 UI Package，不要写入 targets')
  }
  if (targets.includes('global')) {
    const missing = PUBLIC_SURFACES.filter(s => !targets.includes(s))
    if (missing.length) INFO('Surface', `global 不覆盖公共 Surface（${missing.join(', ')}）；需要时请显式加入`)
  }

  const caps = Array.isArray(p.channelCapabilities) ? p.channelCapabilities : []
  if (p.channelCapabilities !== undefined && !Array.isArray(p.channelCapabilities)) ERROR('能力', 'channelCapabilities 必须是数组')
  for (const c of caps) if (!CHANNEL_CAPABILITIES.includes(c)) ERROR('能力', `未知频道能力：${c}`)
  const writes = caps.filter(c => WRITE_CAPABILITIES.includes(c))
  if (writes.length) WARN('能力', `申请了写能力 ${writes.join(', ')}：确认确有必要，并在说明中解释用途`)
  const providers = caps.filter(c => PROVIDER_CAPABILITIES.includes(c))
  if (providers.length) INFO('能力', `${providers.join(', ')} 需要宿主注入 Theater provider，普通世界脚本通常不可用`)
  if (caps.length && (p.runtimeApiVersion ?? 1) < 2) ERROR('能力', '声明 channelCapabilities 需要 runtimeApiVersion 2')

  const css = typeof p.css === 'string' ? p.css : ''
  const js = typeof p.js === 'string' ? p.js : ''
  if (typeof p.css !== 'string') ERROR('格式', 'css 必须是字符串')
  if (p.js !== undefined && typeof p.js !== 'string') ERROR('格式', 'js 必须是字符串')
  const bytes = Buffer.byteLength(css, 'utf8') + Buffer.byteLength(js, 'utf8')
  if (bytes > maxKB * 1024) ERROR('体积', `CSS + JS = ${(bytes / 1024).toFixed(1)} KiB，超过 ${maxKB} KiB`)
  else INFO('体积', `CSS ${(Buffer.byteLength(css) / 1024).toFixed(1)} KiB + JS ${(Buffer.byteLength(js) / 1024).toFixed(1)} KiB`)

  if (css.trim()) checkCSS(css)
  else WARN('CSS', 'CSS 为空')
  if (js.trim()) checkJS(js, caps, targets)
  else if (caps.length) ERROR('能力', '声明了 channelCapabilities 但 js 为空')
}

/* ---------- CSS 解析 ---------- */

function stripComments(src) {
  let out = ''
  let quote = ''
  for (let i = 0; i < src.length; i++) {
    const ch = src[i]
    if (quote) {
      out += ch
      if (ch === '\\') { out += src[++i] ?? ''; continue }
      if (ch === quote) quote = ''
      continue
    }
    if (ch === '"' || ch === "'") { quote = ch; out += ch; continue }
    if (ch === '/' && src[i + 1] === '*') {
      const end = src.indexOf('*/', i + 2)
      i = end < 0 ? src.length : end + 1
      out += ' '
      continue
    }
    out += ch
  }
  return out
}

// 返回 { selector, body, at: [...外层 at-rule prelude], index }
function parseRules(src) {
  const rules = []
  const atRules = []
  let index = 0
  const walk = (text, at) => {
    let i = 0
    while (i < text.length) {
      let j = i
      let quote = ''
      let depth = 0
      // 找到下一个顶层 { 或 ;
      for (; j < text.length; j++) {
        const ch = text[j]
        if (quote) { if (ch === '\\') j++; else if (ch === quote) quote = ''; continue }
        if (ch === '"' || ch === "'") { quote = ch; continue }
        if (ch === '(') depth++
        else if (ch === ')') depth--
        else if (depth === 0 && (ch === '{' || ch === ';')) break
      }
      if (j >= text.length) break
      const prelude = text.slice(i, j).trim()
      if (text[j] === ';') { if (prelude.startsWith('@')) atRules.push({ prelude, at }); i = j + 1; continue }
      // 匹配 }
      let k = j + 1
      let level = 1
      quote = ''
      for (; k < text.length; k++) {
        const ch = text[k]
        if (quote) { if (ch === '\\') k++; else if (ch === quote) quote = ''; continue }
        if (ch === '"' || ch === "'") { quote = ch; continue }
        if (ch === '{') level++
        else if (ch === '}') { level--; if (level === 0) break }
      }
      const body = text.slice(j + 1, k)
      if (prelude.startsWith('@')) {
        const name = prelude.split(/[\s(]/)[0].toLowerCase()
        if (['@media', '@supports', '@layer', '@container', '@scope', '@document'].includes(name)) walk(body, [...at, prelude])
        else atRules.push({ prelude, body, at })
      } else {
        rules.push({ selector: prelude, body, at, index: index++ })
      }
      i = k + 1
    }
  }
  walk(src, [])
  return { rules, atRules }
}

function splitTopLevel(text, sep) {
  const parts = []
  let quote = ''
  let depth = 0
  let start = 0
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (quote) { if (ch === '\\') i++; else if (ch === quote) quote = ''; continue }
    if (ch === '"' || ch === "'") { quote = ch; continue }
    if (ch === '(' || ch === '[') depth++
    else if (ch === ')' || ch === ']') depth--
    else if (ch === sep && depth === 0) { parts.push(text.slice(start, i)); start = i + 1 }
  }
  parts.push(text.slice(start))
  return parts
}

function declarations(body) {
  const out = []
  // 嵌套规则（CSS nesting）不展开，只取本层声明
  for (const raw of splitTopLevel(body.replace(/\{[^{}]*\}/g, ';'), ';')) {
    const at = raw.indexOf(':')
    if (at < 0) continue
    const prop = raw.slice(0, at).trim()
    if (!/^(--)?[a-zA-Z-]+$/.test(prop)) continue
    let value = raw.slice(at + 1).trim()
    const important = /!\s*important\s*$/i.test(value)
    value = value.replace(/!\s*important\s*$/i, '').trim()
    out.push({ prop: prop.startsWith('--') ? prop : prop.toLowerCase(), value, important })
  }
  return out
}

const stripNot = selector => selector.replace(/:not\((?:[^()]|\([^()]*\))*\)/g, '')

/* ---------- 颜色与对比度 ---------- */

const NAMED = {
  transparent: [0, 0, 0, 0], white: [255, 255, 255, 1], black: [0, 0, 0, 1],
  red: [255, 0, 0, 1], gray: [128, 128, 128, 1], grey: [128, 128, 128, 1],
}

function parseColor(value) {
  const v = value.trim().toLowerCase()
  if (NAMED[v]) return NAMED[v]
  let m = /^#([0-9a-f]{3,8})$/.exec(v)
  if (m) {
    const h = m[1]
    if (h.length === 3 || h.length === 4) {
      const c = [...h].map(x => parseInt(x + x, 16))
      return [c[0], c[1], c[2], h.length === 4 ? c[3] / 255 : 1]
    }
    if (h.length === 6 || h.length === 8) {
      const c = [0, 2, 4, 6].map(i => parseInt(h.slice(i, i + 2), 16))
      return [c[0], c[1], c[2], h.length === 8 ? c[3] / 255 : 1]
    }
    return null
  }
  m = /^(rgba?|hsla?)\((.*)\)$/.exec(v)
  if (!m) return null
  const parts = m[2].replace(/\s*\/\s*/, ' / ').split(/[\s,]+/).filter(x => x && x !== '/')
  if (parts.length < 3) return null
  const num = (s, scale) => s.endsWith('%') ? parseFloat(s) / 100 * scale : parseFloat(s)
  const alpha = parts[3] !== undefined ? num(parts[3], 1) : 1
  if (m[1].startsWith('rgb')) {
    const c = parts.slice(0, 3).map(s => num(s, 255))
    if (c.some(Number.isNaN) || Number.isNaN(alpha)) return null
    return [...c, alpha]
  }
  const h = parseFloat(parts[0]) / 360
  const s = num(parts[1], 1)
  const l = num(parts[2], 1)
  if ([h, s, l, alpha].some(Number.isNaN)) return null
  const hue = t => {
    t = ((t % 1) + 1) % 1
    const q = l < 0.5 ? l * (1 + s) : l + s - l * s
    const p = 2 * l - q
    if (t < 1 / 6) return p + (q - p) * 6 * t
    if (t < 1 / 2) return q
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6
    return p
  }
  return [hue(h + 1 / 3) * 255, hue(h) * 255, hue(h - 1 / 3) * 255, alpha]
}

const over = (top, bottom) => {
  const a = top[3] + bottom[3] * (1 - top[3])
  if (a === 0) return [0, 0, 0, 0]
  return [0, 1, 2].map(i => (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / a).concat(a)
}
const luminance = c => {
  const [r, g, b] = c.slice(0, 3).map(x => {
    const s = x / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
const ratio = (a, b) => {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (l1 + 0.05) / (l2 + 0.05)
}

function resolveVar(value, vars, depth = 0) {
  if (depth > 20) return null
  let out = ''
  let i = 0
  while (i < value.length) {
    const at = value.indexOf('var(', i)
    if (at < 0) { out += value.slice(i); break }
    out += value.slice(i, at)
    let level = 0
    let j = at + 3
    for (; j < value.length; j++) {
      if (value[j] === '(') level++
      else if (value[j] === ')') { level--; if (level === 0) break }
    }
    const inner = value.slice(at + 4, j)
    const comma = splitTopLevel(inner, ',')
    const name = comma[0].trim()
    const fallback = comma.length > 1 ? comma.slice(1).join(',').trim() : null
    let resolved = vars.has(name) ? resolveVar(vars.get(name), vars, depth + 1) : null
    if (resolved === null && fallback !== null) resolved = resolveVar(fallback, vars, depth + 1)
    if (resolved === null) return null
    out += resolved
    i = j + 1
  }
  return out.trim()
}

// 把根级令牌块按日 / 夜分别合并（考虑特异性与源码顺序）
function tokenMaps(rules) {
  const blocks = []
  for (const rule of rules) {
    if (rule.at.length) continue
    for (const raw of splitTopLevel(rule.selector, ',')) {
      const sel = raw.trim().replace(/\s+/g, '')
      const m = /^(:root|html)((?:\[[^\]]+\])*)$/.exec(sel)
      if (!m) continue
      const attrs = [...m[2].matchAll(/\[([a-z-]+)(?:=['"]?([^'"\]]+)['"]?)?\]/g)]
      let palette = null
      let ok = true
      for (const [, name, val] of attrs) {
        if (name === 'data-sc-surface' && !val) continue
        if (name === 'data-sc-palette' && (val === 'day' || val === 'night')) { palette = val; continue }
        ok = false
      }
      if (!ok) continue
      const specificity = (m[1] === ':root' ? 10 : 1) + attrs.length * 10
      blocks.push({ palette, specificity, index: rule.index, decls: declarations(rule.body) })
    }
  }
  const build = palette => {
    const vars = new Map()
    blocks
      .filter(b => b.palette === null || b.palette === palette)
      .sort((a, b) => a.specificity - b.specificity || a.index - b.index)
      .forEach(b => b.decls.forEach(d => { if (d.prop.startsWith('--')) vars.set(d.prop, d.value) }))
    return vars
  }
  return { day: build('day'), night: build('night'), hasDay: blocks.some(b => b.palette === 'day'), hasNight: blocks.some(b => b.palette === 'night'), hasBase: blocks.some(b => b.palette === null) }
}

const CONTRAST_PAIRS = [
  // [前景, 背景, 最低值, 不达标级别, 说明]
  ['--sc-text-primary', '--sc-bg-surface', 4.5, 'ERROR', '主要文字 / 页面卡片'],
  ['--sc-text-primary', '--sc-bg-elevated', 4.5, 'ERROR', '主要文字 / 浮层'],
  ['--sc-text-primary', '--sc-bg-input', 4.5, 'ERROR', '主要文字 / 输入框'],
  ['--sc-text-primary', '--sc-bg-header', 4.5, 'ERROR', '主要文字 / 顶栏'],
  ['--sc-text-primary', '--sc-bg-sidebar', 4.5, 'ERROR', '主要文字 / 侧栏'],
  ['--chat-text-primary', '--sc-bg-surface', 4.5, 'ERROR', '聊天正文 / 卡片'],
  ['--chat-text-primary', '--chat-ic-bg', 4.5, 'ERROR', '聊天正文 / 场内底色'],
  ['--chat-text-primary', '--chat-ooc-bg', 4.5, 'ERROR', '聊天正文 / 场外底色'],
  ['--sc-text-secondary', '--sc-bg-surface', 4.5, 'WARN', '次要文字 / 卡片'],
  ['--chat-text-secondary', '--sc-bg-surface', 4.5, 'WARN', '聊天次要文字 / 卡片'],
  ['--chat-inline-code-fg', '--chat-inline-code-bg', 4.5, 'WARN', '行内代码'],
  ['--chat-whisper-tag-fg', '--chat-whisper-tag-bg', 4.5, 'WARN', '悄悄话标签'],
  ['--sc-badge-text', '--sc-badge-bg', 4.5, 'WARN', '徽标'],
  ['--sc-action-ribbon-hover-text', '--primary-color', 4.5, 'WARN', '主色按钮文字'],
  ['--primary-color', '--sc-bg-surface', 3, 'WARN', '主色 / 卡片（非文字元素）'],
]

function checkContrast(maps) {
  for (const palette of ['day', 'night']) {
    const vars = maps[palette]
    const fallback = palette === 'day' ? [255, 255, 255, 1] : [0, 0, 0, 1]
    const color = name => {
      if (!vars.has(name)) return { missing: true }
      const resolved = resolveVar(`var(${name})`, vars)
      const c = resolved === null ? null : parseColor(resolved)
      return c ? { c } : { raw: resolved ?? vars.get(name) }
    }
    // 底色链：目标背景 → 卡片 → 页面 → 估计值
    const backdrop = (() => {
      let approx = false
      let base = fallback
      for (const name of ['--sc-bg-page', '--sc-bg-surface']) {
        const r = color(name)
        if (r.c) base = over(r.c, base)
        else approx = true
      }
      return { base, approx }
    })()
    const rows = []
    for (const [fgName, bgName, min, level, label] of CONTRAST_PAIRS) {
      const fg = color(fgName)
      const bg = color(bgName)
      if (fg.missing || bg.missing) {
        rows.push({ label, fg: fgName, bg: bgName, result: '未定义（沿用 SealChat 默认）' })
        continue
      }
      if (!fg.c || !bg.c) {
        rows.push({ label, fg: fgName, bg: bgName, result: `无法静态解析（${!fg.c ? fg.raw : bg.raw}），请在浏览器中确认` })
        continue
      }
      const bgSolid = bg.c[3] < 1 ? over(bg.c, backdrop.base) : bg.c
      const fgSolid = fg.c[3] < 1 ? over(fg.c, bgSolid) : fg.c
      const value = ratio(fgSolid, bgSolid)
      const approx = (bg.c[3] < 1 && backdrop.approx) ? '（底色含估计）' : ''
      rows.push({ label, fg: fgName, bg: bgName, result: `${value.toFixed(2)}${approx}`, value, min })
      if (value < min) {
        const severity = level === 'WARN' && value < 3 && min >= 4.5 ? 'ERROR' : level
        add(severity, '对比度', `${palette === 'day' ? '日间' : '夜间'} ${label}：${fgName} 对 ${bgName} 为 ${value.toFixed(2)}，低于 ${min}${approx}`)
      }
    }
    contrastTables.push({ package: scope, palette, rows })
  }
}

/* ---------- CSS 检查 ---------- */

function checkCSS(raw) {
  const css = stripComments(raw)
  const { rules, atRules } = parseRules(css)

  // 保留关键帧
  for (const a of atRules) {
    if (/^@(-webkit-)?keyframes\s+sc-performance-/i.test(a.prelude)) ERROR('受保护', `不要声明保留关键帧：${a.prelude}`)
    if (/^@import/i.test(a.prelude)) {
      if (/(https?:)?\/\//i.test(a.prelude)) ERROR('远程资源', `不要远程 @import：${a.prelude.slice(0, 120)}`)
      else WARN('远程资源', `UI Package 中的 @import 无法引用包外文件：${a.prelude.slice(0, 120)}`)
    }
    if (/^@font-face/i.test(a.prelude) && /url\(\s*['"]?(https?:)?\/\//i.test(a.body || '')) ERROR('远程资源', '@font-face 引用了远程字体，请改用本机字体栈')
  }
  for (const m of css.matchAll(/url\(\s*['"]?\s*((?:https?:)?\/\/[^'")\s]+)/gi)) {
    ERROR('远程资源', `不要加载远程资源：${m[1].slice(0, 120)}`)
  }

  // 受保护的富文本演出节点
  const protectedRe = new RegExp(`\\.(${PROTECTED_PERFORMANCE_CLASSES.join('|').replace(/\./g, '\\.')})(?![\\w-])`)
  for (const rule of rules) {
    const reduced = rule.at.some(a => /prefers-reduced-motion/.test(a))
    for (const sel of splitTopLevel(rule.selector, ',')) {
      const bare = stripNot(sel)
      if (protectedRe.test(bare)) {
        for (const d of declarations(rule.body)) {
          if (PROTECTED_PERFORMANCE_PROPERTIES.test(d.prop)) ERROR('受保护', `${sel.trim()} 覆盖了受保护属性 ${d.prop}`)
        }
      }
      // 全局通配规则
      if (/(^|[\s>+~])\*(?![\w-])/.test(sel) || /^\s*\*/.test(sel)) {
        const decls = declarations(rule.body)
        const touches = decls.filter(d => /^(animation|transition)/.test(d.prop))
        if (touches.length) {
          const excluded = PROTECTED_PERFORMANCE_CLASSES.every(c => sel.includes(`:not(.${c})`))
          if (!excluded) {
            if (reduced) WARN('减少动画', `通配规则 ${sel.trim()} 会影响富文本演出节点，请加上 :not(.tiptap-performance):not(.twin-layer-message__char):not(.twin-layer-message__char-glyph)`)
            else WARN('受保护', `通配规则 ${sel.trim()} 修改了动画或过渡，会波及富文本演出节点`)
          }
        }
      }
    }
  }

  // 日夜
  const maps = tokenMaps(rules)
  if (!maps.hasDay && !maps.hasNight) WARN('日夜', "没有 :root[data-sc-palette='day'|'night'] 令牌块，日夜两种配色会共用一套颜色")
  else if (!maps.hasDay || !maps.hasNight) INFO('日夜', `只写了 ${maps.hasDay ? '日间' : '夜间'} 配色块；另一配色由共同令牌块承担，请确认两者都完整`)

  // 21 个传统颜色字段
  const defined = new Set(rules.flatMap(r => declarations(r.body).map(d => d.prop)).filter(p => p.startsWith('--')))
  const missingLegacy = LEGACY_THEME_VARIABLES.filter(v => !defined.has(v))
  if (missingLegacy.length) WARN('变量', `未覆盖传统颜色字段变量：${missingLegacy.join(', ')}`)

  // 减少动画
  const animates = /(^|[;{\s])(animation|animation-name|transition)\s*:/m.test(css) || atRules.some(a => /keyframes/i.test(a.prelude))
  if (animates && !/prefers-reduced-motion/.test(css)) WARN('减少动画', '使用了动画或过渡，但没有 @media (prefers-reduced-motion: reduce)')

  // 焦点
  const killsOutline = rules.some(r => declarations(r.body).some(d => d.prop === 'outline' && /^(none|0)$/.test(d.value)))
  if (killsOutline && !/:focus-visible/.test(css)) WARN('焦点', '存在 outline: none，但没有任何 :focus-visible 样式')

  // !important 与选择器稳定性
  const importantCount = (css.match(/!\s*important/gi) || []).length
  if (importantCount > 300) WARN('稳定性', `!important 共 ${importantCount} 处，优先改变量或调整包顺序`)
  else INFO('稳定性', `!important 共 ${importantCount} 处`)
  for (const rule of rules) {
    const sel = rule.selector
    if (/\.n-[\w-]+\s*>\s*\.n-[\w-]+\s*>/.test(sel)) WARN('稳定性', `依赖 Naive UI 深层结构：${sel.trim().slice(0, 120)}`)
    if (/\.mx-context-menu|\.v-binder-follower|\.n-dropdown-option-body|\[data-v-[0-9a-f]+/.test(sel)) WARN('稳定性', `使用了不推荐的内部选择器：${sel.trim().slice(0, 120)}`)
    for (const d of declarations(rule.body)) {
      if (d.prop === 'width' || d.prop === 'min-width') {
        const px = /^(\d+(?:\.\d+)?)px$/.exec(d.value)
        if (px && Number(px[1]) > 640) WARN('窄屏', `${sel.trim().slice(0, 80)} 写死 ${d.prop}: ${d.value}，窄屏会溢出`)
      }
      if (d.prop === 'display' && d.value === 'none' && !/::?(before|after)\s*$/.test(sel.trim()) && /(alert|error|warning|sign-?in|login|trust|captcha|permission)/i.test(sel)) {
        WARN('可用性', `不要隐藏错误、登录、信任或权限提示：${sel.trim().slice(0, 120)}`)
      }
    }
  }

  // 原创性提醒
  const borrowed = new Set()
  for (const m of css.matchAll(/(--|\.|data-)(nr|rl|ta)-[\w-]+|sc-api-lab[\w-]*/g)) borrowed.add(m[0])
  if (borrowed.size) WARN('原创性', `使用了参考主题的私有命名：${[...borrowed].slice(0, 8).join(', ')}${borrowed.size > 8 ? ' …' : ''}`)

  checkContrast(maps)
}

/* ---------- JS 检查 ---------- */

function checkJS(js, caps, targets) {
  const code = js.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:\\])\/\/.*$/gm, '$1')
  const network = [
    [/\bfetch\s*\(/, 'fetch'], [/XMLHttpRequest/, 'XMLHttpRequest'], [/new\s+WebSocket\b/, 'WebSocket'],
    [/sendBeacon/, 'sendBeacon'], [/new\s+EventSource\b/, 'EventSource'], [/\bimport\s*\(/, '动态 import'],
  ]
  for (const [re, name] of network) if (re.test(code)) ERROR('脚本', `主题脚本不应发网络请求或加载外部模块：${name}`)
  if (/\beval\s*\(|new\s+Function\s*\(/.test(code)) ERROR('脚本', '不要使用 eval / new Function')

  const raw = [
    [/(?<!sc\.listen\([^)]*)\.addEventListener\s*\(/, 'addEventListener', 'sc.listen'],
    [/(?<![\w.]sc\.)\b(?:window\.)?setTimeout\s*\(/, 'setTimeout', 'sc.timeout'],
    [/(?<![\w.]sc\.)\b(?:window\.)?setInterval\s*\(/, 'setInterval', 'sc.interval'],
    [/new\s+MutationObserver\b/, 'MutationObserver', 'sc.observe'],
  ]
  for (const [re, name, alt] of raw) if (re.test(code)) WARN('脚本', `直接使用 ${name}：改用 ${alt}，或确认已在 sc.cleanup 中释放`)
  if (/requestAnimationFrame/.test(code) && !/cancelAnimationFrame/.test(code)) WARN('脚本', 'requestAnimationFrame 没有对应的 cancelAnimationFrame 清理')
  if (/createElement\s*\(/.test(code) && !/sc\.cleanup\s*\(/.test(code)) WARN('脚本', '创建了 DOM 元素但没有 sc.cleanup 移除')
  if (!/prefers-reduced-motion/.test(code)) WARN('减少动画', '脚本没有读取 prefers-reduced-motion；有动效时必须在减少动画下停用')
  if (/innerHTML\s*\+?=\s*(`[^`]*\$\{|[^;\n]*\+\s*[\w.]+)/.test(code)) WARN('脚本', 'innerHTML 中拼接了变量；不要把消息或用户内容拼进 HTML，改用 textContent')
  if (/\.(textContent|innerText|nodeValue)\s*=/.test(code) && /\[data-sc-ui=["']message["']\][^;\n]*\.(textContent|innerText)\s*=/.test(code)) WARN('脚本', '疑似改写消息文本')
  if (/^\s*(import|export)\s/m.test(code) && !/export\s+default/.test(code)) WARN('脚本', 'ES module 写法需要 export default (sc) => {...}')
  if (caps.length && !/sc\.channel\s*\(/.test(code)) WARN('能力', '声明了 channelCapabilities，但脚本没有调用 sc.channel()')
  if (!caps.length && /sc\.channel\s*\(/.test(code)) WARN('能力', '调用了 sc.channel() 但没有声明 channelCapabilities，effective capabilities 将为空')
  if (/sc\.channel\s*\(/.test(code) && targets.some(t => PUBLIC_SURFACES.includes(t))) INFO('能力', '公共 Surface 不运行世界脚本，sc.channel() 只在登录频道上下文可用')
  const storageKeys = [...code.matchAll(/(?:local|session)Storage\.(?:get|set|remove)Item\(\s*['"`]([^'"`]+)/g)].map(m => m[1])
  if (storageKeys.some(k => k.length < 8 || !/[.:_-]/.test(k))) WARN('脚本', `浏览器存储 key 过于通用：${storageKeys.join(', ')}；请加主题前缀`)
  const borrowed = new Set()
  for (const m of code.matchAll(/['"`.](nr|rl|ta)-[\w-]+|sc-api-lab[\w-]*/g)) borrowed.add(m[0].replace(/^['"`.]/, ''))
  if (borrowed.size) WARN('原创性', `脚本使用了参考主题的私有命名：${[...borrowed].slice(0, 8).join(', ')}`)
}

/* ---------- 运行 ---------- */

for (const p of packages) {
  scope = p.__label
  checkPackage(p)
}

const counts = { ERROR: 0, WARN: 0, INFO: 0 }
for (const f of findings) counts[f.level]++

if (opts.json) {
  console.log(JSON.stringify({ counts, findings, contrast: contrastTables }, null, 2))
} else {
  for (const level of ['ERROR', 'WARN', 'INFO']) {
    const items = findings.filter(f => f.level === level)
    if (!items.length) continue
    console.log(`\n== ${level} (${items.length}) ==`)
    for (const f of items) console.log(`${level.padEnd(5)} [${f.area}]${packages.length > 1 ? ` <${f.package}>` : ''} ${f.message}`)
  }
  for (const t of contrastTables) {
    if (t.rows.every(r => r.value === undefined && r.result.startsWith('未定义'))) {
      console.log(`\n== 对比度 · ${t.palette === 'day' ? '日间' : '夜间'}${packages.length > 1 ? ` <${t.package}>` : ''} ==\n   未定义任何检查用变量，全部沿用 SealChat 默认`)
      continue
    }
    console.log(`\n== 对比度 · ${t.palette === 'day' ? '日间' : '夜间'}${packages.length > 1 ? ` <${t.package}>` : ''} ==`)
    for (const r of t.rows) {
      const mark = r.value === undefined ? '  ' : r.value >= r.min ? 'ok' : '!!'
      console.log(`${mark} ${r.label.padEnd(14)} ${r.result}${r.min ? `  (≥${r.min})` : ''}`)
    }
  }
  console.log(`\n合计：${counts.ERROR} ERROR，${counts.WARN} WARN，${counts.INFO} INFO`)
}
process.exit(counts.ERROR ? 1 : 0)
