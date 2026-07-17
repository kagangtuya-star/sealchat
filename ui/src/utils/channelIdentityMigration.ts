export interface MigrationAvatarDecorationSettings {
  scale?: number;
  offsetX?: number;
  offsetY?: number;
  rotation?: number;
  zIndex?: number;
  opacity?: number;
  playbackRate?: number;
  blendMode?: string;
}

export interface MigrationAvatarDecoration {
  id?: string;
  enabled: boolean;
  decorationId?: string;
  resourceAttachmentId?: string;
  fallbackAttachmentId?: string;
  settings?: MigrationAvatarDecorationSettings;
}

export interface IdentityAssetPayload {
  assetKey: string;
  attachmentId?: string;
  hash: string;
  size: number;
  filename?: string;
  mimeType?: string;
  data?: string;
  sourceUrl?: string;
  externalUrl?: string;
  publicUrl?: string;
  presignedUrl?: string;
}

export type IdentityAvatarPayload = Omit<IdentityAssetPayload, 'assetKey'>

export interface IdentityExportDecorationItem extends MigrationAvatarDecoration {
  resourceAssetKey?: string;
  fallbackAssetKey?: string;
}

export interface IdentityExportVariantItem {
  sourceId: string;
  identitySourceId: string;
  selectorEmoji: string;
  keyword: string;
  note: string;
  avatarAssetKey?: string;
  displayName?: string;
  color?: string;
  appearance?: Record<string, any>;
  theaterPresentation?: Record<string, any> | null;
  sortOrder: number;
  enabled: boolean;
}

export interface IdentityExportItem {
  sourceId: string;
  displayName: string;
  color: string;
  isDefault: boolean;
  sortOrder: number;
  folderIds?: string[];
  avatar?: IdentityAvatarPayload;
  avatarDecoration?: IdentityExportDecorationItem | null;
  avatarAssetKey?: string;
  avatarDecorations?: IdentityExportDecorationItem[];
  theaterPresentation?: Record<string, any> | null;
}

export interface IdentityExportFolder {
  sourceId: string;
  name: string;
  sortOrder: number;
  isFavorite?: boolean;
}

export interface IdentityExportFile {
  version: string;
  generatedAt: string;
  source?: {
    channelId?: string;
    channelName?: string;
    guildId?: string;
  };
  items: IdentityExportItem[];
  folders?: IdentityExportFolder[];
  variants?: IdentityExportVariantItem[];
  icOocConfig?: {
    icRoleId?: string | null;
    oocRoleId?: string | null;
  };
  assets?: IdentityAssetPayload[];
}

export const resolveIdentityExportVariantTheaterPresentation = (
  variant?: Pick<IdentityExportVariantItem, 'theaterPresentation' | 'appearance'> | null,
) => {
  if (!variant) return undefined
  if (variant.theaterPresentation !== undefined) return variant.theaterPresentation
  if (variant.appearance && Object.prototype.hasOwnProperty.call(variant.appearance, 'theaterPresentation')) {
    return variant.appearance.theaterPresentation as Record<string, any> | null
  }
  return undefined
}

const identityAssetLookupKeys = (asset: Partial<IdentityAssetPayload>) => {
  const keys: string[] = []
  const attachmentId = String(asset.attachmentId || '').trim()
  if (attachmentId) keys.push(`attachment:${attachmentId.replace(/^id:/, '')}`)
  const hash = String(asset.hash || '').trim()
  const size = Number(asset.size || 0)
  if (hash && size > 0) keys.push(`hash:${hash}:${size}`)
  return keys
}

export const normalizeIdentityExportFileForImport = (
  payload: IdentityExportFile,
  compatibleVersions: readonly string[],
): IdentityExportFile => {
  if (!payload || !compatibleVersions.includes(String(payload.version || ''))) {
    throw new Error('无法识别的导入文件版本')
  }
  if (!Array.isArray(payload.items)) {
    throw new Error('导入文件缺少频道角色列表')
  }

  const normalized = JSON.parse(JSON.stringify(payload)) as IdentityExportFile
  normalized.assets = Array.isArray(normalized.assets) ? normalized.assets : []
  const assetKeyByLookup = new Map<string, string>()
  for (const asset of normalized.assets) {
    if (!asset?.assetKey) continue
    for (const key of identityAssetLookupKeys(asset)) assetKeyByLookup.set(key, asset.assetKey)
  }

  const resolveLegacyAssetKey = (attachmentId?: string) => {
    const normalizedId = String(attachmentId || '').trim().replace(/^id:/, '')
    return normalizedId ? (assetKeyByLookup.get(`attachment:${normalizedId}`) || '') : ''
  }
  const upgradeDecorations = (decorations?: IdentityExportDecorationItem[] | null) => {
    for (const decoration of decorations || []) {
      if (!decoration.resourceAssetKey && decoration.resourceAttachmentId) {
        decoration.resourceAssetKey = resolveLegacyAssetKey(decoration.resourceAttachmentId) || undefined
      }
      if (!decoration.fallbackAssetKey && decoration.fallbackAttachmentId) {
        decoration.fallbackAssetKey = resolveLegacyAssetKey(decoration.fallbackAttachmentId) || undefined
      }
    }
  }
  const upgradeTheaterPresentation = (presentation?: Record<string, any> | null) => {
    if (!presentation) return
    const layers = [
      presentation.portrait,
      ...(Array.isArray(presentation.portraitDecorations) ? presentation.portraitDecorations : []),
      presentation.dialogue?.frame,
    ]
    for (const layer of layers) {
      const media = layer?.media
      if (!media) continue
      if (!media.resourceAssetKey && media.resourceAttachmentId) {
        media.resourceAssetKey = resolveLegacyAssetKey(media.resourceAttachmentId) || undefined
      }
      if (!media.fallbackAssetKey && media.fallbackAttachmentId) {
        media.fallbackAssetKey = resolveLegacyAssetKey(media.fallbackAttachmentId) || undefined
      }
    }
  }

  for (const item of normalized.items) {
    if (!item.avatarAssetKey && item.avatar?.attachmentId) {
      item.avatarAssetKey = resolveLegacyAssetKey(item.avatar.attachmentId) || undefined
    }
    upgradeDecorations(item.avatarDecorations)
    if (item.avatarDecoration) upgradeDecorations([item.avatarDecoration])
    upgradeTheaterPresentation(item.theaterPresentation)
  }
  const variantKeywords = new Map<string, Set<string>>()
  for (const variant of normalized.variants || []) {
    const theaterPresentation = resolveIdentityExportVariantTheaterPresentation(variant)
    if (theaterPresentation !== undefined) {
      variant.theaterPresentation = theaterPresentation
    }
    if (!variant.avatarAssetKey) {
      variant.avatarAssetKey = resolveLegacyAssetKey(String(variant.appearance?.avatarAttachmentId || '')) || undefined
    }
    upgradeTheaterPresentation(variant.theaterPresentation)
    const identitySourceId = String(variant.identitySourceId || '').trim()
    const keyword = String(variant.keyword || '').trim()
    if (!identitySourceId || !keyword || !/^[\p{L}\p{N}_-]{1,64}$/u.test(keyword)) {
      throw new Error('导入文件包含无效差分快捷关键词')
    }
    const keywords = variantKeywords.get(identitySourceId) || new Set<string>()
    const normalizedKeyword = keyword.toLowerCase()
    if (keywords.has(normalizedKeyword)) {
      throw new Error(`导入文件包含重复差分快捷关键词: ${keyword}`)
    }
    keywords.add(normalizedKeyword)
    variantKeywords.set(identitySourceId, keywords)
  }
  return normalized
}

export const buildIdentityAssetKey = (
  meta?: { hash?: string | null; size?: number | null },
  fallbackId = '',
) => {
  const hash = String(meta?.hash || '').trim()
  const size = Number(meta?.size || 0)
  if (hash && size > 0) {
    return `${hash}:${size}`
  }
  return `attachment:${String(fallbackId || '').trim()}`
}

export const normalizeIdentityMatchName = (name?: string | null) => String(name || '').trim().toLowerCase()

export const resolveIdentityMatchByName = <T extends { displayName?: string | null }>(
  items: T[],
  displayName?: string | null,
) => {
  const targetName = normalizeIdentityMatchName(displayName)
  if (!targetName) {
    return null as T | null
  }
  for (const item of items || []) {
    if (normalizeIdentityMatchName(item?.displayName) === targetName) {
      return item
    }
  }
  return null as T | null
}

export const remapDecorationsForImport = (
  decorations: IdentityExportDecorationItem[] | null | undefined,
  assetIdMap: Map<string, string>,
): MigrationAvatarDecoration[] => {
  return (decorations || []).map(item => {
    const resourceAttachmentId = item.resourceAssetKey
      ? (assetIdMap.get(item.resourceAssetKey) || '')
      : ''
    const fallbackAttachmentId = item.fallbackAssetKey
      ? (assetIdMap.get(item.fallbackAssetKey) || '')
      : ''
    if (item.resourceAttachmentId && !item.resourceAssetKey) {
      throw new Error('头像装饰资源来自旧频道且缺少可重建素材')
    }
    if (item.resourceAssetKey && !resourceAttachmentId) {
      throw new Error(`头像装饰资源文件缺失: ${item.resourceAssetKey}`)
    }
    if (item.fallbackAttachmentId && !item.fallbackAssetKey) {
      throw new Error('头像装饰兜底资源来自旧频道且缺少可重建素材')
    }
    if (item.fallbackAssetKey && !fallbackAttachmentId) {
      throw new Error(`头像装饰兜底资源文件缺失: ${item.fallbackAssetKey}`)
    }
    return {
      ...item,
      resourceAttachmentId,
      fallbackAttachmentId,
      settings: item.settings ? { ...item.settings } : undefined,
    }
  })
}

export const resolveIdentityAssetFetchUrl = (options: {
  normalizedId: string;
  externalUrl?: string | null;
  publicUrl?: string | null;
  urlBase: string;
}) => {
  const external = String(options.externalUrl || options.publicUrl || '').trim()
  if (external && /^(https?:|blob:|data:|\/\/|\/)/i.test(external)) {
    return external
  }
  const normalizedId = String(options.normalizedId || '').trim()
  if (!normalizedId) {
    return ''
  }
  return `${String(options.urlBase || '').replace(/\/$/, '')}/api/v1/attachment/${normalizedId}`
}

export const shouldIgnoreIdentityAssetFetchStatus = (status?: number | null) => Number(status || 0) === 404

export const resolveIdentityAssetTransferUrl = (asset?: {
  sourceUrl?: string | null;
  externalUrl?: string | null;
  publicUrl?: string | null;
  presignedUrl?: string | null;
}) => String(asset?.sourceUrl || asset?.externalUrl || asset?.publicUrl || asset?.presignedUrl || '').trim()

export const shouldUseIdentityAssetRemoteImport = (asset?: {
  sourceUrl?: string | null;
  externalUrl?: string | null;
  publicUrl?: string | null;
  presignedUrl?: string | null;
}) => !!resolveIdentityAssetTransferUrl(asset)
