import type { Dice3DSkin } from '@/types'
import { createZip, isZipBytes, parseZip } from '@/services/displaySettingsTransfer'
import { createDiceAtlasCanvas, type DiceAtlasType } from './engine/DiceGeometryRegistry'

const DICE_TYPES = ['d2', 'd4', 'd6', 'd8', 'd10', 'd12', 'd20', 'd100'] as const
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8')

interface DiceSkinManifest {
  kind: 'sealchat-dice3d-skin'
  version: 1
  name?: string
  skin?: Partial<Omit<Dice3DSkin, 'textures'>>
  textures?: Partial<Record<(typeof DICE_TYPES)[number], string>>
}

export interface DiceSkinImportResult {
  name: string
  skin: Dice3DSkin
}

const defaultSkin = (): Dice3DSkin => ({
  faceBackground: '#f5f6fa',
  faceForeground: '#111827',
  edgeColor: '#d1d5db',
  roughness: 0.72,
  metalness: 0.05,
  scale: 1,
  textures: {},
})

const mimeFor = (name: string) => {
  if (/\.webp$/i.test(name)) return 'image/webp'
  if (/\.jpe?g$/i.test(name)) return 'image/jpeg'
  return 'image/png'
}

const canvasPNG = (type: DiceAtlasType, skin: Dice3DSkin) => new Promise<Uint8Array>((resolve, reject) => {
  createDiceAtlasCanvas(type, skin).toBlob(async blob => {
    if (!blob) return reject(new Error(`无法生成 ${type} 默认图集`))
    resolve(new Uint8Array(await blob.arrayBuffer()))
  }, 'image/png')
})

export const createDiceSkinTemplate = async () => {
	const skin = defaultSkin()
  const manifest: DiceSkinManifest = {
    kind: 'sealchat-dice3d-skin',
    version: 1,
    name: '我的骰子样式',
    skin,
    textures: Object.fromEntries(DICE_TYPES.map(type => [type, `textures/${type}.png`])),
  }
  const readme = [
    'SealChat 3D 骰面合集模板',
    '',
    '1. textures/ 已包含全部默认图集，可直接修改或替换。',
    '2. 图集将直接映射到模型 UV；建议使用 PNG/WebP。',
    '3. 不需要自定义的骰型可从 manifest.json textures 中删除。',
    '4. 打包 manifest.json 与 textures/ 为 ZIP 后上传。',
  ].join('\n')
	const textureEntries = await Promise.all(DICE_TYPES.map(async type => ({
		name: `textures/${type}.png`, data: await canvasPNG(type, skin),
	})))
  return new Blob([createZip([
    { name: 'manifest.json', data: encoder.encode(JSON.stringify(manifest, null, 2)) },
    { name: 'README.txt', data: encoder.encode(readme) },
		...textureEntries,
  ])], { type: 'application/zip' })
}

export const downloadDiceSkinTemplate = async () => {
	const url = URL.createObjectURL(await createDiceSkinTemplate())
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'sealchat-dice3d-skin-template.zip'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

export const importDiceSkinPackage = async (
  file: File,
  upload: (file: File) => Promise<string>,
): Promise<DiceSkinImportResult> => {
  const bytes = new Uint8Array(await file.arrayBuffer())
  if (!isZipBytes(bytes)) throw new Error('请选择 SealChat 3D 骰面 ZIP')
  const entries = await parseZip(bytes)
  const manifestEntry = entries.get('manifest.json')
  if (!manifestEntry) throw new Error('ZIP 缺少 manifest.json')
  const manifest = JSON.parse(decoder.decode(manifestEntry.data)) as DiceSkinManifest
  if (manifest.kind !== 'sealchat-dice3d-skin' || manifest.version !== 1) {
    throw new Error('不是支持的 SealChat 3D 骰面合集')
  }
  const skin = { ...defaultSkin(), ...(manifest.skin || {}), textures: {} as Record<string, string> }
  for (const type of DICE_TYPES) {
    const entryName = manifest.textures?.[type]
    if (!entryName) continue
    if (entryName.includes('..') || entryName.startsWith('/')) throw new Error(`${type} 图集路径无效`)
    const entry = entries.get(entryName)
    if (!entry) throw new Error(`ZIP 缺少 ${entryName}`)
    const attachmentId = await upload(new File([entry.data], entryName.split('/').pop() || `${type}.png`, { type: mimeFor(entryName) }))
    skin.textures![type] = attachmentId
  }
  return { name: String(manifest.name || file.name.replace(/\.zip$/i, '') || '骰子样式').trim(), skin }
}
