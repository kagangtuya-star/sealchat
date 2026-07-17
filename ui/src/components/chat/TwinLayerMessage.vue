<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import DOMPurify from 'dompurify';
import { tiptapJsonToHtml } from '@/utils/tiptap-render';
import { hasPerformanceContent, parsePerformanceInstructions } from '@/utils/tiptap-performance-parser';
import { createTwinLayerPlayback } from './twinLayerPlayback';
import type { TwinLayerPlaybackChar } from './twinLayerPlayback';

const props = withDefaults(defineProps<{
  content: string
  autoplay?: boolean
  baseUrl?: string
  imageClass?: string
  linkClass?: string
  attachmentResolver?: (src: string) => string
}>(), {
  content: '',
  autoplay: false,
  baseUrl: '',
  imageClass: 'inline-image',
  linkClass: 'text-blue-500',
});

const emit = defineEmits<{
  (event: 'state-change', value: { waiting: boolean; playing: boolean; completed: boolean }): void
}>();

const hostRef = ref<HTMLElement | null>(null);
const playback = ref<ReturnType<typeof createTwinLayerPlayback> | null>(null);
const visibleText = ref('');
const waiting = ref(false);
const playing = ref(false);
const completed = ref(false);
const overlayTextRef = ref<HTMLElement | null>(null);
const mounted = ref(false);

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

const hasTypewriterEnter = computed(() => instructions.value.some((entry) => (
  entry.type === 'char' && entry.effects.enterMode === 'typewriter'
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
  root.classList.toggle('has-typewriter', hasTypewriterEnter.value);
};

const clearOverlayDom = () => {
  visibleText.value = '';
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
        span.classList.add('tiptap-performance');
        if (attrs.enterMode) {
          span.classList.add(`enter-${String(attrs.enterMode)}`);
        }
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
  if (entry.effects.enterMode) {
    span.classList.add(`enter-${entry.effects.enterMode}`);
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

const refreshState = () => {
  const engine = playback.value;
  if (!engine) {
    waiting.value = false;
    playing.value = false;
    completed.value = false;
    return;
  }
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
};

const startPlayback = async () => {
  if (!parsedDoc.value) {
    visibleText.value = '';
    syncDom();
    return;
  }
  const engine = createTwinLayerPlayback(instructions.value, {
    onChar: (entry) => {
      appendChar(entry);
    },
    onBreak: appendBreak,
    onStateChange: refreshState,
  });
  playback.value = engine;
  await engine.play();
  refreshState();
};

const replay = async () => {
  playback.value?.dispose();
  playback.value = null;
  clearOverlayDom();
  syncDom();
  await startPlayback();
};

const skip = () => {
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
    playback.value?.dispose();
    playback.value = null;
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

onMounted(() => {
  mounted.value = true;
  void nextTick(renderCurrentContent);
});

onBeforeUnmount(() => {
  playback.value?.dispose();
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

.twin-layer-message.has-typewriter:not(.is-completed) .twin-layer-message__base {
  opacity: 0;
  filter: none;
}

.twin-layer-message.has-typewriter:not(.is-completed) .twin-layer-message__base .tiptap-performance {
  opacity: 0;
}

.twin-layer-message.is-completed .twin-layer-message__base {
  opacity: 1;
  filter: none;
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

.enter-blur {
  animation: performance-enter-blur 0.42s ease-out both;
}

.enter-typewriter {
  animation: performance-enter-typewriter calc(140ms + (10 - var(--performance-enter-speed, 5)) * 26ms) cubic-bezier(.17,.84,.44,1) both;
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
</style>
