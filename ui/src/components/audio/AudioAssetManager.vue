<template>
  <div class="audio-library">
    <section class="audio-library__toolbar">
      <div class="audio-library__filters">
        <n-input
          v-model:value="keyword"
          size="small"
          clearable
          placeholder="搜索名称 / 标签 / 描述"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <n-icon size="16">
              <SearchOutline />
            </n-icon>
          </template>
        </n-input>

        <n-select
          v-model:value="selectedTags"
          multiple
          filterable
          tag
          size="small"
          placeholder="标签筛选"
          :options="tagOptions"
          class="audio-library__filter-item"
        />

        <n-select
          v-model:value="selectedCreators"
          multiple
          filterable
          size="small"
          placeholder="上传者"
          :options="creatorOptions"
          class="audio-library__filter-item"
        />

        <n-select
          v-model:value="selectedScope"
          size="small"
          placeholder="作用域"
          :options="scopeOptions"
          class="audio-library__filter-item"
        />

        <div class="audio-library__duration">
          <label>时长 (秒)</label>
          <n-slider v-model:value="durationRange" :max="durationMax" :min="0" range size="small" />
        </div>

        <div class="audio-library__filter-actions">
          <n-button size="small" @click="handleResetFilters">重置</n-button>
          <n-button type="primary" size="small" @click="handleSearch">应用</n-button>
        </div>
      </div>

      <div class="audio-library__toolbar-actions">
        <n-button size="small" quaternary @click="handleRefresh" :loading="audio.assetsLoading">
          <template #icon>
            <n-icon size="16">
              <ReloadOutline />
            </n-icon>
          </template>
          刷新列表
        </n-button>
        <n-button size="small" secondary @click="openUploadPanel" v-if="audio.canManage">
          <template #icon>
            <n-icon size="16">
              <CloudUploadOutline />
            </n-icon>
          </template>
          上传素材
        </n-button>
        <n-button size="small" type="primary" @click="openCreateFolder" v-if="audio.canManage">
          <template #icon>
            <n-icon size="16">
              <FolderOpenOutline />
            </n-icon>
          </template>
          新建文件夹
        </n-button>
        <n-button size="small" secondary @click="openAssetManagement" v-if="audio.canManage">
          素材管理
        </n-button>
      </div>

      <n-alert v-if="audio.networkMode !== 'normal'" class="audio-library__alert" type="warning" closable>
        当前处于弱网模式，素材加载将优先使用本地缓存，建议手动刷新确认最新数据。
      </n-alert>
    </section>

    <section v-if="hasSelection" class="audio-library__selection">
      <div>
        已选 {{ selectionCount }} 项
        <n-button text size="tiny" @click="clearSelection">清空</n-button>
      </div>
      <n-space size="small">
        <n-button size="small" @click="openBatchMoveModal" :loading="audio.assetBulkLoading" secondary>
          批量移动
        </n-button>
        <n-button size="small" @click="openBatchVisibilityModal" :loading="audio.assetBulkLoading">
          批量修改可见性
        </n-button>
        <n-button
          v-if="audio.isSystemAdmin"
          size="small"
          @click="openBatchScopeModal"
          :loading="audio.assetBulkLoading"
        >
          批量修改级别
        </n-button>
        <n-button
          size="small"
          type="error"
          @click="confirmBatchDelete"
          :loading="audio.assetBulkLoading"
        >
          批量删除
        </n-button>
      </n-space>
    </section>

    <section class="audio-library__content" :class="contentClassNames">
      <aside v-if="!isMobileLayout" class="audio-library__folders" :class="{ 'is-collapsed': folderPanelCollapsed }">
        <div class="audio-library__panel-top">
          <div class="audio-library__panel-title">
            <span v-if="!folderPanelCollapsed">文件夹</span>
            <n-button quaternary size="tiny" @click="toggleFolderPanel">
              {{ folderPanelCollapsed ? '展开' : '收起' }}
            </n-button>
          </div>
          <div class="audio-library__folder-actions" v-if="audio.canManage && !folderPanelCollapsed">
            <n-button quaternary size="tiny" @click="openCreateFolder">新建</n-button>
            <n-button quaternary size="tiny" @click="openAssetManagement">素材管理</n-button>
            <n-button quaternary size="tiny" :disabled="!currentFolder" @click="openRenameFolder">重命名</n-button>
            <n-button
              quaternary
              size="tiny"
              :disabled="!currentFolder"
              type="error"
              @click="confirmDeleteFolder"
            >
              删除
            </n-button>
          </div>
        </div>
        <n-tree
          v-if="!folderPanelCollapsed"
          block-line
          :data="folderTreeData"
          :default-expanded-keys="['all']"
          selectable
          :selected-keys="folderKeys"
          :node-props="treeNodeProps"
          @update:selected-keys="handleFolderSelect"
        />
        <div v-else class="audio-library__panel-collapsed">
          <n-button quaternary circle size="large" @click="toggleFolderPanel">
            <template #icon>
              <n-icon size="18">
                <MenuOutline />
              </n-icon>
            </template>
          </n-button>
          <span class="audio-library__panel-collapsed-label">文件夹</span>
        </div>
      </aside>

      <section
        class="audio-library__table"
        :class="{ 'is-drag-over': dragUploadActive }"
        @dragenter.prevent="handleListDragEnter"
        @dragover.prevent="handleListDragOver"
        @dragleave.prevent="handleListDragLeave"
        @drop.prevent="handleListDrop"
      >
        <div class="audio-library__table-top">
          <div class="audio-library__table-summary">
            <strong>{{ audio.assetPagination.total || tableData.length }}</strong>
            <span>条素材</span>
          </div>
          <div class="audio-library__table-actions">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  size="small"
                  :class="['audio-library__manual-sort-toggle', manualSortEnabled && 'is-active']"
                  :secondary="manualSortEnabled"
                  :quaternary="!manualSortEnabled"
                  :type="manualSortEnabled ? 'primary' : 'default'"
                  @click="toggleManualSort"
                >
                  手动排序
                </n-button>
              </template>
              {{ manualSortTooltip }}
            </n-tooltip>
            <n-button quaternary size="small" @click="toggleFolderPanel">
              {{ isMobileLayout ? '文件夹' : folderPanelCollapsed ? '展开文件夹' : '收起文件夹' }}
            </n-button>
            <n-button quaternary size="small" @click="toggleDetailPanel" v-if="!isMobileLayout">
              {{ detailPanelCollapsed ? '展开详情' : '收起详情' }}
            </n-button>
            <n-button quaternary size="small" @click="openDetailPanel" v-else>
              查看详情
            </n-button>
          </div>
        </div>

        <div v-if="dragUploadActive" class="audio-library__drop-overlay">
          <div class="audio-library__drop-overlay-card">
            <strong>释放文件以上传素材</strong>
            <span>支持 OGG / MP3 / WAV，上传后自动进入当前文件夹</span>
          </div>
        </div>

        <n-data-table
          size="small"
          :columns="columns"
          :data="tableData"
          :loading="audio.assetsLoading"
          :row-key="rowKey"
          :row-class-name="rowClassName"
          :row-props="rowProps"
          :checked-row-keys="checkedRowKeys"
          @update:checked-row-keys="handleCheckedRowKeysChange"
          bordered
        />
        <div class="audio-library__pagination">
          <n-pagination
            size="small"
            :page="audio.assetPagination.page"
            :page-size="audio.assetPagination.pageSize"
            :item-count="audio.assetPagination.total"
            :page-sizes="[10, 20, 30, 50]"
            show-size-picker
            @update:page="audio.setAssetPage"
            @update:page-size="audio.setAssetPageSize"
          />
        </div>
      </section>

      <section v-if="!isMobileLayout && !detailPanelCollapsed" class="audio-library__detail">
        <component :is="detailContent" />
      </section>
    </section>

    <n-drawer
      :show="folderDrawerVisible"
      placement="left"
      :width="folderDrawerWidth"
      @update:show="folderDrawerVisible = $event"
    >
      <n-drawer-content>
        <template #header>
          <div class="audio-library__drawer-header">
            <n-button v-if="isMobileLayout" quaternary size="tiny" @click="folderDrawerVisible = false">返回</n-button>
            <span>文件夹</span>
            <n-button quaternary size="tiny" @click="folderDrawerVisible = false">关闭</n-button>
          </div>
        </template>
          <div class="audio-library__drawer-panel">
          <p v-if="isMobileLayout" class="audio-library__mobile-tip">
            点击文件夹进入内容，开启“编辑文件夹”后点击查看详情
          </p>
          <div class="audio-library__panel-top">
            <div class="audio-library__panel-title">
              <span>文件夹</span>
            </div>
            <div class="audio-library__folder-actions" v-if="audio.canManage">
              <n-button quaternary size="tiny" @click="openCreateFolder">新建</n-button>
              <n-button quaternary size="tiny" :disabled="!currentFolder" @click="openRenameFolder">重命名</n-button>
              <n-button
                quaternary
                size="tiny"
                :disabled="!currentFolder"
                type="error"
                @click="confirmDeleteFolder"
              >
                删除
              </n-button>
              <n-button
                quaternary
                size="tiny"
                :type="folderEditMode ? 'primary' : 'default'"
                @click="folderEditMode = !folderEditMode"
              >
                {{ folderEditMode ? '完成编辑' : '编辑文件夹' }}
              </n-button>
            </div>
          </div>
          <n-tree
            block-line
            :data="folderTreeData"
            :default-expanded-keys="['all']"
            selectable
            :selected-keys="folderKeys"
            :node-props="treeNodeProps"
            @update:selected-keys="handleFolderSelect"
          />
        </div>
      </n-drawer-content>
    </n-drawer>

    <n-drawer
      :show="detailDrawerVisible"
      placement="right"
      :width="detailDrawerWidth"
      @update:show="detailDrawerVisible = $event"
    >
      <n-drawer-content>
        <template #header>
          <div class="audio-library__drawer-header">
            <n-button v-if="isMobileLayout" quaternary size="tiny" @click="detailDrawerVisible = false">返回</n-button>
            <span>素材详情</span>
            <n-button v-if="isMobileLayout" quaternary size="tiny" @click="detailDrawerVisible = false">关闭</n-button>
          </div>
        </template>
        <component :is="detailContent" />
      </n-drawer-content>
    </n-drawer>

    <n-drawer
      :show="uploadDrawerVisible"
      placement="right"
      :width="uploadDrawerWidth"
      :destroy-on-close="true"
      @update:show="uploadDrawerVisible = $event"
    >
      <n-drawer-content>
        <template #header>
          <div class="audio-asset-drawer__header">
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="uploadDrawerVisible = false">
              退出
            </n-button>
            <span>上传素材</span>
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="uploadDrawerVisible = false">
              <template #icon>
                <n-icon size="16">
                  <CloseOutline />
                </n-icon>
              </template>
            </n-button>
          </div>
        </template>
        <UploadPanel compact />
      </n-drawer-content>
    </n-drawer>

    <n-drawer
      :show="assetDrawerVisible"
      placement="right"
      :width="assetDrawerWidth"
      :mask-closable="true"
      @update:show="assetDrawerVisible = $event"
    >
      <n-drawer-content>
        <template #header>
          <div class="audio-asset-drawer__header">
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="assetDrawerVisible = false">
              退出
            </n-button>
            <span>编辑素材</span>
            <n-button v-if="isMobileLayout" size="tiny" quaternary @click="assetDrawerVisible = false">
              <template #icon>
                <n-icon size="16">
                  <CloseOutline />
                </n-icon>
              </template>
            </n-button>
          </div>
        </template>
        <n-form ref="assetFormRef" :model="assetForm" :rules="assetFormRules" label-placement="top">
          <n-form-item label="名称" path="name">
            <n-input v-model:value="assetForm.name" maxlength="60" show-count />
          </n-form-item>
          <n-form-item label="备注" path="description">
            <n-input v-model:value="assetForm.description" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" />
          </n-form-item>
          <n-form-item label="标签" path="tags">
            <n-select
              v-model:value="assetForm.tags"
              multiple
              filterable
              tag
              placeholder="输入或选择标签"
              :options="tagOptions"
            />
          </n-form-item>
          <n-form-item label="所属文件夹" path="folderId">
            <n-tree-select
              v-model:value="assetForm.folderId"
              :options="folderSelectOptions"
              clearable
              placeholder="未分类"
            />
          </n-form-item>
        <n-form-item label="可见性" path="visibility">
          <n-radio-group v-model:value="assetForm.visibility">
            <n-radio value="public">公开</n-radio>
            <n-radio value="restricted">受限</n-radio>
          </n-radio-group>
        </n-form-item>
        <template v-if="audio.isSystemAdmin">
          <n-form-item label="素材级别" path="scope">
            <n-radio-group v-model:value="assetForm.scope">
              <n-radio value="common">通用级</n-radio>
              <n-radio value="world">世界级</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-form-item v-if="assetForm.scope === 'world'" label="所属世界" path="worldId">
            <n-select v-model:value="assetForm.worldId" filterable :options="worldOptions" placeholder="选择世界" />
          </n-form-item>
        </template>
      </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="assetDrawerVisible = false">取消</n-button>
            <n-button type="primary" :loading="audio.assetMutationLoading" @click="handleSaveAsset">
              保存
            </n-button>
          </n-space>
        </template>
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="folderModalVisible" preset="dialog" :title="folderModalTitle" :mask-closable="false">
      <n-form ref="folderFormRef" :model="folderForm" :rules="folderFormRules" label-placement="top">
        <n-form-item label="名称" path="name">
          <n-input v-model:value="folderForm.name" maxlength="40" show-count />
        </n-form-item>
        <n-form-item label="上级" path="parentId">
          <n-tree-select
            v-model:value="folderForm.parentId"
            :options="folderSelectOptions"
            default-expand-all
            clearable
            placeholder="根目录"
          />
        </n-form-item>
        <template v-if="(folderModalMode === 'create' || folderModalMode === 'edit') && audio.isSystemAdmin">
          <n-form-item label="级别" path="scope">
            <n-radio-group v-model:value="folderForm.scope">
              <n-radio value="common">通用级</n-radio>
              <n-radio value="world">世界级</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-form-item v-if="folderForm.scope === 'world'" label="所属世界" path="worldId">
            <n-select v-model:value="folderForm.worldId" filterable :options="worldOptions" placeholder="选择世界" />
          </n-form-item>
        </template>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="folderModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="audio.folderActionLoading" @click="handleSaveFolder">
            保存
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="batchMoveModalVisible" preset="dialog" title="批量移动素材" :mask-closable="false">
      <p class="audio-library__modal-tip">选择目标文件夹，未选择则移动到未分类</p>
      <n-tree-select
        v-model:value="batchMoveTarget"
        :options="folderSelectOptions"
        clearable
        placeholder="未分类"
      />
      <template #action>
        <n-space justify="end">
          <n-button @click="batchMoveModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="audio.assetBulkLoading" @click="handleBatchMoveSave">
            确认
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="batchVisibilityModalVisible" preset="dialog" title="批量修改可见性" :mask-closable="false">
      <p class="audio-library__modal-tip">将已选素材设置为统一的可见性状态</p>
      <n-radio-group v-model:value="batchVisibilityValue">
        <n-radio value="public">公开</n-radio>
        <n-radio value="restricted">受限</n-radio>
      </n-radio-group>
      <template #action>
        <n-space justify="end">
          <n-button @click="batchVisibilityModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="audio.assetBulkLoading" @click="handleBatchVisibilitySave">
            确认
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="batchScopeModalVisible" preset="dialog" title="批量修改素材级别" :mask-closable="false">
      <p class="audio-library__modal-tip">世界级素材将归属当前世界</p>
      <n-radio-group v-model:value="batchScopeValue">
        <n-radio value="common">通用级</n-radio>
        <n-radio value="world">世界级</n-radio>
      </n-radio-group>
      <template #action>
        <n-space justify="end">
          <n-button @click="batchScopeModalVisible = false">取消</n-button>
          <n-button type="primary" :loading="audio.assetBulkLoading" @click="handleBatchScopeSave">
            确认
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <AudioAssetManagementDialog
      v-model:show="assetManagementVisible"
      endpoint-base="/api/v1/audio/manage/assets"
      title="音频素材管理"
      show-quota
      @changed="handleAssetManagementChanged"
    />
  </div>
</template>

<script setup lang="ts">
import { SearchOutline, CloudUploadOutline, FolderOpenOutline, ReloadOutline, TrashOutline, CreateOutline, CopyOutline, MenuOutline, CloseOutline } from '@vicons/ionicons5';
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue';
import {
  NButton,
  NSpace,
  NTag,
  NTooltip,
  useDialog,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormRules,
  type TreeOption,
} from 'naive-ui';
import { useWindowSize } from '@vueuse/core';
import type {
  AudioAsset,
  AudioAssetMutationPayload,
  AudioAssetScope,
  AudioBulkDeleteFailure,
  AudioDeleteConflictPayload,
  AudioDeleteImpact,
  AudioAssetUsageSummary,
  AudioSearchFilters,
  AudioFolder,
  AudioFolderPayload,
} from '@/types/audio';
import { api } from '@/stores/_config';
import { useAudioStudioStore } from '@/stores/audioStudio';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { copyTextWithResult } from '@/utils/clipboard';
import UploadPanel from './UploadPanel.vue';
import AudioAssetManagementDialog from './AudioAssetManagementDialog.vue';

const audio = useAudioStudioStore();
const chat = useChatStore();
const user = useUserStore();
const message = useMessage();
const dialog = useDialog();

const durationMax = 600;
const keyword = ref(audio.filters.query ?? '');
const selectedTags = ref<string[]>([...audio.filters.tags]);
const selectedCreators = ref<string[]>([...audio.filters.creatorIds]);
const selectedScope = ref<AudioAssetScope | 'all'>(audio.filters.scope ?? 'all');
const durationRange = ref<[number, number]>(audio.filters.durationRange ?? [0, durationMax]);
const folderKeys = ref<string[]>(audio.filters.folderId ? [audio.filters.folderId] : ['all']);
const folderModalVisible = ref(false);
const folderModalMode = ref<'create' | 'rename' | 'edit'>('create');
const folderModalTitle = computed(() => {
  if (folderModalMode.value === 'create') return '新建文件夹';
  if (folderModalMode.value === 'edit') return '编辑文件夹元数据';
  return '重命名文件夹';
});
const checkedRowKeys = ref<string[]>([]);
const batchMoveModalVisible = ref(false);
const batchMoveTarget = ref<string | null>(null);
const batchVisibilityModalVisible = ref(false);
const batchVisibilityValue = ref<'public' | 'restricted'>('public');
const batchScopeModalVisible = ref(false);
const batchScopeValue = ref<AudioAssetScope>('common');
const folderFormRef = ref<FormInst | null>(null);
const folderForm = reactive({
  id: '',
  name: '',
  parentId: null as string | null,
  scope: null as AudioAssetScope | null,
  worldId: null as string | null,
});
const folderFormRules: FormRules = {
  name: [
    {
      required: true,
      message: '请输入文件夹名称',
      trigger: 'blur',
    },
  ],
};

const assetDrawerVisible = ref(false);
const assetFormRef = ref<FormInst | null>(null);
const assetForm = reactive({
  id: '',
  name: '',
  description: '',
  tags: [] as string[],
  folderId: null as string | null,
  visibility: 'public' as 'public' | 'restricted',
  scope: 'common' as AudioAssetScope,
  worldId: null as string | null,
});
const assetFormRules: FormRules = {
  name: [{ required: true, message: '名称不能为空', trigger: 'blur' }],
};

const detailFocus = ref<'asset' | 'folder'>('asset');
const userLabelCache = reactive<Record<string, string>>({});
const userLookupPending = new Set<string>();
const folderPanelCollapsed = ref(false);
const detailPanelCollapsed = ref(false);
const detailDrawerVisible = ref(false);
const folderDrawerVisible = ref(false);
const uploadDrawerVisible = ref(false);
const assetManagementVisible = ref(false);
const dragUploadActive = ref(false);
const dragUploadDepth = ref(0);
const draggingAssetId = ref<string | null>(null);
const dragOverAssetId = ref<string | null>(null);
const folderEditMode = ref(false);

const { width: viewportWidth } = useWindowSize();
const isMobileLayout = computed(() => viewportWidth.value > 0 && viewportWidth.value < 640);
const assetDrawerWidth = computed(() => (isMobileLayout.value ? '100%' : 360));
const folderDrawerWidth = computed(() => (isMobileLayout.value ? '88vw' : 320));
const detailDrawerWidth = computed(() => (isMobileLayout.value ? '100%' : 380));
const uploadDrawerWidth = computed(() => (isMobileLayout.value ? '100%' : 460));

const tableData = computed(() => audio.filteredAssets);
const selectedAsset = computed(() => audio.selectedAsset);
const assetWorldLabel = computed(() => {
  const worldId = selectedAsset.value?.worldId;
  if (!worldId) return '全局 (common)';
  const world = chat.worldMap?.[worldId];
  const name = world?.name || '未知世界';
  return `${name} (${worldId})`;
});
const folderWorldLabel = computed(() => {
  const worldId = currentFolder.value?.worldId;
  if (!worldId) return '全局 (common)';
  const world = chat.worldMap?.[worldId];
  const name = world?.name || '未知世界';
  return `${name} (${worldId})`;
});
const selectionCount = computed(() => checkedRowKeys.value.length);
const hasSelection = computed(() => selectionCount.value > 0);
const checkedAssets = computed(() => {
  const map = new Map(tableData.value.map((item) => [item.id, item] as const));
  return checkedRowKeys.value
    .map((id) => map.get(id))
    .filter((item): item is AudioAsset => Boolean(item));
});

const folderMap = computed(() => {
  const map = new Map<string, AudioFolder>();
  const traverse = (items: AudioFolder[]) => {
    items.forEach((folder) => {
      map.set(folder.id, folder);
      if (folder.children?.length) {
        traverse(folder.children);
      }
    });
  };
  traverse(audio.folders || []);
  return map;
});

const currentFolder = computed(() => {
  const key = folderKeys.value[0];
  if (!key || key === 'all') return null;
  return folderMap.value.get(key) ?? null;
});

const folderTreeData = computed<TreeOption[]>(() => {
  const build = (folders: AudioFolder[]): TreeOption[] =>
    folders.map((folder) => ({
      key: folder.id,
      label: folder.name,
      children: folder.children ? build(folder.children) : undefined,
    }));
  return [
    {
      key: 'all',
      label: '全部素材',
      children: build(audio.folders || []),
    },
  ];
});

const folderSelectOptions = computed(() => {
  const build = (folders: AudioFolder[]): TreeOption[] =>
    folders.map((folder) => ({
      key: folder.id,
      label: folder.name,
      value: folder.id,
      children: folder.children ? build(folder.children) : undefined,
    }));
  return build(audio.folders || []);
});

const tagOptions = computed(() => {
  const tags = new Set<string>();
  audio.assets.forEach((asset) => {
    const assetTags = Array.isArray(asset.tags) ? asset.tags : [];
    assetTags.forEach((tag) => tags.add(tag));
  });
  return Array.from(tags).map((tag) => ({ label: tag, value: tag }));
});

const creatorOptions = computed(() => {
  const creators = Array.from(new Set(audio.assets.map((asset) => asset.createdBy)));
  return creators.map((creator) => ({ label: creator, value: creator }));
});

const scopeOptions = computed(() => [
  { label: '全部', value: 'all' },
  { label: '通用级', value: 'common' },
  { label: '世界级', value: 'world' },
]);

const canEditSelectedAsset = computed(() => {
  if (!selectedAsset.value) return false;
  return audio.canEditAsset(selectedAsset.value);
});

const selectedAssetTags = computed(() => {
  if (!selectedAsset.value || !Array.isArray(selectedAsset.value.tags)) return [];
  return selectedAsset.value.tags;
});

const dragUploadScope = computed<AudioAssetScope>(() => {
  if (!audio.isSystemAdmin) return 'world';
  if (currentFolder.value?.scope) return currentFolder.value.scope;
  if (selectedScope.value !== 'all') return selectedScope.value;
  if (audio.filters.scope) return audio.filters.scope;
  return audio.currentWorldId ? 'world' : 'common';
});

const contentClassNames = computed(() => ({
  'is-folder-collapsed': folderPanelCollapsed.value,
  'is-detail-collapsed': detailPanelCollapsed.value,
}));
const manualSortEnabled = computed(() => audio.filters.manualSort !== false);
const canReorderAssets = computed(() => audio.canManage && !audio.assetsLoading);
const manualSortTooltip = computed(() =>
  manualSortEnabled.value
    ? '当前排序会叠加手动拖拽顺序。拖拽素材可调整排序号。'
    : '开启后，手动拖拽顺序会作为当前排序的次级规则。'
);

const detailContent = defineComponent({
  name: 'AudioLibraryDetailContent',
  setup() {
    return () => {
      const asset = selectedAsset.value;
      const folder = currentFolder.value;

      if (asset) {
        return h('div', { class: 'audio-library__detail-body' }, [
          h('header', { class: 'audio-library__detail-header' }, [
            h('div', [
              h('h3', asset.name),
              h('p', { class: 'audio-library__detail-subtitle' }, folderLabel(asset.folderId) || '未分类'),
            ]),
            h('div', { class: 'audio-library__detail-tags' }, [
              h(
                NTag,
                { size: 'small', type: asset.scope === 'common' ? 'info' : 'warning' },
                { default: () => (asset.scope === 'common' ? '通用级' : '世界级') }
              ),
              h(
                NTag,
                { size: 'small', type: asset.visibility === 'public' ? 'success' : 'warning' },
                { default: () => (asset.visibility === 'public' ? '公开' : '受限') }
              ),
            ]),
          ]),
          h('div', { class: 'audio-library__detail-actions audio-library__detail-actions--stacked' }, [
            h(
              NButton,
              { size: 'small', quaternary: true, onClick: () => copyStream(asset.id) },
              { default: () => '复制播放链接' }
            ),
            canEditSelectedAsset.value
              ? h(
                  NButton,
                  { size: 'small', secondary: true, onClick: () => openAssetEditor(asset) },
                  { default: () => '编辑元数据' }
                )
              : null,
            audio.canDeleteAsset(asset)
              ? h(
                  NButton,
                  { size: 'small', type: 'error', ghost: true, onClick: () => confirmDeleteAsset(asset) },
                  { default: () => '删除素材' }
                )
              : null,
          ]),
          h('ul', { class: 'audio-library__detail-list' }, [
            h('li', `时长：${formatDuration(asset.duration)}`),
            h('li', `上传者：${formatUserLabel(asset.createdBy)}`),
            h('li', `所属世界：${assetWorldLabel.value}`),
            h('li', `更新时间：${formatDate(asset.updatedAt)}`),
            h('li', `素材级别：${asset.scope === 'common' ? '通用级' : '世界级'}`),
            h('li', `存储：${asset.storageType === 's3' ? '对象存储 (支持跳转)' : '本地文件'}`),
            h('li', `比特率：${asset.bitrate} kbps · 大小：${formatFileSize(asset.size)}`),
          ]),
          h('div', { class: 'audio-library__tags' }, [
            h('strong', '标签：'),
            selectedAssetTags.value.length
              ? h(
                  'div',
                  { class: 'audio-library__tag-list' },
                  selectedAssetTags.value.map((tag) =>
                    h(
                      NTag,
                      { key: tag, size: 'small', class: 'audio-library__tag', bordered: true },
                      { default: () => tag }
                    )
                  )
                )
              : h('span', '未设置'),
          ]),
            h('div', { class: 'audio-library__description' }, [
            h('strong', '备注：'),
            h('p', asset.description || '暂无备注'),
          ]),
        ]);
      }

      if (folder) {
        return h('div', { class: 'audio-library__detail-body' }, [
          h('header', { class: 'audio-library__detail-header' }, [
            h('div', [
              h('h3', folder.name),
              h('p', { class: 'audio-library__detail-subtitle' }, folder.path || '根目录'),
            ]),
            h('div', { class: 'audio-library__detail-tags' }, [
              h(
                NTag,
                { size: 'small', type: folder.scope === 'common' ? 'info' : 'warning' },
                { default: () => (folder.scope === 'common' ? '通用级' : '世界级') }
              ),
            ]),
          ]),
          audio.canManage
            ? h('div', { class: 'audio-library__detail-actions audio-library__detail-actions--stacked' }, [
                h(
                  NButton,
                  { size: 'small', secondary: true, onClick: openEditFolderMeta },
                  { default: () => '编辑元数据' }
                ),
                h(
                  NButton,
                  { size: 'small', quaternary: true, disabled: !folder, onClick: openRenameFolder },
                  { default: () => '重命名文件夹' }
                ),
                h(
                  NButton,
                  { size: 'small', type: 'error', ghost: true, disabled: !folder, onClick: confirmDeleteFolder },
                  { default: () => '删除文件夹' }
                ),
              ])
            : null,
          h('ul', { class: 'audio-library__detail-list' }, [
            h('li', `所属世界：${folderWorldLabel.value}`),
            h('li', `文件夹级别：${folder.scope === 'common' ? '通用级' : '世界级'}`),
            h('li', `创建者：${formatUserLabel(folder.createdBy)}`),
            h('li', `创建时间：${formatDate(folder.createdAt)}`),
            h('li', `更新时间：${formatDate(folder.updatedAt)}`),
          ]),
        ]);
      }

      return h('div', { class: 'audio-library__detail-empty' }, [h('span', '选择一条素材或文件夹以查看详情')]);
    };
  },
});

function renderSortableHeader(label: string, field: NonNullable<AudioSearchFilters['sortBy']>) {
  const active = (audio.filters.sortBy ?? 'updatedAt') === field;
  const glyph = active ? (audio.filters.sortOrder === 'desc' ? '↓' : '↑') : '↕';
  return h(
    'button',
    {
      type: 'button',
      class: ['audio-table__header-sort', active && 'audio-table__header-sort--active'],
      onClick: (event: MouseEvent) => {
        event.stopPropagation();
        void audio.setAssetSort(field);
      },
    },
    [h('span', label), h('span', { class: 'audio-table__header-sort-glyph' }, glyph)]
  );
}

const columns = computed<DataTableColumns<AudioAsset>>(() => [
  {
    type: 'selection',
    multiple: true,
    disabled: (row: AudioAsset) => !audio.canEditAsset(row),
    fixed: 'left',
  },
  {
    title: () => renderSortableHeader('名称', 'name'),
    key: 'name',
    minWidth: 320,
    render: (row) =>
      h('div', { class: 'audio-table__name-wrap' }, [
        h('div', { class: 'audio-table__name' }, [
          h('span', { class: 'audio-table__title' }, row.name),
          h(
            'p',
            { class: 'audio-table__meta' },
            [
              folderLabel(row.folderId) || '未分类',
              row.createdBy ? formatUserLabel(row.createdBy) : '未知上传者',
              row.visibility === 'public' ? '公开' : '受限',
            ].join(' · ')
          ),
          row.description ? h('p', { class: 'audio-table__desc' }, row.description) : null,
          isMobileLayout.value && audio.canEditAsset(row)
            ? h('div', { class: 'audio-table__mobile-reorder' }, [
                h(
                  NButton,
                  { size: 'tiny', quaternary: true, onClick: (event: MouseEvent) => moveAssetByStep(event, row.id, -1) },
                  { default: () => '上移' }
                ),
                h(
                  NButton,
                  { size: 'tiny', quaternary: true, onClick: (event: MouseEvent) => moveAssetByStep(event, row.id, 1) },
                  { default: () => '下移' }
                ),
              ])
            : null,
        ]),
        h('div', { class: 'audio-table__inline-actions' }, [
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              onClick: (event: MouseEvent) => {
                event.stopPropagation();
                copyStream(row.id);
              },
            },
            {
              icon: () => h(CopyOutline),
            }
          ),
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              disabled: !audio.canEditAsset(row),
              onClick: (event: MouseEvent) => {
                event.stopPropagation();
                openAssetEditor(row);
              },
            },
            {
              icon: () => h(CreateOutline),
            }
          ),
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              type: 'error',
              disabled: !audio.canDeleteAsset(row),
              onClick: (event: MouseEvent) => {
                event.stopPropagation();
                confirmDeleteAsset(row);
              },
            },
            {
              icon: () => h(TrashOutline),
            }
          ),
        ]),
      ]),
  },
  {
    title: () => renderSortableHeader('级别', 'scope'),
    key: 'scope',
    width: 84,
    render: (row) => (row.scope === 'common' ? '通用级' : '世界级'),
  },
  {
    title: () => renderSortableHeader('时长', 'duration'),
    key: 'duration',
    width: 76,
    render: (row) => formatDuration(row.duration),
  },
  {
    title: '标签',
    key: 'tags',
    minWidth: 140,
    render: (row) => {
      const tags = Array.isArray(row.tags) ? row.tags : [];
      return tags.length
        ? h(
            NSpace,
            { size: 4, wrap: true },
            {
              default: () =>
                tags.map((tag) =>
                  h(
                    NTag,
                    { size: 'tiny', bordered: false, key: tag },
                    { default: () => tag }
                  )
                ),
            }
          )
        : '-';
    },
  },
  {
    title: () => renderSortableHeader('更新时间', 'updatedAt'),
    key: 'updatedAt',
    width: 148,
    render: (row) => formatDate(row.updatedAt),
  },
]);

const rowKey = (row: AudioAsset) => row.id;
const rowClassName = (row: AudioAsset) =>
  [
    row.id === audio.selectedAssetId ? 'is-selected-row' : '',
    row.id === draggingAssetId.value ? 'is-dragging-row' : '',
    row.id === dragOverAssetId.value ? 'is-drag-over-row' : '',
  ].filter(Boolean).join(' ');
const rowProps = (row: AudioAsset) => ({
  draggable: canReorderAssets.value && audio.canEditAsset(row),
  onClick: () => {
    detailFocus.value = 'asset';
    audio.setSelectedAsset(row.id);
    if (isMobileLayout.value) {
      detailDrawerVisible.value = true;
    }
  },
  onDragstart: (event: DragEvent) => handleAssetDragStart(event, row),
  onDragover: (event: DragEvent) => handleAssetDragOver(event, row),
  onDragleave: () => handleAssetDragLeave(row),
  onDrop: (event: DragEvent) => handleAssetDrop(event, row),
  onDragend: handleAssetDragEnd,
});

const worldOptions = computed(() => {
  const list = Object.values(chat.worldMap || {}) as Array<{ id?: string; name?: string }>;
  return list
    .filter((item) => item && item.id)
    .map((item) => ({ label: item.name || item.id!, value: item.id! }));
});

function handleCheckedRowKeysChange(keys: Array<string | number>) {
  if (!Array.isArray(keys)) {
    checkedRowKeys.value = [];
    return;
  }
  checkedRowKeys.value = keys.map((key) => String(key));
}

function clearSelection() {
  checkedRowKeys.value = [];
}

function folderLabel(folderId: string | null) {
  if (!folderId) return '';
  return audio.folderPathLookup[folderId] || folderMap.value.get(folderId)?.name || '';
}

function getPreferredWorldId(fallback?: string | null) {
  return audio.currentWorldId ?? audio.filters.worldId ?? fallback ?? null;
}

function formatDuration(value: number) {
  if (!value && value !== 0) return '00:00';
  const minutes = Math.floor(value / 60);
  const seconds = Math.floor(value % 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function formatDate(value?: string) {
  if (!value) return '未知';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString();
}

function formatUserLabel(userId?: string | null) {
  const trimmed = (userId ?? '').trim();
  if (!trimmed) return '未知';
  if (user.info?.id === trimmed) {
    return user.info.username || trimmed;
  }
  return userLabelCache[trimmed] || trimmed;
}

async function ensureUserLabel(userId?: string | null) {
  const trimmed = (userId ?? '').trim();
  if (!trimmed) return;
  if (userLabelCache[trimmed] || user.info?.id === trimmed || userLookupPending.has(trimmed)) {
    return;
  }
  userLookupPending.add(trimmed);
  try {
    const resp = await api.get('/api/v1/user-lookup', { params: { userId: trimmed } });
    const data = resp.data?.user;
    const username = data?.username || data?.nick || trimmed;
    userLabelCache[trimmed] = username;
  } catch (err) {
    console.warn('user lookup failed', err);
  } finally {
    userLookupPending.delete(trimmed);
  }
}

function formatFileSize(value: number) {
  if (!value) return '0 B';
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function resolveUsageSummaryText(usage?: AudioAssetUsageSummary | null) {
  if (!usage) return '引用信息未知';
  const parts: string[] = [];
  if (usage.sceneRefCount) {
    const names = usage.sceneNames?.length ? `：${usage.sceneNames.join('、')}` : '';
    parts.push(`${usage.sceneRefCount} 个场景引用${names}`);
  }
  if (usage.playbackStateRefCount) {
    parts.push(`${usage.playbackStateRefCount} 个播放状态正在使用`);
  }
  return parts.length ? parts.join('；') : '当前无引用';
}

function describeDeleteImpact(impact?: AudioDeleteImpact | null) {
  if (!impact) return '';
  const parts: string[] = [];
  if (impact.detachedSceneCount) {
    parts.push(`已解除 ${impact.detachedSceneCount} 个场景引用`);
  }
  if (impact.detachedPlaybackStateCount) {
    parts.push(`已停止并解除 ${impact.detachedPlaybackStateCount} 个播放状态`);
  }
  return parts.join('，');
}

function extractDeleteConflict(error: any): AudioDeleteConflictPayload | null {
  const status = error?.response?.status;
  const data = error?.response?.data as AudioDeleteConflictPayload | undefined;
  if (status !== 409 || !data) {
    return null;
  }
  return data;
}

function extractErrorMessage(error: any, fallback: string) {
  return error?.response?.data?.message || error?.response?.data?.error || error?.message || fallback;
}

function summarizeBatchDeleteFailures(failures: AudioBulkDeleteFailure[]) {
  const grouped = new Map<string, number>();
  failures.forEach((item) => {
    const key = item.reason || '删除失败';
    grouped.set(key, (grouped.get(key) || 0) + 1);
  });
  return Array.from(grouped.entries())
    .map(([reason, count]) => `${count} 条因为${reason}不能删除`)
    .join('；');
}

function summarizeBatchUpdateFailures(failures: Array<{ assetId: string; reason: string }>, actionLabel: string) {
  const grouped = new Map<string, number>();
  failures.forEach((item) => {
    const key = item.reason || `${actionLabel}失败`;
    grouped.set(key, (grouped.get(key) || 0) + 1);
  });
  return Array.from(grouped.entries())
    .map(([reason, count]) => `${count} 条因为${reason}不能${actionLabel}`)
    .join('；');
}

function openBatchDeleteFailureDialog(failures: AudioBulkDeleteFailure[]) {
  dialog.warning({
    title: '批量删除失败详情',
    positiveText: '知道了',
    showIcon: false,
    content: () =>
      h(
        'div',
        { style: 'display:flex;flex-direction:column;gap:8px;max-width:520px;' },
        failures.map((item) =>
          h(
            'div',
            { key: item.assetId, style: 'font-size:13px;line-height:1.5;' },
            `${item.assetId}：${item.reason}${item.usageSummary ? `（${resolveUsageSummaryText(item.usageSummary)}）` : ''}`
          )
        )
      ),
  });
}

function handleSearch() {
  const durationFilter = getDurationFilter();
  audio.applyFilters({
    query: keyword.value,
    tags: selectedTags.value,
    creatorIds: selectedCreators.value,
    durationRange: durationFilter,
    scope: selectedScope.value === 'all' ? undefined : selectedScope.value,
  });
  clearSelection();
}

function handleResetFilters() {
  keyword.value = '';
  selectedTags.value = [];
  selectedCreators.value = [];
  selectedScope.value = 'all';
  durationRange.value = [0, durationMax];
  audio.applyFilters({
    query: '',
    tags: [],
    creatorIds: [],
    durationRange: null,
    sortBy: 'name',
    sortOrder: 'asc',
    scope: undefined,
  });
  clearSelection();
}

function getDurationFilter(): [number, number] | null {
  const [min, max] = durationRange.value;
  if (min <= 0 && max >= durationMax) {
    return null;
  }
  return [min, max];
}

async function handleRefresh() {
  await audio.fetchFolders();
  await audio.fetchAssets();
  message.success('素材列表已刷新');
}

function toggleManualSort() {
  void audio.setManualSortEnabled(!manualSortEnabled.value);
}

function openAssetManagement() {
  assetManagementVisible.value = true;
}

async function handleAssetManagementChanged() {
  await audio.fetchFolders();
  await audio.fetchAssets({ pagination: { page: audio.assetPagination.page }, silent: false });
}

function openUploadPanel() {
  uploadDrawerVisible.value = true;
}

function treeNodeProps() {
  return {
    class: 'audio-library__tree-node',
  };
}

async function handleFolderSelect(keys: Array<string | number>) {
  const target = keys.length ? String(keys[0]) : 'all';
  folderKeys.value = target ? [target] : [];
  clearSelection();
  if (isMobileLayout.value && folderEditMode.value && target !== 'all') {
    detailFocus.value = 'folder';
    audio.setSelectedAsset(null);
    await audio.applyFilters({ folderId: target });
    detailDrawerVisible.value = true;
    return;
  }
  detailFocus.value = 'asset';
  if (target === 'all') {
    if (!audio.selectedAsset && audio.filteredAssets.length) {
      audio.setSelectedAsset(audio.filteredAssets[0].id);
    }
  } else {
    audio.setSelectedAsset(null);
  }
  if (target === 'all') {
    if (isMobileLayout.value) {
      folderDrawerVisible.value = false;
    }
    await audio.applyFilters({ folderId: null });
    return;
  }
  await audio.applyFilters({ folderId: target });
  audio.setSelectedAsset(null);
  if (isMobileLayout.value) {
    folderDrawerVisible.value = false;
  }
}

function openCreateFolder() {
  folderModalMode.value = 'create';
  folderForm.id = '';
  folderForm.name = '';
  folderForm.parentId = currentFolder.value?.id ?? null;
  if (currentFolder.value) {
    folderForm.scope = currentFolder.value.scope;
    folderForm.worldId = folderForm.scope === 'world' ? getPreferredWorldId(currentFolder.value.worldId) : null;
  } else if (audio.isSystemAdmin) {
    const preferredScope = selectedScope.value === 'all' ? audio.filters.scope : selectedScope.value;
    folderForm.scope = preferredScope ?? (audio.currentWorldId ? 'world' : 'common');
    folderForm.worldId = folderForm.scope === 'world' ? getPreferredWorldId() : null;
  } else {
    folderForm.scope = 'world';
    folderForm.worldId = getPreferredWorldId();
  }
  folderModalVisible.value = true;
}

function openRenameFolder() {
  if (!currentFolder.value) return;
  folderModalMode.value = 'rename';
  folderForm.id = currentFolder.value.id;
  folderForm.name = currentFolder.value.name;
  folderForm.parentId = currentFolder.value.parentId;
  folderForm.scope = currentFolder.value.scope;
  folderForm.worldId = currentFolder.value.scope === 'world' ? getPreferredWorldId(currentFolder.value.worldId) : null;
  folderModalVisible.value = true;
}

function openEditFolderMeta() {
  if (!currentFolder.value) return;
  folderModalMode.value = 'edit';
  folderForm.id = currentFolder.value.id;
  folderForm.name = currentFolder.value.name;
  folderForm.parentId = currentFolder.value.parentId;
  folderForm.scope = currentFolder.value.scope;
  folderForm.worldId = currentFolder.value.scope === 'world' ? getPreferredWorldId(currentFolder.value.worldId) : null;
  folderModalVisible.value = true;
}

async function handleSaveFolder() {
  if (!audio.canManage) {
    message.error('没有权限管理文件夹');
    return;
  }
  await folderFormRef.value?.validate();
  try {
    if (folderModalMode.value === 'create') {
      const trimmedName = folderForm.name.trim();
      const targetScope = folderForm.scope ?? (selectedScope.value === 'all' ? audio.filters.scope : selectedScope.value);
      if (folderForm.parentId && currentFolder.value?.scope === 'common' && targetScope === 'world') {
        message.warning('通用文件夹下不能创建世界级子文件夹');
        return;
      }
      const payload: AudioFolderPayload = { name: trimmedName, parentId: folderForm.parentId };
      if (!audio.isSystemAdmin || targetScope === 'world') {
        const worldId =
          folderForm.worldId ?? currentFolder.value?.worldId ?? audio.filters.worldId ?? audio.currentWorldId;
        if (!worldId) {
          message.warning('请先进入一个世界后再创建世界级文件夹');
          return;
        }
        payload.scope = 'world';
        payload.worldId = worldId;
      } else if (targetScope === 'common') {
        payload.scope = 'common';
        payload.worldId = null;
      }
      await audio.createFolder(payload);
      message.success('文件夹已创建');
    } else {
      const payload: Partial<AudioFolderPayload> = {
        name: folderForm.name.trim(),
        parentId: folderForm.parentId,
      };
      if (folderModalMode.value === 'edit' && audio.isSystemAdmin) {
        const targetScope = folderForm.scope ?? currentFolder.value?.scope ?? 'common';
        if (targetScope === 'world') {
          const worldId = folderForm.worldId ?? currentFolder.value?.worldId ?? null;
          if (!worldId) {
            message.warning('世界级文件夹必须指定归属世界');
            return;
          }
          payload.scope = 'world';
          payload.worldId = worldId;
        } else {
          payload.scope = 'common';
          payload.worldId = null;
        }
      }
      await audio.updateFolder(folderForm.id, payload);
      message.success('文件夹已更新');
    }
    folderModalVisible.value = false;
  } catch (err) {
    message.error('保存文件夹失败，请稍后重试');
    console.warn(err);
  }
}

function confirmDeleteFolder() {
  if (!currentFolder.value) return;
  dialog.warning({
    title: '删除确认',
    content: `确定删除“${currentFolder.value.name}”及其子文件夹吗？素材将移动到未分类。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await audio.deleteFolder(currentFolder.value!.id);
        message.success('文件夹已删除');
      } catch (err: any) {
        message.error(err?.response?.data?.message || err?.message || '删除文件夹失败');
        console.warn(err);
      }
    },
  });
}

function openAssetEditor(asset: AudioAsset) {
  if (!audio.canEditAsset(asset)) return;
  assetForm.id = asset.id;
  assetForm.name = asset.name;
  assetForm.description = asset.description || '';
  assetForm.tags = [...asset.tags];
  assetForm.folderId = asset.folderId;
  assetForm.visibility = asset.visibility;
  assetForm.scope = asset.scope;
  assetForm.worldId = asset.scope === 'world' ? getPreferredWorldId(asset.worldId) : null;
  assetDrawerVisible.value = true;
}

async function handleSaveAsset() {
  await assetFormRef.value?.validate();
  try {
    const payload: AudioAssetMutationPayload = {
      name: assetForm.name.trim(),
      description: assetForm.description,
      tags: [...assetForm.tags],
      folderId: assetForm.folderId,
      visibility: assetForm.visibility,
    };
    if (audio.isSystemAdmin) {
      if (assetForm.scope === 'world') {
        if (!assetForm.worldId) {
          message.warning('世界级素材必须指定归属世界');
          return;
        }
        payload.scope = 'world';
        payload.worldId = assetForm.worldId;
      } else {
        payload.scope = 'common';
        payload.worldId = null;
      }
    }
    await audio.updateAssetMeta(assetForm.id, payload);
    message.success('素材信息已保存');
    assetDrawerVisible.value = false;
  } catch (err) {
    console.warn(err);
    message.error('保存失败，请稍后重试');
  }
}

function confirmDeleteAsset(asset: AudioAsset) {
  if (!audio.canDeleteAsset(asset)) return;
  dialog.warning({
    title: '删除素材',
    content: `确定删除“${asset.name}”吗？删除后播放列表将无法引用该素材。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const result = await audio.deleteAsset(asset.id);
        const impactText = describeDeleteImpact(result?.impact);
        message.success(impactText ? `素材已删除，${impactText}` : '素材已删除');
      } catch (error) {
        const conflict = extractDeleteConflict(error);
        if (conflict?.usage) {
          dialog.warning({
            title: '素材仍被引用',
            content: `当前无法直接删除“${asset.name}”。原因：${resolveUsageSummaryText(conflict.usage)}。若继续，系统会停止相关播放、解除引用并删除素材。`,
            positiveText: '解除引用并删除',
            negativeText: '取消',
            onPositiveClick: async () => {
              try {
                const result = await audio.deleteAsset(asset.id, { forceDetach: true });
                const impactText = describeDeleteImpact(result?.impact);
                message.success(impactText ? `素材已删除，${impactText}` : '素材已删除');
              } catch (forceError) {
                console.warn(forceError);
                message.error(extractErrorMessage(forceError, '删除失败，请稍后重试'));
              }
            },
          });
          return;
        }
        console.warn(error);
        message.error(extractErrorMessage(error, '删除失败，请稍后重试'));
      }
    },
  });
}

function copyStream(id: string) {
  const url = audio.buildRawStreamUrl(id);
  void copyTextWithResult(url, {
    onSuccess: () => {
      message.success('播放链接已复制');
    },
    onFailure: () => {
      message.error('复制失败');
    },
  });
}

function toggleFolderPanel() {
  if (isMobileLayout.value) {
    folderDrawerVisible.value = !folderDrawerVisible.value;
    return;
  }
  folderPanelCollapsed.value = !folderPanelCollapsed.value;
}

function toggleDetailPanel() {
  if (isMobileLayout.value) {
    detailDrawerVisible.value = !detailDrawerVisible.value;
    return;
  }
  detailPanelCollapsed.value = !detailPanelCollapsed.value;
}

function openDetailPanel() {
  if (isMobileLayout.value) {
    detailDrawerVisible.value = true;
    return;
  }
  detailPanelCollapsed.value = false;
}

function isAssetReorderDrag(event: DragEvent) {
  return Array.from(event.dataTransfer?.types || []).includes('application/x-audio-asset-id');
}

function handleAssetDragStart(event: DragEvent, row: AudioAsset) {
  if (!canReorderAssets.value || !audio.canEditAsset(row) || !event.dataTransfer) {
    event.preventDefault();
    return;
  }
  draggingAssetId.value = row.id;
  event.dataTransfer.effectAllowed = 'move';
  event.dataTransfer.setData('application/x-audio-asset-id', row.id);
}

function handleAssetDragOver(event: DragEvent, row: AudioAsset) {
  if (!draggingAssetId.value || draggingAssetId.value === row.id || !isAssetReorderDrag(event)) return;
  event.preventDefault();
  event.stopPropagation();
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move';
  }
  dragOverAssetId.value = row.id;
}

function handleAssetDragLeave(row: AudioAsset) {
  if (dragOverAssetId.value === row.id) {
    dragOverAssetId.value = null;
  }
}

async function handleAssetDrop(event: DragEvent, row: AudioAsset) {
  if (!draggingAssetId.value || !isAssetReorderDrag(event)) return;
  event.preventDefault();
  event.stopPropagation();
  const sourceId = event.dataTransfer?.getData('application/x-audio-asset-id') || draggingAssetId.value;
  const targetId = row.id;
  dragOverAssetId.value = null;
  draggingAssetId.value = null;
  if (!sourceId || sourceId === targetId) return;
  const next = [...tableData.value];
  const sourceIndex = next.findIndex((item) => item.id === sourceId);
  const targetIndex = next.findIndex((item) => item.id === targetId);
  if (sourceIndex < 0 || targetIndex < 0) return;
  const [moved] = next.splice(sourceIndex, 1);
  next.splice(targetIndex, 0, moved);
  await saveAssetManualOrder(next.map((item) => item.id), [sourceId]);
}

async function moveAssetByStep(event: MouseEvent, assetId: string, step: -1 | 1) {
  event.stopPropagation();
  if (!canReorderAssets.value) return;
  const next = [...tableData.value];
  const currentIndex = next.findIndex((item) => item.id === assetId);
  if (currentIndex < 0) return;
  const targetIndex = currentIndex + step;
  if (targetIndex < 0 || targetIndex >= next.length) return;
  const [moved] = next.splice(currentIndex, 1);
  next.splice(targetIndex, 0, moved);
  await saveAssetManualOrder(next.map((item) => item.id), [assetId]);
}

async function saveAssetManualOrder(ids: string[], movedIds: string[]) {
  try {
    await audio.reorderAssets(ids, movedIds);
    if (manualSortEnabled.value) {
      message.success('素材手动顺序已更新');
    } else {
      message.success('手动顺序已保存，开启手动排序后生效');
    }
  } catch (err) {
    console.warn(err);
    message.error('素材手动顺序更新失败');
  }
}

function handleAssetDragEnd() {
  draggingAssetId.value = null;
  dragOverAssetId.value = null;
}

function handleListDragEnter(event: DragEvent) {
  if (isAssetReorderDrag(event)) return;
  if (!audio.canManage || !event.dataTransfer?.files?.length) return;
  dragUploadDepth.value += 1;
  dragUploadActive.value = true;
}

function handleListDragOver(event: DragEvent) {
  if (isAssetReorderDrag(event)) return;
  if (!audio.canManage || !event.dataTransfer?.files?.length) return;
  event.dataTransfer.dropEffect = 'copy';
  dragUploadActive.value = true;
}

function handleListDragLeave() {
  if (!audio.canManage) return;
  dragUploadDepth.value = Math.max(0, dragUploadDepth.value - 1);
  if (dragUploadDepth.value === 0) {
    dragUploadActive.value = false;
  }
}

function handleListDrop(event: DragEvent) {
  if (isAssetReorderDrag(event)) return;
  dragUploadDepth.value = 0;
  dragUploadActive.value = false;
  if (!audio.canManage || !event.dataTransfer?.files?.length) return;
  uploadDrawerVisible.value = true;
  const scope = dragUploadScope.value;
  audio.handleUpload(event.dataTransfer.files, {
    scope,
    worldId: scope === 'world' ? audio.currentWorldId ?? undefined : undefined,
    folderId: audio.filters.folderId ?? undefined,
  });
}

function openBatchMoveModal() {
  if (!audio.canManage || !selectionCount.value) return;
  batchMoveTarget.value = currentFolder.value?.id ?? null;
  batchMoveModalVisible.value = true;
}

async function handleBatchMoveSave() {
  if (!audio.canManage) return;
  try {
    const summary = await audio.batchUpdateAssets(checkedRowKeys.value, {
      folderId: batchMoveTarget.value ?? null,
    });
    if (summary.success) {
      message.success(`已移动 ${summary.success} 条素材`);
    }
    if (summary.failed) {
      message.warning(summarizeBatchUpdateFailures(summary.failures, '移动'));
    }
    batchMoveModalVisible.value = false;
    clearSelection();
  } catch (err) {
    console.warn(err);
    message.error('批量移动失败');
  }
}

function openBatchVisibilityModal() {
  if (!audio.canManage || !selectionCount.value) return;
  batchVisibilityValue.value = 'public';
  batchVisibilityModalVisible.value = true;
}

async function handleBatchVisibilitySave() {
  if (!audio.canManage) return;
  try {
    const summary = await audio.batchUpdateAssets(checkedRowKeys.value, {
      visibility: batchVisibilityValue.value,
    });
    if (summary.success) {
      message.success(`已更新 ${summary.success} 条素材的可见性`);
    }
    if (summary.failed) {
      message.warning(`${summary.failed} 条素材更新失败`);
    }
    batchVisibilityModalVisible.value = false;
    clearSelection();
  } catch (err) {
    console.warn(err);
    message.error('批量修改可见性失败');
  }
}

function openBatchScopeModal() {
  if (!audio.isSystemAdmin || !selectionCount.value) return;
  batchScopeValue.value = 'common';
  batchScopeModalVisible.value = true;
}

async function handleBatchScopeSave() {
  if (!audio.isSystemAdmin) return;
  if (batchScopeValue.value === 'world' && !audio.currentWorldId) {
    message.error('当前未选择世界，无法设为世界级');
    return;
  }
  try {
    const summary = await audio.batchUpdateAssets(checkedRowKeys.value, {
      scope: batchScopeValue.value,
      worldId: batchScopeValue.value === 'world' ? audio.currentWorldId : null,
    });
    if (summary.success) {
      message.success(`已更新 ${summary.success} 条素材的级别`);
    }
    if (summary.failed) {
      message.warning(`${summary.failed} 条素材更新失败`);
    }
    batchScopeModalVisible.value = false;
    clearSelection();
  } catch (err) {
    console.warn(err);
    message.error('批量修改级别失败');
  }
}

function confirmBatchDelete() {
  if (!audio.canManage || !selectionCount.value) return;
  dialog.warning({
    title: '批量删除素材',
    content: `确定删除已选的 ${selectionCount.value} 条素材吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const summary = await audio.batchDeleteAssets(checkedRowKeys.value);
        if (summary.success) {
          message.success(`已删除 ${summary.success} 条素材`);
        }
        if (summary.failed) {
          message.warning(summarizeBatchDeleteFailures(summary.failures));
          openBatchDeleteFailureDialog(summary.failures);
        }
        clearSelection();
      } catch (err) {
        console.warn(err);
        message.error('批量删除失败');
      }
    },
  });
}

watch(
  () => audio.filters,
  (filters) => {
    keyword.value = filters.query ?? '';
    selectedTags.value = [...filters.tags];
    selectedCreators.value = [...filters.creatorIds];
    selectedScope.value = filters.scope ?? 'all';
    durationRange.value = filters.durationRange ? [...filters.durationRange] as [number, number] : [0, durationMax];
    folderKeys.value = filters.folderId ? [filters.folderId] : ['all'];
  },
  { deep: true }
);

watch(
  () => audio.filteredAssets,
  (list) => {
    if (!list.length) {
      audio.setSelectedAsset(null);
      return;
    }
    if (detailFocus.value === 'folder') {
      return;
    }
    if (!audio.selectedAssetId || !list.some((item) => item.id === audio.selectedAssetId)) {
      audio.setSelectedAsset(list[0].id);
    }
  },
  { immediate: true }
);

watch(
  isMobileLayout,
  (mobile) => {
    if (mobile) {
      detailPanelCollapsed.value = true;
      folderPanelCollapsed.value = true;
      folderDrawerVisible.value = false;
      return;
    }
    folderEditMode.value = false;
    if (detailPanelCollapsed.value) {
      detailPanelCollapsed.value = false;
    }
  },
  { immediate: true }
);

watch(
  () => folderForm.scope,
  (scope) => {
    if (scope === 'common') {
      folderForm.worldId = null;
      return;
    }
    folderForm.worldId = getPreferredWorldId(folderForm.worldId);
  }
);

watch(
  () => assetForm.scope,
  (scope) => {
    if (scope === 'common') {
      assetForm.worldId = null;
      return;
    }
    assetForm.worldId = getPreferredWorldId(assetForm.worldId);
  }
);

watch(
  () => selectedAsset.value?.createdBy,
  (creatorId) => {
    void ensureUserLabel(creatorId);
  },
  { immediate: true }
);

watch(
  () => currentFolder.value?.createdBy,
  (creatorId) => {
    void ensureUserLabel(creatorId);
  },
  { immediate: true }
);

watch(
  () => tableData.value,
  (list) => {
    const safeList = Array.isArray(list) ? list : [];
    const available = new Set(safeList.map((item) => item.id));
    checkedRowKeys.value = checkedRowKeys.value.filter((key) => available.has(key));
  },
  { deep: true }
);

watch(
  () => audio.canManage,
  (canManage) => {
    if (!canManage) {
      clearSelection();
    }
  }
);

onMounted(() => {
  if (!audio.initialized) {
    audio.ensureInitialized();
  }
  if (!audio.selectedAsset && audio.filteredAssets.length) {
    audio.setSelectedAsset(audio.filteredAssets[0].id);
  }
});
</script>

<style scoped lang="scss">
.audio-library {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.audio-library__toolbar {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.75rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
}

.audio-library__filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 0.5rem;
  align-items: center;
}

.audio-library__filter-item {
  width: 100%;
}

.audio-library__duration {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.audio-library__filter-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.audio-library__toolbar-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  flex-wrap: wrap;
}

.audio-library__alert {
  margin-top: 0.25rem;
}

.audio-library__content {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr) 320px;
  gap: 0.75rem;
  min-height: 420px;
}

.audio-library__content.is-folder-collapsed {
  grid-template-columns: 72px minmax(0, 1fr) 320px;
}

.audio-library__content.is-detail-collapsed {
  grid-template-columns: 240px minmax(0, 1fr);
}

.audio-library__content.is-folder-collapsed.is-detail-collapsed {
  grid-template-columns: 72px minmax(0, 1fr);
}

.audio-library__selection {
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.5rem 0.75rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(99, 179, 237, 0.08);
}

.audio-library__folders,
.audio-library__table,
.audio-library__detail {
  border: 1px solid var(--audio-card-border, var(--sc-border-mute));
  border-radius: 12px;
  padding: 0.75rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
}

.audio-library__folders.is-collapsed {
  padding: 0.5rem;
}

.audio-library__folder-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.audio-library__panel-top {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.audio-library__panel-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.audio-library__folder-actions {
  display: flex;
  gap: 0.25rem;
  flex-wrap: wrap;
}

.audio-library__panel-collapsed {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  min-height: 320px;
}

.audio-library__panel-collapsed-label {
  writing-mode: vertical-rl;
  text-orientation: mixed;
  font-size: 0.78rem;
  color: var(--sc-text-secondary);
  letter-spacing: 0.08em;
}

.audio-library__table {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  position: relative;
  min-width: 0;
}

.audio-library__table-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.audio-library__table-summary {
  display: inline-flex;
  align-items: baseline;
  gap: 0.35rem;
  color: var(--sc-text-secondary);
}

.audio-library__table-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.audio-library__manual-sort-toggle {
  --n-box-shadow: none;
  --n-box-shadow-hover: none;
  --n-box-shadow-pressed: none;
  box-shadow: none !important;
}

.audio-library__manual-sort-toggle.is-active {
  --n-box-shadow: 0 4px 12px rgba(37, 99, 235, 0.18);
  --n-box-shadow-hover: 0 5px 14px rgba(37, 99, 235, 0.22);
  --n-box-shadow-pressed: 0 2px 8px rgba(37, 99, 235, 0.16);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.18);
}

.audio-library__drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 3;
  background: rgba(15, 23, 42, 0.45);
  border: 1px dashed rgba(99, 179, 237, 0.9);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  pointer-events: none;
}

.audio-library__drop-overlay-card {
  min-width: min(100%, 340px);
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 1rem 1.25rem;
  border-radius: 14px;
  background: rgba(17, 24, 39, 0.88);
  color: #fff;
}

.audio-library__table.is-drag-over {
  border-color: rgba(99, 179, 237, 0.9);
}

.audio-library__pagination {
  display: flex;
  justify-content: flex-end;
}

.audio-library__detail {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.audio-library__detail-body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.audio-library__detail-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 0.75rem;
}

.audio-library__detail-tags {
  display: flex;
  gap: 0.25rem;
}

.audio-library__detail-subtitle {
  margin: 0;
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.audio-library__detail-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.85rem;
}

.audio-library__tags,
.audio-library__description {
  font-size: 0.85rem;
}

.audio-library__tag {
  margin-right: 0.25rem;
  margin-bottom: 0.25rem;
}

.audio-library__detail-actions {
  display: flex;
  gap: 0.5rem;
}

.audio-library__detail-actions--stacked {
  flex-wrap: wrap;
}

.audio-library__drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.audio-library__drawer-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.audio-library__mobile-tip {
  margin: 0;
  font-size: 0.78rem;
  color: var(--sc-text-secondary);
}

.audio-library__detail-empty {
  min-height: 240px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--sc-text-secondary);
}

.audio-library__modal-tip {
  margin: 0 0 0.5rem;
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
}

.audio-asset-drawer__header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  justify-content: space-between;
}

.audio-table__name {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.audio-table__name-wrap {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem;
}

.audio-table__title {
  font-weight: 600;
  line-height: 1.35;
}

.audio-table__meta {
  margin: 0;
  font-size: 0.72rem;
  color: var(--sc-text-secondary);
}

.audio-table__desc {
  margin: 0;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audio-table__mobile-reorder {
  display: none;
  gap: 0.25rem;
  margin-top: 0.25rem;
}

.audio-table__inline-actions {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  opacity: 0;
  transition: opacity 0.18s ease;
}

.audio-table__header-sort {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  border: 0;
  padding: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}

.audio-table__header-sort--active {
  color: var(--sc-primary, #2563eb);
}

.audio-table__header-sort-glyph {
  font-size: 0.7rem;
  opacity: 0.75;
}

:deep(.n-data-table-tr:hover .audio-table__inline-actions),
:deep(.is-selected-row .audio-table__inline-actions) {
  opacity: 1;
}

.audio-library__tag-list {
  display: flex;
  flex-wrap: wrap;
}

:deep(.is-selected-row td) {
  background-color: rgba(99, 179, 237, 0.08);
}

:deep(.is-dragging-row td) {
  opacity: 0.55;
}

:deep(.is-drag-over-row td) {
  background-color: rgba(37, 99, 235, 0.12);
  box-shadow: inset 0 2px 0 rgba(37, 99, 235, 0.75);
}

@media (max-width: 960px) {
  .audio-library__content {
    grid-template-columns: 1fr;
  }

  .audio-library__content.is-folder-collapsed,
  .audio-library__content.is-detail-collapsed,
  .audio-library__content.is-folder-collapsed.is-detail-collapsed {
    grid-template-columns: 1fr;
  }

  .audio-library__detail {
    display: none;
  }
}

@media (max-width: 640px) {
  .audio-table__mobile-reorder {
    display: inline-flex;
  }
}

@media (max-width: 720px) {
  .audio-library__filters {
    grid-template-columns: 1fr;
  }

  .audio-library__selection {
    flex-direction: column;
    align-items: stretch;
    gap: 0.75rem;
  }

  .audio-library__table-top {
    align-items: stretch;
  }

  .audio-asset-drawer__header {
    gap: 0.35rem;
  }

  .audio-asset-drawer__header > span {
    flex: 1 1 auto;
    text-align: center;
  }
}
</style>
