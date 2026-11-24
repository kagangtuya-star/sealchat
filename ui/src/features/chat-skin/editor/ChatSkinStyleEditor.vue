<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  CHAT_SKIN_ACCENT_SIDES,
  CHAT_SKIN_BORDER_STYLES,
  CHAT_SKIN_FONTS,
  CHAT_SKIN_IMAGE_FITS,
  CHAT_SKIN_IMAGE_POSITIONS,
  createDefaultChatSkinBackground,
  type ChatSkinLayout,
  type ChatSkinStyle,
  type ChatSkinStyleKey,
} from '../chatSkinSchema'
import ChatSkinColorInput from './ChatSkinColorInput.vue'

// 样式分组编辑器：直接修改传入的 style（日间样式或夜间覆写）。
// 分组「未开启」= 字段不存在 = 继承当前主题；夜间模式下「未开启」= 沿用日间设置。
const props = defineProps<{
  skinStyle: ChatSkinStyle
  layout: ChatSkinLayout
  night?: boolean
  dayStyle?: ChatSkinStyle
  backgroundImageUrl?: string
  uploading?: boolean
  readonly?: boolean
}>()

const emit = defineEmits<{
  (e: 'pick-background-image'): void
}>()

type Section = 'shape' | 'text' | 'space'
const section = ref<Section>('shape')

const groupDefaults: Record<Exclude<ChatSkinStyleKey, 'blur' | 'text' | 'name'>, () => unknown> = {
  background: createDefaultChatSkinBackground,
  border: () => ({ width: 1, style: 'solid', color: '#94a3b8' }),
  radius: () => ({ tl: 12, tr: 12, br: 12, bl: 12 }),
  shadow: () => ({ mode: 'custom', color: '#0f172a29', blur: 12, x: 0, y: 4, spread: 0 }),
  padding: () => (props.layout === 'bubble' ? { x: 14, y: 10 } : { x: 8, y: 4 }),
  size: () => ({ minWidth: 0, minHeight: 0, maxWidth: 0 }),
  spacing: () => ({ top: 0, bottom: 0 }),
  accent: () => ({ color: '#6366f1', width: 3, side: 'left' }),
}

const s = computed(() => props.skinStyle as Record<string, any>)
const has = (key: ChatSkinStyleKey) => s.value[key] !== undefined

const toggleGroup = (key: keyof typeof groupDefaults, enabled: boolean) => {
  if (props.readonly) return
  if (!enabled) {
    delete s.value[key]
    return
  }
  const fromDay = props.night ? (props.dayStyle as Record<string, any> | undefined)?.[key] : undefined
  s.value[key] = fromDay ? JSON.parse(JSON.stringify(fromDay)) : groupDefaults[key]()
}

const ensure = (key: 'text' | 'name') => {
  if (!s.value[key]) s.value[key] = {}
  return s.value[key] as Record<string, any>
}
const setPartial = (key: 'text' | 'name', field: string, value: unknown) => {
  if (props.readonly) return
  const target = ensure(key)
  if (value === '' || value === undefined || value === null || value === false || (field === 'scale' && value === 1)) {
    delete target[field]
  } else {
    target[field] = value
  }
  if (!Object.keys(target).length) delete s.value[key]
}

const radiusLinked = ref(true)
const radiusAll = computed(() => s.value.radius?.tl ?? 12)
const setRadiusAll = (value: number) => {
  if (!s.value.radius) return
  s.value.radius = { tl: value, tr: value, br: value, bl: value }
}

const backgroundMode = computed(() => (s.value.background ? s.value.background.mode : 'inherit'))
const setBackgroundMode = (mode: string) => {
  if (props.readonly) return
  if (mode === 'inherit') {
    delete s.value.background
    return
  }
  if (!s.value.background) toggleGroup('background', true)
  s.value.background.mode = mode
}

const shadowMode = computed(() => (s.value.shadow ? s.value.shadow.mode : 'inherit'))
const setShadowMode = (mode: string) => {
  if (props.readonly) return
  if (mode === 'inherit') {
    delete s.value.shadow
    return
  }
  if (!s.value.shadow) toggleGroup('shadow', true)
  s.value.shadow.mode = mode
}

const fontLabels: Record<string, string> = {
  inherit: '继承主题', sans: '黑体', serif: '宋体', kai: '楷体', mono: '等宽', rounded: '圆体',
}
const fontOptions = CHAT_SKIN_FONTS.map(value => ({ label: fontLabels[value], value }))
const borderStyleOptions = CHAT_SKIN_BORDER_STYLES.map(value => ({
  label: { solid: '实线', dashed: '虚线', dotted: '点线', double: '双线' }[value],
  value,
}))
const fitOptions = CHAT_SKIN_IMAGE_FITS.map(value => ({
  label: { cover: '铺满裁切', contain: '完整显示', tile: '平铺', stretch: '拉伸' }[value],
  value,
}))
const positionOptions = CHAT_SKIN_IMAGE_POSITIONS.map(value => ({
  label: { center: '居中', top: '顶部', bottom: '底部', left: '左侧', right: '右侧' }[value],
  value,
}))
const accentSideOptions = CHAT_SKIN_ACCENT_SIDES.map(value => ({
  label: { left: '左', right: '右', top: '上', bottom: '下' }[value],
  value,
}))
const weightOptions = [
  { label: '继承', value: '' },
  { label: '常规', value: 'normal' },
  { label: '粗体', value: 'bold' },
  { label: '特粗', value: 'black' },
]

const inheritLabel = computed(() => (props.night ? '沿用日间' : '继承主题'))
</script>

<template>
  <div class="csk-style" :class="{ 'is-readonly': readonly }">
    <n-radio-group v-model:value="section" size="small" class="csk-style__tabs">
      <n-radio-button value="shape">外形</n-radio-button>
      <n-radio-button value="text">文字</n-radio-button>
      <n-radio-button value="space">尺寸间距</n-radio-button>
    </n-radio-group>

    <template v-if="section === 'shape'">
      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">底色</span>
          <n-radio-group :value="backgroundMode" size="small" @update:value="setBackgroundMode">
            <n-radio-button value="inherit">{{ inheritLabel }}</n-radio-button>
            <n-radio-button value="color">纯色</n-radio-button>
            <n-radio-button value="gradient">渐变</n-radio-button>
            <n-radio-button value="none">透明</n-radio-button>
          </n-radio-group>
        </div>
        <div v-if="s.background && s.background.mode !== 'none'" class="csk-grid">
          <label class="csk-field">
            <span>{{ s.background.mode === 'gradient' ? '起始色' : '颜色' }}</span>
            <ChatSkinColorInput v-model:value="s.background.color" :clearable="false" />
          </label>
          <template v-if="s.background.mode === 'gradient'">
            <label class="csk-field">
              <span>结束色</span>
              <ChatSkinColorInput v-model:value="s.background.color2" :clearable="false" />
            </label>
            <label class="csk-field csk-field--wide">
              <span>角度 {{ s.background.angle }}°</span>
              <n-slider v-model:value="s.background.angle" :min="0" :max="360" :step="5" />
            </label>
          </template>
        </div>
        <div v-if="s.background" class="csk-image-row">
          <div class="csk-image-row__thumb" :style="backgroundImageUrl ? { backgroundImage: `url(&quot;${backgroundImageUrl}&quot;)` } : undefined">
            <span v-if="!backgroundImageUrl">无底图</span>
          </div>
          <div class="csk-image-row__actions">
            <n-button size="tiny" secondary :loading="uploading" :disabled="readonly" @click="emit('pick-background-image')">
              {{ s.background.imageRef ? '更换底图' : '上传底图' }}
            </n-button>
            <n-button v-if="s.background.imageRef" size="tiny" quaternary :disabled="readonly" @click="s.background.imageRef = ''">移除</n-button>
          </div>
          <template v-if="s.background.imageRef">
            <n-select v-model:value="s.background.imageFit" size="tiny" :options="fitOptions" class="csk-image-row__select" />
            <n-select v-model:value="s.background.imagePosition" size="tiny" :options="positionOptions" class="csk-image-row__select" />
          </template>
        </div>
      </div>

      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">描边</span>
          <n-switch size="small" :value="has('border')" @update:value="toggleGroup('border', $event)" />
        </div>
        <div v-if="s.border" class="csk-grid">
          <label class="csk-field">
            <span>颜色</span>
            <ChatSkinColorInput v-model:value="s.border.color" :clearable="false" />
          </label>
          <label class="csk-field">
            <span>线型</span>
            <n-select v-model:value="s.border.style" size="small" :options="borderStyleOptions" />
          </label>
          <label class="csk-field csk-field--wide">
            <span>粗细 {{ s.border.width }}px</span>
            <n-slider v-model:value="s.border.width" :min="0" :max="8" :step="0.5" />
          </label>
        </div>
      </div>

      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">圆角</span>
          <n-switch size="small" :value="has('radius')" @update:value="toggleGroup('radius', $event)" />
        </div>
        <template v-if="s.radius">
          <div class="csk-grid">
            <label class="csk-field csk-field--wide">
              <span class="csk-field__row">
                <span>{{ radiusLinked ? `四角 ${radiusAll}px` : '分别设置' }}</span>
                <n-checkbox v-model:checked="radiusLinked" size="small">联动</n-checkbox>
              </span>
              <n-slider v-if="radiusLinked" :value="radiusAll" :min="0" :max="48" @update:value="setRadiusAll" />
            </label>
          </div>
          <div v-if="!radiusLinked" class="csk-grid csk-grid--4">
            <label v-for="corner in (['tl', 'tr', 'bl', 'br'] as const)" :key="corner" class="csk-field">
              <span>{{ { tl: '左上', tr: '右上', bl: '左下', br: '右下' }[corner] }}</span>
              <n-input-number v-model:value="s.radius[corner]" size="small" :min="0" :max="48" :show-button="false" />
            </label>
          </div>
        </template>
      </div>

      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">阴影</span>
          <n-radio-group :value="shadowMode" size="small" @update:value="setShadowMode">
            <n-radio-button value="inherit">{{ inheritLabel }}</n-radio-button>
            <n-radio-button value="none">无</n-radio-button>
            <n-radio-button value="custom">自定义</n-radio-button>
          </n-radio-group>
        </div>
        <div v-if="s.shadow && s.shadow.mode === 'custom'" class="csk-grid">
          <label class="csk-field">
            <span>颜色</span>
            <ChatSkinColorInput v-model:value="s.shadow.color" :clearable="false" />
          </label>
          <label class="csk-field">
            <span>模糊 {{ s.shadow.blur }}px</span>
            <n-slider v-model:value="s.shadow.blur" :min="0" :max="48" />
          </label>
          <label class="csk-field">
            <span>水平 {{ s.shadow.x }}</span>
            <n-slider v-model:value="s.shadow.x" :min="-24" :max="24" />
          </label>
          <label class="csk-field">
            <span>垂直 {{ s.shadow.y }}</span>
            <n-slider v-model:value="s.shadow.y" :min="-24" :max="24" />
          </label>
        </div>
      </div>

      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">{{ layout === 'compact' ? '侧边色条' : '强调色条' }}</span>
          <n-switch size="small" :value="has('accent')" @update:value="toggleGroup('accent', $event)" />
        </div>
        <div v-if="s.accent" class="csk-grid">
          <label class="csk-field">
            <span>颜色</span>
            <ChatSkinColorInput v-model:value="s.accent.color" :clearable="false" />
          </label>
          <label class="csk-field">
            <span>位置</span>
            <n-radio-group v-model:value="s.accent.side" size="small">
              <n-radio-button v-for="option in accentSideOptions" :key="option.value" :value="option.value">{{ option.label }}</n-radio-button>
            </n-radio-group>
          </label>
          <label class="csk-field csk-field--wide">
            <span>宽度 {{ s.accent.width }}px</span>
            <n-slider v-model:value="s.accent.width" :min="1" :max="12" />
          </label>
        </div>
      </div>

      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">毛玻璃 {{ s.blur ? `${s.blur}px` : '' }}</span>
          <span class="csk-group__hint">{{ s.blur ? '' : inheritLabel }}</span>
        </div>
        <n-slider
          :value="s.blur || 0"
          :min="0"
          :max="20"
          @update:value="(value: number) => { if (value > 0) s.blur = value; else delete s.blur }"
        />
      </div>
    </template>

    <template v-else-if="section === 'text'">
      <div class="csk-group">
        <div class="csk-group__head"><span class="csk-group__title">正文</span></div>
        <div class="csk-grid">
          <label class="csk-field">
            <span>颜色</span>
            <ChatSkinColorInput :value="s.text?.color" :placeholder="inheritLabel" @update:value="setPartial('text', 'color', $event)" />
          </label>
          <label class="csk-field">
            <span>字体</span>
            <n-select :value="s.text?.font || 'inherit'" size="small" :options="fontOptions" @update:value="setPartial('text', 'font', $event === 'inherit' ? '' : $event)" />
          </label>
          <label class="csk-field">
            <span>字号 ×{{ (s.text?.scale || 1).toFixed(2) }}</span>
            <n-slider :value="s.text?.scale || 1" :min="0.8" :max="1.4" :step="0.05" @update:value="setPartial('text', 'scale', $event)" />
          </label>
          <label class="csk-field">
            <span>文字阴影</span>
            <ChatSkinColorInput :value="s.text?.shadow" placeholder="无" @update:value="setPartial('text', 'shadow', $event)" />
          </label>
        </div>
      </div>
      <div class="csk-group">
        <div class="csk-group__head"><span class="csk-group__title">名字</span></div>
        <div class="csk-grid">
          <label class="csk-field">
            <span>颜色</span>
            <ChatSkinColorInput :value="s.name?.color" placeholder="角色色" @update:value="setPartial('name', 'color', $event)" />
          </label>
          <label class="csk-field">
            <span>字体</span>
            <n-select :value="s.name?.font || 'inherit'" size="small" :options="fontOptions" @update:value="setPartial('name', 'font', $event === 'inherit' ? '' : $event)" />
          </label>
          <label class="csk-field">
            <span>字重</span>
            <n-select :value="s.name?.weight || ''" size="small" :options="weightOptions" @update:value="setPartial('name', 'weight', $event)" />
          </label>
          <label class="csk-field">
            <span>名牌底色</span>
            <ChatSkinColorInput :value="s.name?.chip" placeholder="无" @update:value="setPartial('name', 'chip', $event)" />
          </label>
          <label class="csk-field">
            <span>发光 / 阴影</span>
            <ChatSkinColorInput :value="s.name?.shadow" placeholder="无" @update:value="setPartial('name', 'shadow', $event)" />
          </label>
          <label class="csk-field csk-field--inline">
            <n-checkbox :checked="!!s.name?.italic" @update:checked="setPartial('name', 'italic', $event)">斜体</n-checkbox>
          </label>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">内边距</span>
          <n-switch size="small" :value="has('padding')" @update:value="toggleGroup('padding', $event)" />
        </div>
        <div v-if="s.padding" class="csk-grid">
          <label class="csk-field">
            <span>左右 {{ s.padding.x }}px</span>
            <n-slider v-model:value="s.padding.x" :min="0" :max="48" />
          </label>
          <label class="csk-field">
            <span>上下 {{ s.padding.y }}px</span>
            <n-slider v-model:value="s.padding.y" :min="0" :max="40" />
          </label>
        </div>
      </div>
      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">尺寸</span>
          <n-switch size="small" :value="has('size')" @update:value="toggleGroup('size', $event)" />
        </div>
        <div v-if="s.size" class="csk-grid">
          <label class="csk-field">
            <span>最小宽度 {{ s.size.minWidth || '自动' }}</span>
            <n-slider v-model:value="s.size.minWidth" :min="0" :max="480" :step="4" />
          </label>
          <label class="csk-field">
            <span>最小高度 {{ s.size.minHeight || '自动' }}</span>
            <n-slider v-model:value="s.size.minHeight" :min="0" :max="240" :step="2" />
          </label>
          <label class="csk-field csk-field--wide">
            <span>最大宽度 {{ s.size.maxWidth ? `${s.size.maxWidth}px` : '沿用主题' }}（不会超过可用宽度）</span>
            <n-slider v-model:value="s.size.maxWidth" :min="0" :max="960" :step="8" />
          </label>
        </div>
      </div>
      <div class="csk-group">
        <div class="csk-group__head">
          <span class="csk-group__title">消息间距</span>
          <n-switch size="small" :value="has('spacing')" @update:value="toggleGroup('spacing', $event)" />
        </div>
        <div v-if="s.spacing" class="csk-grid">
          <label class="csk-field">
            <span>上方 {{ s.spacing.top }}px</span>
            <n-slider v-model:value="s.spacing.top" :min="0" :max="64" />
          </label>
          <label class="csk-field">
            <span>下方 {{ s.spacing.bottom }}px</span>
            <n-slider v-model:value="s.spacing.bottom" :min="0" :max="64" />
          </label>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.csk-style { display: flex; flex-direction: column; gap: 10px; }
.csk-style.is-readonly { pointer-events: none; opacity: .6; }
.csk-style__tabs { align-self: flex-start; }
.csk-group { display: flex; flex-direction: column; gap: 8px; padding: 10px 12px; border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, .24)); border-radius: 8px; background: var(--sc-bg-surface, transparent); }
.csk-group__head { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.csk-group__title { font-size: 13px; font-weight: 600; }
.csk-group__hint { font-size: 12px; color: var(--sc-text-secondary, #64748b); }
.csk-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px 12px; }
.csk-grid--4 { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.csk-field { display: flex; flex-direction: column; gap: 4px; min-width: 0; font-size: 12px; color: var(--sc-text-secondary, #64748b); }
.csk-field--wide { grid-column: 1 / -1; }
.csk-field--inline { justify-content: flex-end; }
.csk-field__row { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.csk-image-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.csk-image-row__thumb { width: 44px; height: 30px; flex: none; display: flex; align-items: center; justify-content: center; border-radius: 4px; border: 1px dashed var(--sc-border-strong, rgba(148, 163, 184, .5)); background: center / cover no-repeat; font-size: 10px; color: var(--sc-text-secondary, #64748b); }
.csk-image-row__actions { display: flex; gap: 4px; }
.csk-image-row__select { width: 104px; }
@media (max-width: 480px) {
  .csk-grid { grid-template-columns: minmax(0, 1fr); }
  .csk-grid--4 { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
