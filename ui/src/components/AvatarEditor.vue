<script lang="ts" setup>
import { ref, watch, onUnmounted } from 'vue';
import VueCropper from 'vue-cropperjs';
import 'cropperjs/dist/cropper.css';
import { NButton, NSpace, NSpin } from 'naive-ui';
import { compressImage } from '@/composables/useImageCompressor';

const props = defineProps<{
  /** The image file to edit */
  file: File | null;
  /** Aspect ratio of output (default 1 for square) */
  aspectRatio?: number;
  /** Output size in pixels (default 200) */
  outputSize?: number;
}>();

const emit = defineEmits<{
  (e: 'save', file: File): void;
  (e: 'cancel'): void;
}>();

// Configuration
const OUTPUT_SIZE = props.outputSize ?? 200;
const ASPECT_RATIO = props.aspectRatio ?? 1;

// State
const cropperRef = ref<InstanceType<typeof VueCropper> | null>(null);
const imageDataUrl = ref('');
const isProcessing = ref(false);
const tooSmall = ref(false);
const minDimension = 100;

// Load image when file changes
watch(() => props.file, async (file) => {
  if (!file) {
    imageDataUrl.value = '';
    return;
  }
  const reader = new FileReader();
  reader.onload = (e) => {
    const result = e.target?.result as string;
    imageDataUrl.value = result;
    tooSmall.value = false;
  };
  reader.readAsDataURL(file);
}, { immediate: true });

// Rotate functions
const rotateLeft = () => {
  cropperRef.value?.rotate(-90);
};

const rotateRight = () => {
  cropperRef.value?.rotate(90);
};

// Flip functions
const flipHorizontal = () => {
  const cropper = cropperRef.value?.cropper;
  if (cropper) {
    const data = cropper.getData();
    cropper.scaleX(data.scaleX === -1 ? 1 : -1);
  }
};

const flipVertical = () => {
  const cropper = cropperRef.value?.cropper;
  if (cropper) {
    const data = cropper.getData();
    cropper.scaleY(data.scaleY === -1 ? 1 : -1);
  }
};

// Reset function
const reset = () => {
  cropperRef.value?.reset();
};

// Handle cropper ready
const handleReady = () => {
  const cropper = cropperRef.value?.cropper;
  if (cropper) {
    const imageData = cropper.getImageData();
    if (imageData.naturalWidth < minDimension || imageData.naturalHeight < minDimension) {
      tooSmall.value = true;
    }
  }
};

// Save handler
const handleSave = async () => {
  if (tooSmall.value || !cropperRef.value) return;
  
  isProcessing.value = true;
  try {
    const cropper = cropperRef.value.cropper;
    if (!cropper) {
      throw new Error('Cropper not initialized');
    }
    
    // Get cropped canvas
    const canvas = cropper.getCroppedCanvas({
      width: OUTPUT_SIZE,
      height: OUTPUT_SIZE,
      imageSmoothingEnabled: true,
      imageSmoothingQuality: 'high',
    });
    
    // Convert to blob
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob((b) => {
        if (b) resolve(b);
        else reject(new Error('Failed to create blob'));
      }, 'image/png');
    });
    
    const pngFile = new File([blob], 'avatar.png', { type: 'image/png' });
    
    // Compress to WebP
    const compressedFile = await compressImage(pngFile, {
      maxWidth: OUTPUT_SIZE,
      maxHeight: OUTPUT_SIZE,
    });
    
    emit('save', compressedFile);
  } catch (error) {
    console.error('Failed to process avatar:', error);
  } finally {
    isProcessing.value = false;
  }
};

const handleCancel = () => {
  emit('cancel');
};

// Cleanup
onUnmounted(() => {
  imageDataUrl.value = '';
});
</script>

<template>
  <div class="avatar-editor">
    <div v-if="tooSmall" class="avatar-editor__error">
      图片尺寸过小，请选择至少 {{ minDimension }}x{{ minDimension }} 像素的图片
    </div>
    
    <div v-else-if="imageDataUrl" class="avatar-editor__content">
      <!-- Cropper -->
      <div class="avatar-editor__cropper-wrapper">
        <VueCropper
          ref="cropperRef"
          :src="imageDataUrl"
          :aspect-ratio="ASPECT_RATIO"
          :view-mode="1"
          :drag-mode="'move'"
          :auto-crop-area="0.8"
          :background="true"
          :guides="true"
          :center="true"
          :highlight="true"
          :crop-box-movable="true"
          :crop-box-resizable="true"
          :toggle-drag-mode-on-dblclick="false"
          @ready="handleReady"
          class="avatar-editor__cropper"
        />
      </div>

      <!-- Transform buttons -->
      <div class="avatar-editor__transforms">
        <NSpace justify="center">
          <NButton size="small" @click="rotateLeft" title="逆时针旋转">↺ 左旋</NButton>
          <NButton size="small" @click="rotateRight" title="顺时针旋转">↻ 右旋</NButton>
          <NButton size="small" @click="flipHorizontal" title="水平翻转">↔ 水平</NButton>
          <NButton size="small" @click="flipVertical" title="垂直翻转">↕ 垂直</NButton>
          <NButton size="small" @click="reset" title="重置">重置</NButton>
        </NSpace>
      </div>
    </div>

    <div v-else class="avatar-editor__empty">
      请选择图片
    </div>

    <!-- Action buttons -->
    <div v-if="imageDataUrl && !tooSmall" class="avatar-editor__actions">
      <NButton @click="handleCancel">取消</NButton>
      <NButton type="primary" @click="handleSave" :loading="isProcessing">确认</NButton>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.avatar-editor {
  display: flex;
  flex-direction: column;
  gap: 1rem;

  &__error {
    color: #e53e3e;
    text-align: center;
    padding: 1rem;
  }

  &__empty {
    text-align: center;
    padding: 2rem;
    color: #666;
  }

  &__content {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  &__cropper-wrapper {
    width: 100%;
    max-width: 400px;
    height: 300px;
    margin: 0 auto;
    background: #f0f0f0;
    border-radius: 4px;
    overflow: hidden;
  }

  &__cropper {
    width: 100%;
    height: 100%;
  }

  &__transforms {
    display: flex;
    justify-content: center;
  }

  &__actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }
}
</style>
