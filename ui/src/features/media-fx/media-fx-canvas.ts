import {
  mediaFxAdvancedHasContent,
  mediaFxFilterHasContent,
  mediaFxFilterToCss,
  normalizeMediaFxFilter,
  normalizeMediaFxSpec,
  type MediaFxFilter,
} from './media-fx'
import { applyMediaFxGpu } from './media-fx-gpu'

// Canvas bake adapter: a one-shot conversion of a static image + MediaFxSpec (basic
// filter, then advanced GPU effects) into new pixels. It is not a live renderer and
// never bakes motion (motion cannot be represented by a still PNG/JPEG/WebP). Call it
// on explicit confirm or from a debounced preview, not per input.

let cssFilterSupport: boolean | null = null

export const canvasFilterSupported = () => {
  if (cssFilterSupport !== null) return cssFilterSupport
  try {
    const context = typeof document === 'undefined' ? null : document.createElement('canvas').getContext('2d')
    cssFilterSupport = Boolean(context && 'filter' in context)
  } catch {
    cssFilterSupport = false
  }
  return cssFilterSupport
}

type Matrix3 = [number, number, number, number, number, number, number, number, number]

// Matrices from the Filter Effects spec shorthand definitions (sRGB, unpremultiplied).
const saturateMatrix = (s: number): Matrix3 => [
  0.213 + 0.787 * s, 0.715 - 0.715 * s, 0.072 - 0.072 * s,
  0.213 - 0.213 * s, 0.715 + 0.285 * s, 0.072 - 0.072 * s,
  0.213 - 0.213 * s, 0.715 - 0.715 * s, 0.072 + 0.928 * s,
]

const grayscaleMatrix = (amount: number): Matrix3 => {
  const a = 1 - amount
  return [
    0.2126 + 0.7874 * a, 0.7152 - 0.7152 * a, 0.0722 - 0.0722 * a,
    0.2126 - 0.2126 * a, 0.7152 + 0.2848 * a, 0.0722 - 0.0722 * a,
    0.2126 - 0.2126 * a, 0.7152 - 0.7152 * a, 0.0722 + 0.9278 * a,
  ]
}

const sepiaMatrix = (amount: number): Matrix3 => {
  const a = 1 - amount
  return [
    0.393 + 0.607 * a, 0.769 - 0.769 * a, 0.189 - 0.189 * a,
    0.349 - 0.349 * a, 0.686 + 0.314 * a, 0.168 - 0.168 * a,
    0.272 - 0.272 * a, 0.534 - 0.534 * a, 0.131 + 0.869 * a,
  ]
}

const hueRotateMatrix = (degrees: number): Matrix3 => {
  const radians = (degrees * Math.PI) / 180
  const c = Math.cos(radians)
  const s = Math.sin(radians)
  return [
    0.213 + c * 0.787 - s * 0.213, 0.715 - c * 0.715 - s * 0.715, 0.072 - c * 0.072 + s * 0.928,
    0.213 - c * 0.213 + s * 0.143, 0.715 + c * 0.285 + s * 0.14, 0.072 - c * 0.072 - s * 0.283,
    0.213 - c * 0.213 - s * 0.787, 0.715 - c * 0.715 + s * 0.715, 0.072 + c * 0.928 + s * 0.072,
  ]
}

const clampByte = (value: number) => (value < 0 ? 0 : value > 255 ? 255 : value)

const applyColorStages = (data: Uint8ClampedArray, filter: MediaFxFilter) => {
  const matrices: Matrix3[] = []
  if (filter.saturation !== 1) matrices.push(saturateMatrix(filter.saturation))
  if (filter.grayscale !== 0) matrices.push(grayscaleMatrix(filter.grayscale))
  if (filter.sepia !== 0) matrices.push(sepiaMatrix(filter.sepia))
  if (filter.hueRotate !== 0) matrices.push(hueRotateMatrix(filter.hueRotate))
  const { brightness, contrast } = filter
  for (let index = 0; index < data.length; index += 4) {
    let r = data[index]
    let g = data[index + 1]
    let b = data[index + 2]
    // Same order as mediaFxFilterToCss; each stage clamps like CSS filter primitives.
    if (brightness !== 1) {
      r = clampByte(r * brightness)
      g = clampByte(g * brightness)
      b = clampByte(b * brightness)
    }
    if (contrast !== 1) {
      const offset = 127.5 * (1 - contrast)
      r = clampByte(r * contrast + offset)
      g = clampByte(g * contrast + offset)
      b = clampByte(b * contrast + offset)
    }
    for (const m of matrices) {
      const nr = clampByte(m[0] * r + m[1] * g + m[2] * b)
      const ng = clampByte(m[3] * r + m[4] * g + m[5] * b)
      const nb = clampByte(m[6] * r + m[7] * g + m[8] * b)
      r = nr
      g = ng
      b = nb
    }
    data[index] = r
    data[index + 1] = g
    data[index + 2] = b
  }
}

// Three box passes approximate a Gaussian with the given standard deviation.
const boxSizesForGauss = (sigma: number, passes = 3) => {
  const ideal = Math.sqrt((12 * sigma * sigma) / passes + 1)
  let lower = Math.floor(ideal)
  if (lower % 2 === 0) lower -= 1
  const upper = lower + 2
  const m = Math.round((12 * sigma * sigma - passes * lower * lower - 4 * passes * lower - 3 * passes) / (-4 * lower - 4))
  return Array.from({ length: passes }, (_, index) => (index < m ? lower : upper))
}

const boxBlurPass = (source: Float32Array, target: Float32Array, width: number, height: number, radius: number, horizontal: boolean) => {
  const lines = horizontal ? height : width
  const length = horizontal ? width : height
  const stride = horizontal ? 4 : width * 4
  const scale = 1 / (radius * 2 + 1)
  for (let line = 0; line < lines; line += 1) {
    const base = horizontal ? line * width * 4 : line * 4
    for (let channel = 0; channel < 4; channel += 1) {
      let sum = 0
      for (let offset = -radius; offset <= radius; offset += 1) {
        const position = Math.min(length - 1, Math.max(0, offset))
        sum += source[base + position * stride + channel]
      }
      for (let position = 0; position < length; position += 1) {
        target[base + position * stride + channel] = sum * scale
        const removeAt = Math.max(0, position - radius)
        const addAt = Math.min(length - 1, position + radius + 1)
        sum += source[base + addAt * stride + channel] - source[base + removeAt * stride + channel]
      }
    }
  }
}

const applyBlur = (data: Uint8ClampedArray, width: number, height: number, sigma: number) => {
  if (sigma <= 0 || width < 2 || height < 2) return
  // Premultiply so transparent pixels do not bleed dark fringes into opaque edges.
  const front = new Float32Array(data.length)
  const back = new Float32Array(data.length)
  for (let index = 0; index < data.length; index += 4) {
    const alpha = data[index + 3] / 255
    front[index] = data[index] * alpha
    front[index + 1] = data[index + 1] * alpha
    front[index + 2] = data[index + 2] * alpha
    front[index + 3] = data[index + 3]
  }
  boxSizesForGauss(sigma).forEach((size) => {
    const radius = Math.max(0, (size - 1) / 2)
    if (!radius) return
    boxBlurPass(front, back, width, height, radius, true)
    boxBlurPass(back, front, width, height, radius, false)
  })
  for (let index = 0; index < data.length; index += 4) {
    const alpha = front[index + 3]
    const factor = alpha > 0 ? 255 / alpha : 0
    data[index] = clampByte(front[index] * factor)
    data[index + 1] = clampByte(front[index + 1] * factor)
    data[index + 2] = clampByte(front[index + 2] * factor)
    data[index + 3] = clampByte(alpha)
  }
}

export interface MediaFxBakeSize {
  width: number
  height: number
}

const resolvePixelRatio = (value: unknown) => (
  typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 1
)

const drawFilteredCanvas = (
  source: CanvasImageSource,
  size: MediaFxBakeSize,
  input: MediaFxFilter,
  pixelRatio: number,
) => {
  const filter = normalizeMediaFxFilter(input)
  const width = Math.max(1, Math.round(size.width))
  const height = Math.max(1, Math.round(size.height))
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const context = canvas.getContext('2d')
  if (!context) throw new Error('无法创建图片处理画布')
  if (canvasFilterSupported()) {
    context.filter = mediaFxFilterToCss(filter, { blurScale: pixelRatio }) || 'none'
    context.drawImage(source, 0, 0, width, height)
    context.filter = 'none'
    return { canvas, context }
  }
  context.drawImage(source, 0, 0, width, height)
  if (!mediaFxFilterHasContent(filter)) return { canvas, context }
  const imageData = context.getImageData(0, 0, width, height)
  applyColorStages(imageData.data, filter)
  applyBlur(imageData.data, width, height, filter.blurPx * pixelRatio)
  context.putImageData(imageData, 0, 0)
  return { canvas, context }
}

// Draws `source` at its own size with the filter baked in. Alpha is preserved.
export const bakeMediaFxFilterToCanvas = (
  source: CanvasImageSource,
  size: MediaFxBakeSize,
  input: MediaFxFilter,
): HTMLCanvasElement => drawFilteredCanvas(source, size, input, 1).canvas

export const MEDIA_FX_ADVANCED_BAKE_ERROR_MESSAGE = '当前设备无法完成高级图片效果处理，请关闭高级效果后重试'

// Thrown only when the caller requires advanced effects and the GPU pass cannot run,
// so a confirm never silently produces an image without the requested effect.
export class MediaFxAdvancedBakeError extends Error {
  constructor(message = MEDIA_FX_ADVANCED_BAKE_ERROR_MESSAGE) {
    super(message)
    this.name = 'MediaFxAdvancedBakeError'
  }
}

export interface MediaFxCanvasBakeOptions {
  // Output pixels per pixel of the intended full-resolution result (e.g. < 1 for a
  // downscaled preview), so blur radius and advanced block / offset sizes keep the
  // same look at any output size.
  pixelRatio?: number
  // When advanced effects carry content and the GPU pass fails, throw instead of
  // returning an image with only the basic filter.
  requireAdvanced?: boolean
}

// Full static bake: source -> basic filter -> advanced GPU -> output. Motion is always
// ignored. Without `requireAdvanced`, a GPU failure keeps the basic-filtered pixels.
export const bakeMediaFxToCanvas = (
  source: CanvasImageSource,
  size: MediaFxBakeSize,
  input: unknown,
  options: MediaFxCanvasBakeOptions = {},
): HTMLCanvasElement => {
  const spec = normalizeMediaFxSpec(input)
  const pixelRatio = resolvePixelRatio(options.pixelRatio)
  const { canvas, context } = drawFilteredCanvas(source, size, spec.filter, pixelRatio)
  if (!mediaFxAdvancedHasContent(spec.advanced)) return canvas
  let applied = false
  try {
    const imageData = context.getImageData(0, 0, canvas.width, canvas.height)
    applied = applyMediaFxGpu(imageData, spec.advanced, { pixelRatio })
    if (applied) context.putImageData(imageData, 0, 0)
  } catch {
    applied = false
  }
  if (!applied && options.requireAdvanced) throw new MediaFxAdvancedBakeError()
  return canvas
}

const canvasToBlob = (canvas: HTMLCanvasElement, type: string, quality?: number) => new Promise<Blob>((resolve, reject) => {
  canvas.toBlob((blob) => {
    if (blob) resolve(blob)
    else reject(new Error('图片导出失败'))
  }, type, quality)
})

const decodeImageFile = async (file: Blob) => {
  const url = URL.createObjectURL(file)
  try {
    // onload instead of decode(): decode() may stay pending for detached images in hidden pages.
    return await new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image()
      image.onload = () => resolve(image)
      image.onerror = () => reject(new Error('图片解码失败'))
      image.src = url
    })
  } finally {
    URL.revokeObjectURL(url)
  }
}

const bakeOutputTypes = new Set(['image/png', 'image/jpeg', 'image/webp'])

export interface MediaFxBakeOptions {
  type?: string
  quality?: number
  requireAdvanced?: boolean
}

// Static-only: the spec's motion is ignored by design. Returns the input unchanged
// when neither the filter nor the advanced effects have content, so callers can always
// route through this helper.
export const bakeMediaFxToBlob = async (file: Blob, input: unknown, options: MediaFxBakeOptions = {}): Promise<Blob> => {
  const spec = normalizeMediaFxSpec(input)
  if (!mediaFxFilterHasContent(spec.filter) && !mediaFxAdvancedHasContent(spec.advanced)) return file
  const image = await decodeImageFile(file)
  const canvas = bakeMediaFxToCanvas(
    image,
    { width: image.naturalWidth, height: image.naturalHeight },
    spec,
    { requireAdvanced: options.requireAdvanced },
  )
  const type = options.type || (bakeOutputTypes.has(file.type) ? file.type : 'image/png')
  return canvasToBlob(canvas, type, options.quality)
}

export const bakeMediaFxToFile = async (file: File, spec: unknown, options: MediaFxBakeOptions = {}): Promise<File> => {
  const blob = await bakeMediaFxToBlob(file, spec, options)
  if (blob === file) return file
  return new File([blob], file.name, { type: blob.type || file.type, lastModified: Date.now() })
}
