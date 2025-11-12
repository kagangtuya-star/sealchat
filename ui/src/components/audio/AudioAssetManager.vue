<template>
  <div class="audio-library">
    <div class="audio-library__toolbar">
      <AudioSearchBar v-model="keyword" placeholder="搜索素材" @search="handleSearch">
        <n-button size="small" @click="handleRefresh" :loading="audio.assetsLoading">刷新</n-button>
      </AudioSearchBar>
    </div>

    <div class="audio-library__content">
      <aside class="audio-library__folders">
        <n-tree
          block-line
          :data="folderOptions"
          selectable
          :selected-keys="folderSelected"
          @update:selected-keys="handleFolderSelect"
        />
      </aside>

      <section class="audio-library__table">
        <n-data-table
          size="small"
          :loading="audio.assetsLoading"
          :columns="columns"
          :data="tableData"
          :row-key="rowKey"
          @row-click="handleRowClick"
        />
      </section>

      <section class="audio-library__detail" v-if="selectedAsset">
        <h4>{{ selectedAsset.name }}</h4>
        <p>时长：{{ formatDuration(selectedAsset.duration) }}</p>
        <p>
          标签：
          <n-tag v-for="tag in selectedAsset.tags" :key="tag" size="small">{{ tag }}</n-tag>
        </p>
        <p>存储：{{ selectedAsset.storageType }}</p>
        <p>更新：{{ formatDate(selectedAsset.updatedAt) }}</p>
        <n-button quaternary size="small" @click="copyStream(selectedAsset.id)">复制播放链接</n-button>
      </section>

      <section class="audio-library__detail" v-else>
        <n-empty description="选择一条素材以查看详情" />
      </section>
    </div>

    <UploadPanel class="audio-library__upload" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import type { DataTableColumns, TreeOption } from 'naive-ui';
import { useMessage } from 'naive-ui';
import { useAudioStudioStore } from '@/stores/audioStudio';
import AudioSearchBar from './AudioSearchBar.vue';
import UploadPanel from './UploadPanel.vue';
import type { AudioAsset, AudioFolder } from '@/types/audio';

const audio = useAudioStudioStore();
const keyword = ref('');
const folderSelected = ref<string[]>(['all']);
const selectedAssetId = ref<string | null>(null);
const message = useMessage();

const columns: DataTableColumns<AudioAsset> = [
  {
    title: '名称',
    key: 'name',
  },
  {
    title: '时长',
    key: 'duration',
    render: (row) => formatDuration(row.duration),
  },
  {
    title: '标签',
    key: 'tags',
    render: (row) => row.tags.join(', '),
  },
  {
    title: '上传者',
    key: 'createdBy',
  },
];

const tableData = computed(() => audio.filteredAssets);
const selectedAsset = computed(() => tableData.value.find((asset) => asset.id === selectedAssetId.value) || null);

const folderOptions = computed<TreeOption[]>(() => {
  const build = (folders: AudioFolder[]): TreeOption[] =>
    folders.map((folder) => ({
      key: folder.id,
      label: folder.name,
      children: folder.children ? build(folder.children) : undefined,
    }));
  return [
    { key: 'all', label: '全部素材' },
    ...build(audio.folders),
  ];
});

function handleSearch(value: string) {
  keyword.value = value;
  audio.searchAssetsLocally(value);
}

function rowKey(row: AudioAsset) {
  return row.id;
}

function handleRowClick(row: AudioAsset) {
  selectedAssetId.value = row.id;
}

function handleFolderSelect(keys: Array<string | number>) {
  const raw = keys.length ? keys[0] : undefined;
  const normalized = raw === undefined || raw === null ? '' : String(raw);
  if (!normalized || normalized === 'all') {
    folderSelected.value = normalized === 'all' ? ['all'] : [];
    audio.applyFilters({ folderId: null });
    return;
  }
  folderSelected.value = [normalized];
  audio.applyFilters({ folderId: normalized });
}

function handleRefresh() {
  const query = keyword.value?.trim();
  if (query) {
    audio.fetchAssets({ query });
  } else {
    audio.fetchAssets();
  }
}

function formatDuration(value: number) {
  if (!value) return '00:00';
  const minutes = Math.floor(value / 60);
  const seconds = Math.floor(value % 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function formatDate(value?: string) {
  if (!value) return '未知';
  return new Date(value).toLocaleString();
}

function copyStream(assetId: string) {
  const url = audio.buildStreamUrl(assetId);
  navigator.clipboard.writeText(url);
  message.success('播放链接已复制');
}

async function initializeLibrary() {
  try {
    if (!audio.initialized) {
      await audio.ensureInitialized();
    } else if (!audio.assets.length) {
      await audio.fetchAssets();
    } else if (!audio.filteredAssets.length) {
      audio.filteredAssets = audio.assets;
    }
    if (!folderSelected.value.length) {
      folderSelected.value = ['all'];
    }
  } catch (err) {
    console.warn('初始化素材库失败', err);
  }
}

onMounted(() => {
  initializeLibrary();
});
</script>

<style scoped lang="scss">
.audio-library {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.audio-library__content {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr) 220px;
  gap: 0.75rem;
  min-height: 360px;
}

.audio-library__folders,
.audio-library__table,
.audio-library__detail {
  border: 1px solid var(--audio-card-border, var(--sc-border-mute));
  border-radius: 12px;
  padding: 0.5rem;
  background: var(--audio-card-surface, var(--sc-bg-elevated));
  box-shadow: var(--audio-panel-shadow, 0 20px 40px rgba(15, 23, 42, 0.08));
  backdrop-filter: blur(10px);
  transition: background 0.2s ease, border-color 0.2s ease;
}

.audio-library__upload {
  margin-top: 0.5rem;
}
</style>
