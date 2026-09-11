import {
  Editor,
  Store,
  type Diff,
  type DiffSource,
  type Snapshot,
} from '@quickdrawjs/core'
import {
  QuickdrawCameraAdapter,
  type FitBoundsOptions,
} from './quickdraw-camera-adapter'

export interface QuickdrawAdapterOptions {
  theme?: 'light' | 'dark'
  grid?: 'none' | 'lines' | 'dots'
}

/** The version accepted by the Stage 2 Board document contract. */
export const QUICKDRAW_ENGINE_VERSION = '@quickdrawjs/core@0.2.0' as const

/** Owns Quickdraw's imperative objects for one mounted canvas surface. */
export class QuickdrawAdapter {
  readonly editor: Editor
  readonly store: Store
  readonly camera: QuickdrawCameraAdapter

  constructor(container: HTMLElement, options: QuickdrawAdapterOptions = {}) {
    this.store = new Store()
    this.editor = new Editor({
      container,
      store: this.store,
      theme: options.theme || 'light',
      grid: options.grid || 'dots',
    })
    this.camera = new QuickdrawCameraAdapter(this.editor, container)
  }

  listenChanges(listener: (diff: Diff, source: DiffSource) => void, source: DiffSource | 'all' = 'all'): () => void {
    return this.store.listen(listener, { source })
  }

  listenHistory(listener: () => void): () => void {
    return this.store.listenHistory(listener)
  }

  /** Public Store history stack lengths; no private transaction state is read. */
  historyDepth(): { undo: number; redo: number } {
    return { undo: this.store.undos.length, redo: this.store.redos.length }
  }

  getSnapshot(): Snapshot {
    return this.store.getSnapshot()
  }

  /** Load is explicitly remote so it never becomes a user undo entry. */
  loadSnapshot(snapshot: Snapshot): void {
    this.store.loadSnapshot(snapshot, 'remote')
  }

  fitContent(options?: FitBoundsOptions): boolean {
    return this.camera.fitContent(options)
  }

  destroy(): void {
    this.editor.destroy()
  }
}
