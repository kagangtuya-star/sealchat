import { UI_CUSTOMIZATION_API_VERSION } from './jsRuntime'
import type { UISurface } from './surfaces'
import type { UICustomizationPackage } from './types'

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

/** Parses an import file. Format and content are validated by the server. */
export async function readTransferFile(file: Pick<File, 'text'>): Promise<unknown> {
  let parsed: unknown
  try {
    parsed = JSON.parse(await file.text())
  } catch {
    throw new Error('文件不是有效的 JSON')
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('文件内容不是界面自定义包对象')
  return parsed
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
