<template>
  <n-modal :show="show" preset="dialog" title="音频素材库 S3 模式" :mask-closable="false" @update:show="emit('update:show', $event)">
    <n-space vertical size="medium">
      <n-alert v-if="!settings.s3Available" type="warning">S3 不可用。请先配置服务端 S3。</n-alert>
      <n-space justify="space-between" align="center">
        <span>S3 模式开关</span>
        <n-switch v-model:value="enabled" :disabled="!settings.s3Available && !enabled" />
      </n-space>
      <n-descriptions bordered size="small" :column="1">
        <n-descriptions-item label="当前 bucket">{{ settings.bucketLabel || '未配置' }}</n-descriptions-item>
      </n-descriptions>
      <n-input v-model:value="selectedPrefix" placeholder="空 prefix 表示 bucket 根目录" readonly />
      <n-space align="center" :wrap="false">
        <span>控制台目录遍历深度</span>
        <n-input-number v-model:value="selectorDepth" :min="0" :max="5" :step="1" style="width: 120px" />
      </n-space>
      <n-text depth="3">0 表示只读取当前目录；2 表示自动包含两级子目录。</n-text>
      <n-space align="center" :wrap="false">
        <n-button v-if="currentPrefix" size="tiny" secondary @click="goParent">上一级</n-button>
        <n-breadcrumb>
          <n-breadcrumb-item @click="selectPrefix('')">根目录</n-breadcrumb-item>
          <n-breadcrumb-item
            v-for="item in breadcrumbs"
            :key="item.prefix"
            @click="selectPrefix(item.prefix)"
          >
            {{ item.name }}
          </n-breadcrumb-item>
        </n-breadcrumb>
      </n-space>
      <n-spin :show="loading">
        <n-list hoverable clickable>
          <n-list-item v-for="item in prefixes" :key="item.prefix" @click="openPrefix(item)">
            <n-thing title="📁" :description="item.name" />
          </n-list-item>
          <n-list-item v-if="!prefixes.length"><n-empty description="当前目录没有子目录" /></n-list-item>
        </n-list>
        <n-button v-if="nextCursor" text type="primary" @click="loadChildren(currentPrefix, nextCursor)">加载更多目录</n-button>
      </n-spin>
      <n-text depth="3">当前选择：{{ selectedPrefix || '/' }}</n-text>
    </n-space>
    <template #action>
      <n-space justify="end">
        <n-button @click="emit('update:show', false)">取消</n-button>
        <n-button type="primary" :loading="saving" :disabled="!settings.canConfigure" @click="save">保存</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { NAlert, NBreadcrumb, NBreadcrumbItem, NButton, NDescriptions, NDescriptionsItem, NEmpty, NInput, NInputNumber, NList, NListItem, NSpace, NSpin, NSwitch, NText, NThing, useMessage } from 'naive-ui';
import { api } from '@/stores/_config';
import type { AudioLibraryPrefix, AudioLibrarySettings } from '@/types/audio-library';

const props = defineProps<{ show: boolean; settings: AudioLibrarySettings; worldId?: string | null }>();
const emit = defineEmits<{ (e: 'update:show', value: boolean): void; (e: 'saved', value: AudioLibrarySettings): void }>();
const message = useMessage();
const enabled = ref(false);
const selectedPrefix = ref('');
const selectorDepth = ref(2);
const currentPrefix = ref('');
const prefixes = ref<AudioLibraryPrefix[]>([]);
const breadcrumbs = ref<AudioLibraryPrefix[]>([]);
const loading = ref(false);
const nextCursor = ref('');
const saving = ref(false);
let loadRequestId = 0;
const settings = computed(() => props.settings);

watch(() => props.show, (visible) => {
  if (!visible) {
    loadRequestId += 1;
    return;
  }
  enabled.value = props.settings.mode === 's3';
  selectedPrefix.value = props.settings.prefix || '';
  selectorDepth.value = normalizeSelectorDepth(props.settings.selectorDepth);
  currentPrefix.value = selectedPrefix.value;
  breadcrumbs.value = buildBreadcrumbs(currentPrefix.value);
  void loadChildren(currentPrefix.value);
});

function normalizeSelectorDepth(value: unknown): number {
  const depth = Number(value);
  return Number.isInteger(depth) && depth >= 0 && depth <= 5 ? depth : 2;
}

function buildBreadcrumbs(prefix: string): AudioLibraryPrefix[] {
  const normalized = String(prefix || '').replace(/^\/+|\/+$/g, '');
  if (!normalized) return [];
  const parts = normalized.split('/').filter(Boolean);
  let path = '';
  return parts.map((name) => {
    path += `${name}/`;
    return { ref: '', name, prefix: path };
  });
}

async function loadChildren(prefix: string, cursor = '') {
  const requestId = ++loadRequestId;
  loading.value = true;
  try {
    const resp = await api.get('/api/v1/audio/library/s3/prefixes', { params: { worldId: props.worldId || undefined, prefix, cursor, limit: 100, _refresh: Date.now() } });
    if (requestId !== loadRequestId) return;
    const page = (resp.data?.prefixes || []) as AudioLibraryPrefix[];
    prefixes.value = cursor ? [...prefixes.value, ...page] : page;
    nextCursor.value = String(resp.data?.nextCursor || '');
    currentPrefix.value = prefix;
    if (!cursor) breadcrumbs.value = buildBreadcrumbs(prefix);
  } catch (error: any) {
    if (requestId !== loadRequestId) return;
    message.error(error?.response?.data?.message || error?.message || '读取 S3 目录失败');
  } finally {
    if (requestId === loadRequestId) loading.value = false;
  }
}

function openPrefix(item: AudioLibraryPrefix) {
  selectedPrefix.value = item.prefix;
  void loadChildren(item.prefix);
}

function selectPrefix(prefix: string) {
  selectedPrefix.value = prefix;
  void loadChildren(prefix);
}

function goParent() {
  const current = currentPrefix.value.replace(/^\/+|\/+$/g, '');
  const separator = current.lastIndexOf('/');
  const parent = separator >= 0 ? `${current.slice(0, separator + 1)}` : '';
  selectPrefix(parent);
}

async function save() {
  saving.value = true;
  try {
    const resp = await api.put('/api/v1/audio/library/settings', { worldId: props.worldId || undefined, mode: enabled.value ? 's3' : 'database', prefix: selectedPrefix.value, selectorDepth: normalizeSelectorDepth(selectorDepth.value) });
    emit('saved', resp.data as AudioLibrarySettings);
    emit('update:show', false);
    message.success('音频素材库设置已保存');
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存失败');
  } finally {
    saving.value = false;
  }
}
</script>
