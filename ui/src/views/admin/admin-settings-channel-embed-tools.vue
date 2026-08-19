<script setup lang="ts">
import dayjs from 'dayjs'
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NButton, NSpace, useMessage, useDialog } from 'naive-ui'
import { api } from '@/stores/_config'
import { triggerBlobDownload } from '@/utils/download'

interface TemplateItem {
  ref: string
  origin: string
  name: string
  description?: string
  installable: boolean
  archived?: boolean
  enabled?: boolean
  editable?: boolean
  readOnly?: boolean
  references?: number
  updatedAt?: string
}

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const modalVisible = ref(false)
const editingId = ref('')
const items = ref<TemplateItem[]>([])
const searchText = ref('')
const importFileInputRef = ref<HTMLInputElement | null>(null)
const filteredItems = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  if (!query) return items.value
  return items.value.filter((item) => [item.name, item.description || ''].some((value) => value.toLowerCase().includes(query)))
})
const columns = [
  { title: '名称', key: 'name' },
  { title: '描述', key: 'description' },
  { title: '来源', key: 'origin', render: (row: TemplateItem) => row.origin === 'builtin' ? '内置' : '平台' },
  { title: '状态', key: 'status', render: (row: TemplateItem) => row.archived ? '已归档' : (row.installable ? '可用' : '停用') },
  { title: '引用数', key: 'references', render: (row: TemplateItem) => row.origin === 'platform' ? (row.references || 0) : '—' },
  { title: '上次更新时间', key: 'updatedAt', render: (row: TemplateItem) => row.origin === 'platform' && row.updatedAt ? dayjs(row.updatedAt).format('YYYY-MM-DD HH:mm') : '—' },
  { title: '操作', key: 'actions', render: (row: TemplateItem) => row.origin === 'builtin'
    ? '查看（只读）'
    : h(NSpace, { size: 'small' }, { default: () => [
      h(NButton, { size: 'small', tertiary: true, onClick: () => exportTemplate(row) }, { default: () => '导出' }),
      h(NButton, { size: 'small', tertiary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
      h(NButton, { size: 'small', tertiary: true, onClick: () => setArchived(row, !row.archived) }, { default: () => row.archived ? '恢复' : '归档' }),
      h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => deleteTemplate(row) }, { default: () => '删除' }),
    ] }) },
]
const defaultBridgeCapabilities = 'context.read,user.read,members.read,world.admins.read,characters.read,permissions.read,storage.read,storage.write,events.subscribe,events.publish,messages.send'
const form = reactive({
  name: '', description: '', url: '', embedCode: '', defaultWidth: 640, defaultHeight: 360,
  defaultCollapsed: false, defaultFloating: false, allowPopout: true, enabled: true,
  mediaOptions: { autoPlay: false, autoUnmute: false, autoExpand: false, allowAudio: true, allowVideo: true },
  bridgePolicy: { enabled: true, allowedOrigins: '', capabilities: defaultBridgeCapabilities },
})

const reset = () => {
  editingId.value = ''
  Object.assign(form, { name: '', description: '', url: '', embedCode: '', defaultWidth: 640, defaultHeight: 360, defaultCollapsed: false, defaultFloating: false, allowPopout: true, enabled: true, mediaOptions: { autoPlay: false, autoUnmute: false, autoExpand: false, allowAudio: true, allowVideo: true }, bridgePolicy: { enabled: true, allowedOrigins: '', capabilities: defaultBridgeCapabilities } })
}

const toStringArray = (value: unknown) => {
  if (Array.isArray(value)) return value.map((item) => String(item).trim()).filter(Boolean)
  return String(value ?? '').split(',').map((item) => item.trim()).filter(Boolean)
}

const buildPayload = (source: any) => {
  const width = Number(source?.defaultWidth)
  const height = Number(source?.defaultHeight)
  const bridgeEnabled = source?.bridgePolicy?.enabled
  return {
    name: String(source?.name || '').trim(),
    description: String(source?.description || '').trim(),
    url: String(source?.url || '').trim(),
    embedCode: String(source?.embedCode || '').trim(),
    defaultWidth: Number.isFinite(width) && width > 0 ? width : 640,
    defaultHeight: Number.isFinite(height) && height > 0 ? height : 360,
    defaultCollapsed: !!source?.defaultCollapsed,
    defaultFloating: !!source?.defaultFloating,
    allowPopout: source?.allowPopout !== false,
    mediaOptions: {
      autoPlay: !!source?.mediaOptions?.autoPlay,
      autoUnmute: !!source?.mediaOptions?.autoUnmute,
      autoExpand: !!source?.mediaOptions?.autoExpand,
      allowAudio: source?.mediaOptions?.allowAudio !== false,
      allowVideo: source?.mediaOptions?.allowVideo !== false,
    },
    bridgePolicy: {
      enabled: bridgeEnabled === undefined ? true : !!bridgeEnabled,
      allowedOrigins: toStringArray(source?.bridgePolicy?.allowedOrigins),
      capabilities: toStringArray(source?.bridgePolicy?.capabilities),
    },
    enabled: source?.enabled !== false,
  }
}

const load = async () => {
  loading.value = true
  try {
    const { data } = await api.get<{ items: TemplateItem[] }>('api/v1/admin/channel-embed-tools/templates', { params: { page: 1, pageSize: 100 } })
    items.value = data?.items || []
  } catch (error: any) {
    message.error(error?.response?.data?.message || '读取频道嵌入工具失败')
  } finally { loading.value = false }
}

const openCreate = () => { reset(); modalVisible.value = true }
const openEdit = async (item: TemplateItem) => {
  if (item.origin !== 'platform') return
  reset()
  const id = item.ref.replace(/^platform:/, '')
  try {
    const { data } = await api.get(`api/v1/admin/channel-embed-tools/templates/${id}`)
    const detail = data?.item || {}
    Object.assign(form, {
      ...detail,
      mediaOptions: {
        autoPlay: !!detail.mediaOptions?.autoPlay,
        autoUnmute: !!detail.mediaOptions?.autoUnmute,
        autoExpand: !!detail.mediaOptions?.autoExpand,
        allowAudio: detail.mediaOptions?.allowAudio !== false,
        allowVideo: detail.mediaOptions?.allowVideo !== false,
      },
      bridgePolicy: {
        enabled: !!detail.bridgePolicy?.enabled,
        allowedOrigins: Array.isArray(detail.bridgePolicy?.allowedOrigins) ? detail.bridgePolicy.allowedOrigins.join(',') : '',
        capabilities: Array.isArray(detail.bridgePolicy?.capabilities) ? detail.bridgePolicy.capabilities.join(',') : '',
      },
    })
    editingId.value = id
    modalVisible.value = true
  } catch {
    // list endpoint intentionally metadata-only; edit uses local fields when detail unavailable.
    editingId.value = id
    Object.assign(form, { name: item.name, description: item.description || '' })
    modalVisible.value = true
  }
}

const save = async () => {
  if (!form.name.trim()) return message.warning('模板名称不能为空')
  try {
    const payload = buildPayload(form)
    if (editingId.value) await api.patch(`api/v1/admin/channel-embed-tools/templates/${editingId.value}`, payload)
    else await api.post('api/v1/admin/channel-embed-tools/templates', payload)
    modalVisible.value = false
    await load()
    message.success('模板已保存')
  } catch (error: any) { message.error(error?.response?.data?.message || '保存失败') }
}

const exportTemplate = async (item: TemplateItem) => {
  if (item.origin !== 'platform') return
  const id = item.ref.replace(/^platform:/, '')
  try {
    const { data } = await api.get(`api/v1/admin/channel-embed-tools/templates/${id}`)
    const detail = data?.item || {}
    const payload = {
      type: 'sealchat.channel-embed-template',
      version: 1,
      sourceId: detail.id || id,
      template: buildPayload(detail),
    }
    const fileBase = String(detail.name || item.name || id).trim().replace(/[\\/:*?"<>|]+/g, '_') || id
    triggerBlobDownload(
      new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' }),
      `${fileBase}.channel-embed-template.json`,
    )
    message.success('模板已导出')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '导出失败')
  }
}

const triggerImport = () => {
  importFileInputRef.value?.click()
}

const findExistingTemplate = (sourceId: string, name: string) => {
  if (sourceId) {
    const byId = items.value.find((item) => item.origin === 'platform' && item.ref === `platform:${sourceId}`)
    if (byId) return byId
  }
  const normalizedName = name.trim().toLowerCase()
  const matches = items.value.filter((item) => item.origin === 'platform' && item.name.trim().toLowerCase() === normalizedName)
  if (matches.length > 1) throw new Error('存在多个同名平台模板，请使用包含 sourceId 的导出文件')
  return matches[0]
}

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const raw = JSON.parse(await file.text())
    if (raw?.type && raw.type !== 'sealchat.channel-embed-template') throw new Error('不是频道嵌入模板文件')
    if (raw?.version !== undefined && raw.version !== 1) throw new Error('不支持的模板文件版本')
    const source = raw?.template && typeof raw.template === 'object' ? raw.template : raw
    const payload = buildPayload(source)
    if (!payload.name) throw new Error('模板名称不能为空')
    if (!payload.url && !payload.embedCode) throw new Error('需要提供 URL 或嵌入代码')
    const sourceId = String(raw?.sourceId || raw?.id || source?.id || '').trim()
    const existing = findExistingTemplate(sourceId, payload.name)
    if (existing) {
      const id = existing.ref.replace(/^platform:/, '')
      await api.patch(`api/v1/admin/channel-embed-tools/templates/${id}`, payload)
      await load()
      message.success(`模板「${payload.name}」已覆盖`)
    } else {
      await api.post('api/v1/admin/channel-embed-tools/templates', payload)
      await load()
      message.success(`模板「${payload.name}」已导入`)
    }
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '导入失败')
  } finally {
    input.value = ''
  }
}

const setArchived = (item: TemplateItem, archived: boolean) => {
  const id = item.ref.replace(/^platform:/, '')
  dialog.warning({ title: archived ? '归档模板' : '恢复模板', content: `确认${archived ? '归档' : '恢复'}「${item.name}」？`, positiveText: '确认', negativeText: '取消', async onPositiveClick() {
    try {
      await api.post(`api/v1/admin/channel-embed-tools/templates/${id}/${archived ? 'archive' : 'restore'}`)
      await load()
      message.success(archived ? '模板已归档' : '模板已恢复')
    } catch (error: any) { message.error(error?.response?.data?.message || '操作失败') }
  } })
}

const deleteTemplate = (item: TemplateItem) => {
  if (item.references) {
    message.warning(`模板仍被 ${item.references} 个频道引用，请先解除引用`)
    return
  }
  const id = item.ref.replace(/^platform:/, '')
  dialog.warning({
    title: '删除模板',
    content: `确认删除「${item.name}」？删除后不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await api.delete(`api/v1/admin/channel-embed-tools/templates/${id}`)
        await load()
        message.success('模板已删除')
      } catch (error: any) {
        message.error(error?.response?.data?.message || '删除失败')
      }
    },
  })
}

onMounted(load)
</script>

<template>
  <div class="admin-channel-embed-tools">
    <n-space justify="space-between" align="center" class="mb-3">
      <n-input v-model:value="searchText" clearable placeholder="搜索模板名称或描述" style="width: 280px" />
      <n-space>
        <n-button secondary @click="triggerImport">导入模板</n-button>
        <n-button type="primary" @click="openCreate">创建平台模板</n-button>
      </n-space>
    </n-space>
    <input ref="importFileInputRef" type="file" accept=".json,application/json" class="template-import-input" @change="handleImportFile">
    <n-spin :show="loading">
      <n-data-table :columns="columns" :data="filteredItems" :bordered="false" />
    </n-spin>
    <n-modal v-model:show="modalVisible" preset="card" title="平台频道嵌入模板" style="width: min(720px, 94vw);">
      <n-form label-placement="left" label-width="110">
        <n-form-item label="名称"><n-input v-model:value="form.name" /></n-form-item>
        <n-form-item label="描述"><n-input v-model:value="form.description" type="textarea" /></n-form-item>
        <n-form-item label="URL"><n-input v-model:value="form.url" /></n-form-item>
        <n-form-item label="嵌入代码"><n-input v-model:value="form.embedCode" type="textarea" :rows="4" /></n-form-item>
        <n-form-item label="宽 × 高"><n-space><n-input-number v-model:value="form.defaultWidth" :min="1" /><n-input-number v-model:value="form.defaultHeight" :min="1" /></n-space></n-form-item>
        <n-form-item label="默认状态">
          <n-space size="small" :wrap="true">
            <n-switch v-model:value="form.defaultCollapsed">
              <template #checked>折叠</template>
              <template #unchecked>展开</template>
            </n-switch>
            <n-switch v-model:value="form.defaultFloating">
              <template #checked>弹出</template>
              <template #unchecked>面板</template>
            </n-switch>
            <n-switch v-model:value="form.allowPopout">
              <template #checked>允许弹出</template>
              <template #unchecked>禁止弹出</template>
            </n-switch>
          </n-space>
        </n-form-item>
        <n-form-item label="媒体">
          <n-space size="small" :wrap="true">
            <n-switch v-model:value="form.mediaOptions.allowAudio">
              <template #checked>允许音频</template>
              <template #unchecked>禁用音频</template>
            </n-switch>
            <n-switch v-model:value="form.mediaOptions.allowVideo">
              <template #checked>允许视频</template>
              <template #unchecked>禁用视频</template>
            </n-switch>
            <n-switch v-model:value="form.mediaOptions.autoPlay">
              <template #checked>自动播放</template>
              <template #unchecked>手动播放</template>
            </n-switch>
          </n-space>
        </n-form-item>
        <n-form-item label="Bridge API开关">
          <n-switch v-model:value="form.bridgePolicy.enabled">
            <template #checked>启用</template>
            <template #unchecked>关闭</template>
          </n-switch>
        </n-form-item>
        <n-form-item label="允许来源"><n-input v-model:value="form.bridgePolicy.allowedOrigins" placeholder="以逗号分隔，例如 https://example.com" /></n-form-item>
        <n-form-item label="能力列表"><n-input v-model:value="form.bridgePolicy.capabilities" placeholder="以逗号分隔，例如 resize,fullscreen" /></n-form-item>
      </n-form>
      <template #footer><n-space justify="end"><n-button @click="modalVisible = false">取消</n-button><n-button type="primary" @click="save">保存</n-button></n-space></template>
    </n-modal>
  </div>
</template>

<style scoped>
.template-import-input {
  display: none;
}

.admin-channel-embed-tools :deep(.n-switch) {
  flex-shrink: 0;
}
</style>
