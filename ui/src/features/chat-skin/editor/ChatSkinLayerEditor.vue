<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  CHAT_SKIN_ANCHORS,
  CHAT_SKIN_BLENDS,
  CHAT_SKIN_MAX_LAYERS,
  createChatSkinLayerId,
  createDefaultChatSkinLayer,
  type ChatSkinAnchor,
  type ChatSkinLayer,
} from '../chatSkinSchema'
import { CHAT_SKIN_ORNAMENTS, findChatSkinOrnament, type ChatSkinOrnament } from '../chatSkinOrnaments'
import ChatSkinColorInput from './ChatSkinColorInput.vue'

// 图层管理：列表（显隐 / 排序 / 复制 / 删除）+ 选中图层属性。
// 图层的位置也可以直接在左侧实时预览里拖动。
const props = defineProps<{
  layers: ChatSkinLayer[]
  selectedId: string
  resolveThumb: (ref: string) => string
  uploading?: boolean
  readonly?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:selectedId', value: string): void
  (e: 'upload-layer'): void
}>()

const pickerOpen = ref(false)
const full = computed(() => props.layers.length >= CHAT_SKIN_MAX_LAYERS)
const selected = computed(() => props.layers.find(layer => layer.id === props.selectedId) || null)
const selectedOrnament = computed(() => (selected.value ? findChatSkinOrnament(selected.value.ref) : null))

const anchorLabels: Record<ChatSkinAnchor, string> = {
  'top-left': '左上', top: '上', 'top-right': '右上',
  left: '左', center: '中', right: '右',
  'bottom-left': '左下', bottom: '下', 'bottom-right': '右下',
}
const blendOptions = CHAT_SKIN_BLENDS.map(value => ({
  label: { normal: '正常', multiply: '正片叠底', screen: '滤色', overlay: '叠加', 'soft-light': '柔光' }[value],
  value,
}))

const addOrnament = (ornament: ChatSkinOrnament) => {
  if (full.value || props.readonly) return
  const layer = createDefaultChatSkinLayer(`builtin:${ornament.id}`, {
    label: ornament.label,
    size: ornament.size,
    aspect: ornament.aspect,
    anchor: ornament.group === 'edge' ? 'bottom' : 'top-left',
    tint: ornament.defaultTint,
  })
  props.layers.push(layer)
  emit('update:selectedId', layer.id)
  pickerOpen.value = false
}

const layerName = (layer: ChatSkinLayer, index: number) => (
  layer.label || findChatSkinOrnament(layer.ref)?.label || `图片 ${index + 1}`
)

const move = (index: number, delta: number) => {
  const target = index + delta
  if (target < 0 || target >= props.layers.length) return
  const [item] = props.layers.splice(index, 1)
  props.layers.splice(target, 0, item)
}
const duplicate = (index: number) => {
  if (full.value) return
  const copy = { ...props.layers[index], id: createChatSkinLayerId(), x: props.layers[index].x + 8, y: props.layers[index].y + 8 }
  props.layers.splice(index + 1, 0, copy)
  emit('update:selectedId', copy.id)
}
const remove = (index: number) => {
  const [item] = props.layers.splice(index, 1)
  if (item?.id === props.selectedId) emit('update:selectedId', props.layers[Math.max(0, index - 1)]?.id || '')
}

const thumbStyle = (layer: ChatSkinLayer) => {
  const ornament = findChatSkinOrnament(layer.ref)
  if (ornament) {
    const mask = `url("${ornament.src}") center / contain no-repeat`
    return { '-webkit-mask': mask, mask, backgroundColor: layer.tint || ornament.defaultTint }
  }
  const src = props.resolveThumb(layer.ref)
  return src ? { background: `url("${src.replace(/"/g, '')}") center / contain no-repeat` } : {}
}
const ornamentStyle = (ornament: ChatSkinOrnament) => {
  const mask = `url("${ornament.src}") center / contain no-repeat`
  return { '-webkit-mask': mask, mask, backgroundColor: ornament.defaultTint }
}
const groupedOrnaments = computed(() => ([
  { key: 'shape', label: '造型', items: CHAT_SKIN_ORNAMENTS.filter(item => item.group === 'shape') },
  { key: 'edge', label: '边饰', items: CHAT_SKIN_ORNAMENTS.filter(item => item.group === 'edge') },
  { key: 'stamp', label: '印章', items: CHAT_SKIN_ORNAMENTS.filter(item => item.group === 'stamp') },
]))
</script>

<template>
  <div class="csk-layers" :class="{ 'is-readonly': readonly }">
    <div class="csk-layers__toolbar">
      <n-popover v-model:show="pickerOpen" trigger="click" placement="bottom-start" :show-arrow="false">
        <template #trigger>
          <n-button size="small" type="primary" secondary :disabled="full || readonly">添加纹样</n-button>
        </template>
        <div class="csk-ornaments">
          <div v-for="group in groupedOrnaments" :key="group.key" class="csk-ornaments__group">
            <div class="csk-ornaments__label">{{ group.label }}</div>
            <div class="csk-ornaments__grid">
              <button
                v-for="ornament in group.items"
                :key="ornament.id"
                type="button"
                class="csk-ornaments__item"
                :title="ornament.label"
                @click="addOrnament(ornament)"
              >
                <span class="csk-ornaments__swatch" :style="ornamentStyle(ornament)"></span>
                <span class="csk-ornaments__name">{{ ornament.label }}</span>
              </button>
            </div>
          </div>
        </div>
      </n-popover>
      <n-button size="small" secondary :loading="uploading" :disabled="full || readonly" @click="emit('upload-layer')">上传图片</n-button>
      <span class="csk-layers__count">{{ layers.length }} / {{ CHAT_SKIN_MAX_LAYERS }}</span>
    </div>

    <div v-if="!layers.length" class="csk-layers__empty">
      还没有图层。添加纹样或上传透明 PNG，做出猫耳、蝴蝶结、角花等造型。
    </div>
    <ul v-else class="csk-layers__list">
      <li
        v-for="(layer, index) in layers"
        :key="layer.id"
        class="csk-layers__row"
        :class="{ 'is-selected': layer.id === selectedId, 'is-hidden': !layer.enabled }"
        @click="emit('update:selectedId', layer.id)"
      >
        <span class="csk-layers__thumb" :style="thumbStyle(layer)"></span>
        <span class="csk-layers__name">{{ layerName(layer, index) }}</span>
        <span class="csk-layers__z">{{ layer.z === 'back' ? '底' : '面' }}</span>
        <span class="csk-layers__ops" @click.stop>
          <n-button size="tiny" quaternary :title="layer.enabled ? '隐藏' : '显示'" @click="layer.enabled = !layer.enabled">{{ layer.enabled ? '隐' : '显' }}</n-button>
          <n-button size="tiny" quaternary title="上移" :disabled="index === 0" @click="move(index, -1)">↑</n-button>
          <n-button size="tiny" quaternary title="下移" :disabled="index === layers.length - 1" @click="move(index, 1)">↓</n-button>
          <n-button size="tiny" quaternary title="复制" :disabled="full" @click="duplicate(index)">⧉</n-button>
          <n-button size="tiny" quaternary type="error" title="删除" @click="remove(index)">✕</n-button>
        </span>
      </li>
    </ul>

    <div v-if="selected" class="csk-layer-props">
      <div class="csk-layer-props__row">
        <n-input v-model:value="selected.label" size="small" maxlength="32" placeholder="图层名称（可选）" />
      </div>
      <div class="csk-layer-props__split">
        <div class="csk-anchor" role="group" aria-label="锚点">
          <button
            v-for="anchor in CHAT_SKIN_ANCHORS"
            :key="anchor"
            type="button"
            class="csk-anchor__cell"
            :class="{ 'is-active': selected.anchor === anchor }"
            :title="anchorLabels[anchor]"
            @click="selected.anchor = anchor"
          ></button>
        </div>
        <div class="csk-layer-props__fields">
          <label class="csk-layer-field"><span>X {{ selected.x }}</span><n-slider v-model:value="selected.x" :min="-240" :max="240" /></label>
          <label class="csk-layer-field"><span>Y {{ selected.y }}</span><n-slider v-model:value="selected.y" :min="-240" :max="240" /></label>
        </div>
      </div>
      <label class="csk-layer-field"><span>大小 {{ selected.size }}px</span><n-slider v-model:value="selected.size" :min="8" :max="480" /></label>
      <label class="csk-layer-field"><span>旋转 {{ selected.rotate }}°</span><n-slider v-model:value="selected.rotate" :min="-180" :max="180" /></label>
      <label class="csk-layer-field"><span>不透明度 {{ Math.round(selected.opacity * 100) }}%</span><n-slider v-model:value="selected.opacity" :min="0" :max="1" :step="0.05" /></label>
      <div class="csk-layer-props__grid">
        <label class="csk-layer-field">
          <span>层级</span>
          <n-radio-group v-model:value="selected.z" size="small">
            <n-radio-button value="front">文字上方</n-radio-button>
            <n-radio-button value="back">底板下方</n-radio-button>
          </n-radio-group>
        </label>
        <label class="csk-layer-field">
          <span>混合</span>
          <n-select v-model:value="selected.blend" size="small" :options="blendOptions" />
        </label>
        <label v-if="selectedOrnament" class="csk-layer-field">
          <span>着色</span>
          <ChatSkinColorInput v-model:value="selected.tint" :placeholder="selectedOrnament.defaultTint" />
        </label>
      </div>
      <div class="csk-layer-props__checks">
        <n-checkbox v-model:checked="selected.flipX">水平翻转</n-checkbox>
        <n-checkbox v-model:checked="selected.flipY">垂直翻转</n-checkbox>
        <n-checkbox v-model:checked="selected.mirror">自己发言时随气泡镜像</n-checkbox>
      </div>
      <div class="csk-layers__hint">装饰图层不会拦截点击、选择文字或任何消息操作。</div>
    </div>
  </div>
</template>

<style scoped>
.csk-layers { display: flex; flex-direction: column; gap: 10px; }
.csk-layers.is-readonly { pointer-events: none; opacity: .6; }
.csk-layers__toolbar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.csk-layers__count { margin-left: auto; font-size: 12px; color: var(--sc-text-secondary, #64748b); }
.csk-layers__empty, .csk-layers__hint { font-size: 12px; color: var(--sc-text-secondary, #64748b); }
.csk-layers__list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.csk-layers__row { display: flex; align-items: center; gap: 8px; padding: 4px 6px; border-radius: 6px; border: 1px solid transparent; cursor: pointer; min-width: 0; }
.csk-layers__row:hover { background: var(--sc-bg-hover, rgba(148, 163, 184, .12)); }
.csk-layers__row.is-selected { border-color: var(--primary-color, #6366f1); background: var(--sc-bg-hover, rgba(99, 102, 241, .08)); }
.csk-layers__row.is-hidden .csk-layers__thumb, .csk-layers__row.is-hidden .csk-layers__name { opacity: .4; }
.csk-layers__thumb { width: 26px; height: 26px; flex: none; border-radius: 4px; }
.csk-layers__name { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.csk-layers__z { font-size: 11px; color: var(--sc-text-secondary, #64748b); flex: none; }
.csk-layers__ops { display: flex; flex: none; }
.csk-ornaments { display: flex; flex-direction: column; gap: 8px; max-width: min(320px, 80vw); }
.csk-ornaments__label { font-size: 12px; color: var(--sc-text-secondary, #64748b); margin-bottom: 4px; }
.csk-ornaments__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(58px, 1fr)); gap: 4px; }
.csk-ornaments__item { display: flex; flex-direction: column; align-items: center; gap: 2px; padding: 6px 2px; border-radius: 6px; border: 1px solid transparent; background: none; cursor: pointer; color: inherit; }
.csk-ornaments__item:hover { border-color: var(--sc-border-strong, rgba(148, 163, 184, .5)); }
.csk-ornaments__swatch { width: 28px; height: 28px; }
.csk-ornaments__name { font-size: 11px; }
.csk-layer-props { display: flex; flex-direction: column; gap: 8px; padding: 10px 12px; border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .24)); border-radius: 8px; }
.csk-layer-props__split { display: flex; gap: 12px; align-items: center; }
.csk-layer-props__fields { flex: 1 1 auto; display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.csk-layer-props__grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 8px 12px; }
.csk-layer-props__checks { display: flex; gap: 12px; flex-wrap: wrap; }
.csk-layer-field { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--sc-text-secondary, #64748b); min-width: 0; }
.csk-anchor { display: grid; grid-template-columns: repeat(3, 18px); grid-template-rows: repeat(3, 18px); gap: 3px; flex: none; padding: 4px; border-radius: 6px; border: 1px dashed var(--sc-border-strong, rgba(148, 163, 184, .5)); }
.csk-anchor__cell { width: 18px; height: 18px; padding: 0; border-radius: 50%; border: 1px solid var(--sc-border-strong, rgba(148, 163, 184, .6)); background: transparent; cursor: pointer; }
.csk-anchor__cell.is-active { background: var(--primary-color, #6366f1); border-color: var(--primary-color, #6366f1); }
</style>
