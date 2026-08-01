import { fetchAttachmentFileById } from '@/composables/useAttachmentResolver'
import { createZip, isZipBytes, parseZip } from '@/services/displaySettingsTransfer'
import { CURSOR_SLOTS } from './cursorTypes'
import type { CursorScope, UploadedCursorAsset } from './cursorApi'
import type { CursorThemeConfig } from './cursorTypes'

const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8')
const TRANSFER_KIND = 'sealchat-cursor-theme'
const TRANSFER_VERSION = 1

interface CursorTransferSlot {
  mode: 'inherit' | 'browser' | 'custom'
  entryName?: string
  hotspotX?: number
  hotspotY?: number
  size?: number
}

interface CursorTransferManifest {
  kind: typeof TRANSFER_KIND
  version: number
  scope: CursorScope
  exportedAt: string
  slots: Partial<Record<(typeof CURSOR_SLOTS)[number], CursorTransferSlot>>
}

export const exportCursorThemePackage = async (config: CursorThemeConfig, scope: CursorScope) => {
  const manifest: CursorTransferManifest = {
    kind: TRANSFER_KIND,
    version: TRANSFER_VERSION,
    scope,
    exportedAt: new Date().toISOString(),
    slots: {},
  }
  const entries: Array<{ name: string; data: Uint8Array }> = []
  for (const slot of CURSOR_SLOTS) {
    const asset = config.slots?.[slot]
    if (!asset) continue
    if (asset.mode !== 'custom' || !asset.attachmentId) {
      manifest.slots[slot] = { mode: asset.mode }
      continue
    }
    const entryName = `assets/${slot}.webp`
    const file = await fetchAttachmentFileById(asset.attachmentId, `${slot}.webp`)
    if (!file) throw new Error(`无法读取 ${slot} 鼠标图片`)
    entries.push({ name: entryName, data: new Uint8Array(await file.arrayBuffer()) })
    manifest.slots[slot] = {
      mode: 'custom',
      entryName,
      hotspotX: asset.hotspotX || 0,
      hotspotY: asset.hotspotY || 0,
      size: asset.size || Math.max(asset.width || 0, asset.height || 0) || 32,
    }
  }
  entries.unshift({ name: 'manifest.json', data: encoder.encode(JSON.stringify(manifest, null, 2)) })
  return new Blob([createZip(entries)], { type: 'application/zip' })
}

export const importCursorThemePackage = async (
  file: File,
  targetScope: CursorScope,
  upload: (file: File, size?: number) => Promise<UploadedCursorAsset>,
): Promise<CursorThemeConfig> => {
  const bytes = new Uint8Array(await file.arrayBuffer())
  if (!isZipBytes(bytes)) throw new Error('请选择 SealChat 鼠标样式 ZIP')
  const entries = await parseZip(bytes)
  const manifestEntry = entries.get('manifest.json')
  if (!manifestEntry) throw new Error('ZIP 缺少 manifest.json')
  const manifest = JSON.parse(decoder.decode(manifestEntry.data)) as CursorTransferManifest
  if (manifest.kind !== TRANSFER_KIND) throw new Error('不是 SealChat 鼠标样式文件')
  if (manifest.version !== TRANSFER_VERSION) throw new Error(`不支持的鼠标样式版本：${manifest.version}`)
  const result: CursorThemeConfig = { version: 1, slots: {} }
  for (const slot of CURSOR_SLOTS) {
    const source = manifest.slots?.[slot]
    if (!source) continue
    let mode = source.mode
    if (targetScope === 'platform' && mode === 'inherit') mode = 'browser'
    if (mode !== 'custom') {
      result.slots[slot] = { mode }
      continue
    }
    if (!source.entryName || source.entryName.includes('..') || source.entryName.startsWith('/')) {
      throw new Error(`${slot} 图片路径无效`)
    }
    const entry = entries.get(source.entryName)
    if (!entry) throw new Error(`ZIP 缺少 ${source.entryName}`)
    const uploaded = await upload(new File([entry.data], `${slot}.webp`, { type: 'image/webp' }), source.size)
    result.slots[slot] = {
      mode: 'custom',
      attachmentId: uploaded.attachmentId,
      hotspotX: source.hotspotX || 0,
      hotspotY: source.hotspotY || 0,
      width: uploaded.width,
      height: uploaded.height,
      size: source.size || Math.max(uploaded.width, uploaded.height),
      animated: uploaded.animated,
    }
  }
  return result
}
