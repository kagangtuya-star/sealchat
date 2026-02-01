<template>
  <n-drawer
    class="gallery-drawer"
    :show="visible"
    placement="right"
    :width="drawerWidth"
    @update:show="handleShow"
  >
    <n-drawer-content closable>
      <template #header>
        <div class="gallery-header">
          <n-button v-if="isMobileLayout" text class="gallery-header__back" @click="gallery.closePanel()">
            <template #icon>
              <n-icon :component="ChevronBack" :size="20" />
            </template>
            返回
          </n-button>
          <span class="gallery-header__title">快捷画廊</span>
        </div>
      </template>
      <div class="gallery-panel">
        <GalleryCollectionTree
          :collections="collections"
          :active-id="gallery.activeCollectionId"
          @select="handleCollectionSelect"
          @context-action="handleCollectionAction"
          @drop-items="handleDropItems"
        >
          <template #actions>
            <n-button size="small" type="primary" block @click="emitCreateCollection">新建分类</n-button>
            <n-button
              v-if="gallery.activeCollectionId && !isFavorites && !isSystemCollection"
              size="small"
              tertiary
              block
              @click="toggleEmojiLink"
            >
              {{ isEmojiLinked ? '取消表情联动' : '添加表情联动' }}
            </n-button>
          </template>
        </GalleryCollectionTree>

        <div class="gallery-panel__content" tabindex="0" @keydown="handleKeydown" ref="contentRef">
          <div class="gallery-panel__toolbar">
            <GalleryUploadZone :disabled="uploading" @select="handleUploadSelect" />
            <div class="gallery-panel__toolbar-actions">
              <n-select
                v-model:value="sortBy"
                size="small"
                :options="sortOptions"
                style="width: 100px"
              />
              <n-select
                v-model:value="thumbnailSize"
                size="small"
                :options="sizeOptions"
                style="width: 90px"
              />
              <n-input
                v-model:value="keyword"
                size="small"
                placeholder="搜索备注"
                clearable
                @clear="loadActiveItems"
                @keyup.enter="loadActiveItems"
                style="width: 140px"
              />
              <n-button size="small" :loading="loading" @click="loadActiveItems">刷新</n-button>
            </div>
          </div>

          <!-- Upload progress -->
          <div v-if="uploadProgress.total > 0" class="gallery-panel__progress">
            <n-progress
              type="line"
              :percentage="Math.round((uploadProgress.current / uploadProgress.total) * 100)"
              :show-indicator="true"
            />
            <span class="gallery-panel__progress-text">
              上传中 {{ uploadProgress.current }}/{{ uploadProgress.total }}
            </span>
          </div>

          <!-- Batch operations toolbar -->
          <div v-if="selectedIds.length > 0" class="gallery-panel__batch-toolbar">
            <span class="gallery-panel__batch-count">已选中 {{ selectedIds.length }} 项</span>
            <div class="gallery-panel__batch-actions">
              <n-button size="small" @click="selectAll">全选</n-button>
              <n-button size="small" @click="clearSelection">取消选择</n-button>
              <n-button size="small" type="primary" @click="openMoveModal">移动到</n-button>
              <n-button size="small" type="error" @click="handleBatchDelete">删除</n-button>
              <n-button size="small" type="info" @click="handleBatchInsert">插入</n-button>
            </div>
          </div>

          <GalleryGrid
            :items="items"
            :loading="loading"
            :editable="true"
            :selectable="true"
            :selected-ids="selectedIds"
            :thumbnail-size="thumbnailSize"
            @toggle-select="handleToggleSelect"
            @range-select="handleRangeSelect"
            @insert="handleItemInsert"
            @edit="handleItemEdit"
            @delete="handleItemDelete"
            @reorder="handleReorder"
          />
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>

  <n-modal
    v-model:show="createModalVisible"
    preset="dialog"
    :show-icon="false"
    title="新建分类"
    :positive-text="creatingCollection ? '创建中…' : '创建'"
    :positive-button-props="{ loading: creatingCollection }"
    negative-text="取消"
    @positive-click="handleCreateSubmit"
    @negative-click="handleCreateCancel"
  >
    <n-form label-width="72">
      <n-form-item label="名称">
        <n-input v-model:value="newCollectionName" maxlength="32" placeholder="请输入分类名称" />
      </n-form-item>
      <n-form-item label="排序">
        <n-input-number v-model:value="newCollectionOrder" :show-button="false" placeholder="可选" />
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="editModalVisible"
    preset="dialog"
    :show-icon="false"
    title="修改备注"
    :positive-text="editingRemark ? '保存中…' : '保存'"
    :positive-button-props="{ loading: editingRemark }"
    negative-text="取消"
    @positive-click="handleEditSubmit"
    @negative-click="handleEditCancel"
  >
    <n-form label-width="72">
      <n-form-item label="备注">
        <n-input v-model:value="editRemark" maxlength="64" placeholder="请输入新的备注" />
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="renameModalVisible"
    preset="dialog"
    :show-icon="false"
    title="重命名分类"
    :positive-text="renamingCollection ? '保存中…' : '保存'"
    :positive-button-props="{ loading: renamingCollection }"
    negative-text="取消"
    @positive-click="handleRenameSubmit"
    @negative-click="handleRenameCancel"
  >
    <n-form label-width="72">
      <n-form-item label="名称">
        <n-input v-model:value="renameCollectionName" maxlength="32" placeholder="请输入分类名称" />
      </n-form-item>
    </n-form>
  </n-modal>

  <!-- Move to collection modal -->
  <n-modal
    v-model:show="moveModalVisible"
    preset="dialog"
    :show-icon="false"
    title="移动到分类"
    :positive-text="movingItems ? '移动中…' : '移动'"
    :positive-button-props="{ loading: movingItems }"
    negative-text="取消"
    @positive-click="handleMoveSubmit"
    @negative-click="moveModalVisible = false"
  >
    <div class="move-modal__content">
      <p class="move-modal__hint">将 {{ selectedIds.length }} 个项目移动到：</p>
      <n-select
        v-model:value="moveTargetCollectionId"
        :options="moveCollectionOptions"
        placeholder="选择目标分类"
      />
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue';
import { NDrawer, NDrawerContent, NButton, NInput, useMessage, useDialog, NModal, NForm, NFormItem, NInputNumber, NIcon, NProgress } from 'naive-ui';
import { ChevronBack } from '@vicons/ionicons5';
import type { UploadFileInfo } from 'naive-ui';
import type { GalleryItem } from '@/types';
import { useGalleryStore } from '@/stores/gallery';
import { useUserStore } from '@/stores/user';
import GalleryCollectionTree from './GalleryCollectionTree.vue';
import GalleryGrid from './GalleryGrid.vue';
import GalleryUploadZone from './GalleryUploadZone.vue';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { dialogAskConfirm } from '@/utils/dialog';

interface UploadTask {
  attachmentId: string;
  thumbData: string;
  remark: string;
}

const gallery = useGalleryStore();
const user = useUserStore();
const message = useMessage();
const dialog = useDialog();
const remarkPattern = /^[\p{L}\p{N}_]{1,64}$/u;
const STORAGE_THUMBNAIL_SIZE = 'sealchat.gallery.thumbnailSize';

const emit = defineEmits<{ (e: 'insert', src: string): void }>();

const keyword = ref('');
const uploading = ref(false);
const creatingCollection = ref(false);
const createModalVisible = ref(false);
const newCollectionName = ref('');
const newCollectionOrder = ref<number | null>(null);
const editModalVisible = ref(false);
const editingRemark = ref(false);
const editRemark = ref('');
const editingItem = ref<GalleryItem | null>(null);

const renameModalVisible = ref(false);
const renamingCollection = ref(false);
const renameCollectionName = ref('');
const renamingCollectionId = ref<string | null>(null);

// Batch operations state
const selectedIds = ref<string[]>([]);
const moveModalVisible = ref(false);
const movingItems = ref(false);
const moveTargetCollectionId = ref<string | null>(null);

const moveCollectionOptions = computed(() =>
  collections.value
    .filter(c => c.id !== gallery.activeCollectionId)
    .map(c => ({ label: c.name, value: c.id }))
);

// Sorting
const sortBy = ref<'time' | 'name'>('time');
const sortOptions = [
  { label: '按时间', value: 'time' },
  { label: '按名称', value: 'name' }
];
const thumbnailSize = ref<'small' | 'medium' | 'large' | 'xlarge'>('medium');
const sizeOptions = [
  { label: '小图', value: 'small' },
  { label: '中图', value: 'medium' },
  { label: '大图', value: 'large' },
  { label: '超大', value: 'xlarge' }
];

// Upload progress
const uploadProgress = ref({ current: 0, total: 0 });

// Keyboard handling
const contentRef = ref<HTMLElement | null>(null);

const visible = computed(() => gallery.isPanelVisible);

// Auto-refresh 1 second after panel opens or collection switches to fix data fetch latency
let autoRefreshTimer: ReturnType<typeof setTimeout> | null = null;
watch([visible, () => gallery.activeCollectionId], ([isVisible, activeCollectionId]) => {
  if (autoRefreshTimer) {
    clearTimeout(autoRefreshTimer);
    autoRefreshTimer = null;
  }
  if (isVisible && activeCollectionId) {
    autoRefreshTimer = setTimeout(() => {
      gallery.loadItems(activeCollectionId);
    }, 1000);
  }
});

onMounted(() => {
  if (typeof window === 'undefined') return;
  const stored = window.localStorage.getItem(STORAGE_THUMBNAIL_SIZE);
  if (stored === 'small' || stored === 'medium' || stored === 'large' || stored === 'xlarge') {
    thumbnailSize.value = stored;
  }
});

watch(thumbnailSize, (value) => {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(STORAGE_THUMBNAIL_SIZE, value);
  } catch (error) {
    console.warn('画廊缩略图尺寸写入失败', error);
  }
});

const drawerWidth = computed(() => {
  if (typeof window === 'undefined') return 720;
  return window.innerWidth < 768 ? '100%' : 720;
});
const isMobileLayout = computed(() => {
  if (typeof window === 'undefined') return false;
  return window.innerWidth < 768;
});

const userId = computed(() => gallery.activeOwner?.id || user.info.id || '');
const collections = computed(() => (userId.value ? gallery.getCollections(userId.value) : []));
const activeCollection = computed(() =>
  collections.value.find((collection) => collection.id === gallery.activeCollectionId) ?? null
);
const rawItems = computed(() => (gallery.activeCollectionId ? gallery.getItemsByCollection(gallery.activeCollectionId) : []));
const items = computed(() => {
  const list = [...rawItems.value];
  if (sortBy.value === 'name') {
    list.sort((a, b) => (a.remark || '').localeCompare(b.remark || ''));
  } else {
    // Default: sort by order (time)
    list.sort((a, b) => (b.order ?? 0) - (a.order ?? 0));
  }
  return list;
});
const loading = computed(() => {
  // Show loading during initialization
  if (gallery.isInitializing) return true;
  // No active collection means nothing to load (empty state, not loading)
  if (!gallery.activeCollectionId) return false;
  // Show loading if active collection is loading
  return gallery.isCollectionLoading(gallery.activeCollectionId);
});
const isEmojiLinked = computed(() => gallery.activeCollectionId ? gallery.emojiCollectionIds.includes(gallery.activeCollectionId) : false);
const isFavorites = computed(() => gallery.activeCollectionId === gallery.favoritesCollectionId);
const isSystemCollection = computed(() => !!activeCollection.value?.collectionType);

// Keyboard shortcuts handler
function handleKeydown(evt: KeyboardEvent) {
  // Ignore if typing in input
  if ((evt.target as HTMLElement)?.tagName === 'INPUT') return;
  
  // Ctrl/Cmd + A: Select all
  if ((evt.ctrlKey || evt.metaKey) && evt.key === 'a') {
    evt.preventDefault();
    selectAll();
    return;
  }
  
  // Delete/Backspace: Delete selected
  if ((evt.key === 'Delete' || evt.key === 'Backspace') && selectedIds.value.length > 0) {
    evt.preventDefault();
    handleBatchDelete();
    return;
  }
  
  // Escape: Clear selection
  if (evt.key === 'Escape') {
    clearSelection();
    return;
  }
}

function handleShow(value: boolean) {
  if (!value) {
    gallery.closePanel();
  }
}

async function handleCollectionSelect(collectionId: string) {
  if (!collectionId) return;
  await gallery.setActiveCollection(collectionId);
}

async function handleCollectionAction(action: string, collection: any) {
  if (action === 'rename') {
    renamingCollectionId.value = collection.id;
    renameCollectionName.value = collection.name;
    renameModalVisible.value = true;
  } else if (action === 'delete') {
    const confirmed = await dialogAskConfirm(dialog, `确定删除分类"${collection.name}"吗？`, '此操作不可恢复');
    if (confirmed) {
      try {
        await gallery.deleteCollection(userId.value, collection.id);
        message.success('分类已删除');
      } catch (error: any) {
        message.error(error?.message || '删除失败');
      }
    }
  }
}

function emitCreateCollection() {
  createModalVisible.value = true;
  newCollectionName.value = '';
  newCollectionOrder.value = null;
}

function sanitizeRemark(name: string) {
  const trimmed = name.replace(/\.[^/.]+$/, '');
  const normalized = trimmed
    .replace(/\s+/g, '_')
    .replace(/[^\w\u4e00-\u9fa5]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_+|_+$/g, '');
  return normalized.slice(0, 64) || 'img';
}

function readFileAsDataUrl(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ''));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

async function generateThumbnail(file: File) {
  try {
    const dataUrl = await readFileAsDataUrl(file);
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = (err) => reject(err);
      image.src = dataUrl;
    });
    const maxSize = 128;
    const scale = Math.min(1, maxSize / Math.max(img.width, img.height));
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(img.width * scale));
    canvas.height = Math.max(1, Math.round(img.height * scale));
    const ctx = canvas.getContext('2d');
    if (!ctx) return dataUrl;
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
    return canvas.toDataURL('image/png', 0.92);
  } catch (error) {
    console.warn('生成缩略图失败', error);
    return readFileAsDataUrl(file);
  }
}

async function handleUploadSelect(files: UploadFileInfo[]) {
  if (!userId.value) {
    message.warning('请先登录');
    return;
  }
  const collectionId = gallery.activeCollectionId;
  if (!collectionId) {
    message.warning('请先选择分类');
    return;
  }
  const candidates = files.map((f) => f.file).filter((f): f is File => Boolean(f && f.type.startsWith('image/')));
  if (!candidates.length) {
    message.warning('请选择图片文件');
    return;
  }
  uploading.value = true;
  uploadProgress.value = { current: 0, total: candidates.length };
  try {
    const payloadItems: UploadTask[] = [];
    for (let i = 0; i < candidates.length; i++) {
      const file = candidates[i];
      uploadProgress.value.current = i + 1;
      const { attachmentId } = await uploadImageAttachment(file);
      const normalizedId = attachmentId.startsWith('id:') ? attachmentId.slice(3) : attachmentId;
      const thumbData = await generateThumbnail(file);
      payloadItems.push({
        attachmentId: normalizedId,
        thumbData,
        remark: sanitizeRemark(file.name),
      });
    }
    if (payloadItems.length) {
      // Check for duplicates by attachmentId OR remark (filename)
      const existingIds = new Set(items.value.map(item => item.attachmentId));
      const existingRemarks = new Set(items.value.map(item => item.remark));
      const skippedNames: string[] = [];
      const uniqueItems = payloadItems.filter(item => {
        const isDuplicate = existingIds.has(item.attachmentId) || existingRemarks.has(item.remark);
        if (isDuplicate) {
          skippedNames.push(item.remark);
        }
        return !isDuplicate;
      });
      
      if (uniqueItems.length > 0) {
        await gallery.upload(collectionId, {
          collectionId,
          items: uniqueItems.map((item, index) => ({
            attachmentId: item.attachmentId,
            thumbData: item.thumbData,
            remark: item.remark,
            order: Date.now() + index,
          })),
        });
      }
      
      // Show notifications
      if (skippedNames.length > 0) {
        skippedNames.forEach(name => {
          message.warning(`${name} 已经存在，上传被跳过`);
        });
      }
      if (uniqueItems.length > 0) {
        message.success(`上传成功 ${uniqueItems.length} 张图片`);
      }
      keyword.value = '';
    }
  } catch (error: any) {
    console.error('画廊上传失败', error);
    message.error(error?.message || '上传失败，请稍后重试');
  } finally {
    uploading.value = false;
    uploadProgress.value = { current: 0, total: 0 };
  }
}

function loadActiveItems() {
  if (gallery.activeCollectionId) {
    void gallery.loadItems(gallery.activeCollectionId, { keyword: keyword.value || undefined });
  }
}

function handleToggleSelect(item: GalleryItem, selected: boolean) {
  if (selected) {
    if (!selectedIds.value.includes(item.id)) {
      selectedIds.value = [...selectedIds.value, item.id];
    }
  } else {
    selectedIds.value = selectedIds.value.filter(id => id !== item.id);
  }
}

function handleRangeSelect(startIndex: number, endIndex: number) {
  const start = Math.min(startIndex, endIndex);
  const end = Math.max(startIndex, endIndex);
  const rangeIds = items.value.slice(start, end + 1).map(item => item.id);
  const newSelection = new Set([...selectedIds.value, ...rangeIds]);
  selectedIds.value = Array.from(newSelection);
}

function selectAll() {
  selectedIds.value = items.value.map(item => item.id);
}

function clearSelection() {
  selectedIds.value = [];
}

function handleItemInsert(item: GalleryItem) {
  const src = item.attachmentId ? `id:${item.attachmentId}` : '';
  if (!src) return;
  emit('insert', src);
}

function openMoveModal() {
  if (selectedIds.value.length === 0) return;
  moveTargetCollectionId.value = null;
  moveModalVisible.value = true;
}

async function handleMoveSubmit() {
  if (!moveTargetCollectionId.value || selectedIds.value.length === 0) {
    message.warning('请选择目标分类');
    return false;
  }
  movingItems.value = true;
  try {
    const targetId = moveTargetCollectionId.value;
    const currentCollectionId = gallery.activeCollectionId;
    if (!currentCollectionId) return false;
    
    for (const itemId of selectedIds.value) {
      await gallery.updateItem(currentCollectionId, itemId, { collectionId: targetId });
    }
    message.success(`已移动 ${selectedIds.value.length} 个项目`);
    clearSelection();
    moveModalVisible.value = false;
    return true;
  } catch (error: any) {
    message.error(error?.message || '移动失败');
    return false;
  } finally {
    movingItems.value = false;
  }
}

async function handleDropItems(targetCollectionId: string, itemIds: string[]) {
  const currentCollectionId = gallery.activeCollectionId;
  if (!currentCollectionId || itemIds.length === 0) return;
  if (targetCollectionId === currentCollectionId) return;
  
  try {
    for (const itemId of itemIds) {
      await gallery.updateItem(currentCollectionId, itemId, { collectionId: targetCollectionId });
    }
    message.success(`已移动 ${itemIds.length} 个项目`);
    clearSelection();
  } catch (error: any) {
    message.error(error?.message || '移动失败');
  }
}

async function handleReorder(fromIndex: number, toIndex: number) {
  if (!gallery.activeCollectionId) return;
  
  // Get items in current sorted order
  const currentItems = [...items.value];
  if (fromIndex < 0 || fromIndex >= currentItems.length) return;
  if (toIndex < 0 || toIndex >= currentItems.length) return;
  
  const item = currentItems[fromIndex];
  const targetItem = currentItems[toIndex];
  
  // Calculate new order value
  let newOrder: number;
  if (toIndex === 0) {
    // Move to first position
    newOrder = (currentItems[0]?.order ?? Date.now()) + 1;
  } else if (toIndex === currentItems.length - 1) {
    // Move to last position
    newOrder = (currentItems[currentItems.length - 1]?.order ?? 1) - 1;
  } else {
    // Move between items
    const prevOrder = currentItems[toIndex > fromIndex ? toIndex : toIndex - 1]?.order ?? 0;
    const nextOrder = currentItems[toIndex > fromIndex ? toIndex + 1 : toIndex]?.order ?? 0;
    newOrder = Math.round((prevOrder + nextOrder) / 2);
  }
  
  try {
    await gallery.updateItem(gallery.activeCollectionId, item.id, { order: newOrder });
    message.success('已调整顺序');
  } catch (error: any) {
    message.error(error?.message || '调整顺序失败');
  }
}

async function handleBatchDelete() {
  if (selectedIds.value.length === 0) return;
  if (!gallery.activeCollectionId) return;
  
  const confirmed = await dialogAskConfirm(
    dialog,
    `确认删除 ${selectedIds.value.length} 个资源？`,
    '删除后无法恢复，请谨慎操作'
  );
  if (!confirmed) return;
  
  try {
    await gallery.deleteItems(gallery.activeCollectionId, selectedIds.value);
    message.success(`已删除 ${selectedIds.value.length} 个项目`);
    clearSelection();
  } catch (error: any) {
    message.error(error?.message || '删除失败');
  }
}

function handleBatchInsert() {
  if (selectedIds.value.length === 0) return;
  const selectedItems = items.value.filter(item => selectedIds.value.includes(item.id));
  for (const item of selectedItems) {
    const src = item.attachmentId ? `id:${item.attachmentId}` : '';
    if (src) emit('insert', src);
  }
  clearSelection();
}

function handleItemEdit(item: GalleryItem) {
  editingItem.value = item;
  editRemark.value = item.remark || '';
  editModalVisible.value = true;
}

async function handleEditSubmit() {
  if (!editingItem.value || !gallery.activeCollectionId) {
    return false;
  }
  const remark = editRemark.value.trim();
  if (!remark) {
    message.warning('备注不能为空');
    return false;
  }
  if (!remarkPattern.test(remark)) {
    message.warning('备注仅支持字母、数字和下划线，长度不超过64');
    return false;
  }
  editingRemark.value = true;
  try {
    await gallery.updateItem(gallery.activeCollectionId, editingItem.value.id, { remark });
    message.success('备注已更新');
    editModalVisible.value = false;
    editingItem.value = null;
    return true;
  } catch (error: any) {
    console.error('更新备注失败', error);
    message.error(error?.message || '更新失败，请稍后再试');
    return false;
  } finally {
    editingRemark.value = false;
  }
}

async function handleRenameSubmit() {
  const collectionIdToRename = renamingCollectionId.value;
  if (!collectionIdToRename) return false;
  const name = renameCollectionName.value.trim();
  if (!name) {
    message.warning('分类名称不能为空');
    return false;
  }
  renamingCollection.value = true;
  try {
    await gallery.updateCollection(userId.value, collectionIdToRename, { name });
    message.success('分类已重命名');
    renameModalVisible.value = false;
    return true;
  } catch (error: any) {
    message.error(error?.message || '重命名失败');
    return false;
  } finally {
    renamingCollection.value = false;
  }
}

function handleRenameCancel() {
  if (renamingCollection.value) return false;
  renameModalVisible.value = false;
  return true;
}

function handleEditCancel() {
  if (editingRemark.value) {
    return false;
  }
  editModalVisible.value = false;
  editingItem.value = null;
  return true;
}

async function handleItemDelete(item: GalleryItem) {
  if (!gallery.activeCollectionId) {
    return;
  }
  try {
    if (!(await dialogAskConfirm(dialog, '确认删除该资源？', '删除后无法恢复，请谨慎操作'))) {
      return;
    }
    await gallery.deleteItems(gallery.activeCollectionId, [item.id]);
    message.success('已删除');
  } catch (error: any) {
    console.error('删除失败', error);
    message.error(error?.message || '删除失败，请稍后再试');
  }
}

async function handleCreateSubmit() {
  if (!userId.value) {
    message.warning('请先登录');
    return false;
  }
  const name = newCollectionName.value.trim();
  if (!name) {
    message.warning('请输入分类名称');
    return false;
  }
  creatingCollection.value = true;
  try {
    const created = await gallery.createCollection(userId.value, {
      name,
      order: newCollectionOrder.value ?? 0,
    });
    await gallery.setActiveCollection(created.id);
    message.success('分类创建成功');
    createModalVisible.value = false;
    return true;
  } catch (error: any) {
    console.error('创建分类失败', error);
    message.error(error?.message || '创建失败，请稍后再试');
    return false;
  } finally {
    creatingCollection.value = false;
  }
}

function handleCreateCancel() {
  if (creatingCollection.value) {
    return false;
  }
  createModalVisible.value = false;
  return true;
}

function toggleEmojiLink() {
  if (!userId.value) {
    message.warning('请先登录');
    return;
  }
  if (!gallery.activeCollectionId) return;
  const linked = isEmojiLinked.value;
  gallery.linkEmojiCollection(gallery.activeCollectionId, userId.value, !linked);
  message.success(linked ? '已取消表情联动' : '已添加表情联动');
}
</script>

<style scoped>
.gallery-drawer :deep(.n-drawer),
.gallery-drawer :deep(.n-drawer-body) {
  background-color: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  transition: background-color 0.25s ease, color 0.25s ease;
}

.gallery-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
}

.gallery-header__back {
  margin-right: auto;
}

.gallery-header__title {
  font-weight: 600;
  flex: 1;
}

.gallery-panel {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 16px;
  height: 100%;
}

.gallery-panel__content {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.gallery-panel__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.gallery-panel__toolbar-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

/* Batch operations toolbar */
.gallery-panel__batch-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background-color: var(--sc-chip-bg, rgba(99, 102, 241, 0.1));
  border-radius: 8px;
  gap: 12px;
}

.gallery-panel__batch-count {
  font-size: 14px;
  font-weight: 500;
  color: var(--sc-primary, var(--primary-color));
}

.gallery-panel__batch-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* Upload progress */
.gallery-panel__progress {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
}

.gallery-panel__progress-text {
  font-size: 13px;
  color: var(--sc-text-secondary, var(--text-color-2));
  white-space: nowrap;
}

/* Move modal */
.move-modal__content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.move-modal__hint {
  margin: 0;
  color: var(--sc-text-secondary, var(--text-color-2));
}

@media (max-width: 768px) {
  .gallery-drawer :deep(.n-drawer) {
    max-height: 100vh;
    max-height: 100dvh;
  }

  .gallery-drawer :deep(.n-drawer-body-content-wrapper) {
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  .gallery-header {
    width: 100%;
  }

  .gallery-header__back {
    font-size: 14px;
  }

  .gallery-panel {
    grid-template-columns: 1fr;
    gap: 12px;
    height: calc(100vh - 60px);
    height: calc(100dvh - 60px);
    overflow-y: auto;
  }

  .gallery-panel__content {
    min-height: 0;
    flex: 1;
    overflow: hidden;
  }

  .gallery-panel__toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .gallery-panel__toolbar-actions {
    width: 100%;
  }

  .gallery-panel__toolbar-actions > * {
    flex: 1;
  }

  .gallery-panel__batch-toolbar {
    flex-direction: column;
    align-items: stretch;
    padding: 6px 10px;
  }

  .gallery-panel__batch-actions {
    justify-content: center;
    flex-wrap: wrap;
  }

  .gallery-panel__batch-actions .n-button {
    flex: 1;
    min-width: 60px;
  }
}
</style>
