<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { NButton, NEmpty, NIcon, NInput, NRadioButton, NRadioGroup, NSpin } from 'naive-ui'
import { Code, Edit, ListDetails, Photo, PlayerPlay, Search, Stars } from '@vicons/tabler'
import { theaterEffectConfigFromObject, type TheaterEffectKind } from '@/views/theater/effects/theater-effect-types'
import type { TheaterSequence } from '@/views/theater/sequences/theater-sequence-types'
import type { StageObject } from '@/views/theater/shared/stage-types'
import type { WorldPresentationConfig } from './world-presentation-config'
import { canPlayWorldSequence, type WorldPresentationTarget } from './world-presentation-runtime'

// Live control lists the SAVED config: server playback resolves targets by revision.
const props = defineProps<{
  config: WorldPresentationConfig | null
  failed: boolean
  play: (kind: WorldPresentationTarget, id: string) => Promise<boolean>
}>()

const emit = defineEmits<{
  edit: [kind: WorldPresentationTarget, id: string]
  retry: []
}>()

const search = ref('')
const filter = ref<'all' | WorldPresentationTarget>('all')
const states = ref<Record<string, 'pending' | 'sent'>>({})
const timers = new Map<string, ReturnType<typeof setTimeout>>()

const kindIcons: Record<TheaterEffectKind, typeof Stars> = { builtin: Stars, media: Photo, web: Code }
const kindLabels: Record<TheaterEffectKind, string> = { builtin: '内置', media: '媒体', web: '网页' }

const keywordText = (keywords: string[]) => (keywords.length
  ? `「${keywords.slice(0, 3).join(' / ')}${keywords.length > 3 ? ' …' : ''}」`
  : '仅手动')

const effectCards = computed(() => (props.config?.effects ?? []).filter((effect) => effect.visible).map((effect: StageObject) => {
  const config = theaterEffectConfigFromObject(effect)
  return {
    id: effect.id,
    name: effect.name || '未命名特效',
    icon: kindIcons[config.kind],
    meta: `${kindLabels[config.kind]} · ${(config.durationMs / 1000).toFixed(1)} 秒`,
    trigger: keywordText(config.keywords),
    search: [effect.name, ...config.keywords].join('\n').toLocaleLowerCase(),
  }
}))

const sequenceCards = computed(() => {
  const config = props.config
  if (!config) return []
  return config.sequences.filter((sequence) => canPlayWorldSequence(sequence, config)).map((sequence: TheaterSequence) => {
    const keywords = sequence.triggers.flatMap((trigger) => (trigger.type === 'message' ? trigger.keywords : []))
    return {
      id: sequence.id,
      name: sequence.name || '未命名序列',
      meta: `${sequence.steps.length} 步${sequence.loopCount > 1 ? ` · 循环 ${sequence.loopCount} 轮` : ''}`,
      trigger: keywordText(keywords),
      search: [sequence.name, ...keywords].join('\n').toLocaleLowerCase(),
    }
  })
})

const hiddenCount = computed(() => {
  const config = props.config
  if (!config) return 0
  return config.effects.length - effectCards.value.length + config.sequences.length - sequenceCards.value.length
})

const matches = (card: { search: string }) => {
  const keyword = search.value.trim().toLocaleLowerCase()
  return !keyword || card.search.includes(keyword)
}
const visibleEffects = computed(() => (filter.value === 'sequence' ? [] : effectCards.value.filter(matches)))
const visibleSequences = computed(() => (filter.value === 'effect' ? [] : sequenceCards.value.filter(matches)))
const total = computed(() => effectCards.value.length + sequenceCards.value.length)

const trigger = async (kind: WorldPresentationTarget, id: string) => {
  const key = `${kind}:${id}`
  if (states.value[key] === 'pending') return
  clearTimeout(timers.get(key))
  states.value = { ...states.value, [key]: 'pending' }
  const ok = await props.play(kind, id)
  const next = { ...states.value }
  if (ok) {
    next[key] = 'sent'
    timers.set(key, setTimeout(() => {
      timers.delete(key)
      const rest = { ...states.value }
      delete rest[key]
      states.value = rest
    }, 1600))
  } else {
    delete next[key]
  }
  states.value = next
}

onBeforeUnmount(() => {
  timers.forEach((timer) => clearTimeout(timer))
  timers.clear()
})
</script>

<template>
  <div class="wp-live">
    <div class="wp-live__toolbar">
      <n-input v-model:value="search" size="small" clearable placeholder="搜索名称或关键词" class="wp-live__search">
        <template #prefix><n-icon><Search /></n-icon></template>
      </n-input>
      <n-radio-group v-model:value="filter" size="small">
        <n-radio-button value="all">全部 {{ total }}</n-radio-button>
        <n-radio-button value="effect">特效 {{ effectCards.length }}</n-radio-button>
        <n-radio-button value="sequence">序列 {{ sequenceCards.length }}</n-radio-button>
      </n-radio-group>
    </div>

    <div class="wp-live__body">
      <n-empty v-if="failed" description="无法读取已保存的演出配置">
        <template #extra><n-button size="small" @click="emit('retry')">重试</n-button></template>
      </n-empty>
      <div v-else-if="!config" class="wp-live__center"><n-spin size="small" /></div>
      <n-empty v-else-if="!total" description="还没有可现场播放的特效或序列">
        <template #extra><span class="wp-live__hint">在「特效」或「序列」中新建并保存后，会出现在这里。</span></template>
      </n-empty>
      <n-empty v-else-if="!visibleEffects.length && !visibleSequences.length" description="没有匹配的条目" />
      <template v-else>
        <section v-if="visibleEffects.length" class="wp-live__section" aria-label="特效">
          <h3>特效</h3>
          <div class="wp-live__grid">
            <article
              v-for="card in visibleEffects"
              :key="card.id"
              class="wp-live-card"
              :class="states[`effect:${card.id}`] && `is-${states[`effect:${card.id}`]}`"
            >
              <button type="button" class="wp-live-card__play" :title="`现场播放：${card.name}`" @click="trigger('effect', card.id)">
                <span class="wp-live-card__icon"><n-icon :component="card.icon" /></span>
                <span class="wp-live-card__text">
                  <strong>{{ card.name }}</strong>
                  <small>{{ card.meta }}</small>
                  <small class="wp-live-card__trigger">{{ card.trigger }}</small>
                </span>
                <span class="wp-live-card__go">
                  <n-icon><PlayerPlay /></n-icon>
                  <em>{{ states[`effect:${card.id}`] === 'sent' ? '已播放' : states[`effect:${card.id}`] === 'pending' ? '发送中' : '播放' }}</em>
                </span>
              </button>
              <button type="button" class="wp-live-card__edit" :aria-label="`编辑特效 ${card.name}`" title="编辑" @click="emit('edit', 'effect', card.id)"><n-icon><Edit /></n-icon></button>
            </article>
          </div>
        </section>

        <section v-if="visibleSequences.length" class="wp-live__section" aria-label="序列">
          <h3>序列</h3>
          <div class="wp-live__grid">
            <article
              v-for="card in visibleSequences"
              :key="card.id"
              class="wp-live-card is-sequence"
              :class="states[`sequence:${card.id}`] && `is-${states[`sequence:${card.id}`]}`"
            >
              <button type="button" class="wp-live-card__play" :title="`现场播放：${card.name}`" @click="trigger('sequence', card.id)">
                <span class="wp-live-card__icon"><n-icon><ListDetails /></n-icon></span>
                <span class="wp-live-card__text">
                  <strong>{{ card.name }}</strong>
                  <small>{{ card.meta }}</small>
                  <small class="wp-live-card__trigger">{{ card.trigger }}</small>
                </span>
                <span class="wp-live-card__go">
                  <n-icon><PlayerPlay /></n-icon>
                  <em>{{ states[`sequence:${card.id}`] === 'sent' ? '已播放' : states[`sequence:${card.id}`] === 'pending' ? '发送中' : '播放' }}</em>
                </span>
              </button>
              <button type="button" class="wp-live-card__edit" :aria-label="`编辑序列 ${card.name}`" title="编辑" @click="emit('edit', 'sequence', card.id)"><n-icon><Edit /></n-icon></button>
            </article>
          </div>
        </section>
      </template>
      <p v-if="config && hiddenCount" class="wp-live__footnote">另有 {{ hiddenCount }} 个已停用或没有可播放步骤的条目未列出。</p>
    </div>
  </div>
</template>

<style scoped>
.wp-live { min-height: 0; display: flex; flex: 1; flex-direction: column; }
.wp-live__toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 14px 20px 10px; }
.wp-live__search { min-width: 180px; flex: 1 1 240px; max-width: 420px; }
.wp-live__body { min-height: 0; flex: 1; overflow: auto; padding: 4px 20px 20px; }
.wp-live__center { display: grid; place-items: center; padding: 48px; }
.wp-live__hint { color: var(--sc-text-secondary); font-size: 12px; }
.wp-live__section + .wp-live__section { margin-top: 18px; }
.wp-live__section h3 { margin: 4px 0 10px; color: var(--sc-text-secondary); font-size: 12px; font-weight: 600; letter-spacing: .06em; }
.wp-live__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 10px; }
.wp-live__footnote { margin: 18px 0 0; color: var(--sc-text-secondary); font-size: 12px; }

.wp-live-card { --card-accent: var(--wp-accent); position: relative; min-width: 0; border: 1px solid var(--sc-border-strong); border-radius: 10px; background: var(--sc-bg-elevated); transition: border-color .15s, box-shadow .15s, background-color .15s; }
.wp-live-card.is-sequence { --card-accent: var(--wp-accent-alt); }
.wp-live-card:hover { border-color: color-mix(in srgb, var(--card-accent) 55%, transparent); box-shadow: 0 4px 16px color-mix(in srgb, var(--card-accent) 14%, transparent); }
.wp-live-card:focus-within { border-color: var(--card-accent); }
.wp-live-card.is-sent { border-color: var(--card-accent); background: color-mix(in srgb, var(--card-accent) 10%, var(--sc-bg-elevated)); }
.wp-live-card__play { width: 100%; min-height: 76px; display: grid; grid-template-columns: 38px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 12px 40px 12px 12px; border: 0; border-radius: inherit; color: inherit; background: transparent; text-align: left; font: inherit; cursor: pointer; }
.wp-live-card__play:focus-visible { outline: 2px solid var(--card-accent); outline-offset: 2px; }
.wp-live-card__icon { width: 38px; height: 38px; display: grid; place-items: center; border-radius: 9px; color: var(--card-accent); background: color-mix(in srgb, var(--card-accent) 14%, transparent); font-size: 20px; }
.wp-live-card__text { min-width: 0; display: grid; gap: 3px; }
.wp-live-card__text strong { display: -webkit-box; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow-wrap: anywhere; font-size: 14px; line-height: 1.35; }
.wp-live-card__text small { overflow: hidden; color: var(--sc-text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.wp-live-card__trigger { opacity: .85; }
.wp-live-card__go { display: grid; justify-items: center; gap: 2px; color: var(--card-accent); font-size: 22px; }
.wp-live-card__go em { font-size: 11px; font-style: normal; }
.wp-live-card.is-pending .wp-live-card__go { opacity: .6; }
.wp-live-card__edit { position: absolute; top: 6px; right: 6px; width: 28px; height: 28px; display: grid; place-items: center; border: 0; border-radius: 6px; color: var(--sc-text-secondary); background: transparent; cursor: pointer; opacity: .7; }
.wp-live-card__edit:hover, .wp-live-card__edit:focus-visible { color: var(--sc-text-primary); background: var(--sc-bg-hover); opacity: 1; }

@media (max-width: 640px) {
  .wp-live__toolbar { padding: 12px 12px 8px; }
  .wp-live__search { max-width: none; flex-basis: 100%; }
  .wp-live__body { padding: 4px 12px 16px; }
  .wp-live__grid { grid-template-columns: minmax(0, 1fr); }
}
</style>
