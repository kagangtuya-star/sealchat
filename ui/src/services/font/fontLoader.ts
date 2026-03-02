import { buildGlobalFontFamilyStack, inferFontFamilyFromFilename, sanitizeFontFamilyName } from './fontUtils'
import { getFontAssetById, touchFontAssetById } from './fontCache'
import type { FontAssetRecord, ImportedFontPayload } from './types'

type LocalFontData = {
  family?: string
  fullName?: string
  postscriptName?: string
}

export interface LocalFontCandidate {
  family: string
  displayName: string
  aliases: string[]
}

const FONT_EXTENSION_RE = /\.(ttf|otf|woff2?|ttc|otc)$/i
const FONT_MIME_RE = /(font\/|application\/font|application\/x-font|application\/vnd\.ms-fontobject|application\/octet-stream)/i
const CJK_RE = /[\u3040-\u30ff\u3400-\u9fff\uf900-\ufaff]/

const getWindowQueryLocalFonts = () => {
  if (typeof window === 'undefined') return null
  const fn = (window as any).queryLocalFonts
  return typeof fn === 'function' ? fn : null
}

const assertFontApi = () => {
  if (typeof document === 'undefined' || !document.fonts) {
    throw new Error('当前环境不支持动态字体加载')
  }
}

const isLikelyFontFile = (name: string, mime: string): boolean => {
  if (FONT_EXTENSION_RE.test(name)) return true
  if (mime && FONT_MIME_RE.test(mime)) return true
  return false
}

const toArrayBuffer = async (blob: Blob): Promise<ArrayBuffer> => blob.arrayBuffer()

export const isLocalFontApiAvailable = (): boolean => !!getWindowQueryLocalFonts()

const pickDisplayName = (family: string, fullName: string, postscriptName: string): string => {
  const candidates = [fullName, family, postscriptName].filter(Boolean)
  const localized = candidates.find(name => CJK_RE.test(name))
  if (localized) return localized
  return fullName || family || postscriptName || family
}

export const queryLocalFontCandidates = async (): Promise<LocalFontCandidate[]> => {
  const queryLocalFonts = getWindowQueryLocalFonts()
  if (!queryLocalFonts) {
    throw new Error('浏览器不支持读取本地字体列表')
  }

  const rows = (await queryLocalFonts()) as LocalFontData[]
  const byFamily = new Map<string, { family: string; displayName: string; aliases: Set<string> }>()

  rows.forEach((item) => {
    const family = sanitizeFontFamilyName(item?.family || '')
    const fullName = sanitizeFontFamilyName(item?.fullName || '')
    const postscriptName = sanitizeFontFamilyName(item?.postscriptName || '')
    const familyKey = family || fullName || postscriptName
    if (!familyKey) return

    const displayName = pickDisplayName(familyKey, fullName, postscriptName)
    const existing = byFamily.get(familyKey)
    if (!existing) {
      byFamily.set(familyKey, {
        family: familyKey,
        displayName,
        aliases: new Set([familyKey, fullName, postscriptName].filter(Boolean)),
      })
      return
    }

    ;[fullName, postscriptName, familyKey].forEach((name) => {
      if (name) existing.aliases.add(name)
    })
    if (!CJK_RE.test(existing.displayName) && CJK_RE.test(displayName)) {
      existing.displayName = displayName
    }
  })

  return Array.from(byFamily.values())
    .sort((a, b) => a.displayName.localeCompare(b.displayName, 'zh-CN'))
    .map(item => ({
      family: item.family,
      displayName: item.displayName,
      aliases: Array.from(item.aliases.values()),
    }))
}

export const queryLocalFontFamilies = async (): Promise<string[]> => {
  const candidates = await queryLocalFontCandidates()
  return candidates.map(item => item.family)
}

export const registerFontFromBlob = async (family: string, blob: Blob): Promise<void> => {
  const normalizedFamily = sanitizeFontFamilyName(family)
  if (!normalizedFamily) {
    throw new Error('字体名称不能为空')
  }
  assertFontApi()
  const buffer = await toArrayBuffer(blob)
  const fontFace = new FontFace(normalizedFamily, buffer)
  await fontFace.load()
  document.fonts.add(fontFace)
}

export const registerCachedFontAsset = async (asset: FontAssetRecord): Promise<void> => {
  await registerFontFromBlob(asset.family, asset.blob)
}

export const restoreCachedFontById = async (id: string): Promise<FontAssetRecord | null> => {
  if (!id) return null
  const asset = await getFontAssetById(id)
  if (!asset) return null
  await registerCachedFontAsset(asset)
  await touchFontAssetById(asset.id)
  return asset
}

export const loadFontFromFile = async (
  file: File,
  preferredFamily?: string,
): Promise<ImportedFontPayload> => {
  if (!file) {
    throw new Error('未选择字体文件')
  }
  if (!isLikelyFontFile(file.name || '', file.type || '')) {
    throw new Error('仅支持 ttf/otf/woff/woff2 等字体文件')
  }
  const inferredFamily = inferFontFamilyFromFilename(file.name)
  const family = sanitizeFontFamilyName(preferredFamily || inferredFamily)
  if (!family) {
    throw new Error('无法识别字体名称，请手动输入')
  }
  await registerFontFromBlob(family, file)
  return {
    family,
    cssFontFamily: buildGlobalFontFamilyStack(family),
    sourceType: 'upload',
    blob: file,
    mime: file.type || 'application/octet-stream',
    size: file.size || 0,
  }
}

export const loadFontFromUrl = async (
  url: string,
  preferredFamily?: string,
): Promise<ImportedFontPayload> => {
  const normalizedUrl = (url || '').trim()
  if (!normalizedUrl) {
    throw new Error('字体 URL 不能为空')
  }
  let parsedUrl: URL
  try {
    parsedUrl = new URL(normalizedUrl)
  } catch {
    throw new Error('字体 URL 格式不正确')
  }
  const response = await fetch(parsedUrl.toString())
  if (!response.ok) {
    throw new Error(`字体下载失败（HTTP ${response.status}）`)
  }
  const blob = await response.blob()
  const mime = response.headers.get('content-type') || blob.type || 'application/octet-stream'
  const filename = parsedUrl.pathname.split('/').pop() || ''
  if (!isLikelyFontFile(filename, mime)) {
    throw new Error('URL 返回内容不是可识别的字体文件')
  }
  const inferredFamily = inferFontFamilyFromFilename(filename)
  const family = sanitizeFontFamilyName(preferredFamily || inferredFamily)
  if (!family) {
    throw new Error('无法识别字体名称，请手动输入')
  }
  await registerFontFromBlob(family, blob)
  return {
    family,
    cssFontFamily: buildGlobalFontFamilyStack(family),
    sourceType: 'url',
    blob,
    mime,
    size: blob.size,
    sourceUrl: parsedUrl.toString(),
  }
}
