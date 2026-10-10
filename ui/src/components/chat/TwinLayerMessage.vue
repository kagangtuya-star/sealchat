<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import DOMPurify from 'dompurify';
import { tiptapJsonToHtml } from '@/utils/tiptap-render';
import { hasPerformanceContent, parsePerformanceInstructions } from '@/utils/tiptap-performance-parser';
import { isAnimatedPerformanceEnterMode, resolvePerformanceEnterClassNames } from '@/utils/tiptap-performance-mark';
import { createTwinLayerPlayback } from './twinLayerPlayback';
import type { TwinLayerPlaybackChar } from './twinLayerPlayback';

const props = withDefaults(defineProps<{
  content: string
  autoplay?: boolean
  debugPlayback?: boolean
  charactersPerSecond?: number
  baseUrl?: string
  imageClass?: string
  linkClass?: string
  attachmentResolver?: (src: string) => string
}>(), {
  content: '',
  autoplay: false,
  debugPlayback: false,
  baseUrl: '',
  imageClass: 'inline-image',
  linkClass: 'text-blue-500',
});

const emit = defineEmits<{
  (event: 'state-change', value: { waiting: boolean; playing: boolean; completed: boolean }): void
  (event: 'completed'): void
}>();

const hostRef = ref<HTMLElement | null>(null);
// Playback callbacks compare the original engine by identity; deep refs proxy it and drop every state update.
const playback = shallowRef<ReturnType<typeof createTwinLayerPlayback> | null>(null);
const visibleText = ref('');
const waiting = ref(false);
const playing = ref(false);
const completed = ref(false);
const overlayTextRef = ref<HTMLElement | null>(null);
const mounted = ref(false);
let trackingSegmentIndex = 0;

const debug = (event: string, detail?: Record<string, unknown>) => {
  if (!props.debugPlayback) return;
  console.info(`[theater-dialogue] player.${event}`, detail || {});
};

const parsedDoc = computed(() => {
  if (!props.content) {
    return null;
  }
  try {
    const doc = JSON.parse(props.content);
    return hasPerformanceContent(doc) ? doc : null;
  } catch {
    return null;
  }
});

const instructions = computed(() => {
  const doc = parsedDoc.value;
  if (!doc) {
    return [];
  }
  return parsePerformanceInstructions(doc);
});

const hasBlurBackdrop = computed(() => instructions.value.some((entry) => (
  entry.type === 'char' && entry.effects.enterMode === 'blur'
)));

// 除朦胧显现外的逐字进入效果都从空白开始，播放期间隐藏底层静态文本。
const hasHiddenBackdrop = computed(() => instructions.value.some((entry) => (
  entry.type === 'char'
  && entry.effects.enterMode !== 'blur'
  && isAnimatedPerformanceEnterMode(entry.effects.enterMode)
)));

const baseHtml = computed(() => {
  if (!props.content) {
    return '';
  }
  return DOMPurify.sanitize(tiptapJsonToHtml(props.content, {
    baseUrl: props.baseUrl,
    imageClass: props.imageClass,
    linkClass: props.linkClass,
    attachmentResolver: props.attachmentResolver,
  }));
});

const syncDom = () => {
  const root = hostRef.value;
  if (!root) return;
  root.classList.toggle('is-waiting', waiting.value);
  root.classList.toggle('is-playing', playing.value);
  root.classList.toggle('is-completed', completed.value);
  root.classList.toggle('has-blur-backdrop', hasBlurBackdrop.value);
  root.classList.toggle('has-hidden-backdrop', hasHiddenBackdrop.value);
};

const clearOverlayDom = () => {
  visibleText.value = '';
  trackingSegmentIndex = 0;
  if (overlayTextRef.value) {
    overlayTextRef.value.textContent = '';
  }
};

const appendTextDecoration = (span: HTMLElement, value: string) => {
  const current = span.style.textDecoration;
  span.style.textDecoration = current ? `${current} ${value}` : value;
};

const applyTextStyleAttrs = (span: HTMLElement, attrs: Record<string, any>) => {
  const fontSize = String(attrs.fontSize || '').trim();
  const color = String(attrs.color || '').trim();
  const fontFamily = String(attrs.fontFamily || attrs.platformFontFamily || '').trim();
  const fontAssetId = String(attrs.fontAssetId || '').trim();
  const platformFontFamily = String(attrs.platformFontFamily || '').trim();
  if (fontSize) {
    span.style.fontSize = fontSize;
  }
  if (color) {
    span.style.color = color;
  }
  if (fontFamily) {
    span.style.fontFamily = fontFamily;
  }
  if (fontAssetId) {
    span.dataset.platformFontId = fontAssetId;
  }
  if (platformFontFamily) {
    span.dataset.platformFontFamily = platformFontFamily;
  }
  const toneIntensity = Number(attrs.toneIntensity);
  if (Number.isFinite(toneIntensity)) {
    span.style.setProperty('--performance-tone-intensity', String(toneIntensity));
  }
};

const applyVisualMarks = (span: HTMLElement, marks: TwinLayerPlaybackChar['marks'] = []) => {
  marks.forEach((mark) => {
    const attrs = mark.attrs || {};
    switch (mark.type) {
      case 'bold':
        span.style.fontWeight = '700';
        break;
      case 'italic':
        span.style.fontStyle = 'italic';
        break;
      case 'underline':
        appendTextDecoration(span, 'underline');
        break;
      case 'strike':
        appendTextDecoration(span, 'line-through');
        break;
      case 'code':
        span.classList.add('twin-layer-message__char--code');
        break;
      case 'highlight': {
        const color = String(attrs.color || '').trim();
        if (color) {
          span.style.backgroundColor = color;
        }
        break;
      }
      case 'spoiler':
        span.classList.add('tiptap-spoiler');
        break;
      case 'ruby':
        span.classList.add('tiptap-ruby');
        if (attrs.rubyText) {
          span.dataset.rubyText = String(attrs.rubyText);
        }
        if (attrs.rubyBaseFontFamily || attrs.rubyFontFamily) {
          span.style.setProperty('--ruby-base-font-family', String(attrs.rubyBaseFontFamily || attrs.rubyFontFamily));
        }
        if (attrs.rubyRtFontFamily || attrs.rubyFontFamily) {
          span.style.setProperty('--ruby-rt-font-family', String(attrs.rubyRtFontFamily || attrs.rubyFontFamily));
        }
        if (attrs.rubyBaseFontSize || attrs.rubyFontSize) {
          span.style.setProperty('--ruby-base-font-size', String(attrs.rubyBaseFontSize || attrs.rubyFontSize));
        }
        if (attrs.rubyRtFontSize || attrs.rubyFontSize) {
          span.style.setProperty('--ruby-rt-font-size', String(attrs.rubyRtFontSize || attrs.rubyFontSize));
        }
        if (attrs.rubyColor) {
          span.style.setProperty('--ruby-color', String(attrs.rubyColor));
        }
        if (attrs.rubyFontWeight) {
          span.style.setProperty('--ruby-font-weight', String(attrs.rubyFontWeight));
        }
        if (attrs.rubyFontStyle) {
          span.style.setProperty('--ruby-font-style', String(attrs.rubyFontStyle));
        }
        if (attrs.rubyTextDecoration) {
          span.style.setProperty('--ruby-text-decoration', String(attrs.rubyTextDecoration));
        }
        if (attrs.rubyBackgroundColor) {
          span.style.setProperty('--ruby-background-color', String(attrs.rubyBackgroundColor));
        }
        if (attrs.rubySpoiler === 'true') {
          span.dataset.rubySpoiler = 'true';
        }
        break;
      case 'textStyle':
        applyTextStyleAttrs(span, attrs);
        break;
      case 'performance':
        span.classList.add('tiptap-performance', ...resolvePerformanceEnterClassNames(attrs.enterMode));
        if (Number.isFinite(Number(attrs.enterSpeed))) {
          span.style.setProperty('--performance-enter-speed', String(Number(attrs.enterSpeed)));
        }
        if (Number.isFinite(Number(attrs.toneIntensity))) {
          span.style.setProperty('--performance-tone-intensity', String(Number(attrs.toneIntensity)));
        }
        break;
    }
  });
};

const appendChar = (entry: TwinLayerPlaybackChar) => {
  visibleText.value += entry.char;
  const host = overlayTextRef.value;
  if (!host) {
    return;
  }
  const span = document.createElement('span');
  span.className = 'twin-layer-message__char';
  applyVisualMarks(span, entry.marks);
  span.style.setProperty('--performance-char-index', String(entry.index));
  const glyph = document.createElement('span');
  glyph.className = 'twin-layer-message__char-glyph';
  if (entry.effects.effect) {
    glyph.classList.add(`fx-${entry.effects.effect}`);
  }
  span.classList.add(...resolvePerformanceEnterClassNames(entry.effects.enterMode));
  if (entry.effects.enterMode === 'tracking') {
    const trackingOffset = Math.min(0.2 + trackingSegmentIndex * 0.06, 1.2);
    span.style.setProperty('--performance-tracking-offset', `${trackingOffset.toFixed(2)}em`);
    trackingSegmentIndex += 1;
  } else {
    trackingSegmentIndex = 0;
  }
  if (entry.effects.scale) {
    span.classList.add(`scale-${entry.effects.scale}`);
  }
  if (Number.isFinite(Number(entry.effects.toneIntensity))) {
    span.style.setProperty('--performance-tone-intensity', String(Number(entry.effects.toneIntensity)));
  }
  glyph.textContent = entry.char;
  span.appendChild(glyph);
  host.appendChild(span);
};

const appendBreak = () => {
  visibleText.value += '\n';
  trackingSegmentIndex = 0;
  const host = overlayTextRef.value;
  if (!host) {
    return;
  }
  host.appendChild(document.createElement('br'));
};

const renderFinalOverlay = () => {
  clearOverlayDom();
  instructions.value.forEach((entry) => {
    if (entry.type === 'char') {
      appendChar(entry);
      return;
    }
    if (entry.type === 'break') {
      appendBreak();
    }
  });
  waiting.value = false;
  playing.value = false;
  completed.value = true;
  syncDom();
};

const refreshState = (engine = playback.value) => {
  if (!engine || engine !== playback.value) return;
  waiting.value = engine.isWaiting();
  playing.value = engine.getState() === 'playing';
  completed.value = engine.getState() === 'completed';
  visibleText.value = engine.getVisibleText();
  syncDom();
  emit('state-change', {
    waiting: waiting.value,
    playing: playing.value,
    completed: completed.value,
  });
  debug('state', {
    state: engine.getState(),
    waiting: waiting.value,
    playing: playing.value,
    completed: completed.value,
    visibleLength: Array.from(visibleText.value).length,
  });
};

const startPlayback = async () => {
  if (!parsedDoc.value) {
    visibleText.value = '';
    syncDom();
    return;
  }
  let engine!: ReturnType<typeof createTwinLayerPlayback>;
  engine = createTwinLayerPlayback(instructions.value, {
    charactersPerSecond: props.charactersPerSecond,
    onChar: (entry) => {
      appendChar(entry);
    },
    onBreak: appendBreak,
    onStateChange: () => refreshState(engine),
  });
  playback.value = engine;
  debug('start', { instructionCount: instructions.value.length });
  await engine.play();
  if (engine === playback.value && engine.getState() === 'completed') {
    debug('completed');
    emit('completed');
  }
  refreshState(engine);
};

const disposePlayback = () => {
  const engine = playback.value;
  playback.value = null;
  engine?.dispose();
};

const replay = async () => {
  disposePlayback();
  clearOverlayDom();
  syncDom();
  await startPlayback();
};

const skip = () => {
  debug('skip');
  playback.value?.skip();
  refreshState();
};

defineExpose({ skip, replay });

const handleOverlayClick = () => {
  if (waiting.value) {
    playback.value?.continuePlayback();
  }
};

const renderCurrentContent = () => {
  if (!props.autoplay) {
    disposePlayback();
    renderFinalOverlay();
    return;
  }
  void replay();
};

watch(() => [props.content, props.autoplay], () => {
  if (!mounted.value) {
    return;
  }
  void nextTick(renderCurrentContent);
});

watch(() => props.charactersPerSecond, (charactersPerSecond) => {
  playback.value?.setCharactersPerSecond(charactersPerSecond);
});

onMounted(() => {
  mounted.value = true;
  void nextTick(renderCurrentContent);
});

onBeforeUnmount(() => {
  disposePlayback();
});
</script>

<template>
  <div ref="hostRef" class="twin-layer-message">
    <div class="twin-layer-message__base" v-html="baseHtml"></div>
    <div class="twin-layer-message__overlay" aria-hidden="true" @click.stop="handleOverlayClick">
      <span ref="overlayTextRef" class="twin-layer-message__overlay-text"></span>
    </div>
  </div>
</template>

<style>
.twin-layer-message {
  position: relative;
}

.twin-layer-message__base {
  opacity: 0.25;
  filter: blur(0.6px);
  user-select: none;
  pointer-events: none;
  transition: opacity 180ms ease, filter 180ms ease;
}

.twin-layer-message.has-blur-backdrop:not(.is-completed) .twin-layer-message__base {
  opacity: 0.16;
  filter: blur(3px);
}

.twin-layer-message.has-hidden-backdrop:not(.is-completed) .twin-layer-message__base {
  opacity: 0;
  filter: none;
}

.twin-layer-message.has-hidden-backdrop:not(.is-completed) .twin-layer-message__base .tiptap-performance {
  opacity: 0;
}

.twin-layer-message.is-completed .twin-layer-message__base {
  opacity: 1;
  filter: none;
  pointer-events: auto;
  user-select: auto;
}

.twin-layer-message__overlay {
  position: absolute;
  inset: 0;
  pointer-events: auto;
  transition: opacity 180ms ease;
}

.twin-layer-message.is-completed .twin-layer-message__overlay {
  opacity: 0;
  pointer-events: none;
}

.twin-layer-message.is-waiting .twin-layer-message__overlay {
  background:
    radial-gradient(circle at center, color-mix(in srgb, var(--primary-color, #60a5fa) 18%, transparent), transparent 68%),
    linear-gradient(90deg, transparent, color-mix(in srgb, var(--primary-color, #60a5fa) 8%, transparent), transparent);
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--primary-color, #60a5fa) 28%, transparent),
    inset 0 0 2.4rem color-mix(in srgb, var(--primary-color, #60a5fa) 10%, transparent);
}

.twin-layer-message__overlay-text {
  white-space: pre-wrap;
}

.twin-layer-message__overlay-text .tiptap-performance,
.twin-layer-message__base .tiptap-performance {
  display: inline-block;
  transform-origin: center;
  --performance-scale: scale(1);
  --performance-tone-intensity: 0;
  --performance-char-index: 0;
  --performance-tone-weight: clamp(320, calc(500 + var(--performance-tone-intensity) * 110), 920);
  --performance-tone-spacing: clamp(-0.03em, calc(var(--performance-tone-intensity) * 0.012em), 0.08em);
  --performance-tone-skew: calc(var(--performance-tone-intensity) * 0.7deg);
  --performance-tone-brightness: clamp(0.82, calc(1 + var(--performance-tone-intensity) * 0.04), 1.18);
  font-weight: var(--performance-tone-weight);
  letter-spacing: var(--performance-tone-spacing);
  filter: brightness(var(--performance-tone-brightness));
}

.twin-layer-message__char {
  display: inline-block;
  transform-origin: center;
}

.twin-layer-message__char-glyph {
  display: inline-block;
  transform-origin: center;
}

.twin-layer-message__char--code {
  border-radius: 0.25em;
  padding: 0.02em 0.22em;
  background: var(--chat-inline-code-bg, rgba(148, 163, 184, 0.18));
  color: var(--chat-inline-code-fg, inherit);
  font-family: var(--sc-code-font, ui-monospace, SFMono-Regular, Consolas, monospace);
}

.fx-wave {
  animation: performance-wave 2.6s cubic-bezier(.45,.05,.2,1) infinite;
  animation-delay: calc(var(--performance-char-index) * -150ms);
}

.fx-shake {
  animation: performance-shake 0.24s linear infinite;
}

.fx-rainbow {
  animation: performance-rainbow 1.8s linear infinite;
}

.fx-glitch {
  animation: performance-glitch 0.65s steps(2, end) infinite;
}

.fx-blink {
  animation: performance-blink 1.6s ease-in-out infinite;
}

.fx-glow {
  animation: performance-glow 2.8s ease-in-out infinite;
}

.fx-pulse {
  animation: performance-pulse 1.8s ease-in-out infinite;
}

.fx-float {
  animation: performance-float 3.2s ease-in-out infinite;
  animation-delay: calc(var(--performance-char-index) * -110ms);
}

/* 摇摆以字符底部为支点；选择器需压过 base 层 .tiptap-performance 的 transform-origin，且只作用于 sway。 */
.twin-layer-message__char-glyph.fx-sway,
.tiptap-performance.fx-sway {
  transform-origin: center bottom;
}

.fx-sway {
  animation: performance-sway 2.6s ease-in-out infinite;
  animation-delay: calc(var(--performance-char-index) * -90ms);
}

/* 心跳整段同步，不做逐字错相，避免变成波浪式缩放。 */
.fx-heartbeat {
  animation: performance-heartbeat 1.8s ease-out infinite;
}

.fx-wobble {
  animation: performance-wobble 1.8s ease-in-out infinite;
  animation-delay: calc(var(--performance-char-index) * -70ms);
}

.enter-blur {
  animation: performance-enter-blur 0.42s ease-out both;
}

.enter-typewriter {
  animation: performance-enter-typewriter calc(140ms + (10 - var(--performance-enter-speed, 5)) * 26ms) cubic-bezier(.17,.84,.44,1) both;
}

/* 进入动画挂在字符外层 span，持续效果挂在内层 glyph，两者 transform 逐层叠加。 */
.enter-fade {
  animation: performance-enter-fade calc(220ms + (10 - var(--performance-enter-speed, 5)) * 34ms) ease-out both;
}

.enter-rise {
  animation: performance-enter-rise calc(240ms + (10 - var(--performance-enter-speed, 5)) * 34ms) cubic-bezier(.17,.84,.44,1) both;
}

.enter-drop {
  animation: performance-enter-drop calc(260ms + (10 - var(--performance-enter-speed, 5)) * 34ms) cubic-bezier(.3,.7,.4,1) both;
}

.enter-zoom {
  animation: performance-enter-zoom calc(240ms + (10 - var(--performance-enter-speed, 5)) * 30ms) cubic-bezier(.2,.8,.3,1) both;
}

.enter-tracking {
  animation: performance-enter-tracking calc(280ms + (10 - var(--performance-enter-speed, 5)) * 36ms) cubic-bezier(.17,.84,.44,1) both;
}

.enter-flash {
  animation: performance-enter-flash calc(260ms + (10 - var(--performance-enter-speed, 5)) * 30ms) linear both;
}

.enter-glitch {
  animation: performance-enter-glitch calc(280ms + (10 - var(--performance-enter-speed, 5)) * 30ms) steps(1, end) both;
}

.enter-slam {
  animation: performance-enter-slam calc(180ms + (10 - var(--performance-enter-speed, 5)) * 22ms) both;
}

@keyframes performance-wave {
  0%, 100% { transform: var(--performance-scale) translateY(0.04em) skewX(0deg) scaleY(1); }
  18% { transform: var(--performance-scale) translateY(-0.06em) skewX(calc(var(--performance-tone-skew) * 0.04)) scaleY(1.01); }
  38% { transform: var(--performance-scale) translateY(-0.22em) skewX(calc(var(--performance-tone-skew) * 0.1)) scaleY(1.04); }
  58% { transform: var(--performance-scale) translateY(-0.12em) skewX(calc(var(--performance-tone-skew) * 0.05)) scaleY(1.02); }
  78% { transform: var(--performance-scale) translateY(0.12em) skewX(calc(var(--performance-tone-skew) * -0.09)) scaleY(0.97); }
}

@keyframes performance-shake {
  0%, 100% { transform: var(--performance-scale) translateX(0); }
  25% { transform: var(--performance-scale) translateX(-0.04em); }
  75% { transform: var(--performance-scale) translateX(0.04em); }
}

@keyframes performance-rainbow {
  0% { color: #ef4444; }
  25% { color: #f59e0b; }
  50% { color: #10b981; }
  75% { color: #3b82f6; }
  100% { color: #ef4444; }
}

@keyframes performance-glitch {
  0%, 100% {
    transform: var(--performance-scale) translate(0) skewX(0deg);
    text-shadow: none;
    filter: brightness(var(--performance-tone-brightness));
  }
  16% {
    transform: var(--performance-scale) translate(-0.05em, 0.01em) skewX(-6deg);
    text-shadow:
      0.05em 0 0 rgba(255, 59, 59, 0.72),
      -0.03em 0 0 rgba(80, 180, 255, 0.72);
  }
  32% {
    transform: var(--performance-scale) translate(0.04em, -0.02em) skewX(4deg);
    text-shadow:
      -0.06em 0 0 rgba(255, 59, 59, 0.78),
      0.04em 0 0 rgba(80, 180, 255, 0.78);
    filter: brightness(1.28) contrast(1.18);
  }
  48% {
    transform: var(--performance-scale) translate(-0.02em, 0.03em) skewX(-3deg);
    text-shadow:
      0.02em -0.01em 0 rgba(255, 255, 255, 0.3),
      -0.04em 0 0 rgba(255, 59, 59, 0.62);
  }
  64% {
    transform: var(--performance-scale) translate(0.06em, 0) skewX(7deg);
    text-shadow:
      -0.07em 0 0 rgba(255, 59, 59, 0.86),
      0.06em 0 0 rgba(80, 180, 255, 0.86),
      0 0 0.24em rgba(255, 255, 255, 0.22);
    filter: brightness(1.34) contrast(1.24);
  }
  82% {
    transform: var(--performance-scale) translate(-0.03em, -0.01em) skewX(-5deg);
    text-shadow:
      0.04em 0 0 rgba(255, 59, 59, 0.74),
      -0.05em 0 0 rgba(80, 180, 255, 0.74);
  }
}

@keyframes performance-blink {
  0%, 14%, 32%, 100% { opacity: 1; filter: brightness(var(--performance-tone-brightness)); }
  18% { opacity: 0.62; filter: brightness(calc(var(--performance-tone-brightness) * 0.9)); }
  24% { opacity: 0.96; filter: brightness(calc(var(--performance-tone-brightness) * 1.04)); }
  40%, 74% { opacity: 0.28; filter: brightness(calc(var(--performance-tone-brightness) * 0.82)); }
  82% { opacity: 0.88; filter: brightness(calc(var(--performance-tone-brightness) * 1.06)); }
}

@keyframes performance-enter-blur {
  from { opacity: 0; filter: blur(8px); }
  to { opacity: 1; filter: blur(0); }
}

@keyframes performance-enter-typewriter {
  0% {
    opacity: 0;
    transform: var(--performance-scale);
    filter: brightness(var(--performance-tone-brightness));
  }
  100% {
    opacity: 1;
    transform: var(--performance-scale);
    filter: brightness(var(--performance-tone-brightness));
  }
}

@keyframes performance-glow {
  0%, 100% {
    text-shadow: 0 0 0.1em color-mix(in srgb, currentColor 30%, transparent);
  }
  50% {
    text-shadow:
      0 0 0.16em color-mix(in srgb, currentColor 65%, transparent),
      0 0 0.42em color-mix(in srgb, currentColor 35%, transparent);
  }
}

@keyframes performance-pulse {
  0%, 100% { opacity: 1; transform: var(--performance-scale) scale(1); }
  50% { opacity: 0.8; transform: var(--performance-scale) scale(1.06); }
}

@keyframes performance-float {
  0%, 100% { transform: var(--performance-scale) translate(0, 0); }
  25% { transform: var(--performance-scale) translate(0.02em, -0.06em); }
  50% { transform: var(--performance-scale) translate(0, -0.12em); }
  75% { transform: var(--performance-scale) translate(-0.02em, -0.06em); }
}

@keyframes performance-sway {
  0%, 100% { transform: var(--performance-scale) translateX(0) rotate(0deg); }
  25% { transform: var(--performance-scale) translate(0.015em, -0.02em) rotate(2deg); }
  50% { transform: var(--performance-scale) translateX(0) rotate(0deg); }
  75% { transform: var(--performance-scale) translate(-0.015em, -0.02em) rotate(-2deg); }
}

@keyframes performance-heartbeat {
  0%, 55%, 100% { transform: var(--performance-scale) scale(1); }
  8% { transform: var(--performance-scale) scale(1.12); }
  14% { transform: var(--performance-scale) scale(1); }
  22% { transform: var(--performance-scale) scale(1.07); }
  30% { transform: var(--performance-scale) scale(1); }
}

@keyframes performance-wobble {
  0%, 100% { transform: var(--performance-scale) rotate(0deg) scale(1, 1); }
  20% { transform: var(--performance-scale) translateX(0.01em) rotate(-1.5deg) scale(1.04, 0.96); }
  40% { transform: var(--performance-scale) translateY(-0.01em) rotate(1deg) scale(0.97, 1.05); }
  60% { transform: var(--performance-scale) rotate(0deg) scale(1, 1); }
  80% { transform: var(--performance-scale) translateX(-0.01em) rotate(1.5deg) scale(0.96, 1.04); }
}

@keyframes performance-enter-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes performance-enter-rise {
  from { opacity: 0; transform: var(--performance-scale) translateY(0.5em); }
  to { opacity: 1; transform: var(--performance-scale); }
}

@keyframes performance-enter-drop {
  0% { opacity: 0; transform: var(--performance-scale) translateY(-0.7em); }
  65% { opacity: 1; transform: var(--performance-scale) translateY(0.06em); }
  100% { opacity: 1; transform: var(--performance-scale); }
}

@keyframes performance-enter-zoom {
  0% { opacity: 0; transform: var(--performance-scale) scale(0.35); }
  70% { opacity: 1; transform: var(--performance-scale) scale(1.08); }
  100% { opacity: 1; transform: var(--performance-scale); }
}

/* 只位移字符本身，不改真实 letter-spacing，避免播放中整行重排。 */
@keyframes performance-enter-tracking {
  from {
    opacity: 0;
    transform: var(--performance-scale) translateX(var(--performance-tracking-offset, 0.2em));
  }
  to { opacity: 1; transform: var(--performance-scale); }
}

@keyframes performance-enter-flash {
  0% { opacity: 0; }
  18% { opacity: 1; text-shadow: 0 0 0.32em currentColor; }
  36% { opacity: 0.25; }
  56% { opacity: 1; }
  74% { opacity: 0.7; }
  100% { opacity: 1; }
}

@keyframes performance-enter-glitch {
  0% {
    opacity: 0;
    transform: var(--performance-scale) translateX(-0.14em) skewX(-14deg);
  }
  12% {
    opacity: 1;
    transform: var(--performance-scale) translateX(0.1em) skewX(10deg);
    text-shadow:
      -0.06em 0 0 rgba(255, 59, 59, 0.8),
      0.06em 0 0 rgba(80, 180, 255, 0.8);
  }
  28% {
    opacity: 0.35;
    transform: var(--performance-scale) translate(-0.06em, 0.03em) skewX(-6deg);
    text-shadow:
      0.05em 0 0 rgba(255, 59, 59, 0.7),
      -0.04em 0 0 rgba(80, 180, 255, 0.7);
  }
  44% {
    opacity: 1;
    transform: var(--performance-scale) translateX(0.05em);
    text-shadow: 0 0 0 transparent;
  }
  62% {
    opacity: 0.6;
    transform: var(--performance-scale) translateX(-0.03em) skewX(4deg);
  }
  80%, 100% {
    opacity: 1;
    transform: var(--performance-scale);
  }
}

@keyframes performance-enter-slam {
  0% {
    opacity: 0;
    transform: var(--performance-scale) scale(2.4);
    animation-timing-function: cubic-bezier(.55, 0, 1, .45);
  }
  45% {
    opacity: 1;
    transform: var(--performance-scale) scale(0.9);
    animation-timing-function: cubic-bezier(.2, .7, .4, 1);
  }
  70% { transform: var(--performance-scale) scale(1.05); }
  100% { opacity: 1; transform: var(--performance-scale); }
}
</style>
