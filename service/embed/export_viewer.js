(function () {
  const app = document.getElementById('app')
  let activeInlineAssets = Object.create(null)
  let activeInlineAssetURLs = Object.create(null)

  function resolvePalette(value) {
    return value === 'day' ? 'day' : 'night'
  }

  function resolveScheme(value) {
    return value === 'dossier' ? 'dossier' : 'tabletop'
  }

  function resolveColorizeMode(value) {
    switch (value) {
      case 'off':
      case 'body':
      case 'name':
        return value
      default:
        return 'name'
    }
  }

  function applyDisplayState(displayOptions) {
    document.body.dataset.scheme = resolveScheme(displayOptions.scheme)
    document.body.dataset.palette = resolvePalette(displayOptions.palette)
    document.body.dataset.layout = displayOptions.layout === 'bubble' ? 'bubble' : 'compact'
    document.body.dataset.colorize = resolveColorizeMode(displayOptions.colorizeMode)
    document.body.dataset.hideAvatar = displayOptions.showAvatar ? 'false' : 'true'
    document.body.dataset.hideTimestamp = displayOptions.showTimestamp ? 'false' : 'true'
    document.body.dataset.hideIc = displayOptions.showIC === false ? 'true' : 'false'
    document.body.dataset.hideOoc = displayOptions.showOOC === false ? 'true' : 'false'
    if (!document.body.dataset.searchOpen) {
      document.body.dataset.searchOpen = 'false'
    }
  }

  function refreshDisplayButtons() {
    document.querySelectorAll('.viewer-display button').forEach((btn) => {
      const action = btn.getAttribute('data-action')
      let active = false
      switch (action) {
        case 'scheme-tabletop':
          active = document.body.dataset.scheme === 'tabletop'
          break
        case 'scheme-dossier':
          active = document.body.dataset.scheme === 'dossier'
          break
        case 'palette-day':
          active = document.body.dataset.palette === 'day'
          break
        case 'palette-night':
          active = document.body.dataset.palette === 'night'
          break
        case 'layout-bubble':
          active = document.body.dataset.layout === 'bubble'
          break
        case 'layout-compact':
          active = document.body.dataset.layout === 'compact'
          break
        case 'colorize-off':
          active = document.body.dataset.colorize === 'off'
          break
        case 'colorize-name':
          active = document.body.dataset.colorize === 'name'
          break
        case 'colorize-body':
          active = document.body.dataset.colorize === 'body'
          break
        case 'toggle-avatar':
          active = document.body.dataset.hideAvatar !== 'true'
          btn.textContent = '头像 ' + (active ? '开' : '关')
          break
        case 'toggle-timestamp':
          active = document.body.dataset.hideTimestamp !== 'true'
          btn.textContent = '时间 ' + (active ? '开' : '关')
          break
        case 'toggle-ic':
          active = document.body.dataset.hideIc !== 'true'
          btn.textContent = 'IC ' + (active ? '开' : '关')
          break
        case 'toggle-ooc':
          active = document.body.dataset.hideOoc !== 'true'
          btn.textContent = 'OOC ' + (active ? '开' : '关')
          break
        case 'toggle-search':
          active = document.body.dataset.searchOpen === 'true'
          btn.textContent = '搜索 ' + (active ? '开' : '关')
          break
      }
      btn.dataset.active = active ? 'true' : 'false'
    })
  }

  function formatTime(value) {
    if (!value) return '--'
    try {
      const date = new Date(value)
      if (Number.isNaN(date.getTime())) return value
      return date.toLocaleString()
    } catch (err) {
      return value
    }
  }

  function resolveWhisperTargets(msg) {
    if (!msg || !msg.is_whisper || !Array.isArray(msg.whisper_targets)) {
      return []
    }
    return msg.whisper_targets
      .map((target) => String(target || '').trim())
      .filter((target) => target)
  }

  function formatWhisperMetaText(msg) {
    const targets = resolveWhisperTargets(msg)
    if (!targets.length) return ''
    return `发送给 ${targets.join('、')}`
  }

  function stripHTML(html) {
    if (!html) return ''
    const tmp = document.createElement('div')
    tmp.innerHTML = html
    return tmp.textContent || tmp.innerText || ''
  }

  function initials(name) {
    if (!name) return '??'
    const clean = name.trim()
    if (!clean) return '??'
    return clean.slice(0, 2).toUpperCase()
  }

  function normalizeColor(value) {
    if (!value) return ''
    const probe = document.createElement('span')
    probe.style.color = ''
    probe.style.color = String(value).trim()
    return probe.style.color || ''
  }

  function parseDataURL(dataURL) {
    const value = String(dataURL || '').trim()
    if (!value.startsWith('data:')) return null
    const comma = value.indexOf(',')
    if (comma < 0) return null
    const meta = value.slice(5, comma)
    const body = value.slice(comma + 1)
    if (!/;base64$/i.test(meta)) return null
    const mimeType = meta.replace(/;base64$/i, '') || 'application/octet-stream'
    try {
      const binary = atob(body)
      const bytes = new Uint8Array(binary.length)
      for (let i = 0; i < binary.length; i += 1) {
        bytes[i] = binary.charCodeAt(i)
      }
      return new Blob([bytes], { type: mimeType })
    } catch (err) {
      return null
    }
  }

  function applyInlineAssets(assets) {
    activeInlineAssets = assets && typeof assets === 'object' ? assets : Object.create(null)
    activeInlineAssetURLs = Object.create(null)
  }

  function resolveInlineAssetURL(src) {
    const value = String(src || '').trim()
    if (!value.startsWith('scasset:')) {
      return value
    }
    const assetID = value.slice('scasset:'.length)
    if (!assetID) return ''
    if (activeInlineAssetURLs[assetID]) {
      return activeInlineAssetURLs[assetID]
    }
    const dataURL = activeInlineAssets[assetID]
    if (!dataURL) {
      return ''
    }
    const blob = parseDataURL(dataURL)
    if (blob && typeof URL !== 'undefined' && typeof URL.createObjectURL === 'function') {
      const objectURL = URL.createObjectURL(blob)
      activeInlineAssetURLs[assetID] = objectURL
      return objectURL
    }
    activeInlineAssetURLs[assetID] = dataURL
    return dataURL
  }

  function resolveInlineAssetImages(root) {
    if (!root || typeof root.querySelectorAll !== 'function') return
    root.querySelectorAll('img').forEach((img) => {
      const resolved = resolveInlineAssetURL(img.getAttribute('src') || '')
      if (resolved) {
        img.setAttribute('src', resolved)
      }
    })
  }

  // ---------------------------------------------------------------------------
  // 聊天装扮（chat skin）：按消息发送时的快照 sender_chat_skin 渲染。
  // 与前端 ui/src/features/chat-skin/chatSkinRender.ts 保持同一语义的精简实现；
  // 只通过 style.setProperty 写入经服务端白名单清洗过的值，不使用 innerHTML。
  // ---------------------------------------------------------------------------
  const CHAT_SKIN_ORNAMENTS = {
    "cat-ear": ["#f4a6b8", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path d=\"M8 96 L30 12 Q35 0 44 9 L94 96 Z\"/><path fill-opacity=\".42\" d=\"M27 86 L37 34 Q39 27 44 33 L74 86 Z\"/></svg>"],
    "bunny-ear": ["#f8c8d4", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 50 120\"><path d=\"M25 118 C6 100 2 44 13 12 C17 1 33 1 37 12 C48 44 44 100 25 118 Z\"/><path fill-opacity=\".42\" d=\"M25 104 C14 90 12 50 19 22 C21 15 29 15 31 22 C38 50 36 90 25 104 Z\"/></svg>"],
    "bow": ["#e85d75", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 120 70\"><path d=\"M60 35 C44 14 18 2 8 10 C-2 18 2 52 12 60 C22 68 44 54 60 35 Z\"/><path d=\"M60 35 C76 14 102 2 112 10 C122 18 118 52 108 60 C98 68 76 54 60 35 Z\"/><path fill-opacity=\".45\" d=\"M54 38 L40 68 L50 66 L58 46 Z M66 38 L80 68 L70 66 L62 46 Z\"/><circle cx=\"60\" cy=\"35\" r=\"10\"/></svg>"],
    "star": ["#f5c542", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path d=\"M50 4 L62 37 L97 38 L69 59 L79 94 L50 73 L21 94 L31 59 L3 38 L38 37 Z\"/></svg>"],
    "sparkle": ["#ffd76a", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path d=\"M50 2 C54 34 66 46 98 50 C66 54 54 66 50 98 C46 66 34 54 2 50 C34 46 46 34 50 2 Z\"/></svg>"],
    "heart": ["#ef5b7b", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 90\"><path d=\"M50 88 C20 66 2 48 2 27 C2 12 14 2 28 2 C38 2 46 8 50 16 C54 8 62 2 72 2 C86 2 98 12 98 27 C98 48 80 66 50 88 Z\"/></svg>"],
    "flower": ["#f29cb7", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><g><ellipse cx=\"50\" cy=\"24\" rx=\"15\" ry=\"22\"/><ellipse cx=\"50\" cy=\"24\" rx=\"15\" ry=\"22\" transform=\"rotate(72 50 50)\"/><ellipse cx=\"50\" cy=\"24\" rx=\"15\" ry=\"22\" transform=\"rotate(144 50 50)\"/><ellipse cx=\"50\" cy=\"24\" rx=\"15\" ry=\"22\" transform=\"rotate(216 50 50)\"/><ellipse cx=\"50\" cy=\"24\" rx=\"15\" ry=\"22\" transform=\"rotate(288 50 50)\"/></g><circle cx=\"50\" cy=\"50\" r=\"11\" fill-opacity=\".45\"/></svg>"],
    "paw": ["#c58b6b", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path d=\"M50 50 C30 50 18 70 22 84 C25 94 38 92 50 88 C62 92 75 94 78 84 C82 70 70 50 50 50 Z\"/><ellipse cx=\"20\" cy=\"40\" rx=\"10\" ry=\"13\"/><ellipse cx=\"38\" cy=\"20\" rx=\"10\" ry=\"14\"/><ellipse cx=\"62\" cy=\"20\" rx=\"10\" ry=\"14\"/><ellipse cx=\"80\" cy=\"40\" rx=\"10\" ry=\"13\"/></svg>"],
    "crown": ["#e3b341", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 120 80\"><path d=\"M8 70 L4 18 L34 42 L60 6 L86 42 L116 18 L112 70 Z\"/><rect x=\"8\" y=\"72\" width=\"104\" height=\"8\" rx=\"2\" fill-opacity=\".6\"/><circle cx=\"4\" cy=\"16\" r=\"5\"/><circle cx=\"60\" cy=\"5\" r=\"5\"/><circle cx=\"116\" cy=\"16\" r=\"5\"/></svg>"],
    "leaf": ["#6dbb7a", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path d=\"M8 92 C4 44 40 6 96 4 C94 60 56 96 8 92 Z\"/><path fill-opacity=\".4\" d=\"M14 88 C36 64 58 40 86 14 L88 16 C62 42 40 66 18 90 Z\"/></svg>"],
    "moon": ["#f2d27a", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path d=\"M64 4 C34 8 12 32 12 60 C12 82 30 98 54 98 C76 98 92 86 98 68 C88 74 76 76 66 72 C46 66 36 44 42 24 C46 14 54 8 64 4 Z\"/></svg>"],
    "cloud": ["#cfe3f7", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 120 70\"><path d=\"M24 68 C10 68 2 58 2 48 C2 36 12 28 24 30 C26 14 40 4 56 6 C70 8 78 18 80 28 C86 22 96 22 104 28 C112 34 114 42 112 48 C118 52 120 58 116 64 C114 67 110 68 106 68 Z\"/></svg>"],
    "tail": ["#ffffff", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 40 40\"><path d=\"M40 4 C26 6 12 6 0 0 C8 12 10 26 4 40 C18 32 30 20 40 18 Z\"/></svg>"],
    "corner": ["#b08d57", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path d=\"M4 96 L4 30 C4 14 14 4 30 4 L96 4 L96 12 L32 12 C20 12 12 20 12 32 L12 96 Z\"/><path fill-opacity=\".55\" d=\"M24 96 L24 40 C24 30 30 24 40 24 L96 24 L96 28 L40 28 C33 28 28 33 28 40 L28 96 Z\"/><circle cx=\"30\" cy=\"30\" r=\"6\"/></svg>"],
    "scallop": ["#ffffff", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 200 24\"><path d=\"M0 0 H200 V12 C192 12 190 24 180 24 C170 24 168 12 160 12 C152 12 150 24 140 24 C130 24 128 12 120 12 C112 12 110 24 100 24 C90 24 88 12 80 12 C72 12 70 24 60 24 C50 24 48 12 40 12 C32 12 30 24 20 24 C10 24 8 12 0 12 Z\"/></svg>"],
    "wave": ["#7aa7d9", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 200 20\"><path d=\"M0 10 C17 0 33 0 50 10 C67 20 83 20 100 10 C117 0 133 0 150 10 C167 20 183 20 200 10 L200 14 C183 24 167 24 150 14 C133 4 117 4 100 14 C83 24 67 24 50 14 C33 4 17 4 0 14 Z\"/></svg>"],
    "dots": ["#94a3b8", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 120 30\"><circle cx=\"15\" cy=\"15\" r=\"9\"/><circle cx=\"45\" cy=\"15\" r=\"7\" fill-opacity=\".75\"/><circle cx=\"72\" cy=\"15\" r=\"5\" fill-opacity=\".55\"/><circle cx=\"95\" cy=\"15\" r=\"3.5\" fill-opacity=\".4\"/><circle cx=\"112\" cy=\"15\" r=\"2.5\" fill-opacity=\".3\"/></svg>"],
    "seal": ["#c0392b", "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 100 100\"><path fill-rule=\"evenodd\" d=\"M50 2 A48 48 0 1 1 49.9 2 Z M50 10 A40 40 0 1 0 50.1 10 Z\"/><path fill-rule=\"evenodd\" fill-opacity=\".85\" d=\"M50 16 A34 34 0 1 1 49.9 16 Z M50 22 A28 28 0 1 0 50.1 22 Z\"/><path d=\"M50 32 L54 45 L68 45 L57 53 L61 67 L50 59 L39 67 L43 53 L32 45 L46 45 Z\"/></svg>"],
  }
  const CHAT_SKIN_FONTS = {
    sans: '"PingFang SC", "Microsoft YaHei", "Noto Sans SC", system-ui, sans-serif',
    serif: '"Noto Serif SC", "Source Han Serif SC", "Songti SC", "SimSun", serif',
    kai: '"Kaiti SC", "STKaiti", "KaiTi", "BiauKai", serif',
    mono: 'ui-monospace, "SFMono-Regular", "JetBrains Mono", Consolas, monospace',
    rounded: '"Yuanti SC", "YouYuan", "Varela Round", "Nunito", system-ui, sans-serif',
  }
  const CHAT_SKIN_NAME_WEIGHTS = { normal: '400', bold: '700', black: '900' }
  const chatSkinArticles = []

  function skinNum(value, fallback) {
    const num = Number(value)
    return Number.isFinite(num) ? num : fallback
  }

  function skinPx(value) {
    return Math.round(skinNum(value, 0) * 100) / 100 + 'px'
  }

  function skinColor(value) {
    const raw = String(value || '').trim().toLowerCase()
    if (raw === 'transparent') return raw
    return /^#([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/.test(raw) ? raw : ''
  }

  function skinCssUrl(src) {
    return 'url("' + String(src).replace(/["\\\n\r]/g, '') + '")'
  }

  function resolveChatSkinRef(msg, ref) {
    const value = String(ref || '')
    if (value.indexOf('builtin:') === 0) {
      const ornament = CHAT_SKIN_ORNAMENTS[value.slice(8)]
      return ornament
        ? { src: 'data:image/svg+xml,' + encodeURIComponent(ornament[1]), mask: true, tint: ornament[0] }
        : null
    }
    if (value.indexOf('id:') !== 0) return null
    const assets = msg.sender_chat_skin_assets || {}
    const src = resolveInlineAssetURL(assets[value])
    return src && !src.startsWith('id:') ? { src: src, mask: false } : null
  }

  function anchorPoint(anchor) {
    const value = String(anchor || 'top-left')
    return {
      x: /left$/.test(value) ? 0 : /right$/.test(value) ? 100 : 50,
      y: /^top/.test(value) ? 0 : /^bottom/.test(value) ? 100 : 50,
    }
  }

  function isChatSkinSurfaceEffective(surface) {
    if (!surface || typeof surface !== 'object' || surface.enabled === false) return false
    const style = surface.style && typeof surface.style === 'object' ? surface.style : {}
    const layers = Array.isArray(surface.layers) ? surface.layers : []
    return Object.keys(style).length > 0 || !!surface.night || layers.some((layer) => layer && layer.enabled !== false) || !!surface.advanced
  }

  function setSkinProps(el, props) {
    if (!el) return
    const applied = el.__chatSkinProps || []
    Object.keys(props).forEach((key) => {
      const value = props[key]
      if (value === undefined || value === null || value === '') return
      el.style.setProperty(key, String(value))
      applied.push(key)
    })
    el.__chatSkinProps = applied
  }

  function clearSkinProps(el) {
    if (!el || !el.__chatSkinProps) return
    el.__chatSkinProps.forEach((key) => el.style.removeProperty(key))
    el.__chatSkinProps = []
  }

  const CHAT_SKIN_CSS_PROPERTIES = new Set((
    'background background-color background-image background-size background-position ' +
    'background-repeat background-blend-mode border border-color border-style border-width border-radius ' +
    'border-top border-right border-bottom border-left box-shadow outline outline-offset filter backdrop-filter ' +
    'opacity mix-blend-mode color text-shadow letter-spacing word-spacing line-height font-style font-weight ' +
    'font-variant text-decoration text-decoration-color text-decoration-style text-transform text-align ' +
    'padding padding-top padding-right padding-bottom padding-left min-height min-width'
  ).split(' '))
  const SKIN_FORBIDDEN_CSS = /url\s*\(|expression\s*\(|javascript:|@import|[<>{}\\]|\/\*|image-set|image\s*\(|-moz-binding|behavior\s*:/i

  function constrainSkinCssGeometry(property, value) {
    if (property === 'min-width') return 'min(100%, 480px, max(0px, ' + value + '))'
    if (property === 'min-height') return 'clamp(0px, ' + value + ', 240px)'
    if (property === 'line-height' && value !== 'normal') {
      const length = /^[\d.]+$/.test(value) ? value + 'em' : value
      return 'clamp(0px, ' + length + ', 96px)'
    }
    if (property === 'padding' || property.startsWith('padding-') || property === 'border-width') {
      const parts = value.match(/(?:[^\s()]+|\((?:[^()]|\([^()]*\))*\))+/g) || []
      const limit = 48
      return parts.map((part) => 'clamp(0px, ' + part + ', ' + limit + 'px)').join(' ')
    }
    if (/^border(?:-(?:top|right|bottom|left))?$/.test(property)) {
      return value.replace(/(?:\d*\.)?\d+(?:px|em|rem|vw|vh|vmin|vmax)\b/gi, (length) => 'clamp(0px, ' + length + ', 48px)')
    }
    return value
  }

  function parseSkinCss(css) {
    const out = {}
    let total = 0
    for (const part of String(css || '').slice(0, 1200).split(';')) {
      const index = part.indexOf(':')
      if (index <= 0) continue
      const name = part.slice(0, index).trim().toLowerCase()
      const value = part.slice(index + 1).replace(/!important/gi, '').trim()
      if (!CHAT_SKIN_CSS_PROPERTIES.has(name) || !value || value.length > 200 || SKIN_FORBIDDEN_CSS.test(value)) continue
      const length = name.length + 2 + value.length + (total ? 2 : 0)
      if (total + length > 600) break
      total += length
      out[name] = constrainSkinCssGeometry(name, value)
    }
    return out
  }

  function buildSkinBackground(msg, background, out) {
    if (!background || typeof background !== 'object') return
    const mode = background.mode === 'gradient' || background.mode === 'none' ? background.mode : 'color'
    const color = skinColor(background.color) || '#ffffff'
    const color2 = skinColor(background.color2) || '#f1f5f9'
    const image = background.imageRef ? resolveChatSkinRef(msg, background.imageRef) : null
    const images = []
    const sizes = []
    const repeats = []
    const positions = []
    if (mode === 'gradient') {
      images.push('linear-gradient(' + skinNum(background.angle, 135) + 'deg, ' + color + ', ' + color2 + ')')
      sizes.push('100% 100%'); repeats.push('no-repeat'); positions.push('center')
    } else if (mode === 'color' && image) {
      images.push('linear-gradient(' + color + ', ' + color + ')')
      sizes.push('100% 100%'); repeats.push('no-repeat'); positions.push('center')
    }
    if (image && image.src && !image.mask) {
      const fit = ['cover', 'contain', 'tile', 'stretch'].indexOf(background.imageFit) >= 0 ? background.imageFit : 'cover'
      images.push(skinCssUrl(image.src))
      sizes.push(fit === 'tile' ? 'auto' : fit === 'stretch' ? '100% 100%' : fit)
      repeats.push(fit === 'tile' ? 'repeat' : 'no-repeat')
      positions.push(['center', 'top', 'bottom', 'left', 'right'].indexOf(background.imagePosition) >= 0 ? background.imagePosition : 'center')
    }
    out['background-color'] = mode === 'color' && !image ? color : 'transparent'
    out['background-image'] = images.length ? images.join(', ') : 'none'
    if (images.length) {
      out['background-size'] = sizes.join(', ')
      out['background-repeat'] = repeats.join(', ')
      out['background-position'] = positions.join(', ')
    }
  }

  function skinLayerDimensions(layer) {
    const aspect = Math.round(Math.min(20, Math.max(0.05, skinNum(layer.aspect, 1))) * 1000) / 1000
    const size = Math.round(Math.min(480, Math.max(8, skinNum(layer.size, 40))) * 1000) / 1000
    const width = Math.min(size, Math.floor(480 / aspect * 1000) / 1000)
    return { width: width, height: width * aspect }
  }

  function estimateSkinReserve(layers, surfaceMinHeight = 0) {
    let top = 0
    let bottom = 0
    layers.forEach((layer) => {
      if (layer.enabled === false) return
      const { width, height } = skinLayerDimensions(layer)
      const radians = (Math.abs(skinNum(layer.rotate, 0)) * Math.PI) / 180
      const box = Math.abs(width * Math.sin(radians)) + Math.abs(height * Math.cos(radians))
      const centerY = anchorPoint(layer.anchor).y / 100 * surfaceMinHeight + skinNum(layer.y, 0)
      top = Math.max(top, box / 2 - centerY)
      bottom = Math.max(bottom, centerY + box / 2 - surfaceMinHeight)
    })
    return { top: Math.min(96, Math.max(0, Math.ceil(top))), bottom: Math.min(96, Math.max(0, Math.ceil(bottom))) }
  }

  function renderChatSkin(article) {
    const msg = article.__chatSkinMessage
    const bubble = article.querySelector('.viewer-message__bubble')
    const body = article.querySelector('.viewer-message__body')
    const sender = article.querySelector('.viewer-message__sender')
    ;[article, bubble, body, sender].forEach(clearSkinProps)
    article.querySelectorAll('.chat-skin-layers').forEach((layer) => layer.remove())
    delete article.dataset.chatSkin
    const skin = msg && msg.sender_chat_skin
    if (!skin || typeof skin !== 'object' || msg.is_whisper || msg.is_archived) return
    if (skin.scope !== 'all' && String(msg.ic_mode || 'ic').toLowerCase() !== 'ic') return
    const layout = document.body.dataset.layout === 'bubble' ? 'bubble' : 'compact'
    const surface = skin[layout]
    if (!isChatSkinSurfaceEffective(surface)) return
    const night = document.body.dataset.palette !== 'day' && surface.night && typeof surface.night === 'object'
    const style = Object.assign({}, surface.style || {}, night ? surface.night : {})
    const host = layout === 'bubble' ? bubble : article
    if (!host) return
    const surfaceProps = {}
    buildSkinBackground(msg, style.background, surfaceProps)
    if (style.border && typeof style.border === 'object') {
      const width = skinNum(style.border.width, 1)
      surfaceProps.border = width > 0 ? skinPx(width) + ' ' + (style.border.style || 'solid') + ' ' + (skinColor(style.border.color) || '#94a3b8') : 'none'
    }
    if (style.radius !== undefined) {
      const r = typeof style.radius === 'object' ? style.radius : { tl: style.radius, tr: style.radius, br: style.radius, bl: style.radius }
      surfaceProps['border-radius'] = [r.tl, r.tr, r.br, r.bl].map((value) => skinPx(skinNum(value, 12))).join(' ')
    }
    const shadows = []
    if (style.accent && typeof style.accent === 'object') {
      const width = skinNum(style.accent.width, 3)
      const color = skinColor(style.accent.color) || '#6366f1'
      const side = style.accent.side
      shadows.push(side === 'right' ? 'inset -' + width + 'px 0 0 ' + color
        : side === 'top' ? 'inset 0 ' + width + 'px 0 ' + color
        : side === 'bottom' ? 'inset 0 -' + width + 'px 0 ' + color
        : 'inset ' + width + 'px 0 0 ' + color)
    }
    if (style.shadow && typeof style.shadow === 'object') {
      if (style.shadow.mode !== 'none') {
        shadows.push(skinPx(style.shadow.x) + ' ' + skinPx(skinNum(style.shadow.y, 4)) + ' ' + skinPx(skinNum(style.shadow.blur, 12)) + ' ' + skinPx(style.shadow.spread) + ' ' + (skinColor(style.shadow.color) || '#0f172a29'))
      }
      surfaceProps['box-shadow'] = shadows.length ? shadows.join(', ') : 'none'
    } else if (shadows.length) {
      surfaceProps['box-shadow'] = shadows.join(', ')
    }
    if (style.padding && typeof style.padding === 'object') {
      surfaceProps.padding = skinPx(skinNum(style.padding.y, 10)) + ' ' + skinPx(skinNum(style.padding.x, 14))
    }
    if (style.size && typeof style.size === 'object') {
      if (skinNum(style.size.minWidth, 0) > 0) surfaceProps['min-width'] = 'min(100%, ' + skinPx(style.size.minWidth) + ')'
      if (skinNum(style.size.minHeight, 0) > 0) surfaceProps['min-height'] = skinPx(style.size.minHeight)
      if (skinNum(style.size.maxWidth, 0) > 0) surfaceProps['max-width'] = 'min(100%, ' + skinPx(style.size.maxWidth) + ')'
    }
    if (skinNum(style.blur, 0) > 0) {
      surfaceProps['backdrop-filter'] = 'blur(' + skinPx(style.blur) + ')'
      surfaceProps['-webkit-backdrop-filter'] = 'blur(' + skinPx(style.blur) + ')'
    }
    const layers = (Array.isArray(surface.layers) ? surface.layers : []).slice(0, 8)
      .filter((layer) => layer && layer.enabled !== false && resolveChatSkinRef(msg, layer.ref))
    const reserve = surface.reserveSpace === false ? { top: 0, bottom: 0 } : estimateSkinReserve(layers)
    const spacingTop = skinNum(style.spacing && style.spacing.top, 0) + reserve.top
    const spacingBottom = skinNum(style.spacing && style.spacing.bottom, 0) + reserve.bottom
    if (spacingTop > 0) surfaceProps['margin-top'] = skinPx(spacingTop)
    if (spacingBottom > 0) surfaceProps['margin-bottom'] = skinPx(spacingBottom)
    const textProps = {}
    if (style.text && typeof style.text === 'object') {
      if (skinColor(style.text.color)) textProps.color = skinColor(style.text.color)
      if (CHAT_SKIN_FONTS[style.text.font]) textProps['font-family'] = CHAT_SKIN_FONTS[style.text.font]
      if (skinNum(style.text.scale, 1) !== 1) textProps['font-size'] = (Math.min(1.4, Math.max(0.8, skinNum(style.text.scale, 1)))) + 'em'
      if (skinColor(style.text.shadow)) textProps['text-shadow'] = '0 1px 2px ' + skinColor(style.text.shadow)
    }
    const nameProps = {}
    if (style.name && typeof style.name === 'object') {
      if (skinColor(style.name.color)) nameProps.color = skinColor(style.name.color)
      if (CHAT_SKIN_FONTS[style.name.font]) nameProps['font-family'] = CHAT_SKIN_FONTS[style.name.font]
      if (CHAT_SKIN_NAME_WEIGHTS[style.name.weight]) nameProps['font-weight'] = CHAT_SKIN_NAME_WEIGHTS[style.name.weight]
      if (style.name.italic === true) nameProps['font-style'] = 'italic'
      if (skinColor(style.name.chip)) {
        nameProps['background-color'] = skinColor(style.name.chip)
        nameProps.padding = '0 0.5em'
        nameProps['border-radius'] = '999px'
      }
      if (skinColor(style.name.shadow)) nameProps['text-shadow'] = '0 1px 3px ' + skinColor(style.name.shadow)
    }
    const rootProps = {}
    const advanced = surface.advanced && typeof surface.advanced === 'object' ? surface.advanced : null
    if (advanced) {
      const vars = advanced.vars && typeof advanced.vars === 'object' ? advanced.vars : {}
      Object.keys(vars).forEach((key) => {
        const value = typeof vars[key] === 'string' ? vars[key].replace(/!important/gi, '').trim() : ''
        if (Object.keys(rootProps).length < 24 && /^--(chat|quote|message|sc-skin)-[a-z0-9-]{1,48}$/.test(key)
          && value && value.length <= 200 && !value.includes(';') && !SKIN_FORBIDDEN_CSS.test(value)) rootProps[key] = value
      })
      Object.assign(surfaceProps, parseSkinCss(advanced.surfaceCss))
      Object.assign(textProps, parseSkinCss(advanced.textCss))
      Object.assign(nameProps, parseSkinCss(advanced.nameCss))
    }
    if (layers.length) {
      surfaceProps.position = 'relative'
      surfaceProps.isolation = 'isolate'
    }
    surfaceProps['--sc-skin-reserve-top'] = skinPx(reserve.top)
    surfaceProps['--sc-skin-reserve-bottom'] = skinPx(reserve.bottom)
    setSkinProps(article, rootProps)
    setSkinProps(host, surfaceProps)
    setSkinProps(body, textProps)
    setSkinProps(sender, nameProps)
    article.dataset.chatSkin = layout
    if (!layers.length) return
    const containers = {}
    ;['back', 'front'].forEach((side) => {
      const container = document.createElement('span')
      container.className = 'chat-skin-layers chat-skin-layers--' + side
      container.setAttribute('aria-hidden', 'true')
      containers[side] = container
    })
    layers.slice(0, 8).forEach((layer) => {
      const resolved = resolveChatSkinRef(msg, layer.ref)
      if (!resolved) return
      const point = anchorPoint(layer.anchor)
      const { width, height } = skinLayerDimensions(layer)
      const el = document.createElement('span')
      el.className = 'chat-skin-layer'
      el.style.position = 'absolute'
      el.style.display = 'block'
      el.style.pointerEvents = 'none'
      el.style.left = point.x + '%'
      el.style.top = point.y + '%'
      el.style.width = skinPx(width)
      el.style.height = skinPx(height)
      el.style.transform = 'translate(-50%, -50%) translate(' + skinPx(layer.x) + ', ' + skinPx(layer.y) + ') rotate(' + skinNum(layer.rotate, 0) + 'deg) scale(' + (layer.flipX ? -1 : 1) + ', ' + (layer.flipY ? -1 : 1) + ')'
      el.style.zIndex = layer.z === 'back' ? '-1' : '1'
      if (skinNum(layer.opacity, 1) < 1) el.style.opacity = String(Math.max(0, skinNum(layer.opacity, 1)))
      if (['multiply', 'screen', 'overlay', 'soft-light'].indexOf(layer.blend) >= 0) el.style.mixBlendMode = layer.blend
      if (resolved.mask) {
        const mask = skinCssUrl(resolved.src) + ' center / contain no-repeat'
        el.style.setProperty('-webkit-mask', mask)
        el.style.setProperty('mask', mask)
        el.style.backgroundColor = skinColor(layer.tint) || resolved.tint || 'currentColor'
      } else {
        const img = document.createElement('img')
        img.src = resolved.src
        img.alt = ''
        img.draggable = false
        img.style.cssText = 'display:block;width:100%;height:100%;object-fit:contain;pointer-events:none;'
        el.appendChild(img)
      }
      containers[layer.z === 'back' ? 'back' : 'front'].appendChild(el)
    })
    ;['back', 'front'].forEach((side) => {
      if (containers[side].childNodes.length) host.appendChild(containers[side])
    })
  }

  function attachChatSkin(article, msg) {
    if (!msg || !msg.sender_chat_skin) return
    article.__chatSkinMessage = msg
    chatSkinArticles.push(article)
    renderChatSkin(article)
  }

  function refreshChatSkins() {
    chatSkinArticles.forEach(renderChatSkin)
  }

  function createChip(text) {
    const span = document.createElement('span')
    span.className = 'viewer-chip'
    span.textContent = text
    return span
  }

  function renderEmpty() {
    const wrapper = document.createElement('div')
    wrapper.className = 'viewer-shell viewer-empty'
    wrapper.innerHTML = '<p>未找到导出数据，请重新生成导出文件。</p>'
    app.appendChild(wrapper)
  }

  function renderIndex(manifest) {
    applyInlineAssets(null)
    applyDisplayState({
      scheme: manifest.display_options?.scheme || 'tabletop',
      palette: manifest.display_options?.palette || 'night',
      layout: manifest.display_options?.layout || 'compact',
      colorizeMode: manifest.display_options?.colorizeMode || 'name',
      showAvatar: manifest.display_options?.showAvatar !== false,
      showTimestamp: true,
      showIC: true,
      showOOC: true,
    })
    const shell = document.createElement('div')
    shell.className = 'viewer-shell'

    const header = document.createElement('div')
    header.className = 'viewer-header'
    header.innerHTML = `<h1>${manifest.channel_name}</h1>`

    const meta = document.createElement('div')
    meta.className = 'viewer-meta'
    meta.appendChild(createChip(`分片 ${manifest.part_total}`))
    meta.appendChild(createChip(`消息 ${manifest.total_messages}`))
    meta.appendChild(
      createChip(`切片 ${manifest.slice_limit} / 并发 ${manifest.max_concurrency}`),
    )
    header.appendChild(meta)

    shell.appendChild(header)

    const grid = document.createElement('div')
    grid.className = 'parts-grid'
    if (Array.isArray(manifest.parts)) {
      manifest.parts.forEach((part) => {
        const card = document.createElement('div')
        card.className = 'parts-card'
        card.innerHTML = `
          <h3>Part ${part.part_index}/${part.part_total}</h3>
          <p>消息：${part.messages}</p>
          <p>范围：${formatTime(part.slice_start)} → ${formatTime(part.slice_end)}</p>
          ${part.sha256 ? `<p>SHA256：${part.sha256.slice(0, 12)}…</p>` : ''}
          <a href="${part.file}">打开分片</a>
        `
        grid.appendChild(card)
      })
    }
    shell.appendChild(grid)
    app.appendChild(shell)
  }

  function renderPart(payload) {
    applyInlineAssets(payload.inline_assets)
    const displayOptions = {
      scheme: payload.display_options?.scheme || 'tabletop',
      layout: payload.display_options?.layout || 'compact',
      palette: payload.display_options?.palette || 'night',
      colorizeMode: payload.display_options?.colorizeMode || 'name',
      showAvatar: payload.display_options?.showAvatar !== false,
      showTimestamp: payload.without_timestamp !== true,
      showIC: true,
      showOOC: true,
    }
    applyDisplayState(displayOptions)

    const shell = document.createElement('div')
    shell.className = 'viewer-shell'

    const header = document.createElement('div')
    header.className = 'viewer-header'
    header.innerHTML = `<h1>${payload.channel_name}</h1>`

    const meta = document.createElement('div')
    meta.className = 'viewer-meta'
    meta.appendChild(
      createChip(`分片 ${payload.part_index || 1} / ${payload.part_total || 1}`),
    )
    meta.appendChild(createChip(`消息 ${payload.messages?.length || 0}`))
    meta.appendChild(
      createChip(
        `时间 ${formatTime(payload.slice_start || payload.start_time)} → ${formatTime(
          payload.slice_end || payload.end_time,
        )}`,
      ),
    )
    header.appendChild(meta)
    shell.appendChild(header)

    const controls = document.createElement('div')
    controls.className = 'viewer-controls viewer-search-panel'
    controls.innerHTML = `
      <div class="viewer-search-panel__header">
        <strong>消息搜索</strong>
        <button type="button" id="viewer-search-close">收起</button>
      </div>
      <div class="viewer-search-panel__body">
        <input id="viewer-search-input" type="text" placeholder="关键词 / 正则表达式" />
        <label><input type="checkbox" id="viewer-case" />区分大小写</label>
        <label><input type="checkbox" id="viewer-regex" />正则</label>
      </div>
      <div class="viewer-search-panel__nav">
        <button type="button" id="viewer-prev">上一条</button>
        <button type="button" id="viewer-next">下一条</button>
        <span class="viewer-chip" id="viewer-counter">0 / 0</span>
      </div>
    `

    const display = document.createElement('div')
    display.className = 'viewer-display'
    display.innerHTML = `
      <div>
        <button type="button" data-action="scheme-tabletop">桌游风</button>
        <button type="button" data-action="scheme-dossier">档案风</button>
      </div>
      <div>
        <button type="button" data-action="layout-bubble">气泡</button>
        <button type="button" data-action="layout-compact">紧凑</button>
      </div>
      <div>
        <button type="button" data-action="colorize-off">染色关</button>
        <button type="button" data-action="colorize-name">昵称染色</button>
        <button type="button" data-action="colorize-body">内容染色</button>
      </div>
      <div>
        <button type="button" data-action="palette-day">日间</button>
        <button type="button" data-action="palette-night">夜间</button>
      </div>
      <div>
        <button type="button" data-action="toggle-avatar">头像 ${displayOptions.showAvatar ? '开' : '关'}</button>
      </div>
      <div>
        <button type="button" data-action="toggle-timestamp">时间 ${displayOptions.showTimestamp ? '开' : '关'}</button>
      </div>
      <div>
        <button type="button" data-action="toggle-search">搜索 关</button>
      </div>
      <div>
        <button type="button" data-action="print-pdf">打印为 PDF</button>
      </div>
      <div>
        <button type="button" data-action="toggle-ic">IC 开</button>
        <button type="button" data-action="toggle-ooc">OOC 开</button>
      </div>
    `
    shell.appendChild(display)
    shell.appendChild(controls)

    const list = document.createElement('div')
    list.className = 'viewer-message-list'
    list.id = 'viewer-message-list'
    if (Array.isArray(payload.messages)) {
      payload.messages.forEach((msg) => {
        list.appendChild(createMessageElement(msg))
      })
    }
    shell.appendChild(list)

    if ((payload.part_total || 1) > 1) {
      const nav = document.createElement('div')
      nav.className = 'viewer-nav'
      const idx = payload.part_index || 1
      const total = payload.part_total || 1
      nav.innerHTML = `
        ${idx > 1
          ? `<a href="part-${String(idx - 1).padStart(3, '0')}.html">上一分片</a>`
          : '<span></span>'
        }
        <a href="../index.html">返回索引</a>
        ${idx < total
          ? `<a href="part-${String(idx + 1).padStart(3, '0')}.html">下一分片</a>`
          : '<span></span>'
        }
      `
      shell.appendChild(nav)
    }

    app.appendChild(shell)
    attachSearch()
    attachDisplayControls(displayOptions)
  }

  function attachDisplayControls(displayOptions) {
    applyDisplayState(displayOptions)
    refreshDisplayButtons()
    document.querySelectorAll('.viewer-display button').forEach((btn) => {
      btn.addEventListener('click', () => {
        const action = btn.getAttribute('data-action')
        switch (action) {
          case 'scheme-tabletop':
            document.body.dataset.scheme = 'tabletop'
            break
          case 'scheme-dossier':
            document.body.dataset.scheme = 'dossier'
            break
          case 'layout-bubble':
            document.body.dataset.layout = 'bubble'
            break
          case 'layout-compact':
            document.body.dataset.layout = 'compact'
            break
          case 'colorize-off':
            document.body.dataset.colorize = 'off'
            break
          case 'colorize-name':
            document.body.dataset.colorize = 'name'
            break
          case 'colorize-body':
            document.body.dataset.colorize = 'body'
            break
          case 'palette-day':
            document.body.dataset.palette = 'day'
            break
          case 'palette-night':
            document.body.dataset.palette = 'night'
            break
          case 'toggle-avatar':
            document.body.dataset.hideAvatar =
              document.body.dataset.hideAvatar === 'true' ? 'false' : 'true'
            btn.textContent =
              '头像 ' + (document.body.dataset.hideAvatar === 'true' ? '关' : '开')
            break
          case 'toggle-timestamp':
            document.body.dataset.hideTimestamp =
              document.body.dataset.hideTimestamp === 'true' ? 'false' : 'true'
            break
          case 'toggle-ic':
            document.body.dataset.hideIc =
              document.body.dataset.hideIc === 'true' ? 'false' : 'true'
            break
          case 'toggle-ooc':
            document.body.dataset.hideOoc =
              document.body.dataset.hideOoc === 'true' ? 'false' : 'true'
            break
          case 'toggle-search':
            document.body.dataset.searchOpen =
              document.body.dataset.searchOpen === 'true' ? 'false' : 'true'
            if (document.body.dataset.searchOpen === 'true') {
              const searchInput = document.getElementById('viewer-search-input')
              if (searchInput && typeof searchInput.focus === 'function') {
                searchInput.focus()
              }
            }
            break
          case 'print-pdf':
            if (typeof window.print === 'function') {
              window.print()
            }
            break
        }
        refreshDisplayButtons()
        if (action && /^(layout|palette)-/.test(action)) {
          refreshChatSkins()
        }
      })
    })
  }

  function attachSearch() {
    const panel = document.querySelector('.viewer-search-panel')
    const input = document.getElementById('viewer-search-input')
    const closeBtn = document.getElementById('viewer-search-close')
    const counter = document.getElementById('viewer-counter')
    const prevBtn = document.getElementById('viewer-prev')
    const nextBtn = document.getElementById('viewer-next')
    const caseBox = document.getElementById('viewer-case')
    const regexBox = document.getElementById('viewer-regex')
    const messages = Array.from(document.querySelectorAll('.viewer-message'))

    let hits = []
    let activeIndex = -1

    function updateHits() {
      const raw = input.value.trim()
      const useRegex = regexBox.checked
      const caseSensitive = caseBox.checked
      hits = []
      activeIndex = -1
      messages.forEach((msg) => msg.classList.remove('search-hit', 'search-hit-active'))
      if (!raw) {
        counter.textContent = '0 / 0'
        return
      }
      document.body.dataset.searchOpen = 'true'
      refreshDisplayButtons()
      let matcher
      try {
        if (useRegex) {
          const reg = new RegExp(raw, caseSensitive ? '' : 'i')
          matcher = (text) => reg.test(text)
        } else {
          const needle = caseSensitive ? raw : raw.toLowerCase()
          matcher = (text) => text.includes(needle)
        }
      } catch (err) {
        counter.textContent = '无效表达式'
        return
      }
      messages.forEach((item, idx) => {
        const haystack = caseSensitive
          ? item.dataset.searchText || ''
          : (item.dataset.searchText || '').toLowerCase()
        if (matcher(haystack)) {
          item.classList.add('search-hit')
          hits.push({ element: item, index: idx })
        }
      })
      counter.textContent = `${hits.length ? 1 : 0} / ${hits.length}`
      if (hits.length) {
        activeIndex = 0
        scrollToHit()
      }
    }

    function scrollToHit() {
      if (activeIndex < 0 || activeIndex >= hits.length) return
      const target = hits[activeIndex].element
      target.scrollIntoView({ behavior: 'smooth', block: 'center' })
      target.classList.add('search-hit-active')
      setTimeout(() => target.classList.remove('search-hit-active'), 1200)
      counter.textContent = `${activeIndex + 1} / ${hits.length}`
    }

    function jump(delta) {
      if (!hits.length) return
      activeIndex = (activeIndex + delta + hits.length) % hits.length
      scrollToHit()
    }

    input.addEventListener('input', updateHits)
    caseBox.addEventListener('change', updateHits)
    regexBox.addEventListener('change', updateHits)
    prevBtn.addEventListener('click', () => jump(-1))
    nextBtn.addEventListener('click', () => jump(1))
    if (closeBtn) {
      closeBtn.addEventListener('click', () => {
        document.body.dataset.searchOpen = 'false'
        refreshDisplayButtons()
      })
    }
    input.addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault()
        jump(event.shiftKey ? -1 : 1)
        return
      }
      if (event.key === 'Escape' && !input.value.trim()) {
        document.body.dataset.searchOpen = 'false'
        refreshDisplayButtons()
      }
    })
    if (panel && input.value.trim()) {
      document.body.dataset.searchOpen = 'true'
    }

    updateHits()
  }

  function createMessageElement(msg) {
    const name = msg.sender_name || '匿名'
    const roleColor = normalizeColor(msg.sender_color)
    const article = document.createElement('article')
    article.className = 'viewer-message'
    article.dataset.messageId = msg.id
    article.dataset.icMode = (msg.ic_mode || 'ic').toLowerCase()
    article.dataset.hasRoleColor = roleColor ? 'true' : 'false'
    if (msg.is_merged) {
      article.classList.add('viewer-message--merged')
    }
    if (roleColor) {
      article.style.setProperty('--role-color', roleColor)
    }
    // 使用 content_html 进行渲染，fallback 到 content
    const displayContent = msg.content_html || msg.content || ''
    const whisperMetaText = formatWhisperMetaText(msg)
    article.dataset.searchText = stripHTML(displayContent) + ' ' + name + ' ' + whisperMetaText

    const avatar = document.createElement('div')
    avatar.className = 'viewer-message__avatar'
    avatar.dataset.hasImage = 'false'
    const avatarSrc = resolveInlineAssetURL(msg.sender_avatar)
    const hasAvatarImage = Boolean(avatarSrc && !avatarSrc.startsWith('id:'))
    if (hasAvatarImage) {
      avatar.dataset.hasImage = 'true'
      const img = document.createElement('img')
      img.src = avatarSrc
      img.alt = name
      avatar.appendChild(img)
    } else {
      const fallback = document.createElement('span')
      fallback.className = 'viewer-message__avatar-text'
      fallback.textContent = initials(name)
      avatar.appendChild(fallback)
    }
    article.appendChild(avatar)

    const main = document.createElement('div')
    main.className = 'viewer-message__main'

    const header = document.createElement('div')
    header.className = 'viewer-message__header'
    const title = document.createElement('div')
    title.className = 'viewer-message__title'
    const sender = document.createElement('strong')
    sender.className = 'viewer-message__sender'
    sender.textContent = name
    title.appendChild(sender)
    if (whisperMetaText) {
      const whisperMeta = document.createElement('span')
      whisperMeta.className = 'viewer-message__whisper-meta'
      whisperMeta.textContent = whisperMetaText
      title.appendChild(whisperMeta)
    }
    const time = document.createElement('span')
    time.className = 'viewer-message__time'
    time.textContent = formatTime(msg.created_at)
    header.appendChild(title)
    header.appendChild(time)
    main.appendChild(header)

    const body = document.createElement('div')
    body.className = 'viewer-message__body'
    body.innerHTML = displayContent
    resolveInlineAssetImages(body)
    const hasImage = body.querySelector('img') !== null
    const bodyText = stripHTML(displayContent).replace(/\s+/g, '').trim()
    if (hasImage && !bodyText) {
      article.classList.add('viewer-message--image-only')
    }
    const bubble = document.createElement('div')
    bubble.className = 'viewer-message__bubble'
    bubble.appendChild(body)
    main.appendChild(bubble)

    article.appendChild(main)
    attachChatSkin(article, msg)
    return article
  }

  if (!app) {
    return
  }
  if (window.__EXPORT_DATA__) {
    renderPart(window.__EXPORT_DATA__)
  } else if (window.__EXPORT_INDEX__) {
    renderIndex(window.__EXPORT_INDEX__)
  } else {
    renderEmpty()
  }
})()
