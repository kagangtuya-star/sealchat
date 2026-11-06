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
const baseRef = ref<HTMLElement | null>(null);
// Playback callbacks compare the original engine by identity; deep refs proxy it and drop every state update.
const playback = shallowRef<ReturnType<typeof createTwinLayerPlayback> | null>(null);
const visibleText = ref('');
const waiting = ref(false);
const playing = ref(false);
const completed = ref(false);
// 逻辑完成仍按引擎通知父级；仅切层样式等待实际进入动画收尾。
let visualCompleted = false;
let skipEnterAnimations = false;
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
  root.classList.toggle('is-completed', visualCompleted);
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

const prepareBasePerformanceDom = () => {
  const root = baseRef.value;
  if (!root) return;
  root.querySelectorAll<HTMLElement>('.tiptap-performance').forEach((performance) => {
    if (performance.dataset.performanceCharsPrepared === 'true') return;
    performance.dataset.performanceCharsPrepared = 'true';
    const walker = document.createTreeWalker(performance, NodeFilter.SHOW_TEXT);
    const textNodes: Text[] = [];
    let current = walker.nextNode();
    while (current) {
      const parent = current.parentElement;
      if (parent && !parent.closest('rt') && parent.closest('.tiptap-performance') === performance) {
        textNodes.push(current as Text);
      }
      current = walker.nextNode();
    }
    let charIndex = 0;
    const renderedChars: HTMLElement[] = [];
    textNodes.forEach((node) => {
      const fragment = document.createDocumentFragment();
      const segments = node.data.match(/[A-Za-z0-9]+(?:['’-][A-Za-z0-9]+)*|\s+|./gu) || [];
      segments.forEach((segment) => {
        if (/^\s+$/u.test(segment)) {
          fragment.append(segment);
          return;
        }
        const span = document.createElement('span');
        span.className = 'twin-layer-message__base-char';
        span.style.setProperty('--performance-char-index', String(charIndex));
        span.textContent = segment;
        renderedChars.push(span);
        charIndex += 1;
        fragment.appendChild(span);
      });
      node.replaceWith(fragment);
    });
    renderedChars[renderedChars.length - 1]?.classList.add('twin-layer-message__base-char--tail');
  });
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
  visualCompleted = true;
  syncDom();
};

const finishOverlay = async (engine: ReturnType<typeof createTwinLayerPlayback>) => {
  if (!skipEnterAnimations) {
    const animations = overlayTextRef.value?.getAnimations?.({ subtree: true }).filter((animation) => (
      typeof CSSAnimation !== 'undefined'
      && animation instanceof CSSAnimation
      && animation.animationName.startsWith('sc-performance-enter-')
      && animation.playState === 'running'
      && Number.isFinite(animation.effect?.getComputedTiming().endTime)
    )) || [];
    // 只等待实际运行中的进入动画；暂停、取消或不支持 CSSAnimation 的环境直接回退切层。
    await Promise.allSettled(animations.map((animation) => animation.finished));
  }
  if (engine !== playback.value) return;
  visualCompleted = true;
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
    void finishOverlay(engine);
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
  visualCompleted = false;
  skipEnterAnimations = false;
  syncDom();
  await startPlayback();
};

const skip = () => {
  debug('skip');
  skipEnterAnimations = true;
  if (playback.value?.getState() === 'completed') {
    visualCompleted = true;
    syncDom();
    return;
  }
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
  prepareBasePerformanceDom();
  if (!props.autoplay) {
    disposePlayback();
    renderFinalOverlay();
    return;
  }
  void replay();
};

watch(() => [props.content, props.autoplay] as const, ([content, autoplay], [previousContent]) => {
  if (!mounted.value) {
    return;
  }
  // 剧场模式收到逻辑完成后会关闭 autoplay，保留本轮 overlay 的视觉收尾。
  if (content === previousContent && !autoplay && playback.value?.getState() === 'completed') {
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
    <div ref="baseRef" class="twin-layer-message__base" v-html="baseHtml"></div>
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

/* 演出结构属于功能样式：主题可以改字体/颜色，但不应破坏逐字动画节点。 */
:root[data-sc-surface] .tiptap-performance,
.twin-layer-message__overlay-text .tiptap-performance,
.twin-layer-message__base .tiptap-performance {
  display: inline-block !important;
  transform-origin: center !important;
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

:root[data-sc-surface] .twin-layer-message__char {
  display: inline-block !important;
  transform-origin: center !important;
}

:root[data-sc-surface] .twin-layer-message__char-glyph {
  display: inline-block !important;
  transform-origin: center !important;
}

:root[data-sc-surface] .twin-layer-message__base .twin-layer-message__base-char {
  display: inline-block !important;
  transform-origin: center !important;
}

.twin-layer-message__char--code {
  border-radius: 0.25em;
  padding: 0.02em 0.22em;
  background: var(--chat-inline-code-bg, rgba(148, 163, 184, 0.18));
  color: var(--chat-inline-code-fg, inherit);
  font-family: var(--sc-code-font, ui-monospace, SFMono-Regular, Consolas, monospace);
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-wave .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-wave,
:root[data-sc-surface] .twin-layer-message .fx-wave {
  animation: sc-performance-wave 2.6s cubic-bezier(.45,.05,.2,1) infinite !important;
  animation-delay: calc(var(--performance-char-index) * -150ms) !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-shake .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-shake,
:root[data-sc-surface] .twin-layer-message .fx-shake {
  animation: sc-performance-shake 0.24s linear infinite !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-rainbow .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-rainbow,
:root[data-sc-surface] .twin-layer-message .fx-rainbow {
  animation: sc-performance-rainbow 1.8s linear infinite !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-glitch .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-glitch,
:root[data-sc-surface] .twin-layer-message .fx-glitch {
  animation: sc-performance-glitch 0.65s steps(2, end) infinite !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-blink .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-blink,
:root[data-sc-surface] .twin-layer-message .fx-blink {
  animation: sc-performance-blink 1.6s ease-in-out infinite !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-glow .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-glow,
:root[data-sc-surface] .twin-layer-message .fx-glow {
  animation: sc-performance-glow 2.8s ease-in-out infinite !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-pulse .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-pulse,
:root[data-sc-surface] .twin-layer-message .fx-pulse {
  animation: sc-performance-pulse 1.8s ease-in-out infinite !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-float .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-float,
:root[data-sc-surface] .twin-layer-message .fx-float {
  animation: sc-performance-float 3.2s ease-in-out infinite !important;
  animation-delay: calc(var(--performance-char-index) * -110ms) !important;
}

/* 摇摆以字符底部为支点；选择器需压过 base 层 .tiptap-performance 的 transform-origin，且只作用于 sway。 */
:root[data-sc-surface] .twin-layer-message__char-glyph.fx-sway,
:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-sway .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-sway {
  transform-origin: center bottom !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-sway .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-sway,
:root[data-sc-surface] .twin-layer-message .fx-sway {
  animation: sc-performance-sway 2.6s ease-in-out infinite !important;
  animation-delay: calc(var(--performance-char-index) * -90ms) !important;
}

/* 心跳整段同步，不做逐字错相，避免变成波浪式缩放。 */
:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-heartbeat .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-heartbeat,
:root[data-sc-surface] .twin-layer-message .fx-heartbeat {
  animation: sc-performance-heartbeat 1.8s ease-out infinite !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-wobble .twin-layer-message__base-char,
:root[data-sc-surface] .tiptap-performance.fx-wobble,
:root[data-sc-surface] .twin-layer-message .fx-wobble {
  animation: sc-performance-wobble 1.8s ease-in-out infinite !important;
  animation-delay: calc(var(--performance-char-index) * -70ms) !important;
}

/* 段尾字形只保留轻微位移/旋转，避免非等比缩放让标点或复杂汉字显得变形。 */
:root[data-sc-surface] .twin-layer-message__base .tiptap-performance.fx-wobble .twin-layer-message__base-char--tail {
  animation-name: sc-performance-wobble-tail !important;
}

:root[data-sc-surface] .tiptap-performance.enter-blur,
:root[data-sc-surface] .twin-layer-message .enter-blur {
  animation: sc-performance-enter-blur 0.42s ease-out both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-typewriter,
:root[data-sc-surface] .twin-layer-message .enter-typewriter {
  animation: sc-performance-enter-typewriter calc(140ms + (10 - var(--performance-enter-speed, 5)) * 26ms) cubic-bezier(.17,.84,.44,1) both !important;
}

/* 进入动画挂在字符外层 span，持续效果挂在内层 glyph，两者 transform 逐层叠加。 */
:root[data-sc-surface] .tiptap-performance.enter-fade,
:root[data-sc-surface] .twin-layer-message .enter-fade {
  animation: sc-performance-enter-fade calc(220ms + (10 - var(--performance-enter-speed, 5)) * 34ms) ease-out both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-rise,
:root[data-sc-surface] .twin-layer-message .enter-rise {
  animation: sc-performance-enter-rise calc(240ms + (10 - var(--performance-enter-speed, 5)) * 34ms) cubic-bezier(.17,.84,.44,1) both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-drop,
:root[data-sc-surface] .twin-layer-message .enter-drop {
  animation: sc-performance-enter-drop calc(260ms + (10 - var(--performance-enter-speed, 5)) * 34ms) cubic-bezier(.3,.7,.4,1) both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-zoom,
:root[data-sc-surface] .twin-layer-message .enter-zoom {
  animation: sc-performance-enter-zoom calc(240ms + (10 - var(--performance-enter-speed, 5)) * 30ms) cubic-bezier(.2,.8,.3,1) both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-tracking,
:root[data-sc-surface] .twin-layer-message .enter-tracking {
  animation: sc-performance-enter-tracking calc(280ms + (10 - var(--performance-enter-speed, 5)) * 36ms) cubic-bezier(.17,.84,.44,1) both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-flash,
:root[data-sc-surface] .twin-layer-message .enter-flash {
  animation: sc-performance-enter-flash calc(260ms + (10 - var(--performance-enter-speed, 5)) * 30ms) linear both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-glitch,
:root[data-sc-surface] .twin-layer-message .enter-glitch {
  animation: sc-performance-enter-glitch calc(280ms + (10 - var(--performance-enter-speed, 5)) * 30ms) steps(1, end) both !important;
}

:root[data-sc-surface] .tiptap-performance.enter-slam,
:root[data-sc-surface] .twin-layer-message .enter-slam {
  animation: sc-performance-enter-slam calc(180ms + (10 - var(--performance-enter-speed, 5)) * 22ms) both !important;
}

:root[data-sc-surface] .twin-layer-message__base .tiptap-performance {
  display: inline !important;
  animation: none !important;
  transform: none !important;
}

@keyframes sc-performance-wave {
  0%, 100% { transform: var(--performance-scale) translateY(0.04em) skewX(0deg) scaleY(1); }
  18% { transform: var(--performance-scale) translateY(-0.06em) skewX(calc(var(--performance-tone-skew) * 0.04)) scaleY(1.01); }
  38% { transform: var(--performance-scale) translateY(-0.22em) skewX(calc(var(--performance-tone-skew) * 0.1)) scaleY(1.04); }
  58% { transform: var(--performance-scale) translateY(-0.12em) skewX(calc(var(--performance-tone-skew) * 0.05)) scaleY(1.02); }
  78% { transform: var(--performance-scale) translateY(0.12em) skewX(calc(var(--performance-tone-skew) * -0.09)) scaleY(0.97); }
}

@keyframes sc-performance-shake {
  0%, 100% { transform: var(--performance-scale) translateX(0); }
  25% { transform: var(--performance-scale) translateX(-0.04em); }
  75% { transform: var(--performance-scale) translateX(0.04em); }
}

@keyframes sc-performance-rainbow {
  0% { color: #ef4444; }
  25% { color: #f59e0b; }
  50% { color: #10b981; }
  75% { color: #3b82f6; }
  100% { color: #ef4444; }
}

@keyframes sc-performance-glitch {
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

@keyframes sc-performance-blink {
  0%, 14%, 32%, 100% { opacity: 1; filter: brightness(var(--performance-tone-brightness)); }
  18% { opacity: 0.62; filter: brightness(calc(var(--performance-tone-brightness) * 0.9)); }
  24% { opacity: 0.96; filter: brightness(calc(var(--performance-tone-brightness) * 1.04)); }
  40%, 74% { opacity: 0.28; filter: brightness(calc(var(--performance-tone-brightness) * 0.82)); }
  82% { opacity: 0.88; filter: brightness(calc(var(--performance-tone-brightness) * 1.06)); }
}

@keyframes sc-performance-enter-blur {
  from { opacity: 0; filter: blur(8px); }
  to { opacity: 1; filter: blur(0); }
}

@keyframes sc-performance-enter-typewriter {
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

@keyframes sc-performance-glow {
  0%, 100% {
    text-shadow: 0 0 0.1em color-mix(in srgb, currentColor 30%, transparent);
  }
  50% {
    text-shadow:
      0 0 0.16em color-mix(in srgb, currentColor 65%, transparent),
      0 0 0.42em color-mix(in srgb, currentColor 35%, transparent);
  }
}

@keyframes sc-performance-pulse {
  0%, 100% { opacity: 1; transform: var(--performance-scale) scale(1); }
  50% { opacity: 0.8; transform: var(--performance-scale) scale(1.06); }
}

@keyframes sc-performance-float {
  0%, 100% { transform: var(--performance-scale) translate(0, 0); }
  25% { transform: var(--performance-scale) translate(0.02em, -0.06em); }
  50% { transform: var(--performance-scale) translate(0, -0.12em); }
  75% { transform: var(--performance-scale) translate(-0.02em, -0.06em); }
}

@keyframes sc-performance-sway {
  0%, 100% { transform: var(--performance-scale) translateX(0) rotate(0deg); }
  25% { transform: var(--performance-scale) translate(0.015em, -0.02em) rotate(2deg); }
  50% { transform: var(--performance-scale) translateX(0) rotate(0deg); }
  75% { transform: var(--performance-scale) translate(-0.015em, -0.02em) rotate(-2deg); }
}

@keyframes sc-performance-heartbeat {
  0%, 55%, 100% { transform: var(--performance-scale) scale(1); }
  8% { transform: var(--performance-scale) scale(1.12); }
  14% { transform: var(--performance-scale) scale(1); }
  22% { transform: var(--performance-scale) scale(1.07); }
  30% { transform: var(--performance-scale) scale(1); }
}

@keyframes sc-performance-wobble {
  0%, 100% { transform: var(--performance-scale) rotate(0deg) scale(1, 1); }
  20% { transform: var(--performance-scale) translateX(0.01em) rotate(-1.5deg) scale(1.04, 0.96); }
  40% { transform: var(--performance-scale) translateY(-0.01em) rotate(1deg) scale(0.97, 1.05); }
  60% { transform: var(--performance-scale) rotate(0deg) scale(1, 1); }
  80% { transform: var(--performance-scale) translateX(-0.01em) rotate(1.5deg) scale(0.96, 1.04); }
}

@keyframes sc-performance-wobble-tail {
  0%, 100% { transform: var(--performance-scale) rotate(0deg); }
  20% { transform: var(--performance-scale) translateX(0.005em) rotate(-0.45deg); }
  40% { transform: var(--performance-scale) translateY(-0.005em) rotate(0.35deg); }
  60% { transform: var(--performance-scale) rotate(0deg); }
  80% { transform: var(--performance-scale) translateX(-0.005em) rotate(0.45deg); }
}

@keyframes sc-performance-enter-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes sc-performance-enter-rise {
  from { opacity: 0; transform: var(--performance-scale) translateY(0.5em); }
  to { opacity: 1; transform: var(--performance-scale); }
}

@keyframes sc-performance-enter-drop {
  0% { opacity: 0; transform: var(--performance-scale) translateY(-0.7em); }
  65% { opacity: 1; transform: var(--performance-scale) translateY(0.06em); }
  100% { opacity: 1; transform: var(--performance-scale); }
}

@keyframes sc-performance-enter-zoom {
  0% { opacity: 0; transform: var(--performance-scale) scale(0.35); }
  70% { opacity: 1; transform: var(--performance-scale) scale(1.08); }
  100% { opacity: 1; transform: var(--performance-scale); }
}

/* 只位移字符本身，不改真实 letter-spacing，避免播放中整行重排。 */
@keyframes sc-performance-enter-tracking {
  from {
    opacity: 0;
    transform: var(--performance-scale) translateX(var(--performance-tracking-offset, 0.2em));
  }
  to { opacity: 1; transform: var(--performance-scale); }
}

@keyframes sc-performance-enter-flash {
  0% { opacity: 0; }
  18% { opacity: 1; text-shadow: 0 0 0.32em currentColor; }
  36% { opacity: 0.25; }
  56% { opacity: 1; }
  74% { opacity: 0.7; }
  100% { opacity: 1; }
}

@keyframes sc-performance-enter-glitch {
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

@keyframes sc-performance-enter-slam {
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
