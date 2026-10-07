import { mediaFxAdvancedHasContent, normalizeMediaFxAdvanced, type MediaFxAdvanced } from './media-fx'

// WebGL2 adapter for MediaFxAdvanced.
//
// A single lazily created, page-wide processor runs one fixed single-pass fragment
// shader over an ImageData synchronously (ImageData -> texture -> draw -> readPixels -> same
// ImageData). It is meant to be the last step of a static Konva cache filter chain:
// it only runs when that cache is rebuilt, never per frame, and owns no RAF or timers.
//
// Any failure (no WebGL2, context creation, shader compile/link, oversized input,
// GL error, context loss) leaves the ImageData untouched, so the image keeps its basic
// Media FX look instead of disappearing. WebGL objects never leave this module.

// Renderer-only mapping of the normalized 0..1 strengths. These constants are never
// persisted; another renderer may map the same spec differently.
const PIXELATE_MAX_BLOCK_PX = 48
const RGB_SPLIT_MAX_OFFSET_PX = 12
const SCANLINE_MAX_DARKEN = 0.25
const SCANLINE_PERIOD_PX = 3
const VIGNETTE_MAX_DARKEN = 0.85
const GRAIN_MAX_AMPLITUDE = 0.22
// Posterize goes from barely visible banding (many levels) to a hard poster look.
const POSTERIZE_MAX_LEVELS = 24
const POSTERIZE_MIN_LEVELS = 2
const SHARPEN_MAX_AMOUNT = 2

const VERTEX_SHADER = `#version 300 es
void main() {
  // One oversized triangle covers the viewport; no vertex buffer is needed.
  vec2 position = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  gl_Position = vec4(position * 2.0 - 1.0, 0.0, 1.0);
}
`

// Pixel space: framebuffer row y renders source row y, and readPixels returns rows
// from y = 0 upwards, so the output keeps the exact row order of the uploaded
// ImageData without any flip. texelFetch keeps samples exact (no interpolation).
//
// Fixed effect order (one pass, every consumer gets the same result):
//   1. pixelate   sampling grid      (where pixels are read from)
//   2. rgbSplit   channel sampling
//   3. sharpen    3x3 neighborhood   (on the sampled grid)
//   4. edge       3x3 neighborhood
//   5. posterize  tone
//   6. negative   tone
//   7. grain      surface texture    (deterministic per output pixel, no time input)
//   8. scanline   surface texture
//   9. vignette   lens / frame edge
// Each step is skipped by a uniform branch when its strength is 0; the neighborhood
// (at most 8 extra fetches) is only read when sharpen or edge is on. Only rgb is
// changed: alpha always stays the source alpha of the sampled pixel.
const FRAGMENT_SHADER = `#version 300 es
precision highp float;
precision highp int;
uniform sampler2D uTexture;
uniform vec2 uResolution;
uniform float uPixelate;
uniform float uRgbSplit;
uniform float uKernelStep;
uniform float uSharpen;
uniform float uEdge;
uniform float uPosterizeLevels;
uniform float uNegative;
uniform float uGrain;
uniform float uGrainCell;
uniform float uScanline;
uniform float uScanlinePeriod;
uniform float uVignette;
out vec4 outColor;

const vec3 LUMA = vec3(0.2126, 0.7152, 0.0722);
const float EDGE_GAIN = 1.5;

vec4 fetchPixel(vec2 position) {
  ivec2 size = ivec2(uResolution);
  return texelFetch(uTexture, clamp(ivec2(floor(position)), ivec2(0), size - 1), 0);
}

// Integer hash: identical on every GPU for the same cell, unlike sin()-based noise.
float hashCell(uvec2 cell) {
  uint h = cell.x * 0x8da6b343u ^ cell.y * 0xd8163841u;
  h ^= h >> 16;
  h *= 0x7feb352du;
  h ^= h >> 15;
  h *= 0x846ca68bu;
  h ^= h >> 16;
  return float(h >> 8) / 16777216.0;
}

void main() {
  vec2 position = floor(gl_FragCoord.xy);
  if (uPixelate > 1.0) {
    position = floor(position / uPixelate) * uPixelate + floor(uPixelate * 0.5);
  }
  vec4 center = fetchPixel(position);
  vec4 color = center;
  if (uRgbSplit > 0.0) {
    color.r = fetchPixel(position + vec2(uRgbSplit, 0.0)).r;
    color.b = fetchPixel(position - vec2(uRgbSplit, 0.0)).b;
  }
  if (uSharpen > 0.0 || uEdge > 0.0) {
    vec2 d = vec2(uKernelStep, 0.0);
    vec3 n = fetchPixel(position + d.yx).rgb;
    vec3 s = fetchPixel(position - d.yx).rgb;
    vec3 e = fetchPixel(position + d.xy).rgb;
    vec3 w = fetchPixel(position - d.xy).rgb;
    if (uSharpen > 0.0) {
      color.rgb += uSharpen * (center.rgb - 0.25 * (n + s + e + w));
      color.rgb = clamp(color.rgb, 0.0, 1.0);
    }
    if (uEdge > 0.0) {
      float ne = dot(fetchPixel(position + vec2(uKernelStep, uKernelStep)).rgb, LUMA);
      float nw = dot(fetchPixel(position + vec2(-uKernelStep, uKernelStep)).rgb, LUMA);
      float se = dot(fetchPixel(position + vec2(uKernelStep, -uKernelStep)).rgb, LUMA);
      float sw = dot(fetchPixel(position - vec2(uKernelStep, uKernelStep)).rgb, LUMA);
      float gx = (ne + 2.0 * dot(e, LUMA) + se) - (nw + 2.0 * dot(w, LUMA) + sw);
      float gy = (ne + 2.0 * dot(n, LUMA) + nw) - (se + 2.0 * dot(s, LUMA) + sw);
      float magnitude = clamp(length(vec2(gx, gy)) * EDGE_GAIN, 0.0, 1.0);
      color.rgb = mix(color.rgb, vec3(magnitude), uEdge);
    }
  }
  if (uPosterizeLevels > 1.0) {
    float steps = uPosterizeLevels - 1.0;
    color.rgb = floor(color.rgb * steps + 0.5) / steps;
  }
  if (uNegative > 0.0) {
    color.rgb = mix(color.rgb, 1.0 - color.rgb, uNegative);
  }
  if (uGrain > 0.0) {
    float noise = hashCell(uvec2(floor(gl_FragCoord.xy / uGrainCell))) * 2.0 - 1.0;
    color.rgb = clamp(color.rgb + noise * uGrain, 0.0, 1.0);
  }
  if (uScanline > 0.0) {
    float phase = mod(floor(gl_FragCoord.y), uScanlinePeriod) / uScanlinePeriod;
    color.rgb *= 1.0 - uScanline * (0.5 + 0.5 * cos(phase * 6.28318531));
  }
  if (uVignette > 0.0) {
    vec2 uv = gl_FragCoord.xy / uResolution * 2.0 - 1.0;
    color.rgb *= 1.0 - uVignette * smoothstep(0.35, 1.4, length(uv));
  }
  outColor = vec4(color.rgb, center.a);
}
`

interface MediaFxGpuProcessor {
  canvas: HTMLCanvasElement
  gl: WebGL2RenderingContext
  program: WebGLProgram
  texture: WebGLTexture
  maxTextureSize: number
  uniforms: {
    texture: WebGLUniformLocation | null
    resolution: WebGLUniformLocation | null
    pixelate: WebGLUniformLocation | null
    rgbSplit: WebGLUniformLocation | null
    kernelStep: WebGLUniformLocation | null
    sharpen: WebGLUniformLocation | null
    edge: WebGLUniformLocation | null
    posterizeLevels: WebGLUniformLocation | null
    negative: WebGLUniformLocation | null
    grain: WebGLUniformLocation | null
    grainCell: WebGLUniformLocation | null
    scanline: WebGLUniformLocation | null
    scanlinePeriod: WebGLUniformLocation | null
    vignette: WebGLUniformLocation | null
  }
}

let processor: MediaFxGpuProcessor | null = null
// Set after a non-recoverable setup failure; the page then stays on the CPU look.
let unavailable = false
let warned = false

type MediaFxGpuAvailabilityListener = (available: boolean) => void
const availabilityListeners = new Set<MediaFxGpuAvailabilityListener>()

const warnOnce = (message: string, error?: unknown) => {
  if (warned) return
  warned = true
  console.warn(`[media-fx] 高级效果已停用：${message}`, error ?? '')
}

// Cheap capability probe: never creates a WebGL context.
export const mediaFxGpuSupported = () => (
  !unavailable
  && typeof document !== 'undefined'
  && typeof WebGL2RenderingContext !== 'undefined'
)

// Lets consumers react when a lazy WebGL2 initialization proves unavailable. The
// initial cheap capability probe stays allocation-free; listeners are only notified
// after that optimistic capability is disproved at runtime.
export const subscribeMediaFxGpuAvailability = (listener: MediaFxGpuAvailabilityListener) => {
  availabilityListeners.add(listener)
  return () => availabilityListeners.delete(listener)
}

const markUnavailable = () => {
  if (unavailable) return
  unavailable = true
  const available = mediaFxGpuSupported()
  availabilityListeners.forEach((listener) => {
    try { listener(available) } catch { /* consumer notification must not break fallback */ }
  })
}

const compileShader = (gl: WebGL2RenderingContext, type: number, source: string) => {
  const shader = gl.createShader(type)
  if (!shader) throw new Error('createShader failed')
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    const log = gl.getShaderInfoLog(shader)
    gl.deleteShader(shader)
    throw new Error(`shader compile failed: ${log || 'unknown'}`)
  }
  return shader
}

const createProcessor = (): MediaFxGpuProcessor => {
  const canvas = document.createElement('canvas')
  canvas.width = 1
  canvas.height = 1
  const gl = canvas.getContext('webgl2', {
    alpha: true,
    premultipliedAlpha: false,
    antialias: false,
    depth: false,
    stencil: false,
    preserveDrawingBuffer: false,
  })
  if (!gl) throw new Error('WebGL2 context unavailable')
  const vertex = compileShader(gl, gl.VERTEX_SHADER, VERTEX_SHADER)
  const fragment = compileShader(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER)
  const program = gl.createProgram()
  if (!program) throw new Error('createProgram failed')
  gl.attachShader(program, vertex)
  gl.attachShader(program, fragment)
  gl.linkProgram(program)
  gl.deleteShader(vertex)
  gl.deleteShader(fragment)
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    throw new Error(`program link failed: ${gl.getProgramInfoLog(program) || 'unknown'}`)
  }
  const texture = gl.createTexture()
  if (!texture) throw new Error('createTexture failed')
  gl.bindTexture(gl.TEXTURE_2D, texture)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  // Straight-alpha bytes in and out, with no implicit row flip on upload.
  gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false)
  gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false)
  gl.pixelStorei(gl.UNPACK_COLORSPACE_CONVERSION_WEBGL, gl.NONE)
  gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1)
  gl.pixelStorei(gl.PACK_ALIGNMENT, 1)
  gl.disable(gl.BLEND)
  gl.disable(gl.DEPTH_TEST)
  gl.disable(gl.SCISSOR_TEST)
  gl.useProgram(program)
  const maxTextureSize = Number(gl.getParameter(gl.MAX_TEXTURE_SIZE))
  if (!Number.isFinite(maxTextureSize) || maxTextureSize <= 0) throw new Error('invalid MAX_TEXTURE_SIZE')
  const next: MediaFxGpuProcessor = {
    canvas,
    gl,
    program,
    texture,
    maxTextureSize,
    uniforms: {
      texture: gl.getUniformLocation(program, 'uTexture'),
      resolution: gl.getUniformLocation(program, 'uResolution'),
      pixelate: gl.getUniformLocation(program, 'uPixelate'),
      rgbSplit: gl.getUniformLocation(program, 'uRgbSplit'),
      kernelStep: gl.getUniformLocation(program, 'uKernelStep'),
      sharpen: gl.getUniformLocation(program, 'uSharpen'),
      edge: gl.getUniformLocation(program, 'uEdge'),
      posterizeLevels: gl.getUniformLocation(program, 'uPosterizeLevels'),
      negative: gl.getUniformLocation(program, 'uNegative'),
      grain: gl.getUniformLocation(program, 'uGrain'),
      grainCell: gl.getUniformLocation(program, 'uGrainCell'),
      scanline: gl.getUniformLocation(program, 'uScanline'),
      scanlinePeriod: gl.getUniformLocation(program, 'uScanlinePeriod'),
      vignette: gl.getUniformLocation(program, 'uVignette'),
    },
  }
  // A lost context is simply dropped; the next call lazily builds a fresh processor.
  canvas.addEventListener('webglcontextlost', () => {
    if (processor === next) processor = null
  }, { once: true })
  return next
}

const acquireProcessor = () => {
  if (processor && !processor.gl.isContextLost()) return processor
  processor = null
  if (!mediaFxGpuSupported()) return null
  try {
    processor = createProcessor()
  } catch (error) {
    markUnavailable()
    warnOnce('WebGL2 初始化失败', error)
  }
  return processor
}

// Largest texture side the processor accepts, so callers can size their raster before
// a pass. It lazily creates the page-wide processor: only call it when an advanced pass
// is about to run. null when WebGL2 is unavailable (availability listeners are notified).
export const mediaFxGpuMaxTextureSize = (): number | null => acquireProcessor()?.maxTextureSize ?? null

export interface MediaFxGpuOptions {
  // Device pixels per layout pixel of the ImageData (e.g. the Konva cache pixelRatio),
  // so block / offset / kernel / grain sizes look the same at any cache resolution.
  pixelRatio?: number
}

// Renderer mapping from normalized strengths to pixel parameters of this ImageData.
export const resolveMediaFxGpuParams = (input: MediaFxAdvanced, options: MediaFxGpuOptions = {}) => {
  const advanced = normalizeMediaFxAdvanced(input)
  const scale = typeof options.pixelRatio === 'number' && Number.isFinite(options.pixelRatio) && options.pixelRatio > 0
    ? options.pixelRatio
    : 1
  // Non-linear so the low end of the slider stays fine-grained.
  const blockPx = advanced.pixelate > 0
    ? Math.max(1, Math.round((1 + advanced.pixelate * advanced.pixelate * (PIXELATE_MAX_BLOCK_PX - 1)) * scale))
    : 1
  const devicePx = Math.max(1, Math.round(scale))
  return {
    blockPx,
    rgbSplitPx: advanced.rgbSplit * RGB_SPLIT_MAX_OFFSET_PX * scale,
    // Neighborhood distance for sharpen / edge: one layout pixel, or one block when
    // pixelated so the kernel compares neighboring blocks.
    kernelStepPx: blockPx > 1 ? blockPx : devicePx,
    sharpenAmount: advanced.sharpen * SHARPEN_MAX_AMOUNT,
    edgeMix: advanced.edge,
    // 0 = off; otherwise the number of levels per channel (>= 2).
    posterizeLevels: advanced.posterize > 0
      ? Math.round(POSTERIZE_MAX_LEVELS - advanced.posterize * (POSTERIZE_MAX_LEVELS - POSTERIZE_MIN_LEVELS))
      : 0,
    negativeMix: advanced.negative,
    grainAmplitude: advanced.grain * GRAIN_MAX_AMPLITUDE,
    grainCellPx: devicePx,
    scanlineDarken: advanced.scanline * SCANLINE_MAX_DARKEN,
    scanlinePeriodPx: Math.max(2, SCANLINE_PERIOD_PX * scale),
    vignetteDarken: advanced.vignette * VIGNETTE_MAX_DARKEN,
  }
}

// Processes imageData in place. Returns false (with imageData unchanged) when the
// effect is empty or the GPU path is unavailable for any reason.
export const applyMediaFxGpu = (imageData: ImageData, advanced: MediaFxAdvanced, options: MediaFxGpuOptions = {}): boolean => {
  if (!mediaFxAdvancedHasContent(advanced)) return false
  const { width, height, data } = imageData
  if (!width || !height || data.length !== width * height * 4) return false
  const current = acquireProcessor()
  if (!current) return false
  const { gl, canvas, texture, uniforms } = current
  if (width > current.maxTextureSize || height > current.maxTextureSize) return false
  const params = resolveMediaFxGpuParams(advanced, options)
  try {
    // Drop stale error flags so the checks below only see this pass.
    for (let index = 0; index < 8 && gl.getError() !== gl.NO_ERROR; index += 1) { /* drain */ }
    canvas.width = width
    canvas.height = height
    if (gl.drawingBufferWidth !== width || gl.drawingBufferHeight !== height) return false
    gl.viewport(0, 0, width, height)
    gl.activeTexture(gl.TEXTURE0)
    gl.bindTexture(gl.TEXTURE_2D, texture)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, data)
    gl.useProgram(current.program)
    gl.uniform1i(uniforms.texture, 0)
    gl.uniform2f(uniforms.resolution, width, height)
    gl.uniform1f(uniforms.pixelate, params.blockPx)
    gl.uniform1f(uniforms.rgbSplit, params.rgbSplitPx)
    gl.uniform1f(uniforms.kernelStep, params.kernelStepPx)
    gl.uniform1f(uniforms.sharpen, params.sharpenAmount)
    gl.uniform1f(uniforms.edge, params.edgeMix)
    gl.uniform1f(uniforms.posterizeLevels, params.posterizeLevels)
    gl.uniform1f(uniforms.negative, params.negativeMix)
    gl.uniform1f(uniforms.grain, params.grainAmplitude)
    gl.uniform1f(uniforms.grainCell, params.grainCellPx)
    gl.uniform1f(uniforms.scanline, params.scanlineDarken)
    gl.uniform1f(uniforms.scanlinePeriod, params.scanlinePeriodPx)
    gl.uniform1f(uniforms.vignette, params.vignetteDarken)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
    if (gl.getError() !== gl.NO_ERROR || gl.isContextLost()) return false
    // A failed readPixels writes nothing, so the source pixels stay intact.
    gl.readPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array(data.buffer, data.byteOffset, data.byteLength))
    return gl.getError() === gl.NO_ERROR && !gl.isContextLost()
  } catch (error) {
    warnOnce('GPU 处理失败', error)
    return false
  } finally {
    // Release the large texture / drawing buffer between cache rebuilds.
    if (!gl.isContextLost()) {
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, null)
      canvas.width = 1
      canvas.height = 1
    }
  }
}

// Konva-compatible filter step. Returns null when there is nothing to do, so callers
// never add a no-op stage (and never touch WebGL) for an all-default effect.
export const createMediaFxGpuFilter = (advanced: MediaFxAdvanced, options: MediaFxGpuOptions = {}) => {
  if (!mediaFxAdvancedHasContent(advanced) || !mediaFxGpuSupported()) return null
  const snapshot = normalizeMediaFxAdvanced(advanced)
  const filterOptions = { ...options }
  return (imageData: ImageData) => {
    applyMediaFxGpu(imageData, snapshot, filterOptions)
  }
}
