import { computed, onBeforeUnmount, ref, shallowRef } from 'vue'
import { useMessage } from 'naive-ui'
import { nanoid } from 'nanoid'
import { api } from '@/stores/_config'
import { chatEvent } from '@/stores/chat'
import type { AudioAsset } from '@/types/audio'
import type { TheaterEffectPanelAdapter } from '@/views/theater/effects/theater-effect-panel-adapter'
import { setTheaterEffectConfig, theaterEffectConfigFromObject } from '@/views/theater/effects/theater-effect-types'
import { isSafeStageImageUrl } from '@/views/theater/shared/stage-types'
import { cloneStageData } from '@/views/theater/stage/stage-editing'
import { compareStageLayersBottomToTop } from '@/views/theater/stage/stage-layer-order'
import { readWorldPresentation, saveWorldPresentation } from './world-presentation-api'
import { createWorldEffect, normalizeWorldPresentationConfig, WORLD_PRESENTATION_MAX_EFFECTS, type WorldPresentationConfig } from './world-presentation-config'
import { useWorldPresentationPlayback } from './useWorldPresentationPlayback'
import { deleteWorldPresentationMedia, uploadWorldPresentationMedia } from './world-presentation-media'

/**
 * The one world presentation editing core: load, draft, revision, save, the effect
 * panel adapter, world audio assets and local preview. Hosts (world settings, chat
 * console) only lay it out. The owning component is keyed by world, so worldId is fixed.
 */
export const useWorldPresentationEditor = (worldId: string) => {
  const lifetime = new AbortController()
  const message = useMessage()
  const selectedId = ref<string | null>(null)
  const draft = shallowRef(normalizeWorldPresentationConfig(null))
  const revision = ref('')
  const dirty = ref(false)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref('')
  // Another admin saved a newer revision; saving this draft would be rejected.
  const stale = ref(false)
  const audioAssets = shallowRef<AudioAsset[]>([])
  const audioLoading = ref(false)
  const audioError = ref('')
  const mediaUploading = ref(false)
  const preview = useWorldPresentationPlayback(() => draft.value)
  const ready = computed(() => Boolean(revision.value) && !loading.value)
  const canEdit = computed(() => ready.value && !saving.value && !mediaUploading.value)
  let pendingRemoteRevision = ''
  const pendingMediaResourceIds = new Set<string>()

  const referencedMediaResourceIds = (config: WorldPresentationConfig) => new Set(config.effects.flatMap((object) => {
    const effect = theaterEffectConfigFromObject(object)
    return [effect.media?.resourceId, object.image?.resourceId].filter((id): id is string => Boolean(id?.trim()))
  }))

  const cleanupMediaResources = (resourceIds: Iterable<string>) => {
    for (const resourceId of resourceIds) {
      pendingMediaResourceIds.delete(resourceId)
      void deleteWorldPresentationMedia(worldId, resourceId).catch(() => undefined)
    }
  }

  const load = async () => {
    if (loading.value || saving.value || mediaUploading.value) return
    loading.value = true
    error.value = ''
    try {
      const result = await readWorldPresentation(worldId, lifetime.signal)
      if (lifetime.signal.aborted) return
      cleanupMediaResources([...pendingMediaResourceIds])
      draft.value = result.config
      revision.value = result.revision
      if (!result.config.effects.some((effect) => effect.id === selectedId.value)) {
        selectedId.value = result.config.effects[0]?.id ?? null
      }
      dirty.value = false
      stale.value = false
    } catch {
      if (!lifetime.signal.aborted) error.value = '读取演出配置失败'
    } finally {
      loading.value = false
      const remote = pendingRemoteRevision
      pendingRemoteRevision = ''
      if (remote && !lifetime.signal.aborted) receiveRemoteRevision(remote)
    }
  }

  const loadAudio = async () => {
    audioLoading.value = true
    audioError.value = ''
    try {
      const { data } = await api.get<{ items: AudioAsset[] }>('api/v1/audio/assets', {
        params: { scope: 'world', worldId, pageSize: 500 },
        signal: lifetime.signal,
      })
      if (!lifetime.signal.aborted) audioAssets.value = data.items ?? []
    } catch {
      if (!lifetime.signal.aborted) audioError.value = '无法读取音效素材'
    } finally {
      audioLoading.value = false
    }
  }

  const receiveRemoteRevision = (next: string) => {
    if (!next || next === revision.value) return
    if (loading.value || saving.value || mediaUploading.value) {
      pendingRemoteRevision = next
      return
    }
    if (dirty.value) stale.value = true
    else void load()
  }

  const save = async () => {
    if (!dirty.value || saving.value || mediaUploading.value || !revision.value || stale.value) return false
    saving.value = true
    error.value = ''
    pendingRemoteRevision = ''
    try {
      const result = await saveWorldPresentation(worldId, normalizeWorldPresentationConfig(draft.value), revision.value, lifetime.signal)
      if (lifetime.signal.aborted) return false
      draft.value = result.config
      revision.value = result.revision
      dirty.value = false
      const referenced = referencedMediaResourceIds(result.config)
      const abandoned = [...pendingMediaResourceIds].filter((resourceId) => !referenced.has(resourceId))
      for (const resourceId of referenced) pendingMediaResourceIds.delete(resourceId)
      cleanupMediaResources(abandoned)
      message.success('演出设置已保存')
      return true
    } catch (cause) {
      if (lifetime.signal.aborted) return false
      const response = cause as { response?: { status?: number, data?: { message?: string } } }
      if (response.response?.status === 409) stale.value = true
      else error.value = response.response?.data?.message || '保存演出配置失败'
      return false
    } finally {
      saving.value = false
      const remote = pendingRemoteRevision
      pendingRemoteRevision = ''
      if (remote && !lifetime.signal.aborted) receiveRemoteRevision(remote)
    }
  }

  const update = (patch: Partial<WorldPresentationConfig>) => {
    if (!revision.value || saving.value || lifetime.signal.aborted) return
    draft.value = { ...draft.value, ...patch }
    dirty.value = true
  }

  const adapter: TheaterEffectPanelAdapter = {
    add(kind) {
      if (draft.value.effects.length >= WORLD_PRESENTATION_MAX_EFFECTS) return
      const effects = [...draft.value.effects]
      const order = effects.reduce((largest, effect) => Math.max(largest, effect.transform.order), 0) + 1
      const object = createWorldEffect(nanoid(), kind, order)
      update({ effects: [...effects, object] })
      selectedId.value = object.id
    },
    edit(_, ids, mutate) {
      update({ effects: draft.value.effects.map((effect) => {
        if (!ids.includes(effect.id)) return effect
        const edited = cloneStageData(effect)
        mutate(edited)
        return edited
      }) })
    },
    select(id) { selectedId.value = id },
    remove(ids) {
      const effects = draft.value.effects.filter((effect) => !ids.includes(effect.id))
      update({ effects })
      if (selectedId.value && ids.includes(selectedId.value)) selectedId.value = effects[0]?.id ?? null
    },
    preview: preview.previewEffect,
    move(id, direction) {
      const effects = cloneStageData(draft.value.effects).sort(compareStageLayersBottomToTop)
      const index = effects.findIndex((effect) => effect.id === id)
      const nextIndex = index + direction
      if (index < 0 || nextIndex < 0 || nextIndex >= effects.length) return
      const [object] = effects.splice(index, 1)
      effects.splice(nextIndex, 0, object)
      effects.forEach((effect, order) => { effect.transform.order = order; effect.transform.z = 0 })
      update({ effects })
    },
  }

  const setMedia = (id: string, value: string, asVideo: boolean) => {
    const url = value.trim()
    if (url && !isSafeStageImageUrl(url)) {
      message.error('请输入有效的图片或视频地址')
      return
    }
    adapter.edit('修改特效媒体', [id], (object) => {
      const effect = theaterEffectConfigFromObject(object)
      const isVideo = asVideo || (effect.media?.url !== url && /\.(mp4|webm|mov)(?:[?#]|$)/i.test(url))
      effect.media = url ? {
        resourceId: effect.media?.url === url ? effect.media.resourceId : nanoid(),
        url,
        ...(isVideo ? { mimeType: 'video/mp4' } : {}),
      } : null
      delete object.image
      setTheaterEffectConfig(object, effect)
    })
  }

  const uploadMedia = async (id: string, file: File) => {
    if (!ready.value || saving.value || mediaUploading.value || lifetime.signal.aborted) return
    mediaUploading.value = true
    try {
      const media = await uploadWorldPresentationMedia(worldId, file)
      if (lifetime.signal.aborted) {
        void deleteWorldPresentationMedia(worldId, media.resourceId).catch(() => undefined)
        return
      }
      pendingMediaResourceIds.add(media.resourceId)
      adapter.edit('上传特效媒体', [id], (object) => {
        const effect = theaterEffectConfigFromObject(object)
        effect.media = media
        delete object.image
        setTheaterEffectConfig(object, effect)
      })
    } catch (cause) {
      if (!lifetime.signal.aborted) message.error(cause instanceof Error ? cause.message : '媒体上传失败')
    } finally {
      mediaUploading.value = false
      const remote = pendingRemoteRevision
      pendingRemoteRevision = ''
      if (remote && !lifetime.signal.aborted) receiveRemoteRevision(remote)
    }
  }

  const testSequence = (id: string) => { void preview.play('sequence', id, nanoid()) }

  const onConfigChanged = (event: { argv?: { Options?: { worldId?: string, revision?: string } } }) => {
    const change = event.argv?.Options
    if (change?.worldId === worldId && change.revision) receiveRemoteRevision(change.revision)
  }
  chatEvent.on('world-presentation-updated', onConfigChanged)

  onBeforeUnmount(() => {
    lifetime.abort()
    chatEvent.off('world-presentation-updated', onConfigChanged)
    cleanupMediaResources([...pendingMediaResourceIds])
  })

  void load()
  void loadAudio()

  return {
    draft,
    revision,
    selectedId,
    dirty,
    loading,
    saving,
    error,
    stale,
    ready,
    canEdit,
    audioAssets,
    audioLoading,
    audioError,
    mediaUploading,
    adapter,
    previewPlaybacks: preview.playbacks,
    stopPreview: preview.stop,
    load,
    save,
    update,
    setMedia,
    uploadMedia,
    testSequence,
  }
}

export type WorldPresentationEditor = ReturnType<typeof useWorldPresentationEditor>
