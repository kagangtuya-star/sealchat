import { UI_CUSTOMIZATION_API_VERSION } from './jsRuntime'
import type { UISurface } from './surfaces'
import type { UICustomizationPackage } from './types'

let fflatePromise: Promise<typeof import('fflate')> | null = null
const loadFflate = () => {
  fflatePromise ??= import('fflate')
  return fflatePromise
}

const UI_TRANSFER_ZIP_LIMITS = {
  zipBytes: 20 * 1024 * 1024,
  jsonFiles: 100,
  jsonBytes: 8 * 1024 * 1024,
  totalBytes: 20 * 1024 * 1024,
}

// Versioned transfer documents. Exports never carry database IDs, scope,
// world, author/audit fields, trust state or server timestamps.
export const UI_PACKAGE_FORMAT = 'sealchat.ui-package'
export const UI_BUNDLE_FORMAT = 'sealchat.ui-bundle'
export const UI_TRANSFER_FORMAT_VERSION = 1

export interface UIPackageDocument {
  format: typeof UI_PACKAGE_FORMAT
  formatVersion: number
  runtimeApiVersion: number
  manifest: { name: string; description: string }
  targets: UISurface[]
  css: string
  js: string
  channelCapabilities?: string[]
}

export interface UIBundleDocument {
  format: typeof UI_BUNDLE_FORMAT
  formatVersion: number
  runtimeApiVersion: number
  packages: UIPackageDocument[]
}

type ExportSource = Pick<UICustomizationPackage, 'name' | 'description' | 'surfaces' | 'css' | 'js' | 'channelCapabilities'>

export function buildPackageDocument(pkg: ExportSource): UIPackageDocument {
  return {
    format: UI_PACKAGE_FORMAT,
    formatVersion: UI_TRANSFER_FORMAT_VERSION,
    runtimeApiVersion: UI_CUSTOMIZATION_API_VERSION,
    manifest: { name: pkg.name, description: pkg.description || '' },
    targets: [...pkg.surfaces],
    css: pkg.css || '',
    js: pkg.js || '',
    channelCapabilities: [...(pkg.channelCapabilities || [])],
  }
}

export function buildBundleDocument(packages: ExportSource[]): UIBundleDocument {
  return {
    format: UI_BUNDLE_FORMAT,
    formatVersion: UI_TRANSFER_FORMAT_VERSION,
    runtimeApiVersion: UI_CUSTOMIZATION_API_VERSION,
    packages: packages.map(buildPackageDocument),
  }
}

type TransferImportFile = Pick<File, 'text'> & Partial<Pick<File, 'name' | 'size' | 'arrayBuffer'>>

const isObject = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value)

const parseTransferJSON = (text: string) => {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    throw new Error('文件不是有效的 JSON')
  }
  if (!isObject(parsed)) throw new Error('文件内容不是界面自定义包对象')
  return parsed
}

const readTransferZip = async (file: TransferImportFile): Promise<unknown> => {
  if (!file.arrayBuffer) throw new Error('当前环境无法读取 ZIP 文件')
  if (file.size && file.size > UI_TRANSFER_ZIP_LIMITS.zipBytes) throw new Error('ZIP 超过 20MB')

  const { unzipSync } = await loadFflate()
  let jsonCount = 0
  let declaredTotal = 0
  let raw: Record<string, Uint8Array>
  try {
    raw = unzipSync(new Uint8Array(await file.arrayBuffer()), {
      filter: (entry) => {
        if (entry.name.endsWith('/') || !entry.name.toLowerCase().endsWith('.json')) return false
        jsonCount += 1
        declaredTotal += entry.originalSize
        if (jsonCount > UI_TRANSFER_ZIP_LIMITS.jsonFiles) throw new Error(`ZIP 中 JSON 文件超过 ${UI_TRANSFER_ZIP_LIMITS.jsonFiles} 个`)
        if (entry.originalSize > UI_TRANSFER_ZIP_LIMITS.jsonBytes || declaredTotal > UI_TRANSFER_ZIP_LIMITS.totalBytes) throw new Error(`${entry.name} 超过解压配额`)
        return true
      },
    })
  } catch (error) {
    throw new Error(error instanceof Error && error.message ? error.message : '不是有效的 ZIP 文件')
  }

  const packages: unknown[] = []
  for (const name of Object.keys(raw).sort()) {
    let parsed: unknown
    try {
      parsed = JSON.parse(new TextDecoder().decode(raw[name]))
    } catch {
      continue
    }
    if (!isObject(parsed)) continue
    if (parsed.format === UI_PACKAGE_FORMAT) {
      packages.push(parsed)
    } else if (parsed.format === UI_BUNDLE_FORMAT && Array.isArray(parsed.packages)) {
      packages.push(...parsed.packages)
    }
  }

  if (!packages.length) throw new Error('ZIP 中未找到 sealchat.ui-package / sealchat.ui-bundle JSON')
  if (packages.length === 1) return packages[0]
  return {
    format: UI_BUNDLE_FORMAT,
    formatVersion: UI_TRANSFER_FORMAT_VERSION,
    runtimeApiVersion: UI_CUSTOMIZATION_API_VERSION,
    packages,
  }
}

/** Parses a JSON or ZIP import. Format and content are still validated by the server. */
export async function readTransferFile(file: TransferImportFile): Promise<unknown> {
  if ((file.name || '').toLowerCase().endsWith('.zip')) return readTransferZip(file)
  return parseTransferJSON(await file.text())
}

export function transferFileName(name: string, kind: 'package' | 'bundle') {
  const safe = name.trim().replace(/[^a-zA-Z0-9一-龥_-]+/g, '_').slice(0, 60) || 'package'
  return `sealchat-ui-${kind === 'bundle' ? 'bundle-' : ''}${safe}.json`
}

export function downloadJSON(data: unknown, fileName: string) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = fileName
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}
