<template>
  <n-drawer
    class="gallery-drawer"
    :show="visible"
    placement="right"
    :width="drawerWidth"
    @update:show="handleShow"
  >
    <n-drawer-content title="快捷画廊" closable>
      <div class="gallery-panel">
        <GalleryCollectionTree
          :collections="collections"
          :active-id="gallery.activeCollectionId"
          @select="handleCollectionSelect"
          @context-action="handleCollectionAction"
        >
          <template #actions>
            <n-button size="small" type="primary" block @click="emitCreateCollection">新建分类</n-button>
            <n-button
              v-if="gallery.activeCollectionId"
              size="small"
              tertiary
              block
              @click="toggleEmojiLink"
            >
              {{ isEmojiLinked ? '取消表情联动' : '设为表情联动分类' }}
            </n-button>
          </template>
        </GalleryCollectionTree>

        <div class="gallery-panel__content">
          <div class="gallery-panel__toolbar">
            <GalleryUploadZone :disabled="uploading" @select="handleUploadSelect" />
            <div class="gallery-panel__toolbar-actions">
              <n-input
                v-model:value="keyword"
                size="small"
                placeholder="搜索备注"
                clearable
                @clear="loadActiveItems"
                @keyup.enter="loadActiveItems"
                style="width: 200px"
              />
              <n-button size="small" :loading="loading" @click="loadActiveItems">刷新</n-button>
            </div>
          </div>

          <GalleryGrid
            :items="items"
            :loading="loading"
            :editable="true"
            @select="handleItemSelect"
            @edit="handleItemEdit"
            @delete="handleItemDelete"
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
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { NDrawer, NDrawerContent, NButton, NInput, useMessage, useDialog, NModal, NForm, NFormItem, NInputNumber } from 'naive-ui';
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

const visible = computed(() => gallery.isPanelVisible);
const drawerWidth = computed(() => {
  if (typeof window === 'undefined') return 720;
  return window.innerWidth < 768 ? '100%' : 720;
});

const userId = computed(() => gallery.activeOwner?.id || user.info.id || '');
const collections = computed(() => (userId.value ? gallery.getCollections(userId.value) : []));
const items = computed(() => (gallery.activeCollectionId ? gallery.getItemsByCollection(gallery.activeCollectionId) : []));
const loading = computed(() => (gallery.activeCollectionId ? gallery.isCollectionLoading(gallery.activeCollectionId) : false));
const isEmojiLinked = computed(() => gallery.emojiCollectionId === gallery.activeCollectionId);

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
    const confirmed = await dialogAskConfirm(`确定删除分类"${collection.name}"吗？此操作不可恢复。`);
    if (confirmed) {
      try {
        await gallery.deleteCollection(collection.id);
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
  try {
    const payloadItems: UploadTask[] = [];
    for (const file of candidates) {
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
      await gallery.upload(collectionId, {
        collectionId,
        items: payloadItems.map((item, index) => ({
          attachmentId: item.attachmentId,
          thumbData: item.thumbData,
          remark: item.remark,
          order: Date.now() + index,
        })),
      });
      message.success('上传成功');
      keyword.value = '';
    }
  } catch (error: any) {
    console.error('画廊上传失败', error);
    message.error(error?.message || '上传失败，请稍后重试');
  } finally {
    uploading.value = false;
  }
}

function loadActiveItems() {
  if (gallery.activeCollectionId) {
    void gallery.loadItems(gallery.activeCollectionId, { keyword: keyword.value || undefined });
  }
}

function handleItemSelect(item: GalleryItem) {
  const src = item.attachmentId ? `id:${item.attachmentId}` : '';
  if (!src) return;
  emit('insert', src);
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
  if (!renamingCollectionId.value) return false;
  const name = renameCollectionName.value.trim();
  if (!name) {
    message.warning('分类名称不能为空');
    return false;
  }
  renamingCollection.value = true;
  try {
    await gallery.updateCollection(renamingCollectionId.value, { name });
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
  if (gallery.activeCollectionId === gallery.emojiCollectionId) {
    gallery.linkEmojiCollection(null, userId.value);
    message.success('已取消表情联动');
  } else if (gallery.activeCollectionId) {
    gallery.linkEmojiCollection(gallery.activeCollectionId, userId.value);
    message.success('已设置为表情联动分类');
  }
}
</script>

<style scoped>
.gallery-drawer :deep(.n-drawer),
.gallery-drawer :deep(.n-drawer-body) {
  background-color: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  transition: background-color 0.25s ease, color 0.25s ease;
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

@media (max-width: 768px) {
  .gallery-panel {
    grid-template-columns: 1fr;
    gap: 12px;
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
}
</style>
