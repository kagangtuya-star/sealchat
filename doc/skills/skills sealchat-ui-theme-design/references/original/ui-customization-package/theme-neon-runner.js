// NEON//RUNNER 霓虹夜行 —— 配套脚本（纯装饰）
// 只添加 aria-hidden 的装饰层、class 与 data 属性，不移动 SealChat 的 DOM，
// 不读写聊天数据、不发网络请求。所有资源都通过 sc.* 或 sc.cleanup 自动释放。

const doc = document
const root = doc.documentElement
const THEME = 'neon-runner'
const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
// 这些 Surface 只保留 CSS 外观，不显示 HUD 等装饰层
const QUIET_SURFACES = new Set(['embed', 'log-public', 'theater', 'observer'])
const BOOT_SURFACES = new Set(['chat', 'lobby', 'auth', 'world-detail'])
const BOOT_KEY = 'sealchat.theme.neon-runner.booted'

root.dataset.scTheme = THEME
sc.cleanup(() => {
  if (root.dataset.scTheme === THEME) delete root.dataset.scTheme
  delete root.dataset.nrGlitch
})

const make = (tag, className, parent = doc.body) => {
  const el = doc.createElement(tag)
  el.className = className
  el.setAttribute('aria-hidden', 'true')
  parent.appendChild(el)
  sc.cleanup(() => el.remove())
  return el
}

const pad = (n, size = 2) => String(n).padStart(size, '0')
const shortId = id => (id ? String(id).slice(-4).toUpperCase() : '----')

/* ---------- HUD 状态条与视口定位框 ---------- */

const hud = make('div', 'nr-hud')
hud.innerHTML = [
  '<span class="nr-hud__seg"><i class="nr-hud__dot"></i><span>SEALNET</span></span>',
  '<span class="nr-hud__seg nr-hud__seg--clock" data-k="clock">--:--:--</span>',
  '<span class="nr-hud__seg" data-k="node"></span>',
  '<span class="nr-hud__seg nr-hud__seg--rx">RX <b data-k="rx">000</b></span>',
].join('')
const frame = make('div', 'nr-frame')
frame.innerHTML = '<i></i><i></i><i></i><i></i>'

const hudClock = hud.querySelector('[data-k="clock"]')
const hudNode = hud.querySelector('[data-k="node"]')
const hudRx = hud.querySelector('[data-k="rx"]')
let received = 0

const renderHud = () => {
  const ctx = sc.context()
  const quiet = QUIET_SURFACES.has(ctx.surface)
  hud.hidden = quiet
  frame.hidden = quiet
  hudNode.textContent = ctx.channelId
    ? `${ctx.surface} · CH-${shortId(ctx.channelId)}`
    : `${ctx.surface} · ${ctx.worldId ? 'W-' + shortId(ctx.worldId) : 'OPEN NET'}`
  hudRx.textContent = pad(Math.min(received, 999), 3)
}

const tick = () => {
  const now = new Date()
  hudClock.textContent = `${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
}

tick()
renderHud()
sc.interval(tick, 1000)
sc.on('surface-change', renderHud)
sc.on('world-change', renderHud)

/* ---------- 开机序列（每个标签页会话一次） ---------- */

const bootOnce = () => {
  if (reduceMotion || !BOOT_SURFACES.has(sc.context().surface)) return
  try {
    if (window.sessionStorage.getItem(BOOT_KEY)) return
    window.sessionStorage.setItem(BOOT_KEY, '1')
  } catch { /* 隐私模式：照常播放一次 */ }
  const lines = [
    ['SEAL-OS v7.7 // KERNEL HANDSHAKE', true],
    ['MOUNTING /dev/neural0', true],
    ['DECRYPTING CHANNEL LATTICE', true],
    ['ICE COUNTERMEASURES ........ BYPASSED', false],
    ['LINKING OPERATOR SESSION', true],
    ['WELCOME BACK, RUNNER.', false],
  ]
  const boot = make('div', 'nr-boot')
  const total = 1500
  boot.style.setProperty('--nr-boot-ms', `${total}ms`)
  const logo = doc.createElement('div')
  logo.className = 'nr-boot__logo'
  logo.textContent = 'NEON//RUNNER'
  boot.appendChild(logo)
  lines.forEach(([text, ok], i) => {
    const line = doc.createElement('div')
    line.className = ok ? 'nr-boot__line nr-boot__line--ok' : 'nr-boot__line'
    line.textContent = `> ${text}`
    line.style.animationDelay = `${120 + i * 190}ms`
    boot.appendChild(line)
  })
  sc.timeout(() => boot.remove(), total + 600)
}
bootOnce()

/* ---------- 频道标题故障切片 ---------- */

const glitchTitle = () => {
  if (reduceMotion) return
  const title = doc.querySelector('[data-sc-ui="header"] .sc-channel-title')
  if (!title) return
  title.dataset.nrText = title.textContent || ''
  title.classList.remove('nr-glitch')
  void title.offsetWidth
  title.classList.add('nr-glitch')
  sc.timeout(() => title.classList.remove('nr-glitch'), 560)
}

sc.on('channel-change', () => {
  renderHud()
  // 标题文本在频道切换后的下一次渲染才更新
  sc.timeout(glitchTitle, 180)
})
sc.timeout(glitchTitle, 700)

/* ---------- 新信号到达 ---------- */

const flashRow = (messageId, attempt = 0) => {
  const selector = `[data-sc-ui="message"][data-message-id="${CSS.escape(messageId)}"]`
  const row = doc.querySelector(selector)
  if (!row) {
    if (attempt < 6) sc.timeout(() => flashRow(messageId, attempt + 1), 90)
    return
  }
  row.style.setProperty('--nr-row-h', `${row.offsetHeight}px`)
  row.classList.add('nr-incoming')
  sc.timeout(() => {
    row.classList.remove('nr-incoming')
    row.style.removeProperty('--nr-row-h')
  }, 900)
}

sc.on('message-created', payload => {
  const ctx = sc.context()
  if (payload && payload.channelId && payload.channelId !== ctx.channelId) return
  received += 1
  renderHud()
  if (!reduceMotion && payload && payload.messageId) flashRow(String(payload.messageId))
})

/* ---------- 随机全局色散 ---------- */

if (!reduceMotion) {
  const scheduleGlitch = () => {
    sc.timeout(() => {
      if (!doc.hidden && !QUIET_SURFACES.has(sc.context().surface)) {
        root.dataset.nrGlitch = 'on'
        sc.timeout(() => { delete root.dataset.nrGlitch }, 140)
        if (Math.random() < 0.35) glitchTitle()
      }
      scheduleGlitch()
    }, 9000 + Math.random() * 14000)
  }
  scheduleGlitch()
}
