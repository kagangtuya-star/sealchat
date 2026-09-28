<script setup lang="ts">
import { computed } from 'vue'
import { speechPlayer } from './player'
import { isDisplayVoiceTag, voiceKindLabel, voiceLanguageLabel, voiceSourceLabel, type VoiceCatalogItem } from './voice-catalog'

const props = defineProps<{ item: VoiceCatalogItem; selected: boolean; showModel?: boolean }>()
const emit = defineEmits<{ select: [item: VoiceCatalogItem] }>()
const previewing = computed(() => !!props.item.previewResourceId && speechPlayer.state.key === `resources:${props.item.previewResourceId}`)
// Hide provider age metadata and keep the actual style/use-case tags visible on cards.
const chips = computed(() => [
  ...props.item.languages.map(voiceLanguageLabel),
  ...props.item.tags.filter(isDisplayVoiceTag),
].slice(0, 4))
// Only an existing preview file is replayed here; it never submits synthesis.
function preview() {
  if (props.item.previewResourceId) void speechPlayer.play('resources', props.item.previewResourceId)
}
</script>

<template>
  <article class="voice-card" :class="{ 'is-selected': selected, 'is-unavailable': !item.available }">
    <button
      type="button"
      class="voice-card__select"
      :disabled="!item.available"
      :aria-pressed="selected"
      :aria-label="`选择音色 ${item.name}（${voiceSourceLabel(item)}）`"
      @click="emit('select', item)"
    />
    <div class="voice-card__head">
      <strong class="voice-card__name">{{ item.name }}</strong>
      <span class="voice-card__source">{{ voiceSourceLabel(item) }}</span>
    </div>
    <p v-if="item.description" class="voice-card__desc">{{ item.description }}</p>
    <div v-if="chips.length" class="voice-card__tags">
      <span
        v-for="(chip, index) in chips"
        :key="index"
        class="voice-card__tag"
        :class="{ 'is-language': index < item.languages.length }"
      >{{ chip }}</span>
    </div>
    <div class="voice-card__foot">
      <span class="voice-card__meta">
        {{ voiceKindLabel(item.kind) }}<template v-if="showModel"> · {{ item.modelId }}</template><template v-if="!item.available"> · 当前不可用</template>
      </span>
      <button
        v-if="item.previewResourceId"
        type="button"
        class="voice-card__preview"
        :aria-label="previewing ? `停止试听 ${item.name}` : `免费重放 ${item.name} 的已有试听`"
        @click="preview"
      >{{ previewing ? (speechPlayer.state.loading ? '加载中…' : '停止') : '免费试听' }}</button>
    </div>
  </article>
</template>

<style scoped>
.voice-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  padding: 8px 12px;
  border: 1px solid var(--sc-border-mute);
  border-radius: 8px;
  background: var(--vp-soft);
  color: var(--sc-text-primary);
  transition: border-color .15s ease, background-color .15s ease;
}
.voice-card:hover:not(.is-unavailable) { border-color: var(--sc-border-strong); }
.voice-card.is-selected {
  border-color: var(--vp-accent);
  background: color-mix(in srgb, var(--vp-accent) 12%, transparent);
}
.voice-card.is-unavailable { opacity: .55; }
/* The select button covers the card; content ignores pointer events so a
   click anywhere selects, while the preview button sits above it. */
.voice-card__select {
  position: absolute;
  inset: 0;
  padding: 0;
  border: 0;
  border-radius: inherit;
  background: transparent;
  cursor: pointer;
}
.voice-card__select:disabled { cursor: not-allowed; }
.voice-card__select:focus-visible { outline: 2px solid var(--vp-accent); outline-offset: 2px; }
.voice-card > :not(.voice-card__select) { position: relative; pointer-events: none; }
.voice-card__head { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; min-width: 0; }
.voice-card__name { overflow: hidden; font-size: 14px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.voice-card__source { flex: none; font-size: 12px; color: var(--sc-text-secondary); }
.is-selected .voice-card__source { color: var(--vp-accent); }
.voice-card__desc {
  margin: 0;
  overflow: hidden;
  font-size: 12px;
  line-height: 1.5;
  color: var(--sc-text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.voice-card__tags {
  display: flex;
  flex-wrap: wrap;
  align-content: flex-start;
  gap: 4px 6px;
  min-width: 0;
  max-height: 40px;
  overflow: hidden;
}
.voice-card__tag {
  flex: none;
  max-width: 100%;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--vp-soft);
  font-size: 12px;
  line-height: 18px;
  color: var(--sc-text-secondary);
  white-space: nowrap;
}
.voice-card__tag.is-language { color: var(--sc-text-primary); }
.voice-card__foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; margin-top: auto; }
.voice-card__meta { overflow: hidden; font-size: 12px; color: var(--sc-text-secondary); text-overflow: ellipsis; white-space: nowrap; }
.voice-card .voice-card__preview {
  flex: none;
  padding: 2px 10px;
  border: 1px solid var(--sc-border-strong);
  border-radius: 4px;
  background: transparent;
  color: var(--sc-text-primary);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  pointer-events: auto;
}
.voice-card__preview:hover { border-color: var(--vp-accent); color: var(--vp-accent); }
.voice-card__preview:focus-visible { outline: 2px solid var(--vp-accent); outline-offset: 1px; }
</style>
