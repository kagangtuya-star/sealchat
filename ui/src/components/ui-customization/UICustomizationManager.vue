<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { ChevronDown, ChevronUp, DotsVertical, Download, Plus, Upload } from '@vicons/tabler'
import UICustomizationEditor from './UICustomizationEditor.vue'
import {
  createManagedUICustomization, deleteManagedUICustomization, importManagedUICustomizations,
  listManagedUICustomizations, reorderManagedUICustomizations, replacePlatformUICustomizationFromImport,
  setWorldPlatformUICustomizationEnabled, updateManagedUICustomization,
  type UICustomizationManageScope,
} from '@/services/ui-customization/api'
import {
  isUICustomizationSafeSession, requestUICustomizationRefresh, setUICustomizationSafeSession,
} from '@/services/ui-customization/runtime'
import { UI_PUBLIC_SURFACES, UI_SURFACE_LABELS } from '@/services/ui-customization/surfaces'
import { buildBundleDocument, buildPackageDocument, downloadJSON, readTransferFile, transferFileName } from '@/services/ui-customization/transfer'
import { DEFAULT_UI_CUSTOMIZATION_MAX_CODE_SIZE_KB } from '@/services/ui-customization/types'
import type { UICustomizationPackage, UICustomizationPackageInput, UICustomizationPolicy } from '@/services/ui-customization/types'

const MAX_PACKAGES = 50

const props = defineProps<{
  scope: UICustomizationManageScope
  /** Used in export file names, e.g. the world name. */
  exportName?: string
}>()

const message = useMessage()
const dialog = useDialog()

const packages = ref<UICustomizationPackage[]>([])
const policy = ref<UICustomizationPolicy | null>(null)
const loading = ref(false)
const loadError = ref('')
const busyId = ref('')
const reordering = ref(false)
const importing = ref(false)
const editorShow = ref(false)
const editorSaving = ref(false)
const editing = ref<UICustomizationPackage | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const replaceFileInput = ref<HTMLInputElement | null>(null)
const replaceTargetId = ref('')
const safeSession = ref(isUICustomizationSafeSession())

const isWorld = computed(() => props.scope.type === 'world')
const scopeKey = computed(() => props.scope.type === 'world' ? `world:${props.scope.worldId}` : 'platform')
const worldJSAllowed = computed(() => !isWorld.value || !!policy.value?.allowWorldJs)
const worldCSSAllowed = computed(() => !isWorld.value || policy.value?.allowWorldCss !== false)
const maxCodeSizeKB = computed(() => policy.value?.maxCodeSizeKB || DEFAULT_UI_CUSTOMIZATION_MAX_CODE_SIZE_KB)
const isPlatformCatalog = (pkg: UICustomizationPackage) => isWorld.value && pkg.scopeType === 'platform'
const isPackageEnabled = (pkg: UICustomizationPackage) => isPlatformCatalog(pkg) ? (pkg.enabled || !!pkg.worldEnabled) : pkg.enabled
const isPackageJSAllowed = (pkg: UICustomizationPackage) => !isWorld.value || isPlatformCatalog(pkg) || worldJSAllowed.value
const ownPackages = computed(() => packages.value.filter(pkg => !isPlatformCatalog(pkg)))
const userPackageCount = computed(() => ownPackages.value.filter(pkg => !pkg.readonly).length)
const full = computed(() => userPackageCount.value >= MAX_PACKAGES)

const errorText = (error: any, fallback: string) => error?.response?.data?.message || error?.message || fallback

// Results are dropped unless they still belong to the current scope.
let loadSeq = 0
const load = async () => {
  const seq = ++loadSeq
  const scope = props.scope
  loading.value = true
  loadError.value = ''
  try {
    const result = await listManagedUICustomizations(scope)
    if (seq !== loadSeq) return
    packages.value = result.packages
    policy.value = result.policy || null
  } catch (error) {
    if (seq !== loadSeq) return
    loadError.value = errorText(error, '加载界面自定义包失败')
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}
watch(scopeKey, () => {
  packages.value = []
  editorShow.value = false
  void load()
}, { immediate: true })

const replace = (pkg: UICustomizationPackage) => {
  packages.value = packages.value.map(item => item.id === pkg.id ? pkg : item)
}
// Every successful write reconciles this tab's runtime right away.
const applied = () => requestUICustomizationRefresh()
const toggleSafeSession = (disabled: boolean) => {
  safeSession.value = disabled
  setUICustomizationSafeSession(disabled)
  message.info(disabled ? '本次会话已停用全部界面自定义' : '本次会话已恢复界面自定义')
}

const openCreate = () => {
  if (full.value) return message.warning(`每个作用域最多 ${MAX_PACKAGES} 个自定义包`)
  editing.value = null
  editorShow.value = true
}
const openEdit = (pkg: UICustomizationPackage) => {
  if (isPlatformCatalog(pkg)) {
    message.info('这是平台提供的样式；世界只能引用或停用，内容由平台统一维护')
    return
  }
  if (pkg.readonly) {
    message.info('内置主题由服务器资源维护；如需修改，请先复制为自定义包')
    return
  }
  editing.value = pkg
  editorShow.value = true
}

const handleSave = async (input: UICustomizationPackageInput) => {
  const scope = props.scope
  const key = scopeKey.value
  editorSaving.value = true
  try {
    if (editing.value) {
      const saved = await updateManagedUICustomization(scope, editing.value.id, input)
      if (key !== scopeKey.value) return
      replace(saved)
    } else {
      const saved = await createManagedUICustomization(scope, input)
      if (key !== scopeKey.value) return
      packages.value = [...packages.value, saved]
    }
    editorShow.value = false
    message.success('已保存并应用')
    applied()
  } catch (error) {
    message.error(errorText(error, '保存失败'))
  } finally {
    editorSaving.value = false
  }
}

const toggleEnabled = async (pkg: UICustomizationPackage, enabled: boolean) => {
  busyId.value = pkg.id
  try {
    if (isPlatformCatalog(pkg)) {
      if (props.scope.type !== 'world') return
      await setWorldPlatformUICustomizationEnabled(props.scope.worldId, pkg.id, enabled)
      packages.value = packages.value.map(item => item.id === pkg.id ? { ...item, worldEnabled: enabled } : item)
    } else {
      replace(await updateManagedUICustomization(props.scope, pkg.id, { enabled }))
    }
    applied()
  } catch (error) {
    message.error(errorText(error, enabled ? '启用失败' : '停用失败'))
  } finally {
    busyId.value = ''
  }
}

const move = async (index: number, delta: number) => {
  if (isPlatformCatalog(packages.value[index])) return
  const target = index + delta
  if (target < 0 || target >= packages.value.length || reordering.value || isPlatformCatalog(packages.value[target])) return
  const previous = packages.value
  const next = [...previous]
  ;[next[index], next[target]] = [next[target], next[index]]
  packages.value = next
  reordering.value = true
  try {
    await reorderManagedUICustomizations(props.scope, next.filter(p => !isPlatformCatalog(p)).map(p => p.id))
    applied()
  } catch (error) {
    packages.value = previous
    message.error(errorText(error, '调整顺序失败，已刷新列表'))
    void load()
  } finally {
    reordering.value = false
  }
}

const duplicate = async (pkg: UICustomizationPackage) => {
  if (full.value) return message.warning(`每个作用域最多 ${MAX_PACKAGES} 个自定义包`)
  // A closed world policy rejects new world JS, so the copy keeps CSS only.
  const dropJS = isWorld.value && !worldJSAllowed.value && !!pkg.js
  busyId.value = pkg.id
  try {
    const copy = await createManagedUICustomization(props.scope, {
      name: `${pkg.name} 副本`.slice(0, 100), description: pkg.description, enabled: false,
      surfaces: pkg.surfaces, css: pkg.css, js: dropJS ? '' : pkg.js,
    })
    // The server appends; place the copy right below its source.
    const index = packages.value.findIndex(item => item.id === pkg.id)
    const ordered = [...packages.value]
    ordered.splice(index < 0 ? ordered.length : index + 1, 0, copy)
    packages.value = ordered
    if (ordered[ordered.length - 1].id !== copy.id) {
      await reorderManagedUICustomizations(props.scope, ordered.filter(p => !isPlatformCatalog(p)).map(p => p.id)).catch(() => load())
    }
    message.success(dropJS ? '已复制为停用副本（平台未开放世界脚本，副本未包含 JavaScript）' : '已复制为停用副本')
  } catch (error) {
    message.error(errorText(error, '复制失败'))
  } finally {
    busyId.value = ''
  }
}

const remove = (pkg: UICustomizationPackage) => {
  dialog.warning({
    title: '删除界面自定义包',
    content: `确定删除「${pkg.name}」吗？CSS 与 JavaScript 会一并删除，无法恢复。可以先导出备份。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteManagedUICustomization(props.scope, pkg.id)
        packages.value = packages.value.filter(item => item.id !== pkg.id)
        message.success('已删除')
        applied()
      } catch (error) {
        message.error(errorText(error, '删除失败'))
        return false
      }
    },
  })
}

const exportOne = (pkg: UICustomizationPackage) => downloadJSON(buildPackageDocument(pkg), transferFileName(pkg.name, 'package'))
const exportAll = () => {
  const exportPackages = isWorld.value ? ownPackages.value : packages.value
  if (!exportPackages.length) return
  const name = props.exportName || (isWorld.value ? 'world' : 'platform')
  downloadJSON(buildBundleDocument(exportPackages), transferFileName(name, 'bundle'))
}

const handleImport = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const scope = props.scope
  const key = scopeKey.value
  importing.value = true
  try {
    const document = await readTransferFile(file)
    const created = await importManagedUICustomizations(scope, document)
    if (key !== scopeKey.value) return
    packages.value = isWorld.value
      ? [...packages.value.filter(isPlatformCatalog), ...ownPackages.value, ...created]
      : [...packages.value, ...created]
    message.success(`已导入 ${created.length} 个包（默认停用，检查内容后再启用）`)
  } catch (error) {
    message.error(`导入失败：${errorText(error, '未知错误')}`)
  } finally {
    importing.value = false
  }
}

const beginReplaceImport = (pkg: UICustomizationPackage) => {
  if (isWorld.value || pkg.readonly) return
  replaceTargetId.value = pkg.id
  replaceFileInput.value?.click()
}

const handleReplaceImport = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  const id = replaceTargetId.value
  replaceTargetId.value = ''
  if (!file || !id) return
  busyId.value = id
  try {
    const document = await readTransferFile(file)
    replace(await replacePlatformUICustomizationFromImport(id, document))
    message.success('已覆盖更新，引用该样式的世界会自动使用新内容')
    applied()
  } catch (error) {
    message.error(`覆盖更新失败：${errorText(error, '未知错误')}`)
  } finally {
    busyId.value = ''
  }
}

const platformMoreOptions = [
  { label: '覆盖导入', key: 'replace-import' },
  { label: '导出', key: 'export' },
  { label: '复制', key: 'duplicate' },
  { type: 'divider', key: 'd' },
  { label: '删除', key: 'delete', props: { style: 'color: #dc2626' } },
]
const moreOptions = [
  { label: '导出', key: 'export' },
  { label: '复制', key: 'duplicate' },
  { type: 'divider', key: 'd' },
  { label: '删除', key: 'delete', props: { style: 'color: #dc2626' } },
]
const handleMore = (key: string, pkg: UICustomizationPackage) => {
  if (key === 'export') exportOne(pkg)
  else if (key === 'replace-import') beginReplaceImport(pkg)
  else if (key === 'duplicate') void duplicate(pkg)
  else if (key === 'delete') remove(pkg)
}

const surfaceLabels = (pkg: UICustomizationPackage) => pkg.surfaces.map(s => ({
  key: s, label: UI_SURFACE_LABELS[s] || s, public: UI_PUBLIC_SURFACES.includes(s),
}))
const sizeLabel = (text: string) => {
  const n = new TextEncoder().encode(text).length
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KiB`
}
</script>

<template>
  <div class="sc-uic-manager">
    <input ref="fileInput" type="file" accept=".json,.zip,application/json,application/zip" class="sc-uic-manager__file" @change="handleImport">
    <input ref="replaceFileInput" type="file" accept=".json,.zip,application/json,application/zip" class="sc-uic-manager__file" @change="handleReplaceImport">

    <div class="sc-uic-manager__toolbar">
      <n-button type="primary" size="small" :disabled="full" @click="openCreate">
        <template #icon><n-icon><Plus /></n-icon></template>新建
      </n-button>
      <n-button secondary size="small" :loading="importing" :disabled="full" @click="fileInput?.click()">
        <template #icon><n-icon><Upload /></n-icon></template>导入
      </n-button>
      <n-button secondary size="small" :disabled="isWorld ? !ownPackages.length : !packages.length" @click="exportAll">
        <template #icon><n-icon><Download /></n-icon></template>导出全部
      </n-button>
      <span class="sc-uic-manager__count">{{ userPackageCount }}/{{ MAX_PACKAGES }} 自定义包</span>
    </div>

    <div class="sc-uic-manager__notes">
      <template v-if="isWorld && policy">
        <n-tag size="small" :bordered="false" :type="worldCSSAllowed ? 'success' : 'warning'">
          世界 CSS：{{ worldCSSAllowed ? '平台已开放' : '平台已关闭' }}
        </n-tag>
        <n-tag size="small" :bordered="false" :type="worldJSAllowed ? 'success' : 'default'">
          世界脚本：{{ worldJSAllowed ? '平台已开放，成员需逐个信任' : '平台未开放世界脚本' }}
        </n-tag>
      </template>
      <span>代码总大小上限：{{ maxCodeSizeKB }} KiB（CSS + JavaScript）</span>
      <span>导入支持 JSON 或 ZIP；ZIP 会自动识别其中的单包与 Bundle（sealchat.ui-package / sealchat.ui-bundle）。页面异常时在地址中加入 <code>safe-ui=1</code> 可跳过全部自定义。</span>
      <label class="sc-uic-manager__session-safe">
        <span>本次会话禁用全部自定义</span>
        <n-switch size="small" :value="safeSession" @update:value="toggleSafeSession" />
      </label>
    </div>

    <div v-if="loading && !packages.length" class="sc-uic-manager__state"><n-spin size="small" /></div>
    <n-alert v-else-if="loadError" type="error" :show-icon="false">
      {{ loadError }}
      <n-button size="tiny" secondary style="margin-left: 8px;" @click="load">重试</n-button>
    </n-alert>
    <n-empty v-else-if="!packages.length" description="还没有界面自定义包，可以新建或导入" class="sc-uic-manager__state" />

    <ol v-else class="sc-uic-list">
      <li
        v-for="(pkg, index) in packages"
        :key="pkg.id"
        class="sc-uic-item"
        :class="{ 'is-disabled': !isPackageEnabled(pkg) }"
      >
        <div class="sc-uic-item__order">
          <n-button quaternary size="tiny" :disabled="isPlatformCatalog(pkg) || index === 0 || reordering || isPlatformCatalog(packages[index - 1])" aria-label="上移" @click="move(index, -1)">
            <template #icon><n-icon><ChevronUp /></n-icon></template>
          </n-button>
          <span class="sc-uic-item__index">{{ index + 1 }}</span>
          <n-button quaternary size="tiny" :disabled="isPlatformCatalog(pkg) || index === packages.length - 1 || reordering || isPlatformCatalog(packages[index + 1])" aria-label="下移" @click="move(index, 1)">
            <template #icon><n-icon><ChevronDown /></n-icon></template>
          </n-button>
        </div>

        <div class="sc-uic-item__main" @click="openEdit(pkg)">
          <div class="sc-uic-item__title">
            <span class="sc-uic-item__name">{{ pkg.name }}</span>
            <n-tag v-if="isPlatformCatalog(pkg)" size="tiny" :bordered="false" type="success">
              平台提供{{ pkg.enabled ? ' · 全局启用' : '' }}
            </n-tag>
            <n-tag v-if="pkg.origin === 'builtin'" size="tiny" :bordered="false" type="info">内置 · {{ pkg.ref }}
            </n-tag>
            <span class="sc-uic-badge" :class="pkg.css ? 'is-css' : 'is-empty'">CSS{{ pkg.css ? ` · ${sizeLabel(pkg.css)}` : '' }}</span>
            <span
              v-if="pkg.js"
              class="sc-uic-badge"
              :class="isPackageJSAllowed(pkg) ? 'is-js' : 'is-blocked'"
              :title="isPackageJSAllowed(pkg) ? '' : '平台未开放世界脚本，脚本已保留但不会执行'"
            >JS · {{ sizeLabel(pkg.js) }}{{ isPackageJSAllowed(pkg) ? '' : ' · 未执行' }}</span>
          </div>
          <div v-if="pkg.description" class="sc-uic-item__desc">{{ pkg.description }}</div>
          <div class="sc-uic-item__surfaces">
            <span
              v-for="s in surfaceLabels(pkg)"
              :key="s.key"
              class="sc-uic-surface"
              :class="{ 'is-public': s.public, 'is-global': s.key === 'global' }"
            >{{ s.label }}</span>
          </div>
        </div>

        <div class="sc-uic-item__actions">
          <n-switch
            size="small"
            :value="isPackageEnabled(pkg)"
            :loading="busyId === pkg.id"
            :disabled="isPlatformCatalog(pkg) && pkg.enabled"
            :aria-label="isPackageEnabled(pkg) ? '停用' : '启用'"
            @update:value="(v: boolean) => toggleEnabled(pkg, v)"
          />
          <n-button v-if="!pkg.readonly && !isPlatformCatalog(pkg)" size="small" secondary @click="openEdit(pkg)">编辑</n-button>
          <n-dropdown trigger="click" :options="isPlatformCatalog(pkg) ? [{ label: '导出', key: 'export' }] : (!isWorld && !pkg.readonly ? platformMoreOptions : (pkg.readonly ? [{ label: '复制', key: 'duplicate' }, { label: '导出', key: 'export' }] : moreOptions))" @select="(key: string) => handleMore(key, pkg)">
            <n-button size="small" quaternary aria-label="更多操作">
              <template #icon><n-icon><DotsVertical /></n-icon></template>
            </n-button>
          </n-dropdown>
        </div>
      </li>
    </ol>

    <UICustomizationEditor
      v-model:show="editorShow"
      :scope-type="scope.type"
      :pkg="editing"
      :js-allowed="worldJSAllowed"
      :max-code-size-kb="maxCodeSizeKB"
      :saving="editorSaving"
      @save="handleSave"
    />
  </div>
</template>

<style scoped>
.sc-uic-manager { display: flex; flex-direction: column; gap: 10px; width: 100%; min-width: 0; }
.sc-uic-manager__file { display: none; }
.sc-uic-manager__toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.sc-uic-manager__count { margin-left: auto; font-size: 12px; color: var(--sc-text-secondary, #64748b); font-variant-numeric: tabular-nums; }
.sc-uic-manager__notes { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; font-size: 12px; line-height: 1.6; color: var(--sc-text-secondary, #64748b); }
.sc-uic-manager__notes code { font-family: var(--sc-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace); }
.sc-uic-manager__session-safe { display: inline-flex; align-items: center; gap: 6px; margin-left: auto; white-space: nowrap; }
.sc-uic-manager__state { display: flex; justify-content: center; padding: 20px 0; }
.sc-uic-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
.sc-uic-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px 8px 4px;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.25));
  border-radius: 10px;
  background: var(--sc-bg-surface, rgba(255, 255, 255, 0.02));
  transition: opacity 0.15s;
}
.sc-uic-item.is-disabled .sc-uic-item__main { opacity: 0.55; }
.sc-uic-item__order { display: flex; flex-direction: column; align-items: center; }
.sc-uic-item__index { font-size: 11px; color: var(--sc-text-secondary, #64748b); font-variant-numeric: tabular-nums; }
.sc-uic-item__main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; cursor: pointer; }
.sc-uic-item__title { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.sc-uic-item__name { font-weight: 600; overflow-wrap: anywhere; }
.sc-uic-item__desc {
  font-size: 12px;
  color: var(--sc-text-secondary, #64748b);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.sc-uic-badge {
  padding: 0 6px;
  border-radius: 4px;
  font-size: 11px;
  line-height: 18px;
  font-family: var(--sc-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  border: 1px solid transparent;
}
.sc-uic-badge.is-css { color: #2563eb; background: color-mix(in srgb, #2563eb 12%, transparent); }
.sc-uic-badge.is-empty { color: var(--sc-text-secondary, #94a3b8); border-color: var(--sc-border-mute, rgba(148, 163, 184, 0.3)); }
.sc-uic-badge.is-js { color: #b45309; background: color-mix(in srgb, #d97706 14%, transparent); }
.sc-uic-badge.is-blocked { color: var(--sc-text-secondary, #64748b); border-color: currentColor; text-decoration: line-through; text-decoration-thickness: 1px; }
.sc-uic-item__surfaces { display: flex; flex-wrap: wrap; gap: 4px; }
.sc-uic-surface {
  padding: 0 6px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 18px;
  color: var(--sc-text-secondary, #475569);
  background: color-mix(in srgb, var(--sc-text-secondary, #64748b) 12%, transparent);
}
.sc-uic-surface.is-global { color: var(--sc-text-primary, #0f172a); background: color-mix(in srgb, #3388de 16%, transparent); }
.sc-uic-surface.is-public { color: #b45309; background: color-mix(in srgb, #d97706 14%, transparent); }
.sc-uic-item__actions { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
@media (max-width: 640px) {
  .sc-uic-item { flex-wrap: wrap; padding: 8px; }
  .sc-uic-item__order { flex-direction: row; order: 3; }
  .sc-uic-item__main { flex-basis: calc(100% - 8px); }
  .sc-uic-item__actions { order: 4; margin-left: auto; }
}
</style>
