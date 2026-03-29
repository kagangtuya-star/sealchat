import { getFontAssetById, isFontAssetCacheAvailable, saveFontAsset } from '@/services/font/fontCache'
import { sanitizeFontFamilyName } from '@/services/font/fontUtils'
import type { FontAssetRecord } from '@/services/font/types'
import type { DisplaySettings } from '@/stores/display'

const TRANSFER_KIND = 'sealchat-display-settings'
const TRANSFER_VERSION = 1

type TransferFormat = 'json' | 'zip'
type BundledFontSourceType = 'upload' | 'url'

interface BundledFontManifest {
  assetId: string
  entryName: string
  family: string
  sourceType: BundledFontSourceType
  mime: string
  size: number
  sourceUrl?: string
}

interface DisplaySettingsTransferManifest {
  kind: typeof TRANSFER_KIND
  version: number
  exportedAt: string
  settings: Partial<DisplaySettings>
  bundledFont?: BundledFontManifest | null
}

interface ZipEntry {
  name: string
  data: Uint8Array
  comment?: string
  lastModified?: Date
}

interface ParsedZipEntry {
  name: string
  data: Uint8Array
}

export interface DisplaySettingsExportResult {
  blob: Blob
  filename: string
  format: TransferFormat
  includedFont: boolean
  warnings: string[]
}

export interface DisplaySettingsImportResult {
  settings: Partial<DisplaySettings>
  format: TransferFormat
  importedFont: boolean
  warnings: string[]
}

const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8')

const cloneSettings = (settings: Partial<DisplaySettings>): Partial<DisplaySettings> =>
  JSON.parse(JSON.stringify(settings || {})) as Partial<DisplaySettings>

const formatTimestamp = (date = new Date()): string => {
  const pad = (value: number) => String(value).padStart(2, '0')
  return [
    date.getFullYear(),
    pad(date.getMonth() + 1),
    pad(date.getDate()),
    '-',
    pad(date.getHours()),
    pad(date.getMinutes()),
    pad(date.getSeconds()),
  ].join('')
}

const sanitizeFilenameSegment = (value: string): string => {
  const normalized = (value || '').trim().replace(/[^\w\u4e00-\u9fa5-]+/g, '_')
  return normalized || 'default'
}

const triggerDownload = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

export const downloadDisplaySettingsTransfer = (blob: Blob, filename: string) => {
  triggerDownload(blob, filename)
}

const fixSnapshotWithoutBundledFont = (settings: Partial<DisplaySettings>) => {
  const sourceType = settings.globalFontSourceType
  if (sourceType !== 'upload' && sourceType !== 'url') {
    return settings
  }
  const family = sanitizeFontFamilyName(settings.globalFontFamily || '')
  settings.globalFontAssetId = null
  settings.globalFontSourceType = family ? 'manual' : 'default'
  settings.globalFontFamily = family
  return settings
}

const readDosTime = (date: Date) => {
  const year = Math.max(date.getFullYear(), 1980)
  const month = date.getMonth() + 1
  const day = date.getDate()
  const hours = date.getHours()
  const minutes = date.getMinutes()
  const seconds = Math.floor(date.getSeconds() / 2)
  return {
    time: (hours << 11) | (minutes << 5) | seconds,
    date: ((year - 1980) << 9) | (month << 5) | day,
  }
}

const crcTable = (() => {
  const table = new Uint32Array(256)
  for (let i = 0; i < 256; i += 1) {
    let c = i
    for (let j = 0; j < 8; j += 1) {
      c = (c & 1) ? (0xedb88320 ^ (c >>> 1)) : (c >>> 1)
    }
    table[i] = c >>> 0
  }
  return table
})()

const crc32 = (bytes: Uint8Array): number => {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc = crcTable[(crc ^ byte) & 0xff] ^ (crc >>> 8)
  }
  return (crc ^ 0xffffffff) >>> 0
}

const concatUint8Arrays = (parts: Uint8Array[]): Uint8Array => {
  const total = parts.reduce((sum, part) => sum + part.length, 0)
  const merged = new Uint8Array(total)
  let offset = 0
  for (const part of parts) {
    merged.set(part, offset)
    offset += part.length
  }
  return merged
}

const createZip = (entries: ZipEntry[]): Uint8Array => {
  const localParts: Uint8Array[] = []
  const centralParts: Uint8Array[] = []
  let offset = 0

  entries.forEach((entry) => {
    const nameBytes = encoder.encode(entry.name)
    const commentBytes = entry.comment ? encoder.encode(entry.comment) : new Uint8Array(0)
    const { time, date } = readDosTime(entry.lastModified || new Date())
    const data = entry.data
    const crc = crc32(data)

    const localHeader = new Uint8Array(30 + nameBytes.length)
    const localView = new DataView(localHeader.buffer)
    localView.setUint32(0, 0x04034b50, true)
    localView.setUint16(4, 20, true)
    localView.setUint16(6, 0x0800, true)
    localView.setUint16(8, 0, true)
    localView.setUint16(10, time, true)
    localView.setUint16(12, date, true)
    localView.setUint32(14, crc, true)
    localView.setUint32(18, data.length, true)
    localView.setUint32(22, data.length, true)
    localView.setUint16(26, nameBytes.length, true)
    localView.setUint16(28, 0, true)
    localHeader.set(nameBytes, 30)

    const centralHeader = new Uint8Array(46 + nameBytes.length + commentBytes.length)
    const centralView = new DataView(centralHeader.buffer)
    centralView.setUint32(0, 0x02014b50, true)
    centralView.setUint16(4, 20, true)
    centralView.setUint16(6, 20, true)
    centralView.setUint16(8, 0x0800, true)
    centralView.setUint16(10, 0, true)
    centralView.setUint16(12, time, true)
    centralView.setUint16(14, date, true)
    centralView.setUint32(16, crc, true)
    centralView.setUint32(20, data.length, true)
    centralView.setUint32(24, data.length, true)
    centralView.setUint16(28, nameBytes.length, true)
    centralView.setUint16(30, 0, true)
    centralView.setUint16(32, commentBytes.length, true)
    centralView.setUint16(34, 0, true)
    centralView.setUint16(36, 0, true)
    centralView.setUint32(38, 0, true)
    centralView.setUint32(42, offset, true)
    centralHeader.set(nameBytes, 46)
    if (commentBytes.length > 0) {
      centralHeader.set(commentBytes, 46 + nameBytes.length)
    }

    localParts.push(localHeader, data)
    centralParts.push(centralHeader)
    offset += localHeader.length + data.length
  })

  const centralDirectory = concatUint8Arrays(centralParts)
  const localData = concatUint8Arrays(localParts)
  const eocd = new Uint8Array(22)
  const eocdView = new DataView(eocd.buffer)
  eocdView.setUint32(0, 0x06054b50, true)
  eocdView.setUint16(4, 0, true)
  eocdView.setUint16(6, 0, true)
  eocdView.setUint16(8, entries.length, true)
  eocdView.setUint16(10, entries.length, true)
  eocdView.setUint32(12, centralDirectory.length, true)
  eocdView.setUint32(16, localData.length, true)
  eocdView.setUint16(20, 0, true)

  return concatUint8Arrays([localData, centralDirectory, eocd])
}

const findEndOfCentralDirectory = (bytes: Uint8Array): number => {
  const minOffset = Math.max(0, bytes.length - 65557)
  for (let i = bytes.length - 22; i >= minOffset; i -= 1) {
    if (
      bytes[i] === 0x50
      && bytes[i + 1] === 0x4b
      && bytes[i + 2] === 0x05
      && bytes[i + 3] === 0x06
    ) {
      return i
    }
  }
  return -1
}

const inflateDeflateRaw = async (bytes: Uint8Array): Promise<Uint8Array> => {
  if (typeof DecompressionStream === 'undefined') {
    throw new Error('当前浏览器不支持解压缩 ZIP 条目')
  }
  const stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream('deflate-raw'))
  return new Uint8Array(await new Response(stream).arrayBuffer())
}

const parseZip = async (bytes: Uint8Array): Promise<Map<string, ParsedZipEntry>> => {
  const eocdOffset = findEndOfCentralDirectory(bytes)
  if (eocdOffset < 0) {
    throw new Error('ZIP 文件结构无效')
  }

  const eocd = new DataView(bytes.buffer, bytes.byteOffset + eocdOffset, bytes.length - eocdOffset)
  const totalEntries = eocd.getUint16(10, true)
  const centralDirectorySize = eocd.getUint32(12, true)
  const centralDirectoryOffset = eocd.getUint32(16, true)
  const centralDirectoryEnd = centralDirectoryOffset + centralDirectorySize

  if (centralDirectoryEnd > bytes.length) {
    throw new Error('ZIP 中央目录越界')
  }

  const entries = new Map<string, ParsedZipEntry>()
  let cursor = centralDirectoryOffset
  for (let index = 0; index < totalEntries; index += 1) {
    const headerView = new DataView(bytes.buffer, bytes.byteOffset + cursor, bytes.length - cursor)
    if (headerView.getUint32(0, true) !== 0x02014b50) {
      throw new Error('ZIP 中央目录条目损坏')
    }
    const compressionMethod = headerView.getUint16(10, true)
    const expectedCrc = headerView.getUint32(16, true)
    const compressedSize = headerView.getUint32(20, true)
    const uncompressedSize = headerView.getUint32(24, true)
    const filenameLength = headerView.getUint16(28, true)
    const extraLength = headerView.getUint16(30, true)
    const commentLength = headerView.getUint16(32, true)
    const localHeaderOffset = headerView.getUint32(42, true)
    const filenameBytes = bytes.slice(cursor + 46, cursor + 46 + filenameLength)
    const filename = decoder.decode(filenameBytes)

    const localHeaderView = new DataView(bytes.buffer, bytes.byteOffset + localHeaderOffset, bytes.length - localHeaderOffset)
    if (localHeaderView.getUint32(0, true) !== 0x04034b50) {
      throw new Error(`ZIP 条目 ${filename} 的本地头损坏`)
    }
    const localFilenameLength = localHeaderView.getUint16(26, true)
    const localExtraLength = localHeaderView.getUint16(28, true)
    const dataStart = localHeaderOffset + 30 + localFilenameLength + localExtraLength
    const dataEnd = dataStart + compressedSize
    if (dataEnd > bytes.length) {
      throw new Error(`ZIP 条目 ${filename} 数据越界`)
    }

    const compressedData = bytes.slice(dataStart, dataEnd)
    let entryData: Uint8Array
    if (compressionMethod === 0) {
      entryData = compressedData
    } else if (compressionMethod === 8) {
      entryData = await inflateDeflateRaw(compressedData)
    } else {
      throw new Error(`ZIP 条目 ${filename} 使用了不支持的压缩方式`)
    }

    if (entryData.length !== uncompressedSize) {
      throw new Error(`ZIP 条目 ${filename} 解压后大小异常`)
    }
    if (crc32(entryData) !== expectedCrc) {
      throw new Error(`ZIP 条目 ${filename} CRC 校验失败`)
    }

    entries.set(filename, { name: filename, data: entryData })
    cursor += 46 + filenameLength + extraLength + commentLength
  }

  return entries
}

const inferFontExtension = (asset: Pick<FontAssetRecord, 'mime' | 'sourceUrl'>): string => {
  const sourceUrl = (asset.sourceUrl || '').toLowerCase()
  const urlMatch = sourceUrl.match(/\.(ttf|otf|woff2?|ttc|otc)(?:$|\?)/i)
  if (urlMatch) return urlMatch[1].toLowerCase()
  const mime = (asset.mime || '').toLowerCase()
  if (mime.includes('woff2')) return 'woff2'
  if (mime.includes('woff')) return 'woff'
  if (mime.includes('opentype') || mime.includes('otf')) return 'otf'
  if (mime.includes('truetype') || mime.includes('ttf')) return 'ttf'
  if (mime.includes('ttc')) return 'ttc'
  if (mime.includes('otc')) return 'otc'
  return 'bin'
}

const parseManifestObject = (raw: unknown): DisplaySettingsTransferManifest => {
  if (!raw || typeof raw !== 'object') {
    throw new Error('配置清单格式无效')
  }
  const manifest = raw as DisplaySettingsTransferManifest
  if (manifest.kind !== TRANSFER_KIND) {
    throw new Error('不是 SealChat 显示设置导入文件')
  }
  if (typeof manifest.version !== 'number' || manifest.version < 1 || manifest.version > TRANSFER_VERSION) {
    throw new Error(`不支持的配置版本：${String((manifest as any).version)}`)
  }
  if (!manifest.settings || typeof manifest.settings !== 'object') {
    throw new Error('配置清单缺少 settings 字段')
  }
  return manifest
}

const parseJsonManifest = (content: string): DisplaySettingsTransferManifest => {
  const parsed = JSON.parse(content)
  if (parsed && typeof parsed === 'object' && (parsed as any).kind === TRANSFER_KIND) {
    return parseManifestObject(parsed)
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('JSON 配置内容无效')
  }
  return {
    kind: TRANSFER_KIND,
    version: 1,
    exportedAt: new Date().toISOString(),
    settings: parsed as Partial<DisplaySettings>,
    bundledFont: null,
  }
}

const buildManifestJson = (manifest: DisplaySettingsTransferManifest) =>
  JSON.stringify(manifest, null, 2)

const buildExportFilename = (format: TransferFormat) =>
  `sealchat-display-settings-${formatTimestamp()}.${format}`

export const exportDisplaySettingsPackage = async (
  settings: DisplaySettings,
): Promise<DisplaySettingsExportResult> => {
  const warnings: string[] = []
  const snapshot = cloneSettings(settings)
  let bundledFont: BundledFontManifest | null = null
  let fontBlob: Blob | null = null

  if (snapshot.globalFontSourceType === 'upload' || snapshot.globalFontSourceType === 'url') {
    const assetId = typeof snapshot.globalFontAssetId === 'string' ? snapshot.globalFontAssetId : ''
    const fontAsset = assetId ? await getFontAssetById(assetId) : null
    if (fontAsset) {
      const extension = inferFontExtension(fontAsset)
      const entryName = `fonts/${sanitizeFilenameSegment(fontAsset.id)}.${extension}`
      bundledFont = {
        assetId: fontAsset.id,
        entryName,
        family: fontAsset.family,
        sourceType: fontAsset.sourceType,
        mime: fontAsset.mime,
        size: fontAsset.size,
        sourceUrl: fontAsset.sourceUrl,
      }
      fontBlob = fontAsset.blob
      snapshot.globalFontFamily = sanitizeFontFamilyName(fontAsset.family)
      snapshot.globalFontSourceType = fontAsset.sourceType
      snapshot.globalFontAssetId = fontAsset.id
    } else {
      warnings.push('当前配置引用了字体文件，但本地缓存中已不存在该字体，已退化为纯 JSON 导出。')
      fixSnapshotWithoutBundledFont(snapshot)
    }
  }

  const manifest: DisplaySettingsTransferManifest = {
    kind: TRANSFER_KIND,
    version: TRANSFER_VERSION,
    exportedAt: new Date().toISOString(),
    settings: snapshot,
    bundledFont,
  }

  if (!bundledFont || !fontBlob) {
    const json = buildManifestJson(manifest)
    return {
      blob: new Blob([json], { type: 'application/json;charset=utf-8' }),
      filename: buildExportFilename('json'),
      format: 'json',
      includedFont: false,
      warnings,
    }
  }

  const zipBytes = createZip([
    {
      name: 'manifest.json',
      data: encoder.encode(buildManifestJson(manifest)),
    },
    {
      name: bundledFont.entryName,
      data: new Uint8Array(await fontBlob.arrayBuffer()),
    },
  ])

  return {
    blob: new Blob([zipBytes], { type: 'application/zip' }),
    filename: buildExportFilename('zip'),
    format: 'zip',
    includedFont: true,
    warnings,
  }
}

const applyBundledFontToSettings = async (
  manifest: DisplaySettingsTransferManifest,
  fontBytes: Uint8Array,
): Promise<{ importedFont: boolean; warnings: string[] }> => {
  const warnings: string[] = []
  const bundledFont = manifest.bundledFont
  if (!bundledFont) {
    fixSnapshotWithoutBundledFont(manifest.settings)
    return { importedFont: false, warnings }
  }

  manifest.settings.globalFontFamily = sanitizeFontFamilyName(bundledFont.family)
  if (!isFontAssetCacheAvailable()) {
    manifest.settings.globalFontSourceType = manifest.settings.globalFontFamily ? 'manual' : 'default'
    manifest.settings.globalFontAssetId = null
    warnings.push('当前环境不支持字体缓存，已按字体名恢复设置，未导入字体文件本体。')
    return { importedFont: false, warnings }
  }

  await saveFontAsset({
    id: bundledFont.assetId,
    family: sanitizeFontFamilyName(bundledFont.family),
    sourceType: bundledFont.sourceType,
    mime: bundledFont.mime || 'application/octet-stream',
    size: fontBytes.length,
    blob: new Blob([fontBytes], { type: bundledFont.mime || 'application/octet-stream' }),
    sourceUrl: bundledFont.sourceUrl,
  })
  manifest.settings.globalFontSourceType = bundledFont.sourceType
  manifest.settings.globalFontAssetId = bundledFont.assetId
  return { importedFont: true, warnings }
}

const isZipBytes = (bytes: Uint8Array) =>
  bytes.length >= 4
  && bytes[0] === 0x50
  && bytes[1] === 0x4b
  && (bytes[2] === 0x03 || bytes[2] === 0x05 || bytes[2] === 0x07)
  && (bytes[3] === 0x04 || bytes[3] === 0x06 || bytes[3] === 0x08)

export const importDisplaySettingsPackage = async (
  file: Blob,
): Promise<DisplaySettingsImportResult> => {
  const bytes = new Uint8Array(await file.arrayBuffer())
  if (isZipBytes(bytes)) {
    const entries = await parseZip(bytes)
    const manifestEntry = entries.get('manifest.json')
    if (!manifestEntry) {
      throw new Error('ZIP 文件缺少 manifest.json')
    }
    const manifest = parseJsonManifest(decoder.decode(manifestEntry.data))
    let warnings: string[] = []
    let importedFont = false
    if (manifest.bundledFont) {
      const fontEntry = entries.get(manifest.bundledFont.entryName)
      if (!fontEntry) {
        throw new Error(`ZIP 文件缺少字体文件：${manifest.bundledFont.entryName}`)
      }
      const fontResult = await applyBundledFontToSettings(manifest, fontEntry.data)
      warnings = warnings.concat(fontResult.warnings)
      importedFont = fontResult.importedFont
    } else {
      fixSnapshotWithoutBundledFont(manifest.settings)
    }
    return {
      settings: manifest.settings,
      format: 'zip',
      importedFont,
      warnings,
    }
  }

  const manifest = parseJsonManifest(decoder.decode(bytes))
  fixSnapshotWithoutBundledFont(manifest.settings)
  return {
    settings: manifest.settings,
    format: 'json',
    importedFont: false,
    warnings: [],
  }
}
