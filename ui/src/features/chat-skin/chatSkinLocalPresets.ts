// 「我的预设」：保存在本机浏览器的装扮收藏。只存规范化后的装扮（附件以 `id:` 引用），
// 不含任何图片数据；跨设备迁移请使用装扮文件导出。

import { normalizeChatSkin } from './chatSkinSchema'
import type { ChatSkinPreset } from './chatSkinPresets'

const STORAGE_KEY = 'sealchat_chat_skin_presets_v1'
export const CHAT_SKIN_LOCAL_PRESET_LIMIT = 24

const readRaw = (): unknown[] => {
  try {
    const raw = globalThis.localStorage?.getItem(STORAGE_KEY)
    const parsed = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

export const loadChatSkinLocalPresets = (): ChatSkinPreset[] => {
  const result: ChatSkinPreset[] = []
  for (const item of readRaw()) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const skin = normalizeChatSkin(record.skin)
    const id = typeof record.id === 'string' && /^mine:[A-Za-z0-9_-]{1,40}$/.test(record.id) ? record.id : ''
    if (!skin || !id) continue
    result.push({
      id,
      name: typeof record.name === 'string' && record.name.trim() ? record.name.trim().slice(0, 40) : '未命名预设',
      description: typeof record.description === 'string' ? record.description.slice(0, 80) : '',
      source: 'mine',
      skin,
    })
  }
  return result.slice(0, CHAT_SKIN_LOCAL_PRESET_LIMIT)
}

const write = (presets: ChatSkinPreset[]) => {
  try {
    globalThis.localStorage?.setItem(STORAGE_KEY, JSON.stringify(presets.map((preset) => ({
      id: preset.id,
      name: preset.name,
      description: preset.description,
      skin: preset.skin,
    }))))
    return true
  } catch {
    return false
  }
}

export const saveChatSkinLocalPreset = (name: string, skin: unknown): ChatSkinPreset[] | null => {
  const normalized = normalizeChatSkin(skin)
  if (!normalized) return null
  const id = `mine:${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  const preset: ChatSkinPreset = {
    id,
    name: name.trim().slice(0, 40) || '未命名预设',
    description: '',
    source: 'mine',
    skin: { ...normalized, meta: { name: name.trim().slice(0, 40), presetId: '' } },
  }
  const next = [preset, ...loadChatSkinLocalPresets()].slice(0, CHAT_SKIN_LOCAL_PRESET_LIMIT)
  return write(next) ? next : null
}

export const removeChatSkinLocalPreset = (id: string): ChatSkinPreset[] => {
  const next = loadChatSkinLocalPresets().filter((preset) => preset.id !== id)
  write(next)
  return next
}
