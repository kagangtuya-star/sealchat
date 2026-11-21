<script setup lang="ts">
import { computed } from 'vue'
import type { ChatSkinLayerRender } from './chatSkinRender'

// 装饰图层只负责视觉：整层 pointer-events: none、不可选中、对读屏隐藏，
// 不会拦截引用、TTS、编辑、撤回、图片点击、骰子与文本选择等任何交互。
const props = defineProps<{ layers: ChatSkinLayerRender[] }>()
// 裁切会建立层叠上下文，前后景分别承载，保持原有的 surface 前后关系。
const groups = computed(() => [false, true]
  .map(front => ({ front, layers: props.layers.filter(layer => layer.front === front) }))
  .filter(group => group.layers.length))
</script>

<template>
  <span v-for="group in groups" :key="String(group.front)" class="chat-skin-layers"
    :class="group.front ? 'chat-skin-layers--front' : 'chat-skin-layers--back'" aria-hidden="true">
    <span
      v-for="layer in group.layers"
      :key="layer.key"
      :data-layer-id="layer.key"
      class="chat-skin-layer"
      :class="[`chat-skin-layer--${layer.kind}`, layer.front ? 'chat-skin-layer--front' : 'chat-skin-layer--back']"
      :style="layer.style"
    >
      <img
        v-if="layer.kind === 'image'"
        class="chat-skin-layer__img"
        :src="layer.src"
        alt=""
        draggable="false"
        decoding="async"
        loading="lazy"
      >
    </span>
  </span>
</template>

<style>
.chat-skin-layers {
  position: absolute;
  inset: 0;
  overflow: clip;
  overflow-clip-margin: 96px;
  clip-path: inset(calc(-1 * var(--sc-skin-reserve-top, 0px)) -96px calc(-1 * var(--sc-skin-reserve-bottom, 0px)) -96px);
  pointer-events: none;
  user-select: none;
  -webkit-user-select: none;
}

.chat-skin-layers--front { z-index: 1; }
.chat-skin-layers--back { z-index: -1; }

.chat-skin-layer {
  position: absolute;
  display: block;
  pointer-events: none;
}

.chat-skin-layer__img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
}
</style>
