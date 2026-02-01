<script lang="ts" setup>
import { ref, computed, watch, onUnmounted, type PropType } from 'vue';
import {
  NButton,
  NSpace,
  NSlider,
  NRadioGroup,
  NRadio,
  NUpload,
  NIcon,
  NColorPicker,
  NSwitch,
  NCard,
  NInput,
  NPopconfirm,
  useMessage,
  type UploadFileInfo,
  type SelectOption,
} from 'naive-ui';
import { Photo as ImageIcon, Trash, Edit, Check, X, Plus, FolderPlus } from '@vicons/tabler';
import VueCropper from 'vue-cropperjs';
import 'cropperjs/dist/cropper.css';
import { compressImage } from '@/composables/useImageCompressor';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { useChatStore } from '@/stores/chat';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import type { SChannel, ChannelBackgroundSettings, BackgroundPreset } from '@/types';
import {
  loadPresets,
  loadCategories,
  saveCategories,
  createPreset,
  addPreset,
  updatePreset,
  deletePreset,
  addCategory,
} from '@/utils/backgroundPreset';

const props = defineProps({
  channel: {
    type: Object as PropType<SChannel>,
  },
});

const emit = defineEmits<{
  (e: 'update'): void;
}>();

const message = useMessage();
const chat = useChatStore();
const defaultSettings: ChannelBackgroundSettings = {
  mode: 'cover',
  opacity: 30,
  blur: 0,
  brightness: 100,
  overlayColor: undefined,
  overlayOpacity: 0,
};

const parseSettings = (input?: ChannelBackgroundSettings | string): ChannelBackgroundSettings => {
  if (!input) return { ...defaultSettings };
  if (typeof input !== 'string') {
    return { ...defaultSettings, ...input };
  }
  try {
    return { ...defaultSettings, ...JSON.parse(input) };
  } catch {
    return { ...defaultSettings };
  }
};

const backgroundAttachmentId = ref<string>('');
const settings = ref<ChannelBackgroundSettings>({ ...defaultSettings });
const saving = ref(false);

const cropperVisible = ref(false);
const cropperFile = ref<File | null>(null);
const cropperRef = ref<InstanceType<typeof VueCropper> | null>(null);
const cropperImageUrl = ref('');
const cropperProcessing = ref(false);

const enableOverlay = ref(false);

// 预设管理状态
const presets = ref<BackgroundPreset[]>([]);
const categories = ref<string[]>([]);
const selectedCategory = ref<string | null>(null);
const editingPresetId = ref<string | null>(null);
const editingPresetName = ref('');
const editingPresetCategory = ref<string | null>(null);
const newCategoryName = ref('');
const showNewCategoryInput = ref(false);
const saveAsPresetName = ref('');
const saveAsPresetCategory = ref<string | null>(null);
const showSavePresetModal = ref(false);
const showEditPresetModal = ref(false);

const backgroundUrl = computed(() => {
  if (!backgroundAttachmentId.value) return '';
  return resolveAttachmentUrl(backgroundAttachmentId.value);
});

const previewStyle = computed(() => {
  if (!backgroundUrl.value) return {};
  const s = settings.value;
  let bgSize = 'cover';
  let bgRepeat = 'no-repeat';
  let bgPosition = 'center';
  switch (s.mode) {
    case 'contain':
      bgSize = 'contain';
      break;
    case 'tile':
      bgSize = 'auto';
      bgRepeat = 'repeat';
      break;
    case 'center':
      bgSize = 'auto';
      bgPosition = 'center';
      break;
  }
  return {
    backgroundImage: `url(${backgroundUrl.value})`,
    backgroundSize: bgSize,
    backgroundRepeat: bgRepeat,
    backgroundPosition: bgPosition,
    opacity: s.opacity / 100,
    filter: `blur(${s.blur}px) brightness(${s.brightness}%)`,
  };
});

const overlayStyle = computed(() => {
  if (!enableOverlay.value || !settings.value.overlayColor) return {};
  return {
    backgroundColor: settings.value.overlayColor,
    opacity: (settings.value.overlayOpacity ?? 0) / 100,
  };
});

const categoryOptions = computed<SelectOption[]>(() => {
  return categories.value.map((c) => ({ label: c, value: c }));
});

const filteredPresets = computed(() => {
  if (!selectedCategory.value) return presets.value;
  return presets.value.filter((p) => p.category === selectedCategory.value);
});

const filterCategoryOptions = computed<SelectOption[]>(() => {
  return [
    { label: '全部', value: null as any },
    ...categories.value.map((c) => ({ label: c, value: c })),
  ];
});

const loadChannelPresets = () => {
  if (!props.channel?.id) return;
  presets.value = loadPresets(props.channel.id);
  categories.value = loadCategories(props.channel.id);
  if (selectedCategory.value && !categories.value.includes(selectedCategory.value)) {
    selectedCategory.value = null;
  }
};

watch(
  () => props.channel,
  (ch) => {
    if (ch) {
      backgroundAttachmentId.value = ch.backgroundAttachmentId || '';
      settings.value = parseSettings(ch.backgroundSettings);
      enableOverlay.value = !!settings.value.overlayColor && (settings.value.overlayOpacity ?? 0) > 0;
      loadChannelPresets();
    }
  },
  { immediate: true }
);

const handleFileChange = (options: { file: UploadFileInfo }) => {
  const file = options.file.file;
  if (!file) return;
  cropperFile.value = file;
  const reader = new FileReader();
  reader.onload = (e) => {
    cropperImageUrl.value = e.target?.result as string;
    cropperVisible.value = true;
  };
  reader.readAsDataURL(file);
};

const handleCropConfirm = async () => {
  if (!cropperRef.value) return;
  cropperProcessing.value = true;
  try {
    const cropper = cropperRef.value.cropper;
    if (!cropper) throw new Error('Cropper not initialized');
    const canvas = cropper.getCroppedCanvas({
      maxWidth: 1920,
      maxHeight: 1080,
      imageSmoothingEnabled: true,
      imageSmoothingQuality: 'high',
    });
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob((b) => {
        if (b) resolve(b);
        else reject(new Error('Failed to create blob'));
      }, 'image/png');
    });
    const pngFile = new File([blob], 'background.png', { type: 'image/png' });
    const compressed = await compressImage(pngFile, { maxWidth: 1920, maxHeight: 1080 });
    const result = await uploadImageAttachment(compressed, { channelId: props.channel?.id });
    let attachId = result.attachmentId || '';
    if (attachId.startsWith('id:')) {
      attachId = attachId.slice(3);
    }
    backgroundAttachmentId.value = attachId;
    cropperVisible.value = false;
    cropperFile.value = null;
    cropperImageUrl.value = '';
  } catch (err: any) {
    message.error(err?.message || '上传失败');
  } finally {
    cropperProcessing.value = false;
  }
};

const handleCropCancel = () => {
  cropperVisible.value = false;
  cropperFile.value = null;
  cropperImageUrl.value = '';
};

const removeBackground = () => {
  backgroundAttachmentId.value = '';
};

const resetSettings = () => {
  settings.value = { ...defaultSettings };
  enableOverlay.value = false;
};

const handleSave = async () => {
  if (!props.channel?.id) return;
  saving.value = true;
  try {
    const finalSettings: ChannelBackgroundSettings = { ...settings.value };
    if (!enableOverlay.value) {
      finalSettings.overlayColor = undefined;
      finalSettings.overlayOpacity = 0;
    }
    await chat.channelBackgroundEdit(props.channel.id, {
      backgroundAttachmentId: backgroundAttachmentId.value,
      backgroundSettings: JSON.stringify(finalSettings),
    });
    message.success('保存成功');
    emit('update');
  } catch (err: any) {
    message.error(err?.message || '保存失败');
  } finally {
    saving.value = false;
  }
};

// 预设管理功能
const openSavePresetModal = () => {
  saveAsPresetName.value = '';
  saveAsPresetCategory.value = null;
  showSavePresetModal.value = true;
};

const handleSaveAsPreset = () => {
  if (!props.channel?.id || !backgroundAttachmentId.value) return;
  const name = saveAsPresetName.value.trim() || `预设 ${presets.value.length + 1}`;
  const preset = createPreset(
    backgroundAttachmentId.value,
    settings.value,
    name,
    saveAsPresetCategory.value || undefined,
    backgroundUrl.value
  );
  presets.value = addPreset(props.channel.id, preset);
  showSavePresetModal.value = false;
  message.success('预设已保存');
};

const applyPreset = (preset: BackgroundPreset) => {
  backgroundAttachmentId.value = preset.attachmentId;
  settings.value = { ...preset.settings };
  enableOverlay.value = !!preset.settings.overlayColor && (preset.settings.overlayOpacity ?? 0) > 0;
};

const startEditPreset = (preset: BackgroundPreset) => {
  editingPresetId.value = preset.id;
  editingPresetName.value = preset.name;
  editingPresetCategory.value = preset.category || null;
  showEditPresetModal.value = true;
};

const confirmEditPreset = () => {
  if (!props.channel?.id || !editingPresetId.value) return;
  presets.value = updatePreset(props.channel.id, editingPresetId.value, {
    name: editingPresetName.value.trim() || '未命名预设',
    category: editingPresetCategory.value || undefined,
  });
  editingPresetId.value = null;
  showEditPresetModal.value = false;
};

const cancelEditPreset = () => {
  showEditPresetModal.value = false;
  editingPresetId.value = null;
};

const handleDeletePreset = (presetId: string) => {
  if (!props.channel?.id) return;
  presets.value = deletePreset(props.channel.id, presetId);
  message.success('预设已删除');
};

const handleAddCategory = () => {
  if (!props.channel?.id) return;
  const name = newCategoryName.value.trim();
  if (!name) return;
  categories.value = addCategory(props.channel.id, name);
  newCategoryName.value = '';
  showNewCategoryInput.value = false;
};

const getPresetThumbUrl = (preset: BackgroundPreset) => {
  return resolveAttachmentUrl(preset.attachmentId);
};

onUnmounted(() => {
  cropperImageUrl.value = '';
});
</script>

<template>
  <div class="tab-appearance">
    <!-- 无背景选项 -->
    <div class="section">
      <h4 class="section-title">背景图片</h4>
      <div class="upload-area">
        <div class="no-bg-option" :class="{ active: !backgroundAttachmentId }" @click="removeBackground">
          <div class="no-bg-icon">
            <NIcon :component="X" :size="20" />
          </div>
          <span>无</span>
        </div>
        <div v-if="backgroundUrl" class="current-bg">
          <img :src="backgroundUrl" alt="当前背景" class="bg-thumb" />
        </div>
        <NUpload
          accept="image/*"
          :show-file-list="false"
          :custom-request="() => {}"
          @change="handleFileChange"
        >
          <NButton type="primary" size="small">
            <template #icon><NIcon :component="ImageIcon" /></template>
            {{ backgroundUrl ? '更换' : '上传' }}
          </NButton>
        </NUpload>
        <NButton
          v-if="backgroundUrl"
          size="small"
          @click="openSavePresetModal"
        >
          <template #icon><NIcon :component="Plus" /></template>
          存为预设
        </NButton>
      </div>
    </div>

    <!-- 预设管理区域 -->
    <div class="section" v-if="presets.length > 0 || categories.length > 0">
      <div class="section-header">
        <h4 class="section-title">快速切换</h4>
        <div class="category-filter">
          <NSelect
            v-model:value="selectedCategory"
            :options="filterCategoryOptions"
            size="small"
            placeholder="分类筛选"
            clearable
            style="width: 100px;"
          />
        </div>
      </div>
      <div class="presets-grid">
        <div
          v-for="preset in filteredPresets"
          :key="preset.id"
          class="preset-item"
          :class="{ active: preset.attachmentId === backgroundAttachmentId }"
          @click="applyPreset(preset)"
        >
          <img :src="getPresetThumbUrl(preset)" :alt="preset.name" class="preset-thumb" />
          <div class="preset-overlay">
            <div class="preset-actions" @click.stop>
              <NButton quaternary circle size="tiny" @click="startEditPreset(preset)">
                <template #icon><NIcon :component="Edit" :size="14" /></template>
              </NButton>
              <NPopconfirm @positive-click="handleDeletePreset(preset.id)">
                <template #trigger>
                  <NButton quaternary circle size="tiny" type="error">
                    <template #icon><NIcon :component="Trash" :size="14" /></template>
                  </NButton>
                </template>
                确定删除此预设？
              </NPopconfirm>
            </div>
          </div>
          <div class="preset-meta">
            <span class="preset-name">{{ preset.name }}</span>
            <span v-if="preset.category" class="preset-category">{{ preset.category }}</span>
          </div>
        </div>
      </div>
      <!-- 分类管理 -->
      <div class="category-manage">
        <span class="category-label">分类：</span>
        <span v-for="cat in categories" :key="cat" class="category-tag">{{ cat }}</span>
        <NButton v-if="!showNewCategoryInput" quaternary size="tiny" @click="showNewCategoryInput = true">
          <template #icon><NIcon :component="FolderPlus" :size="14" /></template>
          添加分类
        </NButton>
        <div v-else class="new-category-input">
          <NInput v-model:value="newCategoryName" size="tiny" placeholder="分类名" style="width: 80px;" />
          <NButton size="tiny" type="primary" @click="handleAddCategory">确定</NButton>
          <NButton size="tiny" @click="showNewCategoryInput = false">取消</NButton>
        </div>
      </div>
    </div>

    <div class="section" v-if="backgroundUrl">
      <h4 class="section-title">显示设置</h4>
      <div class="settings-grid">
        <div class="setting-row">
          <span class="setting-label">显示模式</span>
          <NRadioGroup v-model:value="settings.mode" size="small">
            <NRadio value="cover">铺满</NRadio>
            <NRadio value="contain">适应</NRadio>
            <NRadio value="tile">平铺</NRadio>
            <NRadio value="center">居中</NRadio>
          </NRadioGroup>
        </div>
        <div class="setting-row">
          <span class="setting-label">透明度 {{ settings.opacity }}%</span>
          <NSlider v-model:value="settings.opacity" :min="5" :max="100" :step="5" />
        </div>
        <div class="setting-row">
          <span class="setting-label">模糊 {{ settings.blur }}px</span>
          <NSlider v-model:value="settings.blur" :min="0" :max="20" :step="1" />
        </div>
        <div class="setting-row">
          <span class="setting-label">亮度 {{ settings.brightness }}%</span>
          <NSlider v-model:value="settings.brightness" :min="50" :max="150" :step="5" />
        </div>
        <div class="setting-row">
          <span class="setting-label">颜色叠加</span>
          <NSwitch v-model:value="enableOverlay" size="small" />
        </div>
        <template v-if="enableOverlay">
          <div class="setting-row">
            <span class="setting-label">叠加颜色</span>
            <NColorPicker v-model:value="settings.overlayColor" :show-alpha="false" size="small" />
          </div>
          <div class="setting-row">
            <span class="setting-label">叠加透明度 {{ settings.overlayOpacity ?? 0 }}%</span>
            <NSlider v-model:value="settings.overlayOpacity" :min="0" :max="100" :step="5" />
          </div>
        </template>
      </div>
      <NButton size="small" quaternary @click="resetSettings">重置为默认</NButton>
    </div>

    <div class="section" v-if="backgroundUrl">
      <h4 class="section-title">预览</h4>
      <NCard class="preview-card">
        <div class="preview-container">
          <div class="preview-bg" :style="previewStyle"></div>
          <div class="preview-overlay" :style="overlayStyle"></div>
          <div class="preview-content">
            <div class="preview-msg preview-msg--other">
              <div class="preview-avatar">A</div>
              <div class="preview-bubble">这是一条示例消息</div>
            </div>
            <div class="preview-msg preview-msg--self">
              <div class="preview-bubble">我的回复内容</div>
              <div class="preview-avatar">我</div>
            </div>
          </div>
        </div>
      </NCard>
    </div>

    <div class="actions">
      <NButton type="primary" :loading="saving" @click="handleSave">保存外观设置</NButton>
    </div>

    <n-modal v-model:show="cropperVisible" preset="card" title="裁剪背景图" style="max-width: 600px;">
      <div class="cropper-wrapper" v-if="cropperImageUrl">
        <VueCropper
          ref="cropperRef"
          :src="cropperImageUrl"
          :aspect-ratio="16 / 9"
          :view-mode="1"
          drag-mode="move"
          :auto-crop-area="0.9"
          :background="true"
          :guides="true"
          class="cropper-instance"
        />
      </div>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="handleCropCancel">取消</NButton>
          <NButton type="primary" :loading="cropperProcessing" @click="handleCropConfirm">确认</NButton>
        </NSpace>
      </template>
    </n-modal>

    <n-modal v-model:show="showSavePresetModal" preset="card" title="保存为预设" style="max-width: 400px;">
      <NSpace vertical>
        <NInput v-model:value="saveAsPresetName" placeholder="预设名称（可选）" />
        <NSelect
          v-model:value="saveAsPresetCategory"
          :options="categoryOptions"
          placeholder="选择分类（可选）"
          clearable
        />
      </NSpace>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showSavePresetModal = false">取消</NButton>
          <NButton type="primary" @click="handleSaveAsPreset">保存</NButton>
        </NSpace>
      </template>
    </n-modal>

    <n-modal
      v-model:show="showEditPresetModal"
      preset="card"
      title="编辑预设"
      style="max-width: 400px;"
      @after-leave="cancelEditPreset"
    >
      <NSpace vertical>
        <NInput v-model:value="editingPresetName" placeholder="预设名称" />
        <NSelect
          v-model:value="editingPresetCategory"
          :options="categoryOptions"
          placeholder="分类（可选）"
          clearable
        />
      </NSpace>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="cancelEditPreset">取消</NButton>
          <NButton type="primary" @click="confirmEditPreset">保存</NButton>
        </NSpace>
      </template>
    </n-modal>
  </div>
</template>

<style lang="scss" scoped>
.tab-appearance {
  padding: 1rem 0;
}

.section {
  margin-bottom: 1.5rem;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.75rem;
}

.section-title {
  font-size: 0.95rem;
  font-weight: 600;
  margin-bottom: 0.75rem;
  color: var(--n-text-color);
}

.section-header .section-title {
  margin-bottom: 0;
}

.upload-area {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.no-bg-option {
  width: 60px;
  height: 45px;
  border: 2px dashed var(--n-border-color);
  border-radius: 4px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
  color: var(--n-text-color-3);
  font-size: 0.75rem;

  &:hover {
    border-color: var(--n-primary-color);
    color: var(--n-primary-color);
  }

  &.active {
    border-color: var(--n-primary-color);
    background: var(--n-primary-color-suppl);
    color: var(--n-primary-color);
  }
}

.no-bg-icon {
  margin-bottom: 2px;
}

.current-bg {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.bg-thumb {
  width: 80px;
  height: 45px;
  object-fit: cover;
  border-radius: 4px;
  border: 2px solid var(--n-primary-color);
}

.presets-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
  gap: 0.75rem;
}

.preset-item {
  position: relative;
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  border: 2px solid transparent;
  transition: all 0.2s;

  &:hover {
    border-color: var(--n-primary-color-hover);

    .preset-overlay {
      opacity: 1;
    }
  }

  &.active {
    border-color: var(--n-primary-color);
  }
}

.preset-thumb {
  width: 100%;
  aspect-ratio: 16 / 9;
  object-fit: cover;
  display: block;
}

.preset-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 24px;
  background: rgba(0, 0, 0, 0.5);
  opacity: 0;
  transition: opacity 0.2s;
  display: flex;
  align-items: flex-start;
  justify-content: flex-end;
  padding: 4px;
}

.preset-actions {
  display: flex;
  gap: 2px;
}

.preset-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 4px 6px;
  background: var(--n-color-modal);
  min-height: 26px;
}

.preset-name {
  font-size: 0.75rem;
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
  flex: 1;
}

.preset-category {
  font-size: 0.625rem;
  padding: 1px 6px;
  background: var(--n-tag-color);
  color: var(--n-text-color-2);
  border-radius: 10px;
  white-space: nowrap;
  flex-shrink: 0;
}

.category-manage {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.75rem;
  flex-wrap: wrap;
}

.category-label {
  font-size: 0.8rem;
  color: var(--n-text-color-3);
}

.category-tag {
  font-size: 0.75rem;
  padding: 2px 8px;
  background: var(--n-tag-color);
  border-radius: 4px;
  color: var(--n-text-color-2);
}

.new-category-input {
  display: flex;
  gap: 4px;
  align-items: center;
}

.settings-grid {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
}

.setting-row {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.setting-label {
  min-width: 140px;
  font-size: 0.875rem;
  color: var(--n-text-color-3);
}

.preview-card {
  padding: 0;
}

.preview-container {
  position: relative;
  height: 180px;
  border-radius: 4px;
  overflow: hidden;
  background: #1a1a1a;
}

.preview-bg {
  position: absolute;
  inset: 0;
  z-index: 1;
}

.preview-overlay {
  position: absolute;
  inset: 0;
  z-index: 2;
  pointer-events: none;
}

.preview-content {
  position: relative;
  z-index: 3;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.preview-msg {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;

  &--self {
    justify-content: flex-end;
  }
}

.preview-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.75rem;
  color: #fff;
  flex-shrink: 0;
}

.preview-bubble {
  background: rgba(255, 255, 255, 0.15);
  backdrop-filter: blur(4px);
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  color: #fff;
  font-size: 0.875rem;
  max-width: 200px;
}

.actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 1rem;
  border-top: 1px solid var(--n-border-color);
}

.cropper-wrapper {
  width: 100%;
  height: 350px;
  background: #333;
  border-radius: 4px;
  overflow: hidden;
}

.cropper-instance {
  width: 100%;
  height: 100%;
}
</style>
