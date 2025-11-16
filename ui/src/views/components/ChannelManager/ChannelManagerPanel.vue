<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useChannelFolderStore } from '@/stores/channelFolders'
import { useChatStore } from '@/stores/chat'
import type { SChannel, ChannelConfigSyncResult } from '@/types'
import { useDialog, useMessage } from 'naive-ui'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ (e: 'update:show', value: boolean): void }>()

const folderStore = useChannelFolderStore()
const chat = useChatStore()
const message = useMessage()
const dialog = useDialog()

const visible = computed({
  get: () => props.show,
  set: (value: boolean) => emit('update:show', value),
})

const newFolderName = ref('')
const newFolderDesc = ref('')
const activeFolderId = ref('')
const selectedFolderKeys = ref<string[]>([])
const renameName = ref('')
const renameDesc = ref('')
const channelSearch = ref('')
const selectedChannelIds = ref<string[]>([])
const includeChildren = ref(true)

const syncSource = ref('')
const syncTargets = ref<string[]>([])
const syncScopes = ref<string[]>(['roles'])
const syncLoading = ref(false)
const syncResult = ref<ChannelConfigSyncResult | null>(null)

const folderTree = computed(() => folderStore.folderTree)
const folderOptions = computed(() => folderStore.folders.map((folder) => ({ label: folder.name, value: folder.id })))
const favoriteFolderSet = computed(() => folderStore.favoriteFolderSet)
const channelFolderMap = computed(() => folderStore.channelFolderMap)

const convertTree = (nodes = folderTree.value) =>
  nodes.map((node) => ({
    key: node.id,
    label: node.name,
    children: node.children ? convertTree(node.children as any) : undefined,
  }))

const treeData = computed(() => convertTree())

const flattenChannels = (channels?: SChannel[]): SChannel[] => {
  if (!channels || !channels.length) return []
  const result: SChannel[] = []
  const stack = [...channels]
  while (stack.length) {
    const current = stack.shift()
    if (!current) continue
    result.push(current)
    if (current.children && current.children.length) {
      stack.unshift(...(current.children as SChannel[]))
    }
  }
  return result
}

const allChannels = computed(() => flattenChannels(chat.channelTree as SChannel[]))

const filteredChannels = computed(() => {
  const keyword = channelSearch.value.trim().toLowerCase()
  if (!keyword) return allChannels.value
  return allChannels.value.filter((channel) => channel.name.toLowerCase().includes(keyword))
})

const channelOptions = computed(() =>
  allChannels.value.map((channel) => ({
    label: channel.name,
    value: channel.id,
  })),
)

const currentFolder = computed(() => folderStore.folders.find((folder) => folder.id === activeFolderId.value))

watch(currentFolder, (folder) => {
  if (folder) {
    renameName.value = folder.name
    renameDesc.value = folder.description || ''
    selectedFolderKeys.value = [folder.id]
  } else {
    renameName.value = ''
    renameDesc.value = ''
    selectedFolderKeys.value = []
  }
})

watch(visible, (val) => {
  if (val) {
    folderStore.ensureLoaded()
  } else {
    selectedChannelIds.value = []
    activeFolderId.value = ''
    channelSearch.value = ''
    syncSource.value = ''
    syncTargets.value = []
    syncScopes.value = ['roles']
    syncResult.value = null
  }
})

const handleTreeSelect = (keys: string[]) => {
  selectedFolderKeys.value = keys
  activeFolderId.value = keys[0] || ''
}

const handleCreateFolder = async () => {
  const name = newFolderName.value.trim()
  if (!name) {
    message.warning('请输入文件夹名称')
    return
  }
  await folderStore.createFolder({
    name,
    description: newFolderDesc.value.trim(),
    parentId: activeFolderId.value || undefined,
  })
  newFolderName.value = ''
  newFolderDesc.value = ''
  message.success('已创建文件夹')
}

const handleSaveFolder = async () => {
  if (!activeFolderId.value) {
    message.warning('请选择文件夹以编辑')
    return
  }
  const name = renameName.value.trim()
  if (!name) {
    message.warning('名称不能为空')
    return
  }
  await folderStore.updateFolder(activeFolderId.value, {
    name,
    description: renameDesc.value.trim(),
  })
  message.success('已更新文件夹')
}

const handleDeleteFolder = () => {
  if (!activeFolderId.value) {
    message.warning('请选择要删除的文件夹')
    return
  }
  const folder = currentFolder.value
  if (!folder) return
  dialog.warning({
    title: '删除文件夹',
    content: `确认删除「${folder.name}」？其中的频道不会被删除。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      await folderStore.deleteFolder(folder.id)
      message.success('已删除文件夹')
      activeFolderId.value = ''
    },
  })
}

const handleToggleFavorite = async () => {
  if (!activeFolderId.value) {
    message.warning('请选择文件夹')
    return
  }
  const favored = favoriteFolderSet.value.has(activeFolderId.value)
  await folderStore.toggleFavorite(activeFolderId.value, !favored)
}

const ensureChannelSelection = () => {
  if (!selectedChannelIds.value.length) {
    message.warning('请选择频道')
    return false
  }
  return true
}

const handleAssign = async (mode: 'append' | 'replace' | 'remove') => {
  if (!ensureChannelSelection()) return
  const folderIds = activeFolderId.value ? [activeFolderId.value] : []
  if (mode !== 'remove' && !folderIds.length) {
    message.warning('请选择目标文件夹')
    return
  }
  await folderStore.assignChannels({
    folderIds,
    channelIds: [...selectedChannelIds.value],
    mode,
    includeChildren: includeChildren.value,
  })
  message.success('操作成功')
}

const getChannelFolderNames = (channelId: string) => {
  const folderIds = channelFolderMap.value.get(channelId) || []
  return folderIds.map((id) => folderStore.folders.find((folder) => folder.id === id)?.name).filter(Boolean)
}

const handleSync = async () => {
  if (!syncSource.value) {
    message.warning('请选择主频道')
    return
  }
  if (!syncTargets.value.length) {
    message.warning('请选择目标频道')
    return
  }
  syncLoading.value = true
  try {
    const result = await folderStore.syncChannelConfig({
      sourceChannelId: syncSource.value,
      targetChannelIds: syncTargets.value,
      scopes: syncScopes.value,
    })
    syncResult.value = result
    message.success('已同步配置')
  } catch (error: any) {
    const msg = error?.response?.data?.error || error?.message || '同步失败'
    message.error(msg)
  } finally {
    syncLoading.value = false
  }
}
</script>

<template>
  <n-drawer v-model:show="visible" placement="left" :width="960" class="channel-manager-panel">
    <template #header>
      <div class="flex items-center justify-between w-full">
        <span>频道管理</span>
        <n-button quaternary size="small" @click="visible = false">关闭</n-button>
      </div>
    </template>

    <n-tabs type="line">
      <n-tab-pane name="structure" tab="组织结构">
        <div class="manager-grid">
          <section class="folder-column">
            <header class="section-header">
              <div class="title">文件夹</div>
              <div class="actions">
                <n-input v-model:value="newFolderName" size="small" placeholder="新建文件夹" />
                <n-input v-model:value="newFolderDesc" size="small" placeholder="描述" />
                <n-button size="small" type="primary" @click="handleCreateFolder">创建</n-button>
              </div>
            </header>
            <n-tree
              block-line
              :data="treeData"
              :selected-keys="selectedFolderKeys"
              default-expand-all
              selectable
              @update:selected-keys="handleTreeSelect"
            />
            <div v-if="currentFolder" class="folder-editor">
              <div class="editor-row">
                <span>文件夹名称</span>
                <n-input v-model:value="renameName" size="small" />
              </div>
              <div class="editor-row">
                <span>描述</span>
                <n-input v-model:value="renameDesc" size="small" />
              </div>
              <div class="editor-actions">
                <n-button size="small" type="primary" @click="handleSaveFolder">保存</n-button>
                <n-button size="small" @click="handleToggleFavorite">
                  {{ favoriteFolderSet.has(activeFolderId) ? '取消收藏' : '收藏文件夹' }}
                </n-button>
                <n-button size="small" type="error" @click="handleDeleteFolder">删除</n-button>
              </div>
            </div>
            <n-empty v-else description="选择一个文件夹以编辑" />
          </section>

          <section class="channel-column">
            <header class="section-header">
              <div class="title">频道列表</div>
              <div class="actions">
                <n-input v-model:value="channelSearch" size="small" placeholder="搜索频道" clearable />
              </div>
            </header>
            <div class="channel-toolbar">
              <span>已选择 {{ selectedChannelIds.length }} 个频道</span>
              <div class="flex items-center space-x-2">
                <n-switch v-model:value="includeChildren">包含子频道</n-switch>
                <n-button size="tiny" @click="handleAssign('append')">添加至文件夹</n-button>
                <n-button size="tiny" @click="handleAssign('replace')">替换</n-button>
                <n-button size="tiny" @click="handleAssign('remove')">移出</n-button>
              </div>
            </div>
            <n-checkbox-group v-model:value="selectedChannelIds">
              <div class="channel-list">
                <div v-for="channel in filteredChannels" :key="channel.id" class="channel-row">
                  <div class="left">
                    <n-checkbox :value="channel.id">{{ channel.name }}</n-checkbox>
                    <span class="meta">{{ channel.permType === 'non-public' ? '非公开' : '公开' }}</span>
                  </div>
                  <div class="folders">
                    <n-tag v-for="folder in getChannelFolderNames(channel.id)" :key="folder" size="small">
                      {{ folder }}
                    </n-tag>
                  </div>
                </div>
              </div>
            </n-checkbox-group>
          </section>
        </div>
      </n-tab-pane>

      <n-tab-pane name="config" tab="配置覆写">
        <div class="sync-section">
          <n-form label-placement="top">
            <n-form-item label="主频道">
              <n-select v-model:value="syncSource" :options="channelOptions" placeholder="选择主频道" />
            </n-form-item>
            <n-form-item label="目标频道">
              <n-select v-model:value="syncTargets" :options="channelOptions" placeholder="选择目标频道" multiple clearable />
            </n-form-item>
            <n-form-item label="同步范围">
              <n-checkbox-group v-model:value="syncScopes">
                <n-checkbox value="roles">角色与权限</n-checkbox>
              </n-checkbox-group>
            </n-form-item>
            <n-form-item>
              <n-button type="primary" :loading="syncLoading" @click="handleSync">执行同步</n-button>
            </n-form-item>
          </n-form>
          <div v-if="syncResult" class="sync-result">
            <h4>同步结果</h4>
            <n-timeline>
              <n-timeline-item
                v-for="item in syncResult.targets"
                :key="item.channelId"
                :title="item.channelId"
                :type="item.error ? 'error' : 'success'"
              >
                <template v-if="item.error">
                  <span class="text-red-500">{{ item.error }}</span>
                </template>
                <template v-else>
                  <span>已同步：{{ item.scopes.join(', ') }}</span>
                </template>
              </n-timeline-item>
            </n-timeline>
          </div>
        </div>
      </n-tab-pane>
    </n-tabs>
  </n-drawer>
</template>

<style scoped lang="scss">
.channel-manager-panel {
  .manager-grid {
    display: grid;
    grid-template-columns: 320px 1fr;
    gap: 1rem;
  }

  .folder-column,
  .channel-column {
    border: 1px solid var(--sc-border-color, #e5e7eb);
    border-radius: 0.5rem;
    padding: 1rem;
    background: var(--sc-panel-bg, #fff);
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
    .title {
      font-weight: 600;
    }
    .actions {
      display: inline-flex;
      gap: 0.4rem;
    }
  }

  .folder-editor {
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid rgba(226, 232, 240, 0.8);
    .editor-row {
      display: flex;
      flex-direction: column;
      margin-bottom: 0.5rem;
      span {
        font-size: 0.78rem;
        color: #94a3b8;
        margin-bottom: 0.2rem;
      }
    }
    .editor-actions {
      display: flex;
      gap: 0.5rem;
    }
  }

  .channel-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }

  .channel-list {
    max-height: 440px;
    overflow: auto;
  }

  .channel-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.35rem 0;
    border-bottom: 1px solid rgba(226, 232, 240, 0.5);
    .left {
      display: flex;
      flex-direction: column;
      gap: 0.15rem;
      .meta {
        font-size: 0.75rem;
        color: #94a3b8;
      }
    }
    .folders {
      display: inline-flex;
      gap: 0.25rem;
      flex-wrap: wrap;
      justify-content: flex-end;
      max-width: 260px;
    }
  }

  .sync-section {
    max-width: 520px;
  }

  .sync-result {
    margin-top: 1rem;
  }
}
</style>
