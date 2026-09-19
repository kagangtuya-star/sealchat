<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { NIcon } from 'naive-ui';
import { Copy, Download } from '@vicons/tabler';
import Viewer from 'viewerjs';
import 'viewerjs/dist/viewer.css';

const props = defineProps<{
  blob: Blob;
  objectUrl: string;
  logicalWidth: number;
  fileName: string;
  showMessage: (type: 'success' | 'error', content: string) => void;
}>();

const emit = defineEmits<{
  (event: 'close'): void;
}>();

const imageRef = ref<HTMLImageElement | null>(null);
const copying = ref(false);
let imageViewer: Viewer | null = null;

const destroyViewer = () => {
  imageViewer?.destroy();
  imageViewer = null;
};

onMounted(async () => {
  await nextTick();
  if (!imageRef.value) return;
  imageViewer = new Viewer(imageRef.value, {
    navbar: false,
    title: false,
    toolbar: {
      zoomIn: true,
      zoomOut: true,
      oneToOne: true,
      reset: true,
      prev: false,
      play: false,
      next: false,
      rotateLeft: true,
      rotateRight: true,
      flipHorizontal: false,
      flipVertical: false,
    },
    tooltip: true,
    movable: true,
    zoomable: true,
    scalable: true,
    rotatable: true,
    transition: true,
    fullscreen: true,
    keyboard: true,
    zIndex: 3000,
  });
});

onBeforeUnmount(() => {
  destroyViewer();
});

const handleSave = () => {
  try {
    const link = document.createElement('a');
    link.href = props.objectUrl;
    link.download = props.fileName;
    link.style.display = 'none';
    document.body.appendChild(link);
    link.click();
    link.remove();
  } catch (error) {
    console.error('Failed to save the generated message image', error);
    props.showMessage('error', '保存图片失败，请长按或右键图片保存。');
  }
};

type ClipboardItemConstructorWithSupports = typeof ClipboardItem & {
  supports?: (type: string) => boolean;
};

const supportsDirectImageCopy = () => {
  if (typeof ClipboardItem === 'undefined' || typeof navigator.clipboard?.write !== 'function') {
    return false;
  }
  const clipboardItem = ClipboardItem as ClipboardItemConstructorWithSupports;
  if (typeof clipboardItem.supports === 'function') {
    try {
      if (clipboardItem.supports('image/png') === false) return false;
    } catch (error) {
      console.warn('ClipboardItem.supports failed; attempting clipboard write directly', error);
    }
  }
  return true;
};

const handleCopy = async () => {
  if (!supportsDirectImageCopy()) {
    props.showMessage('error', '当前浏览器不支持直接复制图片，请使用保存到本地或长按/右键图片。');
    return;
  }

  copying.value = true;
  try {
    await navigator.clipboard.write([
      new ClipboardItem({
        'image/png': props.blob,
      }),
    ]);
    props.showMessage('success', '图片已复制到剪贴板');
  } catch (error) {
    console.error('Failed to copy the generated message image', error);
    props.showMessage('error', '图片已生成，但复制到剪贴板失败，请使用保存到本地或长按/右键图片。');
  } finally {
    copying.value = false;
  }
};
</script>

<template>
  <div
    class="message-image-export-preview"
    role="dialog"
    aria-modal="true"
    aria-label="消息图片预览"
    :style="{ '--preview-image-half-width': `${props.logicalWidth / 2}px` }"
    @click.self="emit('close')"
  >
    <div class="message-image-export-preview__stage" @click.self="emit('close')">
      <div class="message-image-export-preview__scroll">
        <div class="message-image-export-preview__canvas" @click.self="emit('close')">
          <img
            ref="imageRef"
            class="message-image-export-preview__image"
            :src="props.objectUrl"
            :style="{ width: `${props.logicalWidth}px` }"
            alt="导出的消息图片"
          >
        </div>
      </div>
    </div>
    <button class="message-image-export-preview__close" type="button" aria-label="关闭预览" @click="emit('close')">
      <span aria-hidden="true">×</span>
    </button>
    <aside class="message-image-export-preview__toolbar" aria-label="图片操作">
      <button class="message-image-export-preview__action" type="button" @click="handleSave">
        <n-icon :size="17" aria-hidden="true"><Download /></n-icon>
        <span>保存到本地</span>
      </button>
      <button
        class="message-image-export-preview__action"
        type="button"
        :disabled="copying"
        @click="handleCopy"
      >
        <n-icon :size="17" aria-hidden="true"><Copy /></n-icon>
        <span>{{ copying ? '复制中…' : '复制到剪贴板' }}</span>
      </button>
    </aside>
  </div>
</template>

<style scoped>
.message-image-export-preview {
  position: fixed;
  z-index: 2400;
  inset: 0;
  box-sizing: border-box;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.92);
  background: rgba(5, 8, 13, 0.78);
  backdrop-filter: blur(18px) saturate(0.82);
  -webkit-backdrop-filter: blur(18px) saturate(0.82);
}

.message-image-export-preview__stage {
  position: absolute;
  z-index: 1;
  inset: 0;
  margin: 0;
}

.message-image-export-preview__close {
  position: absolute;
  z-index: 3;
  top: calc(18px + env(safe-area-inset-top));
  right: calc(18px + env(safe-area-inset-right));
  display: grid;
  width: 42px;
  height: 42px;
  padding: 0;
  place-items: center;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 50%;
  color: rgba(255, 255, 255, 0.9);
  background: rgba(15, 19, 27, 0.64);
  box-shadow: 0 8px 26px rgba(0, 0, 0, 0.24);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
  font: inherit;
  font-size: 28px;
  line-height: 1;
  cursor: pointer;
  transition: border-color 150ms ease, background-color 150ms ease, transform 150ms ease;
}

.message-image-export-preview__close:hover {
  border-color: rgba(255, 255, 255, 0.24);
  background: rgba(38, 44, 56, 0.82);
  transform: scale(1.04);
}

.message-image-export-preview__close:active {
  transform: scale(0.96);
}

.message-image-export-preview__scroll {
  position: absolute;
  inset: 0;
  overflow: auto;
  overscroll-behavior: contain;
}

.message-image-export-preview__canvas {
  display: flex;
  min-width: 100%;
  min-height: 100%;
  box-sizing: border-box;
  align-items: center;
  justify-content: center;
  padding: 48px 72px;
}

.message-image-export-preview__image {
  display: block;
  max-width: min(92vw, calc(100vw - 144px));
  height: auto;
  flex: 0 0 auto;
  box-shadow: 0 18px 56px rgba(0, 0, 0, 0.36);
  cursor: zoom-in;
}

.message-image-export-preview__toolbar {
  position: absolute;
  z-index: 3;
  top: 50%;
  left: min(
    calc(100vw - 160px - env(safe-area-inset-right)),
    calc(50% + min(var(--preview-image-half-width), 46vw, calc(50vw - 72px)) + 18px)
  );
  display: flex;
  width: 142px;
  box-sizing: border-box;
  gap: 8px;
  flex-direction: column;
  transform: translateY(-50%);
}

.message-image-export-preview__action {
  display: flex;
  min-width: 0;
  min-height: 48px;
  padding: 8px 12px;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border: 1px solid rgba(255, 255, 255, 0.11);
  border-radius: 14px;
  color: rgba(255, 255, 255, 0.86);
  background: rgba(13, 17, 24, 0.68);
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.24);
  backdrop-filter: blur(14px) saturate(1.12);
  -webkit-backdrop-filter: blur(14px) saturate(1.12);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
  transition: border-color 150ms ease, background-color 150ms ease, transform 150ms ease, opacity 150ms ease;
}

.message-image-export-preview__action:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.2);
  background: rgba(35, 42, 54, 0.86);
}

.message-image-export-preview__action:active:not(:disabled) {
  transform: translateY(1px);
}

.message-image-export-preview__action:disabled {
  opacity: 0.56;
  cursor: wait;
}

.message-image-export-preview__close:focus-visible,
.message-image-export-preview__action:focus-visible {
  outline: 2px solid rgba(130, 190, 255, 0.94);
  outline-offset: 2px;
}

@media (max-width: 720px) {
  .message-image-export-preview__close {
    top: calc(12px + env(safe-area-inset-top));
    right: calc(12px + env(safe-area-inset-right));
  }

  .message-image-export-preview__canvas {
    padding: calc(66px + env(safe-area-inset-top)) 16px calc(94px + env(safe-area-inset-bottom));
  }

  .message-image-export-preview__image {
    max-width: calc(100vw - 32px);
  }

  .message-image-export-preview__toolbar {
    top: auto;
    bottom: calc(12px + env(safe-area-inset-bottom));
    left: 50%;
    display: grid;
    width: min(360px, calc(100vw - 24px));
    grid-template-columns: 1fr 1fr;
    transform: translateX(-50%);
  }

  .message-image-export-preview__action {
    min-height: 44px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .message-image-export-preview__close,
  .message-image-export-preview__action {
    transition: none;
  }
}
</style>
