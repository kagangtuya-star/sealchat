import { compressImage } from '@/composables/useImageCompressor'
import { normalizedTheaterMediaType, supportedTheaterMedia } from './stage-media'

const isAnimatedPNG = async (file: File) => {
  let offset = 8
  while (offset + 12 <= file.size) {
    const header = new Uint8Array(await file.slice(offset, offset + 8).arrayBuffer())
    if (header.length < 8) return false
    const length = new DataView(header.buffer, header.byteOffset, header.byteLength).getUint32(0)
    const type = String.fromCharCode(...header.slice(4, 8))
    if (type === 'acTL') return true
    if (type === 'IDAT' || type === 'IEND') return false
    offset += 12 + length
  }
  return false
}

export const prepareTheaterMediaUpload = async (file: File) => {
  const mimeType = normalizedTheaterMediaType(file)
  if (!supportedTheaterMedia.has(mimeType)) throw new Error('仅支持 PNG、APNG、JPEG、WebP、GIF、WebM')
  if ((mimeType === 'image/png' || mimeType === 'image/apng') && await isAnimatedPNG(file)) return file
  if (mimeType !== 'image/jpeg' && mimeType !== 'image/png') return file
  return compressImage(file, { mimeType: 'image/webp' })
}
