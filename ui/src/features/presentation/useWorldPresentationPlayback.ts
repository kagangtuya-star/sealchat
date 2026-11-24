import { onBeforeUnmount, shallowRef } from 'vue'
import { api } from '@/stores/_config'
import { TheaterEffectRuntime, type TheaterEffectPlayback } from '@/views/theater/effects/theater-effect-runtime'
import { rememberPresentationId } from '@/views/theater/shared/presentation-message-source'
import type { WorldPresentationConfig } from './world-presentation-config'
import { playWorldSequence, type WorldPresentationTarget } from './world-presentation-runtime'

export const useWorldPresentationPlayback = (getConfig: () => WorldPresentationConfig) => {
  const lifetime = new AbortController()
  const playbacks = shallowRef<TheaterEffectPlayback[]>([])
  const executions = new Set<string>()
  const runningSequences = new Map<string, AbortController>()
  const audio = new Map<string, { request: AbortController, player?: HTMLAudioElement }>()
  const runtime = new TheaterEffectRuntime({ getObjects: () => getConfig().effects })

  const stopAudio = (id: string) => {
    const current = audio.get(id)
    if (!current) return
    audio.delete(id)
    current.request.abort()
    if (current.player) {
      current.player.pause()
      current.player.removeAttribute('src')
      current.player.load()
    }
  }

  const startAudio = async (playback: TheaterEffectPlayback) => {
    const asset = playback.config.audio
    if (!asset?.assetId || audio.has(playback.instanceId)) return
    const current: { request: AbortController, player?: HTMLAudioElement } = { request: new AbortController() }
    audio.set(playback.instanceId, current)
    try {
      const { data } = await api.post<{ streamUrl: string }>(
        `api/v1/audio/assets/${encodeURIComponent(asset.assetId)}/play-token`, {}, { signal: current.request.signal },
      )
      if (current.request.signal.aborted || playback.expiresAt <= Date.now() || !data.streamUrl) return
      current.player = new Audio(data.streamUrl)
      current.player.volume = asset.volume
      await current.player.play()
    } catch {
      // Visuals still play when a browser blocks sound or the asset is unavailable.
    }
  }

  const unsubscribe = runtime.subscribe((active) => {
    playbacks.value = active
    const activeIds = new Set(active.map(({ instanceId }) => instanceId))
    for (const id of audio.keys()) if (!activeIds.has(id)) stopAudio(id)
    active.forEach((playback) => { void startAudio(playback) })
  })

  const play = async (kind: WorldPresentationTarget, targetId: string, executionId: string) => {
    if (lifetime.signal.aborted || !rememberPresentationId(executions, executionId, 1024)) return
    if (kind === 'effect') {
      runtime.play(targetId, executionId)
      return
    }
    const sequence = getConfig().sequences.find((item) => item.id === targetId)
    if (!sequence) return
    if (runningSequences.has(targetId)) return
    const execution = new AbortController()
    runningSequences.set(targetId, execution)
    try {
      await playWorldSequence(sequence, executionId, runtime.play, execution.signal)
    } finally {
      if (runningSequences.get(targetId) === execution) runningSequences.delete(targetId)
    }
  }

  // Local preview only: clears what this instance is showing without touching server playback.
  const stop = () => {
    runningSequences.forEach((execution) => execution.abort())
    runningSequences.clear()
    new Set(playbacks.value.map(({ effectId }) => effectId)).forEach(runtime.stop)
  }

  onBeforeUnmount(() => {
    lifetime.abort()
    unsubscribe()
    runtime.dispose()
    audio.forEach((_, id) => stopAudio(id))
    executions.clear()
    runningSequences.forEach((execution) => execution.abort())
    runningSequences.clear()
    playbacks.value = []
  })

  return { playbacks, play, stop, previewEffect: runtime.preview }
}
