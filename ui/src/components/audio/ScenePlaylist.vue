<template>
  <div class="scene-board">
    <section class="scene-board__toolbar">
      <n-input
        v-model:value="keyword"
        size="small"
        clearable
        placeholder="搜索播放列表"
        @keyup.enter="handleSearch"
      >
        <template #prefix>
          <n-icon size="16">
            <SearchOutline />
          </n-icon>
        </template>
      </n-input>
      <n-space size="small">
        <n-button size="small" quaternary @click="handleRefresh" :loading="audio.scenesLoading">
          <template #icon>
            <n-icon size="16">
              <ReloadOutline />
            </n-icon>
          </template>
          刷新
        </n-button>
        <n-button v-if="audio.canManage" size="small" secondary @click="openCreateDrawer">
          <template #icon>
            <n-icon size="16">
              <AddOutline />
            </n-icon>
          </template>
          新建播放列表
        </n-button>
        <n-button v-if="audio.canManage" size="small" type="primary" @click="openSaveCurrentDrawer">
          保存当前音轨
        </n-button>
      </n-space>
    </section>

    <section v-if="hasSelection" class="scene-board__selection">
      <div>已选 {{ checkedSceneIds.length }} 项</div>
      <n-space size="small">
        <n-button size="tiny" @click="clearSelection">清空</n-button>
        <n-button size="tiny" type="error" :loading="audio.scenesLoading" @click="confirmBatchDelete">
          批量删除
        </n-button>
      </n-space>
    </section>

    <section class="scene-board__content">
      <div class="scene-board__list">
        <n-data-table
          size="small"
          :columns="columns"
          :data="sceneData"
          :loading="audio.scenesLoading"
          :row-key="rowKey"
          :checked-row-keys="checkedSceneIds"
          @update:checked-row-keys="handleCheckChange"
          :row-class-name="rowClassName"
          :row-props="rowProps"
          virtual-scroll
          :max-height="360"
        />
        <div class="scene-board__pagination">
          <n-pagination
            size="small"
            :page="audio.scenePagination.page"
            :page-size="audio.scenePagination.pageSize"
            :item-count="audio.scenePagination.total"
            :page-sizes="[5, 10, 20]"
            show-size-picker
            @update:page="audio.setScenePage"
            @update:page-size="audio.setScenePageSize"
          />
        </div>
      </div>

      <aside class="scene-board__detail">
        <template v-if="selectedScene">
          <header>
            <div>
              <h3>{{ selectedScene.name }}</h3>
              <p>{{ selectedScene.description || '暂无描述' }}</p>
            </div>
            <n-tag size="small" v-if="selectedScene.channelScope">频道限定</n-tag>
          </header>
          <section class="scene-board__detail-section">
            <strong>标签</strong>
            <div class="scene-board__tags">
              <n-tag v-for="tag in selectedScene.tags" :key="tag" size="small" bordered>{{ tag }}</n-tag>
              <span v-if="!selectedScene.tags.length">未设置</span>
            </div>
          </section>
          <section class="scene-board__detail-section">
            <strong>音轨</strong>
            <ul>
              <li v-for="track in selectedScene.tracks" :key="track.type">
                {{ trackLabel(track.type) }} · {{ findAssetName(track.assetId) }}（音量 {{ Math.round(track.volume * 100) }}%）
              </li>
            </ul>
          </section>
          <section class="scene-board__detail-actions">
            <n-button size="small" type="primary" @click="applyScene(selectedScene, true)">加载并播放</n-button>
            <n-button size="small" tertiary @click="applyScene(selectedScene, false)">仅加载</n-button>
            <n-button v-if="audio.canManage" size="small" @click="openEditDrawer(selectedScene)">编辑</n-button>
          </section>
        </template>
        <n-empty v-else description="请选择一个播放列表" />
      </aside>
    </section>

    <n-drawer :show="formDrawerVisible" width="420" @update:show="formDrawerVisible = $event">
      <n-drawer-content :title="drawerTitle">
        <n-form ref="sceneFormRef" :model="sceneForm" :rules="sceneFormRules" label-placement="top">
          <n-form-item label="名称" path="name">
            <n-input v-model:value="sceneForm.name" maxlength="60" show-count />
          </n-form-item>
          <n-form-item label="描述" path="description">
            <n-input v-model:value="sceneForm.description" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" />
          </n-form-item>
          <n-form-item label="标签">
            <n-select v-model:value="sceneForm.tags" multiple filterable tag placeholder="输入或选择标签" />
          </n-form-item>
          <n-form-item v-if="sceneForm.mode === 'save'" label="保存后立即播放">
            <n-switch v-model:value="sceneForm.autoPlayAfterSave" />
          </n-form-item>
        </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="formDrawerVisible = false">取消</n-button>
            <n-button type="primary" :loading="audio.scenesLoading" @click="handleSubmit">
              保存
            </n-button>
          </n-space>
        </template>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<script setup lang="ts">
import { AddOutline, ReloadOutline, SearchOutline } from '@vicons/ionicons5';
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import {
  NButton,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormRules,
} from 'naive-ui';
import type { AudioScene } from '@/types/audio';
import { useAudioStudioStore } from '@/stores/audioStudio';

const audio = useAudioStudioStore();
const message = useMessage();
const dialog = useDialog();

const keyword = ref(audio.sceneFilters.query ?? '');
const checkedSceneIds = ref<string[]>([]);
const formDrawerVisible = ref(false);
const sceneFormRef = ref<FormInst | null>(null);
const sceneForm = reactive({
  id: '',
  name: '',
  description: '',
  tags: [] as string[],
  mode: 'save' as 'save' | 'create' | 'edit',
  autoPlayAfterSave: true,
});

const sceneFormRules: FormRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
};

const sceneData = computed(() => audio.scenes);
const selectedScene = computed(() => audio.selectedScene);
const hasSelection = computed(() => checkedSceneIds.value.length > 0);

const columns = computed<DataTableColumns<AudioScene>>(() => [
  {
    type: 'selection',
    disabled: () => !audio.canManage,
  },
  {
    title: '名称',
    key: 'name',
    minWidth: 160,
    render: (row) =>
      h('div', { class: 'scene-board__row-name' }, [
        h('strong', row.name),
        row.description ? h('p', row.description) : null,
      ]),
  },
  {
    title: '标签',
    key: 'tags',
    minWidth: 120,
    render: (row) =>
      row.tags.length
        ? row.tags.map((tag) => h(NTag, { size: 'tiny', bordered: false, key: tag }, { default: () => tag }))
        : '-',
  },
  {
    title: '音轨',
    key: 'tracks',
    minWidth: 140,
    render: (row) => row.tracks.map((t) => trackLabel(t.type)).join(' / '),
  },
  {
    title: '更新时间',
    key: 'updatedAt',
    width: 140,
    render: (row) => new Date(row.updatedAt).toLocaleString(),
  },
  {
    title: '操作',
    key: 'actions',
    width: 160,
    render: (row) =>
      h(
        'div',
        { class: 'scene-board__row-actions' },
        [
          h(
            NButton,
            { size: 'tiny', type: 'primary', ghost: true, onClick: () => applyScene(row, true) },
            { default: () => '播放' }
          ),
          h(
            NButton,
            { size: 'tiny', quaternary: true, onClick: () => applyScene(row, false) },
            { default: () => '加载' }
          ),
          audio.canManage
            ? h(
                NButton,
                { size: 'tiny', tertiary: true, onClick: () => openEditDrawer(row) },
                { default: () => '编辑' }
              )
            : null,
        ].filter(Boolean)
      ),
  },
]);

const rowKey = (row: AudioScene) => row.id;
const rowClassName = (row: AudioScene) => (row.id === audio.selectedSceneId ? 'is-selected-row' : '');
const rowProps = (row: AudioScene) => ({
  onClick: () => audio.setSelectedScene(row.id),
});

function handleCheckChange(keys: Array<string | number>) {
  checkedSceneIds.value = keys.map((key) => String(key));
}

function clearSelection() {
  checkedSceneIds.value = [];
}

async function handleSearch() {
  await audio.fetchScenes({ query: keyword.value });
}

async function handleRefresh() {
  await audio.fetchScenes();
  message.success('播放列表已刷新');
}

function openCreateDrawer() {
  if (!audio.canManage) return;
  sceneForm.mode = 'create';
  sceneForm.id = '';
  sceneForm.name = '';
  sceneForm.description = '';
  sceneForm.tags = [];
  sceneForm.autoPlayAfterSave = false;
  formDrawerVisible.value = true;
}

function openSaveCurrentDrawer() {
  if (!audio.canManage) return;
  sceneForm.mode = 'save';
  sceneForm.id = '';
  sceneForm.name = '';
  sceneForm.description = '';
  sceneForm.tags = [];
  sceneForm.autoPlayAfterSave = true;
  formDrawerVisible.value = true;
}

function openEditDrawer(scene: AudioScene) {
  if (!audio.canManage) return;
  sceneForm.mode = 'edit';
  sceneForm.id = scene.id;
  sceneForm.name = scene.name;
  sceneForm.description = scene.description || '';
  sceneForm.tags = [...scene.tags];
  sceneForm.autoPlayAfterSave = false;
  formDrawerVisible.value = true;
}

async function handleSubmit() {
  await sceneFormRef.value?.validate();
  try {
    if (sceneForm.mode === 'save') {
      await audio.createSceneFromCurrentTracks({
        name: sceneForm.name.trim(),
        description: sceneForm.description,
        tags: [...sceneForm.tags],
        autoPlayAfterSave: sceneForm.autoPlayAfterSave,
      });
      message.success('已保存当前音轨');
    } else if (sceneForm.mode === 'create') {
      await audio.createSceneFromCurrentTracks({
        name: sceneForm.name.trim(),
        description: sceneForm.description,
        tags: [...sceneForm.tags],
        autoPlayAfterSave: false,
      });
      message.success('播放列表已创建');
    } else {
      await audio.updateScene(sceneForm.id, {
        name: sceneForm.name.trim(),
        description: sceneForm.description,
        tags: [...sceneForm.tags],
      });
      message.success('播放列表已更新');
    }
    formDrawerVisible.value = false;
  } catch (err) {
    console.warn(err);
    message.error('保存失败，请稍后重试');
  }
}

function confirmBatchDelete() {
  if (!audio.canManage || !checkedSceneIds.value.length) return;
  dialog.warning({
    title: '批量删除播放列表',
    content: `确定删除已选的 ${checkedSceneIds.value.length} 个播放列表吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      const summary = await audio.deleteScenes(checkedSceneIds.value);
      if (summary.success) {
        message.success(`已删除 ${summary.success} 个播放列表`);
      }
      if (summary.failed) {
        message.warning(`${summary.failed} 个播放列表删除失败`);
      }
      clearSelection();
    },
  });
}

async function applyScene(scene: AudioScene, autoPlay: boolean) {
  await audio.applyScene(scene.id, { autoPlay });
  if (autoPlay) {
    message.success(`已套用并播放「${scene.name}」`);
  } else {
    message.success(`已加载「${scene.name}」，等待播放`);
  }
}

function trackLabel(type: string) {
  return { music: '音乐', ambience: '环境', sfx: '音效' }[type] || type;
}

function findAssetName(assetId: string | null) {
  if (!assetId) return '未绑定';
  return audio.assets.find((asset) => asset.id === assetId)?.name || '未知素材';
}

const drawerTitle = computed(() => {
  if (sceneForm.mode === 'save') return '保存当前音轨为播放列表';
  if (sceneForm.mode === 'edit') return '编辑播放列表';
  return '新建播放列表';
});

watch(
  () => audio.sceneFilters.query,
  (val) => {
    keyword.value = val ?? '';
  }
);

watch(
  () => audio.scenes,
  (list) => {
    const available = new Set(list.map((item) => item.id));
    checkedSceneIds.value = checkedSceneIds.value.filter((id) => available.has(id));
  }
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
  if (!audio.scenes.length) {
    audio.fetchScenes();
  }
});
</script>

<style scoped lang="scss">
.scene-board {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.scene-board__toolbar {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  align-items: center;
}

.scene-board__selection {
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  padding: 0.35rem 0.6rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(99, 179, 237, 0.08);
}

.scene-board__content {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 280px;
  gap: 0.75rem;
}

.scene-board__list {
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.5rem;
  background: var(--sc-bg-elevated);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.scene-board__pagination {
  display: flex;
  justify-content: flex-end;
}

.scene-board__detail {
  border: 1px solid var(--sc-border-mute);
  border-radius: 12px;
  padding: 0.75rem;
  background: var(--sc-bg-elevated);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.scene-board__detail header h3 {
  margin: 0;
}

.scene-board__detail header p {
  margin: 0.2rem 0 0;
  color: var(--sc-text-secondary);
}

.scene-board__detail-section {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.scene-board__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
}

.scene-board__detail-actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.scene-board__row-name {
  display: flex;
  flex-direction: column;
}

.scene-board__row-name p {
  margin: 0;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.scene-board__row-actions {
  display: flex;
  gap: 0.25rem;
}

:deep(.is-selected-row td) {
  background-color: rgba(99, 179, 237, 0.08);
}
</style>
