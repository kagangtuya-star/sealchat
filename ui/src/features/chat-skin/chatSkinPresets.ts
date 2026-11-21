// 聊天装扮预设库。预设与用户装扮共用同一数据结构（ChatSkin），
// 未来世界 / 平台预设只需提供同样结构的条目即可接入编辑器。

import {
  type ChatSkin,
  type ChatSkinLayer,
  type ChatSkinStyle,
  type ChatSkinSurface,
  createDefaultChatSkinLayer,
  createEmptyChatSkinSurface,
  normalizeChatSkin,
} from './chatSkinSchema'

export type ChatSkinPresetSource = 'builtin' | 'mine' | 'identity'

export interface ChatSkinPreset {
  id: string
  name: string
  description: string
  source: ChatSkinPresetSource
  skin: ChatSkin
}

const layer = (ref: string, patch: Partial<ChatSkinLayer>): ChatSkinLayer => createDefaultChatSkinLayer(ref, {
  id: `${ref.replace(/[^a-z0-9]+/gi, '-')}-${patch.anchor || 'a'}-${patch.x ?? 0}-${patch.y ?? 0}`,
  ...patch,
})

const surface = (style: ChatSkinStyle, layers: ChatSkinLayer[] = [], extra: Partial<ChatSkinSurface> = {}): ChatSkinSurface => ({
  ...createEmptyChatSkinSurface(),
  style,
  layers,
  ...extra,
})

const preset = (id: string, name: string, description: string, raw: Omit<ChatSkin, 'version' | 'meta'>): ChatSkinPreset => ({
  id: `builtin:${id}`,
  name,
  description,
  source: 'builtin',
  skin: normalizeChatSkin({ ...raw, version: 1, meta: { name, presetId: `builtin:${id}` } }) as ChatSkin,
})

const radius = (tl: number, tr = tl, br = tl, bl = tl) => ({ tl, tr, br, bl })

export const BUILTIN_CHAT_SKIN_PRESETS: ChatSkinPreset[] = [
  preset('paper', '旧纸笺', '暖色纸面、细描边与楷体正文，适合书信与旁白感的角色。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'color', color: '#fdf6e6', color2: '#f5ead2', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 1, style: 'solid', color: '#dcc7a1' },
      radius: radius(4, 10, 10, 10),
      shadow: { mode: 'custom', color: '#7a5b2e24', blur: 10, x: 0, y: 3, spread: 0 },
      text: { color: '#4a3b28', font: 'kai' },
      name: { color: '#8a6232', font: 'kai', weight: 'bold' },
    }, [
      layer('builtin:tail', { anchor: 'top-left', x: -5, y: 9, size: 12, z: 'back', tint: '#dcc7a1' }),
      layer('builtin:tail', { anchor: 'top-left', x: -3, y: 9, size: 11, z: 'back', tint: '#fdf6e6' }),
    ]),
    compact: surface({
      background: { mode: 'color', color: '#fdf6e6b3', color2: '#f5ead2', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      accent: { color: '#c9a66b', width: 3, side: 'left' },
      text: { color: '#4a3b28', font: 'kai' },
      name: { color: '#8a6232', font: 'kai', weight: 'bold' },
    }),
  }),
  preset('neko', '猫耳软糖', '粉色圆角气泡顶上一对猫耳，右下角带肉垫。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'color', color: '#fff0f4', color2: '#ffe3ea', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 1.5, style: 'solid', color: '#f5b8c8' },
      radius: radius(6, 18, 18, 18),
      size: { minWidth: 96, minHeight: 0, maxWidth: 0 },
      text: { color: '#5b3a45' },
      name: { color: '#e0708f', weight: 'bold' },
    }, [
      layer('builtin:cat-ear', { anchor: 'top-left', x: 20, y: -9, size: 24, rotate: -14, tint: '#f5b8c8', z: 'back' }),
      layer('builtin:cat-ear', { anchor: 'top-left', x: 50, y: -9, size: 24, rotate: 14, tint: '#f5b8c8', z: 'back' }),
      layer('builtin:paw', { anchor: 'bottom-right', x: -10, y: -8, size: 14, opacity: 0.55, tint: '#e0708f' }),
    ]),
    compact: surface({
      background: { mode: 'color', color: '#fff0f499', color2: '#ffe3ea', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      accent: { color: '#f5b8c8', width: 4, side: 'left' },
      name: { color: '#e0708f', weight: 'bold' },
    }, [
      layer('builtin:paw', { anchor: 'right', x: -16, y: 0, size: 16, opacity: 0.35, tint: '#e0708f' }),
    ]),
  }),
  preset('starry', '星夜', '深蓝紫渐变、月牙与星光，夜间与暗色主题下尤其合适。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'gradient', color: '#1f2a4d', color2: '#43306a', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 1, style: 'solid', color: '#ffffff26' },
      radius: radius(14),
      shadow: { mode: 'custom', color: '#43306a59', blur: 16, x: 0, y: 6, spread: 0 },
      text: { color: '#eef0ff' },
      name: { color: '#ffd76a', weight: 'bold', shadow: '#ffd76a66' },
    }, [
      layer('builtin:moon', { anchor: 'top-right', x: -6, y: 2, size: 20, rotate: -20, tint: '#ffe08a' }),
      layer('builtin:sparkle', { anchor: 'top-right', x: -30, y: -2, size: 10, tint: '#fff3c4' }),
      layer('builtin:sparkle', { anchor: 'bottom-left', x: 10, y: -6, size: 8, opacity: 0.7, tint: '#fff3c4' }),
    ]),
    compact: surface({
      background: { mode: 'gradient', color: '#1f2a4dcc', color2: '#43306a99', angle: 90, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      accent: { color: '#ffd76a', width: 3, side: 'left' },
      text: { color: '#eef0ff' },
      name: { color: '#ffd76a', weight: 'bold' },
    }, [
      layer('builtin:sparkle', { anchor: 'top-right', x: -18, y: 8, size: 10, opacity: 0.8, tint: '#fff3c4' }),
    ]),
  }),
  preset('sakura', '樱吹雪', '淡粉渐变与角落花朵，柔和不抢正文。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'gradient', color: '#fff7f9', color2: '#ffe2ea', angle: 160, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 1, style: 'solid', color: '#f7c6d3' },
      radius: radius(16),
      text: { color: '#5a3d47' },
      name: { color: '#d4567a', weight: 'bold' },
    }, [
      layer('builtin:flower', { anchor: 'top-right', x: -2, y: 0, size: 22, rotate: 18, tint: '#f29cb7' }),
      layer('builtin:flower', { anchor: 'top-right', x: -20, y: -6, size: 13, rotate: -10, opacity: 0.75, tint: '#f7b9cb' }),
      layer('builtin:flower', { anchor: 'bottom-left', x: 6, y: -2, size: 12, opacity: 0.6, tint: '#f7b9cb' }),
    ], {
      night: {
        background: { mode: 'gradient', color: '#4a2f3a', color2: '#5e3446', angle: 160, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
        border: { width: 1, style: 'solid', color: '#f29cb766' },
        text: { color: '#ffe6ee' },
        name: { color: '#ff9fbd', weight: 'bold' },
      },
    }),
    compact: surface({
      background: { mode: 'gradient', color: '#ffe2ea99', color2: '#ffffff00', angle: 90, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      name: { color: '#d4567a', weight: 'bold' },
    }, [
      layer('builtin:flower', { anchor: 'left', x: 6, y: 0, size: 14, opacity: 0.5, tint: '#f29cb7', z: 'back' }),
    ]),
  }),
  preset('ink', '水墨朱印', '素净宣纸、墨色侧线与右下角朱红图章。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'color', color: '#f8f6f1', color2: '#efebe2', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 0, style: 'solid', color: '#2b2b2b' },
      radius: radius(2),
      shadow: { mode: 'none', color: '#00000000', blur: 0, x: 0, y: 0, spread: 0 },
      accent: { color: '#2b2b2b', width: 3, side: 'left' },
      padding: { x: 18, y: 10 },
      text: { color: '#262626', font: 'kai' },
      name: { color: '#262626', font: 'kai', weight: 'black' },
    }, [
      layer('builtin:seal', { anchor: 'bottom-right', x: -8, y: -6, size: 22, rotate: -8, opacity: 0.85, tint: '#c0392b' }),
    ]),
    compact: surface({
      accent: { color: '#2b2b2b', width: 2, side: 'left' },
      text: { font: 'kai' },
      name: { font: 'kai', weight: 'black' },
    }, [
      layer('builtin:seal', { anchor: 'right', x: -18, y: 0, size: 18, opacity: 0.6, tint: '#c0392b' }),
    ]),
  }),
  preset('royal', '金饰宫廷', '双线金边、角花与王冠，适合贵族或神祇角色。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'gradient', color: '#fffaf0', color2: '#fbf0d6', angle: 180, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 3, style: 'double', color: '#c9a227' },
      radius: radius(6),
      padding: { x: 18, y: 12 },
      text: { color: '#3f3420', font: 'serif' },
      name: { color: '#8a6d1d', font: 'serif', weight: 'black' },
    }, [
      layer('builtin:crown', { anchor: 'top-left', x: 18, y: -4, size: 26, rotate: -10, tint: '#e3b341' }),
      layer('builtin:corner', { anchor: 'bottom-right', x: -10, y: -10, size: 18, rotate: 180, tint: '#c9a227', opacity: 0.8 }),
    ]),
    compact: surface({
      background: { mode: 'gradient', color: '#fbf0d6cc', color2: '#fffaf000', angle: 90, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      accent: { color: '#c9a227', width: 3, side: 'left' },
      name: { color: '#8a6d1d', font: 'serif', weight: 'black' },
    }),
  }),
  preset('mint', '薄荷胶囊', '大圆角薄荷气泡与胶囊式名牌，简洁清爽。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'color', color: '#e8f8f1', color2: '#d3f2e5', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 0, style: 'solid', color: '#9fdcc4' },
      radius: radius(20),
      shadow: { mode: 'none', color: '#00000000', blur: 0, x: 0, y: 0, spread: 0 },
      text: { color: '#1f4a3c' },
      name: { color: '#16624a', chip: '#c3eedd', weight: 'bold' },
    }),
    compact: surface({
      name: { color: '#16624a', chip: '#c3eedd', weight: 'bold' },
    }),
  }),
  preset('terminal', '终端', '深色等宽字体与荧光正文，适合 AI、黑客或系统角色。', {
    scope: 'all',
    bubble: surface({
      background: { mode: 'color', color: '#0d1117', color2: '#161b22', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 1, style: 'solid', color: '#30363d' },
      radius: radius(6),
      text: { color: '#7ee787', font: 'mono', shadow: '#7ee78740' },
      name: { color: '#58a6ff', font: 'mono', weight: 'bold' },
    }),
    compact: surface({
      background: { mode: 'color', color: '#0d1117e6', color2: '#161b22', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      text: { color: '#7ee787', font: 'mono' },
      name: { color: '#58a6ff', font: 'mono', weight: 'bold' },
    }),
  }),
  preset('lace', '蕾丝蝴蝶结', '白色气泡底部垂下蕾丝花边，顶部系一个蝴蝶结。', {
    scope: 'ic',
    bubble: surface({
      background: { mode: 'color', color: '#ffffff', color2: '#fff5f8', angle: 135, imageRef: '', imageFit: 'cover', imagePosition: 'center' },
      border: { width: 1, style: 'solid', color: '#f0c6d4' },
      radius: radius(12, 12, 4, 4),
      size: { minWidth: 140, minHeight: 0, maxWidth: 0 },
      text: { color: '#4b3a40' },
      name: { color: '#c2577a', weight: 'bold' },
    }, [
      layer('builtin:scallop', { anchor: 'bottom', x: 0, y: 6, size: 132, z: 'back', tint: '#f0c6d4' }),
      layer('builtin:bow', { anchor: 'top', x: 0, y: -3, size: 30, tint: '#e85d75' }),
    ]),
    compact: surface({
      accent: { color: '#f0c6d4', width: 4, side: 'left' },
      name: { color: '#c2577a', weight: 'bold' },
    }, [
      layer('builtin:bow', { anchor: 'left', x: 12, y: -12, size: 18, rotate: -16, tint: '#e85d75' }),
    ]),
  }),
]

/** 从气泡设计派生一份适合整行展示的紧凑表现：保留色彩与文字，去掉对话框外形。 */
export const deriveCompactSurfaceFromBubble = (bubble: ChatSkinSurface | null): ChatSkinSurface => {
  const next = createEmptyChatSkinSurface()
  if (!bubble) return next
  const style = bubble.style
  if (style.background && style.background.mode !== 'none') {
    const alpha = (color: string) => (color.length === 7 ? `${color}99` : color)
    next.style.background = {
      ...style.background,
      color: alpha(style.background.color),
      color2: style.background.mode === 'gradient' ? alpha(style.background.color2) : style.background.color2,
      angle: style.background.mode === 'gradient' ? 90 : style.background.angle,
    }
  }
  const accentColor = style.accent?.color || style.border?.color || style.name?.color
  if (accentColor) next.style.accent = { color: accentColor, width: 3, side: 'left' }
  if (style.text) next.style.text = { ...style.text }
  if (style.name) next.style.name = { ...style.name }
  if (bubble.night) {
    const night: ChatSkinStyle = {}
    if (bubble.night.text) night.text = { ...bubble.night.text }
    if (bubble.night.name) night.name = { ...bubble.night.name }
    next.night = Object.keys(night).length ? night : null
  }
  return next
}
