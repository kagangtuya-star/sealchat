<script setup lang="ts">
import { computed } from 'vue'

import { THEATER_EFFECT_WEB_SANDBOX, buildTheaterEffectWebSrcdoc } from './theater-effect-web'

// One instance per effect playback: the overlay keys it by playback instance, so a
// replay mounts a fresh browsing context and unmounting destroys the page together
// with every timer or animation it started. No messaging channel is exposed.
const props = defineProps<{
  html: string
}>()

const srcdoc = computed(() => buildTheaterEffectWebSrcdoc(props.html))
</script>

<template>
  <iframe
    v-if="srcdoc"
    class="theater-effect-web-frame"
    :srcdoc="srcdoc"
    :sandbox="THEATER_EFFECT_WEB_SANDBOX"
    referrerpolicy="no-referrer"
    title="网页代码特效"
    aria-hidden="true"
    tabindex="-1"
  />
</template>

<style scoped>
/* Non-interactive: pointer and wheel input fall through to the stage. A matching
   color-scheme keeps the frame canvas transparent under dark themes. */
.theater-effect-web-frame { width: 100%; height: 100%; display: block; border: 0; background: transparent; color-scheme: normal; pointer-events: none; }
</style>
