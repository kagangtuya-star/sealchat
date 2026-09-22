<script setup lang="ts">
import { computed } from 'vue';
import { useGlassBackground, useGlassBackgroundRuntime } from '@/composables/useGlassBackground';
import { useWorldGlassBackgroundRuntime } from '@/composables/useWorldGlassBackground';
import { buildBackgroundImageStyle, buildBackgroundOverlayStyle } from '@/utils/backgroundPresentation';

const { effectiveSettings: settings } = useGlassBackground();
const presentation = computed(() => ({
  mode: settings.value.mode, opacity: settings.value.backgroundOpacity, blur: settings.value.backgroundBlur,
  brightness: settings.value.backgroundBrightness, overlayColor: settings.value.overlayColor, overlayOpacity: settings.value.overlayOpacity,
}));
useGlassBackgroundRuntime();
useWorldGlassBackgroundRuntime();
const imageStyle = computed(() => ({
  ...buildBackgroundImageStyle(settings.value.attachmentId, presentation.value),
  inset: `${-settings.value.backgroundBlur * 3}px`,
}));
const overlayStyle = computed(() => buildBackgroundOverlayStyle(presentation.value));
</script>

<template>
  <div v-if="settings.enabled && settings.attachmentId" class="sc-global-glass-background" aria-hidden="true">
    <div class="sc-global-glass-background__image" :style="imageStyle" />
    <div v-if="overlayStyle" class="sc-global-glass-background__overlay" :style="overlayStyle" />
  </div>
</template>

<style scoped>
.sc-global-glass-background { position: fixed; inset: 0; z-index: -1; overflow: hidden; pointer-events: none; }
.sc-global-glass-background__image, .sc-global-glass-background__overlay { position: absolute; inset: 0; }
</style>
