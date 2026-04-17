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
  data: string;
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
  return (decorations || []).map(item => ({
    ...item,
    resourceAttachmentId: item.resourceAssetKey
      ? (assetIdMap.get(item.resourceAssetKey) || item.resourceAttachmentId || '')
      : (item.resourceAttachmentId || ''),
    fallbackAttachmentId: item.fallbackAssetKey
      ? (assetIdMap.get(item.fallbackAssetKey) || item.fallbackAttachmentId || '')
      : (item.fallbackAttachmentId || ''),
    settings: item.settings ? { ...item.settings } : undefined,
  }))
}

export const resolveIdentityAssetFetchUrl = (options: {
  normalizedId: string;
  externalUrl?: string | null;
  publicUrl?: string | null;
  urlBase: string;
}) => {
  const external = String(options.externalUrl || options.publicUrl || '').trim()
  if (external) {
    return external
  }
  const normalizedId = String(options.normalizedId || '').trim()
  if (!normalizedId) {
    return ''
  }
  return `${String(options.urlBase || '').replace(/\/$/, '')}/api/v1/attachment/${normalizedId}`
}

export const shouldIgnoreIdentityAssetFetchStatus = (status?: number | null) => Number(status || 0) === 404
