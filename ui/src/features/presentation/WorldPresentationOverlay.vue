<script setup lang="ts">
import { urlBase } from '@/stores/_config'
import TheaterEffectOverlay from '@/views/theater/effects/TheaterEffectOverlay.vue'
import type { TheaterEffectPlayback } from '@/views/theater/effects/theater-effect-runtime'
import type { StageImageRef } from '@/views/theater/shared/stage-types'
import { resolveTheaterStageMediaLocation } from '@/views/theater/stage/stage-media'

// Elevated overlays sit above open consoles and modals so admins can see what they play.
const props = defineProps<{ worldId: string, playbacks: TheaterEffectPlayback[], elevated?: boolean }>()
const resolveMedia = (image: StageImageRef) => resolveTheaterStageMediaLocation(image, {
  worldId: props.worldId,
  channelId: '',
  scopeType: 'world',
  urlBase,
})
</script>

<template>
  <Teleport to="body">
    <div v-if="playbacks.length" class="world-presentation-overlay" :class="{ 'is-elevated': elevated }" aria-hidden="true" inert>
      <TheaterEffectOverlay :playbacks="playbacks" :resolve-media="resolveMedia" />
    </div>
  </Teleport>
</template>

<style scoped>
.world-presentation-overlay {
  position: fixed;
  inset: 0;
  z-index: 900;
  pointer-events: none;
}
.world-presentation-overlay.is-elevated { z-index: 4000; }
</style>
