<script setup lang="ts">
import dayjs from 'dayjs'
import { h, onMounted, reactive, ref } from 'vue'
import { NButton, NSpace, useDialog, useMessage } from 'naive-ui'
import { api } from '@/stores/_config'

interface TemplateItem {
  id: string
  ref: string
  name: string
  sheetType: string
  content?: string
  badgeTemplateOverride?: string
  theaterOverlayTemplateJson?: string
  enabled: boolean
  references: number
  updatedAt?: string
}

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const modalVisible = ref(false)
const editingId = ref('')
const items = ref<TemplateItem[]>([])
const page = ref(1)
const pageSize = ref(100)
const total = ref(0)
const form = reactive({
  name: '', sheetType: '', content: '', enabled: true,
  badgeOverrideEnabled: false, badgeTemplateOverride: '',
  theaterOverrideEnabled: false, theaterOverlayTemplateJson: '',
})

const reset = () => {
  editingId.value = ''
  Object.assign(form, {
    name: '', sheetType: '', content: '', enabled: true,
    badgeOverrideEnabled: false, badgeTemplateOverride: '',
    theaterOverrideEnabled: false, theaterOverlayTemplateJson: '',
  })
}

const columns = [
  { title: '名称', key: 'name' },
  { title: '规则类型', key: 'sheetType' },
  { title: '状态', key: 'enabled', render: (row: TemplateItem) => row.enabled ? '启用' : '停用' },
  { title: '引用数', key: 'references' },
  { title: '更新时间', key: 'updatedAt', render: (row: TemplateItem) => row.updatedAt ? dayjs(row.updatedAt).format('YYYY-MM-DD HH:mm') : '—' },
  { title: '操作', key: 'actions', render: (row: TemplateItem) => h(NSpace, { size: 'small' }, { default: () => [
    h(NButton, { size: 'small', tertiary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
    h(NButton, { size: 'small', tertiary: true, onClick: () => toggleEnabled(row) }, { default: () => row.enabled ? '停用' : '启用' }),
    h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => remove(row) }, { default: () => '删除' }),
  ] }) },
]

const load = async (requestedPage = page.value) => {
  loading.value = true
  try {
    const { data } = await api.get<{ items: TemplateItem[]; page?: number; pageSize?: number; total?: number }>('api/v1/admin/character-card-templates', { params: { page: requestedPage, pageSize: pageSize.value } })
    items.value = Array.isArray(data?.items) ? data.items : []
    page.value = Number(data?.page || requestedPage)
    pageSize.value = Number(data?.pageSize || pageSize.value)
    total.value = Number(data?.total || 0)
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.response?.data?.error || '读取人物卡模板失败')
  } finally { loading.value = false }
}

const changePage = (nextPage: number) => {
  page.value = nextPage
  void load(nextPage)
}

const openCreate = () => { reset(); modalVisible.value = true }
const openEdit = async (row: TemplateItem) => {
  reset()
  try {
    const { data } = await api.get<{ item: TemplateItem }>(`api/v1/admin/character-card-templates/${row.id}`)
    const item = data.item
    editingId.value = item.id
    Object.assign(form, {
      name: item.name, sheetType: item.sheetType, content: item.content || '', enabled: item.enabled,
      badgeOverrideEnabled: !!item.badgeTemplateOverride,
      badgeTemplateOverride: item.badgeTemplateOverride || '',
      theaterOverrideEnabled: !!item.theaterOverlayTemplateJson,
      theaterOverlayTemplateJson: item.theaterOverlayTemplateJson || '',
    })
    modalVisible.value = true
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.response?.data?.error || '读取模板详情失败')
  }
}

const validateOverlay = (value: string) => {
  if (!value.trim()) return true
  try {
    const parsed = JSON.parse(value)
    if (parsed?.version !== 1 || !Array.isArray(parsed.items)) throw new Error('version/items')
    return true
  } catch {
    message.warning('小剧场浮窗模板必须是 version=1 且 items 为数组的 JSON')
    return false
  }
}

const save = async () => {
  if (!form.name.trim() || !form.content.trim()) return message.warning('名称、模板内容不能为空')
  if (form.theaterOverrideEnabled && !validateOverlay(form.theaterOverlayTemplateJson)) return
  const payload = {
    name: form.name.trim(), sheetType: form.sheetType.trim(), content: form.content,
    badgeTemplateOverride: form.badgeOverrideEnabled ? form.badgeTemplateOverride : '',
    theaterOverlayTemplateJson: form.theaterOverrideEnabled ? form.theaterOverlayTemplateJson : '',
    enabled: form.enabled,
  }
  try {
    if (editingId.value) await api.patch(`api/v1/admin/character-card-templates/${editingId.value}`, payload)
    else await api.post('api/v1/admin/character-card-templates', payload)
    modalVisible.value = false
    await load()
    message.success('平台人物卡模板已保存')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.response?.data?.error || '保存失败')
  }
}

const toggleEnabled = (row: TemplateItem) => {
  dialog.warning({ title: row.enabled ? '停用模板' : '启用模板', content: `确认${row.enabled ? '停用' : '启用'}「${row.name}」？`, positiveText: '确认', negativeText: '取消', async onPositiveClick() {
    try {
      await api.post(`api/v1/admin/character-card-templates/${row.id}/${row.enabled ? 'disable' : 'enable'}`)
      await load()
    } catch (error: any) { message.error(error?.response?.data?.message || error?.response?.data?.error || '状态更新失败') }
  } })
}

const remove = (row: TemplateItem) => {
  if (row.references > 0) return message.warning(`模板仍被 ${row.references} 个人物卡引用，请先解除引用`)
  dialog.warning({ title: '删除模板', content: `确认删除「${row.name}」？`, positiveText: '删除', negativeText: '取消', async onPositiveClick() {
    try {
      await api.delete(`api/v1/admin/character-card-templates/${row.id}`)
      await load()
      message.success('模板已删除')
    } catch (error: any) { message.error(error?.response?.data?.message || error?.response?.data?.error || '删除失败') }
  } })
}

onMounted(load)
</script>

<template>
  <div class="admin-character-card-templates">
    <n-space justify="end" class="mb-3"><n-button type="primary" @click="openCreate">创建平台模板</n-button></n-space>
    <n-spin :show="loading"><n-data-table :columns="columns" :data="items" :bordered="false" /></n-spin>
    <n-space justify="end" class="mt-3"><n-pagination :page="page" :page-size="pageSize" :item-count="total" @update:page="changePage" /></n-space>
    <n-modal v-model:show="modalVisible" preset="card" :title="editingId ? '编辑平台人物卡模板' : '创建平台人物卡模板'" style="width: min(820px, 94vw);">
      <n-form label-placement="left" label-width="150">
        <n-form-item label="名称"><n-input v-model:value="form.name" /></n-form-item>
        <n-form-item label="规则类型"><n-input v-model:value="form.sheetType" /></n-form-item>
        <n-form-item label="人物卡模板 Content"><n-input v-model:value="form.content" type="textarea" :rows="12" /></n-form-item>
        <n-form-item label="覆盖频道角色徽标">
          <n-space vertical style="width: 100%"><n-switch v-model:value="form.badgeOverrideEnabled" /><n-input v-if="form.badgeOverrideEnabled" v-model:value="form.badgeTemplateOverride" type="textarea" :rows="3" /></n-space>
        </n-form-item>
        <n-form-item label="覆盖小剧场数据浮窗">
          <n-space vertical style="width: 100%"><n-switch v-model:value="form.theaterOverrideEnabled" /><n-input v-if="form.theaterOverrideEnabled" v-model:value="form.theaterOverlayTemplateJson" type="textarea" :rows="8" /></n-space>
        </n-form-item>
      </n-form>
      <template #footer><n-space justify="end"><n-button @click="modalVisible = false">取消</n-button><n-button type="primary" @click="save">保存</n-button></n-space></template>
    </n-modal>
  </div>
</template>
