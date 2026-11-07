// TRIANGLE AGENCY 三角机构 —— 配套脚本
// 只向 body 追加装饰与 HUD，并给消息行写入 data-ta-* 标记；
// 不移动、不改写 SealChat 的 DOM 文本，不发网络请求。
// 混沌计数只保存在本浏览器的 localStorage，按频道区分，与游戏数据无关。

const doc = document
const root = doc.documentElement
const THEME = 'triangle-agency'
const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
const QUIET_SURFACES = new Set(['embed', 'log-public', 'theater'])
const HUD_SURFACES = new Set(['chat', 'split'])
const CHAOS_PREFIX = 'sealchat.theme.triangle-agency.chaos.'
// 6d4 等 d4 骰池：取「=」之后的各骰点数，如 [6d4=3+1+2+4+3+1] 或 6d4=[3,1,2,4,3,1]
const D4_POOL = /(\d*)d4\s*=\s*\[?\s*([1-4](?:\s*[+,，、\s]\s*[1-4])+)/i
const MEMOS = [
  ['质量保证部', '一切正常。请继续保持一切正常。'],
  ['设施管理', '四楼茶水间的门今天朝内开。请勿纠正它。'],
  ['人力资源部', '如果你记得同事 #0417，请立即停止记得。'],
  ['考勤系统', '您本周的工时已超过本周的长度。'],
  ['安保科', '走廊尽头的第五扇门不存在。请勿敲门。'],
  ['总务处', '今日的天空已通过审核。'],
  ['现实稳定办公室', '提醒：保持现实稳定是每位员工的共同责任。'],
  ['档案室', '本条备忘从未发送。'],
]

root.dataset.scTheme = THEME
sc.cleanup(() => { if (root.dataset.scTheme === THEME) delete root.dataset.scTheme })

const make = (tag, className, parent = doc.body) => {
  const el = doc.createElement(tag)
  el.className = className
  parent.appendChild(el)
  sc.cleanup(() => el.remove())
  return el
}
const rand = (min, max) => min + Math.random() * (max - min)
const pad = n => String(n).padStart(2, '0')

/* ---------- 顶栏工牌 HUD ---------- */

const hud = make('div', 'ta-hud')
hud.innerHTML = [
  '<span class="ta-hud__mark" aria-hidden="true"></span>',
  '<span class="ta-hud__dept">三角机构 · 外勤部</span>',
  '<span class="ta-hud__sep" aria-hidden="true"></span>',
  '<span class="ta-hud__clock">--:--</span>',
  '<span class="ta-hud__sep" aria-hidden="true"></span>',
  '<span class="ta-hud__chaos" title="本频道累计混沌（仅本浏览器）。双击数字编辑。">',
  '<span>混沌 CHAOS</span><b>00</b>',
  '<button type="button" data-step="-1" aria-label="混沌减一">−</button>',
  '<button type="button" data-step="1" aria-label="混沌加一">+</button>',
  '</span>',
].join('')
const clock = hud.querySelector('.ta-hud__clock')
const chaosBox = hud.querySelector('.ta-hud__chaos')
const chaosValue = chaosBox.querySelector('b')

const tick = () => {
  const now = new Date()
  clock.textContent = `${pad(now.getHours())}:${pad(now.getMinutes())}`
}
tick()
sc.interval(tick, 15000)

/* ---------- 混沌计数（按频道） ---------- */

const chaosKey = () => CHAOS_PREFIX + (sc.context().channelId || 'none')
const readChaos = () => {
  try {
    const value = Number(window.localStorage.getItem(chaosKey()))
    return Number.isFinite(value) && value > 0 ? Math.floor(value) : 0
  } catch { return 0 }
}
let chaos = readChaos()

const renderChaos = (bump = false) => {
  chaosValue.textContent = pad(chaos)
  if (bump && !reduceMotion) {
    chaosBox.classList.remove('is-bump')
    void chaosBox.offsetWidth
    chaosBox.classList.add('is-bump')
  }
}

const setChaos = (next, bump = false) => {
  chaos = Math.max(0, Math.min(999, Math.round(next)))
  try { window.localStorage.setItem(chaosKey(), String(chaos)) } catch { /* 隐私模式 */ }
  renderChaos(bump)
}
renderChaos()

sc.listen(chaosBox, 'click', event => {
  const button = event.target instanceof Element ? event.target.closest('button[data-step]') : null
  if (button) setChaos(chaos + Number(button.dataset.step), true)
})

let editingChaos = false
const finishChaosEdit = () => {
  editingChaos = false
  chaosValue.removeAttribute('contenteditable')
  chaosValue.removeAttribute('data-editing')
  chaosValue.removeAttribute('role')
}
const commitChaosEdit = () => {
  if (!editingChaos) return
  const raw = chaosValue.textContent.trim()
  finishChaosEdit()
  if (/^\d+$/.test(raw)) setChaos(Number(raw), true)
  else renderChaos()
}
const cancelChaosEdit = () => {
  if (!editingChaos) return
  finishChaosEdit()
  renderChaos()
}
sc.listen(chaosValue, 'dblclick', event => {
  event.preventDefault()
  if (editingChaos) return
  editingChaos = true
  chaosValue.setAttribute('contenteditable', 'true')
  chaosValue.dataset.editing = 'true'
  chaosValue.setAttribute('role', 'textbox')
  chaosValue.textContent = String(chaos)
  chaosValue.focus()
  const selection = doc.getSelection()
  if (selection) {
    const range = doc.createRange()
    range.selectNodeContents(chaosValue)
    selection.removeAllRanges()
    selection.addRange(range)
  }
})
sc.listen(chaosValue, 'keydown', event => {
  if (!editingChaos) return
  if (event.key === 'Enter') {
    event.preventDefault()
    commitChaosEdit()
  } else if (event.key === 'Escape') {
    event.preventDefault()
    cancelChaosEdit()
  }
})
sc.listen(chaosValue, 'blur', commitChaosEdit)

/* ---------- Surface / 频道切换 ---------- */

const renderSurface = () => {
  hud.hidden = !HUD_SURFACES.has(sc.context().surface)
}
renderSurface()
sc.on('surface-change', renderSurface)
sc.on('channel-change', () => { chaos = readChaos(); renderChaos() })

/* ---------- 6d4 判定解析 ---------- */

const rollTexts = row => {
  const texts = []
  for (const chip of row.querySelectorAll('.dice-chip')) {
    texts.push(chip.dataset.diceResultDetail || chip.textContent || '')
  }
  const content = row.querySelector('.content')
  if (content) texts.push(content.textContent || '')
  return texts
}

const analyse = row => {
  for (const text of rollTexts(row)) {
    const match = D4_POOL.exec(text)
    if (!match) continue
    const dice = match[2].split(/[^1-4]+/).filter(Boolean).map(Number)
    if (dice.length < 2) continue
    const threes = dice.filter(value => value === 3).length
    const gained = dice.length - threes
    const short = `▲${threes} · +${gained}`
    if (threes === 3) return {
      kind: 'triscendence',
      gained: 0,
      short: '▲▲▲',
      label: '▲▲▲ 三重升华',
    }
    if (threes > 0) return { kind: 'success', gained, short, label: `▲×${threes} 成功 · 混沌 +${gained}` }
    return { kind: 'fail', gained, short, label: `失败 · 混沌 +${gained}` }
  }
  return null
}

const markRow = row => {
  const signature = String((row.textContent || '').length)
  if (row.dataset.taSig === signature) return
  row.dataset.taSig = signature
  const result = analyse(row)
  if (result) {
    row.dataset.taRoll = result.kind
    row.dataset.taLabel = result.label
    row.dataset.taShort = result.short
  } else {
    delete row.dataset.taRoll
    delete row.dataset.taLabel
    delete row.dataset.taShort
  }
}

const scanRows = () => {
  for (const row of doc.querySelectorAll('[data-sc-ui="message-list"] [data-sc-ui="message"]')) markRow(row)
}

let scanQueued = false
const queueScan = () => {
  if (scanQueued) return
  scanQueued = true
  sc.timeout(() => { scanQueued = false; scanRows() }, 200)
}
scanRows()
sc.observe(doc.body, { childList: true, subtree: true }, queueScan)
sc.on('message-updated', queueScan)
sc.cleanup(() => {
  for (const row of doc.querySelectorAll('[data-ta-sig]')) {
    delete row.dataset.taSig
    delete row.dataset.taRoll
    delete row.dataset.taLabel
    delete row.dataset.taShort
  }
})

// 新判定：把未出 3 的骰子计入本频道混沌；三重升华不增加混沌，同一条消息只计一次
const counted = new Set()
const countRoll = (messageId, attempt = 0) => {
  const row = doc.querySelector(`[data-sc-ui="message"][data-message-id="${CSS.escape(messageId)}"]`)
  if (!row || !row.querySelector('.content')) {
    if (attempt < 8) sc.timeout(() => countRoll(messageId, attempt + 1), 120)
    return
  }
  const result = analyse(row)
  if (!result || counted.has(messageId)) return
  counted.add(messageId)
  if (result.gained > 0) setChaos(chaos + result.gained, true)
}

sc.on('message-created', payload => {
  queueScan()
  if (!payload || !payload.messageId) return
  const ctx = sc.context()
  if (payload.channelId && payload.channelId !== ctx.channelId) return
  countRoll(String(payload.messageId))
})

/* ---------- 现实偏移与内部备忘（极低频率） ---------- */

const scheduleShift = () => {
  sc.timeout(() => {
    if (!doc.hidden && !hud.hidden) {
      hud.classList.add('is-shifted')
      sc.timeout(() => hud.classList.remove('is-shifted'), 1200)
    }
    scheduleShift()
  }, rand(120000, 300000))
}

const showMemo = () => {
  const [from, text] = MEMOS[Math.floor(Math.random() * MEMOS.length)]
  for (const old of doc.querySelectorAll('.ta-memo')) old.remove()
  const memo = make('div', 'ta-memo')
  memo.setAttribute('aria-hidden', 'true')
  const small = doc.createElement('small')
  small.textContent = `内部备忘 · ${from}`
  memo.append(small, text)
  sc.timeout(() => memo.remove(), 7200)
}

const scheduleMemo = (first = false) => {
  sc.timeout(() => {
    if (!doc.hidden && HUD_SURFACES.has(sc.context().surface)) showMemo()
    scheduleMemo()
  }, first ? rand(180000, 360000) : rand(360000, 840000))
}

if (!reduceMotion) {
  scheduleShift()
  scheduleMemo(true)
}

// 安静 surface 不显示备忘
sc.on('surface-change', () => {
  if (QUIET_SURFACES.has(sc.context().surface)) {
    for (const memo of doc.querySelectorAll('.ta-memo')) memo.remove()
  }
})
