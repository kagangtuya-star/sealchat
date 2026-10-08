import {
  mediaFxAdvancedHasContent,
  mediaFxTemporalHasContent,
  normalizeMediaFxAdvanced,
  normalizeMediaFxTemporal,
  type MediaFxAdvanced,
  type MediaFxTemporal,
} from './media-fx'

// WebGL2 adapter for MediaFxAdvanced (static) and MediaFxTemporal (live).
//
// A single lazily created, page-wide processor owns the only WebGL2 context. Its main
// program is one fixed single-pass fragment shader. It has two entry points:
//
// - Static: applyMediaFxGpu processes an ImageData synchronously (ImageData -> texture ->
//   draw -> readPixels -> same ImageData). It is the last step of a static Konva cache
//   filter chain / bake: it only runs when that output is rebuilt, never per frame.
//   With bloom and glow off this is exactly the single pass above. Only when bloom or
//   glow is on, a small fixed multi-pass chain runs first (see "Light pipeline"):
//     source -> extract light (downscaled scratch A) -> blur H (A -> B) -> blur V (B -> A)
//     -> main pass with the light variant of the shader (source + A) -> readPixels
//   Its programs and two scratch targets are created lazily once per context and reused;
//   the scratch storage is shrunk to 1x1 again after every static pass.
// - Live: a temporal session keeps the static final look (the baseline) as its own
//   texture of the shared context, uploaded once per baseline. Each frame only binds
//   that texture, updates the time uniforms and draws; the frame is then copied by the
//   caller from the shared canvas (GPU to GPU), never read back into JS memory.
//
// This module owns no RAF or timers: the shared temporal clock drives live frames, and
// live frames never run the light pipeline (bloom / glow are already in the baseline).
// Any failure (no WebGL2, context creation, shader compile/link, incomplete framebuffer,
// oversized input, GL error, context loss) leaves the ImageData untouched / returns no
// frame, so the image keeps its static look instead of disappearing. A light pipeline
// failure only affects specs with bloom / glow; the single-pass effects keep working.
// WebGL objects never leave this module; only the shared canvas is handed out for an
// immediate copy.

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
// Temporal renderer constants (never persisted). Sizes are in layout pixels and are
// scaled by the live raster pixel ratio in the shader.
const TEMPORAL_GRAIN_MAX_AMPLITUDE = 0.2
// Flicker changes exposure by at most +-18%: never black, always readable.
const FLICKER_MAX_GAIN = 0.18
const SCANLINE_ROLL_MAX_DARKEN = 0.3
// Light (bloom / glow) renderer constants. Blur sizes are in layout pixels (scaled by the
// raster pixel ratio); a stronger effect lowers the bloom threshold and widens the blur.
const BLOOM_MAX_GAIN = 2.5
const BLOOM_THRESHOLD_LOW_STRENGTH = 0.82
const BLOOM_THRESHOLD_HIGH_STRENGTH = 0.5
const BLOOM_KNEE = 0.2
const BLOOM_SIGMA_RANGE_PX: [number, number] = [3, 14]
const GLOW_MAX_GAIN = 0.7
// Glow also lifts alpha inside transparent raster areas (a soft halo within the raster).
const GLOW_MAX_ALPHA_GAIN = 1.6
const GLOW_SIGMA_RANGE_PX: [number, number] = [3, 12]
// Fixed blur kernel: at most this many taps per side, radius = ceil(3 sigma). The light
// raster is downscaled until the blur fits, which also bounds the cost per pass.
const LIGHT_BLUR_MAX_RADIUS = 16
const LIGHT_BLUR_SIGMAS = 3
// Light raster budget: never more than half the input resolution and never more than
// this many pixels per scratch target (two RGBA8 targets ~ 4 MiB in total).
const LIGHT_MAX_SCALE = 0.5
export const MEDIA_FX_GPU_LIGHT_MAX_PIXELS = 512 * 1024
// Temporal time is wrapped so float precision in the shader stays fine on long sessions.
export const MEDIA_FX_TEMPORAL_TIME_WRAP_SECONDS = 3600

const VERTEX_SHADER = `#version 300 es
void main() {
  // One oversized triangle covers the viewport; no vertex buffer is needed.
  vec2 position = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  gl_Position = vec4(position * 2.0 - 1.0, 0.0, 1.0);
}
`

// Pixel space: framebuffer row y renders source row y, and readPixels returns rows
// from y = 0 upwards, so the static output keeps the exact row order of the uploaded
// ImageData without any flip. A live frame is displayed as a canvas image instead
// (bottom row last), so live passes set uFlipY to read source rows top-down; static
// passes keep uFlipY = 0 and therefore the exact v3 pixel math. texelFetch keeps
// samples exact (no interpolation).
//
// Fixed effect order (one pass, every consumer gets the same result):
//   static (advanced; time independent)
//   1. pixelate   sampling grid      (where pixels are read from)
//   2. rgbSplit   channel sampling
//   3. sharpen    3x3 neighborhood   (on the sampled grid)
//   4. edge       3x3 neighborhood
//   5. posterize  tone
//   6. negative   tone
//   L. light      bloom / glow composite (light variant only, see the light pipeline)
//   7. grain      surface texture    (deterministic per output pixel, no time input)
//   8. scanline   surface texture
//   9. vignette   lens / frame edge
//   temporal (live only; the texture is then already the baked static baseline, so
//   every static uniform above is 0 there and static passes have every temporal one 0)
//   T1. glitch        row-band displacement + short channel offset (moves RGBA together)
//   T2. flicker       frame-wide exposure
//   T3. grain         time-varying grain
//   T4. scanlineRoll  scrolling scanlines + a slow rolling band
// Each step is skipped by a uniform branch when its strength is 0; the neighborhood
// (at most 8 extra fetches) is only read when sharpen or edge is on. Only rgb is
// changed, alpha stays the source alpha of the sampled pixel; only glitch moves a whole
// sampled pixel (alpha included), it never invents alpha. The light composite (L) is
// compiled only into the light variant (MEDIA_FX_LIGHT): the single-pass program has no
// light sampler at all. It may raise alpha inside the raster for the light halo.
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
uniform float uFlipY;
uniform float uTime;
uniform float uTemporalScale;
uniform float uTemporalGrain;
uniform float uFlicker;
uniform float uGlitch;
uniform float uScanlineRoll;
#ifdef MEDIA_FX_LIGHT
uniform sampler2D uLight;
uniform float uLightGain;
uniform float uGlowAlpha;
#endif
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

// Smooth 1D value noise in [0, 1] over a time axis; salt picks an independent curve.
float timeNoise(float t, uint salt) {
  float cell = floor(t);
  float a = hashCell(uvec2(uint(cell), salt));
  float b = hashCell(uvec2(uint(cell) + 1u, salt));
  return mix(a, b, smoothstep(0.0, 1.0, t - cell));
}

void main() {
  // Image-space pixel of this fragment (row 0 = first source row).
  vec2 pixel = floor(gl_FragCoord.xy);
  if (uFlipY > 0.5) {
    pixel.y = uResolution.y - 1.0 - pixel.y;
  }
  vec2 position = pixel;
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
  float alpha = center.a;
#ifdef MEDIA_FX_LIGHT
  {
    // Blurred light: premultiplied emission + coverage, bilinear upscaled.
    vec4 light = texture(uLight, (position + 0.5) / uResolution);
    // Glow halo behind (dst-over) the less opaque pixels: the hue of the nearby light at
    // full brightness, its opacity from the coverage and how bright that light is, so
    // dark content never casts a shadow-like halo.
    vec3 average = light.a > 0.0001 ? light.rgb / light.a : vec3(0.0);
    float peak = max(average.r, max(average.g, average.b));
    vec3 haloColor = peak > 0.0001 ? average / peak : vec3(0.0);
    float halo = clamp(light.a * uGlowAlpha, 0.0, 1.0) * clamp(peak, 0.0, 1.0) * (1.0 - alpha);
    vec3 lit = color.rgb * alpha + haloColor * halo;
    float litAlpha = alpha + halo;
    // Emission is screen-blended in premultiplied space: highlights saturate, never clip.
    vec3 emission = min(light.rgb * uLightGain, vec3(1.0));
    lit = lit + emission - lit * emission;
    litAlpha = max(litAlpha, max(lit.r, max(lit.g, lit.b)));
    color.rgb = litAlpha > 0.0 ? clamp(lit / litAlpha, 0.0, 1.0) : color.rgb;
    alpha = litAlpha;
  }
#endif
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
  if (uGlitch > 0.0) {
    // About nine slices per second; a slice glitches with a chance that grows with the
    // strength, so the effect stays intermittent. Inside a glitching slice, random row
    // bands shift sideways and get a short red / blue offset.
    float slice = floor(uTime * 9.0);
    uint sliceId = uint(slice);
    if (hashCell(uvec2(sliceId, 101u)) < 0.1 + 0.6 * uGlitch) {
      float bandHeight = max(1.0, floor(mix(4.0, 36.0, hashCell(uvec2(sliceId, 7u))) * uTemporalScale));
      uint band = uint(floor(pixel.y / bandHeight));
      if (hashCell(uvec2(band, sliceId ^ 0x5bd1e995u)) < 0.25 + 0.35 * uGlitch) {
        float direction = hashCell(uvec2(band, sliceId ^ 0x27d4eb2du)) * 2.0 - 1.0;
        float shift = floor(direction * uGlitch * 32.0 * uTemporalScale);
        float chroma = floor(uGlitch * 6.0 * uTemporalScale);
        vec4 shifted = fetchPixel(pixel - vec2(shift, 0.0));
        color = shifted;
        alpha = shifted.a;
        if (chroma > 0.0) {
          color.r = fetchPixel(pixel - vec2(shift - chroma, 0.0)).r;
          color.b = fetchPixel(pixel - vec2(shift + chroma, 0.0)).b;
        }
      }
    }
  }
  if (uFlicker > 0.0) {
    float wave = timeNoise(uTime * 10.0, 17u) * 0.65 + timeNoise(uTime * 2.7, 43u) * 0.35;
    color.rgb = clamp(color.rgb * (1.0 + uFlicker * (wave * 2.0 - 1.0)), 0.0, 1.0);
  }
  if (uTemporalGrain > 0.0) {
    // New grain about 24 times per second; the same time always gives the same grain.
    uint frame = uint(floor(uTime * 24.0));
    uvec2 cell = uvec2(floor(pixel / max(1.0, floor(uTemporalScale + 0.5))));
    float noise = hashCell(cell ^ uvec2(frame * 0x9e3779b9u, frame * 0x7f4a7c15u)) * 2.0 - 1.0;
    color.rgb = clamp(color.rgb + noise * uTemporalGrain, 0.0, 1.0);
  }
  if (uScanlineRoll > 0.0) {
    // Fine lines (4 layout px) scroll down at 30 layout px/s; a soft bright band rolls
    // over the picture about every eight seconds.
    float period = max(2.0, 4.0 * uTemporalScale);
    float phase = fract((pixel.y - uTime * 30.0 * uTemporalScale) / period);
    float lines = 0.5 + 0.5 * cos(phase * 6.28318531);
    float offset = pixel.y / uResolution.y - fract(uTime * 0.12);
    offset -= floor(offset + 0.5);
    float rollBand = exp(-offset * offset * 180.0);
    color.rgb = clamp(color.rgb * (1.0 - uScanlineRoll * lines) + uScanlineRoll * 0.35 * rollBand, 0.0, 1.0);
  }
  outColor = vec4(color.rgb, alpha);
}
`

// Light variant of the main shader: same effects plus the bloom / glow composite.
const LIGHT_FRAGMENT_SHADER = FRAGMENT_SHADER.replace('#version 300 es\n', '#version 300 es\n#define MEDIA_FX_LIGHT 1\n')

// Light extraction: one light texel averages a fixed 4x4 grid of the source pixels it
// covers (premultiplied, so transparent pixels add no color). rgb is the emission:
// highlights above the bloom threshold plus every visible color for glow, weighted so
// it fits 0..1 (the composite multiplies the gain back); alpha is the coverage that
// glow turns into a halo. Same row order as the source, no flip.
const LIGHT_EXTRACT_SHADER = `#version 300 es
precision highp float;
precision highp int;
uniform sampler2D uSource;
uniform vec2 uSourceSize;
uniform vec2 uLightSize;
uniform float uBloomWeight;
uniform float uGlowWeight;
uniform float uThreshold;
uniform float uKnee;
out vec4 outColor;

const vec3 LUMA = vec3(0.2126, 0.7152, 0.0722);

void main() {
  vec2 cell = uSourceSize / uLightSize;
  vec2 origin = floor(gl_FragCoord.xy) * cell;
  ivec2 last = ivec2(uSourceSize) - 1;
  vec3 emission = vec3(0.0);
  float coverage = 0.0;
  for (int y = 0; y < 4; y++) {
    for (int x = 0; x < 4; x++) {
      vec2 point = origin + (vec2(float(x), float(y)) + 0.5) * cell * 0.25;
      vec4 texel = texelFetch(uSource, clamp(ivec2(floor(point)), ivec2(0), last), 0);
      float bright = smoothstep(uThreshold, uThreshold + uKnee, dot(texel.rgb, LUMA));
      emission += texel.rgb * texel.a * (uBloomWeight * bright + uGlowWeight);
      coverage += texel.a;
    }
  }
  outColor = vec4(emission / 16.0, coverage / 16.0);
}
`

// Separable Gaussian blur over premultiplied light; one direction per pass. The loop
// bound is a constant, the effective radius a uniform (never above the constant).
const LIGHT_BLUR_SHADER = `#version 300 es
precision highp float;
precision highp int;
uniform sampler2D uInput;
uniform vec2 uSize;
uniform ivec2 uDirection;
uniform int uRadius;
uniform float uSigma;
out vec4 outColor;

void main() {
  ivec2 pixel = ivec2(floor(gl_FragCoord.xy));
  ivec2 last = ivec2(uSize) - 1;
  vec4 sum = vec4(0.0);
  float total = 0.0;
  for (int i = -${LIGHT_BLUR_MAX_RADIUS}; i <= ${LIGHT_BLUR_MAX_RADIUS}; i++) {
    if (i < -uRadius || i > uRadius) continue;
    float weight = exp(-0.5 * float(i * i) / (uSigma * uSigma));
    sum += weight * texelFetch(uInput, clamp(pixel + uDirection * i, ivec2(0), last), 0);
    total += weight;
  }
  outColor = sum / total;
}
`

interface MediaFxGpuMainUniforms {
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
  flipY: WebGLUniformLocation | null
  time: WebGLUniformLocation | null
  temporalScale: WebGLUniformLocation | null
  temporalGrain: WebGLUniformLocation | null
  flicker: WebGLUniformLocation | null
  glitch: WebGLUniformLocation | null
  scanlineRoll: WebGLUniformLocation | null
}

interface MediaFxGpuLightTarget {
  texture: WebGLTexture
  framebuffer: WebGLFramebuffer
}

// Lazily created per context, only for specs with bloom / glow, then reused.
interface MediaFxGpuLightPipeline {
  composite: WebGLProgram
  compositeUniforms: MediaFxGpuMainUniforms & {
    light: WebGLUniformLocation | null
    lightGain: WebGLUniformLocation | null
    glowAlpha: WebGLUniformLocation | null
  }
  extract: WebGLProgram
  extractUniforms: {
    source: WebGLUniformLocation | null
    sourceSize: WebGLUniformLocation | null
    lightSize: WebGLUniformLocation | null
    bloomWeight: WebGLUniformLocation | null
    glowWeight: WebGLUniformLocation | null
    threshold: WebGLUniformLocation | null
    knee: WebGLUniformLocation | null
  }
  blur: WebGLProgram
  blurUniforms: {
    input: WebGLUniformLocation | null
    size: WebGLUniformLocation | null
    direction: WebGLUniformLocation | null
    radius: WebGLUniformLocation | null
    sigma: WebGLUniformLocation | null
  }
  // Ping-pong: extract -> [0], blur H [0] -> [1], blur V [1] -> [0]; [0] feeds the composite.
  targets: [MediaFxGpuLightTarget, MediaFxGpuLightTarget]
}

interface MediaFxGpuProcessor {
  canvas: HTMLCanvasElement
  gl: WebGL2RenderingContext
  program: WebGLProgram
  texture: WebGLTexture
  maxTextureSize: number
  uniforms: MediaFxGpuMainUniforms
  light: MediaFxGpuLightPipeline | null
  // A light pipeline that failed to build is not retried in this context.
  lightFailed: boolean
}

let processor: MediaFxGpuProcessor | null = null
// Set after a non-recoverable setup failure; the page then stays on the CPU look.
let unavailable = false
let warned = false
let temporalWarned = false
let lightWarned = false

type MediaFxGpuAvailabilityListener = (available: boolean) => void
const availabilityListeners = new Set<MediaFxGpuAvailabilityListener>()
const lightAvailabilityListeners = new Set<MediaFxGpuAvailabilityListener>()

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

export const mediaFxGpuLightSupported = () => (
  mediaFxGpuSupported()
  && processor?.lightFailed !== true
)

const notifyLightAvailability = () => {
  const available = mediaFxGpuLightSupported()
  lightAvailabilityListeners.forEach((listener) => {
    try { listener(available) } catch { /* consumer notification must not break fallback */ }
  })
}

// Lets consumers react when a lazy WebGL2 initialization proves unavailable. The
// initial cheap capability probe stays allocation-free; listeners are only notified
// after that optimistic capability is disproved at runtime.
export const subscribeMediaFxGpuAvailability = (listener: MediaFxGpuAvailabilityListener) => {
  availabilityListeners.add(listener)
  return () => availabilityListeners.delete(listener)
}

export const subscribeMediaFxGpuLightAvailability = (listener: MediaFxGpuAvailabilityListener) => {
  lightAvailabilityListeners.add(listener)
  return () => lightAvailabilityListeners.delete(listener)
}

const warnLightOnce = (message: string, error?: unknown) => {
  if (lightWarned) return
  lightWarned = true
  console.warn(`[media-fx] 光效（泛光 / 辉光）处理失败，保留原有静态外观：${message}`, error ?? '')
}

const markLightUnavailable = (current: MediaFxGpuProcessor) => {
  if (current.gl.isContextLost() || current.lightFailed) return
  current.lightFailed = true
  notifyLightAvailability()
}

const warnTemporalOnce = (error: unknown) => {
  if (temporalWarned) return
  temporalWarned = true
  console.warn('[media-fx] 动态像素效果渲染失败，保持静态效果', error ?? '')
}

const markUnavailable = () => {
  if (unavailable) return
  unavailable = true
  const available = mediaFxGpuSupported()
  availabilityListeners.forEach((listener) => {
    try { listener(available) } catch { /* consumer notification must not break fallback */ }
  })
  notifyLightAvailability()
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

const linkProgram = (gl: WebGL2RenderingContext, fragmentSource: string) => {
  const vertex = compileShader(gl, gl.VERTEX_SHADER, VERTEX_SHADER)
  let fragment: WebGLShader
  try {
    fragment = compileShader(gl, gl.FRAGMENT_SHADER, fragmentSource)
  } catch (error) {
    gl.deleteShader(vertex)
    throw error
  }
  const program = gl.createProgram()
  if (!program) {
    gl.deleteShader(vertex)
    gl.deleteShader(fragment)
    throw new Error('createProgram failed')
  }
  gl.attachShader(program, vertex)
  gl.attachShader(program, fragment)
  gl.linkProgram(program)
  gl.deleteShader(vertex)
  gl.deleteShader(fragment)
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    const log = gl.getProgramInfoLog(program)
    gl.deleteProgram(program)
    throw new Error(`program link failed: ${log || 'unknown'}`)
  }
  return program
}

const readMainUniforms = (gl: WebGL2RenderingContext, program: WebGLProgram): MediaFxGpuMainUniforms => ({
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
  flipY: gl.getUniformLocation(program, 'uFlipY'),
  time: gl.getUniformLocation(program, 'uTime'),
  temporalScale: gl.getUniformLocation(program, 'uTemporalScale'),
  temporalGrain: gl.getUniformLocation(program, 'uTemporalGrain'),
  flicker: gl.getUniformLocation(program, 'uFlicker'),
  glitch: gl.getUniformLocation(program, 'uGlitch'),
  scanlineRoll: gl.getUniformLocation(program, 'uScanlineRoll'),
})

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
  const program = linkProgram(gl, FRAGMENT_SHADER)
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
    uniforms: readMainUniforms(gl, program),
    light: null,
    lightFailed: false,
  }
  // A lost context is simply dropped; the next call lazily builds a fresh processor.
  canvas.addEventListener('webglcontextlost', () => {
    if (processor === next) {
      processor = null
      // A fresh context may support the light pipeline again, so return to optimistic
      // availability until the replacement context proves otherwise.
      notifyLightAvailability()
    }
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

type MediaFxGpuAdvancedParams = ReturnType<typeof resolveMediaFxGpuParams>

const OFF_ADVANCED_PARAMS: MediaFxGpuAdvancedParams = resolveMediaFxGpuParams(normalizeMediaFxAdvanced(null))

const mixRange = (range: [number, number], amount: number) => range[0] + (range[1] - range[0]) * amount

// Renderer mapping of bloom / glow. `active` is false when both are off: the static pass
// then never touches the light pipeline. Weights split the 0..1 extraction range between
// bloom and glow; `gain` restores their absolute strength in the composite.
export const resolveMediaFxGpuLightParams = (input: MediaFxAdvanced, options: MediaFxGpuOptions = {}) => {
  const advanced = normalizeMediaFxAdvanced(input)
  const scale = typeof options.pixelRatio === 'number' && Number.isFinite(options.pixelRatio) && options.pixelRatio > 0
    ? options.pixelRatio
    : 1
  const bloomGain = advanced.bloom * BLOOM_MAX_GAIN
  const glowGain = advanced.glow * GLOW_MAX_GAIN
  const gain = bloomGain + glowGain
  // The wider of the two blurs is used for the shared light buffer.
  const sigmaLayoutPx = Math.max(
    advanced.bloom > 0 ? mixRange(BLOOM_SIGMA_RANGE_PX, advanced.bloom) : 0,
    advanced.glow > 0 ? mixRange(GLOW_SIGMA_RANGE_PX, advanced.glow) : 0,
  )
  return {
    active: gain > 0,
    bloomWeight: gain > 0 ? bloomGain / gain : 0,
    glowWeight: gain > 0 ? glowGain / gain : 0,
    gain,
    glowAlpha: advanced.glow * GLOW_MAX_ALPHA_GAIN,
    threshold: BLOOM_THRESHOLD_LOW_STRENGTH + (BLOOM_THRESHOLD_HIGH_STRENGTH - BLOOM_THRESHOLD_LOW_STRENGTH) * advanced.bloom,
    knee: BLOOM_KNEE,
    sigmaPx: sigmaLayoutPx * scale,
  }
}

export type MediaFxGpuLightParams = ReturnType<typeof resolveMediaFxGpuLightParams>

// Size of the light scratch targets for one static pass: at most half the input size,
// at most MEDIA_FX_GPU_LIGHT_MAX_PIXELS, and small enough that the blur radius fits the
// fixed kernel. Pure, so callers / tests can check the budget without WebGL.
export const resolveMediaFxGpuLightRaster = (width: number, height: number, sigmaPx: number) => {
  if (!(width >= 1 && height >= 1)) return null
  const safeSigma = Number.isFinite(sigmaPx) && sigmaPx > 0 ? sigmaPx : 1
  const scale = Math.min(
    LIGHT_MAX_SCALE,
    Math.sqrt(MEDIA_FX_GPU_LIGHT_MAX_PIXELS / (width * height)),
    LIGHT_BLUR_MAX_RADIUS / LIGHT_BLUR_SIGMAS / safeSigma,
  )
  const lightWidth = Math.max(1, Math.min(width, Math.floor(width * scale)))
  const lightHeight = Math.max(1, Math.min(height, Math.floor(height * scale)))
  const sigma = Math.max(0.5, safeSigma * Math.min(lightWidth / width, lightHeight / height))
  return {
    width: lightWidth,
    height: lightHeight,
    sigma,
    radius: Math.min(LIGHT_BLUR_MAX_RADIUS, Math.max(1, Math.ceil(sigma * LIGHT_BLUR_SIGMAS))),
  }
}

export interface MediaFxTemporalGpuOptions {
  // Device pixels per layout pixel of the live raster, so grain cells, glitch bands and
  // scanline periods keep their layout size at any DPR / raster scale.
  pixelRatio?: number
}

// Renderer mapping of the normalized temporal strengths. Speed is not a GPU parameter:
// the temporal player scales the time it passes in.
export const resolveMediaFxTemporalGpuParams = (input: MediaFxTemporal, options: MediaFxTemporalGpuOptions = {}) => {
  const temporal = normalizeMediaFxTemporal(input)
  const scale = typeof options.pixelRatio === 'number' && Number.isFinite(options.pixelRatio) && options.pixelRatio > 0
    ? options.pixelRatio
    : 1
  return {
    scale,
    grainAmplitude: temporal.grain * TEMPORAL_GRAIN_MAX_AMPLITUDE,
    flickerGain: temporal.flicker * FLICKER_MAX_GAIN,
    glitch: temporal.glitch,
    scanlineRollDarken: temporal.scanlineRoll * SCANLINE_ROLL_MAX_DARKEN,
  }
}

type MediaFxGpuTemporalParams = ReturnType<typeof resolveMediaFxTemporalGpuParams>

const OFF_TEMPORAL_PARAMS: MediaFxGpuTemporalParams = { scale: 1, grainAmplitude: 0, flickerGain: 0, glitch: 0, scanlineRollDarken: 0 }

const wrapTemporalTime = (seconds: number) => {
  if (!Number.isFinite(seconds) || seconds <= 0) return 0
  return seconds % MEDIA_FX_TEMPORAL_TIME_WRAP_SECONDS
}

// Every uniform is written on every pass: static and live passes share one program (and
// the light variant has the same main uniforms), so no pass inherits another's values.
const writeUniforms = (
  gl: WebGL2RenderingContext,
  uniforms: MediaFxGpuMainUniforms,
  width: number,
  height: number,
  advanced: MediaFxGpuAdvancedParams,
  temporal: MediaFxGpuTemporalParams,
  live: { flipY: boolean, time: number },
) => {
  gl.uniform1i(uniforms.texture, 0)
  gl.uniform2f(uniforms.resolution, width, height)
  gl.uniform1f(uniforms.pixelate, advanced.blockPx)
  gl.uniform1f(uniforms.rgbSplit, advanced.rgbSplitPx)
  gl.uniform1f(uniforms.kernelStep, advanced.kernelStepPx)
  gl.uniform1f(uniforms.sharpen, advanced.sharpenAmount)
  gl.uniform1f(uniforms.edge, advanced.edgeMix)
  gl.uniform1f(uniforms.posterizeLevels, advanced.posterizeLevels)
  gl.uniform1f(uniforms.negative, advanced.negativeMix)
  gl.uniform1f(uniforms.grain, advanced.grainAmplitude)
  gl.uniform1f(uniforms.grainCell, advanced.grainCellPx)
  gl.uniform1f(uniforms.scanline, advanced.scanlineDarken)
  gl.uniform1f(uniforms.scanlinePeriod, advanced.scanlinePeriodPx)
  gl.uniform1f(uniforms.vignette, advanced.vignetteDarken)
  gl.uniform1f(uniforms.flipY, live.flipY ? 1 : 0)
  gl.uniform1f(uniforms.time, live.time)
  gl.uniform1f(uniforms.temporalScale, temporal.scale)
  gl.uniform1f(uniforms.temporalGrain, temporal.grainAmplitude)
  gl.uniform1f(uniforms.flicker, temporal.flickerGain)
  gl.uniform1f(uniforms.glitch, temporal.glitch)
  gl.uniform1f(uniforms.scanlineRoll, temporal.scanlineRollDarken)
}

// ---------------------------------------------------------------------------
// Light pipeline (bloom / glow): a fixed extract -> blur H -> blur V chain
// ---------------------------------------------------------------------------

const createLightTarget = (gl: WebGL2RenderingContext, track: { textures: WebGLTexture[], framebuffers: WebGLFramebuffer[] }) => {
  const texture = gl.createTexture()
  if (!texture) throw new Error('createTexture failed')
  track.textures.push(texture)
  gl.bindTexture(gl.TEXTURE_2D, texture)
  // Linear: the composite upscales the light bilinearly (passes use exact texelFetch).
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, null)
  const framebuffer = gl.createFramebuffer()
  if (!framebuffer) throw new Error('createFramebuffer failed')
  track.framebuffers.push(framebuffer)
  gl.bindFramebuffer(gl.FRAMEBUFFER, framebuffer)
  gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, texture, 0)
  gl.bindFramebuffer(gl.FRAMEBUFFER, null)
  return { texture, framebuffer }
}

// Built once per context on the first bloom / glow pass, then reused for every later
// pass (slider edits never compile again). A build failure only disables the light
// effects of this context; the single-pass program is untouched.
const acquireLightPipeline = (current: MediaFxGpuProcessor): MediaFxGpuLightPipeline | null => {
  if (current.lightFailed) return null
  if (current.light) return current.light
  const { gl } = current
  const track = { programs: [] as WebGLProgram[], textures: [] as WebGLTexture[], framebuffers: [] as WebGLFramebuffer[] }
  const link = (source: string) => {
    const program = linkProgram(gl, source)
    track.programs.push(program)
    return program
  }
  try {
    const composite = link(LIGHT_FRAGMENT_SHADER)
    const extract = link(LIGHT_EXTRACT_SHADER)
    const blur = link(LIGHT_BLUR_SHADER)
    const targets: [MediaFxGpuLightTarget, MediaFxGpuLightTarget] = [createLightTarget(gl, track), createLightTarget(gl, track)]
    gl.bindTexture(gl.TEXTURE_2D, current.texture)
    current.light = {
      composite,
      compositeUniforms: {
        ...readMainUniforms(gl, composite),
        light: gl.getUniformLocation(composite, 'uLight'),
        lightGain: gl.getUniformLocation(composite, 'uLightGain'),
        glowAlpha: gl.getUniformLocation(composite, 'uGlowAlpha'),
      },
      extract,
      extractUniforms: {
        source: gl.getUniformLocation(extract, 'uSource'),
        sourceSize: gl.getUniformLocation(extract, 'uSourceSize'),
        lightSize: gl.getUniformLocation(extract, 'uLightSize'),
        bloomWeight: gl.getUniformLocation(extract, 'uBloomWeight'),
        glowWeight: gl.getUniformLocation(extract, 'uGlowWeight'),
        threshold: gl.getUniformLocation(extract, 'uThreshold'),
        knee: gl.getUniformLocation(extract, 'uKnee'),
      },
      blur,
      blurUniforms: {
        input: gl.getUniformLocation(blur, 'uInput'),
        size: gl.getUniformLocation(blur, 'uSize'),
        direction: gl.getUniformLocation(blur, 'uDirection'),
        radius: gl.getUniformLocation(blur, 'uRadius'),
        sigma: gl.getUniformLocation(blur, 'uSigma'),
      },
      targets,
    }
    return current.light
  } catch (error) {
    if (!gl.isContextLost()) {
      gl.bindFramebuffer(gl.FRAMEBUFFER, null)
      track.framebuffers.forEach((framebuffer) => gl.deleteFramebuffer(framebuffer))
      track.textures.forEach((texture) => gl.deleteTexture(texture))
      track.programs.forEach((program) => gl.deleteProgram(program))
      markLightUnavailable(current)
    }
    warnLightOnce('多通道渲染初始化失败', error)
    return null
  }
}

// Runs extract -> blur H -> blur V into targets[0]. Every pass binds its own framebuffer,
// viewport, program, input texture (unit 0) and uniforms; unit 1 stays empty, and no
// texture is ever read while it is the current render target.
const runLightPasses = (
  current: MediaFxGpuProcessor,
  pipeline: MediaFxGpuLightPipeline,
  light: MediaFxGpuLightParams,
  width: number,
  height: number,
) => {
  const { gl } = current
  const raster = resolveMediaFxGpuLightRaster(width, height, light.sigmaPx)
  if (!raster) return false
  const [first, second] = pipeline.targets
  gl.activeTexture(gl.TEXTURE1)
  gl.bindTexture(gl.TEXTURE_2D, null)
  gl.activeTexture(gl.TEXTURE0)
  for (const target of pipeline.targets) {
    gl.bindTexture(gl.TEXTURE_2D, target.texture)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, raster.width, raster.height, 0, gl.RGBA, gl.UNSIGNED_BYTE, null)
  }
  if (gl.getError() !== gl.NO_ERROR) return false
  for (const target of pipeline.targets) {
    gl.bindFramebuffer(gl.FRAMEBUFFER, target.framebuffer)
    if (gl.checkFramebufferStatus(gl.FRAMEBUFFER) !== gl.FRAMEBUFFER_COMPLETE) {
      // RGBA8 scratch framebuffer incompleteness is a context capability failure, not an
      // input-size failure. Latch only the light path so slider edits do not repeat a
      // deterministically failing multi-pass setup; the original single-pass stays live.
      markLightUnavailable(current)
      warnLightOnce('帧缓冲不完整')
      return false
    }
  }

  // 1. Extract: source (unit 0) -> first.
  gl.bindFramebuffer(gl.FRAMEBUFFER, first.framebuffer)
  gl.viewport(0, 0, raster.width, raster.height)
  gl.useProgram(pipeline.extract)
  gl.activeTexture(gl.TEXTURE0)
  gl.bindTexture(gl.TEXTURE_2D, current.texture)
  const extract = pipeline.extractUniforms
  gl.uniform1i(extract.source, 0)
  gl.uniform2f(extract.sourceSize, width, height)
  gl.uniform2f(extract.lightSize, raster.width, raster.height)
  gl.uniform1f(extract.bloomWeight, light.bloomWeight)
  gl.uniform1f(extract.glowWeight, light.glowWeight)
  gl.uniform1f(extract.threshold, light.threshold)
  gl.uniform1f(extract.knee, light.knee)
  gl.drawArrays(gl.TRIANGLES, 0, 3)

  // 2. / 3. Separable blur: first -> second (horizontal), second -> first (vertical).
  const blur = pipeline.blurUniforms
  for (const [input, output, dx, dy] of [[first, second, 1, 0], [second, first, 0, 1]] as const) {
    gl.bindFramebuffer(gl.FRAMEBUFFER, output.framebuffer)
    gl.viewport(0, 0, raster.width, raster.height)
    gl.useProgram(pipeline.blur)
    gl.activeTexture(gl.TEXTURE0)
    gl.bindTexture(gl.TEXTURE_2D, input.texture)
    gl.uniform1i(blur.input, 0)
    gl.uniform2f(blur.size, raster.width, raster.height)
    gl.uniform2i(blur.direction, dx, dy)
    gl.uniform1i(blur.radius, raster.radius)
    gl.uniform1f(blur.sigma, raster.sigma)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
  }
  return gl.getError() === gl.NO_ERROR && !gl.isContextLost()
}

// Shrinks the scratch storage again: no large light buffer outlives a static pass.
const releaseLightTargets = (gl: WebGL2RenderingContext, pipeline: MediaFxGpuLightPipeline) => {
  gl.bindFramebuffer(gl.FRAMEBUFFER, null)
  gl.activeTexture(gl.TEXTURE1)
  gl.bindTexture(gl.TEXTURE_2D, null)
  gl.activeTexture(gl.TEXTURE0)
  for (const target of pipeline.targets) {
    gl.bindTexture(gl.TEXTURE_2D, target.texture)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, null)
  }
}

// Processes imageData in place. Returns false (with imageData unchanged) when the
// effect is empty or the GPU path is unavailable for any reason. Only a complete result
// is read back: every pass is checked before the single readPixels.
export const applyMediaFxGpu = (imageData: ImageData, advanced: MediaFxAdvanced, options: MediaFxGpuOptions = {}): boolean => {
  if (!mediaFxAdvancedHasContent(advanced)) return false
  const { width, height, data } = imageData
  if (!width || !height || data.length !== width * height * 4) return false
  const current = acquireProcessor()
  if (!current) return false
  const { gl, canvas, texture } = current
  if (width > current.maxTextureSize || height > current.maxTextureSize) return false
  const params = resolveMediaFxGpuParams(advanced, options)
  const light = resolveMediaFxGpuLightParams(advanced, options)
  // Bloom / glow off: the original single pass; the light pipeline is never built.
  const pipeline = light.active ? acquireLightPipeline(current) : null
  if (light.active && !pipeline) return false
  try {
    // Drop stale error flags so the checks below only see this pass.
    for (let index = 0; index < 8 && gl.getError() !== gl.NO_ERROR; index += 1) { /* drain */ }
    canvas.width = width
    canvas.height = height
    if (gl.drawingBufferWidth !== width || gl.drawingBufferHeight !== height) return false
    gl.activeTexture(gl.TEXTURE0)
    gl.bindTexture(gl.TEXTURE_2D, texture)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, data)
    if (pipeline) {
      if (!runLightPasses(current, pipeline, light, width, height)) return false
      // Final pass: the light variant reads the source (unit 0) and the light (unit 1).
      gl.bindFramebuffer(gl.FRAMEBUFFER, null)
      gl.viewport(0, 0, width, height)
      gl.useProgram(pipeline.composite)
      gl.activeTexture(gl.TEXTURE0)
      gl.bindTexture(gl.TEXTURE_2D, texture)
      gl.activeTexture(gl.TEXTURE1)
      gl.bindTexture(gl.TEXTURE_2D, pipeline.targets[0].texture)
      const uniforms = pipeline.compositeUniforms
      writeUniforms(gl, uniforms, width, height, params, OFF_TEMPORAL_PARAMS, { flipY: false, time: 0 })
      gl.uniform1i(uniforms.light, 1)
      gl.uniform1f(uniforms.lightGain, light.gain)
      gl.uniform1f(uniforms.glowAlpha, light.glowAlpha)
    } else {
      gl.bindFramebuffer(gl.FRAMEBUFFER, null)
      gl.viewport(0, 0, width, height)
      gl.useProgram(current.program)
      // Static pass: no flip, every temporal effect off, so the v3 pixel math is unchanged.
      writeUniforms(gl, current.uniforms, width, height, params, OFF_TEMPORAL_PARAMS, { flipY: false, time: 0 })
    }
    gl.drawArrays(gl.TRIANGLES, 0, 3)
    if (gl.getError() !== gl.NO_ERROR || gl.isContextLost()) return false
    // A failed readPixels writes nothing, so the source pixels stay intact.
    gl.readPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array(data.buffer, data.byteOffset, data.byteLength))
    return gl.getError() === gl.NO_ERROR && !gl.isContextLost()
  } catch (error) {
    if (pipeline) warnLightOnce('多通道渲染失败', error)
    else warnOnce('GPU 处理失败', error)
    return false
  } finally {
    // Release the large textures / drawing buffer between cache rebuilds.
    if (!gl.isContextLost()) {
      if (pipeline) releaseLightTargets(gl, pipeline)
      gl.activeTexture(gl.TEXTURE0)
      gl.bindTexture(gl.TEXTURE_2D, texture)
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

// ---------------------------------------------------------------------------
// Live temporal sessions
// ---------------------------------------------------------------------------

export interface MediaFxTemporalBaseline {
  // The static final look (source -> fit -> basic filter -> advanced), already at the
  // live raster size. It is only read when the texture has to be (re)uploaded.
  source: TexImageSource
  width: number
  height: number
  // Device pixels per layout pixel of this raster.
  pixelRatio: number
}

export interface MediaFxTemporalGpuFrame {
  // The shared processor canvas. Copy the region right away (drawImage); it is reused by
  // the next session's frame and is not preserved after the current task.
  canvas: HTMLCanvasElement
  x: number
  y: number
  width: number
  height: number
}

export interface MediaFxTemporalGpuSession {
  // Sets or clears the baseline. Never uploads by itself: the next frame uploads it once.
  setBaseline(baseline: MediaFxTemporalBaseline | null): void
  // Draws one live frame for `timeSeconds` of (speed-scaled) temporal time. null when
  // there is nothing to draw or the GPU path failed; the caller keeps its static look.
  render(temporal: MediaFxTemporal, timeSeconds: number): MediaFxTemporalGpuFrame | null
  dispose(): void
}

const liveSessions = new Set<object>()

// Releases the shared drawing buffer once no live session is left.
const shrinkSharedCanvas = () => {
  const current = processor
  if (liveSessions.size || !current || current.gl.isContextLost()) return
  current.canvas.width = 1
  current.canvas.height = 1
}

export const createMediaFxTemporalGpuSession = (): MediaFxTemporalGpuSession => {
  const token = {}
  let baseline: MediaFxTemporalBaseline | null = null
  // The texture belongs to one processor (context); a new context means a new texture.
  let owner: MediaFxGpuProcessor | null = null
  let texture: WebGLTexture | null = null
  let uploaded = false
  let disposed = false

  const releaseTexture = () => {
    if (owner && texture && !owner.gl.isContextLost()) owner.gl.deleteTexture(texture)
    owner = null
    texture = null
    uploaded = false
  }

  return {
    setBaseline(next) {
      if (disposed) return
      baseline = next && next.width >= 1 && next.height >= 1 ? next : null
      uploaded = false
      if (!baseline) {
        releaseTexture()
        liveSessions.delete(token)
        shrinkSharedCanvas()
      }
    },
    render(temporal, timeSeconds) {
      if (disposed || !baseline || !mediaFxTemporalHasContent(temporal)) return null
      const current = acquireProcessor()
      if (!current) return null
      const { gl, canvas } = current
      const { width, height } = baseline
      if (width > current.maxTextureSize || height > current.maxTextureSize) return null
      try {
        for (let index = 0; index < 8 && gl.getError() !== gl.NO_ERROR; index += 1) { /* drain */ }
        if (owner !== current) {
          // Textures of a lost context are gone; rebuild lazily in the new one.
          owner = current
          texture = null
          uploaded = false
        }
        if (!texture) {
          texture = gl.createTexture()
          if (!texture) return null
          gl.bindTexture(gl.TEXTURE_2D, texture)
          gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
          gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
          gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
          gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
        }
        liveSessions.add(token)
        gl.activeTexture(gl.TEXTURE0)
        gl.bindTexture(gl.TEXTURE_2D, texture)
        if (!uploaded) {
          // The only upload: once per baseline (and once more after a context loss).
          gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE, baseline.source)
          if (gl.getError() !== gl.NO_ERROR) return null
          uploaded = true
        }
        // The shared canvas only grows while live sessions run; each frame draws into
        // its bottom-left corner, which the caller copies from the top-left region below.
        if (canvas.width < width || canvas.height < height) {
          canvas.width = Math.max(canvas.width, width)
          canvas.height = Math.max(canvas.height, height)
        }
        if (gl.drawingBufferWidth < width || gl.drawingBufferHeight < height) return null
        // Live frames always use the single-pass program on the default framebuffer.
        gl.bindFramebuffer(gl.FRAMEBUFFER, null)
        gl.viewport(0, 0, width, height)
        gl.useProgram(current.program)
        writeUniforms(
          gl,
          current.uniforms,
          width,
          height,
          OFF_ADVANCED_PARAMS,
          resolveMediaFxTemporalGpuParams(temporal, { pixelRatio: baseline.pixelRatio }),
          { flipY: true, time: wrapTemporalTime(timeSeconds) },
        )
        gl.drawArrays(gl.TRIANGLES, 0, 3)
        if (gl.getError() !== gl.NO_ERROR || gl.isContextLost()) return null
        return { canvas, x: 0, y: canvas.height - height, width, height }
      } catch (error) {
        warnTemporalOnce(error)
        return null
      }
    },
    dispose() {
      if (disposed) return
      disposed = true
      baseline = null
      releaseTexture()
      liveSessions.delete(token)
      shrinkSharedCanvas()
    },
  }
}
