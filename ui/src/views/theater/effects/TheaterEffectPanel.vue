<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  NButton,
  NButtonGroup,
  NCheckbox,
  NColorPicker,
  NIcon,
  NInput,
  NInputNumber,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSlider,
  useDialog,
} from 'naive-ui'
import { ArrowDown, ArrowUp, Eye, EyeOff, Photo, PlayerPlay, Stars, Trash, Upload } from '@vicons/tabler'

import type { StageObject } from '../shared/stage-types'
import type { AudioAsset } from '@/types/audio'
import type { TheaterStageStore } from '../stage/StageStore'
import type { TheaterEffectRuntime } from './theater-effect-runtime'
import {
  createDefaultTheaterEffectConfig,
  isTheaterEffectObject,
  setTheaterEffectConfig,
  theaterBuiltinEffectThemes,
  theaterEffectConfigFromObject,
  type TheaterEffectConfig,
  type TheaterEffectKind,
} from './theater-effect-types'

const props = defineProps<{
  store: TheaterStageStore
  runtime: TheaterEffectRuntime
  canEdit: boolean
  canUpload: boolean
  editingTarget: 'frame' | 'media'
  audioAssets: AudioAsset[]
  audioLoading: boolean
  audioUploading: boolean
  audioError: string
}>()

const emit = defineEmits<{
  upload: [objectId: string]
  uploadAudio: [objectId: string, file: File]
  'update:editingTarget': [value: 'frame' | 'media']
}>()

const effects = computed(() => Object.values(props.store.activeObjects.value)
  .filter(isTheaterEffectObject)
  .sort((left, right) => right.transform.z - left.transform.z || right.transform.order - left.transform.order))
const selectedEffect = computed(() => {
  const id = props.store.state.selectedObjectId
  const object = id ? props.store.activeObjects.value[id] : null
  return isTheaterEffectObject(object) ? object : null
})
const config = computed(() => selectedEffect.value ? theaterEffectConfigFromObject(selectedEffect.value) : null)
const hasMedia = computed(() => Boolean(config.value?.media || selectedEffect.value?.image))
const keywordDraft = ref('')
const targetActorNameDraft = ref('')
const audioInputRef = ref<HTMLInputElement | null>(null)
const pendingAudioEffectId = ref('')
const dialog = useDialog()

watch(
  [
    () => selectedEffect.value?.id,
    () => config.value?.keywords.join('\n') || '',
    () => config.value?.targetActorName || '',
  ],
  ([, keywords, targetActorName]) => {
    keywordDraft.value = keywords
    targetActorNameDraft.value = targetActorName
  },
  { immediate: true },
)

const themeOptions = theaterBuiltinEffectThemes.map((theme) => ({ label: theme, value: theme }))
const kindOptions = [
  { label: '内置特效', value: 'builtin' },
  { label: '媒体', value: 'media' },
]
const audioOptions = computed(() => {
  const options = props.audioAssets
  .filter((asset) => !asset.transcodeStatus || asset.transcodeStatus === 'ready')
    .map((asset) => ({ label: asset.name, value: asset.id }))
  const selected = config.value?.audio
  if (selected && !options.some((option) => option.value === selected.assetId)) {
    options.unshift({ label: selected.name || selected.assetId, value: selected.assetId })
  }
  return options
})

const editConfig = (label: string, mutate: (value: TheaterEffectConfig) => void) => {
  const object = selectedEffect.value
  if (!object || !props.canEdit) return
  props.store.beginObjectEdit(label)
  const next = theaterEffectConfigFromObject(object)
  mutate(next)
  setTheaterEffectConfig(object, next)
  props.store.commitObjectEdit()
}

const addEffect = (kind: TheaterEffectKind) => {
  if (!props.canEdit) return
  const object = props.store.addObject('effect')
  object.name = kind === 'media' ? '新建媒体特效' : '新建内置特效'
  setTheaterEffectConfig(object, createDefaultTheaterEffectConfig(kind))
}

const selectEffect = (object: StageObject) => props.store.selectObject(object.id)

const removeSelectedEffect = () => {
  const object = selectedEffect.value
  if (!object || !props.canEdit) return
  dialog.warning({
    title: '删除特效',
    content: `确定删除特效“${object.name}”？`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: () => props.store.removeObjects([object.id]),
  })
}

const updateKeywords = (value: string) => editConfig('修改特效关键词', (next) => {
  next.keywords = value.split(/[\n,，]/).map((item) => item.trim()).filter(Boolean)
})

const updateTargetActorName = () => editConfig('修改触发角色', (next) => {
  next.targetActorName = targetActorNameDraft.value.trim() || null
})

const updateAudioAsset = (assetId: string | null) => editConfig('修改特效音效', (next) => {
  const asset = props.audioAssets.find((item) => item.id === assetId)
  next.audio = asset ? { assetId: asset.id, name: asset.name, volume: next.audio?.volume ?? 1 } : null
})

const requestAudioUpload = (objectId: string) => {
  pendingAudioEffectId.value = objectId
  audioInputRef.value?.click()
}

const handleAudioInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  const objectId = pendingAudioEffectId.value
  input.value = ''
  pendingAudioEffectId.value = ''
  if (file && objectId) emit('uploadAudio', objectId, file)
}

</script>

<template>
  <div class="theater-effect-panel-content">
    <input ref="audioInputRef" class="theater-effect-audio-input" type="file" accept="audio/ogg,audio/mpeg,audio/wav,.ogg,.mp3,.wav" @change="handleAudioInput">
    <div v-if="canEdit" class="theater-effect-add-row">
      <n-button size="tiny" secondary @click="addEffect('builtin')"><template #icon><n-icon><Stars /></n-icon></template>内置</n-button>
      <n-button size="tiny" secondary @click="addEffect('media')"><template #icon><n-icon><Photo /></n-icon></template>媒体</n-button>
    </div>

    <div class="theater-effect-list">
      <div
        v-for="object in effects"
        :key="object.id"
        class="theater-effect-row"
        :class="{ 'is-active': object.id === selectedEffect?.id, 'is-hidden': !object.visible }"
      >
        <button type="button" class="theater-effect-row__select" @click="selectEffect(object)">
          <n-icon :component="theaterEffectConfigFromObject(object).kind === 'builtin' ? Stars : Photo" />
          <span>{{ object.name }}</span>
          <small>{{ theaterEffectConfigFromObject(object).keywords.length }}</small>
        </button>
        <button
          v-if="canEdit"
          type="button"
          class="theater-effect-row__icon"
          :aria-label="object.visible ? '禁用特效' : '启用特效'"
          @click="store.setObjectFlag(object.id, 'visible', !object.visible)"
        ><n-icon :component="object.visible ? Eye : EyeOff" /></button>
      </div>
      <div v-if="!effects.length" class="theater-effect-empty">暂无特效</div>
    </div>

    <div v-if="selectedEffect && config" class="theater-effect-editor">
      <label>名称</label>
      <n-input :value="selectedEffect.name" size="small" maxlength="512" @update:value="value => { store.beginObjectEdit('修改特效名称'); selectedEffect!.name = value; store.commitObjectEdit() }" />

      <label>类型</label>
      <n-select :value="config.kind" :options="kindOptions" size="small" @update:value="value => editConfig('修改特效类型', next => { next.kind = value as TheaterEffectKind })" />

      <label>关键词</label>
      <n-input
        :value="keywordDraft"
        type="textarea"
        :autosize="{ minRows: 2, maxRows: 4 }"
        placeholder="每行一个；命中任意关键词触发"
        @update:value="keywordDraft = $event"
        @change="updateKeywords(keywordDraft)"
      />

      <label>指定频道角色名</label>
      <n-input
        :value="targetActorNameDraft"
        size="small"
        clearable
        placeholder="按角色名匹配；留空表示全部"
        @update:value="targetActorNameDraft = $event"
        @change="updateTargetActorName"
      />

      <label>持续时间</label>
      <n-input-number :value="config.durationMs" :min="300" :max="30000" :step="100" @update:value="value => value !== null && editConfig('修改特效时长', next => { next.durationMs = value })" />

      <label>冷却时间</label>
      <n-input-number :value="config.cooldownMs" :min="0" :max="300000" :step="500" @update:value="value => value !== null && editConfig('修改特效冷却', next => { next.cooldownMs = value })" />

      <label>媒体</label>
      <div class="theater-effect-media-row">
        <n-button size="small" :type="hasMedia ? 'primary' : 'default'" secondary :disabled="!canUpload" @click="emit('upload', selectedEffect.id)">
          <template #icon><n-icon><Photo /></n-icon></template>
          {{ hasMedia ? '图片' : '上传' }}
        </n-button>
      </div>

      <label>音效</label>
      <div class="theater-effect-audio-row">
        <n-select
          :value="config.audio?.assetId || null"
          :options="audioOptions"
          :loading="audioLoading"
          size="small"
          clearable
          filterable
          placeholder="从频道素材选择"
          @update:value="updateAudioAsset"
        />
        <n-button size="small" secondary :disabled="!canUpload" :loading="audioUploading" @click="requestAudioUpload(selectedEffect.id)">
          <template #icon><n-icon><Upload /></n-icon></template>
          上传
        </n-button>
      </div>

      <template v-if="config.audio">
        <label>声音大小 {{ Math.round(config.audio.volume * 100) }}%</label>
        <n-slider :value="config.audio.volume" :min="0" :max="1" :step="0.05" @update:value="value => editConfig('修改特效音量', next => { if (next.audio) next.audio.volume = value })" />
      </template>

      <p v-if="audioError" class="theater-effect-audio-error">{{ audioError }}</p>

      <template v-if="config.kind === 'builtin'">
        <label>主题</label>
        <n-select :value="config.builtin.theme" :options="themeOptions" size="small" @update:value="value => editConfig('修改特效主题', next => { next.builtin.theme = value })" />

        <label>格式</label>
        <n-radio-group :value="config.builtin.format" size="small" @update:value="value => editConfig('修改特效格式', next => { next.builtin.format = value })">
          <n-radio-button value="popout">弹出</n-radio-button>
          <n-radio-button value="boxed">框内</n-radio-button>
        </n-radio-group>

        <label>主文案</label>
        <n-input :value="config.builtin.text" size="small" maxlength="512" @update:value="value => editConfig('修改特效文案', next => { next.builtin.text = value })" />

        <label>副文案</label>
        <n-input :value="config.builtin.subText" size="small" maxlength="512" @update:value="value => editConfig('修改特效文案', next => { next.builtin.subText = value })" />

        <div class="theater-effect-colors">
          <label><span>强调</span><n-color-picker :value="config.builtin.accentColor" :show-alpha="false" :modes="['hex']" @update:value="value => editConfig('修改特效颜色', next => { next.builtin.accentColor = value })" /></label>
          <label><span>主字</span><n-color-picker :value="config.builtin.mainTextColor" :show-alpha="false" :modes="['hex']" @update:value="value => editConfig('修改特效颜色', next => { next.builtin.mainTextColor = value })" /></label>
          <label><span>副字</span><n-color-picker :value="config.builtin.subTextColor" :show-alpha="false" :modes="['hex']" @update:value="value => editConfig('修改特效颜色', next => { next.builtin.subTextColor = value })" /></label>
        </div>

        <label>背景压暗 {{ config.builtin.dimIntensity }}%</label>
        <n-slider :value="config.builtin.dimIntensity" :min="0" :max="100" :step="1" @update:value="value => editConfig('修改特效背景', next => { next.builtin.dimIntensity = value })" />

        <label>震动 {{ config.builtin.shakeIntensity.toFixed(1) }}</label>
        <n-slider :value="config.builtin.shakeIntensity" :min="0" :max="10" :step="0.5" @update:value="value => editConfig('修改特效震动', next => { next.builtin.shakeIntensity = value })" />

        <label>媒体缩放 {{ config.builtin.mediaTransform.scale.toFixed(2) }}</label>
        <n-slider :value="config.builtin.mediaTransform.scale" :min="0.1" :max="5" :step="0.05" @update:value="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.scale = value })" />

        <label>媒体旋转</label>
        <n-input-number :value="config.builtin.mediaTransform.rotation" :min="-360" :max="360" @update:value="value => value !== null && editConfig('修改特效媒体', next => { next.builtin.mediaTransform.rotation = value })" />
        <n-checkbox :checked="config.builtin.mediaTransform.mirror" @update:checked="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.mirror = value })">镜像媒体</n-checkbox>
      </template>

      <template v-if="config.kind === 'media'">
        <label>媒体缩放 {{ config.builtin.mediaTransform.scale.toFixed(2) }}</label>
        <n-slider :value="config.builtin.mediaTransform.scale" :min="0.1" :max="5" :step="0.05" @update:value="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.scale = value })" />
        <label>媒体旋转</label>
        <n-input-number :value="config.builtin.mediaTransform.rotation" :min="-360" :max="360" @update:value="value => value !== null && editConfig('修改特效媒体', next => { next.builtin.mediaTransform.rotation = value })" />
        <n-checkbox :checked="config.builtin.mediaTransform.mirror" @update:checked="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.mirror = value })">镜像媒体</n-checkbox>
      </template>

      <label>画布控制</label>
      <n-button-group size="small">
        <n-button :type="editingTarget === 'frame' ? 'primary' : 'default'" @click="emit('update:editingTarget', 'frame')">特效框</n-button>
        <n-button :type="editingTarget === 'media' ? 'primary' : 'default'" @click="emit('update:editingTarget', 'media')">媒体</n-button>
      </n-button-group>

      <div class="theater-effect-actions">
        <n-button size="small" secondary @click="runtime.preview(selectedEffect)"><template #icon><n-icon><PlayerPlay /></n-icon></template>测试</n-button>
        <n-button size="small" quaternary @click="store.moveOrder(selectedEffect.id, 1)"><template #icon><n-icon><ArrowUp /></n-icon></template></n-button>
        <n-button size="small" quaternary @click="store.moveOrder(selectedEffect.id, -1)"><template #icon><n-icon><ArrowDown /></n-icon></template></n-button>
        <n-button v-if="canEdit" size="small" type="error" secondary @click="removeSelectedEffect"><template #icon><n-icon><Trash /></n-icon></template>删除</n-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.theater-effect-panel-content { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; }
.theater-effect-add-row { display: flex; gap: 6px; padding: 8px; border-bottom: 1px solid var(--theater-border); }
.theater-effect-list { max-height: 190px; overflow: auto; border-bottom: 1px solid var(--theater-border); }
.theater-effect-row { min-height: 38px; display: flex; align-items: center; }
.theater-effect-row:hover, .theater-effect-row.is-active { background: color-mix(in srgb, var(--theater-accent) 16%, transparent); }
.theater-effect-row.is-hidden { opacity: .55; }
.theater-effect-row__select { min-width: 0; flex: 1; display: flex; align-items: center; gap: 7px; padding: 7px 9px; border: 0; color: inherit; background: transparent; text-align: left; cursor: pointer; }
.theater-effect-row__select span { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-effect-row__select small { color: var(--sc-text-secondary); }
.theater-effect-row__icon { width: 32px; height: 32px; border: 0; color: inherit; background: transparent; cursor: pointer; }
.theater-effect-empty { padding: 22px; color: var(--sc-text-secondary); text-align: center; }
.theater-effect-editor { min-height: 0; display: grid; grid-template-columns: 92px minmax(0, 1fr); align-items: center; gap: 8px; overflow: auto; padding: 10px; }
.theater-effect-editor > label { color: var(--sc-text-secondary); font-size: 12px; }
.theater-effect-media-row, .theater-effect-actions { display: flex; gap: 5px; }
.theater-effect-audio-row { min-width: 0; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 5px; }
.theater-effect-audio-input { display: none; }
.theater-effect-audio-error { grid-column: 1 / -1; margin: 0; color: #f87171; font-size: 11px; }
.theater-effect-colors { grid-column: 1 / -1; display: grid; grid-template-columns: repeat(3, 1fr); gap: 7px; }
.theater-effect-colors label { display: grid; gap: 4px; color: var(--sc-text-secondary); font-size: 11px; }
.theater-effect-actions { grid-column: 1 / -1; justify-content: flex-end; padding-top: 4px; }
</style>
