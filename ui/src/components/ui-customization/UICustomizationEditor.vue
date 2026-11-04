<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDialog } from 'naive-ui'
import { CHANNEL_EXTENSION_CAPABILITIES, isChannelExtensionWriteCapability } from '@/bridge/channelExtensionCapabilities'
import { UI_PUBLIC_SURFACES, UI_SURFACE_LABELS, surfaceCoveredByGlobal, UI_SURFACES, type UISurface } from '@/services/ui-customization/surfaces'
import type { UICustomizationPackage, UICustomizationPackageInput, UICustomizationScopeType } from '@/services/ui-customization/types'

const props = defineProps<{
  show: boolean
  scopeType: UICustomizationScopeType
  /** null creates a new package. */
  pkg: UICustomizationPackage | null
  /** False when the platform has not opened world scripts. */
  jsAllowed: boolean
  maxCodeSizeKb: number
  saving?: boolean
}>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'save', input: UICustomizationPackageInput): void
}>()

const dialog = useDialog()

interface Draft { name: string; description: string; enabled: boolean; surfaces: UISurface[]; css: string; js: string; channelCapabilities: string[] }
const emptyDraft = (): Draft => ({ name: '', description: '', enabled: true, surfaces: ['chat'], css: '', js: '', channelCapabilities: [] })
const draft = ref<Draft>(emptyDraft())
const baseline = ref('')
const codeTab = ref<'css' | 'js'>('css')

const fromPackage = (pkg: UICustomizationPackage | null): Draft => pkg
  ? { name: pkg.name, description: pkg.description, enabled: pkg.enabled, surfaces: [...pkg.surfaces], css: pkg.css, js: pkg.js, channelCapabilities: [...(pkg.channelCapabilities || [])] }
  : emptyDraft()

watch(() => [props.show, props.pkg] as const, ([show]) => {
  if (!show) return
  draft.value = fromPackage(props.pkg)
  baseline.value = JSON.stringify(draft.value)
  codeTab.value = props.pkg && !props.pkg.css && props.pkg.js ? 'js' : 'css'
}, { immediate: true })

const dirty = computed(() => JSON.stringify(draft.value) !== baseline.value)

const ordinarySurfaces = UI_SURFACES.filter(surfaceCoveredByGlobal)
const globalSelected = computed(() => draft.value.surfaces.includes('global'))
const hasSurface = (s: UISurface) => draft.value.surfaces.includes(s)
const toggleSurface = (s: UISurface) => {
  const set = new Set(draft.value.surfaces)
  if (set.has(s)) set.delete(s)
  else set.add(s)
  draft.value.surfaces = UI_SURFACES.filter(item => set.has(item))
}

const encoder = new TextEncoder()
const bytes = (text: string) => encoder.encode(text).length
const cssBytes = computed(() => bytes(draft.value.css))
const jsBytes = computed(() => bytes(draft.value.js))
const maxBytes = computed(() => props.maxCodeSizeKb * 1024)
const codeOver = computed(() => cssBytes.value + jsBytes.value > maxBytes.value)
const formatSize = (n: number) => n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KiB`

// World JS while closed: existing code stays (read-only) and may only be cleared.
const jsLocked = computed(() => props.scopeType === 'world' && !props.jsAllowed)
const savedJS = computed(() => props.pkg?.js || '')

const problems = computed(() => {
  const list: string[] = []
  if (!draft.value.name.trim()) list.push('请填写名称')
  if (!draft.value.surfaces.length) list.push('至少选择一个目标页面')
  if (codeOver.value) list.push(`CSS 与 JavaScript 合计超过 ${props.maxCodeSizeKb} KiB`)
  if (jsLocked.value && draft.value.js && draft.value.js !== savedJS.value) list.push('平台未开放世界脚本')
  return list
})

const close = () => emit('update:show', false)
const requestClose = () => {
  if (!dirty.value) return close()
  dialog.warning({
    title: '放弃未保存的修改？',
    content: '关闭后，本次对名称、目标页面或代码的修改不会保存。',
    positiveText: '放弃修改',
    negativeText: '继续编辑',
    onPositiveClick: close,
  })
}

const submit = () => {
  if (problems.value.length || props.saving) return
  const d = draft.value
  // global already covers every ordinary surface; keep the stored list minimal.
  const surfaces = d.surfaces.includes('global')
    ? d.surfaces.filter(s => s === 'global' || !surfaceCoveredByGlobal(s))
    : d.surfaces
  const input: UICustomizationPackageInput = { name: d.name.trim(), description: d.description.trim(), enabled: d.enabled, surfaces, css: d.css, channelCapabilities: [...d.channelCapabilities] }
  // Only send JS when it changed, so a closed world policy never sees a write.
  if (!props.pkg || d.js !== savedJS.value) input.js = d.js
  emit('save', input)
}

const jsRiskText = computed(() => props.scopeType === 'world'
  ? '世界脚本会在成员浏览器中执行。平台必须允许世界脚本，成员也需要信任当前脚本版本。'
  : '平台脚本会在匹配页面执行。请只保存你信任的代码。')
const jsPlaceholder = `// 直接书写脚本正文，sc 为 SealChat 提供的脚本 API（版本 2，兼容版本 1）
const banner = document.createElement('div')
banner.textContent = '欢迎来到 ' + sc.context().surface
document.body.append(banner)
sc.cleanup(() => banner.remove())

sc.on('message-created', (msg) => console.log(msg.contentText))`
</script>

<template>
  <n-drawer
    :show="show"
    :width="'min(820px, 100vw)'"
    placement="right"
    :mask-closable="false"
    :close-on-esc="false"
    class="sc-uic-editor"
    @update:show="(v: boolean) => !v && requestClose()"
  >
    <n-drawer-content :native-scrollbar="false" body-content-style="padding: 16px 20px;">
      <template #header>
        <div class="sc-uic-editor__header">
          <span>{{ pkg ? '编辑界面自定义包' : '新建界面自定义包' }}</span>
          <n-tag size="small" :bordered="false">{{ scopeType === 'platform' ? '平台' : '世界' }}</n-tag>
          <n-tag v-if="dirty" size="small" type="warning" :bordered="false">未保存</n-tag>
        </div>
      </template>

      <div class="sc-uic-editor__body">
        <div class="sc-uic-editor__meta">
          <n-input v-model:value="draft.name" placeholder="包名称" maxlength="100" show-count />
          <div class="sc-uic-editor__enabled">
            <n-switch v-model:value="draft.enabled" />
            <span>{{ draft.enabled ? '启用' : '停用' }}</span>
          </div>
        </div>
        <n-input
          v-model:value="draft.description"
          type="textarea"
          placeholder="描述（可选）"
          maxlength="500"
          show-count
          :autosize="{ minRows: 1, maxRows: 3 }"
        />

        <section class="sc-uic-editor__section">
          <div class="sc-uic-editor__label">目标页面</div>
          <div class="sc-uic-surfaces">
            <button
              type="button"
              class="sc-uic-chip sc-uic-chip--global"
              :class="{ 'is-active': globalSelected }"
              @click="toggleSurface('global')"
            >{{ UI_SURFACE_LABELS.global }}</button>
            <button
              v-for="s in ordinarySurfaces"
              :key="s"
              type="button"
              class="sc-uic-chip"
              :class="{ 'is-active': hasSurface(s), 'is-covered': globalSelected }"
              :disabled="globalSelected"
              @click="toggleSurface(s)"
            >{{ UI_SURFACE_LABELS[s] }}</button>
          </div>
          <div class="sc-uic-editor__sublabel">公共页面不会被“全部普通页面”覆盖，需要单独选择</div>
          <div class="sc-uic-surfaces">
            <button
              v-for="s in UI_PUBLIC_SURFACES"
              :key="s"
              type="button"
              class="sc-uic-chip sc-uic-chip--public"
              :class="{ 'is-active': hasSurface(s) }"
              @click="toggleSurface(s)"
            >{{ UI_SURFACE_LABELS[s] }}</button>
          </div>
          <div v-if="scopeType === 'world' && draft.surfaces.some(s => UI_PUBLIC_SURFACES.includes(s))" class="sc-uic-editor__hint">
            公共页面只加载世界 CSS（需平台允许），世界脚本不会在 OB、嵌入和 LOG 公开页执行。
          </div>
        </section>

        <section class="sc-uic-editor__section sc-uic-editor__code">
          <div class="sc-uic-editor__hint">CSS 与 JavaScript 合计最多 {{ maxCodeSizeKb }} KiB。</div>
          <n-tabs v-model:value="codeTab" type="segment" size="small" animated>
            <n-tab-pane name="css">
              <template #tab>
                CSS <span class="sc-uic-editor__size" :class="{ 'is-over': codeOver }">{{ formatSize(cssBytes) }}</span>
              </template>
              <n-input
                v-model:value="draft.css"
                type="textarea"
                class="sc-uic-code"
                :autosize="{ minRows: 14, maxRows: 28 }"
                :input-props="{ spellcheck: false, autocapitalize: 'off', autocomplete: 'off' }"
                placeholder="[data-sc-surface='chat'] [data-sc-ui='message'] { line-height: 1.8; }"
              />
              <div class="sc-uic-editor__hint">
                优先使用 <code>data-sc-surface</code> 与 <code>data-sc-ui</code> 选择器，避免依赖内部类名。保存成功后才会应用到页面。
              </div>
            </n-tab-pane>
            <n-tab-pane name="js">
              <template #tab>
                JavaScript <span class="sc-uic-editor__size" :class="{ 'is-over': codeOver }">{{ formatSize(jsBytes) }}</span>
              </template>
              <n-alert type="warning" :show-icon="true" class="sc-uic-editor__risk">
                {{ jsRiskText }}脚本与 SealChat 页面同源运行，可以访问页面与当前登录状态，不是安全沙箱。
              </n-alert>
              <n-alert v-if="jsLocked" type="error" :show-icon="true" class="sc-uic-editor__risk">
                平台未开放世界脚本。{{ savedJS ? '已保存的脚本会保留但不会执行，平台重新开放后恢复；此时只能清空，不能修改。' : '暂时无法为世界包添加 JavaScript。' }}
                <n-button v-if="draft.js" size="tiny" secondary type="error" style="margin-left: 8px;" @click="draft.js = ''">清空脚本</n-button>
                <n-button v-else-if="savedJS" size="tiny" quaternary style="margin-left: 8px;" @click="draft.js = savedJS">撤销清空</n-button>
              </n-alert>
              <n-input
                v-model:value="draft.js"
                type="textarea"
                class="sc-uic-code"
                :readonly="jsLocked"
                :autosize="{ minRows: 14, maxRows: 28 }"
                :input-props="{ spellcheck: false, autocapitalize: 'off', autocomplete: 'off' }"
                :placeholder="jsLocked ? '' : jsPlaceholder"
              />
              <n-collapse class="sc-uic-editor__api">
                <n-collapse-item title="频道能力" name="capabilities">
                  <div class="sc-uic-editor__capability-actions">
                    <n-button size="tiny" secondary :disabled="draft.channelCapabilities.length === CHANNEL_EXTENSION_CAPABILITIES.length" @click="draft.channelCapabilities = [...CHANNEL_EXTENSION_CAPABILITIES]">全部授予</n-button>
                  </div>
                  <n-select v-model:value="draft.channelCapabilities" multiple clearable :options="CHANNEL_EXTENSION_CAPABILITIES.map(value => ({ value, label: value + (isChannelExtensionWriteCapability(value) ? ' · 写能力' : '') }))" placeholder="选择脚本需要的频道能力" />
                  <div class="sc-uic-editor__hint">声明能力与当前频道权限共同决定实际可用 API。修改能力后，成员需要重新确认信任。</div>
                </n-collapse-item>
                <n-collapse-item title="脚本 API 速查（sc.apiVersion = 2）" name="api">
                  <ul class="sc-uic-editor__api-list">
                    <li><code>sc.context()</code> 当前 surface、worldId、channelId、observer、palette</li>
                    <li><code>sc.channel()</code> 当前频道扩展 client；无有效频道时返回 null，切换或卸载时自动销毁</li>
                    <li><code>sc.cleanup(fn)</code> 注册卸载回调；也可以在模块中 <code>export default function activate(sc) { return () =&gt; {} }</code></li>
                    <li><code>sc.on(event, handler)</code> 事件：surface-change、world-change、channel-change、theme-change、message-created、message-updated、message-deleted</li>
                    <li><code>sc.navigate('/path')</code> 站内跳转；<code>sc.notify('info', '文本')</code> 提示</li>
                    <li><code>sc.listen / sc.timeout / sc.interval / sc.observe</code> 卸载时自动清理的监听、定时器与 MutationObserver</li>
                  </ul>
                </n-collapse-item>
              </n-collapse>
            </n-tab-pane>
          </n-tabs>
        </section>
      </div>

      <template #footer>
        <div class="sc-uic-editor__footer">
          <span v-if="problems.length" class="sc-uic-editor__problem">{{ problems[0] }}</span>
          <span v-else class="sc-uic-editor__muted">保存后立即应用到当前页面</span>
          <n-button @click="requestClose">取消</n-button>
          <n-button type="primary" :loading="saving" :disabled="problems.length > 0" @click="submit">保存</n-button>
        </div>
      </template>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.sc-uic-editor__header { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.sc-uic-editor__body { display: flex; flex-direction: column; gap: 14px; }
.sc-uic-editor__meta { display: flex; align-items: center; gap: 12px; }
.sc-uic-editor__meta > :first-child { flex: 1; min-width: 0; }
.sc-uic-editor__enabled { display: flex; align-items: center; gap: 6px; font-size: 13px; white-space: nowrap; }
.sc-uic-editor__section { display: flex; flex-direction: column; gap: 8px; }
.sc-uic-editor__label { font-size: 13px; font-weight: 600; }
.sc-uic-editor__sublabel,
.sc-uic-editor__hint,
.sc-uic-editor__muted { font-size: 12px; color: var(--sc-text-secondary, #64748b); line-height: 1.6; }
.sc-uic-editor__hint code,
.sc-uic-editor__api-list code { font-family: var(--sc-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace); font-size: 12px; }
.sc-uic-surfaces { display: flex; flex-wrap: wrap; gap: 6px; }
.sc-uic-chip {
  padding: 3px 10px;
  border-radius: 999px;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.35));
  background: transparent;
  color: var(--sc-text-secondary, #475569);
  font-size: 12px;
  line-height: 20px;
  cursor: pointer;
  transition: background-color 0.15s, border-color 0.15s, color 0.15s;
}
.sc-uic-chip:hover:not(:disabled) { border-color: var(--sc-accent, #3388de); }
.sc-uic-chip.is-active {
  border-color: var(--sc-accent, #3388de);
  background: color-mix(in srgb, var(--sc-accent, #3388de) 14%, transparent);
  color: var(--sc-text-primary, #0f172a);
}
.sc-uic-chip.is-covered { opacity: 0.55; cursor: default; }
.sc-uic-chip--public.is-active { border-color: #d97706; background: color-mix(in srgb, #d97706 14%, transparent); }
.sc-uic-editor__code :deep(.n-tabs-pane-wrapper) { overflow: visible; }
.sc-uic-editor__size { margin-left: 4px; font-size: 11px; opacity: 0.7; font-variant-numeric: tabular-nums; }
.sc-uic-editor__size.is-over { color: #dc2626; opacity: 1; }
.sc-uic-code :deep(textarea) {
  font-family: var(--sc-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  font-size: 12.5px;
  line-height: 1.55;
  tab-size: 2;
  white-space: pre;
  overflow-x: auto;
}
.sc-uic-editor__risk { margin-bottom: 8px; }
.sc-uic-editor__api { margin-top: 8px; }
.sc-uic-editor__capability-actions { display: flex; justify-content: flex-end; margin-bottom: 6px; }
.sc-uic-editor__api-list { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--sc-text-secondary, #475569); }
.sc-uic-editor__footer { display: flex; align-items: center; justify-content: flex-end; gap: 8px; width: 100%; }
.sc-uic-editor__footer > :first-child { margin-right: auto; }
.sc-uic-editor__problem { font-size: 12px; color: #dc2626; }
@media (max-width: 640px) {
  .sc-uic-editor__meta { flex-direction: column; align-items: stretch; }
}
</style>
