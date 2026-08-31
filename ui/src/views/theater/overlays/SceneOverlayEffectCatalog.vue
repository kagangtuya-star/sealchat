<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NIcon } from 'naive-ui'
import { Plus } from '@vicons/tabler'

import type { StageSceneOverlayBinding } from '../shared/stage-types'
import { registerBuiltInSceneOverlayEffects } from './effects'
import { createSceneOverlayBinding, listSceneOverlayEffects } from './scene-overlay-registry'
import type { SceneOverlayCategory } from './scene-overlay-types'

const emit = defineEmits<{
  add: [binding: StageSceneOverlayBinding]
}>()

registerBuiltInSceneOverlayEffects()

const categoryLabels: Record<SceneOverlayCategory, string> = {
  weather: '天气',
  environment: '环境',
  lighting: '光照',
  special: '特殊',
}
const categories: SceneOverlayCategory[] = ['weather', 'environment', 'lighting', 'special']
const groupedEffects = computed(() => categories.map((category) => ({
  category,
  label: categoryLabels[category],
  effects: listSceneOverlayEffects().filter((definition) => definition.category === category),
})))
</script>

<template>
  <div class="scene-overlay-catalog">
    <section v-for="group in groupedEffects" :key="group.category" class="scene-overlay-catalog__group">
      <h3>{{ group.label }}</h3>
      <div class="scene-overlay-catalog__items">
        <n-button
          v-for="definition in group.effects"
          :key="definition.id"
          size="small"
          secondary
          @click="emit('add', createSceneOverlayBinding(definition.id))"
        >
          <template #icon><n-icon><Plus /></n-icon></template>
          <span>{{ definition.name }}</span>
          <small v-if="definition.description">{{ definition.description }}</small>
        </n-button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.scene-overlay-catalog { max-height: 300px; overflow: auto; padding: 8px; border-bottom: 1px solid var(--theater-border); }
.scene-overlay-catalog__group + .scene-overlay-catalog__group { margin-top: 9px; }
.scene-overlay-catalog__group h3 { margin: 0 0 5px; color: var(--sc-text-secondary); font-size: 11px; }
.scene-overlay-catalog__items { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 5px; }
.scene-overlay-catalog__items :deep(.n-button) { height: auto; min-height: 36px; justify-content: flex-start; padding: 5px 8px; text-align: left; white-space: normal; }
.scene-overlay-catalog__items :deep(.n-button__content) { min-width: 0; display: grid; justify-items: start; line-height: 1.25; }
.scene-overlay-catalog__items small { color: var(--sc-text-secondary); font-size: 9px; }
</style>
