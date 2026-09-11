import type { Bounds, Editor } from '@quickdrawjs/core'

export interface SealChatCamera {
  /** CSS-pixel translation inside the board viewport. */
  offsetX: number
  offsetY: number
  zoom: number
}

export interface CameraPoint {
  x: number
  y: number
}

export interface FitBoundsOptions {
  /** Insets in the board's CSS pixels. */
  margin?: number
  maxZoom?: number
  animate?: number
}

export const QUICKDRAW_MIN_ZOOM = 0.05
export const QUICKDRAW_MAX_ZOOM = 8

const finiteOr = (value: number, fallback: number) => (
  Number.isFinite(value) ? value : fallback
)

const clampZoom = (value: number) => Math.min(
  QUICKDRAW_MAX_ZOOM,
  Math.max(QUICKDRAW_MIN_ZOOM, finiteOr(value, 1)),
)

/**
 * The only place where Quickdraw's page/camera coordinates are translated to
 * SealChat coordinates. Quickdraw renders with
 * `(world + cameraOffset) * zoom`; its public camera x/y are world units, so
 * the exposed offset is the equivalent CSS-pixel translation (`x * zoom`).
 */
export class QuickdrawCameraAdapter {
  private readonly editor: Editor
  private readonly container: HTMLElement

  constructor(editor: Editor, container: HTMLElement) {
    this.editor = editor
    this.container = container
  }

  readCamera(): SealChatCamera {
    const camera = this.editor.camera
    const zoom = clampZoom(camera.z)
    return {
      offsetX: finiteOr(camera.x, 0) * zoom,
      offsetY: finiteOr(camera.y, 0) * zoom,
      zoom,
    }
  }

  subscribeCamera(listener: (camera: SealChatCamera) => void): () => void {
    const emit = () => listener(this.readCamera())
    const unsubscribe = this.editor.on('camera', emit)
    emit()
    return unsubscribe
  }

  /** Convert a local screen point (CSS pixels) into Quickdraw world/page space. */
  screenToWorld(point: CameraPoint): CameraPoint {
    return this.editor.screenToPage(point.x, point.y)
  }

  /** Convert a Quickdraw world/page point into local screen CSS pixels. */
  worldToScreen(point: CameraPoint): CameraPoint {
    return this.editor.pageToScreen(point.x, point.y)
  }

  /** Convert client coordinates, including a CSS-scaled host, to local CSS px. */
  clientToLocal(clientX: number, clientY: number): CameraPoint {
    const rect = this.container.getBoundingClientRect()
    const layoutWidth = this.container.clientWidth || rect.width || 1
    const layoutHeight = this.container.clientHeight || rect.height || 1
    const scaleX = rect.width > 0 ? rect.width / layoutWidth : 1
    const scaleY = rect.height > 0 ? rect.height / layoutHeight : 1
    return {
      x: (clientX - rect.left) / (scaleX || 1),
      y: (clientY - rect.top) / (scaleY || 1),
    }
  }

  clientToWorld(clientX: number, clientY: number): CameraPoint {
    return this.screenToWorld(this.clientToLocal(clientX, clientY))
  }

  setCamera(camera: SealChatCamera, options?: { animate?: number }): void {
    const zoom = clampZoom(camera.zoom)
    const offsetX = finiteOr(camera.offsetX, 0)
    const offsetY = finiteOr(camera.offsetY, 0)
    this.editor.setCamera({
      x: offsetX / zoom,
      y: offsetY / zoom,
      z: zoom,
    }, options)
  }

  zoomAt(point: CameraPoint, multiplier: number): void {
    const factor = finiteOr(multiplier, 1)
    if (factor <= 0) return
    this.editor.zoomAt(point.x, point.y, factor)
  }

  /** Apply a two-pointer pinch while keeping the world anchor under its center. */
  pinch(
    startCamera: SealChatCamera,
    anchorWorld: CameraPoint,
    center: CameraPoint,
    scale: number,
  ): void {
    const zoom = clampZoom(startCamera.zoom * finiteOr(scale, 1))
    this.setCamera({
      zoom,
      offsetX: center.x - anchorWorld.x * zoom,
      offsetY: center.y - anchorWorld.y * zoom,
    })
  }

  /** Read Quickdraw's public content bounds; null means the board is empty. */
  contentBounds(): Bounds | null {
    return this.editor.contentBounds()
  }

  /**
   * Contain-fit an explicit world rect. This deliberately does not call
   * Quickdraw's `followBounds`, whose public contract is cover-fit and may crop.
   */
  fitBounds(bounds: Bounds | null | undefined, options: FitBoundsOptions = {}): boolean {
    if (!bounds || !(bounds.w > 0) || !(bounds.h > 0)) return false
    const { w, h } = this.editor.viewSize()
    if (!(w > 1) || !(h > 1)) return false

    const requestedMargin = finiteOr(options.margin ?? 48, 48)
    const margin = Math.max(0, Math.min(requestedMargin, Math.min(w, h) / 2 - 1))
    const availableWidth = w - margin * 2
    const availableHeight = h - margin * 2
    const requestedMaxZoom = finiteOr(options.maxZoom ?? QUICKDRAW_MAX_ZOOM, QUICKDRAW_MAX_ZOOM)
    const maxZoom = Math.min(QUICKDRAW_MAX_ZOOM, Math.max(QUICKDRAW_MIN_ZOOM, requestedMaxZoom))
    const zoom = Math.min(maxZoom, availableWidth / bounds.w, availableHeight / bounds.h)
    // Refuse a fit that would require going below Quickdraw's public zoom
    // floor; claiming success there would crop the caller's bounds.
    if (!(zoom >= QUICKDRAW_MIN_ZOOM) || !Number.isFinite(zoom)) return false

    const z = Math.min(QUICKDRAW_MAX_ZOOM, zoom)
    this.editor.setCamera({
      z,
      x: w / 2 / z - (bounds.x + bounds.w / 2),
      y: h / 2 / z - (bounds.y + bounds.h / 2),
    }, { animate: options.animate })
    return true
  }

  fitContent(options: FitBoundsOptions = {}): boolean {
    return this.fitBounds(this.contentBounds(), options)
  }

  resize(): void {
    this.editor.resize()
  }
}
