<template>
  <n-modal
    :show="show"
    preset="card"
    title="S3模式"
    :style="{ width: 'min(720px, 94vw)' }"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <div class="audio-s3-mode">
      <n-alert v-if="!store.settings.available" type="warning">
        当前服务端未启用可用的 S3 兼容存储。请先完成 storage.s3 配置。
      </n-alert>

      <div class="audio-s3-mode__switch-row">
        <div>
          <strong>使用 S3 目录作为音频素材库</strong>
          <p>启用后，音频对象直接从所选前缀读取，不创建本地素材或文件夹数据库记录。</p>
        </div>
        <n-switch
          v-model:value="form.enabled"
          :disabled="!store.settings.available || !store.settings.canConfigure"
        />
      </div>

      <n-descriptions :column="1" size="small" bordered>
        <n-descriptions-item label="Bucket">
          {{ store.settings.bucket || '未配置' }}
        </n-descriptions-item>
        <n-descriptions-item label="已选路径">
          /{{ form.prefix || '' }}
        </n-descriptions-item>
      </n-descriptions>

      <section class="audio-s3-mode__browser" :class="{ 'is-disabled': !form.enabled }">
        <header class="audio-s3-mode__browser-header">
          <div class="audio-s3-mode__breadcrumbs">
            <n-button text size="small" @click="browse('')">根目录</n-button>
            <template v-for="crumb in breadcrumbs" :key="crumb.prefix">
              <span>/</span>
              <n-button text size="small" @click="browse(crumb.prefix)">{{ crumb.name }}</n-button>
            </template>
          </div>
          <n-button
            size="small"
            type="primary"
            secondary
            :disabled="!form.enabled"
            @click="selectCurrent"
          >
            选择当前路径
          </n-button>
        </header>

        <n-spin :show="browseLoading">
          <div class="audio-s3-mode__folder-list">
            <button
              v-if="browseResult.parent !== browseResult.current"
              type="button"
              class="audio-s3-mode__folder-row"
              @click="browse(browseResult.parent)"
            >
              <n-icon size="18"><ArrowUpOutline /></n-icon>
              <span>返回上级</span>
            </button>
            <button
              v-for="item in browseResult.prefixes"
              :key="item.prefix"
              type="button"
              class="audio-s3-mode__folder-row"
              @click="browse(item.prefix)"
            >
              <n-icon size="18"><FolderOpenOutline /></n-icon>
              <span>{{ item.name }}</span>
            </button>
            <n-empty
              v-if="!browseLoading && !browseResult.prefixes.length"
              description="当前路径下没有子目录"
              size="small"
            />
          </div>
        </n-spin>
      </section>

      <n-alert type="info" :bordered="false">
        路径选择器从 Bucket 根路径开始按层读取。现有对象不会导入本地数据库；对象名称、大小、ETag 与更新时间直接来自 S3。
      </n-alert>
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button @click="emit('update:show', false)">取消</n-button>
        <n-button
          type="primary"
          :loading="store.settingsLoading || saving"
          :disabled="!store.settings.canConfigure"
          @click="save"
        >
          保存
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ArrowUpOutline, FolderOpenOutline } from '@vicons/ionicons5';
import { computed, reactive, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import {
  useAudioS3LibraryStore,
  type AudioS3BrowseResult,
} from '@/stores/audioS3Library';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
  saved: [];
}>();

const store = useAudioS3LibraryStore();
const message = useMessage();
const saving = ref(false);
const browseLoading = ref(false);
const form = reactive({
  enabled: false,
  prefix: '',
});
const browseResult = reactive<AudioS3BrowseResult>({
  current: '',
  parent: '',
  prefixes: [],
});

const breadcrumbs = computed(() => {
  const segments = browseResult.current.split('/').filter(Boolean);
  let prefix = '';
  return segments.map((name) => {
    prefix += `${name}/`;
    return { name, prefix };
  });
});

watch(
  () => props.show,
  async (show) => {
    if (!show) return;
    try {
      await store.ensureSettings(true);
      form.enabled = store.settings.enabled;
      form.prefix = store.settings.prefix || '';
      if (store.settings.canConfigure && store.settings.available) {
        await browse('');
      } else {
        browseResult.current = form.prefix;
        browseResult.parent = '';
        browseResult.prefixes = [];
      }
    } catch (error: any) {
      message.error(error?.response?.data?.message || error?.message || '读取 S3 模式配置失败');
    }
  },
);

async function browse(prefix: string) {
  if (!store.settings.available) return;
  browseLoading.value = true;
  try {
    const result = await store.browse(prefix);
    browseResult.current = result.current || '';
    browseResult.parent = result.parent || '';
    browseResult.prefixes = result.prefixes || [];
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '读取 S3 目录失败');
  } finally {
    browseLoading.value = false;
  }
}

function selectCurrent() {
  form.prefix = browseResult.current || '';
  message.success(`已选择 /${form.prefix}`);
}

async function save() {
  saving.value = true;
  try {
    await store.saveSettings({
      enabled: form.enabled,
      prefix: form.prefix,
    });
    message.success(form.enabled ? 'S3 模式已启用' : 'S3 模式已关闭');
    emit('saved');
    emit('update:show', false);
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存 S3 模式失败');
  } finally {
    saving.value = false;
  }
}
</script>

<style scoped lang="scss">
.audio-s3-mode {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.audio-s3-mode__switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.audio-s3-mode__switch-row p {
  margin: 0.25rem 0 0;
  color: var(--sc-text-secondary);
  font-size: 0.82rem;
}

.audio-s3-mode__browser {
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  overflow: hidden;
}

.audio-s3-mode__browser.is-disabled {
  opacity: 0.6;
}

.audio-s3-mode__browser-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.65rem 0.75rem;
  border-bottom: 1px solid var(--sc-border-mute);
}

.audio-s3-mode__breadcrumbs {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.2rem;
  min-width: 0;
}

.audio-s3-mode__folder-list {
  min-height: 220px;
  max-height: 360px;
  overflow-y: auto;
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.audio-s3-mode__folder-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  border: 0;
  border-radius: 8px;
  padding: 0.55rem 0.65rem;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.audio-s3-mode__folder-row:hover {
  background: rgba(99, 179, 237, 0.1);
}
</style>
