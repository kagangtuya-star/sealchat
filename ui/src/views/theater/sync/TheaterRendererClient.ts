import { chatEvent } from '@/stores/chat'
import { watch, type WatchStopHandle } from 'vue'
import { createDefaultTheaterEffectConfig, theaterBuiltinEffectThemes } from '../effects/theater-effect-types'
import { listSceneOverlayEffects } from '../overlays/scene-overlay-registry'
import { theaterRendererCommandSchema, sameRendererScope, type TheaterRendererCommand, type TheaterRendererScope, type TheaterRendererView, type TheaterExecutionPlan, type TheaterCaptureResult } from '../shared/theater-renderer-protocol'
import { runStageActionSequence, STAGE_ACTION_CANCELLED, waitAbortable } from '../stage/theater-action-sequence-runtime'
import type { TheaterSyncClient } from './TheaterSyncClient'
import { chatComposerInsertPayloadSchema, type ChatComposerInsertPayload } from '../bridge/theater-bridge-protocol'

interface RendererStage {
  getRendererView(): TheaterRendererView
  applyRendererView(command: TheaterRendererCommand): Promise<TheaterRendererView>
  captureRenderer(command: TheaterRendererCommand, signal: AbortSignal, revision: number): Promise<TheaterCaptureResult>
  playEffect(effectId: string, triggerId?: string): boolean
}
interface RendererOptions {
  scope: TheaterRendererScope
  userId: string
  sync: TheaterSyncClient
  getStage: () => RendererStage | null
  send: (api: string, data: Record<string, unknown>) => Promise<unknown>
  insert: (payload: ChatComposerInsertPayload) => Promise<unknown>
  confirm: () => Promise<boolean>
  onInvalidated: () => void
}
const record = (input: unknown): Record<string, unknown> => input !== null && typeof input === 'object' && !Array.isArray(input) ? input as Record<string, unknown> : {}

// Disposable collaboration transport, not a second stage-state owner.
export class TheaterRendererClient {
  readonly rendererId = `renderer-${crypto.randomUUID()}`
  private active = false
  private heartbeat: ReturnType<typeof setInterval> | null = null
  private registering = false
  private stopWatch: WatchStopHandle | null = null
  private readonly seen = new Set<string>()
  private readonly controllers = new Map<string, AbortController>()
  constructor(private readonly options: RendererOptions) {}

  async start() {
    this.active = true
    chatEvent.on('theater.renderer.command' as never, this.onCommand as never)
    chatEvent.on('connection.changed' as never, this.onConnection as never)
    try { await this.register() } catch (error) { this.stop(); throw error }
    if (this.active) {
      this.heartbeat = setInterval(() => { void this.register().catch(() => this.invalidate()) }, 15000)
      this.stopWatch = watch(() => this.options.sync.getRendererState().sceneId, () => { void this.register().catch(() => this.invalidate()) })
    }
  }
  stop() {
    if (!this.active) return
    this.active = false
    if (this.heartbeat) clearInterval(this.heartbeat)
    this.heartbeat = null
    this.stopWatch?.(); this.stopWatch = null
    this.controllers.forEach(controller => controller.abort())
    this.controllers.clear()
    chatEvent.off('theater.renderer.command' as never, this.onCommand as never)
    chatEvent.off('connection.changed' as never, this.onConnection as never)
    void this.options.send('theater.renderer.unregister', {}).catch(() => undefined)
  }
  private invalidate() { this.stop(); this.options.onInvalidated() }
  private readonly onConnection = (event: unknown) => {
    if (record(event).state !== 'connected') this.invalidate()
  }
  private async rpc(api: string, data: Record<string, unknown>) {
    const response = record(await this.options.send(api, data))
    if (response.err) throw new Error(String(response.err))
    return record(response.data)
  }
  private async register() {
    if (!this.active || this.registering) return
    const stage = this.options.getStage()
    if (!stage) throw new Error('renderer_unavailable')
    const state = this.options.sync.getRendererState()
    const viewport = stage.getRendererView()
    const registrationViewport = {
      ...viewport,
      width: Math.max(1, Math.round(viewport.width)),
      height: Math.max(1, Math.round(viewport.height)),
    }
    this.registering = true
    try {
      await this.rpc('theater.renderer.register', {
        ...this.options.scope, version: 1, rendererId: this.rendererId, userId: this.options.userId,
        activeSceneId: state.sceneId || '', revision: state.revision, viewport: registrationViewport,
        capabilities: ['view', 'capture', 'execute'],
        catalog: { overlays: listSceneOverlayEffects().map(({ buildRenderDescriptor: _build, ...definition }) => definition),
          effects: theaterBuiltinEffectThemes.map(theme => { const config = createDefaultTheaterEffectConfig(); config.builtin.theme = theme; return config }) },
      })
    } finally { this.registering = false }
  }
  private readonly onCommand = (event: unknown) => {
    const parsed = theaterRendererCommandSchema.safeParse(record(record(event).theater).payload)
    if (!parsed.success) return
    const command = parsed.data
    if (!this.active || command.rendererId !== this.rendererId || !sameRendererScope(command.scope, this.options.scope)
      || (command.scope.inputChannelId || '') !== (this.options.scope.inputChannelId || '')) return
    if (command.operation === 'cancel') { this.controllers.get(command.requestId)?.abort(); return }
    if (this.seen.has(command.requestId)) return
    if (this.seen.size >= 512) this.seen.delete(this.seen.values().next().value || '')
    this.seen.add(command.requestId)
    void this.handle(command)
  }
  private async handle(command: TheaterRendererCommand) {
    const controller = new AbortController()
    this.controllers.set(command.requestId, controller)
    const { signal } = controller
    let revision = command.expectedRevision
    let sceneId = command.sceneId
    const timer = setTimeout(() => controller.abort(), Math.max(1, command.expiresAt - Date.now()))
    const guard = async () => {
      if (!this.active || signal.aborted || Date.now() >= command.expiresAt) throw new Error('stale_command')
      await this.options.sync.ensureRendererRevision(revision, sceneId)
    }
    const reply = (status: string, result?: unknown, error?: string) => this.rpc('theater.renderer.reply', {
      version: 1, requestId: command.requestId, rendererId: this.rendererId, scope: this.options.scope,
      sceneId, revision, status, result, error,
    })
    try {
      await guard()
      const stage = this.options.getStage()
      if (!stage) throw new Error('renderer_unavailable')
      await reply('running')
      let result: unknown
      if (command.operation === 'capture') {
        result = await stage.captureRenderer(command, signal, revision)
      } else if (command.operation !== 'execute') {
        result = await stage.applyRendererView(command)
      } else {
        // Native business steps are serialized even within visual sync batches:
        // each step must consume the actual revision produced by its predecessor.
        let stepQueue = Promise.resolve()
        const execute = async (node: TheaterExecutionPlan): Promise<void | typeof STAGE_ACTION_CANCELLED> => {
          if (signal.aborted) return STAGE_ACTION_CANCELLED
          await waitAbortable(node.delayMs, signal)
          if (signal.aborted) return STAGE_ACTION_CANCELLED
          if (node.children) return runStageActionSequence(node.children, execute, signal)
          if (!node.stepId) throw new Error('renderer_invalid_plan')
          const pending = stepQueue.then(async () => {
            await guard()
            if (node.confirm && !await this.options.confirm()) { controller.abort(); throw new Error('action_cancelled') }
            await guard()
            const action = await this.rpc('theater.renderer.step', {
              version: 1, requestId: command.requestId, rendererId: this.rendererId, scope: this.options.scope,
              sceneId, revision, status: 'running', stepId: node.stepId,
            })
            const mutation = record(action.mutation)
            if (typeof mutation.revision === 'number') {
              revision = mutation.revision
              const state = await this.options.sync.acceptRendererActionRevision(revision)
              sceneId = state.sceneId || ''
            }
            await guard()
            if (action.kind === 'effect') {
              const fx = record(action.effect)
              if (typeof fx.effectId !== 'string' || typeof fx.triggerId !== 'string' || !stage.playEffect(fx.effectId, fx.triggerId)) throw new Error('renderer_effect_unavailable')
            } else if (action.kind === 'local') {
              await this.options.insert(chatComposerInsertPayloadSchema.parse(action.descriptor))
            }
          })
          stepQueue = pending
          await pending
        }
        const cancelled = await execute(command.payload as TheaterExecutionPlan)
        if (cancelled === STAGE_ACTION_CANCELLED) throw new Error('action_cancelled')
        result = { executed: true }
      }
      await guard()
      await reply('completed', result)
    } catch (error) {
      await reply(signal.aborted ? 'cancelled' : 'failed', undefined, error instanceof Error ? error.message : 'renderer_failed').catch(() => undefined)
    } finally {
      clearTimeout(timer); this.controllers.delete(command.requestId)
      if (this.active) void this.register().catch(() => this.invalidate())
    }
  }
}
