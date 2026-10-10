import { domToCanvas } from 'modern-screenshot'
import type { TheaterCaptureResult, TheaterRendererCommand } from '../shared/theater-renderer-protocol'
import type { CameraState } from '../shared/stage-types'

const editorSelector = '.theater-selection-quick-bar,.theater-surface-test-exit,.theater-appearance-preview-layer,[data-theater-editor],.theater-effect-selection-label,.theater-effect-resize-handle,.theater-dialogue-controls,.theater-character-overlay__handle,.theater-character-overlay__resize'
const visible = (el: Element) => {
  const style = getComputedStyle(el)
  const rect = el.getBoundingClientRect()
  return style.display !== 'none' && style.visibility !== 'hidden' && rect.width > 0 && rect.height > 0
}

// Readiness is event/frame based and bounded; no fixed delay stands in for media loading.
export const waitForCaptureReady = async (root: HTMLElement, ready: () => boolean, signal: AbortSignal) => {
  const deadline = performance.now() + 5000
  await bounded(document.fonts.ready, signal, deadline)
  const images = [...root.querySelectorAll('img')].filter(visible)
  await bounded(Promise.all(images.map(image => image.decode().catch(() => undefined))), signal, deadline)
  await new Promise<void>((resolve, reject) => {
    let frame = 0
    const cleanup = () => { cancelAnimationFrame(frame); signal.removeEventListener('abort', abort); clearTimeout(timer) }
    const abort = () => { cleanup(); reject(new Error('capture_cancelled')) }
    const timer = setTimeout(() => { cleanup(); reject(new Error('capture_readiness_timeout')) }, Math.max(1, deadline - performance.now()))
    const check = () => {
      if (signal.aborted) return abort()
      if (ready() && [...root.querySelectorAll('video')].filter(visible).every(video => video.readyState >= 2)) {
        cleanup(); resolve()
      } else frame = requestAnimationFrame(check)
    }
    signal.addEventListener('abort', abort, { once: true })
    frame = requestAnimationFrame(check)
  })
}
const bounded = <T>(promise: Promise<T>, signal: AbortSignal, deadline: number) => new Promise<T>((resolve, reject) => {
  const cleanup = () => { clearTimeout(timer); signal.removeEventListener('abort', abort) }
  const abort = () => { cleanup(); reject(new Error('capture_cancelled')) }
  const timer = setTimeout(() => { cleanup(); reject(new Error('capture_timeout')) }, Math.max(1, deadline - performance.now()))
  signal.addEventListener('abort', abort, { once: true })
  if (signal.aborted) return abort()
  promise.then(value => { cleanup(); resolve(value) }, error => { cleanup(); reject(error) })
})

export const requestTheaterDisplayCapture = async (): Promise<MediaStream> => {
  if (!navigator.mediaDevices?.getDisplayMedia) throw new Error('当前浏览器不支持完整剧场截图')
  const constraints = {
    video: { displaySurface: 'browser' },
    audio: false,
    preferCurrentTab: true,
    selfBrowserSurface: 'include',
    surfaceSwitching: 'exclude',
    monitorTypeSurfaces: 'exclude',
  }
  const stream = await navigator.mediaDevices.getDisplayMedia(constraints as unknown as DisplayMediaStreamOptions)
  const track = stream.getVideoTracks()[0]
  if (!track) {
    stream.getTracks().forEach(item => item.stop())
    throw new Error('未获得浏览器画面')
  }
  const displaySurface = (track.getSettings() as MediaTrackSettings & { displaySurface?: string }).displaySurface
  if (displaySurface && displaySurface !== 'browser') {
    stream.getTracks().forEach(item => item.stop())
    throw new Error('完整截图需要共享当前浏览器标签页')
  }
  return stream
}

const dataUrlBytes = (url: string) => Math.floor((url.length - url.indexOf(',') - 1) * 3 / 4)
const encodeCaptureCanvas = (canvas: HTMLCanvasElement, maxBytes: number) => {
  let dataUrl = canvas.toDataURL('image/png')
  if (dataUrlBytes(dataUrl) > maxBytes) {
    for (const quality of [0.85, 0.65, 0.45, 0.25]) {
      dataUrl = canvas.toDataURL('image/jpeg', quality)
      if (dataUrlBytes(dataUrl) <= maxBytes) break
    }
  }
  if (dataUrlBytes(dataUrl) > maxBytes) throw new Error('capture_size_limit')
  return dataUrl
}

const waitForCapturedVideoFrame = async (video: HTMLVideoElement, signal: AbortSignal) => {
  const deadline = performance.now() + 3000
  if (!video.videoWidth || !video.videoHeight || video.readyState < 2) {
    await bounded(new Promise<void>((resolve, reject) => {
      const done = () => { cleanup(); resolve() }
      const failed = () => { cleanup(); reject(new Error('capture_stream_unavailable')) }
      const cleanup = () => {
        video.removeEventListener('loadeddata', done)
        video.removeEventListener('error', failed)
      }
      video.addEventListener('loadeddata', done, { once: true })
      video.addEventListener('error', failed, { once: true })
    }), signal, deadline)
  }
  await bounded(video.play(), signal, deadline)
  const requestVideoFrame = (video as HTMLVideoElement & {
    requestVideoFrameCallback?: (callback: () => void) => number
  }).requestVideoFrameCallback
  if (requestVideoFrame) {
    await bounded(new Promise<void>(resolve => requestVideoFrame.call(video, () => resolve())), signal, deadline)
  } else {
    await bounded(new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))), signal, deadline)
  }
}

const captureBrowserComposite = async (
  options: CaptureOptions,
  bounds: DOMRect,
  crop: { x: number, y: number, width: number, height: number },
  payload: { maxEdge: number, maxBytes: number },
): Promise<TheaterCaptureResult> => {
  const stream = options.displayStream
  const track = stream?.getVideoTracks()[0]
  if (!stream || !track || track.readyState !== 'live') throw new Error('capture_stream_unavailable')
  const displaySurface = (track.getSettings() as MediaTrackSettings & { displaySurface?: string }).displaySurface
  if (displaySurface && displaySurface !== 'browser') throw new Error('capture_surface_mismatch')

  const restore: (() => void)[] = []
  for (const element of options.root.querySelectorAll<HTMLElement>(editorSelector)) {
    if (!visible(element)) continue
    const old = element.style.visibility
    element.style.setProperty('visibility', 'hidden', 'important')
    restore.push(() => { element.style.visibility = old })
  }

  const video = document.createElement('video')
  video.muted = true
  video.playsInline = true
  video.srcObject = stream
  try {
    await waitForCapturedVideoFrame(video, options.signal)
    const viewportWidth = Math.max(1, window.innerWidth)
    const viewportHeight = Math.max(1, window.innerHeight)
    const scaleX = video.videoWidth / viewportWidth
    const scaleY = video.videoHeight / viewportHeight
    if (!Number.isFinite(scaleX) || !Number.isFinite(scaleY) || scaleX <= 0 || scaleY <= 0
      || Math.abs(scaleX / scaleY - 1) > 0.08) throw new Error('capture_surface_mismatch')

    const sourceX = (bounds.left + crop.x) * scaleX
    const sourceY = (bounds.top + crop.y) * scaleY
    const sourceWidth = crop.width * scaleX
    const sourceHeight = crop.height * scaleY
    if (sourceX < -1 || sourceY < -1 || sourceX + sourceWidth > video.videoWidth + 1
      || sourceY + sourceHeight > video.videoHeight + 1) throw new Error('capture_surface_mismatch')

    const outputScale = Math.min(1, payload.maxEdge / Math.max(sourceWidth, sourceHeight))
    const output = document.createElement('canvas')
    output.width = Math.max(1, Math.round(sourceWidth * outputScale))
    output.height = Math.max(1, Math.round(sourceHeight * outputScale))
    const context = output.getContext('2d')
    if (!context) throw new Error('capture_canvas_unavailable')
    context.drawImage(video, sourceX, sourceY, sourceWidth, sourceHeight, 0, 0, output.width, output.height)
    const dataUrl = encodeCaptureCanvas(output, payload.maxBytes)
    const scale = output.width / crop.width
    return {
      captureId: options.command.requestId,
      rendererId: options.command.rendererId,
      sceneId: options.command.sceneId,
      revision: options.revision,
      status: 'complete',
      width: output.width,
      height: output.height,
      mimeType: dataUrl.slice(5, dataUrl.indexOf(';')),
      coordinateMapping: {
        worldUnitPx: 24,
        anchor: 'center',
        camera: { ...options.camera },
        crop,
        worldOriginPx: {
          x: (bounds.width / 2 + options.camera.x - crop.x) * scale,
          y: (bounds.height / 2 + options.camera.y - crop.y) * scale,
        },
        pixelsPerWorldUnit: 24 * options.camera.zoom * scale,
        effectDesignSize: { width: 1920, height: 1080 },
      },
      includedLayers: ['browser-composite'],
      missingLayers: [],
      unsupportedObjects: [],
      warnings: [],
      data: dataUrl.slice(dataUrl.indexOf(',') + 1),
    }
  } finally {
    video.pause()
    video.srcObject = null
    restore.reverse().forEach(fn => fn())
  }
}

export interface CaptureOptions {
  command: TheaterRendererCommand
  root: HTMLElement
  camera: CameraState
  revision: number
  crop?: { x: number, y: number, width: number, height: number }
  signal: AbortSignal
  snapshotCanvases: () => Map<HTMLCanvasElement, string | null>
  displayStream?: MediaStream | null
}

export const captureTheaterViewport = async (options: CaptureOptions): Promise<TheaterCaptureResult> => {
  const { root, command, signal } = options
  const payload = command.payload as { maxEdge: number, maxBytes: number }
  const bounds = root.getBoundingClientRect()
  const crop = options.crop || { x: 0, y: 0, width: bounds.width, height: bounds.height }
  if (crop.width <= 0 || crop.height <= 0) throw new Error('capture_object_outside_viewport')
  let compositeWarning = ''
  if (options.displayStream) {
    try {
      return await captureBrowserComposite(options, bounds, crop, payload)
    } catch (error) {
      if (signal.aborted) throw error
      compositeWarning = `浏览器合成截图不可用，已回退 DOM 合成：${error instanceof Error ? error.message : 'capture_failed'}`
    }
  }
  const missing = new Set<string>()
  const unsupported = new Set<string>()
  const warnings: string[] = compositeWarning ? [compositeWarning] : []
  const restore: (() => void)[] = []
  const snapshots = options.snapshotCanvases()
  type CaptureSlot = { kind: 'canvas', source: string | null } | { kind: 'frame', style: Partial<CSSStyleDeclaration> }
  const slots = new Map<string, CaptureSlot>()
  const includeNode = (node: Node) => !(node instanceof Element && (node.matches(editorSelector) || node.tagName === 'IFRAME' || node.tagName === 'VIDEO'))
  let index = 0
  // The screenshot library turns canvas into img and filters iframe entirely.
  // Paired, non-rendered anchors preserve their exact slots in the DOM clone,
  // regardless of filtered siblings and how many canvases share a parent.
  const markSlot = (element: HTMLElement, slot: CaptureSlot) => {
    const parent = element.parentElement
    if (!parent) return null
    const key = String(index++)
    const start = document.createElement('span')
    const end = document.createElement('span')
    start.setAttribute('data-mcp-capture-start', key)
    end.setAttribute('data-mcp-capture-end', key)
    start.style.setProperty('display', 'none', 'important')
    end.style.setProperty('display', 'none', 'important')
    start.setAttribute('aria-hidden', 'true')
    end.setAttribute('aria-hidden', 'true')
    parent.insertBefore(start, element)
    parent.insertBefore(end, element.nextSibling)
    restore.push(() => { start.remove(); end.remove() })
    slots.set(key, slot)
    return key
  }
  try {
    for (const canvas of root.querySelectorAll('canvas')) {
      if (!visible(canvas)) continue
      let source = snapshots.get(canvas)
      if (source === undefined) {
        try { source = canvas.toDataURL('image/png') } catch { source = null }
      }
      const key = markSlot(canvas, { kind: 'canvas', source })
      if (key === null || !source) missing.add(`canvas:${key ?? index}`)
    }
    // Opaque iForm sandbox and cross-origin frames are deliberately not read.
    // Placeholders stay inside their original stacking context, preserving occlusion.
    for (const iframe of root.querySelectorAll('iframe')) {
      if (!visible(iframe)) continue
      const id = iframe.closest('[data-stage-object-id]')?.getAttribute('data-stage-object-id')
      const surface = iframe.closest('[data-stage-surface]')?.getAttribute('data-stage-surface')
      unsupported.add(id || (surface ? `surface:${surface}` : `iframe:${index}`))
      missing.add(surface ? `surfaceEmbeds.${surface}` : `iframe:${id || index}`)
      const style = getComputedStyle(iframe)
      markSlot(iframe, { kind: 'frame', style: {
        width: style.width, height: style.height, transform: style.transform, transformOrigin: style.transformOrigin,
        position: style.position, left: style.left, top: style.top, right: style.right, bottom: style.bottom,
        display: style.display, zIndex: style.zIndex, opacity: style.opacity, borderRadius: style.borderRadius,
      } })
    }
    for (const placeholder of root.querySelectorAll('.theater-iframe-visual-object__placeholder')) {
      if (!visible(placeholder)) continue
      const id = placeholder.closest('[data-stage-object-id]')?.getAttribute('data-stage-object-id')
      const surface = placeholder.closest('[data-stage-surface]')?.getAttribute('data-stage-surface')
      unsupported.add(id || (surface ? `surface:${surface}` : 'iframe:unavailable'))
      missing.add(surface ? `surfaceEmbeds.${surface}` : `iframe:${id || 'unavailable'}`)
    }
    if (unsupported.size) warnings.push('iframe/iForm 内容不可可靠捕获；灰色占位保留原层级，不代表实际网页。')
    for (const image of root.querySelectorAll('img')) {
      if (visible(image) && (!image.complete || image.naturalWidth === 0)) missing.add('image:unavailable')
    }
    if ([...root.querySelectorAll('video')].some(visible)) { missing.add('media:video'); warnings.push('当前协作导出不保证视频帧可捕获。') }
    if (typeof root.getAnimations === 'function' && root.getAnimations({ subtree: true }).some(animation => animation.playState === 'running')) missing.add('animation:active')
    for (const element of root.querySelectorAll('*')) {
      if (!visible(element) || element.matches(editorSelector)) continue
      const style = getComputedStyle(element)
      if (style.backdropFilter && style.backdropFilter !== 'none') missing.add('css:backdrop-filter')
      if (style.animationName !== 'none') missing.add('css:animation-frame')
    }
    const canvas = await bounded(domToCanvas(root, {
      width: bounds.width, height: bounds.height,
      scale: Math.min(1, payload.maxEdge / Math.max(bounds.width, bounds.height)),
      timeout: 6000,
      filter: includeNode,
      fetch: { placeholderImage: () => { missing.add('resource:unavailable'); return 'data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=' } },
      onCloneEachNode: node => {
        if (!(node instanceof Element)) return
        for (const start of [...node.children]) {
          const key = start.getAttribute('data-mcp-capture-start')
          if (key === null) continue
          const slot = slots.get(key)
          const end = [...node.children].find(child => child.getAttribute('data-mcp-capture-end') === key)
          if (!slot || !end) { missing.add(`capture-slot:${key}`); continue }
          const children: Element[] = []
          for (let child = start.nextSibling; child && child !== end; child = child.nextSibling) {
            if (child instanceof Element) children.push(child)
          }
          if (slot.kind === 'canvas') {
            const child = children.length === 1 ? children[0] : null
            if (!(child instanceof HTMLImageElement || child instanceof HTMLCanvasElement)) {
              missing.add(`canvas:${key}`)
            } else {
              const image = document.createElement('img')
              image.style.cssText = (child as HTMLElement).style.cssText
              image.src = slot.source || 'data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs='
              if (!slot.source) image.style.background = '#555'
              child.replaceWith(image)
            }
          } else {
            const placeholder = document.createElement('div')
            Object.assign(placeholder.style, slot.style, { background: '#555', pointerEvents: 'none' })
            node.insertBefore(placeholder, end)
            if (children.length) missing.add(`iframe:clone-unexpected:${key}`)
          }
          start.remove()
          end.remove()
          slots.delete(key)
        }
      },
      onCloneNode: () => { for (const [key, slot] of slots) if (slot.kind === 'canvas') missing.add(`canvas:${key}`) },
    }), signal, performance.now() + 7000)
    if (signal.aborted) throw new Error('capture_cancelled')
    const scaleX = canvas.width / bounds.width
    const scaleY = canvas.height / bounds.height
    const output = document.createElement('canvas')
    const outputScale = Math.min(1, payload.maxEdge / Math.max(crop.width * scaleX, crop.height * scaleY))
    output.width = Math.max(1, Math.round(crop.width * scaleX * outputScale))
    output.height = Math.max(1, Math.round(crop.height * scaleY * outputScale))
    const context = output.getContext('2d')
    if (!context) throw new Error('capture_canvas_unavailable')
    context.drawImage(canvas, crop.x * scaleX, crop.y * scaleY, crop.width * scaleX, crop.height * scaleY, 0, 0, output.width, output.height)
    const dataUrl = encodeCaptureCanvas(output, payload.maxBytes)
    const scale = output.width / crop.width
    return {
      captureId: command.requestId, rendererId: command.rendererId, sceneId: command.sceneId,
      revision: options.revision, status: missing.size || unsupported.size ? 'partial' : 'complete',
      width: output.width, height: output.height, mimeType: dataUrl.slice(5, dataUrl.indexOf(';')),
      coordinateMapping: { worldUnitPx: 24, anchor: 'center', camera: { ...options.camera }, crop,
        worldOriginPx: { x: (bounds.width / 2 + options.camera.x - crop.x) * scale, y: (bounds.height / 2 + options.camera.y - crop.y) * scale },
        pixelsPerWorldUnit: 24 * options.camera.zoom * scale, effectDesignSize: { width: 1920, height: 1080 } },
      includedLayers: ['stage-dom', ...(snapshots.size ? ['konva'] : [])],
      missingLayers: [...missing], unsupportedObjects: [...unsupported], warnings, data: dataUrl.slice(dataUrl.indexOf(',') + 1),
    }
  } finally { restore.reverse().forEach(fn => fn()) }
}
