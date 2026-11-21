// 聊天装扮的导入 / 导出：与频道角色导出共用素材载荷结构（IdentityAssetPayload），
// 装扮中的 `id:<附件>` 在文件里改写为 `asset:<素材键>`，导入时再重建为目标频道的附件。
// 内置纹样 `builtin:*` 原样保留。文件是普通 JSON，不依赖浏览器私有格式。

import {
  type ChatSkin,
  type ChatSkinPatch,
  isPlainRecord,
  normalizeChatSkin,
  normalizeChatSkinPatch,
} from './chatSkinSchema'

export const CHAT_SKIN_EXPORT_FORMAT = 'sealchat.chat-skin/v1'
export const CHAT_SKIN_ASSET_REF_PREFIX = 'asset:'

/** 与 utils/channelIdentityMigration 的 IdentityAssetPayload 结构一致。 */
export interface ChatSkinAssetPayload {
  assetKey: string
  attachmentId?: string
  hash: string
  size: number
  filename?: string
  mimeType?: string
  data?: string
  sourceUrl?: string
  externalUrl?: string
  publicUrl?: string
}

export interface ChatSkinExportFile {
  format: typeof CHAT_SKIN_EXPORT_FORMAT
  exportedAt: string
  name: string
  skin: Record<string, unknown> | null
  /** 可选：来源差分的局部覆写（差分模式导出时携带）。 */
  patch?: Record<string, unknown> | null
  assets: ChatSkinAssetPayload[]
}

type RefMapper = (ref: string) => string

/**
 * 遍历装扮文档（完整装扮或差分 skin 片段）中的资源引用：图层 `ref` 与背景 `imageRef`。
 * 返回空字符串表示丢弃：图层被移除、背景图被清空。与服务端 MapChatSkinAttachmentRefs 一致。
 */
export const mapChatSkinDocumentRefs = (document: unknown, mapRef: RefMapper): unknown => {
  if (Array.isArray(document)) return document.map((item) => mapChatSkinDocumentRefs(item, mapRef))
  if (!isPlainRecord(document)) return document
  const result: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(document)) {
    if (key === 'imageRef' && typeof value === 'string') {
      result[key] = value ? mapRef(value) : value
      continue
    }
    if (key === 'layers' && Array.isArray(value)) {
      result[key] = value
        .map((item) => mapChatSkinDocumentRefs(item, mapRef))
        .filter((item) => {
          if (!isPlainRecord(item) || typeof item.ref !== 'string') return isPlainRecord(item)
          item.ref = mapRef(item.ref)
          return !!item.ref
        })
      continue
    }
    result[key] = mapChatSkinDocumentRefs(value, mapRef)
  }
  return result
}

const collectRefs = (document: unknown, out: Set<string>) => {
  mapChatSkinDocumentRefs(document, (ref) => {
    out.add(ref)
    return ref
  })
}

/** 文档中引用的附件 ID（不含 `id:` 前缀）。 */
export const collectChatSkinDocumentAttachmentIds = (...documents: unknown[]) => {
  const refs = new Set<string>()
  documents.forEach((document) => collectRefs(document, refs))
  return [...refs].filter((ref) => ref.startsWith('id:')).map((ref) => ref.slice(3))
}

/** 导出：附件引用替换为 `asset:<键>`；registry 返回空字符串的附件会被丢弃。 */
export const exportChatSkinDocument = (document: unknown, assetKeyByAttachmentId: Map<string, string>) => {
  if (!isPlainRecord(document)) return null
  return mapChatSkinDocumentRefs(JSON.parse(JSON.stringify(document)), (ref) => {
    if (!ref.startsWith('id:')) return ref.startsWith('builtin:') ? ref : ''
    const key = assetKeyByAttachmentId.get(ref.slice(3))
    return key ? `${CHAT_SKIN_ASSET_REF_PREFIX}${key}` : ''
  }) as Record<string, unknown>
}

/**
 * 导入：`asset:<键>` 映射为新附件；未知的 `id:` 引用默认丢弃（跨频道 / 跨实例不可用），
 * keepAttachmentRefs 为 true 时保留（同一账号内复制）。
 */
export const importChatSkinDocument = (
  document: unknown,
  attachmentIdByAssetKey: Map<string, string>,
  options: { keepAttachmentRefs?: boolean } = {},
) => {
  if (!isPlainRecord(document)) return null
  return mapChatSkinDocumentRefs(JSON.parse(JSON.stringify(document)), (ref) => {
    if (ref.startsWith(CHAT_SKIN_ASSET_REF_PREFIX)) {
      const attachmentId = attachmentIdByAssetKey.get(ref.slice(CHAT_SKIN_ASSET_REF_PREFIX.length))
      return attachmentId ? `id:${attachmentId.replace(/^id:/, '')}` : ''
    }
    if (ref.startsWith('builtin:')) return ref
    if (ref.startsWith('id:')) return options.keepAttachmentRefs ? ref : ''
    return ''
  }) as Record<string, unknown>
}

export const importChatSkin = (
  document: unknown,
  attachmentIdByAssetKey: Map<string, string>,
  options: { keepAttachmentRefs?: boolean } = {},
): ChatSkin | null => normalizeChatSkin(importChatSkinDocument(document, attachmentIdByAssetKey, options))

export const exportChatSkinPatch = (patch: ChatSkinPatch | null, assetKeyByAttachmentId: Map<string, string>) => {
  if (!patch) return null
  return {
    mode: patch.mode,
    skin: patch.skin ? exportChatSkinDocument(patch.skin, assetKeyByAttachmentId) : null,
  }
}

export const importChatSkinPatch = (
  raw: unknown,
  attachmentIdByAssetKey: Map<string, string>,
  options: { keepAttachmentRefs?: boolean } = {},
): ChatSkinPatch | null => {
  if (!isPlainRecord(raw)) return null
  return normalizeChatSkinPatch({
    mode: raw.mode,
    skin: isPlainRecord(raw.skin) ? importChatSkinDocument(raw.skin, attachmentIdByAssetKey, options) : raw.skin,
  })
}

export const buildChatSkinExportFile = (options: {
  name: string
  skin: Record<string, unknown> | null
  patch?: Record<string, unknown> | null
  assets: ChatSkinAssetPayload[]
}): ChatSkinExportFile => ({
  format: CHAT_SKIN_EXPORT_FORMAT,
  exportedAt: new Date().toISOString(),
  name: options.name.slice(0, 40),
  skin: options.skin,
  ...(options.patch ? { patch: options.patch } : {}),
  assets: options.assets,
})

const ASSET_KEY = /^[A-Za-z0-9:_.-]{1,160}$/

/** 解析装扮文件；也接受直接粘贴的裸装扮 JSON（无素材）。 */
export const parseChatSkinExportFile = (text: string): ChatSkinExportFile => {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    throw new Error('文件不是有效的 JSON')
  }
  if (!isPlainRecord(raw)) throw new Error('装扮文件格式不正确')
  if (raw.format === undefined && (isPlainRecord(raw.bubble) || isPlainRecord(raw.compact))) {
    return buildChatSkinExportFile({ name: '', skin: raw, assets: [] })
  }
  if (raw.format !== CHAT_SKIN_EXPORT_FORMAT) throw new Error('不支持的装扮文件版本')
  const assets = Array.isArray(raw.assets)
    ? raw.assets.filter((item): item is ChatSkinAssetPayload => (
      isPlainRecord(item) && typeof item.assetKey === 'string' && ASSET_KEY.test(item.assetKey)
    ))
    : []
  return {
    format: CHAT_SKIN_EXPORT_FORMAT,
    exportedAt: typeof raw.exportedAt === 'string' ? raw.exportedAt : '',
    name: typeof raw.name === 'string' ? raw.name.slice(0, 40) : '',
    skin: isPlainRecord(raw.skin) ? raw.skin : null,
    patch: isPlainRecord(raw.patch) ? raw.patch : null,
    assets,
  }
}

/** 仅保留文档实际引用到的素材，避免把无关二进制带进文件。 */
export const pickReferencedChatSkinAssets = (assets: ChatSkinAssetPayload[], ...documents: unknown[]) => {
  const refs = new Set<string>()
  documents.forEach((document) => collectRefs(document, refs))
  return assets.filter((asset) => refs.has(`${CHAT_SKIN_ASSET_REF_PREFIX}${asset.assetKey}`))
}
