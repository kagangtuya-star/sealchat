<script setup lang="ts">
import { computed, ref } from 'vue'
import { NAlert, NButton, NEmpty, NIcon, NSpin, NTab, NTabs, useDialog, useThemeVars } from 'naive-ui'
import { useMediaQuery } from '@vueuse/core'
import { Bolt, DeviceFloppy, PlayerPlay, PlayerStop, Stars } from '@vicons/tabler'
import { urlBase } from '@/stores/_config'
import TheaterEffectPanel from '@/views/theater/effects/TheaterEffectPanel.vue'
import TheaterEffectPositionAdjuster from '@/views/theater/effects/TheaterEffectPositionAdjuster.vue'
import { setTheaterEffectConfig, theaterEffectConfigFromObject } from '@/views/theater/effects/theater-effect-types'
import TheaterSequencePanel from '@/views/theater/sequences/TheaterSequencePanel.vue'
import type { StageImageRef, StageObjectTransform } from '@/views/theater/shared/stage-types'
import { resolveTheaterStageMediaLocation } from '@/views/theater/stage/stage-media'
import { useWorldPresentationEditor } from './useWorldPresentationEditor'
import { WORLD_PRESENTATION_MAX_EFFECTS } from './world-presentation-config'
import { canPlayWorldSequence, type WorldPresentationLiveControl, type WorldPresentationTarget } from './world-presentation-runtime'
import WorldPresentationLiveBoard from './WorldPresentationLiveBoard.vue'
import WorldPresentationOverlay from './WorldPresentationOverlay.vue'

const props = defineProps<{ worldId: string, live?: WorldPresentationLiveControl }>()
const emit = defineEmits<{ saved: [revision: string] }>()

const editor = useWorldPresentationEditor(props.worldId)
const { draft, selectedId, dirty, loading, saving, error, stale, ready, canEdit, adapter } = editor
const dialog = useDialog()
const themeVars = useThemeVars()
const wide = useMediaQuery('(min-width: 860px)')
const sequencePanel = ref<InstanceType<typeof TheaterSequencePanel> | null>(null)
const mediaInput = ref<HTMLInputElement | null>(null)
const pendingMediaEffectId = ref('')
type Tab = 'live' | 'effects' | 'sequences'
const tab = ref<Tab>(props.live ? 'live' : 'effects')

const themeStyle = computed(() => ({
  '--wp-accent': themeVars.value.primaryColor,
  '--wp-accent-alt': themeVars.value.infoColor,
  '--theater-accent': themeVars.value.primaryColor,
  '--theater-border': 'var(--sc-border-strong)',
  '--theater-panel-bg': 'var(--sc-bg-surface)',
}))

const status = computed(() => {
  if (!ready.value) return loading.value ? { tone: 'muted', text: '读取中…' } : { tone: 'error', text: '读取失败' }
  if (saving.value) return { tone: 'muted', text: '保存中…' }
  if (stale.value) return { tone: 'warning', text: '版本已过期' }
  if (dirty.value) return { tone: 'warning', text: '有未保存的修改' }
  return { tone: 'success', text: '已保存' }
})

// Live playback resolves targets in the saved revision, never in this draft.
const savedEffect = (id: string) => props.live?.config?.effects.find((effect) => effect.id === id && effect.visible)
const savedSequencePlayable = (id: string) => {
  const config = props.live?.config
  const sequence = config?.sequences.find((item) => item.id === id)
  return Boolean(config && sequence && canPlayWorldSequence(sequence, config))
}
const livePlayTitle = (available: boolean) => (!available
  ? '保存并启用后才能现场播放'
  : dirty.value ? '现场播放已保存的版本' : '向当前频道现场播放')

const save = async () => {
  if (await editor.save()) emit('saved', editor.revision.value)
}

const confirmDiscard = (title: string, content: string, positiveText: string, onConfirm: () => void) => {
  dialog.warning({ title, content, positiveText, negativeText: '取消', onPositiveClick: onConfirm })
}

const reload = () => {
  if (!dirty.value) {
    void editor.load()
    return
  }
  confirmDiscard('重新加载演出配置', '本地未保存的修改将被丢弃，并载入服务器上的最新版本。', '放弃修改并重新加载', () => { void editor.load() })
}

const jumpToEdit = (kind: WorldPresentationTarget, id: string) => {
  if (kind === 'effect') {
    selectedId.value = id
    tab.value = 'effects'
  } else {
    tab.value = 'sequences'
    sequencePanel.value?.openSequence(id)
  }
}

const requestMediaUpload = (effectId: string) => {
  if (!canEdit.value || editor.mediaUploading.value) return
  pendingMediaEffectId.value = effectId
  mediaInput.value?.click()
}

const handleMediaInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  const effectId = pendingMediaEffectId.value
  input.value = ''
  pendingMediaEffectId.value = ''
  if (file && effectId) void editor.uploadMedia(effectId, file)
}

// Position edits go straight into the draft, so the adjuster previews unsaved changes.
const resolveMedia = (image: StageImageRef) => resolveTheaterStageMediaLocation(image, {
  worldId: props.worldId,
  channelId: '',
  scopeType: 'world',
  urlBase,
})
const updateEffectTransform = (id: string, transform: StageObjectTransform) => {
  if (canEdit.value) adapter.edit('调整特效位置', [id], (object) => { object.transform = transform })
}
const updateEffectMediaPosition = (id: string, position: { x: number, y: number }) => {
  if (!canEdit.value) return
  adapter.edit('移动特效媒体', [id], (object) => {
    const effect = theaterEffectConfigFromObject(object)
    effect.builtin.mediaTransform.x = position.x
    effect.builtin.mediaTransform.y = position.y
    setTheaterEffectConfig(object, effect)
  })
}

const handleKeydown = (event: KeyboardEvent) => {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    void save()
  }
}

defineExpose({
  hasUnsavedChanges: () => dirty.value,
  isSaving: () => saving.value || editor.mediaUploading.value,
})
</script>

<template>
  <div class="wp-console" :style="themeStyle" @keydown="handleKeydown">
    <input ref="mediaInput" class="wp-console__media-input" type="file" accept="image/png,image/apng,image/jpeg,image/webp,image/gif,video/webm,.apng,.webm" @change="handleMediaInput">
    <header class="wp-console__header">
      <div class="wp-console__title">
        <span class="wp-console__badge"><n-icon><Stars /></n-icon></span>
        <div>
          <strong>演出与特效</strong>
          <small>世界级配置 · 对本世界所有频道生效</small>
        </div>
      </div>

      <n-tabs v-model:value="tab" type="segment" size="small" class="wp-console__tabs" :animated="false">
        <n-tab v-if="live" name="live">现场</n-tab>
        <n-tab name="effects">特效{{ ready ? ` ${draft.effects.length}` : '' }}</n-tab>
        <n-tab name="sequences">序列{{ ready ? ` ${draft.sequences.length}` : '' }}</n-tab>
      </n-tabs>

      <div class="wp-console__actions">
        <span class="wp-console__status" :class="`is-${status.tone}`" role="status" :title="status.text">
          <i aria-hidden="true" /><span>{{ status.text }}</span>
        </span>
        <n-button v-if="dirty && !saving" size="small" quaternary @click="reload">放弃修改</n-button>
        <n-button
          size="small"
          type="primary"
          :loading="saving"
          :disabled="!dirty || stale || !ready || editor.mediaUploading.value"
          title="保存（Ctrl+S）"
          @click="save"
        >
          <template #icon><n-icon><DeviceFloppy /></n-icon></template>
          保存
        </n-button>
        <slot name="header-extra" />
      </div>
    </header>

    <div v-if="ready && (stale || error)" class="wp-console__banners">
      <n-alert v-if="stale" type="warning" :show-icon="true" title="其他管理员已保存了更新的演出配置">
        <div class="wp-console__banner-body">
          <span>你的修改基于旧版本，无法直接保存。重新加载会载入最新版本并丢弃本地修改。</span>
          <n-button size="small" @click="reload">重新加载</n-button>
        </div>
      </n-alert>
      <n-alert v-else-if="error" type="error" closable @close="error = ''">{{ error }}</n-alert>
    </div>

    <main class="wp-console__body">
      <div v-if="!ready && loading" class="wp-console__center"><n-spin /></div>
      <n-empty v-else-if="!ready" class="wp-console__center" description="无法读取演出配置">
        <template #extra><n-button size="small" @click="editor.load()">重试</n-button></template>
      </n-empty>
      <template v-else>
        <WorldPresentationLiveBoard
          v-if="live && tab === 'live'"
          :config="live.config"
          :failed="live.failed"
          :play="live.play"
          @edit="jumpToEdit"
          @retry="live.retry()"
        />
        <p v-if="live && tab === 'live' && dirty" class="wp-console__note">
          <n-icon><Bolt /></n-icon>现场播放使用已保存的版本，未保存的修改不会出现在这里。
        </p>

        <section v-show="tab === 'effects'" class="wp-console__pane" aria-label="特效">
          <TheaterEffectPanel
            :objects="draft.effects"
            :selected-id="selectedId"
            :adapter="adapter"
            :can-edit="canEdit"
            :can-add="draft.effects.length < WORLD_PRESENTATION_MAX_EFFECTS"
            :can-upload="canEdit && !editor.mediaUploading.value"
            :audio-assets="editor.audioAssets.value"
            :audio-loading="editor.audioLoading.value"
            :audio-uploading="false"
            :audio-error="editor.audioError.value"
            :organizer-folders="[]"
            :organizer-items="[]"
            :layout="wide ? 'split' : 'stack'"
            @upload="requestMediaUpload"
            @media-url="editor.setMedia"
          >
            <template #position="{ object }">
              <TheaterEffectPositionAdjuster
                :object="object"
                :resolve-media="resolveMedia"
                :disabled="!canEdit"
                @update-transform="transform => updateEffectTransform(object.id, transform)"
                @update-media-position="position => updateEffectMediaPosition(object.id, position)"
              />
            </template>
            <template v-if="live" #actions="{ object }">
              <n-button
                size="small"
                type="primary"
                secondary
                :disabled="!savedEffect(object.id)"
                :title="livePlayTitle(!!savedEffect(object.id))"
                @click="live.play('effect', object.id)"
              >
                <template #icon><n-icon><Bolt /></n-icon></template>
                现场播放
              </n-button>
            </template>
          </TheaterEffectPanel>
        </section>

        <section v-show="tab === 'sequences'" class="wp-console__pane wp-console__pane--sequences" aria-label="序列">
          <p class="wp-console__scope-note">世界级序列支持消息关键词触发和「播放特效」步骤；小剧场专属的步骤与组件点击会原样保留，但不会在聊天页执行。</p>
          <TheaterSequencePanel
            ref="sequencePanel"
            :scope-id="worldId"
            :sequences="draft.sequences"
            :world-effects="draft.effects"
            :commit-sequences="sequences => editor.update({ sequences })"
            :can-edit="canEdit"
            can-test
            @test="editor.testSequence"
          >
            <template v-if="live" #row-actions="{ sequence }">
              <n-button
                text
                size="tiny"
                type="primary"
                :aria-label="`现场播放序列 ${sequence.name}`"
                :title="livePlayTitle(savedSequencePlayable(sequence.id))"
                :disabled="!savedSequencePlayable(sequence.id)"
                @click="live.play('sequence', sequence.id)"
              >
                <n-icon><Bolt /></n-icon>
              </n-button>
            </template>
          </TheaterSequencePanel>
        </section>
      </template>
    </main>

    <WorldPresentationOverlay :world-id="worldId" :playbacks="editor.previewPlaybacks.value" elevated />
    <Teleport to="body">
      <button
        v-if="editor.previewPlaybacks.value.length"
        type="button"
        class="wp-preview-stop"
        :style="themeStyle"
        @click="editor.stopPreview()"
      >
        <n-icon><PlayerStop /></n-icon>停止预览
        <small><n-icon><PlayerPlay /></n-icon>仅你可见</small>
      </button>
    </Teleport>
  </div>
</template>

<style scoped>
.wp-console { height: 100%; min-height: 0; display: flex; flex-direction: column; overflow: hidden; border-radius: inherit; color: var(--sc-text-primary); background: var(--sc-bg-surface); }
.wp-console__media-input { display: none; }
.wp-console__header { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--sc-border-strong); background: var(--sc-bg-header, var(--sc-bg-surface)); }
.wp-console__title { min-width: 0; display: flex; align-items: center; gap: 10px; }
.wp-console__title > div { min-width: 0; display: grid; gap: 1px; }
.wp-console__title strong { font-size: 15px; }
.wp-console__title small { overflow: hidden; color: var(--sc-text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.wp-console__badge { width: 32px; height: 32px; display: grid; flex: 0 0 auto; place-items: center; border-radius: 9px; color: var(--wp-accent); background: color-mix(in srgb, var(--wp-accent) 15%, transparent); font-size: 18px; }
.wp-console__tabs { width: 300px; }
.wp-console__actions { min-width: 0; display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.wp-console__status { display: inline-flex; align-items: center; gap: 6px; overflow: hidden; color: var(--sc-text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.wp-console__status i { width: 7px; height: 7px; flex: 0 0 auto; border-radius: 50%; background: currentColor; }
.wp-console__status.is-success i { background: v-bind('themeVars.successColor'); }
.wp-console__status.is-warning { color: v-bind('themeVars.warningColor'); }
.wp-console__status.is-error { color: v-bind('themeVars.errorColor'); }
.wp-console__banners { display: grid; gap: 8px; padding: 10px 16px 0; }
.wp-console__banner-body { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; }
.wp-console__body { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; }
.wp-console__center { margin: auto; padding: 48px 16px; }
.wp-console__note { display: flex; align-items: center; gap: 6px; margin: 0; padding: 8px 20px; border-top: 1px solid var(--sc-border-mute); color: var(--sc-text-secondary); font-size: 12px; }
.wp-console__pane { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; }
.wp-console__pane--sequences { width: min(100%, 980px); margin-inline: auto; }
.wp-console__scope-note { margin: 0; padding: 12px 16px 4px; color: var(--sc-text-secondary); font-size: 12px; line-height: 1.6; }

/* The theater panels are tuned for a compact floating panel; give them console density. */
.wp-console :deep(.theater-effect-row__select),
.wp-console :deep(.theater-effect-folder__main strong) { font-size: 13px; }
.wp-console :deep(.theater-effect-row.is-active) { box-shadow: inset 3px 0 0 var(--wp-accent); }
.wp-console :deep(.theater-effect-editor > label) { font-size: 13px; }
.wp-console :deep(.theater-effect-panel-content.is-split .theater-effect-editor) { grid-template-columns: 110px minmax(0, 640px); }
.wp-console :deep(.theater-effect-panel-content.is-split .theater-effect-media-fx),
.wp-console :deep(.theater-effect-panel-content.is-split .theater-effect-colors) { max-width: 750px; }
.wp-console :deep(.theater-sequence-panel__toolbar) { padding: 10px 16px; }
.wp-console :deep(.theater-sequence-panel__row) { min-height: 64px; padding: 8px 16px; gap: 12px; }
.wp-console :deep(.theater-sequence-panel__select strong) { font-size: 14px; }
.wp-console :deep(.theater-sequence-panel__select span),
.wp-console :deep(.theater-sequence-panel__select small) { font-size: 12px; }
.wp-console :deep(.theater-sequence-panel__actions) { gap: 6px; }
.wp-console :deep(.theater-sequence-panel__actions .n-button) { width: 28px; height: 28px; font-size: 16px; }
.wp-console :deep(.theater-sequence-panel__empty) { font-size: 13px; }

.wp-preview-stop { position: fixed; z-index: 4001; top: 16px; right: 0; left: 0; width: fit-content; display: flex; margin-inline: auto; align-items: center; gap: 8px; padding: 9px 16px; border: 1px solid color-mix(in srgb, var(--wp-accent) 60%, transparent); border-radius: 999px; color: #fff; background: rgba(15, 23, 42, .86); box-shadow: 0 10px 30px rgba(0, 0, 0, .35); font: inherit; font-size: 13px; cursor: pointer; }
.wp-preview-stop:hover, .wp-preview-stop:focus-visible { background: rgba(15, 23, 42, .96); outline: 2px solid var(--wp-accent); outline-offset: 2px; }
.wp-preview-stop small { display: inline-flex; align-items: center; gap: 3px; opacity: .7; font-size: 11px; }

@media (max-width: 859px) {
  .wp-console__header { grid-template-columns: minmax(0, 1fr) auto; gap: 10px; padding: 10px 12px; }
  .wp-console__tabs { width: 100%; grid-column: 1 / -1; grid-row: 2; }
  .wp-console__title small { display: none; }
}
@media (max-width: 480px) {
  .wp-console__status > span { display: none; }
}
</style>
