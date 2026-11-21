// 聊天装扮渲染模型：把规范化后的装扮 + 当前消息上下文转换为「单条消息的样式覆写层」。
//
// 设计要点：
// - 只输出用户配置过的属性；未配置的属性不出现在内联样式中，从而自然继承当前主题；
// - 气泡模式作用于 `.content`（对话框外形），紧凑模式作用于 `.chat-item` 根节点（整行表现）；
// - 装饰图层全部 `pointer-events: none`，只占视觉，不参与任何交互；
// - 本模块是纯函数，不依赖 store / DOM，供聊天、编辑器预览、导出查看器共用。

import {
  type ChatSkin,
  type ChatSkinAnchor,
  type ChatSkinFont,
  type ChatSkinLayer,
  type ChatSkinLayout,
  type ChatSkinStyle,
  type ChatSkinSurface,
  isChatSkinSurfaceEffective,
  CHAT_SKIN_MAX_LAYER_AXIS,
  parseChatSkinCssDeclarations,
} from './chatSkinSchema'

export type ChatSkinStyleMap = Record<string, string>

export interface ChatSkinResolvedRef {
  src: string
  /** true 表示以遮罩方式渲染（内置单色纹样，可着色）。 */
  mask: boolean
  defaultTint?: string
}

export interface ChatSkinRenderContext {
  layout: ChatSkinLayout
  tone?: string
  isSelf?: boolean
  palette?: 'day' | 'night'
  whisper?: boolean
  /** 资源引用解析：返回 null 表示不可用（图层会被跳过）。 */
  resolveRef?: (ref: string) => ChatSkinResolvedRef | null
  /** 预览中强制展示，即使 scope 为仅场内而当前为场外。 */
  ignoreScope?: boolean
}

export interface ChatSkinLayerRender {
  key: string
  kind: 'image' | 'mask'
  src: string
  front: boolean
  style: ChatSkinStyleMap
}

export interface ChatSkinRender {
  layout: ChatSkinLayout
  rootAttrs: Record<string, string>
  rootClass: string[]
  /** 根节点：CSS 变量与（紧凑模式）整行表面样式。 */
  rootStyle: ChatSkinStyleMap
  /** `.content`：气泡外形（气泡模式）+ 正文表现（两种模式）。 */
  contentStyle: ChatSkinStyleMap
  nameStyle: ChatSkinStyleMap
  layers: ChatSkinLayerRender[]
  reserve: { top: number; bottom: number }
}

export const CHAT_SKIN_FONT_FAMILIES: Record<Exclude<ChatSkinFont, 'inherit'>, string> = {
  sans: '"PingFang SC", "Microsoft YaHei", "Noto Sans SC", system-ui, sans-serif',
  serif: '"Noto Serif SC", "Source Han Serif SC", "Songti SC", "SimSun", serif',
  kai: '"Kaiti SC", "STKaiti", "KaiTi", "BiauKai", serif',
  mono: 'ui-monospace, "SFMono-Regular", "JetBrains Mono", Consolas, monospace',
  rounded: '"Yuanti SC", "YouYuan", "Varela Round", "Nunito", system-ui, sans-serif',
}

const NAME_WEIGHTS = { normal: '400', bold: '700', black: '900' } as const

const RESERVE_LIMIT = 96

const px = (value: number) => `${Math.round(value * 100) / 100}px`

const cssUrl = (src: string) => `url("${src.replace(/["\\\n\r]/g, '')}")`

const mirrorAnchor = (anchor: ChatSkinAnchor): ChatSkinAnchor => (
  anchor.includes('left')
    ? anchor.replace('left', 'right') as ChatSkinAnchor
    : anchor.includes('right')
      ? anchor.replace('right', 'left') as ChatSkinAnchor
      : anchor
)

const anchorPoint = (anchor: ChatSkinAnchor) => ({
  x: anchor.endsWith('left') ? 0 : anchor.endsWith('right') ? 100 : 50,
  y: anchor.startsWith('top') ? 0 : anchor.startsWith('bottom') ? 100 : 50,
})

/** 按分组合并夜间覆写。 */
export const resolveChatSkinSurfaceStyle = (surface: ChatSkinSurface, palette: 'day' | 'night' = 'day'): ChatSkinStyle => (
  palette === 'night' && surface.night ? { ...surface.style, ...surface.night } : surface.style
)

const shouldMirror = (surface: ChatSkinSurface, ctx: ChatSkinRenderContext) => (
  ctx.layout === 'bubble' && !!ctx.isSelf && surface.mirrorOnSelf
)

const resolveLayerGeometry = (layer: ChatSkinLayer, mirrored: boolean) => {
  const flip = mirrored && layer.mirror
  return {
    anchor: flip ? mirrorAnchor(layer.anchor) : layer.anchor,
    x: flip ? -layer.x : layer.x,
    rotate: flip ? -layer.rotate : layer.rotate,
    flipX: flip ? !layer.flipX : layer.flipX,
  }
}

export const getChatSkinLayerDimensions = (layer: ChatSkinLayer) => {
  const width = Math.min(layer.size, CHAT_SKIN_MAX_LAYER_AXIS, CHAT_SKIN_MAX_LAYER_AXIS / layer.aspect)
  return { width, height: width * layer.aspect }
}

/** 使用表面高度下界作保守估算；未知高度按 0 计算，确保中心锚点两侧都得到保护。 */
export const estimateChatSkinLayerOverflow = (layers: ChatSkinLayer[], surfaceMinHeight = 0) => {
  let top = 0
  let bottom = 0
  for (const layer of layers) {
    if (!layer.enabled) continue
    const { width, height } = getChatSkinLayerDimensions(layer)
    const radians = (Math.abs(layer.rotate) * Math.PI) / 180
    const boxHeight = Math.abs(width * Math.sin(radians)) + Math.abs(height * Math.cos(radians))
    const centerY = anchorPoint(layer.anchor).y / 100 * surfaceMinHeight + layer.y
    top = Math.max(top, boxHeight / 2 - centerY)
    bottom = Math.max(bottom, centerY + boxHeight / 2 - surfaceMinHeight)
  }
  return {
    top: Math.min(RESERVE_LIMIT, Math.max(0, Math.ceil(top))),
    bottom: Math.min(RESERVE_LIMIT, Math.max(0, Math.ceil(bottom))),
  }
}

const buildBackground = (style: ChatSkinStyle, ctx: ChatSkinRenderContext, out: ChatSkinStyleMap) => {
  const background = style.background
  if (!background) return
  const image = background.imageRef ? ctx.resolveRef?.(background.imageRef) : null
  const images: string[] = []
  const sizes: string[] = []
  const repeats: string[] = []
  const positions: string[] = []
  if (background.mode === 'gradient') {
    images.push(`linear-gradient(${background.angle}deg, ${background.color}, ${background.color2})`)
    sizes.push('100% 100%')
    repeats.push('no-repeat')
    positions.push('center')
  } else if (background.mode === 'color' && image) {
    // 纯色叠在纹理之上，颜色的透明度即纹理的显现程度。
    images.push(`linear-gradient(${background.color}, ${background.color})`)
    sizes.push('100% 100%')
    repeats.push('no-repeat')
    positions.push('center')
  }
  if (image?.src && !image.mask) {
    images.push(cssUrl(image.src))
    sizes.push(background.imageFit === 'tile' ? 'auto' : background.imageFit === 'stretch' ? '100% 100%' : background.imageFit)
    repeats.push(background.imageFit === 'tile' ? 'repeat' : 'no-repeat')
    positions.push(background.imagePosition)
  }
  out['background-color'] = background.mode === 'color' && !image ? background.color : 'transparent'
  out['background-image'] = images.length ? images.join(', ') : 'none'
  if (images.length) {
    out['background-size'] = sizes.join(', ')
    out['background-repeat'] = repeats.join(', ')
    out['background-position'] = positions.join(', ')
  }
}

const accentShadow = (style: ChatSkinStyle, mirrored: boolean) => {
  const accent = style.accent
  if (!accent) return ''
  let side = accent.side
  if (mirrored && (side === 'left' || side === 'right')) side = side === 'left' ? 'right' : 'left'
  const width = accent.width
  switch (side) {
    case 'right': return `inset -${width}px 0 0 ${accent.color}`
    case 'top': return `inset 0 ${width}px 0 ${accent.color}`
    case 'bottom': return `inset 0 -${width}px 0 ${accent.color}`
    default: return `inset ${width}px 0 0 ${accent.color}`
  }
}

const buildSurfaceStyle = (
  style: ChatSkinStyle,
  ctx: ChatSkinRenderContext,
  mirrored: boolean,
  reserve: { top: number; bottom: number },
  hasBackLayers: boolean,
) => {
  const out: ChatSkinStyleMap = {}
  buildBackground(style, ctx, out)
  if (style.border) {
    out.border = style.border.width > 0 ? `${px(style.border.width)} ${style.border.style} ${style.border.color}` : 'none'
  }
  if (style.radius) {
    const { tl, tr, br, bl } = style.radius
    out['border-radius'] = mirrored
      ? `${px(tr)} ${px(tl)} ${px(bl)} ${px(br)}`
      : `${px(tl)} ${px(tr)} ${px(br)} ${px(bl)}`
  }
  const accent = accentShadow(style, mirrored)
  const shadows: string[] = []
  if (accent) shadows.push(accent)
  if (style.shadow) {
    if (style.shadow.mode === 'custom') {
      const x = mirrored ? -style.shadow.x : style.shadow.x
      shadows.push(`${px(x)} ${px(style.shadow.y)} ${px(style.shadow.blur)} ${px(style.shadow.spread)} ${style.shadow.color}`)
    }
    out['box-shadow'] = shadows.length ? shadows.join(', ') : 'none'
  } else if (accent) {
    // 只配置色条时保留主题自身的投影。
    out['box-shadow'] = ctx.layout === 'bubble' ? `${accent}, var(--chat-message-shadow, 0 0 #0000)` : accent
  }
  if (style.size) {
    if (style.size.minWidth > 0) out['min-width'] = `min(100%, ${px(style.size.minWidth)})`
    if (style.size.minHeight > 0) out['min-height'] = px(style.size.minHeight)
    if (style.size.maxWidth > 0) out['max-width'] = `min(100%, ${px(style.size.maxWidth)})`
  }
  const spacingTop = (style.spacing?.top ?? 0) + reserve.top
  const spacingBottom = (style.spacing?.bottom ?? 0) + reserve.bottom
  if (spacingTop > 0) out['margin-top'] = px(spacingTop)
  if (spacingBottom > 0) out['margin-bottom'] = px(spacingBottom)
  if (style.blur && style.blur > 0) {
    out['backdrop-filter'] = `blur(${px(style.blur)})`
    out['-webkit-backdrop-filter'] = `blur(${px(style.blur)})`
  } else if (hasBackLayers && ctx.layout === 'bubble') {
    // backdrop-filter 会建立层叠上下文，使「背景层」装饰压在气泡底色之上。
    out['backdrop-filter'] = 'none'
    out['-webkit-backdrop-filter'] = 'none'
  }
  return out
}

const buildTextStyle = (style: ChatSkinStyle) => {
  const out: ChatSkinStyleMap = {}
  const text = style.text
  if (!text) return out
  if (text.color) out.color = text.color
  if (text.font) out['font-family'] = CHAT_SKIN_FONT_FAMILIES[text.font as Exclude<ChatSkinFont, 'inherit'>]
  if (text.scale && text.scale !== 1) out['font-size'] = `calc(var(--chat-font-size, 0.95rem) * ${text.scale})`
  if (text.shadow) out['text-shadow'] = `0 1px 2px ${text.shadow}`
  return out
}

const buildNameStyle = (style: ChatSkinStyle) => {
  const out: ChatSkinStyleMap = {}
  const name = style.name
  if (!name) return out
  if (name.color) out.color = name.color
  if (name.font) out['font-family'] = CHAT_SKIN_FONT_FAMILIES[name.font as Exclude<ChatSkinFont, 'inherit'>]
  if (name.weight) out['font-weight'] = NAME_WEIGHTS[name.weight]
  if (name.italic) out['font-style'] = 'italic'
  if (name.chip) {
    out['background-color'] = name.chip
    out.padding = '0 0.5em'
    out['border-radius'] = '999px'
  }
  if (name.shadow) out['text-shadow'] = `0 1px 3px ${name.shadow}`
  return out
}

const buildPaddingVars = (style: ChatSkinStyle, layout: ChatSkinLayout, target: ChatSkinStyleMap) => {
  if (!style.padding) return
  if (layout === 'bubble') {
    // 气泡规则以 0.8 / 0.95 系数消费这两个变量；编辑操作槽位的补白也会继续生效。
    target['--chat-message-padding-y'] = px(style.padding.y / 0.8)
    target['--chat-message-padding-x'] = px(style.padding.x / 0.95)
  } else {
    target.padding = `${px(style.padding.y)} ${px(style.padding.x)}`
  }
}

const buildLayers = (surface: ChatSkinSurface, ctx: ChatSkinRenderContext, mirrored: boolean) => {
  const result: ChatSkinLayerRender[] = []
  surface.layers.forEach((layer) => {
    if (!layer.enabled) return
    const resolved = ctx.resolveRef?.(layer.ref)
    if (!resolved?.src) return
    const geometry = resolveLayerGeometry(layer, mirrored)
    const point = anchorPoint(geometry.anchor)
    const { width, height } = getChatSkinLayerDimensions(layer)
    const style: ChatSkinStyleMap = {
      left: `${point.x}%`,
      top: `${point.y}%`,
      width: px(width),
      height: px(height),
      transform: `translate(-50%, -50%) translate(${px(geometry.x)}, ${px(layer.y)}) rotate(${geometry.rotate}deg) scale(${geometry.flipX ? -1 : 1}, ${layer.flipY ? -1 : 1})`,
      // 前景层统一 z-index 1（同层按 DOM 顺序叠放），消息操作栏等交互控件在其之上。
      'z-index': layer.z === 'back' ? '-1' : '1',
    }
    if (layer.opacity < 1) style.opacity = String(layer.opacity)
    if (layer.blend !== 'normal') style['mix-blend-mode'] = layer.blend
    if (resolved.mask) {
      const mask = `${cssUrl(resolved.src)} center / contain no-repeat`
      style['-webkit-mask'] = mask
      style.mask = mask
      style['background-color'] = layer.tint || resolved.defaultTint || 'currentColor'
    }
    result.push({
      key: layer.id,
      kind: resolved.mask ? 'mask' : 'image',
      src: resolved.src,
      front: layer.z !== 'back',
      style,
    })
  })
  return result
}

/** 是否应对该消息应用装扮（场景范围、悄悄话与归档消息保持主题原样）。 */
export const isChatSkinApplicable = (skin: ChatSkin | null | undefined, ctx: ChatSkinRenderContext) => {
  if (!skin || ctx.whisper) return false
  const tone = ctx.tone || 'ic'
  if (tone === 'archived') return false
  if (!ctx.ignoreScope && skin.scope === 'ic' && tone !== 'ic') return false
  return isChatSkinSurfaceEffective(skin[ctx.layout])
}

export const buildChatSkinRender = (skin: ChatSkin | null | undefined, ctx: ChatSkinRenderContext): ChatSkinRender | null => {
  if (!skin || !isChatSkinApplicable(skin, ctx)) return null
  const surface = skin[ctx.layout] as ChatSkinSurface
  const style = resolveChatSkinSurfaceStyle(surface, ctx.palette)
  const mirrored = shouldMirror(surface, ctx)
  const layers = buildLayers(surface, ctx, mirrored)
  const hasBackLayers = layers.some((layer) => !layer.front)
  const reserve = surface.reserveSpace
    ? estimateChatSkinLayerOverflow(surface.layers.filter((layer) => layers.some((item) => item.key === layer.id)))
    : { top: 0, bottom: 0 }

  const surfaceStyle = buildSurfaceStyle(style, ctx, mirrored, reserve, hasBackLayers)
  const textStyle = buildTextStyle(style)
  const nameStyle = buildNameStyle(style)
  const rootStyle: ChatSkinStyleMap = {}
  const contentStyle: ChatSkinStyleMap = {}

  // 稳定的主题扩展变量：只在配置时出现，第三方主题可 var(--sc-chat-skin-*, fallback) 引用。
  if (style.background && style.background.mode !== 'none') rootStyle['--sc-chat-skin-bg'] = style.background.color
  if (style.border) rootStyle['--sc-chat-skin-border'] = style.border.color
  if (style.accent) rootStyle['--sc-chat-skin-accent'] = style.accent.color
  if (style.text?.color) rootStyle['--sc-chat-skin-text'] = style.text.color
  if (style.name?.color) rootStyle['--sc-chat-skin-name'] = style.name.color

  if (surface.advanced) {
    Object.assign(rootStyle, surface.advanced.vars)
    Object.assign(surfaceStyle, parseChatSkinCssDeclarations(surface.advanced.surfaceCss))
    Object.assign(textStyle, parseChatSkinCssDeclarations(surface.advanced.textCss))
    Object.assign(nameStyle, parseChatSkinCssDeclarations(surface.advanced.nameCss))
  }

  if (ctx.layout === 'bubble') {
    buildPaddingVars(style, 'bubble', contentStyle)
    Object.assign(contentStyle, surfaceStyle, textStyle)
  } else {
    buildPaddingVars(style, 'compact', surfaceStyle)
    Object.assign(rootStyle, surfaceStyle)
    Object.assign(contentStyle, textStyle)
  }
  // 裁切只发生在装饰容器；正文保持正常溢出与交互。
  const hostStyle = ctx.layout === 'bubble' ? contentStyle : rootStyle
  hostStyle['--sc-skin-reserve-top'] = px(reserve.top)
  hostStyle['--sc-skin-reserve-bottom'] = px(reserve.bottom)

  const rootClass = ['chat-item--chat-skin', `chat-item--chat-skin-${ctx.layout}`]
  if (layers.length) rootClass.push('chat-item--chat-skin-layered')
  if (ctx.layout === 'compact' && (
    (style.background && style.background.mode !== 'none')
    || (style.border && style.border.width > 0)
    || style.accent
    || style.shadow?.mode === 'custom'
  )) {
    // 紧凑模式出现整行底色 / 描边 / 色条时，补一个与主题无关的最小内边距与圆角（内联值优先）。
    rootClass.push('chat-item--chat-skin-surface')
  }
  const rootAttrs: Record<string, string> = { 'data-chat-skin': ctx.layout }
  if (skin.meta.presetId) rootAttrs['data-chat-skin-preset'] = skin.meta.presetId

  return {
    layout: ctx.layout,
    rootAttrs,
    rootClass,
    rootStyle,
    contentStyle,
    nameStyle,
    layers,
    reserve,
  }
}
