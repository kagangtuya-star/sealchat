// R'LYEH ARCHIVE 拉莱耶档案馆 —— 配套脚本（纯装饰）
// 只添加 aria-hidden 的装饰层，并给消息行写入 data-rl-* 标记；
// 不移动、不改写 SealChat 的 DOM 文本，不发网络请求。
// 「理智值」只是本标签页内的氛围效果，保存在 sessionStorage，与游戏数据无关。

const doc = document
const root = doc.documentElement
const THEME = 'rlyeh-archive'
const SVG_NS = 'http://www.w3.org/2000/svg'
const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
const QUIET_SURFACES = new Set(['embed', 'log-public', 'theater'])
const SANITY_SURFACES = new Set(['chat', 'split'])
const SAN_KEY = 'sealchat.theme.rlyeh-archive.san'
const SAN_MAX = 99

const ELDRITCH = /克苏鲁|cthulhu|拉莱耶|r'?lyeh|ph'?nglui|fhtagn|古神|旧日支配者|外神|深潜者|触手|奈亚拉托提普|nyarlathotep|阿撒托斯|azathoth|犹格|yog-?sothoth|死灵之书|necronomicon|不可名状|疯狂/i
const D100 = /d100\s*[=:：]\s*(\d{1,3})/i
const WHISPERS = [
  "Ph'nglui mglw'nafh Cthulhu R'lyeh wgah'nagl fhtagn",
  '它在沉没之城中做梦',
  '不要回头看书架的阴影',
  '那不是潮声',
  '门后的东西记住了你的名字',
  '星辰的位置……快要正确了',
  'Iä! Iä!',
  '你确定刚才那句话是你写的吗？',
]

root.dataset.scTheme = THEME
sc.cleanup(() => {
  if (root.dataset.scTheme === THEME) delete root.dataset.scTheme
  delete root.dataset.rlMadness
})

const make = (tag, className, parent = doc.body) => {
  const el = doc.createElement(tag)
  el.className = className
  el.setAttribute('aria-hidden', 'true')
  parent.appendChild(el)
  sc.cleanup(() => el.remove())
  return el
}
const rand = (min, max) => min + Math.random() * (max - min)

/* ---------- 雾气与触须 ---------- */

const fogs = reduceMotion ? [] : [make('div', 'rl-fog'), make('div', 'rl-fog rl-fog--slow')]

// 以中心线加锥形宽度生成一条触须轮廓
const tentaclePath = (baseX, height, bend, width, curl) => {
  const steps = 28
  const centre = []
  for (let i = 0; i <= steps; i += 1) {
    const t = i / steps
    const x = baseX + Math.sin(t * Math.PI * 1.25) * bend * t + curl * t * t * t
    const y = 400 - t * height - Math.max(0, t - 0.85) * curl * 0.6
    centre.push([x, y, (1 - t) * (width / 2) + 0.8])
  }
  const left = []
  const right = []
  centre.forEach(([x, y, w], i) => {
    const [ax, ay] = centre[Math.max(0, i - 1)]
    const [bx, by] = centre[Math.min(steps, i + 1)]
    const len = Math.hypot(bx - ax, by - ay) || 1
    const nx = -(by - ay) / len
    const ny = (bx - ax) / len
    left.push(`${(x + nx * w).toFixed(1)} ${(y + ny * w).toFixed(1)}`)
    right.unshift(`${(x - nx * w).toFixed(1)} ${(y - ny * w).toFixed(1)}`)
  })
  return `M${left.join(' L')} L${right.join(' L')} Z`
}

const buildTendrils = side => {
  const svg = doc.createElementNS(SVG_NS, 'svg')
  svg.setAttribute('class', `rl-tendrils rl-tendrils--${side}`)
  svg.setAttribute('viewBox', '0 0 200 400')
  svg.setAttribute('preserveAspectRatio', 'xMidYMax meet')
  svg.setAttribute('aria-hidden', 'true')
  const specs = [
    [44, 330, 38, 34, 26, 11, 0],
    [92, 250, -30, 26, -22, 8.5, -2.5],
    [20, 190, 22, 20, 18, 13, -5],
  ]
  for (const [x, h, bend, w, curl, sway, delay] of specs) {
    const path = doc.createElementNS(SVG_NS, 'path')
    path.setAttribute('d', tentaclePath(x, h, bend, w, curl))
    path.style.setProperty('--rl-sway', `${sway}s`)
    path.style.setProperty('--rl-delay', `${delay}s`)
    svg.appendChild(path)
  }
  doc.body.appendChild(svg)
  sc.cleanup(() => svg.remove())
  return svg
}
const tendrils = [buildTendrils('left'), buildTendrils('right')]

/* ---------- 注视之眼 ---------- */

const eye = make('div', 'rl-eye')
eye.innerHTML = '<div class="rl-eye__lid"><div class="rl-eye__pupil"></div></div>'
let pointerFrame = 0
sc.listen(window, 'pointermove', event => {
  if (!eye.classList.contains('is-open') || pointerFrame) return
  pointerFrame = window.requestAnimationFrame(() => {
    pointerFrame = 0
    const rect = eye.getBoundingClientRect()
    const dx = event.clientX - (rect.left + rect.width / 2)
    const dy = event.clientY - (rect.top + rect.height / 2)
    const dist = Math.hypot(dx, dy) || 1
    eye.style.setProperty('--rl-look-x', `${((dx / dist) * Math.min(12, dist / 30)).toFixed(1)}px`)
    eye.style.setProperty('--rl-look-y', `${((dy / dist) * Math.min(3, dist / 90)).toFixed(1)}px`)
  })
}, { passive: true })
sc.cleanup(() => { if (pointerFrame) window.cancelAnimationFrame(pointerFrame) })

const scheduleEye = () => {
  sc.timeout(() => {
    if (!doc.hidden && !eye.hidden) {
      eye.classList.add('is-open')
      sc.timeout(() => eye.classList.remove('is-open'), rand(5000, 9000))
    }
    scheduleEye()
  }, rand(22000, 55000))
}
if (!reduceMotion) scheduleEye()

/* ---------- 理智值 ---------- */

const sanity = make('div', 'rl-sanity')
sanity.innerHTML = '<span class="rl-sanity__label">理智 <b>SAN</b></span><span class="rl-sanity__bar"><span class="rl-sanity__fill"></span></span><span class="rl-sanity__value"></span>'
const sanityValue = sanity.querySelector('.rl-sanity__value')

const readSan = () => {
  try {
    const value = Number(window.sessionStorage.getItem(SAN_KEY))
    return Number.isFinite(value) && value > 0 ? Math.min(SAN_MAX, value) : SAN_MAX
  } catch { return SAN_MAX }
}
let san = readSan()

const renderSan = () => {
  sanityValue.textContent = String(san)
  sanity.style.setProperty('--rl-san-pct', `${Math.round((san / SAN_MAX) * 100)}%`)
  const level = san < 10 ? '3' : san < 30 ? '2' : san < 60 ? '1' : ''
  if (level) root.dataset.rlMadness = level
  else delete root.dataset.rlMadness
}

const setSan = (next, hit = false) => {
  san = Math.max(1, Math.min(SAN_MAX, Math.round(next)))
  try { window.sessionStorage.setItem(SAN_KEY, String(san)) } catch { /* 隐私模式 */ }
  renderSan()
  if (hit && !reduceMotion) {
    sanity.classList.remove('is-hit')
    void sanity.offsetWidth
    sanity.classList.add('is-hit')
  }
}
renderSan()
sc.interval(() => { if (san < SAN_MAX) setSan(san + 1) }, 90000)

/* ---------- 低语 ---------- */

const whisper = () => {
  const el = make('div', 'rl-whisper')
  el.textContent = WHISPERS[Math.floor(Math.random() * WHISPERS.length)]
  const leftSide = Math.random() < 0.5
  el.style.top = `${rand(18, 70).toFixed(0)}vh`
  el.style[leftSide ? 'left' : 'right'] = `${rand(3, 22).toFixed(0)}vw`
  sc.timeout(() => el.remove(), 7600)
}

const scheduleWhisper = () => {
  sc.timeout(() => {
    const surface = sc.context().surface
    if (!doc.hidden && (surface === 'chat' || surface === 'lobby' || surface === 'world-detail')) whisper()
    scheduleWhisper()
  }, rand(48000, 110000))
}
if (!reduceMotion) scheduleWhisper()

/* ---------- Surface 切换 ---------- */

const renderSurface = () => {
  const surface = sc.context().surface
  const quiet = QUIET_SURFACES.has(surface)
  for (const el of [...fogs, ...tendrils, eye]) el.hidden = quiet
  sanity.hidden = !SANITY_SURFACES.has(surface)
}
renderSurface()
sc.on('surface-change', renderSurface)

/* ---------- 消息标记：禁忌词与 d100 ---------- */

const rowText = row => {
  const content = row.querySelector('.content')
  return (content || row).textContent || ''
}

const markRow = row => {
  const text = rowText(row)
  const signature = String(text.length)
  if (row.dataset.rlSig === signature) return
  row.dataset.rlSig = signature
  if (ELDRITCH.test(text)) row.dataset.rlEldritch = '1'
  else delete row.dataset.rlEldritch
  const roll = D100.exec(text)
  const value = roll ? Number(roll[1]) : NaN
  if (value >= 96 && value <= 100) row.dataset.rlRoll = 'fumble'
  else if (value >= 1 && value <= 5) row.dataset.rlRoll = 'crit'
  else delete row.dataset.rlRoll
}

const scanRows = () => {
  for (const row of doc.querySelectorAll('[data-sc-ui="message"]')) markRow(row)
}

let scanQueued = false
const queueScan = () => {
  if (scanQueued) return
  scanQueued = true
  sc.timeout(() => { scanQueued = false; scanRows() }, 220)
}
scanRows()
sc.observe(doc.body, { childList: true, subtree: true }, queueScan)

sc.cleanup(() => {
  for (const row of doc.querySelectorAll('[data-rl-sig]')) {
    delete row.dataset.rlSig
    delete row.dataset.rlEldritch
    delete row.dataset.rlRoll
  }
})

sc.on('message-updated', queueScan)
sc.on('message-created', payload => {
  queueScan()
  const ctx = sc.context()
  if (!payload || !SANITY_SURFACES.has(ctx.surface)) return
  if (payload.channelId && payload.channelId !== ctx.channelId) return
  const text = String(payload.contentText || payload.contentRaw || '')
  let loss = 0
  if (ELDRITCH.test(text)) loss += Math.ceil(rand(0.01, 3))
  const roll = D100.exec(text)
  if (roll && Number(roll[1]) >= 96) loss += 1
  if (loss) setSan(san - loss, true)
})
